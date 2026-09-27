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

// Package apk provides Alpine package index search functionality.
package apk

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloud-exit/exitbox/internal/config"
)

// Package represents an Alpine package entry.
type Package struct {
	Name        string
	Version     string
	Description string
}

// cacheMaxAge is the maximum age of the cached APKINDEX before re-downloading.
const cacheMaxAge = 24 * time.Hour

// repos are the Alpine repositories to search.
var repos = []string{"main", "community"}

// indexURL returns the APKINDEX.tar.gz URL for a given repository and
// Alpine release branch (e.g. "3.21").
func indexURL(version, repo string) string {
	return fmt.Sprintf("https://dl-cdn.alpinelinux.org/alpine/v%s/%s/x86_64/APKINDEX.tar.gz", version, repo)
}

// cacheDir returns the directory for cached APKINDEX files.
func cacheDir() string {
	return filepath.Join(config.Cache, "apkindex")
}

// cacheFile returns the path to a cached repo index file for an Alpine
// release branch.
func cacheFile(version, repo string) string {
	return filepath.Join(cacheDir(), version, repo+".txt")
}

// isCacheFresh returns true if the cache file exists and is less than cacheMaxAge old.
func isCacheFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < cacheMaxAge
}

// downloadIndex fetches APKINDEX.tar.gz and extracts the APKINDEX text.
func downloadIndex(version, repo string) ([]byte, error) {
	url := indexURL(version, repo)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decompressing %s: %w", repo, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar %s: %w", repo, err)
		}
		if hdr.Name == "APKINDEX" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading APKINDEX from %s: %w", repo, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("APKINDEX not found in %s archive", repo)
}

// ensureCache downloads and caches the index for a repo if not fresh.
func ensureCache(version, repo string) error {
	path := cacheFile(version, repo)
	if isCacheFresh(path) {
		return nil
	}

	data, err := downloadIndex(version, repo)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing cache %s: %w", path, err)
	}

	return nil
}

// parseIndex parses the APKINDEX text format into Package entries.
// Entries are separated by blank lines. P: = package name, T: = description.
func parseIndex(data []byte) []Package {
	var packages []Package
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	var name, version, desc string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if name != "" {
				packages = append(packages, Package{Name: name, Version: version, Description: desc})
			}
			name = ""
			version = ""
			desc = ""
			continue
		}
		if strings.HasPrefix(line, "P:") {
			name = strings.TrimPrefix(line, "P:")
		} else if strings.HasPrefix(line, "V:") {
			version = strings.TrimPrefix(line, "V:")
		} else if strings.HasPrefix(line, "T:") {
			desc = strings.TrimPrefix(line, "T:")
		}
	}
	// Handle last entry without trailing blank line
	if name != "" {
		packages = append(packages, Package{Name: name, Version: version, Description: desc})
	}

	return packages
}

// LoadIndex downloads (if needed) and parses the full Alpine package index
// for the configured Alpine release.
func LoadIndex() ([]Package, error) {
	return LoadIndexFor(config.LoadOrDefault().AlpineVersionOrDefault())
}

// LoadIndexFor downloads (if needed) and parses the full Alpine package
// index for a specific Alpine release branch (e.g. "3.21").
func LoadIndexFor(version string) ([]Package, error) {
	var all []Package
	for _, repo := range repos {
		if err := ensureCache(version, repo); err != nil {
			return nil, fmt.Errorf("loading %s index: %w", repo, err)
		}

		data, err := os.ReadFile(cacheFile(version, repo))
		if err != nil {
			return nil, fmt.Errorf("reading cache for %s: %w", repo, err)
		}

		all = append(all, parseIndex(data)...)
	}

	// Deduplicate by name (main takes precedence over community)
	seen := make(map[string]bool, len(all))
	deduped := make([]Package, 0, len(all))
	for _, p := range all {
		if !seen[p.Name] {
			seen[p.Name] = true
			deduped = append(deduped, p)
		}
	}

	return deduped, nil
}

// Search filters packages by case-insensitive substring match on name and description.
// Returns at most maxResults results.
func Search(index []Package, query string, maxResults int) []Package {
	if query == "" {
		return nil
	}

	q := strings.ToLower(query)
	var results []Package

	// Prioritize name matches first, then description-only matches
	var descOnly []Package
	for _, p := range index {
		if len(results)+len(descOnly) >= maxResults*2 {
			break
		}
		nameLower := strings.ToLower(p.Name)
		if strings.Contains(nameLower, q) {
			results = append(results, p)
		} else if strings.Contains(strings.ToLower(p.Description), q) {
			descOnly = append(descOnly, p)
		}
	}

	results = append(results, descOnly...)
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

// AvailableVersions returns the versions available for an Alpine package,
// newest first. The result uses the full apk version strings (e.g.
// "3.12.9-r0") that can be passed to `apk add pkg=version`.
func AvailableVersions(name string) ([]string, error) {
	return AvailableVersionsFor(name, config.LoadOrDefault().AlpineVersionOrDefault())
}

// AvailableVersionsFor returns the versions available for an Alpine package
// in a specific Alpine release branch, newest first.
func AvailableVersionsFor(name, alpineVersion string) ([]string, error) {
	index, err := LoadIndexFor(alpineVersion)
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, p := range index {
		if p.Name == name && p.Version != "" {
			versions = append(versions, p.Version)
		}
	}
	sortVersionsDesc(versions)
	return versions, nil
}

// ResolveVersion finds the exact Alpine version for a package that matches
// the requested prefix. A full version (e.g. "3.12.9-r0") is returned
// unchanged; a partial prefix (e.g. "3.12") is matched against the package
// index and the newest matching version is returned.
func ResolveVersion(name, requested string) (string, error) {
	requested = strings.TrimPrefix(requested, name+"=")
	if requested == "" {
		return "", fmt.Errorf("empty version for %s", name)
	}
	versions, err := AvailableVersions(name)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for Alpine package %q", name)
	}
	return matchVersion(versions, requested, name)
}

// matchVersion picks the requested version from a sorted (newest-first)
// version list. Full versions pass through; partial prefixes match the
// newest version with that prefix.
func matchVersion(versions []string, requested, name string) (string, error) {
	// Full version string: pass through so `apk add pkg=version` can validate.
	for _, v := range versions {
		if v == requested {
			return requested, nil
		}
	}
	// Match before the Alpine revision suffix (e.g. "3.12.9" or "3.12").
	for _, v := range versions {
		if strings.HasPrefix(v, requested) || strings.HasPrefix(stripRevision(v), requested) {
			return v, nil
		}
	}
	available := strings.Join(versions[:min(len(versions), 5)], ", ")
	return "", fmt.Errorf("no version %q found for %s (available: %s)", requested, name, available)
}

func stripRevision(v string) string {
	if i := strings.Index(v, "-r"); i >= 0 {
		return v[:i]
	}
	return v
}

// sortVersionsDesc sorts Alpine version strings newest first.
func sortVersionsDesc(versions []string) {
	for i := 1; i < len(versions); i++ {
		for j := i; j > 0 && compareAlpineVersions(versions[j-1], versions[j]) < 0; j-- {
			versions[j-1], versions[j] = versions[j], versions[j-1]
		}
	}
}

// compareAlpineVersions compares two Alpine version strings (e.g.
// "3.12.9-r0"). Returns negative if a < b, positive if a > b, zero if equal.
func compareAlpineVersions(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int64
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
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

// splitVersion splits "3.12.9-r0" into integer parts [3, 12, 9, 0].
// Non-numeric suffixes are ignored for ordering purposes.
func splitVersion(v string) []int64 {
	base := v
	if i := strings.IndexAny(base, "-+"); i >= 0 {
		base = base[:i]
	}
	var out []int64
	for _, part := range strings.Split(base, ".") {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	if i := strings.Index(v, "-r"); i >= 0 {
		if n, err := strconv.ParseInt(v[i+2:], 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// FetchAndSearch handles download/cache/parse/search in one call.
// It loads the index if needed and searches for the query.
func FetchAndSearch(query string) ([]Package, error) {
	index, err := LoadIndex()
	if err != nil {
		return nil, err
	}
	return Search(index, query, 50), nil
}
