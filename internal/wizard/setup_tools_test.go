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
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/toolbox"
)

func TestApplyResultPersistsToolVersions(t *testing.T) {
	oldHome := config.Home
	config.Home = t.TempDir()
	t.Cleanup(func() { config.Home = oldHome })

	existing := config.DefaultConfig()
	state := State{
		WorkspaceName: "default",
		Roles:         []string{"Backend"},
		Languages:     []string{"Go"},
		Agents:        []string{"claude"},
		ToolVersions:  map[string]string{"go": "1.27.0"},
	}
	if err := applyResult(state, existing); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfigFrom(filepath.Join(config.Home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetToolVersion("go"); got != "1.27.0" {
		t.Errorf("go version = %q, want 1.27.0", got)
	}
	if cfg.Settings.AutoUpdateTools {
		t.Error("AutoUpdateTools should stay false")
	}
}

func TestApplyResultAutoUpdateToolsSetsLatest(t *testing.T) {
	oldHome := config.Home
	config.Home = t.TempDir()
	t.Cleanup(func() { config.Home = oldHome })

	existing := config.DefaultConfig()
	state := State{
		WorkspaceName:   "default",
		Roles:           []string{"Backend"},
		Languages:       []string{"Go"},
		Agents:          []string{"claude"},
		AutoUpdateTools: true,
		ToolVersions:    map[string]string{"go": "1.27.0"},
	}
	if err := applyResult(state, existing); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfigFrom(filepath.Join(config.Home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Settings.AutoUpdateTools {
		t.Error("AutoUpdateTools should be true")
	}
	// Explicit pins are preserved.
	if got := cfg.GetToolVersion("go"); got != "1.27.0" {
		t.Errorf("go version = %q, want pinned 1.27.0", got)
	}
	// Unpinned managed tools become latest.
	for _, name := range []string{"node", "python", "bun", "golangci-lint"} {
		if got := cfg.GetToolVersion(name); got != "latest" {
			t.Errorf("%s version = %q, want latest", name, got)
		}
	}
}

func TestApplyResultRemovesDefaultToolVersions(t *testing.T) {
	oldHome := config.Home
	config.Home = t.TempDir()
	t.Cleanup(func() { config.Home = oldHome })

	existing := config.DefaultConfig()
	existing.SetToolVersion("go", "1.35.0")
	state := State{
		WorkspaceName: "default",
		Roles:         []string{"Backend"},
		Languages:     []string{"Go"},
		Agents:        []string{"claude"},
		ToolVersions:  map[string]string{"go": "default"},
	}
	if err := applyResult(state, existing); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfigFrom(filepath.Join(config.Home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetToolVersion("go"); got != "" {
		t.Errorf("go version = %q, want default (empty)", got)
	}
}

func TestToolsStepLeadsToToolVersions(t *testing.T) {
	m := NewModel()
	m.step = stepTools
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.step != stepToolVersions {
		t.Fatalf("step = %v, want stepToolVersions", m.step)
	}
	if len(m.relevantManagedTools()) == 0 {
		t.Fatal("expected relevant managed tools for Backend + Go")
	}
}

func TestToolVersionsToggleAndContinue(t *testing.T) {
	m := NewModel()
	m.step = stepToolVersions
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true
	m.checked["extool:Bun"] = true

	// Focus the Go row (row 1; row 0 is the Alpine base dropdown).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)

	// Space opens the tool's dropdown with real versions.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if !m.toolDropdownOpen {
		t.Fatal("expected tool dropdown to open")
	}
	next, _ = m.Update(toolVersionsLoadedMsg{name: "go", versions: []string{"1.27.0"}})
	m = next.(Model)

	// Options: default, latest, 1.27.0. Move to latest and select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.toolDropdownOpen {
		t.Fatal("dropdown should close after selection")
	}
	if got := m.toolVersions["go"]; got != "latest" {
		t.Fatalf("go version = %q, want latest", got)
	}

	// Enter moves on to packages.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.step != stepPackages {
		t.Fatalf("step = %v, want stepPackages", m.step)
	}
}

func TestToolDropdownSelectsVersion(t *testing.T) {
	m := NewModel()
	m.step = stepToolVersions
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true

	// Focus the Go row and open its dropdown with Space.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if !m.toolDropdownOpen || !m.toolDropdownLoading {
		t.Fatalf("expected open + loading dropdown, got open=%v loading=%v", m.toolDropdownOpen, m.toolDropdownLoading)
	}
	if cmd == nil {
		t.Fatal("expected a fetch command for tool versions")
	}

	// Deliver fetched versions (simulates the async fetch result).
	next, _ = m.Update(toolVersionsLoadedMsg{name: "go", versions: []string{"1.27.0", "1.26.0"}})
	m = next.(Model)
	if m.toolDropdownLoading {
		t.Fatal("dropdown should no longer be loading")
	}

	// Options are default, latest, 1.27.0, 1.26.0. Move to "latest" and pick it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.toolDropdownOpen {
		t.Fatal("dropdown should close after selection")
	}
	if got := m.toolVersions["go"]; got != "latest" {
		t.Fatalf("go version = %q, want latest", got)
	}
}

func TestToolDropdownSelectsConcreteVersion(t *testing.T) {
	m := NewModel()
	m.step = stepToolVersions
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	next, _ = m.Update(toolVersionsLoadedMsg{name: "go", versions: []string{"1.27.0", "1.26.0"}})
	m = next.(Model)

	// Jump to the first concrete version (index 2) and select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if got := m.toolVersions["go"]; got != "1.27.0" {
		t.Fatalf("go version = %q, want 1.27.0", got)
	}
}

func TestAlpineBaseDropdown(t *testing.T) {
	m := NewModel()
	m.step = stepToolVersions
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true

	if m.alpineVersion != config.DefaultAlpineVersion {
		t.Fatalf("default alpine = %q, want %q", m.alpineVersion, config.DefaultAlpineVersion)
	}

	// Space on the Alpine row opens the dropdown.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if !m.alpineDropdown {
		t.Fatal("expected Alpine dropdown to open")
	}

	// Move to a different real Alpine version and select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	selected := toolbox.AlpineVersions[m.alpineDropdownCursor]
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.alpineDropdown {
		t.Fatal("dropdown should close after selection")
	}
	if m.alpineVersion != selected {
		t.Fatalf("alpine version = %q, want %q", m.alpineVersion, selected)
	}
}

func TestToolVersionsStepViaSidebarUsesCheckedState(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Roles = []string{"Backend"}
	cfg.Workspaces.Items = []config.Workspace{
		{Name: "default", Development: []string{"go", "python"}},
	}
	cfg.Workspaces.Active = "default"

	m := NewModelFromConfig(cfg)
	if m.isFirstRun {
		t.Fatal("expected re-run model")
	}
	// Put the model on the Role step with sidebar focus, then jump to
	// Tool versions.
	m.step = stepRole
	m.sidebarFocused = true
	found := false
	for i, si := range m.visibleSidebarSteps() {
		if si.Step == stepToolVersions {
			m.sidebarCursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Tool versions step not in sidebar")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.step != stepToolVersions {
		t.Fatalf("step = %v, want stepToolVersions", m.step)
	}
	if len(m.relevantManagedTools()) == 0 {
		t.Fatal("relevant tools should be populated from checked state after sidebar jump")
	}
	view := m.viewToolVersions()
	if !strings.Contains(view, "Alpine base") {
		t.Errorf("tool versions view missing Alpine base row:\n%s", view)
	}
	if !strings.Contains(view, "Go") {
		t.Errorf("tool versions view missing Go tool row:\n%s", view)
	}
}

func TestAlpineVersionOnlyOffersRealReleases(t *testing.T) {
	for _, v := range toolbox.AlpineVersions {
		if !toolbox.IsValidAlpineVersion(v) {
			t.Errorf("catalog version %q should be valid", v)
		}
	}
	for _, v := range []string{"3.0", "3.25", "4.0", "latest", ""} {
		if toolbox.IsValidAlpineVersion(v) {
			t.Errorf("invalid alpine version %q accepted", v)
		}
	}
}

func TestRelevantManagedTools(t *testing.T) {
	m := NewModel()
	m.checked["role:Backend"] = true
	m.checked["lang:Go"] = true
	m.checked["extool:Bun"] = true

	got := m.relevantManagedTools()
	want := map[string]bool{"go": true, "golangci-lint": true, "python": true, "bun": true}
	if len(got) != len(want) {
		t.Fatalf("relevantManagedTools = %d tools, want %d: %v", len(got), len(want), got)
	}
	for _, tool := range got {
		if !want[tool.Name] {
			t.Errorf("unexpected relevant tool %q", tool.Name)
		}
	}
}
