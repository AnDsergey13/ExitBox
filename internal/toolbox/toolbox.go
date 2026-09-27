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

// Package toolbox describes the tools and languages ExitBox manages with
// per-version pins. It is the source of truth for `exitbox tools list` and
// `exitbox tools update`, and is consumed by the Dockerfile generators.
package toolbox

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloud-exit/exitbox/internal/apk"
	"github.com/cloud-exit/exitbox/internal/config"
)

// Kind classifies a managed item as a language or a tool.
type Kind string

const (
	KindLanguage Kind = "language"
	KindTool     Kind = "tool"
)

// Mode describes how a tool is installed and versioned.
type Mode string

const (
	// ModeApk installs from Alpine repositories. A version pin is applied as
	// `apk add pkg=version`; "latest" leaves the package unpinned.
	ModeApk Mode = "apk"
	// ModeTarball installs an official archive with checksum verification.
	// A version pin bakes the exact version into the Dockerfile, while
	// "latest" resolves the newest upstream version during image build.
	ModeTarball Mode = "tarball"
	// ModeNpm installs through npm. A version pin is passed to npm as
	// package@version; "latest" installs without a version constraint.
	ModeNpm Mode = "npm"
)

// Tool represents one version-managed tool or language.
type Tool struct {
	Name        string   // config key and CLI name, e.g. "go"
	Display     string   // human-friendly name, e.g. "Go"
	Kind        Kind     // language or tool
	Mode        Mode     // installation and versioning strategy
	Description string   // short explanation shown by tools list
	Packages    []string // Alpine packages owned by this tool (apk installs)
	Roles       []string // exitbox development roles that provide this tool
	Default     string   // version used when no pin is configured ("" = latest)
}

// All is the catalog of managed tools and languages. Adding an entry here
// makes it updateable via `exitbox tools update <name>`; the Dockerfile
// generators in internal/profile and internal/wizard must know how to apply
// pins for each Mode.
var All = []Tool{
	{
		Name:        "go",
		Display:     "Go",
		Kind:        KindLanguage,
		Mode:        ModeTarball,
		Description: "Go toolchain (checksum verified from go.dev)",
		Packages:    []string{"go"},
		Roles:       []string{"go"},
	},
	{
		Name:        "golangci-lint",
		Display:     "golangci-lint",
		Kind:        KindTool,
		Mode:        ModeTarball,
		Description: "Go linter (GitHub release)",
		Roles:       []string{"go"},
		Default:     "1.64.8",
	},
	{
		Name:        "bun",
		Display:     "Bun",
		Kind:        KindTool,
		Mode:        ModeNpm,
		Description: "JavaScript runtime and package manager (npm)",
	},
	{
		Name:        "node",
		Display:     "Node.js",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "Node.js runtime and npm (Alpine apk)",
		Packages:    []string{"nodejs"},
		Roles:       []string{"node", "javascript"},
	},
	{
		Name:        "python",
		Display:     "Python",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "Python 3 runtime (Alpine apk)",
		Packages:    []string{"python3"},
		Roles:       []string{"python"},
	},
	{
		Name:        "rust",
		Display:     "Rust",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "Rust and Cargo (Alpine apk)",
		Packages:    []string{"rust", "cargo"},
		Roles:       []string{"rust"},
	},
	{
		Name:        "java",
		Display:     "Java",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "OpenJDK runtime (Alpine apk)",
		Packages:    []string{"openjdk17-jdk"},
		Roles:       []string{"java"},
	},
	{
		Name:        "dotnet",
		Display:     ".NET",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: ".NET SDK (Alpine apk)",
		Packages:    []string{"dotnet8-sdk"},
		Roles:       []string{"dotnet"},
	},
	{
		Name:        "ruby",
		Display:     "Ruby",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "Ruby runtime (Alpine apk)",
		Packages:    []string{"ruby", "ruby-dev"},
		Roles:       []string{"ruby"},
	},
	{
		Name:        "php",
		Display:     "PHP",
		Kind:        KindLanguage,
		Mode:        ModeApk,
		Description: "PHP runtime (Alpine apk)",
		Packages:    []string{"php83"},
		Roles:       []string{"php"},
	},
}

// AlpineVersions lists the Alpine releases available on the official
// mirror. The setup wizard offers these as a dropdown for the base image,
// and config validation only accepts values from this list.
var AlpineVersions = []string{
	"3.18",
	"3.19",
	"3.20",
	"3.21",
	"3.22",
	"3.23",
	"3.24",
}

// IsValidAlpineVersion reports whether v is a real, available Alpine release.
func IsValidAlpineVersion(v string) bool {
	for _, candidate := range AlpineVersions {
		if candidate == v {
			return true
		}
	}
	return false
}

// AlpineVersionDisplay returns the human-friendly base image label for a
// configured Alpine version (e.g. "alpine:3.21").
func AlpineVersionDisplay(v string) string {
	if v == "" {
		v = config.DefaultAlpineVersion
	}
	return "alpine:" + v
}

// Get returns the managed tool with the given name, or nil.
func Get(name string) *Tool {
	for i := range All {
		if strings.EqualFold(All[i].Name, name) {
			return &All[i]
		}
	}
	return nil
}

// LookupByRole returns the managed tool installed by a development role,
// or nil if the role is not version-managed.
func LookupByRole(role string) *Tool {
	for i := range All {
		for _, r := range All[i].Roles {
			if r == role {
				return &All[i]
			}
		}
	}
	return nil
}

// DefaultVersion returns the version shown when no pin is configured.
// Empty means "latest".
func (t *Tool) DefaultVersion() string { return t.Default }

// IsVersioned returns true when a non-empty version changes the install.
func (t *Tool) IsVersioned(version string) bool {
	return version != "" && version != "default"
}

// PackagesPinned returns the Alpine package specs used when a concrete apk
// version is pinned (e.g. "nodejs=22.14.0-r0").
func (t *Tool) PackagesPinned(version string) []string {
	specs := make([]string, 0, len(t.Packages))
	for _, p := range t.Packages {
		specs = append(specs, p+"="+version)
	}
	return specs
}

// OwnsPackage reports whether p is an Alpine package provided by this tool.
func (t *Tool) OwnsPackage(p string) bool {
	for _, owned := range t.Packages {
		if owned == p {
			return true
		}
	}
	return false
}

// NormalizeVersion canonicalizes a user-supplied version for storage.
func (t *Tool) NormalizeVersion(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	requested = strings.TrimPrefix(requested, t.Name+"=")
	requested = strings.TrimPrefix(requested, t.Name+"@")
	switch requested {
	case "", "default":
		return "default", nil
	case "latest":
		return "latest", nil
	}

	switch t.Name {
	case "go":
		requested = strings.TrimPrefix(requested, "go")
		requested = strings.TrimPrefix(requested, "v")
		if requested == "" {
			return "", fmt.Errorf("empty Go version")
		}
		if strings.Count(requested, ".") == 1 {
			requested += ".0"
		}
	case "golangci-lint":
		requested = strings.TrimPrefix(requested, "v")
	}
	if requested == "" {
		return "", fmt.Errorf("empty version for %s", t.Name)
	}
	return requested, nil
}

// ResolvePin turns a requested version into the exact value stored in
// config. "latest" stays as-is; apk tools resolve partial versions against
// the Alpine package index; tarball/npm tools use the normalized value.
func (t *Tool) ResolvePin(requested string) (string, error) {
	normalized, err := t.NormalizeVersion(requested)
	if err != nil {
		return "", err
	}
	if normalized == "default" {
		return "default", nil
	}
	if normalized == "latest" {
		return "latest", nil
	}
	if t.Mode == ModeApk {
		if len(t.Packages) == 0 {
			return "", fmt.Errorf("%s has no Alpine package to pin", t.Name)
		}
		return apk.ResolveVersion(t.Packages[0], normalized)
	}
	return normalized, nil
}

// VersionMap returns a snapshot of the configured tool versions with a
// deterministic sorted key order for hashing and display.
func VersionMap(cfg *config.Config) map[string]string {
	out := make(map[string]string)
	if cfg == nil || cfg.Tools.Versions == nil {
		return out
	}
	for k, v := range cfg.Tools.Versions {
		if v == "" || v == "default" {
			continue
		}
		out[k] = v
	}
	return out
}

// SortedVersions returns configured versions as a sorted, comma-joined
// string (name=version pairs) suitable for image hash inputs.
func SortedVersions(cfg *config.Config) string {
	m := VersionMap(cfg)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}
