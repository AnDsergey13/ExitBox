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

// Package distro describes the operating system an ExitBox image is built
// on. ExitBox is Alpine by default; a workspace with GPU passthrough builds
// on an Ubuntu CUDA base instead, because the driver userspace the NVIDIA
// container toolkit injects, and every CUDA-based ML wheel, are built
// against glibc and cannot load on musl.
//
// Package names throughout ExitBox (development roles, tool categories,
// workspace packages) are written as Alpine names. A distro translates them
// to its own names, and reports the ones it has no equivalent for instead of
// failing the build.
package distro

import (
	"os"
	"sort"
	"strings"
)

// ID names an image flavor.
type ID string

const (
	// Alpine is the default flavor: small, musl based.
	Alpine ID = "alpine"
	// GPU is the CUDA flavor: Ubuntu and glibc, so the injected NVIDIA
	// driver userspace and CUDA wheels work.
	GPU ID = "gpu"
)

// Libc identifies which C library an image's binaries must target.
type Libc string

const (
	Musl Libc = "musl"
	Gnu  Libc = "gnu"
)

// Distro is one image flavor.
type Distro struct {
	ID          ID
	DisplayName string
	Libc        Libc
	// BaseImage is the upstream image the base layer builds on.
	BaseImage string
	// packages maps an Alpine package name to this distro's name. A missing
	// key means the name is used unchanged; an empty value means the distro
	// has no equivalent.
	packages map[string]string
}

// CUDABaseImage is the Ubuntu CUDA runtime image the GPU flavor builds on.
// The runtime image carries the CUDA libraries; the driver itself is
// injected by the container toolkit at run time.
//
// The registry host is part of the name on purpose: Podman does not resolve
// short names without a containers-registries.conf, and unlike "alpine",
// "nvidia/cuda" has no entry in the default shortname aliases.
const CUDABaseImage = "docker.io/nvidia/cuda:12.6.3-cudnn-runtime-ubuntu24.04"

// GPUBaseImageEnv overrides the CUDA base image, for hosts whose driver is
// older or newer than the default image expects (CUDA 12.6 needs driver
// 560+). The value must be fully qualified, e.g.
// docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04.
const GPUBaseImageEnv = "EXITBOX_GPU_BASE_IMAGE"

var registry = map[ID]*Distro{
	Alpine: {
		ID:          Alpine,
		DisplayName: "Alpine",
		Libc:        Musl,
		BaseImage:   "alpine",
	},
	GPU: {
		ID:          GPU,
		DisplayName: "Ubuntu CUDA",
		Libc:        Gnu,
		BaseImage:   CUDABaseImage,
		packages:    debianPackages,
	},
}

// Get returns the flavor with the given id, or the default flavor.
func Get(id ID) *Distro {
	if d, ok := registry[id]; ok {
		return d
	}
	return registry[Alpine]
}

// Default returns the flavor used when a workspace asks for nothing special.
func Default() *Distro { return registry[Alpine] }

// For returns the flavor a workspace needs. GPU workspaces need glibc.
func For(gpu bool) *Distro {
	if gpu {
		return registry[GPU]
	}
	return registry[Alpine]
}

// IsDefault reports whether this is the standard Alpine flavor.
func (d *Distro) IsDefault() bool { return d.ID == Alpine }

// ImageSuffix is appended to image names so the flavors never collide.
func (d *Distro) ImageSuffix() string {
	if d.IsDefault() {
		return ""
	}
	return "-" + string(d.ID)
}

// BaseImageRef returns the upstream image this flavor builds on, honouring
// the GPU base image override.
func (d *Distro) BaseImageRef() string {
	if d.IsDefault() {
		return d.BaseImage
	}
	if v := strings.TrimSpace(os.Getenv(GPUBaseImageEnv)); v != "" {
		return v
	}
	return d.BaseImage
}

// LibcTag returns the libc token used in upstream release artifact names
// ("musl" or "gnu").
func (d *Distro) LibcTag() string { return string(d.Libc) }

// Package translates an Alpine package name. The second return value is
// false when this distro has no equivalent package.
func (d *Distro) Package(name string) (string, bool) {
	if d.packages == nil {
		return name, true
	}
	// Version pins travel with the name ("nodejs=22.14.0-r0"); both apk and
	// apt take pkg=version, so only the name is translated.
	base, version := splitPin(name)
	mapped, ok := d.packages[base]
	if !ok {
		return name, true
	}
	if mapped == "" {
		return "", false
	}
	return joinPin(mapped, version), true
}

// Packages translates a list of Alpine package names, returning the
// translated names and the ones this distro cannot provide.
func (d *Distro) Packages(names []string) (mapped []string, unavailable []string) {
	seen := map[string]bool{}
	for _, name := range names {
		translated, ok := d.Package(name)
		if !ok {
			unavailable = append(unavailable, name)
			continue
		}
		for _, part := range strings.Fields(translated) {
			if !seen[part] {
				seen[part] = true
				mapped = append(mapped, part)
			}
		}
	}
	return mapped, unavailable
}

// NodeVersion is the Node.js release installed on flavors whose
// distribution packages a version older than the agents need. Ubuntu 24.04
// ships Node 18; Copilot needs 22+ and Qwen 20+.
const NodeVersion = "v22.14.0"

// InstallSnippet returns the Dockerfile RUN step installing packages with
// this distro's package manager, or "" when there is nothing to install.
// The whole step is a single RUN so callers can chain onto it.
func (d *Distro) InstallSnippet(names []string) string {
	if d.IsDefault() {
		pkgs, _ := d.Packages(names)
		if len(pkgs) == 0 {
			return ""
		}
		return "RUN --mount=type=cache,target=/var/cache/apk apk add --no-cache " + strings.Join(pkgs, " ")
	}

	// Node comes from nodejs.org rather than apt, so the version matches
	// what the agents require.
	rest, node := splitNode(names)
	pkgs, _ := d.Packages(rest)

	var parts []string
	if len(pkgs) > 0 {
		parts = append(parts, "apt-get update", aptInstallCommand(pkgs), "rm -rf /var/lib/apt/lists/*")
	}
	if node {
		parts = append(parts, nodeInstallCommands()...)
	}
	if len(parts) == 0 {
		return ""
	}
	return "RUN --mount=type=cache,target=/var/cache/apt set -e && \\\n    " +
		strings.Join(parts, " && \\\n    ")
}

// aptInstallCommand installs packages, and on failure retries them one by
// one so a single name Ubuntu does not carry cannot fail the whole image.
// Package names come from workspaces and tool categories written for Alpine,
// and the translation table cannot cover every name a user picks, so an
// unknown one is reported and skipped instead of aborting the build.
func aptInstallCommand(pkgs []string) string {
	list := strings.Join(pkgs, " ")
	install := "DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends"
	return "if ! " + install + " " + list + "; then " +
		`echo "[exitbox] some packages are unavailable on this distribution; installing one by one" >&2; ` +
		"for pkg in " + list + "; do " + install + ` "$pkg" || echo "[exitbox] skipped unavailable package: $pkg" >&2; ` +
		"done; fi"
}

// splitNode separates the Node packages from the rest, reporting whether
// Node was requested at all.
func splitNode(names []string) (rest []string, node bool) {
	for _, name := range names {
		base, _ := splitPin(name)
		if base == "nodejs" || base == "npm" {
			node = true
			continue
		}
		rest = append(rest, name)
	}
	return rest, node
}

// nodeInstallCommands installs the official Node.js build, checksum
// verified against the release's SHASUMS256.txt.
func nodeInstallCommands() []string {
	archive := "node-" + NodeVersion + "-linux-${NODE_ARCH}.tar.xz"
	return []string{
		`case "$(uname -m)" in x86_64|amd64) NODE_ARCH="x64" ;; aarch64|arm64) NODE_ARCH="arm64" ;; *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; esac`,
		"cd /tmp",
		`curl -fsSLO "https://nodejs.org/dist/` + NodeVersion + `/` + archive + `"`,
		`curl -fsSLO "https://nodejs.org/dist/` + NodeVersion + `/SHASUMS256.txt"`,
		`grep " ` + archive + `$" SHASUMS256.txt | sha256sum -c -`,
		`tar -xJf "` + archive + `" -C /usr/local --strip-components=1 --no-same-owner`,
		`rm -f "` + archive + `" SHASUMS256.txt`,
		"node --version && npm --version",
	}
}

// InstallCommand returns the bare package install command (no RUN prefix),
// for snippets that chain several steps in one layer.
func (d *Distro) InstallCommand(names []string) string {
	pkgs, _ := d.Packages(names)
	if len(pkgs) == 0 {
		return ""
	}
	if d.IsDefault() {
		return "apk add --no-cache " + strings.Join(pkgs, " ")
	}
	return "apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends " + strings.Join(pkgs, " ")
}

// RemoveCommand returns the command removing build-only packages again.
func (d *Distro) RemoveCommand(names []string) string {
	pkgs, _ := d.Packages(names)
	if len(pkgs) == 0 {
		return ""
	}
	if d.IsDefault() {
		return "apk del " + strings.Join(pkgs, " ")
	}
	return "apt-get purge -y " + strings.Join(pkgs, " ") + " && apt-get autoremove -y"
}

// Names returns every flavor id, sorted, for CLI help and validation.
func Names() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return out
}

func splitPin(name string) (base, version string) {
	if i := strings.Index(name, "="); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

func joinPin(base, version string) string {
	if version == "" {
		return base
	}
	return base + "=" + version
}
