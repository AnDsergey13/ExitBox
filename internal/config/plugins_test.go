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
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkspaceLookup(t *testing.T) {
	cfg := &Config{Workspaces: WorkspaceCatalog{Items: []Workspace{{Name: "work"}, {Name: "home"}}}}
	ws := cfg.Workspace("home")
	if ws == nil {
		t.Fatal("Workspace(home) = nil")
	}
	// The pointer must alias the catalog entry so edits persist.
	ws.GPU = true
	if !cfg.Workspaces.Items[1].GPU {
		t.Error("Workspace() did not return a pointer into the catalog")
	}
	if cfg.Workspace("nope") != nil {
		t.Error("Workspace(nope) should be nil")
	}
}

func TestPluginEntryRoundTrip(t *testing.T) {
	ws := &Workspace{Name: "default"}
	if _, ok := ws.PluginEntry("codex", "codex-router"); ok {
		t.Error("an unconfigured plugin should report ok=false")
	}

	ws.SetPluginEntry("codex", "codex-router", PluginEntry{Enabled: true, Settings: map[string]string{"port": "4202"}})
	entry, ok := ws.PluginEntry("codex", "codex-router")
	if !ok || !entry.Enabled || entry.Settings["port"] != "4202" {
		t.Fatalf("PluginEntry() = %+v, ok=%v", entry, ok)
	}
	if !ws.IsPluginEnabled("codex", "codex-router") {
		t.Error("IsPluginEnabled() = false")
	}
	if ws.IsPluginEnabled("claude", "codex-router") {
		t.Error("plugin state must not leak across agents")
	}
}

func TestSetPluginEnabledKeepsSettings(t *testing.T) {
	ws := &Workspace{Name: "default"}
	ws.SetPluginSetting("codex", "codex-router", "port", "4300")
	ws.SetPluginEnabled("codex", "codex-router", true)

	entry, _ := ws.PluginEntry("codex", "codex-router")
	if !entry.Enabled || entry.Settings["port"] != "4300" {
		t.Errorf("entry = %+v, want enabled with its settings kept", entry)
	}
}

func TestRemovePluginEntryPrunesMaps(t *testing.T) {
	ws := &Workspace{Name: "default"}
	ws.SetPluginEntry("codex", "codex-router", PluginEntry{Enabled: true})
	ws.RemovePluginEntry("codex", "codex-router")
	if ws.Plugins != nil {
		t.Errorf("Plugins = %v, want nil after removing the last entry", ws.Plugins)
	}
	// Removing something that was never there is a no-op.
	ws.RemovePluginEntry("codex", "codex-router")
}

func TestEnabledPluginsSorted(t *testing.T) {
	ws := &Workspace{Name: "default"}
	ws.SetPluginEntry("codex", "zeta", PluginEntry{Enabled: true})
	ws.SetPluginEntry("codex", "alpha", PluginEntry{Enabled: true})
	ws.SetPluginEntry("codex", "off", PluginEntry{Enabled: false})

	got := ws.EnabledPlugins("codex")
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnabledPlugins() = %v, want %v", got, want)
	}
	if got := ws.EnabledPlugins("claude"); got != nil {
		t.Errorf("EnabledPlugins(claude) = %v, want nil", got)
	}
}

func TestSetPluginSettingClearsOnEmpty(t *testing.T) {
	ws := &Workspace{Name: "default"}
	ws.SetPluginSetting("codex", "codex-router", "port", "4300")
	if ws.PluginSetting("codex", "codex-router", "port") != "4300" {
		t.Fatal("setting not stored")
	}
	ws.SetPluginSetting("codex", "codex-router", "port", "")
	if ws.PluginSetting("codex", "codex-router", "port") != "" {
		t.Error("an empty value should clear the setting")
	}
	entry, _ := ws.PluginEntry("codex", "codex-router")
	if entry.Settings != nil {
		t.Errorf("Settings = %v, want nil once empty", entry.Settings)
	}
}

func TestPluginConfigSurvivesYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	ws := cfg.Workspace("default")
	if ws == nil {
		t.Fatal("default workspace missing")
	}
	ws.GPU = true
	ws.SetPluginEntry("codex", "codex-router", PluginEntry{
		Enabled:  true,
		Version:  "v0.6.0",
		Settings: map[string]string{"providers": "anthropic-api", "anthropic_api_key": "vault:MY_KEY"},
	})

	if err := SaveConfigTo(cfg, path); err != nil {
		t.Fatalf("SaveConfigTo: %v", err)
	}
	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	loadedWs := loaded.Workspace("default")
	if loadedWs == nil {
		t.Fatal("workspace missing after reload")
	}
	if !loadedWs.GPU {
		t.Error("GPU flag did not survive the round trip")
	}
	entry, ok := loadedWs.PluginEntry("codex", "codex-router")
	if !ok || !entry.Enabled || entry.Version != "v0.6.0" {
		t.Fatalf("entry = %+v, ok=%v", entry, ok)
	}
	if entry.Settings["anthropic_api_key"] != "vault:MY_KEY" {
		t.Errorf("secret reference lost: %v", entry.Settings)
	}
}
