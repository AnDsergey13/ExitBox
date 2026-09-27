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

package profile

import (
	"fmt"
	"strings"

	"github.com/cloud-exit/exitbox/internal/toolbox"
)

// Packages returns the space-separated Alpine packages for a role,
// or empty string if the role has none or only custom install steps.
func Packages(name string) string {
	// The node/javascript roles have apk packages too.
	switch name {
	case "node", "javascript":
		return "nodejs npm"
	}
	p := GetRole(name)
	if p == nil {
		return ""
	}
	return p.Packages
}

// CollectPackages returns a deduplicated list of all Alpine packages
// needed by the given profiles, without version pins.
func CollectPackages(profiles []string) []string {
	return CollectVersionedPackages(profiles, nil)
}

// CollectVersionedPackages returns a deduplicated list of all Alpine
// packages needed by the given profiles, applying configured tool version
// pins. Alpine packages owned by a tarball-installed tool (e.g. Go) are
// omitted when that tool has a version configured, and apk-managed tools
// are emitted as pkg=version when an exact version is pinned.
func CollectVersionedPackages(profiles []string, versions map[string]string) []string {
	seen := make(map[string]bool)
	var pkgs []string

	for _, name := range profiles {
		raw := Packages(name)
		t := toolbox.LookupByRole(name)
		ver := ""
		if t != nil {
			ver = versionFor(versions, t.Name)
		}

		rawFields := strings.Fields(raw)
		for _, p := range rawFields {
			if seen[p] {
				continue
			}
			// Tarball-installed tools replace their apk package when a
			// version (including "latest") is configured.
			if t != nil && t.Mode == toolbox.ModeTarball && ver != "" && t.OwnsPackage(p) {
				seen[p] = true
				continue
			}
			spec := p
			if t != nil && t.Mode == toolbox.ModeApk && ver != "" && ver != "latest" && t.OwnsPackage(p) {
				spec = p + "=" + ver
			}
			seen[p] = true
			pkgs = append(pkgs, spec)
		}
		// Some apk-managed languages (e.g. python) are provided by the base
		// image rather than the role's package list. Add an explicit install
		// (pinned or latest) when the tool is active.
		if t != nil && t.Mode == toolbox.ModeApk && ver != "" {
			for _, p := range t.Packages {
				if seen[p] || contains(rawFields, p) {
					continue
				}
				spec := p
				if ver != "latest" {
					spec = p + "=" + ver
				}
				seen[p] = true
				pkgs = append(pkgs, spec)
			}
		}
	}
	return pkgs
}

// contains reports whether list contains s.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// versionFor returns the configured version for name, or "" when no map is
// provided or the entry is "default".
func versionFor(versions map[string]string, name string) string {
	if versions == nil {
		return ""
	}
	v := versions[name]
	if v == "default" || v == "" {
		return ""
	}
	return v
}

// CustomSnippet returns the non-apk Dockerfile instructions for a profile
// (e.g. Go download, Python venv, Flutter install, node npm globals).
// versions carries the configured tool version pins.
// Returns empty string if the profile only needs apk packages.
func CustomSnippet(name string, versions map[string]string) string {
	switch name {
	case "python":
		return `# Python profile - venv with pip, setuptools, wheel
RUN python3 -m venv /home/user/.venv && \
    /home/user/.venv/bin/pip install --upgrade pip setuptools wheel
ENV PATH="/home/user/.venv/bin:$PATH"
`
	case "go":
		var b strings.Builder
		if v := versionFor(versions, "go"); v != "" {
			b.WriteString(goSnippet(v))
			b.WriteString("\n")
		}
		b.WriteString(golangciLintSnippet(versionFor(versions, "golangci-lint")))
		return b.String()
	case "flutter":
		return `RUN set -e && \
    case "$(uname -m)" in \
        x86_64|amd64) FLUTTER_ARCH="x64" ;; \
        aarch64|arm64) FLUTTER_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
    esac && \
    RELEASES_JSON="$(wget -qO- https://storage.googleapis.com/flutter_infra_release/releases/releases_linux.json)" && \
    STABLE_HASH="$(printf '%s' "$RELEASES_JSON" | jq -r '.current_release.stable')" && \
    FLUTTER_ARCHIVE="$(printf '%s' "$RELEASES_JSON" | jq -r --arg h "$STABLE_HASH" --arg a "$FLUTTER_ARCH" '.releases[] | select(.hash == $h and .dart_sdk_arch == $a) | .archive' | head -n1)" && \
    FLUTTER_SHA256="$(printf '%s' "$RELEASES_JSON" | jq -r --arg h "$STABLE_HASH" --arg a "$FLUTTER_ARCH" '.releases[] | select(.hash == $h and .dart_sdk_arch == $a) | .sha256' | head -n1)" && \
    test -n "$FLUTTER_ARCHIVE" && \
    test -n "$FLUTTER_SHA256" && \
    wget -q -O /tmp/flutter.tar.xz "https://storage.googleapis.com/flutter_infra_release/releases/${FLUTTER_ARCHIVE}" && \
    echo "${FLUTTER_SHA256}  /tmp/flutter.tar.xz" | sha256sum -c - && \
    rm -rf /opt/flutter && \
    mkdir -p /opt && \
    tar -xJf /tmp/flutter.tar.xz -C /opt && \
    rm -f /tmp/flutter.tar.xz && \
    ln -sf /opt/flutter/bin/flutter /usr/local/bin/flutter && \
    ln -sf /opt/flutter/bin/dart /usr/local/bin/dart
`
	case "node", "javascript":
		// apk packages (nodejs npm) are collected separately;
		// this is just the npm global installs.
		return `RUN npm install -g typescript eslint prettier yarn pnpm
`
	case "ml":
		// Install AI/ML Python tooling into the venv created by the python profile.
		// huggingface_hub[cli] provides the `huggingface-cli` command.
		return `# ML profile - AI/ML tooling (huggingface-cli, etc.) into python venv
RUN /home/user/.venv/bin/pip install --no-cache-dir \
    "huggingface_hub[cli]" \
    safetensors
`
	}
	return ""
}

// goSnippet installs the official Go toolchain from go.dev. version may be
// an exact version like "1.35.0" or "latest" to resolve during image build.
func goSnippet(version string) string {
	if version == "" {
		return ""
	}
	resolve := ""
	goVersion := version
	if version == "latest" {
		resolve = "    GO_VERSION=\"$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1 | sed 's/^go//')\" && \\\n"
		goVersion = "${GO_VERSION}"
	}
	return fmt.Sprintf(`# Go profile - official Go toolchain (%s)
RUN set -e && \
    case "$(uname -m)" in \
        x86_64|amd64) GO_ARCH="amd64" ;; \
        aarch64|arm64) GO_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
    esac && \
%s    GO_FILE="go%s.linux-${GO_ARCH}.tar.gz" && \
    GO_SHA256="$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | jq -r --arg f "$GO_FILE" '.[] | .files[] | select(.filename == $f) | .sha256')" && \
    test -n "$GO_SHA256" && \
    curl -fsSL "https://go.dev/dl/$GO_FILE" -o "/tmp/$GO_FILE" && \
    echo "${GO_SHA256}  /tmp/${GO_FILE}" | sha256sum -c - && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf "/tmp/$GO_FILE" && \
    rm -f "/tmp/$GO_FILE" && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt && \
    go version
`, version, resolve, goVersion)
}

// golangciLintSnippet installs golangci-lint from GitHub releases. An empty
// version uses the built-in default; "latest" resolves during image build.
func golangciLintSnippet(version string) string {
	if version == "" {
		version = "1.64.8"
	}
	assign := `LINT_VERSION="` + version + `"`
	if version == "latest" {
		assign = `LINT_VERSION="$(curl -fsSL https://api.github.com/repos/golangci/golangci-lint/releases/latest | jq -r '.tag_name | ltrimstr("v")')"`
	}
	return fmt.Sprintf(`# golangci-lint (v%s)
RUN set -e && \
    case "$(uname -m)" in \
        x86_64|amd64) LINT_ARCH="amd64" ;; \
        aarch64|arm64) LINT_ARCH="arm64" ;; \
        *) echo "Unsupported architecture" >&2; exit 1 ;; \
    esac && \
    %s && \
    wget -q -O /tmp/golangci-lint.tar.gz "https://github.com/golangci/golangci-lint/releases/download/v${LINT_VERSION}/golangci-lint-${LINT_VERSION}-linux-${LINT_ARCH}.tar.gz" && \
    tar -xzf /tmp/golangci-lint.tar.gz -C /tmp && \
    mv /tmp/golangci-lint-${LINT_VERSION}-linux-${LINT_ARCH}/golangci-lint /usr/local/bin/golangci-lint && \
    chmod +x /usr/local/bin/golangci-lint && \
    rm -rf /tmp/golangci-lint*
`, version, assign)
}

// DockerfileSnippet returns the full Dockerfile instructions for a profile.
// Deprecated: use CollectPackages + CustomSnippet for batched apk installs.
func DockerfileSnippet(name string) string {
	pkgs := Packages(name)
	custom := CustomSnippet(name, nil)

	var parts []string
	if pkgs != "" {
		parts = append(parts, "RUN apk add --no-cache "+pkgs)
	}
	if custom != "" {
		parts = append(parts, custom)
	}
	return strings.Join(parts, "\n")
}
