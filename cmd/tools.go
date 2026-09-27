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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloud-exit/exitbox/internal/agents"
	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/container"
	"github.com/cloud-exit/exitbox/internal/image"
	"github.com/cloud-exit/exitbox/internal/toolbox"
	"github.com/cloud-exit/exitbox/internal/ui"
	"github.com/spf13/cobra"
)

var toolsRebuild bool

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage tools and languages in ExitBox images",
	Long:  "List, pin, and update the tools and languages installed in ExitBox images.",
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed tools and their configured versions",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.LoadOrDefault()

		ui.Cecho("Managed tools and languages:", ui.Cyan)
		fmt.Println()
		fmt.Printf("  %-16s %-10s %-22s %s\n", "TOOL", "TYPE", "VERSION", "INSTALL")
		fmt.Printf("  %-16s %-10s %-22s %s\n", "----", "----", "-------", "-------")
		fmt.Printf("  %-16s %-10s %-22s %s\n", "alpine", "base", cfg.AlpineVersionOrDefault(), "official Alpine image")
		for _, t := range toolbox.All {
			version := cfg.GetToolVersion(t.Name)
			if version == "" || version == "default" {
				version = "default"
				if t.Default != "" {
					version = "default (" + t.Default + ")"
				}
			}
			fmt.Printf("  %-16s %-10s %-22s %s\n", t.Name, t.Kind, version, installLabel(t))
		}
		fmt.Println()
		fmt.Println("Run `exitbox tools update <tool> [version]` to change a version.")
	},
}

var toolsUpdateCmd = &cobra.Command{
	Use:   "update [tool [version] | tool@version ...]",
	Short: "Update tools and languages to latest or a pinned version",
	Long: `Update the tools and languages installed in ExitBox images and rebuild
the images with the new versions.

With no arguments every managed tool is updated to "latest". "latest"
tracks the newest upstream version at image build time; a concrete version
pins the tool (for apk-installed tools the exact Alpine package version is
resolved before saving); "default" removes a pin and restores the built-in
install.

Versions are saved to config.yaml without rebuilding anything; use --rebuild
to rebuild the tools and project images for every enabled agent afterwards
(or run "exitbox rebuild all" or the next "exitbox run").

The active workspace must already include a tool's role/language for it to
be installed (e.g. "go" in a workspace's development list).

Examples:
  exitbox tools update                         # all tools to latest
  exitbox tools update go                      # Go to latest
  exitbox tools update go 1.35.0               # Pin Go to 1.35.0
  exitbox tools update go@1.35.0 node@22       # Pin multiple tools
  exitbox tools update python default          # Use the default Python again
  exitbox tools update --rebuild go 1.35.0     # Save and rebuild enabled agents`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runToolsUpdate(args)
	},
}

func init() {
	toolsUpdateCmd.Flags().BoolVar(&toolsRebuild, "rebuild", false, "Rebuild images for all enabled agents after saving versions")
	toolsCmd.AddCommand(toolsListCmd)
	toolsCmd.AddCommand(toolsUpdateCmd)
	rootCmd.AddCommand(toolsCmd)
}

func installLabel(t toolbox.Tool) string {
	switch t.Mode {
	case toolbox.ModeApk:
		return "Alpine apk"
	case toolbox.ModeTarball:
		return "official tarball"
	case toolbox.ModeNpm:
		return "npm"
	default:
		return string(t.Mode)
	}
}

type toolsUpdateRequest struct {
	name    string
	version string
}

// parseToolsUpdateArgs turns CLI arguments into per-tool update requests.
// Accepted forms: nothing (all latest), "go", "go 1.35.0", "go@1.35.0",
// "go=1.35.0", and arbitrary combinations of those.
func parseToolsUpdateArgs(args []string) ([]toolsUpdateRequest, error) {
	if len(args) == 0 {
		reqs := make([]toolsUpdateRequest, 0, len(toolbox.All))
		for _, t := range toolbox.All {
			reqs = append(reqs, toolsUpdateRequest{name: t.Name, version: "latest"})
		}
		return reqs, nil
	}
	if len(args) == 1 && (args[0] == "latest" || args[0] == "all") {
		return parseToolsUpdateArgs(nil)
	}

	var reqs []toolsUpdateRequest
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.Contains(arg, "@") || strings.Contains(arg, "=") {
			sep := "@"
			if !strings.Contains(arg, "@") {
				sep = "="
			}
			parts := strings.SplitN(arg, sep, 2)
			reqs = append(reqs, toolsUpdateRequest{name: parts[0], version: parts[1]})
			continue
		}

		req := toolsUpdateRequest{name: arg, version: "latest"}
		// Two-token form: "go 1.35.0". A second bare token is a version
		// unless it is another known tool name.
		if i+1 < len(args) &&
			!strings.Contains(args[i+1], "@") &&
			!strings.Contains(args[i+1], "=") &&
			toolbox.Get(args[i+1]) == nil {
			req.version = args[i+1]
			i++
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

type toolsVersionChange struct {
	name string
	old  string
	new  string
}

// applyToolVersionRequests resolves and stores the requested tool versions,
// returning only the entries that actually changed. Unchanged requests are
// skipped so repeated `exitbox tools update` runs are no-ops.
func applyToolVersionRequests(cfg *config.Config, reqs []toolsUpdateRequest) ([]toolsVersionChange, error) {
	var changes []toolsVersionChange
	for _, req := range reqs {
		if req.name == "alpine" {
			change, err := applyAlpineVersionRequest(cfg, req)
			if err != nil {
				return nil, err
			}
			if change != nil {
				changes = append(changes, *change)
			}
			continue
		}
		t := toolbox.Get(req.name)
		if t == nil {
			return nil, unknownToolError(req.name)
		}
		pin, err := t.ResolvePin(req.version)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve %s version %q: %w", t.Name, req.version, err)
		}

		old := cfg.GetToolVersion(t.Name)
		if pin == "default" {
			cfg.SetToolVersion(t.Name, "")
		} else {
			cfg.SetToolVersion(t.Name, pin)
		}
		newValue := cfg.GetToolVersion(t.Name)
		if old == newValue {
			continue
		}
		oldDisplay := old
		if oldDisplay == "" {
			oldDisplay = "default"
		}
		newDisplay := newValue
		if newDisplay == "" {
			newDisplay = "default"
		}
		changes = append(changes, toolsVersionChange{name: t.Name, old: oldDisplay, new: newDisplay})
	}
	return changes, nil
}

func runToolsUpdate(args []string) {
	if !config.ConfigExists() {
		ui.Error("No configuration found. Run `exitbox setup` first to configure your tools.")
	}

	reqs, err := parseToolsUpdateArgs(args)
	if err != nil {
		ui.Errorf("Invalid arguments: %v", err)
	}

	cfg := config.LoadOrDefault()
	changes, err := applyToolVersionRequests(cfg, reqs)
	if err != nil {
		ui.Errorf("%v", err)
	}
	if len(changes) == 0 {
		ui.Info("No tool version changes — nothing to save or rebuild.")
		return
	}

	if err := config.SaveConfig(cfg); err != nil {
		ui.Errorf("Failed to save config: %v", err)
	}

	fmt.Println("Updated tool versions:")
	for _, c := range changes {
		fmt.Printf("  %s: %s → %s\n", c.name, c.old, c.new)
	}

	if !toolsRebuild {
		ui.Info("Config saved. Run `exitbox rebuild all` (or the next `exitbox run`) to apply the changes.")
		return
	}

	rebuildToolsForAllAgents()
}

// applyAlpineVersionRequest handles the "alpine" base image pseudo-tool.
// Only real Alpine releases from the official mirror are accepted.
func applyAlpineVersionRequest(cfg *config.Config, req toolsUpdateRequest) (*toolsVersionChange, error) {
	old := cfg.Settings.AlpineVersion
	if old == "" {
		old = config.DefaultAlpineVersion
	}
	switch req.version {
	case "default":
		cfg.Settings.AlpineVersion = ""
	case "latest":
		return nil, fmt.Errorf("the Alpine base image has no latest; choose one of: %s", strings.Join(toolbox.AlpineVersions, ", "))
	default:
		if !toolbox.IsValidAlpineVersion(req.version) {
			return nil, fmt.Errorf("invalid Alpine version %q. Choose one of: %s", req.version, strings.Join(toolbox.AlpineVersions, ", "))
		}
		cfg.Settings.AlpineVersion = req.version
	}
	newValue := cfg.Settings.AlpineVersion
	if newValue == "" {
		newValue = config.DefaultAlpineVersion
	}
	if old == newValue {
		return nil, nil
	}
	return &toolsVersionChange{name: "alpine", old: old, new: newValue}, nil
}

func unknownToolError(name string) error {
	names := make([]string, 0, len(toolbox.All)+1)
	names = append(names, "alpine")
	for _, t := range toolbox.All {
		names = append(names, t.Name)
	}
	return fmt.Errorf("unknown managed tool %q. Available tools: %s", name, strings.Join(names, ", "))
}

func rebuildToolsForAllAgents() {
	rt := container.Detect()
	if rt == nil {
		ui.Warn("No container runtime found; versions were saved. Install Podman or Docker, then run `exitbox rebuild all`.")
		return
	}

	cfg := config.LoadOrDefault()
	var enabled []string
	for _, a := range agents.All() {
		if cfg.IsAgentEnabled(a.Name()) {
			enabled = append(enabled, a.Name())
		}
	}
	if len(enabled) == 0 {
		ui.Info("No agents are enabled; versions were saved. Enable an agent, then run `exitbox run` or `exitbox rebuild all`.")
		return
	}

	projectDir, _ := os.Getwd()
	ctx := context.Background()

	for _, name := range enabled {
		agt := agents.Get(name)
		ui.Infof("Rebuilding %s images with updated tools...", agt.DisplayName())

		image.Version = Version
		image.AutoUpdate = true
		version := cfg.GetAgentVersion(name)
		image.AgentVersion = version
		if version != "" {
			image.AutoUpdate = false
		}

		if err := image.BuildTools(ctx, rt, name, true, image.WorkspaceDistro(cfg, projectDir, "")); err != nil {
			ui.Errorf("Failed to rebuild %s tools image: %v", agt.DisplayName(), err)
		}
		if err := image.BuildProject(ctx, rt, name, projectDir, "", true); err != nil {
			ui.Errorf("Failed to rebuild %s project image: %v", agt.DisplayName(), err)
		}
		ui.Successf("%s image rebuilt with updated tools", agt.DisplayName())
	}
}
