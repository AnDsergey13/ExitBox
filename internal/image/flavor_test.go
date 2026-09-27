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

package image

import (
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/distro"
)

func TestImageNamesPerFlavor(t *testing.T) {
	alpine, gpu := distro.Default(), distro.Get(distro.GPU)

	if got := BaseImageName(alpine); got != "exitbox-base" {
		t.Errorf("BaseImageName(alpine) = %q", got)
	}
	if got := BaseImageName(gpu); got != "exitbox-base-gpu" {
		t.Errorf("BaseImageName(gpu) = %q", got)
	}
	if got := CoreImageName("codex", gpu); got != "exitbox-codex-core-gpu" {
		t.Errorf("CoreImageName(gpu) = %q", got)
	}
	if got := ToolsImageName("codex", gpu); got != "exitbox-codex-tools-gpu" {
		t.Errorf("ToolsImageName(gpu) = %q", got)
	}
	if got := PluginsImageName("codex", "abc", gpu); got != "exitbox-codex-plugins-gpu-abc" {
		t.Errorf("PluginsImageName(gpu) = %q", got)
	}
}

func TestWorkspaceDistroFollowsGPUFlag(t *testing.T) {
	dir := t.TempDir()

	cfg := config.DefaultConfig()
	if got := WorkspaceDistro(cfg, dir, ""); got.ID != distro.Alpine {
		t.Errorf("WorkspaceDistro() = %q, want alpine", got.ID)
	}

	cfg.Workspace("default").GPU = true
	if got := WorkspaceDistro(cfg, dir, ""); got.ID != distro.GPU {
		t.Errorf("WorkspaceDistro() = %q, want gpu for a GPU workspace", got.ID)
	}
}

func TestToolsHashDiffersPerFlavor(t *testing.T) {
	cfg := config.DefaultConfig()
	if ToolsHash(cfg, distro.Default()) == ToolsHash(cfg, distro.Get(distro.GPU)) {
		t.Error("the flavors must not share a tools image hash")
	}
}

func TestTemplateBaseDockerfilePerFlavor(t *testing.T) {
	cfg := config.DefaultConfig()

	alpine := string(templateBaseDockerfile(cfg, distro.Default()))
	if !strings.Contains(alpine, "FROM docker.io/library/alpine:"+cfg.AlpineVersionOrDefault()) {
		t.Errorf("Alpine base Dockerfile does not build on the configured Alpine:\n%s", alpine[:200])
	}

	gpu := string(templateBaseDockerfile(cfg, distro.Get(distro.GPU)))
	if !strings.Contains(gpu, "FROM "+distro.CUDABaseImage) {
		t.Errorf("GPU base Dockerfile does not build on the CUDA image")
	}
	// Podman cannot resolve a short name here.
	if !strings.Contains(gpu, "FROM docker.io/") {
		t.Errorf("GPU base Dockerfile must name the registry:\n%s", gpu[:300])
	}
	if !strings.Contains(gpu, "NVIDIA_DRIVER_CAPABILITIES=compute,utility") {
		t.Error("GPU base image must request the compute capability")
	}
	if strings.Contains(gpu, "apk add") {
		t.Error("GPU base image must not use apk")
	}
}

func TestTemplateBaseDockerfileHonoursBaseImageOverride(t *testing.T) {
	t.Setenv(distro.GPUBaseImageEnv, "docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04")
	gpu := string(templateBaseDockerfile(config.DefaultConfig(), distro.Get(distro.GPU)))

	if !strings.Contains(gpu, "FROM docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04") {
		t.Errorf("override not applied to the FROM line:\n%s", gpu[:400])
	}
	if strings.Contains(gpu, "FROM "+distro.CUDABaseImage) {
		t.Error("the default base image is still in the Dockerfile")
	}
	if !strings.Contains(gpu, `LABEL exitbox.base.image="docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04"`) {
		t.Error("the built image should record which base it came from")
	}
}

func TestLocalIntermediaryDockerfilePerFlavor(t *testing.T) {
	alpine := string(localIntermediaryDockerfile(distro.Default()))
	if !strings.Contains(alpine, "adduser") {
		t.Error("Alpine intermediary should use BusyBox adduser")
	}
	gpu := string(localIntermediaryDockerfile(distro.Get(distro.GPU)))
	if !strings.Contains(gpu, "useradd") {
		t.Error("GPU intermediary should use Debian useradd")
	}
	if !strings.Contains(gpu, "userdel") {
		t.Error("GPU intermediary must free UID 1000, which Ubuntu ships in use")
	}
}

func TestPluginsDockerfileUsesFlavorPackageManager(t *testing.T) {
	ws := workspaceWithPlugin()
	hash := "abc"

	alpine := PluginsDockerfile(ws, "codex", "exitbox-codex-tools", hash, distro.Default())
	if !strings.Contains(alpine, "apk add") {
		t.Errorf("Alpine plugins layer should install with apk:\n%s", alpine)
	}

	gpu := PluginsDockerfile(ws, "codex", "exitbox-codex-tools-gpu", hash, distro.Get(distro.GPU))
	if !strings.Contains(gpu, "apt-get install") {
		t.Errorf("GPU plugins layer should install with apt:\n%s", gpu)
	}
	if strings.Contains(gpu, "apk add") {
		t.Errorf("GPU plugins layer must not use apk:\n%s", gpu)
	}
}
