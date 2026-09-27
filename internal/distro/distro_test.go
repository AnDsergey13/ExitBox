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

package distro

import (
	"reflect"
	"strings"
	"testing"
)

func TestForPicksGPUFlavor(t *testing.T) {
	if got := For(false); got.ID != Alpine {
		t.Errorf("For(false) = %q, want alpine", got.ID)
	}
	gpu := For(true)
	if gpu.ID != GPU {
		t.Errorf("For(true) = %q, want gpu", gpu.ID)
	}
	if gpu.Libc != Gnu {
		t.Errorf("GPU flavor libc = %q, want gnu", gpu.Libc)
	}
	if !strings.Contains(gpu.BaseImage, "nvidia/cuda:") {
		t.Errorf("GPU base image = %q, want a CUDA image", gpu.BaseImage)
	}
	// Podman resolves no short names without a registries.conf, and
	// "nvidia/cuda" has no shortname alias, so the registry host is required.
	if !strings.HasPrefix(gpu.BaseImage, "docker.io/") {
		t.Errorf("GPU base image = %q, want a fully qualified reference", gpu.BaseImage)
	}
}

func TestGPUBaseImageOverride(t *testing.T) {
	d := Get(GPU)
	if got := d.BaseImageRef(); got != CUDABaseImage {
		t.Errorf("BaseImageRef() = %q, want the default", got)
	}
	t.Setenv(GPUBaseImageEnv, "docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04")
	if got := d.BaseImageRef(); got != "docker.io/nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04" {
		t.Errorf("BaseImageRef() = %q, want the override", got)
	}
	// The override is GPU only; Alpine keeps its own base.
	if got := Default().BaseImageRef(); got != "alpine" {
		t.Errorf("Alpine BaseImageRef() = %q", got)
	}
}

func TestImageSuffixKeepsFlavorsApart(t *testing.T) {
	if got := Default().ImageSuffix(); got != "" {
		t.Errorf("Alpine suffix = %q, want empty so existing image names are unchanged", got)
	}
	if got := Get(GPU).ImageSuffix(); got != "-gpu" {
		t.Errorf("GPU suffix = %q", got)
	}
}

func TestGetUnknownFallsBackToDefault(t *testing.T) {
	if got := Get("does-not-exist"); got.ID != Alpine {
		t.Errorf("Get(unknown) = %q, want the default flavor", got.ID)
	}
}

func TestAlpinePackagesPassThrough(t *testing.T) {
	names := []string{"nodejs", "postgresql16-client", "kubectl"}
	mapped, unavailable := Default().Packages(names)
	if !reflect.DeepEqual(mapped, names) {
		t.Errorf("Packages() = %v, want them unchanged on Alpine", mapped)
	}
	if len(unavailable) != 0 {
		t.Errorf("unavailable = %v, want none on Alpine", unavailable)
	}
}

func TestGPUPackageTranslation(t *testing.T) {
	d := Get(GPU)
	tests := map[string]string{
		"postgresql16-client": "postgresql-client-16",
		"openjdk17-jdk":       "openjdk-17-jdk",
		"sqlite":              "sqlite3",
		"bind-tools":          "dnsutils",
		"php83-mysqli":        "php8.3-mysql",
		"openssl-dev":         "libssl-dev",
		"nodejs":              "nodejs", // spelled the same
		"htop":                "htop",   // not in the map at all
	}
	for alpine, want := range tests {
		got, ok := d.Package(alpine)
		if !ok {
			t.Errorf("Package(%q) reported unavailable", alpine)
			continue
		}
		if got != want {
			t.Errorf("Package(%q) = %q, want %q", alpine, got, want)
		}
	}
}

func TestGPUPackageUnavailable(t *testing.T) {
	d := Get(GPU)
	if _, ok := d.Package("kubectl"); ok {
		t.Error("kubectl has no Ubuntu package and must be reported as unavailable")
	}
	mapped, unavailable := d.Packages([]string{"git", "kubectl", "helm"})
	if len(mapped) != 1 || mapped[0] != "git" {
		t.Errorf("mapped = %v, want just git", mapped)
	}
	if !reflect.DeepEqual(unavailable, []string{"kubectl", "helm"}) {
		t.Errorf("unavailable = %v", unavailable)
	}
	// Every unavailable package that ExitBox knows a download for has a
	// binary fallback, so the tools layer can still install it.
	for _, name := range unavailable {
		if _, ok := BinaryFallbacks[name]; !ok {
			t.Errorf("no binary fallback for %q", name)
		}
	}
}

func TestPackageKeepsVersionPin(t *testing.T) {
	got, ok := Get(GPU).Package("postgresql16-client=16.4")
	if !ok || got != "postgresql-client-16=16.4" {
		t.Errorf("Package(pinned) = %q, %v", got, ok)
	}
}

func TestPackagesDeduplicates(t *testing.T) {
	// clang-extra-tools expands to several Debian packages; a second entry
	// naming one of them must not repeat it.
	mapped, _ := Get(GPU).Packages([]string{"clang-extra-tools", "clang-format"})
	seen := map[string]int{}
	for _, p := range mapped {
		seen[p]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("package %q appears %d times: %v", name, count, mapped)
		}
	}
}

func TestInstallSnippets(t *testing.T) {
	alpine := Default().InstallSnippet([]string{"git", "curl"})
	if !strings.Contains(alpine, "apk add --no-cache git curl") {
		t.Errorf("Alpine install = %q", alpine)
	}
	gpu := Get(GPU).InstallSnippet([]string{"git", "bind-tools"})
	if !strings.Contains(gpu, "apt-get install -y --no-install-recommends git dnsutils") {
		t.Errorf("GPU install = %q", gpu)
	}
	if !strings.HasPrefix(gpu, "RUN ") {
		t.Errorf("install snippet must be a RUN step: %q", gpu)
	}
	if !strings.Contains(gpu, "rm -rf /var/lib/apt/lists/*") {
		t.Errorf("GPU install should clean the package lists: %q", gpu)
	}
	if got := Get(GPU).InstallSnippet([]string{"kubectl"}); got != "" {
		t.Errorf("an install with nothing installable should be empty, got %q", got)
	}
}

func TestInstallAndRemoveCommands(t *testing.T) {
	if got := Default().InstallCommand([]string{"gcc"}); got != "apk add --no-cache gcc" {
		t.Errorf("InstallCommand() = %q", got)
	}
	if got := Default().RemoveCommand([]string{"gcc"}); got != "apk del gcc" {
		t.Errorf("RemoveCommand() = %q", got)
	}
	gpu := Get(GPU).RemoveCommand([]string{"musl-dev"})
	if !strings.Contains(gpu, "apt-get purge -y libc6-dev") {
		t.Errorf("GPU RemoveCommand() = %q", gpu)
	}
}

func TestBinaryInstallSnippet(t *testing.T) {
	raw := BinaryInstallSnippet("kubectl", BinaryFallbacks["kubectl"])
	if !strings.Contains(raw, "-o /usr/local/bin/kubectl") || !strings.Contains(raw, "chmod +x") {
		t.Errorf("raw binary install = %q", raw)
	}
	if !strings.Contains(raw, "${ARCH}") {
		t.Errorf("architecture placeholder not resolved to a shell variable: %q", raw)
	}

	tarball := BinaryInstallSnippet("helm", BinaryFallbacks["helm"])
	if !strings.Contains(tarball, "tar -xzf") || !strings.Contains(tarball, "/usr/local/bin/helm") {
		t.Errorf("tarball install = %q", tarball)
	}

	// opentofu ships its binary as "tofu": the installed command keeps the
	// upstream name, as it does on Alpine.
	zip := BinaryInstallSnippet("opentofu", BinaryFallbacks["opentofu"])
	if !strings.Contains(zip, "unzip -q") || !strings.Contains(zip, "/usr/local/bin/tofu") {
		t.Errorf("zip install = %q", zip)
	}
}

func TestLibcTag(t *testing.T) {
	if got := Default().LibcTag(); got != "musl" {
		t.Errorf("Alpine libc tag = %q", got)
	}
	if got := Get(GPU).LibcTag(); got != "gnu" {
		t.Errorf("GPU libc tag = %q", got)
	}
}

func TestNames(t *testing.T) {
	got := Names()
	want := []string{"alpine", "gpu"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestGPUInstallsNodeFromUpstream(t *testing.T) {
	// Ubuntu 24.04 packages Node 18; the agents need 20+ and Copilot 22+,
	// so Node comes from nodejs.org with a checksum check instead.
	got := Get(GPU).InstallSnippet([]string{"nodejs", "npm"})
	for _, want := range []string{
		"https://nodejs.org/dist/" + NodeVersion,
		"SHASUMS256.txt",
		"sha256sum -c -",
		"tar -xJf",
		"node --version && npm --version",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("GPU node install missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "apt-get install -y --no-install-recommends nodejs") {
		t.Errorf("node must not come from apt:\n%s", got)
	}
	if !strings.HasPrefix(got, "RUN ") || strings.Count(got, "\nRUN ") != 0 {
		t.Errorf("the install must stay a single RUN step so callers can chain onto it:\n%s", got)
	}
}

func TestGPUInstallsNodeAlongsideOtherPackages(t *testing.T) {
	got := Get(GPU).InstallSnippet([]string{"nodejs", "npm", "git", "bind-tools"})
	if !strings.Contains(got, "apt-get install -y --no-install-recommends git dnsutils") {
		t.Errorf("other packages should still come from apt:\n%s", got)
	}
	if !strings.Contains(got, "nodejs.org") {
		t.Errorf("node should still be installed from upstream:\n%s", got)
	}
}

func TestAlpineKeepsNodeFromPackages(t *testing.T) {
	got := Default().InstallSnippet([]string{"nodejs", "npm"})
	if got != "RUN --mount=type=cache,target=/var/cache/apk apk add --no-cache nodejs npm" {
		t.Errorf("Alpine node install changed: %q", got)
	}
}

func TestGPUMapsPackagesFromRealWorkspaces(t *testing.T) {
	// Names taken from a real workspace package list that broke the build.
	d := Get(GPU)
	if got, ok := d.Package("iputils"); !ok || got != "iputils-ping iputils-tracepath" {
		t.Errorf("Package(iputils) = %q, %v", got, ok)
	}
	if _, ok := d.Package("libgo"); ok {
		t.Error("libgo has no Ubuntu package and must be reported as unavailable")
	}
}

func TestGPUInstallSurvivesUnknownPackages(t *testing.T) {
	// A package name that exists only on Alpine must not fail the image:
	// apt is retried per package so everything else still installs.
	got := Get(GPU).InstallSnippet([]string{"git", "some-alpine-only-name"})
	for _, want := range []string{
		"if ! DEBIAN_FRONTEND=noninteractive apt-get install",
		"for pkg in git some-alpine-only-name",
		"skipped unavailable package",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("install step missing %q:\n%s", want, got)
		}
	}
}

func TestAlpineInstallStaysStrict(t *testing.T) {
	// The default flavor keeps apk's behaviour: a bad package fails the
	// build, because Alpine names are what the config is written in.
	got := Default().InstallSnippet([]string{"git", "htop"})
	if strings.Contains(got, "skipped unavailable package") {
		t.Errorf("Alpine install should not tolerate unknown packages: %q", got)
	}
}
