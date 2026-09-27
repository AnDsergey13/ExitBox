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
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloud-exit/exitbox/internal/config"
)

// twoWorkspaceConfig has a GPU workspace that is not the active one, which
// is where per-workspace settings used to be read from the wrong workspace.
func twoWorkspaceConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Workspaces.Active = "default"
	gpuWs := config.Workspace{Name: "ml", GPU: true}
	gpuWs.SetPluginEntry("codex", "codex-router", config.PluginEntry{Enabled: true})
	cfg.Workspaces.Items = append(cfg.Workspaces.Items, gpuWs)
	return cfg
}

// selectWorkspace drives the workspace picker onto the named workspace and
// confirms it, exactly as the setup wizard does.
func selectWorkspace(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, ws := range m.workspaces {
		if ws.Name == name {
			m.cursor = i
		}
	}
	updated, _ := m.updateWorkspaceSelect(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
}

func settingsCursor(key string) int {
	for i, opt := range settingsOptions {
		if opt.Key == key {
			return i
		}
	}
	return 0
}

func TestSelectingWorkspaceLoadsItsGPUFlag(t *testing.T) {
	cfg := twoWorkspaceConfig()
	m := NewModelFromConfig(cfg)

	// The active workspace has no GPU, so the wizard starts with it off.
	if m.state.GPU || m.checked["setting:gpu"] {
		t.Fatalf("expected GPU off for the active workspace")
	}

	m = selectWorkspace(t, m, "ml")
	if !m.state.GPU || !m.checked["setting:gpu"] {
		t.Errorf("selecting the GPU workspace must load its flag: state=%v checked=%v",
			m.state.GPU, m.checked["setting:gpu"])
	}
	if !m.state.Plugins["codex"]["codex-router"].Enabled {
		t.Errorf("selecting a workspace must load its plugins: %v", m.state.Plugins)
	}
}

func TestSelectingWorkspaceDoesNotOverwriteItsGPUFlag(t *testing.T) {
	config.Home = t.TempDir()
	cfg := twoWorkspaceConfig()

	m := NewModelFromConfig(cfg)
	m = selectWorkspace(t, m, "ml")

	// The user changes nothing else and confirms.
	if err := applyResult(m.state, cfg); err != nil {
		t.Fatalf("applyResult: %v", err)
	}

	saved := config.LoadOrDefault()
	ws := saved.Workspace("ml")
	if ws == nil {
		t.Fatal("workspace ml missing after save")
	}
	if !ws.GPU {
		t.Error("GPU was switched off by a run that never touched it")
	}
	if !ws.IsPluginEnabled("codex", "codex-router") {
		t.Error("plugin configuration was dropped by a run that never touched it")
	}
}

func TestTogglingGPUSavesToTheSelectedWorkspace(t *testing.T) {
	config.Home = t.TempDir()
	cfg := twoWorkspaceConfig()

	m := NewModelFromConfig(cfg)
	m = selectWorkspace(t, m, "default")

	m.step = stepSettings
	m.cursor = settingsCursor("setting:gpu")
	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if err := applyResult(m.state, cfg); err != nil {
		t.Fatalf("applyResult: %v", err)
	}
	saved := config.LoadOrDefault()
	if ws := saved.Workspace("default"); ws == nil || !ws.GPU {
		t.Errorf("GPU not saved for the edited workspace: %+v", ws)
	}
	if ws := saved.Workspace("ml"); ws == nil || !ws.GPU {
		t.Errorf("editing one workspace must not disturb another: %+v", ws)
	}
}

func TestNewWorkspaceStartsWithoutGPU(t *testing.T) {
	cfg := twoWorkspaceConfig()
	m := NewModelFromConfig(cfg)
	m = selectWorkspace(t, m, "ml")

	// "Create new workspace" is the row past the existing ones.
	m.cursor = len(m.workspaces)
	updated, _ := m.updateWorkspaceSelect(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.state.GPU || m.checked["setting:gpu"] {
		t.Error("a new workspace must not inherit the previous workspace's GPU flag")
	}
	if m.state.Plugins != nil {
		t.Errorf("a new workspace must not inherit plugins: %v", m.state.Plugins)
	}
}

func TestReviewShowsPerWorkspaceSettings(t *testing.T) {
	cfg := twoWorkspaceConfig()
	m := NewModelFromConfig(cfg)
	m = selectWorkspace(t, m, "ml")
	m.step = stepReview

	review := m.viewReview()
	for _, want := range []string{"GPU:", "yes (Ubuntu CUDA image)", "Plugins:", "codex/codex-router", "RTK:"} {
		if !strings.Contains(review, want) {
			t.Errorf("review summary missing %q:\n%s", want, review)
		}
	}
}

// TestEverySettingReachesState is the guard for the class of bug that lost
// the GPU flag: a checkbox in the settings step that no snapshot path copies
// into wizard state. Flipping any one of them must change the state the
// review step renders and applyResult saves.
func TestEverySettingReachesState(t *testing.T) {
	for _, opt := range settingsOptions {
		t.Run(opt.Key, func(t *testing.T) {
			m := NewModelFromConfig(twoWorkspaceConfig())
			before := m.collectAllStepState().state

			m.checked[opt.Key] = !m.checked[opt.Key]
			after := m.collectAllStepState().state

			if reflect.DeepEqual(before, after) {
				t.Errorf("toggling %q changes nothing in wizard state, so it is dropped on save", opt.Key)
			}
		})
	}
}

// TestSidebarJumpKeepsSettingToggles covers the exact path that lost the
// flag: toggle in the settings step, then jump to Review from the sidebar
// without pressing Enter.
func TestSidebarJumpKeepsSettingToggles(t *testing.T) {
	config.Home = t.TempDir()
	cfg := twoWorkspaceConfig()

	m := NewModelFromConfig(cfg)
	m = selectWorkspace(t, m, "default")
	m.step = stepSettings
	m.cursor = settingsCursor("setting:gpu")

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)

	// Tab into the sidebar and pick Review.
	m.sidebarFocused = true
	for i, si := range m.visibleSidebarSteps() {
		if si.Step == stepReview {
			m.sidebarCursor = i
		}
	}
	updated, _ = m.updateSidebar(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.step != stepReview {
		t.Fatalf("expected the sidebar jump to land on Review, got %v", m.step)
	}
	if !m.state.GPU {
		t.Error("the GPU toggle was lost by the sidebar jump")
	}
	if !strings.Contains(m.viewReview(), "yes (Ubuntu CUDA image)") {
		t.Errorf("review does not show the GPU flag:\n%s", m.viewReview())
	}

	if err := applyResult(m.state, cfg); err != nil {
		t.Fatalf("applyResult: %v", err)
	}
	if ws := config.LoadOrDefault().Workspace("default"); ws == nil || !ws.GPU {
		t.Errorf("GPU not saved after a sidebar jump: %+v", ws)
	}
}
