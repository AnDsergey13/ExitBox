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

package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/agent"
	"github.com/cloud-exit/exitbox/internal/distro"
)

// gpuInput builds the PrepareBuild input the image package passes for the
// GPU flavor, so the agents are exercised exactly as they are in a build.
func gpuInput(t *testing.T, version string) agent.PrepareBuildInput {
	t.Helper()
	d := distro.Get(distro.GPU)
	dir := t.TempDir()
	return agent.PrepareBuildInput{
		Version:        version,
		BuildDir:       dir,
		DockerfilePath: filepath.Join(dir, "Dockerfile"),
		BaseImage:      "exitbox-base-gpu",
		Libc:           d.LibcTag(),
		Install:        d.InstallSnippet,
		InstallInline:  d.InstallCommand,
		Remove:         d.RemoveCommand,
		Logf:           func(string, ...interface{}) {},
	}
}

// npmAgents install through npm and only differ in their package manager
// line, so they are checked together.
func TestNpmAgentsBuildOnGPUFlavor(t *testing.T) {
	for _, name := range []string{"qwen", "copilot", "kimi", "pi"} {
		t.Run(name, func(t *testing.T) {
			a := Get(name)
			if a == nil {
				t.Fatalf("agent %q missing", name)
			}
			in := gpuInput(t, "1.2.3")
			if err := a.PrepareBuild(in); err != nil {
				t.Fatalf("PrepareBuild: %v", err)
			}
			df := readDockerfile(t, in.DockerfilePath)

			if !strings.Contains(df, "FROM exitbox-base-gpu") {
				t.Errorf("Dockerfile does not build on the GPU base:\n%s", df)
			}
			if strings.Contains(df, "apk add") {
				t.Errorf("Dockerfile still uses apk:\n%s", df)
			}
			// Node comes from nodejs.org on this flavor: Ubuntu packages a
			// version older than the agents accept.
			if !strings.Contains(df, "https://nodejs.org/dist/") {
				t.Errorf("Dockerfile does not install a current Node:\n%s", df)
			}
			if !strings.Contains(df, "sha256sum -c -") {
				t.Errorf("Node download is not checksum verified:\n%s", df)
			}
			if !strings.Contains(df, "npm install -g") {
				t.Errorf("npm install missing:\n%s", df)
			}
		})
	}
}

func TestClaudeSkipsMuslWorkaroundsOnGPUFlavor(t *testing.T) {
	a := Get("claude")
	in := gpuInput(t, "1.0.0")
	if err := a.PrepareBuild(in); err != nil {
		t.Fatalf("PrepareBuild: %v", err)
	}
	df := readDockerfile(t, in.DockerfilePath)

	if strings.Contains(df, "gcompat") || strings.Contains(df, "posix_getdents") {
		t.Errorf("musl workarounds must not be built into a glibc image:\n%s", df)
	}
	if !strings.Contains(df, "claude.ai/install.sh") {
		t.Errorf("installer missing:\n%s", df)
	}
	if !strings.Contains(df, "FROM exitbox-base-gpu") {
		t.Errorf("Dockerfile does not build on the GPU base:\n%s", df)
	}
}

func TestClaudeKeepsMuslWorkaroundsOnAlpine(t *testing.T) {
	install, err := Get("claude").GetDockerfileInstall("")
	if err != nil {
		t.Fatalf("GetDockerfileInstall: %v", err)
	}
	if !strings.Contains(install, "gcompat") || !strings.Contains(install, "posix_getdents") {
		t.Errorf("the Alpine install lost its musl workarounds:\n%s", install)
	}
	if !strings.Contains(install, "apk add --no-cache musl-dev gcc") {
		t.Errorf("the Alpine install should still use apk:\n%s", install)
	}
}

func TestCursorAndOpenCodeInstallWithFlavorPackageManager(t *testing.T) {
	for _, name := range []string{"cursor", "opencode"} {
		t.Run(name, func(t *testing.T) {
			install, err := Get(name).GetDockerfileInstall("")
			if err != nil {
				t.Fatalf("GetDockerfileInstall: %v", err)
			}
			if !strings.Contains(install, "apk add --no-cache") {
				t.Errorf("the Alpine install should use apk:\n%s", install)
			}
		})
	}
}

func TestAlpineDockerfilesUnchangedByDefault(t *testing.T) {
	// The default flavor must keep building exactly as before: same base
	// image name, same package manager.
	for _, a := range All() {
		df, err := a.GetFullDockerfile("")
		if err != nil {
			continue // architecture-specific agents may refuse here
		}
		if !strings.HasPrefix(df, "FROM exitbox-base\n") {
			t.Errorf("%s: Alpine Dockerfile starts with %q", a.Name(), firstLine(df))
		}
	}
}

func readDockerfile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	return string(data)
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
