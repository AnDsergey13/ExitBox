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
	"reflect"
	"testing"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/toolbox"
)

func TestParseToolsUpdateArgs_NoArgs(t *testing.T) {
	reqs, err := parseToolsUpdateArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) == 0 {
		t.Fatal("expected every managed tool for no-arg update")
	}
	for _, r := range reqs {
		if r.version != "latest" {
			t.Errorf("no-arg update version = %q, want latest", r.version)
		}
	}
}

func TestParseToolsUpdateArgs_SingleTool(t *testing.T) {
	reqs, err := parseToolsUpdateArgs([]string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	want := []toolsUpdateRequest{{name: "go", version: "latest"}}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("reqs = %v, want %v", reqs, want)
	}
}

func TestParseToolsUpdateArgs_ToolAndVersion(t *testing.T) {
	reqs, err := parseToolsUpdateArgs([]string{"go", "1.35.0"})
	if err != nil {
		t.Fatal(err)
	}
	want := []toolsUpdateRequest{{name: "go", version: "1.35.0"}}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("reqs = %v, want %v", reqs, want)
	}
}

func TestParseToolsUpdateArgs_ToolAtVersion(t *testing.T) {
	reqs, err := parseToolsUpdateArgs([]string{"go@1.35.0", "node@22"})
	if err != nil {
		t.Fatal(err)
	}
	want := []toolsUpdateRequest{
		{name: "go", version: "1.35.0"},
		{name: "node", version: "22"},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("reqs = %v, want %v", reqs, want)
	}
}

func TestParseToolsUpdateArgs_TwoBareTools(t *testing.T) {
	reqs, err := parseToolsUpdateArgs([]string{"go", "node"})
	if err != nil {
		t.Fatal(err)
	}
	want := []toolsUpdateRequest{
		{name: "go", version: "latest"},
		{name: "node", version: "latest"},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("reqs = %v, want %v", reqs, want)
	}
}

func TestParseToolsUpdateArgs_Mixed(t *testing.T) {
	reqs, err := parseToolsUpdateArgs([]string{"go", "1.35.0", "node@22", "python", "3.12"})
	if err != nil {
		t.Fatal(err)
	}
	want := []toolsUpdateRequest{
		{name: "go", version: "1.35.0"},
		{name: "node", version: "22"},
		{name: "python", version: "3.12"},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("reqs = %v, want %v", reqs, want)
	}
}

func TestApplyToolVersionRequests_NoChange(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SetToolVersion("go", "1.27.0")

	changes, err := applyToolVersionRequests(cfg, []toolsUpdateRequest{{name: "go", version: "1.27.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}
	if cfg.GetToolVersion("go") != "1.27.0" {
		t.Errorf("go version changed to %q", cfg.GetToolVersion("go"))
	}
}

func TestApplyToolVersionRequests_Change(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SetToolVersion("go", "1.35.0")

	changes, err := applyToolVersionRequests(cfg, []toolsUpdateRequest{{name: "go", version: "1.27.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].old != "1.35.0" || changes[0].new != "1.27.0" {
		t.Fatalf("changes = %v, want one 1.35.0 -> 1.27.0 change", changes)
	}
}

func TestApplyToolVersionRequests_ResetToDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SetToolVersion("go", "latest")

	changes, err := applyToolVersionRequests(cfg, []toolsUpdateRequest{{name: "go", version: "default"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].old != "latest" || changes[0].new != "default" {
		t.Fatalf("changes = %v, want one latest -> default change", changes)
	}
	if cfg.GetToolVersion("go") != "" {
		t.Errorf("go version = %q, want empty", cfg.GetToolVersion("go"))
	}
}

func TestApplyAlpineVersionRequest_Valid(t *testing.T) {
	cfg := config.DefaultConfig()
	change, err := applyAlpineVersionRequest(cfg, toolsUpdateRequest{name: "alpine", version: "3.24"})
	if err != nil {
		t.Fatal(err)
	}
	if change == nil || change.new != "3.24" {
		t.Fatalf("change = %+v, want 3.21 -> 3.24", change)
	}
	if cfg.AlpineVersionOrDefault() != "3.24" {
		t.Fatalf("alpine = %q, want 3.24", cfg.AlpineVersionOrDefault())
	}
}

func TestApplyAlpineVersionRequest_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, version := range []string{"latest", "3.25", "4.0", ""} {
		if _, err := applyAlpineVersionRequest(cfg, toolsUpdateRequest{name: "alpine", version: version}); err == nil {
			t.Errorf("applyAlpineVersionRequest(%q) expected error", version)
		}
	}
}

func TestApplyAlpineVersionRequest_Default(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Settings.AlpineVersion = "3.24"
	change, err := applyAlpineVersionRequest(cfg, toolsUpdateRequest{name: "alpine", version: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if change == nil || change.new != config.DefaultAlpineVersion {
		t.Fatalf("change = %+v, want reset to default", change)
	}
	if cfg.AlpineVersionOrDefault() != config.DefaultAlpineVersion {
		t.Fatalf("alpine = %q, want default", cfg.AlpineVersionOrDefault())
	}
}

func TestApplyAlpineVersionRequest_NoChange(t *testing.T) {
	cfg := config.DefaultConfig()
	change, err := applyAlpineVersionRequest(cfg, toolsUpdateRequest{name: "alpine", version: "3.21"})
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		t.Fatalf("expected no change, got %+v", change)
	}
}

func TestApplyToolVersionRequests_UnknownTool(t *testing.T) {
	cfg := config.DefaultConfig()
	if _, err := applyToolVersionRequests(cfg, []toolsUpdateRequest{{name: "nope", version: "1.0"}}); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestParseToolsUpdateArgs_AllLatest(t *testing.T) {
	for _, arg := range []string{"all", "latest"} {
		reqs, err := parseToolsUpdateArgs([]string{arg})
		if err != nil {
			t.Fatal(err)
		}
		if len(reqs) != len(toolbox.All) {
			t.Errorf("parse(%q) returned %d requests, want %d", arg, len(reqs), len(toolbox.All))
		}
	}
}
