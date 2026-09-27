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

package claude

import (
	"fmt"
	"os"

	"github.com/cloud-exit/exitbox/internal/agent"
)

func (c *Claude) GetFullDockerfile(version string) (string, error) {
	return c.fullDockerfile(agent.PrepareBuildInput{Version: version}), nil
}

func (c *Claude) fullDockerfile(in agent.PrepareBuildInput) string {
	df := "FROM " + in.Base() + "\n\n"
	if in.Version != "" {
		df += fmt.Sprintf("ARG CLAUDE_VERSION=%s\n\n", in.Version)
	}
	return df + c.installSnippet(in)
}

func (c *Claude) GetDockerfileInstall(buildCtx string) (string, error) {
	return c.installSnippet(agent.PrepareBuildInput{}), nil
}

// installSnippet renders the install step for the image flavor being built.
// The getdents shim and the gcompat preload exist because Claude Code ships a
// glibc-oriented binary that ExitBox runs on musl; a glibc image needs
// neither, and loading them there would break the binary it is meant to fix.
func (c *Claude) installSnippet(in agent.PrepareBuildInput) string {
	var prelude string
	if in.IsMusl() {
		prelude = `
` + in.InstallStep("musl-dev", "gcc") + ` && \
    printf '#include <sys/syscall.h>\n#include <unistd.h>\nint posix_getdents(int fd, void *buf, unsigned long nbytes, int flags) {\n  (void)flags;\n  return syscall(SYS_getdents64, fd, buf, nbytes);\n}\n' \
        > /tmp/posix_getdents.c && \
    gcc -shared -o /usr/local/lib/posix_getdents.so /tmp/posix_getdents.c && \
    rm /tmp/posix_getdents.c && \
    ` + in.RemoveStep("musl-dev", "gcc") + `

ENV LD_PRELOAD="/lib/libgcompat.so.0 /usr/local/lib/posix_getdents.so"
`
	}

	return prelude + `
ARG CLAUDE_VERSION
RUN set -e && \
    if [ -n "$CLAUDE_VERSION" ]; then \
        curl -fsSL https://claude.ai/install.sh | bash -s "$CLAUDE_VERSION"; \
    else \
        curl -fsSL https://claude.ai/install.sh | bash; \
    fi && \
    claude --version && \
    echo "Claude Code installed successfully"

# Configure settings for runtime user (as root, since /home/user may not be owned by user yet)
RUN mkdir -p /home/user/.claude && \
    echo '{"env":{"DISABLE_AUTOUPDATER":"1","USE_BUILTIN_RIPGREP":"0"}}' > /home/user/.claude/settings.json && \
    chown -R user:user /home/user/.claude`
}

func (c *Claude) PrepareBuild(in agent.PrepareBuildInput) error {
	df := c.fullDockerfile(in)
	if err := os.WriteFile(in.DockerfilePath, []byte(df), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}
