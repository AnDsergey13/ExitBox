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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/container"
	"github.com/cloud-exit/exitbox/internal/ui"
	"github.com/cloud-exit/exitbox/static"
)

// BuildSquid builds the exitbox-squid proxy image.
func BuildSquid(ctx context.Context, rt container.Runtime, force bool) error {
	imageName := "exitbox-squid"
	cmd := container.Cmd(rt)
	cfg := config.LoadOrDefault()
	alpine := validAlpineVersion(cfg.AlpineVersionOrDefault())

	if !force && rt.ImageExists(imageName) {
		v, _ := rt.ImageInspect(imageName, `{{index .Config.Labels "exitbox.version"}}`)
		a, _ := rt.ImageInspect(imageName, `{{index .Config.Labels "exitbox.alpine"}}`)
		if a == "" {
			a = config.DefaultAlpineVersion
		}
		if v == Version && a == alpine {
			return nil
		}
		ui.Infof("Squid image mismatch (version %s, alpine %s). Rebuilding...", v, a)
	}

	// A custom Alpine release cannot be pulled from the registry; build locally.
	if isReleaseVersion(Version) && alpine != config.DefaultAlpineVersion {
		ui.Infof("Custom Alpine base image (%s), building squid locally...", "alpine:"+alpine)
		return buildSquidFull(rt, cmd, imageName)
	}

	// For release versions, try pulling the pre-built squid image from GHCR.
	if isReleaseVersion(Version) {
		remoteRef := SquidImageRegistry + ":" + Version
		if err := pullImage(rt, remoteRef, "Pulling Squid image..."); err == nil {
			if err := container.TagImage(rt, remoteRef, imageName); err == nil {
				ui.Success("Squid image ready (from registry)")
				return nil
			}
			ui.Warnf("Failed to tag squid image, building locally")
		} else {
			ui.Warnf("Could not pull %s, building locally", remoteRef)
		}
	}

	// Full local build (dev versions or when pull fails).
	return buildSquidFull(rt, cmd, imageName)
}

// templateSquidDockerfile renders the embedded squid Dockerfile with the
// configured Alpine release.
func templateSquidDockerfile(cfg *config.Config) []byte {
	version := validAlpineVersion(cfg.AlpineVersionOrDefault())
	from := "FROM alpine:" + config.DefaultAlpineVersion
	sq := strings.ReplaceAll(string(static.DockerfileSquid), from, "FROM alpine:"+version)
	sq += fmt.Sprintf("\nLABEL exitbox.alpine=\"%s\"\n", version)
	return []byte(sq)
}

// buildSquidFull builds the squid image locally from the embedded Dockerfile.
func buildSquidFull(rt container.Runtime, cmd, imageName string) error {
	ui.Info("Building Squid proxy image...")

	buildCtx := filepath.Join(config.Cache, "build-squid")
	if err := os.MkdirAll(buildCtx, 0755); err != nil {
		return fmt.Errorf("failed to create build context dir: %w", err)
	}

	cfg := config.LoadOrDefault()
	if err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), templateSquidDockerfile(cfg), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	args := buildArgs(cmd)
	args = append(args,
		"--build-arg", fmt.Sprintf("EXITBOX_VERSION=%s", Version),
		"-t", imageName,
		buildCtx,
	)

	if err := buildImage(rt, args, "Building Squid proxy image..."); err != nil {
		return fmt.Errorf("failed to build Squid image: %w", err)
	}

	return nil
}
