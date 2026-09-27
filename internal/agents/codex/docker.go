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

package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-exit/exitbox/internal/agent"
)

// PackageRoot is where the Codex release package is unpacked. Codex derives
// its package layout from the canonical path of the running executable, so
// $HOME/.local/bin/codex is a symlink into this directory.
const PackageRoot = "/opt/codex"

// packageInstallSnippet installs Codex from its release package. The tarball
// is bind mounted rather than COPYed so the 150 MB archive never becomes an
// image layer, and the voice resources are dropped: they are a third of the
// package and useless in a headless container.
func (c *Codex) packageInstallSnippet(in agent.PrepareBuildInput, packageFile string) string {
	return fmt.Sprintf(`# Install the Codex CLI from its release package. A bare binary is not
# enough: Codex resolves bin/codex, codex-package.json, codex-path/rg and
# codex-resources/bwrap from the running executable, and its app-server
# daemon aborts with "this CLI has no complete local package" without them.
%s
ARG CODEX_VERSION
ARG CODEX_PACKAGE_CHECKSUM
RUN --mount=type=bind,source=%s,target=/tmp/codex-package.tar.gz \
    echo "${CODEX_PACKAGE_CHECKSUM}  /tmp/codex-package.tar.gz" | sha256sum -c - && \
    mkdir -p %s "$HOME/.local/bin" && \
    tar -xzf /tmp/codex-package.tar.gz -C %s --exclude 'codex-resources/voice*' && \
    ln -sf %s/bin/codex "$HOME/.local/bin/codex" && \
    ln -sf %s/bin/codex-code-mode-host "$HOME/.local/bin/codex-code-mode-host" && \
    chmod -R a+rX %s && \
    codex --version`,
		in.InstallStep("bubblewrap"), packageFile,
		PackageRoot, PackageRoot, PackageRoot, PackageRoot, PackageRoot)
}

func (c *Codex) GetDockerfileInstall(buildCtx string) (string, error) {
	return c.installSnippet(agent.PrepareBuildInput{})
}

// installSnippet renders the install step for the image flavor being built.
// Codex publishes static musl binaries only; they run on glibc images too,
// so only the runtime dependency install differs per flavor.
func (c *Codex) installSnippet(in agent.PrepareBuildInput) (string, error) {
	binaryName := c.BinaryName()
	if binaryName == "" {
		return "", fmt.Errorf("unsupported architecture for Codex")
	}
	hostBinaryName := c.CodeModeHostBinaryName()
	if hostBinaryName == "" {
		return "", fmt.Errorf("unsupported architecture for Codex")
	}
	binaryInside := strings.TrimSuffix(binaryName, ".tar.gz")
	hostBinaryInside := strings.TrimSuffix(hostBinaryName, ".tar.gz")

	return fmt.Sprintf(`# Install Codex runtime dependencies and binaries with SHA-256 verification.
# codex-code-mode-host is a sibling binary recent Codex versions spawn for code
# review / "code mode"; it ships as a separate release asset and must sit next to
# the codex binary or the feature fails with "codex-code-mode-host is missing".
%s
ARG CODEX_VERSION
ARG CODEX_CHECKSUM
ARG CODEX_CODE_MODE_HOST_CHECKSUM
COPY %s /tmp/codex.tar.gz
COPY %s /tmp/codex-code-mode-host.tar.gz
RUN echo "${CODEX_CHECKSUM}  /tmp/codex.tar.gz" | sha256sum -c - && \
    echo "${CODEX_CODE_MODE_HOST_CHECKSUM}  /tmp/codex-code-mode-host.tar.gz" | sha256sum -c - && \
    mkdir -p $HOME/.local/bin && \
    tar -xzf /tmp/codex.tar.gz -C /tmp && \
    tar -xzf /tmp/codex-code-mode-host.tar.gz -C /tmp && \
    mv /tmp/%s $HOME/.local/bin/codex && \
    mv /tmp/%s $HOME/.local/bin/codex-code-mode-host && \
    chmod +x $HOME/.local/bin/codex $HOME/.local/bin/codex-code-mode-host && \
    rm -f /tmp/codex.tar.gz /tmp/codex-code-mode-host.tar.gz && \
    $HOME/.local/bin/codex --version`, in.InstallStep("bubblewrap"), binaryName, hostBinaryName, binaryInside, hostBinaryInside), nil
}

func (c *Codex) GetFullDockerfile(version string) (string, error) {
	in := agent.PrepareBuildInput{Version: version}
	install, err := c.installSnippet(in)
	if err != nil {
		return "", err
	}
	df := "FROM " + in.Base() + "\n\n"
	if version != "" {
		df += fmt.Sprintf("ARG CODEX_VERSION=%s\n", version)
	}
	df += install
	return df, nil
}

func (c *Codex) PrepareBuild(in agent.PrepareBuildInput) error {
	version := in.Version
	if version == "" {
		var err error
		version, err = c.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest Codex version: %w", err)
		}
	}
	if in.Download == nil || in.FileSHA256 == nil {
		return fmt.Errorf("PrepareBuildInput.Download and FileSHA256 are required for Codex")
	}

	// The release package is the supported layout. Releases that predate it
	// (or a version pinned to one) fall back to the individual binaries.
	if err := c.prepareFromPackage(in, version); err == nil {
		return nil
	} else if in.Logf != nil {
		in.Logf("Codex release package unavailable (%v); installing the standalone binaries", err)
	}

	binaryName := c.BinaryName()
	if binaryName == "" {
		return fmt.Errorf("unsupported architecture for Codex")
	}
	hostBinaryName := c.CodeModeHostBinaryName()
	if hostBinaryName == "" {
		return fmt.Errorf("unsupported architecture for Codex")
	}
	if in.Download == nil || in.FileSHA256 == nil {
		return fmt.Errorf("PrepareBuildInput.Download and FileSHA256 are required for Codex")
	}
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", codexGitHubRepo, version, binaryName)
	if in.Logf != nil {
		in.Logf("Downloading Codex %s...", version)
	}
	dlPath := filepath.Join(in.BuildDir, binaryName)
	if err := in.Download(in.Ctx, url, dlPath); err != nil {
		return fmt.Errorf("failed to download Codex: %w", err)
	}
	checksum := in.FileSHA256(dlPath)
	if in.Logf != nil {
		in.Logf("Codex SHA-256: %s", checksum)
	}

	// codex-code-mode-host is a separate release asset installed alongside codex.
	hostURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", codexGitHubRepo, version, hostBinaryName)
	if in.Logf != nil {
		in.Logf("Downloading codex-code-mode-host %s...", version)
	}
	hostDlPath := filepath.Join(in.BuildDir, hostBinaryName)
	if err := in.Download(in.Ctx, hostURL, hostDlPath); err != nil {
		return fmt.Errorf("failed to download codex-code-mode-host: %w", err)
	}
	hostChecksum := in.FileSHA256(hostDlPath)
	if in.Logf != nil {
		in.Logf("codex-code-mode-host SHA-256: %s", hostChecksum)
	}

	df := fmt.Sprintf("FROM %s\n\nARG CODEX_VERSION=%s\nARG CODEX_CHECKSUM=%s\nARG CODEX_CODE_MODE_HOST_CHECKSUM=%s\n", in.Base(), version, checksum, hostChecksum)
	install, err := c.installSnippet(in)
	if err != nil {
		return fmt.Errorf("failed to get Codex install instructions: %w", err)
	}
	df += install
	if err := os.WriteFile(in.DockerfilePath, []byte(df), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}

// prepareFromPackage downloads the Codex release package and writes the
// Dockerfile that installs it. It returns an error when the release has no
// package asset, which leaves the caller on the standalone binary path.
func (c *Codex) prepareFromPackage(in agent.PrepareBuildInput, version string) error {
	packageName := c.PackageName()
	if packageName == "" {
		return fmt.Errorf("unsupported architecture for Codex")
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", codexGitHubRepo, version, packageName)
	if in.Logf != nil {
		in.Logf("Downloading Codex package %s...", version)
	}
	dlPath := filepath.Join(in.BuildDir, packageName)
	if err := in.Download(in.Ctx, url, dlPath); err != nil {
		return fmt.Errorf("downloading %s: %w", packageName, err)
	}
	checksum := in.FileSHA256(dlPath)
	if checksum == "" {
		return fmt.Errorf("could not checksum %s", packageName)
	}
	if in.Logf != nil {
		in.Logf("Codex package SHA-256: %s", checksum)
	}

	df := fmt.Sprintf("FROM %s\n\nARG CODEX_VERSION=%s\nARG CODEX_PACKAGE_CHECKSUM=%s\n",
		in.Base(), version, checksum)
	df += c.packageInstallSnippet(in, packageName)
	if err := os.WriteFile(in.DockerfilePath, []byte(df), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}
