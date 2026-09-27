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

package codexrouter

import (
	"strings"
	"testing"

	"github.com/cloud-exit/exitbox/internal/plugin"
)

func TestDockerfilePinsVersion(t *testing.T) {
	df := New().Dockerfile(plugin.BuildInput{Agent: "codex", Version: "v0.6.0"})
	if !strings.Contains(df, "--branch v0.6.0") {
		t.Errorf("version not pinned: %s", df)
	}
	if !strings.Contains(df, "npm ci --omit=dev") {
		t.Errorf("dependencies not installed at build time: %s", df)
	}
	if !strings.Contains(df, "ENV CODEX_ROUTER_SOURCE_ROOT="+SourceRoot) {
		t.Errorf("source root not exported: %s", df)
	}
}

func TestDockerfileFallsBackToDefaultVersion(t *testing.T) {
	df := New().Dockerfile(plugin.BuildInput{Agent: "codex"})
	if !strings.Contains(df, "--branch "+DefaultVersion) {
		t.Errorf("default ref missing: %s", df)
	}
}

func TestDockerfileRejectsUnsafeRef(t *testing.T) {
	df := New().Dockerfile(plugin.BuildInput{Agent: "codex", Version: "main; rm -rf /"})
	if strings.Contains(df, "rm -rf /") {
		t.Errorf("unsafe ref reached the Dockerfile: %s", df)
	}
	if !strings.Contains(df, "--branch "+DefaultVersion) {
		t.Errorf("unsafe ref should fall back to the default: %s", df)
	}
}

func TestSettingDomains(t *testing.T) {
	domains := New().SettingDomains(map[string]string{"providers": "anthropic-api,deepseek"})
	joined := strings.Join(domains, ",")
	if !strings.Contains(joined, "anthropic.com") || !strings.Contains(joined, "deepseek.com") {
		t.Errorf("SettingDomains() = %v", domains)
	}
	if len(New().SettingDomains(map[string]string{})) != 0 {
		t.Error("no providers selected means no extra domains")
	}
}

func TestProviderDomainsIgnoresUnknown(t *testing.T) {
	if got := ProviderDomains([]string{"not-a-provider"}); len(got) != 0 {
		t.Errorf("ProviderDomains() = %v, want none", got)
	}
}

func TestTarget(t *testing.T) {
	for _, agent := range []string{"codex", "claude", "cursor"} {
		if Target(agent) != agent {
			t.Errorf("Target(%q) = %q", agent, Target(agent))
		}
	}
	if Target("qwen") != "" {
		t.Errorf("Target(qwen) = %q, want empty", Target("qwen"))
	}
}

func TestKeySettingKey(t *testing.T) {
	if got := KeySettingKey("anthropic-api"); got != "anthropic_api_key" {
		t.Errorf("KeySettingKey() = %q", got)
	}
	if got := KeySettingKey("deepseek"); got != "deepseek_key" {
		t.Errorf("KeySettingKey() = %q", got)
	}
}

func TestSecretSettingsCarryVaultKeySuggestions(t *testing.T) {
	p := New()
	s := p.Setting(KeySettingKey("anthropic-api"))
	if s == nil {
		t.Fatal("anthropic key setting missing")
	}
	if s.Type != plugin.TypeSecret {
		t.Errorf("provider key setting type = %q, want secret", s.Type)
	}
	if s.VaultKey != "CODEX_ROUTER_ANTHROPIC_API_KEY" {
		t.Errorf("VaultKey = %q", s.VaultKey)
	}
	if s.DependsValue != "anthropic-api" {
		t.Errorf("DependsValue = %q", s.DependsValue)
	}
}
