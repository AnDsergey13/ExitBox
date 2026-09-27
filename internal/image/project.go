// ExitBox - Multi-Agent Container Sandbox
// Copyright (C) 2026 Cloud Exit B.V.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package image

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/container"
	"github.com/cloud-exit/exitbox/internal/distro"
	"github.com/cloud-exit/exitbox/internal/plugins"
	"github.com/cloud-exit/exitbox/internal/profile"
	proj "github.com/cloud-exit/exitbox/internal/project"
	"github.com/cloud-exit/exitbox/internal/toolbox"
	"github.com/cloud-exit/exitbox/internal/ui"
)

// WorkspaceDistro returns the image flavor a workspace builds on.
func WorkspaceDistro(cfg *config.Config, projectDir, overrideName string) *distro.Distro {
	ws := activeWorkspace(cfg, projectDir, overrideName)
	return distro.For(ws != nil && ws.GPU)
}

// activeWorkspace resolves the workspace a build or run applies to, or nil
// when the config has no workspaces.
func activeWorkspace(cfg *config.Config, projectDir, overrideName string) *config.Workspace {
	active, _ := profile.ResolveActiveWorkspace(cfg, projectDir, overrideName)
	if active == nil {
		return nil
	}
	return &active.Workspace
}

// WorkspaceHash computes a short hash encoding the active workspace
// configuration for an agent. Each distinct workspace produces a different
// hash, which becomes part of the image name. Global tool config
// (cfg.Tools.User, cfg.Tools.Binaries) is NOT included — those live in the
// shared tools layer and are tracked via its own hash/label. The workspace's
// enabled plugins are included so switching a plugin on or off lands on a
// different project image instead of reusing a stale one.
func WorkspaceHash(cfg *config.Config, agentName, projectDir string, overrideName string) string {
	active, _ := profile.ResolveActiveWorkspace(cfg, projectDir, overrideName)
	var parts []string
	if active != nil {
		parts = append(parts, active.Scope, active.Workspace.Name)
		parts = append(parts, active.Workspace.Development...)
		parts = append(parts, active.Workspace.Packages...)
		if ph := plugins.BuildHash(&active.Workspace, agentName); ph != "" {
			parts = append(parts, "plugins="+ph)
		}
		if active.Workspace.GPU {
			parts = append(parts, "gpu=true")
		}
	}
	parts = append(parts, SessionTools...)
	parts = append(parts, "alpine="+validAlpineVersion(cfg.AlpineVersionOrDefault()))
	if versions := toolbox.SortedVersions(cfg); versions != "" {
		parts = append(parts, versions)
	}
	h := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return fmt.Sprintf("%x", h[:8])
}

// BuildProject builds the agent project image (with workspaces).
// When force is true, the image is rebuilt even if it already exists.
// workspaceOverride selects a specific workspace (empty = use resolution chain).
func BuildProject(ctx context.Context, rt container.Runtime, agentName, projectDir, workspaceOverride string, force bool) error {
	cfg := config.LoadOrDefault()
	wh := WorkspaceHash(cfg, agentName, projectDir, workspaceOverride)
	imageName := proj.ImageName(agentName, projectDir, wh)
	cmd := container.Cmd(rt)

	// A GPU workspace builds on the glibc CUDA flavor; everything else on
	// Alpine.
	d := WorkspaceDistro(cfg, projectDir, workspaceOverride)

	// Ensure tools image exists (tools → core → base cascade)
	if err := BuildTools(ctx, rt, agentName, force, d); err != nil {
		return err
	}

	// Plugins layer sits between tools and project. With no plugins enabled
	// this is the tools image itself.
	baseImage, err := BuildPlugins(ctx, rt, agentName, projectDir, workspaceOverride, force, d)
	if err != nil {
		return err
	}

	// Each workspace has its own image name. If it already exists, skip.
	if !force && rt.ImageExists(imageName) {
		// Check if the layer below is newer (e.g. user changed tool
		// categories, or a plugin was rebuilt).
		baseCreated, _ := rt.ImageInspect(baseImage, "{{.Created}}")
		projectCreated, _ := rt.ImageInspect(imageName, "{{.Created}}")
		if baseCreated == "" || projectCreated == "" || baseCreated <= projectCreated {
			return nil
		}
		ui.Info("Base image updated, rebuilding project image...")
	}

	// Resolve active workspace.
	active, err := profile.ResolveActiveWorkspace(cfg, projectDir, workspaceOverride)
	if err != nil {
		ui.Warnf("Failed to resolve active workspace: %v", err)
	}
	var developmentProfiles []string
	if active != nil {
		developmentProfiles = append(developmentProfiles, active.Workspace.Development...)
	}

	ui.Infof("Building %s project image with %s...", agentName, cmd)

	buildCtx := filepath.Join(config.Cache, "build-"+agentName+"-project")
	if err := os.MkdirAll(buildCtx, 0755); err != nil {
		return fmt.Errorf("failed to create build context dir: %w", err)
	}

	dockerfilePath := filepath.Join(buildCtx, "Dockerfile")
	var df strings.Builder

	df.WriteString("# syntax=docker/dockerfile:1\n")
	fmt.Fprintf(&df, "FROM %s\n\n", baseImage)

	// Switch to root for package installation (tools image stays as root,
	// but be explicit in case that changes)
	df.WriteString("USER root\n\n")

	// Validate all development roles up front.
	for _, p := range developmentProfiles {
		if !profile.RoleExists(p) {
			return fmt.Errorf("unknown development role '%s'. Run 'exitbox setup' to configure your development stack", p)
		}
	}

	// Collect ALL Alpine packages into a single apk add call:
	// workspace packages + profile packages + session tools.
	var allPkgs []string
	if active != nil {
		allPkgs = append(allPkgs, active.Workspace.Packages...)
	}
	allPkgs = append(allPkgs, profile.CollectVersionedPackages(developmentProfiles, cfg.Tools.Versions)...)
	allPkgs = append(allPkgs, SessionTools...)
	if active != nil && active.Workspace.GPU && d.IsDefault() {
		// A GPU workspace on the Alpine flavor (the user pinned it) at least
		// gets the glibc shim, so the injected driver userspace has a chance.
		allPkgs = append(allPkgs, GPUCompatPackages...)
	}
	allPkgs = dedup(allPkgs)

	if len(allPkgs) > 0 {
		mapped, unavailable := d.Packages(allPkgs)
		if len(mapped) > 0 {
			fmt.Fprintf(&df, "%s\n\n", d.InstallSnippet(allPkgs))
		}
		for _, name := range unavailable {
			fb, ok := distro.BinaryFallbacks[name]
			if !ok {
				ui.Warnf("%s has no package for %q; it is not installed in this workspace image", d.DisplayName, name)
				continue
			}
			fmt.Fprintf(&df, "# %s (no %s package, official binary)\n%s\n\n", name, d.DisplayName, distro.BinaryInstallSnippet(name, fb))
		}
	}

	// Add non-apk custom install steps (Go download, Python venv, etc.).
	for _, p := range developmentProfiles {
		snippet := profile.CustomSnippet(p, cfg.Tools.Versions)
		if snippet != "" {
			df.WriteString(snippet)
			df.WriteString("\n")
		}
	}

	// Fix home dir ownership after root package installs
	df.WriteString("RUN chown -R user:user /home/user\n\n")

	// Switch back to non-root user
	df.WriteString("USER user\n")

	if err := os.WriteFile(dockerfilePath, []byte(df.String()), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	args := buildArgs(cmd)
	if force {
		args = append(args, "--no-cache")
	}
	args = append(args,
		"-t", imageName,
		"-f", dockerfilePath,
		buildCtx,
	)

	if err := buildImage(rt, args, fmt.Sprintf("Building %s project image...", agentName)); err != nil {
		return fmt.Errorf("failed to build %s project image: %w", agentName, err)
	}

	ui.Successf("%s project image built", agentName)
	return nil
}

// GPUCompatPackages are added to a GPU-enabled workspace image so the
// driver userspace injected by the container toolkit has its glibc shim and
// C++ runtime available.
var GPUCompatPackages = []string{"gcompat", "libstdc++"}

// dedup returns a new slice with duplicate strings removed, preserving order.
func dedup(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
