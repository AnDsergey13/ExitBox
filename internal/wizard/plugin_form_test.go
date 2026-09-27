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

package wizard

import (
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/plugin"
)

func formPlugin() *plugin.Plugin {
	return &plugin.Plugin{
		Name:    "demo",
		Display: "Demo",
		Agents:  []string{"codex"},
		Settings: []plugin.Setting{
			{Key: "providers", Label: "Providers", Type: plugin.TypeMulti, Options: []string{"a", "b"}},
			{Key: "a_key", Label: "A key", Type: plugin.TypeSecret, DependsOn: "providers", DependsValue: "a"},
			{Key: "port", Label: "Port", Type: plugin.TypeInt, Default: "4202"},
			{Key: "auto_start", Label: "Auto start", Type: plugin.TypeBool, Default: "true"},
		},
	}
}

func TestPluginFormStartsFromDefaultsAndEntry(t *testing.T) {
	f := NewPluginForm("codex", formPlugin(), config.PluginEntry{
		Enabled:  true,
		Settings: map[string]string{"port": "4300"},
	})
	result := f.Result()
	if !result.Enabled {
		t.Error("stored enable state lost")
	}
	if result.Settings["port"] != "4300" {
		t.Errorf("stored value should win over the default: %v", result.Settings)
	}
	if result.Settings["auto_start"] != "true" {
		t.Errorf("default missing: %v", result.Settings)
	}
	if result.Saved {
		t.Error("a fresh form is not saved yet")
	}
}

func TestPluginFormHidesDependentSettings(t *testing.T) {
	f := NewPluginForm("codex", formPlugin(), config.PluginEntry{})
	for _, s := range f.visibleSettings() {
		if s.Key == "a_key" {
			t.Fatal("the provider key should be hidden until its provider is selected")
		}
	}
	f.settings["providers"] = "a"
	found := false
	for _, s := range f.visibleSettings() {
		if s.Key == "a_key" {
			found = true
		}
	}
	if !found {
		t.Error("the provider key should appear once its provider is selected")
	}
}

func TestPluginFormTogglesEnabled(t *testing.T) {
	f := NewPluginForm("codex", formPlugin(), config.PluginEntry{})
	updated, _ := f.updateBrowsing(key(" "))
	form := updated.(*PluginForm)
	if !form.Result().Enabled {
		t.Error("space on the first row should enable the plugin")
	}
}

func TestPluginFormSaveAndCancel(t *testing.T) {
	f := NewPluginForm("codex", formPlugin(), config.PluginEntry{})
	updated, _ := f.updateBrowsing(key("s"))
	if !updated.(*PluginForm).Result().Saved || !updated.(*PluginForm).Done() {
		t.Error("s should save and finish")
	}

	f = NewPluginForm("codex", formPlugin(), config.PluginEntry{})
	updated, _ = f.updateBrowsing(key("esc"))
	if updated.(*PluginForm).Result().Saved {
		t.Error("esc must not save")
	}
	if !updated.(*PluginForm).Done() {
		t.Error("esc should finish the menu")
	}
}

func TestPluginFormValidatesTextInput(t *testing.T) {
	f := NewPluginForm("codex", formPlugin(), config.PluginEntry{})
	port := f.plugin.Setting("port")
	f.editing = true
	f.buffer = "port"
	f.text = "not-a-number"

	updated, _ := f.updateText(key("enter"), port)
	form := updated.(*PluginForm)
	if form.status == "" {
		t.Error("an invalid value should report an error")
	}
	if !form.editing {
		t.Error("the editor should stay open on an invalid value")
	}

	form.text = "4300"
	updated, _ = form.updateText(key("enter"), port)
	form = updated.(*PluginForm)
	if form.editing {
		t.Error("a valid value should close the editor")
	}
	if form.settings["port"] != "4300" {
		t.Errorf("value not stored: %v", form.settings)
	}
}

func TestPluginFormSecretStoresVaultReference(t *testing.T) {
	p := formPlugin()
	f := NewPluginForm("codex", p, config.PluginEntry{Settings: map[string]string{"providers": "a"}})
	secret := p.Setting("a_key")
	f.editing = true
	f.buffer = "a_key"
	f.text = "MY_KEY"

	updated, _ := f.updateText(key("enter"), secret)
	form := updated.(*PluginForm)
	if form.settings["a_key"] != "vault:MY_KEY" {
		t.Errorf("secret stored as %q, want a vault reference", form.settings["a_key"])
	}
	if !strings.Contains(form.View(), "MY_KEY") {
		t.Error("the menu should show the vault key")
	}
}

func TestPluginFormMultiSelect(t *testing.T) {
	p := formPlugin()
	f := NewPluginForm("codex", p, config.PluginEntry{})
	providers := p.Setting("providers")
	f.beginEdit(*providers)
	if !f.editing || f.choices == nil {
		t.Fatal("a multi-select should open the option picker")
	}

	updated, _ := f.updateChoice(key(" "), providers) // toggle "a"
	form := updated.(*PluginForm)
	updated, _ = form.updateChoice(key("enter"), providers)
	form = updated.(*PluginForm)

	if form.settings["providers"] != "a" {
		t.Errorf("providers = %q, want a", form.settings["providers"])
	}
	if form.editing {
		t.Error("enter should close the picker")
	}
}
