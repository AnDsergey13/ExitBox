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

package kimi

import (
	"fmt"
	"os"

	"github.com/cloud-exit/exitbox/internal/agent"
	"github.com/cloud-exit/exitbox/internal/agents/jstools"
)

// kimiNPMPackage is the official Moonshot AI Kimi Code CLI npm package. The package
// ships the CLI as a Node bundle (bin: kimi) with no platform restriction, so a global
// npm install on the Node-equipped base image is reliable across amd64/arm64.
const kimiNPMPackage = "@moonshot-ai/kimi-code"

func (k *Kimi) GetDockerfileInstall(buildCtx string) (string, error) {
	return k.installSnippet(agent.PrepareBuildInput{}), nil
}

// installSnippet renders the install step for the image flavor being built.
func (k *Kimi) installSnippet(in agent.PrepareBuildInput) string {
	return `# Install Node.js and the Kimi Code CLI via npm
ARG KIMI_VERSION
` + jstools.InstallDependenciesFor(in, []string{"nodejs", "npm"}, []string{kimiNPMPackage + "@${KIMI_VERSION}"}) + ` && \
    kimi --version
LABEL exitbox.agent.version="${KIMI_VERSION}"`
}

func (k *Kimi) GetFullDockerfile(version string) (string, error) {
	return k.fullDockerfile(agent.PrepareBuildInput{Version: version}), nil
}

func (k *Kimi) fullDockerfile(in agent.PrepareBuildInput) string {
	version := in.Version
	if version == "" {
		version = "latest"
	}
	df := "FROM " + in.Base() + "\n\n"
	df += fmt.Sprintf("ARG KIMI_VERSION=%s\n", version)
	df += k.installSnippet(in)
	return df
}

func (k *Kimi) PrepareBuild(in agent.PrepareBuildInput) error {
	version := in.Version
	if version == "" {
		var err error
		version, err = k.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest Kimi Code CLI version: %w", err)
		}
	}
	if in.Logf != nil {
		in.Logf("Building Kimi Code CLI image with version %s (npm install at build time)", version)
	}
	in.Version = version
	df := k.fullDockerfile(in)
	if err := os.WriteFile(in.DockerfilePath, []byte(df), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}
