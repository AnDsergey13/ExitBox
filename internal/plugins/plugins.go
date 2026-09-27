package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloud-exit/exitbox/internal/fsutil"
)

// Installed is a marketplace plugin cloned into the current project.
type Installed struct {
	Name string
	Dir  string
}

type marketplace struct {
	Name      string            `json:"name"`
	Interface marketplaceUI     `json:"interface"`
	Plugins   []marketplaceItem `json:"plugins"`
}

type marketplaceUI struct {
	DisplayName string `json:"displayName"`
}

type marketplaceItem struct {
	Name     string            `json:"name"`
	Source   marketplaceSource `json:"source"`
	Policy   marketplacePolicy `json:"policy"`
	Category string            `json:"category"`
}

type marketplaceSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type marketplacePolicy struct {
	Installation   string `json:"installation"`
	Authentication string `json:"authentication"`
}

// ProjectPluginsDir returns the plugin root for an agent in the current project.
func ProjectPluginsDir(projectDir, agentName string) (string, error) {
	switch agentName {
	case "codex":
		return filepath.Join(projectDir, "plugins"), nil
	default:
		return "", fmt.Errorf("agent %q does not support plugins", agentName)
	}
}

func projectMarketplaceFile(projectDir, agentName string) (string, error) {
	switch agentName {
	case "codex":
		return filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), nil
	default:
		return "", fmt.Errorf("agent %q does not support plugins", agentName)
	}
}

// List returns installed plugin repositories for an agent in the current project.
func List(projectDir, agentName string) ([]Installed, error) {
	root, err := ProjectPluginsDir(projectDir, agentName)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var result []Installed
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		result = append(result, Installed{
			Name: entry.Name(),
			Dir:  filepath.Join(root, entry.Name()),
		})
	}
	return result, nil
}

// Install imports a Codex marketplace repo into the current project by copying
// its referenced local plugins into ./plugins and merging marketplace entries
// into ./.agents/plugins/marketplace.json.
func Install(projectDir, agentName, source, name string) ([]Installed, error) {
	if _, err := ProjectPluginsDir(projectDir, agentName); err != nil {
		return nil, err
	}
	if name != "" {
		if err := validatePluginName(name); err != nil {
			return nil, err
		}
	}

	tmpDir, err := os.MkdirTemp("", "exitbox-plugin-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDir := filepath.Join(tmpDir, "repo")
	cmd := exec.Command("git", "clone", "--depth", "1", "--recurse-submodules", source, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("git clone failed: %s", msg)
		}
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	repoMarketplacePath, err := projectMarketplaceFile(cloneDir, agentName)
	if err != nil {
		return nil, err
	}
	repoMarketplace, err := loadMarketplace(repoMarketplacePath)
	if err != nil {
		return nil, err
	}
	if len(repoMarketplace.Plugins) == 0 {
		return nil, fmt.Errorf("no plugins found in marketplace %s", repoMarketplacePath)
	}

	projectPluginsDir, _ := ProjectPluginsDir(projectDir, agentName)
	if err := os.MkdirAll(projectPluginsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating plugin root: %w", err)
	}

	var installed []Installed
	var importedEntries []marketplaceItem
	for _, entry := range repoMarketplace.Plugins {
		if name != "" && entry.Name != name {
			continue
		}
		if entry.Source.Source != "local" {
			continue
		}
		if err := validatePluginName(entry.Name); err != nil {
			return nil, err
		}
		srcPath := filepath.Join(cloneDir, filepath.Clean(entry.Source.Path))
		destPath := filepath.Join(projectPluginsDir, entry.Name)
		if _, err := os.Stat(destPath); err == nil {
			return nil, fmt.Errorf("plugin %q already exists", entry.Name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if _, err := os.Stat(srcPath); err != nil {
			return nil, fmt.Errorf("marketplace entry %q points to missing path %s", entry.Name, entry.Source.Path)
		}
		if err := fsutil.CopyDirRecursive(srcPath, destPath); err != nil {
			return nil, fmt.Errorf("copying plugin %q: %w", entry.Name, err)
		}
		entry.Source.Path = "./plugins/" + entry.Name
		importedEntries = append(importedEntries, entry)
		installed = append(installed, Installed{Name: entry.Name, Dir: destPath})
	}

	if len(installed) == 0 {
		return nil, fmt.Errorf("no matching plugins found in marketplace")
	}

	projectMarketplacePath, _ := projectMarketplaceFile(projectDir, agentName)
	if err := mergeMarketplace(projectMarketplacePath, importedEntries); err != nil {
		return nil, err
	}

	return installed, nil
}

// Remove deletes a project plugin and removes its marketplace entry.
func Remove(projectDir, agentName, name string) error {
	root, err := ProjectPluginsDir(projectDir, agentName)
	if err != nil {
		return err
	}
	if err := validatePluginName(name); err != nil {
		return err
	}

	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q not found", name)
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}

	marketplacePath, err := projectMarketplaceFile(projectDir, agentName)
	if err != nil {
		return err
	}
	return removeMarketplaceEntry(marketplacePath, name)
}

func validatePluginName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("plugin name %q cannot contain path separators", name)
	}
	return nil
}

func mergeMarketplace(path string, newEntries []marketplaceItem) error {
	current, err := loadMarketplace(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if current.Name == "" {
		current.Name = "local-plugins"
	}
	if current.Interface.DisplayName == "" {
		current.Interface.DisplayName = "Local Plugins"
	}

	existing := make(map[string]bool, len(current.Plugins))
	for _, entry := range current.Plugins {
		existing[entry.Name] = true
	}
	for _, entry := range newEntries {
		if !existing[entry.Name] {
			current.Plugins = append(current.Plugins, entry)
			existing[entry.Name] = true
		}
	}
	return saveMarketplace(path, current)
}

func removeMarketplaceEntry(path, name string) error {
	current, err := loadMarketplace(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	filtered := current.Plugins[:0]
	for _, entry := range current.Plugins {
		if entry.Name != name {
			filtered = append(filtered, entry)
		}
	}
	current.Plugins = filtered
	return saveMarketplace(path, current)
}

func loadMarketplace(path string) (marketplace, error) {
	var m marketplace
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing marketplace file %s: %w", path, err)
	}
	return m, nil
}

func saveMarketplace(path string, m marketplace) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating marketplace dir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding marketplace file: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing marketplace file: %w", err)
	}
	return nil
}
