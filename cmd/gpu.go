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
	"fmt"
	"os"
	"strings"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/container"
	"github.com/cloud-exit/exitbox/internal/distro"
	"github.com/cloud-exit/exitbox/internal/gpu"
	"github.com/cloud-exit/exitbox/internal/profile"
	"github.com/cloud-exit/exitbox/internal/ui"
	"github.com/spf13/cobra"
)

func newGPUCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpu",
		Short: "Inspect experimental GPU passthrough",
		Long: `Report whether this host can expose its GPUs to ExitBox containers.

GPU passthrough needs three things: the NVIDIA kernel driver (so
/dev/nvidia* exists), the NVIDIA Container Toolkit (which injects the
driver's userspace into the container), and a workspace with gpu: true.`,
	}
	cmd.AddCommand(newGPUStatusCmd())
	cmd.AddCommand(newGPUToggleCmd("enable", true))
	cmd.AddCommand(newGPUToggleCmd("disable", false))
	return cmd
}

// newGPUToggleCmd builds `exitbox gpu enable|disable [workspace]`, the
// non-wizard path for the per-workspace GPU flag.
func newGPUToggleCmd(verb string, enable bool) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " [workspace]",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " GPU passthrough for a workspace",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.LoadOrDefault()

			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			projectDir, err := os.Getwd()
			if err != nil {
				projectDir = ""
			}
			active, err := profile.ResolveActiveWorkspace(cfg, projectDir, name)
			if err != nil {
				ui.Errorf("%v", err)
			}
			if active == nil {
				ui.Error("No workspace configured. Run `exitbox setup` first.")
			}
			ws := cfg.Workspace(active.Workspace.Name)
			if ws == nil {
				ui.Errorf("Workspace '%s' not found in config", active.Workspace.Name)
			}

			if ws.GPU == enable {
				ui.Infof("GPU passthrough is already %sd for workspace '%s'", verb, ws.Name)
				return
			}
			ws.GPU = enable
			if err := config.SaveConfig(cfg); err != nil {
				ui.Errorf("Failed to save config: %v", err)
			}
			ui.Successf("GPU passthrough %sd for workspace '%s'", verb, ws.Name)

			if enable {
				fmt.Printf("This workspace now builds on %s instead of Alpine.\n", distro.CUDABaseImage)
				support := gpu.Detect(detectRuntimeCmd())
				if !support.Available() {
					ui.Warn("The host cannot pass GPUs through yet:")
					for _, problem := range support.Problems {
						ui.Warnf("  - %s", problem)
					}
				}
			} else {
				fmt.Println("This workspace goes back to the Alpine image.")
			}
			fmt.Println("Apply it with: exitbox rebuild <agent> (or the next `exitbox run <agent>`)")
		},
	}
}

// detectRuntimeCmd returns the container runtime command, defaulting to
// docker when none is installed so status output still makes sense.
func detectRuntimeCmd() string {
	if rt := container.Detect(); rt != nil {
		return container.Cmd(rt)
	}
	return "docker"
}

func newGPUStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show host GPU passthrough support and which workspaces use it",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runtimeCmd := detectRuntimeCmd()
			if container.Detect() == nil {
				ui.Warn("No container runtime found; reporting host GPU state only.")
			}

			support := gpu.Detect(runtimeCmd)

			ui.Cecho("GPU passthrough:", ui.Cyan)
			fmt.Println()
			fmt.Printf("  %-22s %s\n", "Container runtime:", runtimeCmd)
			fmt.Printf("  %-22s %s\n", "Mode:", string(support.Mode))
			fmt.Printf("  %-22s %s\n", "Device nodes:", devicesLabel(support.Devices))
			fmt.Printf("  %-22s %s\n", "nvidia-smi on host:", yesNo(support.DriverTool))
			fmt.Printf("  %-22s %s\n", "Container Toolkit:", yesNo(support.Toolkit))
			fmt.Printf("  %-22s %s\n", "CDI spec:", orNone(support.CDISpec))
			fmt.Printf("  %-22s %s\n", "Driver capabilities:", gpu.DefaultCapabilities)

			if len(support.Problems) > 0 {
				fmt.Println()
				ui.Cecho("Missing:", ui.Cyan)
				for _, problem := range support.Problems {
					fmt.Printf("  - %s\n", problem)
				}
			}

			cfg := config.LoadOrDefault()
			var enabled []string
			for _, ws := range cfg.Workspaces.Items {
				if ws.GPU {
					enabled = append(enabled, ws.Name)
				}
			}
			fmt.Println()
			if len(enabled) == 0 {
				fmt.Println("No workspace has GPU passthrough enabled (exitbox setup -> Settings -> GPU passthrough).")
			} else {
				fmt.Printf("Workspaces with GPU enabled: %s\n", strings.Join(enabled, ", "))
				fmt.Printf("Those workspaces build on the %s image (%s), not Alpine, because the\n",
					distro.Get(distro.GPU).DisplayName, distro.CUDABaseImage)
				fmt.Println("driver userspace the toolkit injects and every CUDA wheel need glibc.")
				fmt.Println("Switching a workspace's GPU flag rebuilds its images.")
			}
		},
	}
}

func devicesLabel(devices []string) string {
	if len(devices) == 0 {
		return "none found"
	}
	return strings.Join(devices, " ")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func init() {
	rootCmd.AddCommand(newGPUCmd())
}
