package a2a

import (
	"testing"

	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func TestProjectAgentCard(t *testing.T) {
	agent := &store.AgentMessage{
		ID:          1,
		ResourceID:  "agent-review-1",
		Name:        "Code Review Specialist",
		Description: "Reviews code changes for security and style",
		Enabled:     true,
	}

	manifest := &a2a888.ProviderManifest{
		ProviderId: "opencode",
		Capabilities: &a2a888.ProviderCapabilities{
			Streaming: true,
		},
	}

	skills := []SkillInput{
		{
			ID:          "skill-review",
			Name:        "Code Review",
			Description: "Analyzes pull requests and patches",
			InputModes:  []string{"text/plain", "application/json"},
			OutputModes: []string{"text/plain"},
			Tags:        []string{"code", "review"},
			Disabled:    false,
			Private:     false,
		},
		{
			ID:          "skill-secret-audit",
			Name:        "Internal Secret Audit",
			Description: "Internal auditing only",
			Disabled:    false,
			Private:     true, // Should be omitted
		},
		{
			ID:          "skill-legacy-formatter",
			Name:        "Legacy Formatter",
			Description: "Old disabled formatter",
			Disabled:    true, // Should be omitted
			Private:     false,
		},
	}

	card, err := ProjectAgentCard(ProjectAgentCardOptions{
		Agent:    agent,
		Manifest: manifest,
		Skills:   skills,
		BaseURL:  "https://api.888a2a.local",
		Tenant:   "tenant-alpha",
	})
	if err != nil {
		t.Fatalf("ProjectAgentCard failed: %v", err)
	}

	if card.Name != "Code Review Specialist" {
		t.Errorf("expected name %q, got %q", "Code Review Specialist", card.Name)
	}
	if card.Description != agent.Description {
		t.Errorf("expected description %q, got %q", agent.Description, card.Description)
	}
	if !card.Capabilities.Streaming {
		t.Error("expected streaming capability to be true")
	}
	if len(card.SupportedInterfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(card.SupportedInterfaces))
	}
	expectedURL := "https://api.888a2a.local/a2a/v1/tenant-alpha/agents/agent-review-1"
	if card.SupportedInterfaces[0].URL != expectedURL {
		t.Errorf("expected interface URL %q, got %q", expectedURL, card.SupportedInterfaces[0].URL)
	}

	// Verify disabled/private skills are omitted
	if len(card.Skills) != 1 {
		t.Fatalf("expected exactly 1 visible skill, got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "skill-review" {
		t.Errorf("expected skill ID 'skill-review', got %q", card.Skills[0].ID)
	}
}
