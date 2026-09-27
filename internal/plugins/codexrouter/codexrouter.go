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

// Package codexrouter describes the codex-router plugin: a local model
// router that lets Codex, Claude Code, and Cursor talk to third-party
// providers through one gateway running inside the container.
package codexrouter

import (
	"fmt"
	"strings"

	"github.com/cloud-exit/exitbox/internal/plugin"
)

// Name is the plugin's config key and CLI name.
const Name = "codex-router"

// SourceRoot is where the plugin's code is baked into the image. The
// router's state stays in the agent config directory, which is mounted
// per workspace, so provider credentials and catalogs survive rebuilds.
const SourceRoot = "/opt/exitbox/plugins/codex-router"

// Repo is the upstream repository.
const Repo = "https://github.com/duolahypercho/codex-router"

// DefaultVersion is the git ref cloned when no version is pinned.
const DefaultVersion = "main"

// providers are the router provider IDs offered in the configuration menu.
// They are the ids the upstream provider catalog uses, which is what
// `model-router <target> providers enable <id>` expects.
var providers = []string{
	"anthropic-api",
	"cerebras",
	"deepseek",
	"fireworks",
	"gemini-api",
	"github-copilot",
	"groq",
	"grok-api",
	"huggingface",
	"kimi-api",
	"minimax-token-plan",
	"mistral",
	"nvidia-nim",
	"ollama-cloud",
	"openrouter",
	"qwen-plan",
	"together",
	"zai-coding",
}

// providerLabels are the display names shown in the configuration menu.
var providerLabels = map[string]string{
	"anthropic-api":      "Anthropic",
	"cerebras":           "Cerebras",
	"deepseek":           "DeepSeek",
	"fireworks":          "Fireworks",
	"gemini-api":         "Gemini",
	"github-copilot":     "GitHub Copilot",
	"groq":               "Groq",
	"grok-api":           "Grok (xAI)",
	"huggingface":        "Hugging Face",
	"kimi-api":           "Kimi (Moonshot)",
	"minimax-token-plan": "MiniMax",
	"mistral":            "Mistral",
	"nvidia-nim":         "NVIDIA NIM",
	"ollama-cloud":       "Ollama Cloud",
	"openrouter":         "OpenRouter",
	"qwen-plan":          "Qwen",
	"together":           "Together",
	"zai-coding":         "Z.ai",
}

// providerDomains are the upstream API hosts each provider needs reachable
// through the firewall.
var providerDomains = map[string][]string{
	"anthropic-api":      {"anthropic.com"},
	"cerebras":           {"cerebras.ai"},
	"deepseek":           {"deepseek.com"},
	"fireworks":          {"fireworks.ai"},
	"gemini-api":         {"googleapis.com"},
	"github-copilot":     {"githubcopilot.com", "github.com"},
	"groq":               {"groq.com"},
	"grok-api":           {"x.ai"},
	"huggingface":        {"huggingface.co"},
	"kimi-api":           {"moonshot.ai"},
	"minimax-token-plan": {"minimaxi.com"},
	"mistral":            {"mistral.ai"},
	"nvidia-nim":         {"nvidia.com"},
	"ollama-cloud":       {"ollama.com"},
	"openrouter":         {"openrouter.ai"},
	"qwen-plan":          {"aliyuncs.com"},
	"together":           {"together.xyz"},
	"zai-coding":         {"z.ai", "bigmodel.cn"},
}

// oauthProviders authenticate through a browser or another CLI's session,
// so they get no API key field in the configuration menu.
var oauthProviders = map[string]bool{
	"github-copilot": true,
}

// KeySettingKey returns the settings key holding a provider's API key
// reference. The entrypoint derives the same name from the provider id.
func KeySettingKey(providerID string) string {
	return strings.ReplaceAll(providerID, "-", "_") + "_key"
}

// New returns the plugin descriptor.
func New() *plugin.Plugin {
	return &plugin.Plugin{
		Name:        Name,
		Display:     "Codex Router",
		Description: "Local model router: use Anthropic, DeepSeek, Kimi, Grok and other providers from Codex, Claude Code and Cursor",
		Agents:      []string{"codex", "claude", "cursor"},
		Repo:        Repo,
		Version:     DefaultVersion,
		Domains:     []string{"github.com", "npmjs.org"},
		Settings:    settings(),
		Dockerfile:  dockerfile,
		SettingDomains: func(settings map[string]string) []string {
			return ProviderDomains(plugin.SplitMulti(settings["providers"]))
		},
	}
}

// ProviderDomains returns the firewall domains required by the selected
// providers, on top of the plugin's own domains.
func ProviderDomains(selected []string) []string {
	var out []string
	for _, p := range selected {
		out = append(out, providerDomains[p]...)
	}
	return out
}

// Providers returns the provider IDs offered by the configuration menu.
func Providers() []string { return append([]string(nil), providers...) }

// Target maps an ExitBox agent to the codex-router target it configures.
func Target(agent string) string {
	switch agent {
	case "codex", "claude", "cursor":
		return agent
	default:
		return ""
	}
}

func settings() []plugin.Setting {
	out := []plugin.Setting{
		{
			Key:     "providers",
			Label:   "Providers",
			Help:    "Model providers the router exposes to the agent",
			Type:    plugin.TypeMulti,
			Options: providers,
		},
	}
	// One vault-backed API key per provider, shown only while that provider
	// is selected.
	for _, id := range providers {
		if oauthProviders[id] {
			continue
		}
		label := providerLabel(id)
		out = append(out, plugin.Setting{
			Key:          KeySettingKey(id),
			Label:        label + " API key",
			Help:         "Vault key holding the " + label + " API key",
			Type:         plugin.TypeSecret,
			VaultKey:     "CODEX_ROUTER_" + strings.ToUpper(strings.ReplaceAll(id, "-", "_")) + "_KEY",
			DependsOn:    "providers",
			DependsValue: id,
		})
	}
	return append(out,
		plugin.Setting{
			Key:     "port",
			Label:   "Router port",
			Help:    "Local port the router listens on inside the container",
			Type:    plugin.TypeInt,
			Default: "4202",
		},
		plugin.Setting{
			Key:     "auto_start",
			Label:   "Start with the container",
			Help:    "Start the router service when the container starts",
			Type:    plugin.TypeBool,
			Default: "true",
		},
	)
}

// providerLabel returns the display name of a provider id.
func providerLabel(id string) string {
	if label, ok := providerLabels[id]; ok {
		return label
	}
	return id
}

// dockerfile installs codex-router into the image. The clone is pinned to
// the configured ref and its production dependencies are installed at build
// time so container start does no network work.
func dockerfile(in plugin.BuildInput) string {
	ref := in.Version
	if ref == "" {
		ref = DefaultVersion
	}
	return fmt.Sprintf(`# codex-router (%s)
%s
RUN git clone --depth 1 --branch %s %s %s && \
    cd %s && \
    npm ci --omit=dev --no-audit --no-fund && \
    ln -sf %s/bin/model-router /usr/local/bin/model-router && \
    chown -R user:user %s
ENV CODEX_ROUTER_SOURCE_ROOT=%s
`, Repo, in.InstallStep("nodejs", "npm", "git"), shellArg(ref), Repo, SourceRoot, SourceRoot, SourceRoot, SourceRoot, SourceRoot)
}

// shellArg rejects refs that could break out of the git clone command.
// Versions come from config, so a bad value is a configuration error, not
// an attack, but the Dockerfile must stay well-formed either way.
func shellArg(ref string) string {
	if strings.ContainsAny(ref, " \t\n\"'`$&|;<>()\\") {
		return DefaultVersion
	}
	return ref
}
