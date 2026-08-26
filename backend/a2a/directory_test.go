package a2a

import (
	"context"
	"testing"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type fakeCaller struct {
	id            string
	tenant        string
	authenticated bool
}

func (f *fakeCaller) GetPrincipalID() string { return f.id }
func (f *fakeCaller) GetTenantID() string    { return f.tenant }
func (f *fakeCaller) IsAuthenticated() bool  { return f.authenticated }

type fakeDirectoryStore struct {
	agents []*store.AgentMessage
}

func (f *fakeDirectoryStore) ListAgents(_ context.Context, _ *store.FindAgentMessage) ([]*store.AgentMessage, error) {
	return f.agents, nil
}

func (f *fakeDirectoryStore) GetAgentByResourceID(_ context.Context, resourceID string) (*store.AgentMessage, error) {
	for _, a := range f.agents {
		if a.ResourceID == resourceID {
			return a, nil
		}
	}
	return nil, nil
}

func TestDirectoryService_ListPeers_FilteringAndReadiness(t *testing.T) {
	ctx := context.Background()

	agents := []*store.AgentMessage{
		{
			ID:          1,
			ResourceID:  "agent-review",
			Name:        "Review Agent",
			Description: "Code review specialist",
			Enabled:     true,
			Status: &models.AgentStatus{
				State: models.AgentStatus_ONLINE,
			},
		},
		{
			ID:          2,
			ResourceID:  "agent-offline",
			Name:        "Offline Agent",
			Description: "Agent currently offline",
			Enabled:     true,
			Status: &models.AgentStatus{
				State: models.AgentStatus_OFFLINE,
			},
		},
		{
			ID:          3,
			ResourceID:  "agent-stopped",
			Name:        "Stopped Agent",
			Description: "Disabled agent",
			Enabled:     false,
		},
		{
			ID:          4,
			ResourceID:  "agent-deleted",
			Name:        "Deleted Agent",
			Description: "Soft deleted agent",
			Enabled:     true,
			Deleted:     true,
		},
	}

	skills := map[string][]SkillInput{
		"agent-review": {
			{
				ID:          "skill-security",
				Name:        "Security Scan",
				Description: "Vulnerability analysis",
				Tags:        []string{"security", "scan"},
			},
		},
	}

	svc := NewDirectoryService(&fakeDirectoryStore{agents: agents}, "https://api.888a2a.local", skills)

	// Unauthenticated caller should fail
	_, err := svc.ListPeers(ctx, &fakeCaller{authenticated: false}, "tenant-1", PeerFilter{})
	if err != ErrUnauthenticatedCaller {
		t.Fatalf("expected ErrUnauthenticatedCaller, got %v", err)
	}

	// Authenticated caller should list accessible peers
	caller := &fakeCaller{id: "user-1", authenticated: true}
	peers, err := svc.ListPeers(ctx, caller, "tenant-1", PeerFilter{})
	if err != nil {
		t.Fatalf("ListPeers failed: %v", err)
	}

	// Deleted agent should be excluded (3 remaining: review, offline, stopped)
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(peers))
	}

	// Check readiness states
	readinessMap := make(map[string]RuntimeReadiness)
	for _, p := range peers {
		readinessMap[p.AgentResourceID] = p.Readiness
	}
	if readinessMap["agent-review"] != ReadinessReady {
		t.Errorf("expected agent-review to be READY, got %s", readinessMap["agent-review"])
	}
	if readinessMap["agent-offline"] != ReadinessOffline {
		t.Errorf("expected agent-offline to be OFFLINE, got %s", readinessMap["agent-offline"])
	}
	if readinessMap["agent-stopped"] != ReadinessUnavailable {
		t.Errorf("expected agent-stopped to be UNAVAILABLE, got %s", readinessMap["agent-stopped"])
	}

	// Test ReadyOnly filter
	readyPeers, err := svc.ListPeers(ctx, caller, "tenant-1", PeerFilter{ReadyOnly: true})
	if err != nil {
		t.Fatalf("ListPeers with ReadyOnly failed: %v", err)
	}
	if len(readyPeers) != 1 || readyPeers[0].AgentResourceID != "agent-review" {
		t.Fatalf("expected only agent-review with ReadyOnly, got %d peers", len(readyPeers))
	}

	// Test skill filtering
	skillPeers, err := svc.ListPeers(ctx, caller, "tenant-1", PeerFilter{SkillTag: "security"})
	if err != nil {
		t.Fatalf("ListPeers with SkillTag failed: %v", err)
	}
	if len(skillPeers) != 1 || skillPeers[0].AgentResourceID != "agent-review" {
		t.Fatalf("expected only agent-review matching skill tag 'security', got %d peers", len(skillPeers))
	}
}
