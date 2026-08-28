package provider

import (
	"testing"
)

func TestCatalogContainsRequestedProviderFamilies(t *testing.T) {
	want := []string{
		"openclaw", "hermes", "claude-code", "codex", "antigravity", "deepseek-harness", "workbuddy",
		"qwen-office", "dumate", "traework", "cline", "zeroclaw", "qwen-code", "kiro", "github-copilot",
		"openhands", "aider", "opencode", "goose", "gemini", "cursor", "grok", "pi", "reasonix",
	}
	entries := Catalog()
	if len(entries) != len(want) {
		t.Fatalf("catalog entries = %d, want %d", len(entries), len(want))
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.ID] = struct{}{}
		if entry.DisplayName == "" || len(entry.Transports) == 0 || entry.InstallHint == "" {
			t.Fatalf("catalog entry is incomplete: %+v", entry)
		}
		for _, transport := range entry.Transports {
			if transport.ID == "" || transport.Mode == "" || len(transport.Operations) == 0 {
				t.Fatalf("catalog transport is incomplete: %+v", transport)
			}
		}
	}
	for _, id := range want {
		if _, ok := seen[id]; !ok {
			t.Errorf("catalog is missing provider %q", id)
		}
	}
}

func TestCatalogAliasesNormalizeToCanonicalFamily(t *testing.T) {
	cases := map[string]string{
		"OpenClaw":        "openclaw",
		"openclaw-cli":    "openclaw",
		"agy":             "antigravity",
		"antigravity-cli": "antigravity",
		"github copilot":  "github-copilot",
		"qwen code":       "qwen-code",
	}
	for alias, want := range cases {
		if got := NormalizeCatalogID(alias); got != want {
			t.Errorf("NormalizeCatalogID(%q) = %q, want %q", alias, got, want)
		}
	}
}

func TestCatalogDefaultsAreConservative(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.Readiness == ReadinessReady && entry.ID != "opencode" && entry.ID != "claude-code" && entry.ID != "codex" && entry.ID != "pi" {
			t.Errorf("unverified provider %q must not default to READY", entry.ID)
		}
		for _, transport := range entry.Transports {
			if transport.AutoEnabled && entry.Readiness != ReadinessReady {
				t.Errorf("non-ready provider %q has auto-enabled transport %+v", entry.ID, transport)
			}
		}
	}
}
