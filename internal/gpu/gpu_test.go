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

package gpu

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHost replaces the host lookups for the duration of a test.
type fakeHost struct {
	binaries    map[string]bool
	devices     []string
	cdiDir      string
	dockerNvida bool
}

func (f fakeHost) install(t *testing.T) {
	t.Helper()
	origLook, origGlob, origStat, origRuntimes := lookPath, globFiles, statFile, runtimes
	origDirs := cdiSpecDirs

	lookPath = func(name string) (string, error) {
		if f.binaries[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	globFiles = func(pattern string) ([]string, error) {
		if strings.HasPrefix(pattern, "/dev/") {
			var out []string
			for _, d := range f.devices {
				if pattern == "/dev/nvidia[0-9]*" && strings.HasPrefix(d, "/dev/nvidia") &&
					d != "/dev/nvidiactl" && d != "/dev/nvidia-uvm" {
					out = append(out, d)
				}
				if pattern == d {
					out = append(out, d)
				}
			}
			return out, nil
		}
		return filepath.Glob(pattern)
	}
	statFile = os.Stat
	runtimes = func(string) bool { return f.dockerNvida }
	if f.cdiDir != "" {
		cdiSpecDirs = []string{f.cdiDir}
	} else {
		cdiSpecDirs = []string{filepath.Join(t.TempDir(), "empty")}
	}

	t.Cleanup(func() {
		lookPath, globFiles, statFile, runtimes = origLook, origGlob, origStat, origRuntimes
		cdiSpecDirs = origDirs
	})
}

func cdiDirWithNvidia(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	spec := "cdiVersion: 0.5.0\nkind: nvidia.com/gpu\n"
	if err := os.WriteFile(filepath.Join(dir, "nvidia.yaml"), []byte(spec), 0644); err != nil {
		t.Fatalf("writing CDI spec: %v", err)
	}
	return dir
}

func TestDetectDockerWithToolkit(t *testing.T) {
	fakeHost{
		binaries:    map[string]bool{"nvidia-smi": true, "nvidia-ctk": true},
		devices:     []string{"/dev/nvidia0", "/dev/nvidiactl"},
		dockerNvida: true,
	}.install(t)

	s := Detect("docker")
	if s.Mode != ModeDockerGPUs {
		t.Errorf("Mode = %q, want %q", s.Mode, ModeDockerGPUs)
	}
	if !s.Available() {
		t.Error("Available() = false")
	}
	if len(s.Problems) != 0 {
		t.Errorf("Problems = %v, want none", s.Problems)
	}
	if got := Args("docker", s); strings.Join(got, " ") != "--gpus all" {
		t.Errorf("Args() = %v", got)
	}
}

func TestDetectDockerWithoutToolkit(t *testing.T) {
	fakeHost{
		binaries: map[string]bool{"nvidia-smi": true},
		devices:  []string{"/dev/nvidia0", "/dev/nvidiactl"},
	}.install(t)

	s := Detect("docker")
	if s.Available() {
		t.Error("a host without the container toolkit cannot pass GPUs through")
	}
	if len(s.Problems) == 0 || !strings.Contains(strings.Join(s.Problems, " "), "nvidia-ctk runtime configure") {
		t.Errorf("Problems = %v, want the toolkit fix named", s.Problems)
	}
	if got := Args("docker", s); got != nil {
		t.Errorf("Args() = %v, want nil when unsupported", got)
	}
}

func TestDetectDockerFallsBackToCDI(t *testing.T) {
	fakeHost{
		binaries: map[string]bool{"nvidia-smi": true},
		devices:  []string{"/dev/nvidia0"},
		cdiDir:   cdiDirWithNvidia(t),
	}.install(t)

	s := Detect("docker")
	if s.Mode != ModeCDI {
		t.Errorf("Mode = %q, want %q", s.Mode, ModeCDI)
	}
	args := strings.Join(Args("docker", s), " ")
	if args != "--device nvidia.com/gpu=all" {
		t.Errorf("Args(docker, cdi) = %q", args)
	}
}

func TestDetectPodmanUsesCDI(t *testing.T) {
	fakeHost{
		binaries: map[string]bool{"nvidia-smi": true, "nvidia-ctk": true},
		devices:  []string{"/dev/nvidia0", "/dev/nvidiactl"},
		cdiDir:   cdiDirWithNvidia(t),
	}.install(t)

	s := Detect("podman")
	if s.Mode != ModeCDI {
		t.Errorf("Mode = %q, want %q", s.Mode, ModeCDI)
	}
	args := strings.Join(Args("podman", s), " ")
	if !strings.Contains(args, "nvidia.com/gpu=all") || !strings.Contains(args, "label=disable") {
		t.Errorf("Args(podman) = %q", args)
	}
}

func TestDetectPodmanWithoutCDISpec(t *testing.T) {
	fakeHost{
		binaries: map[string]bool{"nvidia-smi": true, "nvidia-ctk": true},
		devices:  []string{"/dev/nvidia0"},
	}.install(t)

	s := Detect("podman")
	if s.Available() {
		t.Error("podman without a CDI spec cannot pass GPUs through")
	}
	if !strings.Contains(strings.Join(s.Problems, " "), "nvidia-ctk cdi generate") {
		t.Errorf("Problems = %v, want the CDI fix named", s.Problems)
	}
}

func TestDetectWithoutDriver(t *testing.T) {
	fakeHost{
		binaries:    map[string]bool{"nvidia-ctk": true},
		dockerNvida: true,
	}.install(t)

	s := Detect("docker")
	if s.Available() {
		t.Error("no device nodes means no GPU, whatever the runtime supports")
	}
	problems := strings.Join(s.Problems, " ")
	if !strings.Contains(problems, "/dev/nvidia*") {
		t.Errorf("Problems = %v, want the missing device nodes named", s.Problems)
	}
	if !strings.Contains(problems, "nvidia-smi") {
		t.Errorf("Problems = %v, want the missing driver named", s.Problems)
	}
}

func TestEnvVarsRequestComputeCapability(t *testing.T) {
	s := Support{Mode: ModeDockerGPUs}
	vars := EnvVars(s)
	joined := strings.Join(vars, " ")
	if !strings.Contains(joined, "NVIDIA_VISIBLE_DEVICES=all") {
		t.Errorf("EnvVars() = %v", vars)
	}
	// Without "compute" the hook injects nvidia-smi and no CUDA libraries.
	if !strings.Contains(joined, "NVIDIA_DRIVER_CAPABILITIES=compute,utility") {
		t.Errorf("EnvVars() = %v, want the compute capability requested", vars)
	}
	if EnvVars(Support{Mode: ModeNone}) != nil {
		t.Error("EnvVars() should be empty when GPUs are unavailable")
	}
}

func TestEnvVarsHonoursCapabilityOverride(t *testing.T) {
	t.Setenv("EXITBOX_GPU_CAPABILITIES", "compute,utility,video")
	vars := EnvVars(Support{Mode: ModeCDI})
	if !strings.Contains(strings.Join(vars, " "), "NVIDIA_DRIVER_CAPABILITIES=compute,utility,video") {
		t.Errorf("EnvVars() = %v", vars)
	}
}
