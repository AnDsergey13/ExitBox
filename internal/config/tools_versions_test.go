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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetSetToolVersion(t *testing.T) {
	cfg := DefaultConfig()

	if got := cfg.GetToolVersion("go"); got != "" {
		t.Fatalf("GetToolVersion(go) = %q, want empty", got)
	}

	cfg.SetToolVersion("go", "1.35.0")
	if got := cfg.GetToolVersion("go"); got != "1.35.0" {
		t.Fatalf("GetToolVersion(go) = %q, want 1.35.0", got)
	}

	cfg.SetToolVersion("go", "default")
	if got := cfg.GetToolVersion("go"); got != "" {
		t.Fatalf("GetToolVersion(go) after default = %q, want empty", got)
	}

	cfg.SetToolVersion("node", "latest")
	if got := cfg.GetToolVersion("node"); got != "latest" {
		t.Fatalf("GetToolVersion(node) = %q, want latest", got)
	}
}

func TestToolVersionsYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.SetToolVersion("go", "1.35.0")
	cfg.SetToolVersion("node", "22.14.0-r0")
	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetToolVersion("go") != "1.35.0" {
		t.Errorf("loaded go version = %q, want 1.35.0", loaded.GetToolVersion("go"))
	}
	if loaded.GetToolVersion("node") != "22.14.0-r0" {
		t.Errorf("loaded node version = %q, want 22.14.0-r0", loaded.GetToolVersion("node"))
	}
}

func TestToolVersionsYAMLField(t *testing.T) {
	input := `tools:
  versions:
    go: "1.35.0"
    bun: latest
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GetToolVersion("go") != "1.35.0" {
		t.Errorf("go = %q, want 1.35.0", cfg.GetToolVersion("go"))
	}
	if cfg.GetToolVersion("bun") != "latest" {
		t.Errorf("bun = %q, want latest", cfg.GetToolVersion("bun"))
	}
}

func TestToolVersionsPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := DefaultConfig()
	cfg.SetToolVersion("go", "1.35.0")
	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	tools, ok := parsed["tools"].(map[string]any)
	if !ok {
		t.Fatalf("tools section not in saved config: %s", data)
	}
	versions, ok := tools["versions"].(map[string]any)
	if !ok {
		t.Fatalf("tools.versions not in saved config: %s", data)
	}
	if versions["go"] != "1.35.0" {
		t.Errorf("saved go version = %v, want 1.35.0", versions["go"])
	}
}
