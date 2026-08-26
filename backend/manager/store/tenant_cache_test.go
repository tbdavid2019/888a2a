package store

import (
	"testing"

	lru "github.com/hashicorp/golang-lru/v2"
)

func TestAgentAndMachineCachesAreTenantScoped(t *testing.T) {
	agentCache, err := lru.New[string, *AgentMessage](8)
	if err != nil {
		t.Fatal(err)
	}
	agentResourceCache, err := lru.New[string, *AgentMessage](8)
	if err != nil {
		t.Fatal(err)
	}
	machineCache, err := lru.New[string, *MachineMessage](8)
	if err != nil {
		t.Fatal(err)
	}
	machineResourceCache, err := lru.New[string, *MachineMessage](8)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{
		agentIDCache:           agentCache,
		agentResourceIDCache:   agentResourceCache,
		machineIDCache:         machineCache,
		machineResourceIDCache: machineResourceCache,
	}
	agentA := &AgentMessage{ID: 7, ResourceID: "agent-7", OrganizationID: "org-a"}
	agentB := &AgentMessage{ID: 7, ResourceID: "agent-7", OrganizationID: "org-b"}
	s.cacheAgent(agentA)
	s.cacheAgent(agentB)
	if got, ok := agentCache.Get(TenantCacheKey("org-a", "agent", "7")); !ok || got != agentA {
		t.Fatal("tenant A agent cache entry was not retained")
	}
	if got, ok := agentCache.Get(TenantCacheKey("org-b", "agent", "7")); !ok || got != agentB {
		t.Fatal("tenant B agent cache entry was not retained")
	}
	machineA := &MachineMessage{ID: 3, ResourceID: "machine-3", OrganizationID: "org-a"}
	machineB := &MachineMessage{ID: 3, ResourceID: "machine-3", OrganizationID: "org-b"}
	s.cacheMachine(machineA)
	s.cacheMachine(machineB)
	if got, ok := machineCache.Get(TenantCacheKey("org-a", "machine", "3")); !ok || got != machineA {
		t.Fatal("tenant A machine cache entry was not retained")
	}
	if got, ok := machineCache.Get(TenantCacheKey("org-b", "machine", "3")); !ok || got != machineB {
		t.Fatal("tenant B machine cache entry was not retained")
	}
}
