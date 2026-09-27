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
	"reflect"
	"strings"
	"testing"
)

func TestGoProfileUsesAPKGoPackage(t *testing.T) {
	pkgs := CollectVersionedPackages([]string{"go"}, nil)
	if len(pkgs) != 1 || pkgs[0] != "go" {
		t.Fatalf("CollectVersionedPackages([go], nil) = %v, want [go]", pkgs)
	}
}

func TestGoCustomSnippetDoesNotDownloadGoDevToolchainByDefault(t *testing.T) {
	snippet := CustomSnippet("go", nil)
	for _, forbidden := range []string{
		"go.dev/VERSION",
		"go.dev/dl",
		"GO_TARBALL",
		"/usr/local/go",
	} {
		if strings.Contains(snippet, forbidden) {
			t.Fatalf("Go CustomSnippet contains %q; compiler should be installed via apk by default", forbidden)
		}
	}
	if !strings.Contains(snippet, "golangci-lint") {
		t.Fatal("Go CustomSnippet should still install golangci-lint")
	}
}

func TestGoCustomSnippetPinnedVersionDownloadsGoDevToolchain(t *testing.T) {
	snippet := CustomSnippet("go", map[string]string{"go": "1.35.0"})
	for _, want := range []string{
		"go1.35.0.linux-${GO_ARCH}.tar.gz",
		"https://go.dev/dl/?mode=json&include=all",
		".[] | .files[] | select(.filename == $f)",
		"sha256sum -c",
		"ln -sf /usr/local/go/bin/go /usr/local/bin/go",
	} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("versioned Go CustomSnippet missing %q in:\n%s", want, snippet)
		}
	}
	if !strings.Contains(snippet, "golangci-lint") {
		t.Fatal("Go CustomSnippet should still install golangci-lint")
	}
}

func TestGoCustomSnippetLatestResolvesAtBuildTime(t *testing.T) {
	snippet := CustomSnippet("go", map[string]string{"go": "latest"})
	if !strings.Contains(snippet, "https://go.dev/VERSION?m=text") {
		t.Fatalf("latest Go CustomSnippet should resolve the version at build time:\n%s", snippet)
	}
}

func TestCollectVersionedPackagesPinsApkLanguage(t *testing.T) {
	got := CollectVersionedPackages([]string{"node"}, map[string]string{"node": "22.14.0-r0"})
	want := []string{"nodejs=22.14.0-r0", "npm"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectVersionedPackages = %v, want %v", got, want)
	}
}

func TestCollectVersionedPackagesRemovesGoWhenPinned(t *testing.T) {
	got := CollectVersionedPackages([]string{"go"}, map[string]string{"go": "1.35.0"})
	if len(got) != 0 {
		t.Fatalf("CollectVersionedPackages([go], pin) = %v, want empty (tarball replaces apk)", got)
	}
}

func TestCollectVersionedPackagesIgnoresLatestForApk(t *testing.T) {
	got := CollectVersionedPackages([]string{"python"}, map[string]string{"python": "latest"})
	want := []string{"python3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectVersionedPackages = %v, want %v", got, want)
	}
}

func TestGolangciLintVersionedSnippet(t *testing.T) {
	snippet := CustomSnippet("go", map[string]string{"golangci-lint": "1.70.0"})
	if !strings.Contains(snippet, `LINT_VERSION="1.70.0"`) {
		t.Fatalf("golangci-lint snippet missing pinned version:\n%s", snippet)
	}
}

func TestGolangciLintLatestSnippet(t *testing.T) {
	snippet := CustomSnippet("go", map[string]string{"golangci-lint": "latest"})
	if !strings.Contains(snippet, "api.github.com/repos/golangci/golangci-lint/releases/latest") {
		t.Fatalf("latest golangci-lint snippet should resolve at build time:\n%s", snippet)
	}
}
