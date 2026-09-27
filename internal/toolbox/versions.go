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

package toolbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloud-exit/exitbox/internal/apk"
	"github.com/cloud-exit/exitbox/internal/config"
)

const (
	versionCacheTTL     = 15 * time.Minute
	maxDropdownVersions = 30
)

var versionCache struct {
	sync.Mutex
	m map[string]cachedVersions
}

type cachedVersions struct {
	versions []string
	fetched  time.Time
}

// AvailableVersions returns the real, published versions available for a
// managed tool, newest first and capped for dropdown display.
//
// Sources:
//   - apk tools: the Alpine package index for the configured Alpine release
//   - go: go.dev download API
//   - golangci-lint: GitHub releases API
//   - bun: npm registry
func AvailableVersions(t *Tool) ([]string, error) {
	if t == nil {
		return nil, fmt.Errorf("nil tool")
	}
	key := t.Name
	if t.Mode == ModeApk {
		key += "@" + config.LoadOrDefault().AlpineVersionOrDefault()
	}

	versionCache.Lock()
	if c, ok := versionCache.m[key]; ok && time.Since(c.fetched) < versionCacheTTL {
		versionCache.Unlock()
		return append([]string(nil), c.versions...), nil
	}
	versionCache.Unlock()

	var (
		versions []string
		err      error
	)
	switch t.Mode {
	case ModeApk:
		if len(t.Packages) == 0 {
			return nil, fmt.Errorf("%s has no Alpine package", t.Name)
		}
		versions, err = apk.AvailableVersionsFor(t.Packages[0], config.LoadOrDefault().AlpineVersionOrDefault())
	case ModeTarball:
		switch t.Name {
		case "go":
			versions, err = fetchGoVersions()
		case "golangci-lint":
			versions, err = fetchGitHubReleaseVersions("golangci/golangci-lint")
		default:
			return nil, fmt.Errorf("%s has no version source", t.Name)
		}
	case ModeNpm:
		versions, err = fetchNPMVersions("bun")
	default:
		return nil, fmt.Errorf("%s has unsupported version mode %q", t.Name, t.Mode)
	}
	if err != nil {
		return nil, err
	}

	versions = normalizeVersionList(versions)
	if len(versions) > maxDropdownVersions {
		versions = versions[:maxDropdownVersions]
	}

	versionCache.Lock()
	if versionCache.m == nil {
		versionCache.m = make(map[string]cachedVersions)
	}
	versionCache.m[key] = cachedVersions{versions: append([]string(nil), versions...), fetched: time.Now()}
	versionCache.Unlock()

	return append([]string(nil), versions...), nil
}

func fetchGoVersions() ([]string, error) {
	data, err := httpGet("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return nil, err
	}
	return parseGoVersions(data)
}

func parseGoVersions(data []byte) ([]string, error) {
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
		Files   []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("parsing go.dev JSON: %w", err)
	}
	var versions []string
	for _, r := range releases {
		if !r.Stable {
			continue
		}
		hasLinux := false
		for _, f := range r.Files {
			if f.OS == "linux" && (f.Arch == "amd64" || f.Arch == "arm64") {
				hasLinux = true
				break
			}
		}
		if !hasLinux {
			continue
		}
		versions = append(versions, strings.TrimPrefix(r.Version, "go"))
	}
	return versions, nil
}

func fetchGitHubReleaseVersions(repo string) ([]string, error) {
	u := "https://api.github.com/repos/" + repo + "/releases?per_page=50"
	data, err := httpGet(u)
	if err != nil {
		return nil, err
	}
	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("parsing GitHub releases JSON: %w", err)
	}
	versions := make([]string, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, strings.TrimPrefix(r.TagName, "v"))
	}
	return versions, nil
}

func fetchNPMVersions(pkg string) ([]string, error) {
	u := "https://registry.npmjs.org/" + url.PathEscape(pkg)
	data, err := httpGet(u)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing npm registry JSON: %w", err)
	}
	versions := make([]string, 0, len(doc.Versions))
	for v := range doc.Versions {
		versions = append(versions, v)
	}
	return versions, nil
}

func httpGet(u string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %d", u, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// normalizeVersionList sorts versions newest first and removes duplicates
// and empty entries.
func normalizeVersionList(versions []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		v = strings.TrimSpace(strings.TrimPrefix(v, "go"))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		c := compareVersionNumbers(out[i], out[j])
		if c != 0 {
			return c > 0
		}
		// Tie-break: stable releases come before canaries/betas.
		return isStableVersion(out[i]) && !isStableVersion(out[j])
	})
	return out
}

func isStableVersion(v string) bool {
	return !strings.ContainsAny(v, "-+")
}

// compareVersionNumbers compares dotted numeric versions. Returns positive
// when a is newer than b.
func compareVersionNumbers(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var ai, bi int64
		if i < len(ap) {
			ai = ap[i]
		}
		if i < len(bp) {
			bi = bp[i]
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int64 {
	base := v
	for _, sep := range []string{"-", "+", "_"} {
		if i := strings.Index(base, sep); i >= 0 {
			base = base[:i]
			break
		}
	}
	var out []int64
	for _, part := range strings.Split(base, ".") {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}
