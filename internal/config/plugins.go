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

import "sort"

// PluginEntry is the per-workspace configuration of one agent plugin.
// Settings values are plain strings; a secret setting stores a vault key
// reference ("vault:KEY_NAME") rather than the secret itself.
type PluginEntry struct {
	Enabled  bool              `yaml:"enabled"`
	Version  string            `yaml:"version,omitempty"`
	Settings map[string]string `yaml:"settings,omitempty"`
}

// PluginMap maps an agent name to its plugins, keyed by plugin name.
type PluginMap map[string]map[string]PluginEntry

// Workspace returns a pointer to the named workspace, or nil when the
// workspace does not exist. The pointer aliases the catalog entry, so
// mutations are persisted by SaveConfig.
func (c *Config) Workspace(name string) *Workspace {
	if c == nil {
		return nil
	}
	for i := range c.Workspaces.Items {
		if c.Workspaces.Items[i].Name == name {
			return &c.Workspaces.Items[i]
		}
	}
	return nil
}

// PluginEntry returns the stored configuration for one plugin and whether
// it was configured at all.
func (w *Workspace) PluginEntry(agent, plugin string) (PluginEntry, bool) {
	if w == nil || w.Plugins == nil {
		return PluginEntry{}, false
	}
	byPlugin, ok := w.Plugins[agent]
	if !ok {
		return PluginEntry{}, false
	}
	entry, ok := byPlugin[plugin]
	return entry, ok
}

// SetPluginEntry stores the configuration for one plugin, creating the
// nested maps as needed.
func (w *Workspace) SetPluginEntry(agent, plugin string, entry PluginEntry) {
	if w == nil {
		return
	}
	if w.Plugins == nil {
		w.Plugins = PluginMap{}
	}
	if w.Plugins[agent] == nil {
		w.Plugins[agent] = map[string]PluginEntry{}
	}
	w.Plugins[agent][plugin] = entry
}

// RemovePluginEntry deletes a plugin's stored configuration, pruning empty
// agent maps so the YAML stays clean.
func (w *Workspace) RemovePluginEntry(agent, plugin string) {
	if w == nil || w.Plugins == nil {
		return
	}
	byPlugin, ok := w.Plugins[agent]
	if !ok {
		return
	}
	delete(byPlugin, plugin)
	if len(byPlugin) == 0 {
		delete(w.Plugins, agent)
	}
	if len(w.Plugins) == 0 {
		w.Plugins = nil
	}
}

// SetPluginEnabled flips a plugin's enable flag, preserving its settings.
func (w *Workspace) SetPluginEnabled(agent, plugin string, enabled bool) {
	entry, _ := w.PluginEntry(agent, plugin)
	entry.Enabled = enabled
	w.SetPluginEntry(agent, plugin, entry)
}

// IsPluginEnabled reports whether a plugin is enabled for an agent.
func (w *Workspace) IsPluginEnabled(agent, plugin string) bool {
	entry, ok := w.PluginEntry(agent, plugin)
	return ok && entry.Enabled
}

// EnabledPlugins returns the enabled plugin names for an agent, sorted so
// image hashes and command output are deterministic.
func (w *Workspace) EnabledPlugins(agent string) []string {
	if w == nil || w.Plugins == nil {
		return nil
	}
	var names []string
	for name, entry := range w.Plugins[agent] {
		if entry.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// PluginSetting returns one stored setting value, or "" when unset.
func (w *Workspace) PluginSetting(agent, plugin, key string) string {
	entry, ok := w.PluginEntry(agent, plugin)
	if !ok || entry.Settings == nil {
		return ""
	}
	return entry.Settings[key]
}

// SetPluginSetting stores one setting value. An empty value removes the
// setting so the plugin falls back to its default.
func (w *Workspace) SetPluginSetting(agent, plugin, key, value string) {
	entry, _ := w.PluginEntry(agent, plugin)
	if value == "" {
		delete(entry.Settings, key)
		if len(entry.Settings) == 0 {
			entry.Settings = nil
		}
		w.SetPluginEntry(agent, plugin, entry)
		return
	}
	if entry.Settings == nil {
		entry.Settings = map[string]string{}
	}
	entry.Settings[key] = value
	w.SetPluginEntry(agent, plugin, entry)
}
