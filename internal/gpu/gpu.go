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

// Package gpu detects whether the host can expose its GPUs to a container
// and renders the runtime flags that do it.
//
// GPU passthrough only works when three separate things line up:
//
//  1. the kernel driver is loaded, so /dev/nvidia* device nodes exist;
//  2. the NVIDIA Container Toolkit (or a CDI spec for Podman) is installed,
//     because that is what injects the host's driver userspace into the
//     container and creates the device nodes inside it;
//  3. the container asks for the right driver capabilities, or the hook
//     injects nothing but nvidia-smi and no CUDA libraries at all.
//
// Detection reports which of those are missing so the failure is named
// instead of showing up later as a container without a GPU.
package gpu

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Mode is how a container runtime is asked for GPUs.
type Mode string

const (
	// ModeNone means no supported way to pass GPUs through was found.
	ModeNone Mode = "none"
	// ModeDockerGPUs uses Docker's own --gpus flag, served by the NVIDIA
	// Container Toolkit's runtime hook.
	ModeDockerGPUs Mode = "docker-gpus"
	// ModeCDI uses a Container Device Interface device name, which is how
	// Podman (and newer Docker) consume GPUs.
	ModeCDI Mode = "cdi"
)

// DefaultCapabilities is what the toolkit hook injects when asked. Without
// "compute" there is no libcuda/CUDA runtime in the container, which is the
// most common reason a GPU container starts but no CUDA library is found.
const DefaultCapabilities = "compute,utility"

// Support describes what the host can actually do.
type Support struct {
	Mode Mode
	// Devices are the NVIDIA device nodes found on the host.
	Devices []string
	// DriverTool is true when nvidia-smi is on the host PATH.
	DriverTool bool
	// Toolkit is true when the NVIDIA Container Toolkit is installed.
	Toolkit bool
	// CDISpec is the CDI specification file that declares nvidia.com/gpu.
	CDISpec string
	// Problems lists what is missing, in the order it should be fixed.
	Problems []string
}

// Available reports whether GPUs can be passed to a container.
func (s Support) Available() bool { return s.Mode != ModeNone }

// Hooks for tests. Production code uses the real filesystem and PATH.
var (
	lookPath  = exec.LookPath
	globFiles = filepath.Glob
	statFile  = os.Stat
	runtimes  = dockerRuntimes
)

// cdiSpecDirs are the standard locations a CDI specification is written to.
var cdiSpecDirs = []string{"/etc/cdi", "/var/run/cdi", "/run/cdi"}

// Detect inspects the host for a usable GPU passthrough path. cmd is the
// container runtime command ("docker" or "podman").
func Detect(cmd string) Support {
	s := Support{Mode: ModeNone}

	s.Devices = nvidiaDevices()
	_, err := lookPath("nvidia-smi")
	s.DriverTool = err == nil
	s.Toolkit = toolkitInstalled()
	s.CDISpec = cdiSpec()

	if len(s.Devices) == 0 {
		s.Problems = append(s.Problems, "no NVIDIA device nodes on the host (/dev/nvidia*): the kernel driver is not loaded")
	}
	if !s.DriverTool {
		s.Problems = append(s.Problems, "nvidia-smi is not on the host PATH: install the NVIDIA driver")
	}

	switch cmd {
	case "podman":
		// Podman consumes GPUs as CDI devices; the spec is generated with
		// `nvidia-ctk cdi generate`.
		if s.CDISpec != "" {
			s.Mode = ModeCDI
		} else {
			s.Problems = append(s.Problems,
				"no CDI spec for nvidia.com/gpu: run `sudo nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml`")
		}
	default:
		if s.Toolkit && runtimes(cmd) {
			s.Mode = ModeDockerGPUs
		} else if s.CDISpec != "" {
			s.Mode = ModeCDI
		} else {
			s.Problems = append(s.Problems,
				"the NVIDIA Container Toolkit is not configured for "+cmd+
					": install it, then run `sudo nvidia-ctk runtime configure --runtime="+cmd+"` and restart "+cmd)
		}
	}

	if len(s.Devices) == 0 || !s.DriverTool {
		// Devices and driver are prerequisites; the flags would be accepted
		// but the container would come up without a GPU.
		s.Mode = ModeNone
	}
	return s
}

// Args returns the runtime flags exposing the host GPUs to the container.
func Args(cmd string, s Support) []string {
	switch s.Mode {
	case ModeDockerGPUs:
		return []string{"--gpus", "all"}
	case ModeCDI:
		args := []string{"--device", "nvidia.com/gpu=all"}
		if cmd == "podman" {
			// The device nodes are SELinux-labelled on the host; without
			// this the container cannot open them.
			args = append(args, "--security-opt=label=disable")
		}
		return args
	default:
		return nil
	}
}

// EnvVars returns the environment the NVIDIA container hook reads. Docker's
// --gpus flag sets NVIDIA_VISIBLE_DEVICES itself, but the capability list
// defaults to "utility" alone, which injects nvidia-smi and no CUDA
// libraries, so it is always passed explicitly.
func EnvVars(s Support) []string {
	if !s.Available() {
		return nil
	}
	return []string{
		"NVIDIA_VISIBLE_DEVICES=all",
		"NVIDIA_DRIVER_CAPABILITIES=" + capabilities(),
	}
}

// capabilities returns the driver capabilities requested from the hook,
// overridable for workloads that need graphics or video as well.
func capabilities() string {
	if v := strings.TrimSpace(os.Getenv("EXITBOX_GPU_CAPABILITIES")); v != "" {
		return v
	}
	return DefaultCapabilities
}

// nvidiaDevices returns the NVIDIA device nodes present on the host.
func nvidiaDevices() []string {
	var out []string
	for _, pattern := range []string{"/dev/nvidia[0-9]*", "/dev/nvidiactl", "/dev/nvidia-uvm"} {
		matches, err := globFiles(pattern)
		if err != nil {
			continue
		}
		out = append(out, matches...)
	}
	return out
}

// toolkitInstalled reports whether the NVIDIA Container Toolkit binaries are
// installed on the host.
func toolkitInstalled() bool {
	for _, bin := range []string{"nvidia-ctk", "nvidia-container-runtime", "nvidia-container-toolkit"} {
		if _, err := lookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// cdiSpec returns the CDI specification file declaring nvidia.com/gpu, or "".
func cdiSpec() string {
	for _, dir := range cdiSpecDirs {
		if info, err := statFile(dir); err != nil || !info.IsDir() {
			continue
		}
		for _, ext := range []string{"*.yaml", "*.yml", "*.json"} {
			matches, err := globFiles(filepath.Join(dir, ext))
			if err != nil {
				continue
			}
			for _, path := range matches {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				if strings.Contains(string(data), "nvidia.com/gpu") {
					return path
				}
			}
		}
	}
	return ""
}

// dockerRuntimes reports whether the container runtime knows the nvidia
// runtime, which is what `--gpus` needs behind it.
func dockerRuntimes(cmd string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd, "info", "--format", "{{json .Runtimes}}").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "nvidia")
}
