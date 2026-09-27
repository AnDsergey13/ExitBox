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

package cmd

import (
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/plugin"
	"github.com/cloud-exit/exitbox/internal/plugins"
)

func TestPluginsCommandSurface(t *testing.T) {
	cmd := newPluginsCmd()
	if cmd.Use != "plugins" {
		t.Errorf("Use = %q", cmd.Use)
	}
	// The old singular command keeps working.
	hasAlias := false
	for _, alias := range cmd.Aliases {
		if alias == "plugin" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Error("`exitbox plugin` must stay available as an alias")
	}

	verbs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		verbs[strings.Fields(sub.Use)[0]] = true
	}
	for _, want := range []string{"list", "show", "enable", "disable", "edit", "set", "install", "remove"} {
		if !verbs[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
	// Agent-first form: `exitbox plugins codex list`.
	for _, agentName := range plugins.Agents() {
		if !verbs[agentName] {
			t.Errorf("missing agent subcommand %q", agentName)
		}
	}
}

func TestPluginsAgentCmdVerbs(t *testing.T) {
	cmd := newPluginsAgentCmd("codex")
	verbs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		verbs[strings.Fields(sub.Use)[0]] = true
	}
	for _, want := range []string{"list", "show", "enable", "disable", "edit", "set"} {
		if !verbs[want] {
			t.Errorf("agent command missing %q", want)
		}
	}
}

func TestPrunedSettings(t *testing.T) {
	p := plugins.Get("codex-router")
	got := prunedSettings(p, map[string]string{
		"port":              "4202", // default
		"providers":         "deepseek",
		"anthropic_api_key": "vault:SOME_KEY", // hidden: provider not selected
		"auto_start":        "false",
	})
	if _, ok := got["port"]; ok {
		t.Errorf("defaults should not be written to config: %v", got)
	}
	if _, ok := got["anthropic_api_key"]; ok {
		t.Errorf("hidden settings should not be written: %v", got)
	}
	if got["providers"] != "deepseek" || got["auto_start"] != "false" {
		t.Errorf("explicit values lost: %v", got)
	}
	if prunedSettings(p, map[string]string{"port": "4202"}) != nil {
		t.Error("a config equal to the defaults should store nothing")
	}
}

func TestSettingDisplay(t *testing.T) {
	secret := plugin.Setting{Key: "k", Type: plugin.TypeSecret}
	if got := settingDisplay(secret, "vault:MY_KEY"); got != "vault:MY_KEY" {
		t.Errorf("settingDisplay() = %q", got)
	}
	if got := settingDisplay(secret, ""); got != "(unset)" {
		t.Errorf("settingDisplay(empty) = %q", got)
	}
	plain := plugin.Setting{Key: "k", Type: plugin.TypeString}
	if got := settingDisplay(plain, "value"); got != "value" {
		t.Errorf("settingDisplay() = %q", got)
	}
}

func TestSettingKeys(t *testing.T) {
	keys := settingKeys(plugins.Get("codex-router"))
	joined := strings.Join(keys, ",")
	for _, want := range []string{"providers", "port", "auto_start"} {
		if !strings.Contains(joined, want) {
			t.Errorf("settingKeys() = %v, want it to include %q", keys, want)
		}
	}
}

func TestEnabledLabel(t *testing.T) {
	if enabledLabel(true) != "enabled" || enabledLabel(false) != "disabled" {
		t.Error("enabledLabel() wrong")
	}
}

func TestGPUCommandSurface(t *testing.T) {
	cmd := newGPUCmd()
	verbs := map[string]bool{}
	for _, sub := range cmd.Commands() {
		verbs[strings.Fields(sub.Use)[0]] = true
	}
	// GPU is a per-workspace flag; the wizard is not the only way to set it.
	for _, want := range []string{"status", "enable", "disable"} {
		if !verbs[want] {
			t.Errorf("missing `exitbox gpu %s`", want)
		}
	}
}
