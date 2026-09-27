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
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultAlpineVersion(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.AlpineVersionOrDefault(); got != DefaultAlpineVersion {
		t.Fatalf("AlpineVersionOrDefault() = %q, want %q", got, DefaultAlpineVersion)
	}
}

func TestAlpineVersionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"
	cfg := DefaultConfig()
	cfg.Settings.AlpineVersion = "3.24"
	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.AlpineVersionOrDefault(); got != "3.24" {
		t.Fatalf("AlpineVersionOrDefault() = %q, want 3.24", got)
	}
}

func TestDefaultAutoUpdateTools(t *testing.T) {
	if DefaultConfig().Settings.AutoUpdateTools {
		t.Error("AutoUpdateTools should be false by default")
	}
}

func TestAutoUpdateToolsYAML(t *testing.T) {
	input := "settings:\n  auto_update_tools: true\n"
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Settings.AutoUpdateTools {
		t.Error("AutoUpdateTools not unmarshaled")
	}
}

func TestAutoUpdateToolsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"
	cfg := DefaultConfig()
	cfg.Settings.AutoUpdateTools = true
	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Settings.AutoUpdateTools {
		t.Error("AutoUpdateTools lost in round trip")
	}
}
