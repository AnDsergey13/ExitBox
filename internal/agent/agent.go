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

package agent

import (
	"context"
	"strings"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/container"
)

// Mount describes a volume mount for a container.
type Mount struct {
	Source string
	Target string
}

// PrepareBuildInput holds parameters for preparing an agent's Docker build context.
// It is passed to Agent.PrepareBuild to support download, checksum, and logging.
type PrepareBuildInput struct {
	Ctx            context.Context
	Version        string
	BuildDir       string
	DockerfilePath string
	Download       func(ctx context.Context, url, dest string) error
	FileSHA256     func(path string) string
	Logf           func(format string, args ...interface{})

	// BaseImage is the image this agent layer builds on. Empty means the
	// default Alpine base image.
	BaseImage string
	// Libc is the C library the image's binaries must target, "musl"
	// (Alpine) or "gnu" (the glibc GPU flavor). Empty means musl.
	Libc string
	// Install renders a package install step for the image's distribution.
	// Nil falls back to Alpine's apk.
	Install func(pkgs []string) string
	// Remove renders the matching uninstall step for build-only packages.
	// Nil falls back to Alpine's apk.
	Remove func(pkgs []string) string
	// InstallInline renders a package install command without the RUN
	// prefix, for agents that chain it into a longer step. Nil falls back
	// to Alpine's apk.
	InstallInline func(pkgs []string) string
}

// Base returns the image the agent layer builds on.
func (in PrepareBuildInput) Base() string {
	if in.BaseImage == "" {
		return "exitbox-base"
	}
	return in.BaseImage
}

// IsMusl reports whether the image is musl based, which decides both the
// release artifacts an agent downloads and whether musl-only workarounds
// apply.
func (in PrepareBuildInput) IsMusl() bool {
	return in.Libc == "" || in.Libc == "musl"
}

// LibcTag returns the token upstream projects use in release artifact names.
func (in PrepareBuildInput) LibcTag() string {
	if in.IsMusl() {
		return "musl"
	}
	return in.Libc
}

// InstallStep renders a package install step for the image's distribution.
func (in PrepareBuildInput) InstallStep(pkgs ...string) string {
	if in.Install != nil {
		return in.Install(pkgs)
	}
	return "RUN apk add --no-cache " + strings.Join(pkgs, " ")
}

// InlineInstall renders a package install command without the RUN prefix.
func (in PrepareBuildInput) InlineInstall(pkgs ...string) string {
	if in.InstallInline != nil {
		return in.InstallInline(pkgs)
	}
	return "apk add --no-cache " + strings.Join(pkgs, " ")
}

// RemoveStep renders the uninstall step for build-only packages, without a
// RUN prefix so it can be chained into an existing step.
func (in PrepareBuildInput) RemoveStep(pkgs ...string) string {
	if in.Remove != nil {
		return in.Remove(pkgs)
	}
	return "apk del " + strings.Join(pkgs, " ")
}

type Agent interface {
	Name() string
	DisplayName() string
	Description() string

	GetLatestVersion() (string, error)
	GetInstalledVersion(rt container.Runtime, img string) (string, error)

	GetDockerfileInstall(buildCtx string) (string, error)
	GetFullDockerfile(version string) (string, error)

	HostConfigPaths() []string
	ContainerMounts(cfgDir string) []Mount
	DetectHostConfig() (string, error)
	ImportConfig(src, dst string) error

	GenerateConfig(cfg config.ServerConfig) (map[string]interface{}, error)
	LogSearchDirs(home, agentCfgDir string) []string

	// PrepareBuild prepares the build context (downloads binaries, writes Dockerfile) for the agent's core image.
	PrepareBuild(in PrepareBuildInput) error

	// EnsureWorkspaceAgentConfig creates and seeds agent config dirs for the given workspace.
	EnsureWorkspaceAgentConfig(workspaceName string) error

	OllamaEnvVars(ollamaBaseURL string) []string
	ConfigFilePath(agentDir string) string
	// ExtractConfigServerURLs returns server URLs from parsed agent config JSON.
	ExtractConfigServerURLs(data map[string]interface{}) []string

	ImportFile(src, dst string) error
}
