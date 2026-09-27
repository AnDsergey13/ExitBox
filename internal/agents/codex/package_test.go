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
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/agent"
)

func TestPackageName(t *testing.T) {
	got := (&Codex{}).PackageName()
	switch runtime.GOARCH {
	case "amd64":
		if got != "codex-package-x86_64-unknown-linux-musl.tar.gz" {
			t.Errorf("PackageName() = %q", got)
		}
	case "arm64":
		if got != "codex-package-aarch64-unknown-linux-musl.tar.gz" {
			t.Errorf("PackageName() = %q", got)
		}
	}
}

// buildInput returns a PrepareBuildInput whose download either succeeds or
// fails, so both install paths can be exercised without network access.
func buildInput(t *testing.T, fail func(url string) bool) agent.PrepareBuildInput {
	t.Helper()
	dir := t.TempDir()
	return agent.PrepareBuildInput{
		Ctx:            context.Background(),
		Version:        "rust-v0.157.1",
		BuildDir:       dir,
		DockerfilePath: filepath.Join(dir, "Dockerfile"),
		Download: func(_ context.Context, url, dest string) error {
			if fail(url) {
				return errors.New("HTTP 404")
			}
			return os.WriteFile(dest, []byte("archive"), 0644)
		},
		FileSHA256: func(string) string { return "deadbeef" },
		Logf:       func(string, ...interface{}) {},
	}
}

func TestPrepareBuildInstallsTheReleasePackage(t *testing.T) {
	in := buildInput(t, func(string) bool { return false })
	if err := (&Codex{}).PrepareBuild(in); err != nil {
		t.Fatalf("PrepareBuild: %v", err)
	}
	df := readFile(t, in.DockerfilePath)

	for _, want := range []string{
		"ARG CODEX_PACKAGE_CHECKSUM=deadbeef",
		"--mount=type=bind,source=codex-package-",
		"sha256sum -c -",
		"tar -xzf /tmp/codex-package.tar.gz -C /opt/codex",
		"--exclude 'codex-resources/voice*'",
		`ln -sf /opt/codex/bin/codex "$HOME/.local/bin/codex"`,
		`ln -sf /opt/codex/bin/codex-code-mode-host "$HOME/.local/bin/codex-code-mode-host"`,
		"codex --version",
		"apk add --no-cache bubblewrap",
	} {
		if !strings.Contains(df, want) {
			t.Errorf("package Dockerfile missing %q:\n%s", want, df)
		}
	}
	// The package must not be copied into a layer: it is ~150 MB.
	if strings.Contains(df, "COPY codex-package") {
		t.Errorf("the package should be bind mounted, not COPYed:\n%s", df)
	}
}

func TestPrepareBuildFallsBackToStandaloneBinaries(t *testing.T) {
	// A release that predates the package asset: only that download fails.
	in := buildInput(t, func(url string) bool { return strings.Contains(url, "codex-package-") })
	if err := (&Codex{}).PrepareBuild(in); err != nil {
		t.Fatalf("PrepareBuild: %v", err)
	}
	df := readFile(t, in.DockerfilePath)

	if strings.Contains(df, "codex-package") {
		t.Errorf("fallback Dockerfile should not reference the package:\n%s", df)
	}
	for _, want := range []string{
		"COPY codex-",
		"ARG CODEX_CHECKSUM=deadbeef",
		"ARG CODEX_CODE_MODE_HOST_CHECKSUM=deadbeef",
		"$HOME/.local/bin/codex --version",
	} {
		if !strings.Contains(df, want) {
			t.Errorf("fallback Dockerfile missing %q:\n%s", want, df)
		}
	}
}

func TestPackageInstallUsesFlavorPackageManager(t *testing.T) {
	in := buildInput(t, func(string) bool { return false })
	in.BaseImage = "exitbox-base-gpu"
	in.Libc = "gnu"
	in.Install = func(pkgs []string) string {
		return "RUN apt-get install -y " + strings.Join(pkgs, " ")
	}
	if err := (&Codex{}).PrepareBuild(in); err != nil {
		t.Fatalf("PrepareBuild: %v", err)
	}
	df := readFile(t, in.DockerfilePath)

	if !strings.Contains(df, "FROM exitbox-base-gpu") {
		t.Errorf("Dockerfile does not build on the GPU base:\n%s", df)
	}
	if !strings.Contains(df, "apt-get install -y bubblewrap") {
		t.Errorf("Dockerfile should install bubblewrap with apt:\n%s", df)
	}
	if strings.Contains(df, "apk add") {
		t.Errorf("Dockerfile still uses apk:\n%s", df)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
