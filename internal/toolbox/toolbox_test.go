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

package toolbox

import (
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/config"
)

func TestAlpineVersionValidation(t *testing.T) {
	if !IsValidAlpineVersion(config.DefaultAlpineVersion) {
		t.Errorf("default Alpine version %q not in catalog", config.DefaultAlpineVersion)
	}
	for _, invalid := range []string{"3.0", "3.25", "latest", ""} {
		if IsValidAlpineVersion(invalid) {
			t.Errorf("invalid Alpine version %q accepted", invalid)
		}
	}
	if got := AlpineVersionDisplay("3.24"); got != "alpine:3.24" {
		t.Errorf("AlpineVersionDisplay = %q, want alpine:3.24", got)
	}
}

func TestCatalogHasExpectedTools(t *testing.T) {
	for _, name := range []string{"go", "golangci-lint", "bun", "node", "python", "rust", "java", "dotnet", "ruby", "php"} {
		if Get(name) == nil {
			t.Errorf("catalog missing managed tool %q", name)
		}
	}
}

func TestLookupByRole(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"go", "go"},
		{"node", "node"},
		{"javascript", "node"},
		{"python", "python"},
		{"rust", "rust"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		tool := LookupByRole(tt.role)
		got := ""
		if tool != nil {
			got = tool.Name
		}
		if got != tt.want {
			t.Errorf("LookupByRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		tool      string
		requested string
		want      string
	}{
		{"go", "latest", "latest"},
		{"go", "1.35", "1.35.0"},
		{"go", "go1.35.0", "1.35.0"},
		{"go", "v1.35.0", "1.35.0"},
		{"golangci-lint", "v1.64.8", "1.64.8"},
		{"golangci-lint", "1.64.8", "1.64.8"},
		{"node", "22", "22"},
		{"node", "default", "default"},
		{"node", "", "default"},
		{"bun", "1.2.3", "1.2.3"},
	}
	for _, tt := range tests {
		tool := Get(tt.tool)
		got, err := tool.NormalizeVersion(tt.requested)
		if err != nil {
			t.Fatalf("NormalizeVersion(%s, %q) error: %v", tt.tool, tt.requested, err)
		}
		if got != tt.want {
			t.Errorf("NormalizeVersion(%s, %q) = %q, want %q", tt.tool, tt.requested, got, tt.want)
		}
	}
}

func TestSortedVersions(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetToolVersion("node", "22.14.0-r0")
	cfg.SetToolVersion("go", "1.35.0")

	got := SortedVersions(cfg)
	if got != "go=1.35.0,node=22.14.0-r0" {
		t.Errorf("SortedVersions = %q, want deterministic sorted output", got)
	}
}

func TestSortedVersionsSkipsDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetToolVersion("go", "default")
	if got := SortedVersions(cfg); got != "" {
		t.Errorf("SortedVersions = %q, want empty", got)
	}
}

func TestPackagesPinned(t *testing.T) {
	tool := Get("node")
	got := tool.PackagesPinned("22.14.0-r0")
	if len(got) != 1 || got[0] != "nodejs=22.14.0-r0" {
		t.Errorf("PackagesPinned = %v", got)
	}
	rust := Get("rust")
	got = rust.PackagesPinned("1.85.0-r0")
	if len(got) != 2 || !strings.Contains(got[0], "=1.85.0-r0") || !strings.Contains(got[1], "=1.85.0-r0") {
		t.Errorf("rust PackagesPinned = %v", got)
	}
}
