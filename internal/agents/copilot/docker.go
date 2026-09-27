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

package copilot

import (
	"fmt"
	"os"

	"github.com/cloud-exit/exitbox/internal/agent"
	"github.com/cloud-exit/exitbox/internal/agents/jstools"
)

// copilotNPMPackage is the official GitHub Copilot CLI npm package.
// The base image (Alpine 3.21) ships Node 22, satisfying Copilot's Node 22+ requirement.
const copilotNPMPackage = "@github/copilot"

func (c *Copilot) GetDockerfileInstall(buildCtx string) (string, error) {
	return c.installSnippet(agent.PrepareBuildInput{}), nil
}

// installSnippet renders the install step for the image flavor being built.
func (c *Copilot) installSnippet(in agent.PrepareBuildInput) string {
	return `# Install Node.js and GitHub Copilot CLI via npm (requires Node 22+)
ARG COPILOT_VERSION
` + jstools.InstallDependenciesFor(in, []string{"nodejs", "npm"}, []string{copilotNPMPackage + "@${COPILOT_VERSION}"}) + ` && \
    copilot --version
LABEL exitbox.agent.version="${COPILOT_VERSION}"`
}

func (c *Copilot) GetFullDockerfile(version string) (string, error) {
	return c.fullDockerfile(agent.PrepareBuildInput{Version: version}), nil
}

func (c *Copilot) fullDockerfile(in agent.PrepareBuildInput) string {
	version := in.Version
	if version == "" {
		version = "latest"
	}
	df := "FROM " + in.Base() + "\n\n"
	df += fmt.Sprintf("ARG COPILOT_VERSION=%s\n", version)
	df += c.installSnippet(in)
	return df
}

func (c *Copilot) PrepareBuild(in agent.PrepareBuildInput) error {
	version := in.Version
	if version == "" {
		var err error
		version, err = c.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest GitHub Copilot CLI version: %w", err)
		}
	}
	if in.Logf != nil {
		in.Logf("Building GitHub Copilot CLI image with version %s (npm install at build time)", version)
	}
	in.Version = version
	df := c.fullDockerfile(in)
	if err := os.WriteFile(in.DockerfilePath, []byte(df), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}
