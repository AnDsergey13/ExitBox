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

import "strings"

// debianPackages maps the Alpine package names used across ExitBox
// (development roles, tool categories, base image, managed tools) to their
// Debian/Ubuntu equivalents. A name that is absent is used unchanged, which
// covers the many packages that are spelled the same on both. An empty
// value means Ubuntu has no equivalent package: those are reported as
// unavailable rather than silently dropped, and are installed as binaries
// by the tools layer where ExitBox knows a download for them.
var debianPackages = map[string]string{
	// Base image and shell utilities.
	"bind-tools":      "dnsutils",
	"fd":              "fd-find",
	"bat":             "bat",
	"netcat-openbsd":  "netcat-openbsd",
	"iputils":         "iputils-ping iputils-tracepath",
	"libgo":           "", // gccgo's runtime is version-suffixed on Ubuntu (libgo22)
	"openssh-client":  "openssh-client",
	"procps":          "procps",
	"xz":              "xz-utils",
	"gzip":            "gzip",
	"bzip2":           "bzip2",
	"gawk":            "gawk",
	"gettext":         "gettext",
	"libgcc":          "libgcc-s1",
	"libstdc++":       "libstdc++6",
	"gcompat":         "", // musl-to-glibc shim: meaningless on glibc
	"musl-dev":        "libc6-dev",
	"linux-headers":   "linux-libc-dev",
	"alpine-sdk":      "build-essential",
	"coreutils":       "coreutils",
	"findutils":       "findutils",
	"grep":            "grep",
	"sed":             "sed",
	"tar":             "tar",
	"shadow":          "passwd",
	"su-exec":         "gosu",
	"tzdata":          "tzdata",
	"ca-certificates": "ca-certificates",

	// Languages and build tooling.
	"g++":                 "g++",
	"pkgconf":             "pkgconf",
	"openssl-dev":         "libssl-dev",
	"libffi-dev":          "libffi-dev",
	"zlib-dev":            "zlib1g-dev",
	"ncurses-dev":         "libncurses-dev",
	"readline-dev":        "libreadline-dev",
	"yaml-dev":            "libyaml-dev",
	"sqlite-dev":          "libsqlite3-dev",
	"libxml2-dev":         "libxml2-dev",
	"libxslt-dev":         "libxslt1-dev",
	"curl-dev":            "libcurl4-openssl-dev",
	"boost-dev":           "libboost-all-dev",
	"clang-extra-tools":   "clang-tools clang-format clang-tidy",
	"samurai":             "ninja-build",
	"go":                  "golang-go",
	"rust":                "rustc",
	"cargo":               "cargo",
	"ruby-dev":            "ruby-dev",
	"openjdk17-jdk":       "openjdk-17-jdk",
	"dotnet8-sdk":         "dotnet-sdk-8.0",
	"nodejs":              "nodejs",
	"npm":                 "npm",
	"python3":             "python3",
	"py3-pip":             "python3-pip",
	"py3-virtualenv":      "python3-venv",
	"R":                   "r-base-core",
	"php83":               "php8.3",
	"php83-cli":           "php8.3-cli",
	"php83-fpm":           "php8.3-fpm",
	"php83-mysqli":        "php8.3-mysql",
	"php83-pgsql":         "php8.3-pgsql",
	"php83-sqlite3":       "php8.3-sqlite3",
	"php83-curl":          "php8.3-curl",
	"php83-gd":            "php8.3-gd",
	"php83-mbstring":      "php8.3-mbstring",
	"php83-xml":           "php8.3-xml",
	"php83-zip":           "php8.3-zip",
	"postgresql16-client": "postgresql-client-16",
	"mariadb-client":      "mariadb-client",
	"sqlite":              "sqlite3",
	"redis":               "redis-tools",

	// Containers and infrastructure CLIs.
	"docker-cli":         "docker.io",
	"docker-cli-compose": "docker-compose-v2",
	"kubectl":            "", // no Ubuntu package: shipped as a binary download
	"helm":               "",
	"k9s":                "",
	"kustomize":          "",
	"opentofu":           "",

	// External tools.
	"github-cli": "gh",

	// Web and diagnostics.
	"apache2-utils": "apache2-utils",
	"httpie":        "httpie",
	"nginx":         "nginx",
	"nmap":          "nmap",
	"tcpdump":       "tcpdump",
	"iproute2":      "iproute2",
	"iptables":      "iptables",
	"ipset":         "ipset",
	"mandoc":        "mandoc",
	"bubblewrap":    "bubblewrap",
}

// BinaryFallback describes an official binary download used where a distro
// has no package for a tool.
type BinaryFallback struct {
	// URL carries {arch} (amd64/arm64), the same placeholder user-configured
	// binaries use.
	URL string
	// Bin is the executable's name inside the archive and on PATH. Empty
	// means it matches the tool name.
	Bin string
}

// BinaryFallbacks are the tools that have no Ubuntu package but publish an
// official Linux binary. The tools layer installs these on flavors that
// report them as unavailable.
var BinaryFallbacks = map[string]BinaryFallback{
	"kubectl":   {URL: "https://dl.k8s.io/release/v1.32.1/bin/linux/{arch}/kubectl"},
	"helm":      {URL: "https://get.helm.sh/helm-v3.16.4-linux-{arch}.tar.gz"},
	"kustomize": {URL: "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.5.0/kustomize_v5.5.0_linux_{arch}.tar.gz"},
	"k9s":       {URL: "https://github.com/derailed/k9s/releases/download/v0.32.7/k9s_Linux_{arch}.tar.gz"},
	"opentofu":  {URL: "https://github.com/opentofu/opentofu/releases/download/v1.9.0/tofu_1.9.0_linux_{arch}.zip", Bin: "tofu"},
}

// BinaryInstallSnippet returns a Dockerfile RUN step installing one binary
// fallback into /usr/local/bin. It handles the three shapes upstream
// projects publish: a bare binary, a .tar.gz, and a .zip.
func BinaryInstallSnippet(name string, fb BinaryFallback) string {
	bin := fb.Bin
	if bin == "" {
		bin = name
	}
	url := strings.ReplaceAll(fb.URL, "{arch}", "${ARCH}")
	head := "RUN ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') && \\\n"
	extract := func(archive, unpack string) string {
		return head +
			"    curl -sL \"" + url + "\" -o /tmp/" + name + archive + " && \\\n" +
			"    mkdir -p /tmp/" + name + "-x && " + unpack + " && \\\n" +
			"    find /tmp/" + name + "-x -type f -name " + bin + " -exec install -m 755 {} /usr/local/bin/" + bin + " \\; && \\\n" +
			"    rm -rf /tmp/" + name + archive + " /tmp/" + name + "-x"
	}
	switch {
	case strings.HasSuffix(fb.URL, ".tar.gz"), strings.HasSuffix(fb.URL, ".tgz"):
		return extract(".tar.gz", "tar -xzf /tmp/"+name+".tar.gz -C /tmp/"+name+"-x")
	case strings.HasSuffix(fb.URL, ".zip"):
		return extract(".zip", "unzip -q -o /tmp/"+name+".zip -d /tmp/"+name+"-x")
	default:
		return head +
			"    curl -sL \"" + url + "\" -o /usr/local/bin/" + bin + " && \\\n" +
			"    chmod +x /usr/local/bin/" + bin
	}
}
