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
	"github.com/cloud-exit/exitbox/internal/toolbox"
	"github.com/cloud-exit/exitbox/internal/ui"
	"github.com/cloud-exit/exitbox/internal/wizard"
)

// ToolsHash computes a short hash of the global tool configuration
// (user packages and binary downloads). This hash is stored as a label
// on the tools image and used to detect when it needs rebuilding.
func ToolsHash(cfg *config.Config, d *distro.Distro) string {
	var parts []string
	parts = append(parts, "flavor="+string(d.ID))
	parts = append(parts, cfg.Tools.User...)
	for _, b := range cfg.Tools.Binaries {
		parts = append(parts, b.Name+"="+b.URLPattern)
	}
	parts = append(parts, cfg.ExternalTools...)
	parts = append(parts, "alpine="+validAlpineVersion(cfg.AlpineVersionOrDefault()))
	if versions := toolbox.SortedVersions(cfg); versions != "" {
		parts = append(parts, versions)
	}
	h := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return fmt.Sprintf("%x", h[:8])
}

// BuildTools builds the shared tools image (exitbox-<agent>-tools).
// This layer sits between core and project, containing global tool
// installations (cfg.Tools.User and cfg.Tools.Binaries) that are
// shared across all workspaces. Workspace switches that only differ
// in packages or dev profiles skip this build entirely.
func BuildTools(ctx context.Context, rt container.Runtime, agentName string, force bool, d *distro.Distro) error {
	cfg := config.LoadOrDefault()
	toolsHash := ToolsHash(cfg, d)
	imageName := ToolsImageName(agentName, d)
	coreImage := CoreImageName(agentName, d)
	cmd := container.Cmd(rt)

	// Ensure core image exists
	if err := BuildCore(ctx, rt, agentName, false, AgentVersion, d); err != nil {
		return err
	}

	if !force && !ForceRebuild && rt.ImageExists(imageName) {
		h, _ := rt.ImageInspect(imageName, `{{index .Config.Labels "exitbox.tools.hash"}}`)
		coreCreated, _ := rt.ImageInspect(coreImage, "{{.Created}}")
		toolsCreated, _ := rt.ImageInspect(imageName, "{{.Created}}")
		if h == toolsHash && (coreCreated == "" || toolsCreated == "" || coreCreated <= toolsCreated) {
			return nil
		}
		if h != toolsHash {
			ui.Info("Tool configuration changed. Rebuilding tools image...")
		} else {
			ui.Info("Core image updated, rebuilding tools image...")
		}
	}

	ui.Infof("Building %s tools image with %s...", agentName, cmd)

	buildCtx := filepath.Join(config.Cache, "build-"+agentName+"-tools"+d.ImageSuffix())
	if err := os.MkdirAll(buildCtx, 0755); err != nil {
		return fmt.Errorf("failed to create build context dir: %w", err)
	}

	dockerfilePath := filepath.Join(buildCtx, "Dockerfile")
	var df strings.Builder

	df.WriteString("# syntax=docker/dockerfile:1\n")
	fmt.Fprintf(&df, "FROM %s\n\n", coreImage)

	// Switch to root for package installation
	df.WriteString("USER root\n\n")

	// Install global tools (from tool categories). Package names are Alpine
	// names; other flavors translate them and report what they cannot
	// provide, which the binary fallbacks below then install.
	userPkgs, unavailable := d.Packages(cfg.Tools.User)
	if len(userPkgs) > 0 {
		fmt.Fprintf(&df, "%s\n\n", d.InstallSnippet(cfg.Tools.User))
	}
	for _, name := range unavailable {
		fb, ok := distro.BinaryFallbacks[name]
		if !ok {
			ui.Warnf("%s has no %s package for %q; install it as a binary in config.yaml (tools.binaries)", d.DisplayName, d.DisplayName, name)
			continue
		}
		fmt.Fprintf(&df, "# %s (no %s package, official binary)\n%s\n\n", name, d.DisplayName, distro.BinaryInstallSnippet(name, fb))
	}

	// Install binary tools (from config)
	for _, b := range cfg.Tools.Binaries {
		df.WriteString(fmt.Sprintf("# Install %s (binary download)\n", b.Name))
		df.WriteString("RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \\\n")
		url := strings.ReplaceAll(b.URLPattern, "{arch}", "${ARCH}")
		df.WriteString(fmt.Sprintf("    curl -sL \"%s\" -o /usr/local/bin/%s && \\\n", url, b.Name))
		df.WriteString(fmt.Sprintf("    chmod +x /usr/local/bin/%s\n\n", b.Name))
	}

	for _, step := range wizard.ComputeExternalToolInstallStepsForDistro(cfg.ExternalTools, cfg.Tools.Versions, d) {
		df.WriteString(step)
		if !strings.HasSuffix(step, "\n") {
			df.WriteString("\n")
		}
		df.WriteString("\n")
	}

	// Stay as root — project layer handles USER switch
	fmt.Fprintf(&df, "LABEL exitbox.tools.hash=\"%s\"\n", toolsHash)

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

	if err := buildImage(rt, args, fmt.Sprintf("Building %s tools image...", agentName)); err != nil {
		if len(cfg.Tools.User) > 0 {
			ui.Warnf("User tools in config: %s", strings.Join(cfg.Tools.User, ", "))
			ui.Warnf("If a package was not found, check your config: %s", config.ConfigFile())
			ui.Warnf("Packages must be valid Alpine Linux (apk) package names.")
		}
		return fmt.Errorf("failed to build %s tools image: %w", agentName, err)
	}

	ui.Successf("%s tools image built", agentName)
	return nil
}
