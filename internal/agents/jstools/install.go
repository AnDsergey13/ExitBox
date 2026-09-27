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

package jstools

import (
	"strings"

	"github.com/cloud-exit/exitbox/internal/agent"
)

// InstallDependencies generates a Dockerfile RUN step for apk-installed
// packages and globally installed npm packages.
// Callers must pass trusted package specifiers only (shell-injection-prone).
func InstallDependencies(apkPackages, npmPackages []string) string {
	return InstallDependenciesFor(agent.PrepareBuildInput{}, apkPackages, npmPackages)
}

// InstallDependenciesFor generates the same step for a specific image
// flavor, so system packages are installed with that image's package
// manager. Package names are always given as Alpine names and translated by
// the flavor.
func InstallDependenciesFor(in agent.PrepareBuildInput, systemPackages, npmPackages []string) string {
	var b strings.Builder

	if len(systemPackages) > 0 {
		b.WriteString(in.InstallStep(systemPackages...))
	}
	if len(npmPackages) > 0 {
		if b.Len() > 0 {
			b.WriteString(" && \\\n    ")
		} else {
			b.WriteString("RUN ")
		}
		b.WriteString(npmGlobalInstallCommand(npmPackages))
	}
	return b.String()
}

// InstallBun generates a Dockerfile RUN step that installs the latest Bun
// on Alpine.
func InstallBun() string {
	return bunInstall("", "musl")
}

// InstallBunVersion generates a Dockerfile RUN step that installs a pinned
// Bun version on Alpine (e.g. "1.2.3").
func InstallBunVersion(version string) string {
	return bunInstall(version, "musl")
}

// InstallBunFor generates the Bun install step for an image flavor. Bun
// ships separate musl and glibc builds, and the wrong one fails to run.
func InstallBunFor(version, libc string) string {
	return bunInstall(version, libc)
}

func bunInstall(version, libc string) string {
	spec := `"${BUN_PACKAGE}"`
	if version != "" {
		spec = `"${BUN_PACKAGE}@` + version + `"`
	}
	suffix := "-musl"
	if libc != "" && libc != "musl" {
		suffix = ""
	}
	return `RUN set -e && \
    case "$(uname -m)" in \
        x86_64|amd64) BUN_PACKAGE="@oven/bun-linux-x64` + suffix + `" ;; \
        aarch64|arm64) BUN_PACKAGE="@oven/bun-linux-aarch64` + suffix + `" ;; \
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
    esac && \
    ` + npmGlobalInstallCommand([]string{spec}) + ` && \
    install -m 755 "$(npm root -g)/${BUN_PACKAGE}/bin/bun" /usr/local/bin/bun && \
    ln -sf /usr/local/bin/bun /usr/local/bin/bunx && \
    bun --version`
}

func npmGlobalInstallCommand(npmPackages []string) string {
	// Some npm CLIs ship their platform binary as an optional dependency. Force
	// optional deps on even if user/global npm config omits them.
	return "npm_config_optional=true npm_config_omit= npm install -g --include=optional " + strings.Join(npmPackages, " ")
}
