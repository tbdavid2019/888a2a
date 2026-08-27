package tenantresource

import (
	"errors"
	"fmt"
	"testing"
)

var errTenantResourceNotFound = errors.New("tenant resource not found")

type tenantResource struct {
	organizationID string
	workspaceID    string
	kind           string
	id             string
}

type tenantResourcePrototype struct {
	resources map[string]tenantResource
}

func newTenantResourcePrototype(resources ...tenantResource) tenantResourcePrototype {
	byName := make(map[string]tenantResource, len(resources))
	for _, resource := range resources {
		byName[resourceName(resource.organizationID, resource.workspaceID, resource.kind, resource.id)] = resource
	}
	return tenantResourcePrototype{resources: byName}
}

func (p tenantResourcePrototype) resolve(organizationID, workspaceID, kind, id string) (tenantResource, error) {
	resource, ok := p.resources[resourceName(organizationID, workspaceID, kind, id)]
	if !ok {
		return tenantResource{}, errTenantResourceNotFound
	}
	return resource, nil
}

func resourceName(organizationID, workspaceID, kind, id string) string {
	return fmt.Sprintf("organizations/%s/workspaces/%s/%s/%s", organizationID, workspaceID, kind, id)
}

func TestTenantResourceIsolationPrototype(t *testing.T) {
	prototype := newTenantResourcePrototype(
		tenantResource{organizationID: "org-alpha", workspaceID: "workspace-alpha", kind: "agents", id: "agent-shared"},
		tenantResource{organizationID: "org-bravo", workspaceID: "workspace-bravo", kind: "agents", id: "agent-shared"},
		tenantResource{organizationID: "org-alpha", workspaceID: "workspace-alpha", kind: "conversations", id: "conversation-shared"},
		tenantResource{organizationID: "org-bravo", workspaceID: "workspace-bravo", kind: "conversations", id: "conversation-shared"},
	)

	alphaAgent, err := prototype.resolve("org-alpha", "workspace-alpha", "agents", "agent-shared")
	if err != nil || alphaAgent.organizationID != "org-alpha" {
		t.Fatalf("Alpha should resolve its own resource: resource=%+v err=%v", alphaAgent, err)
	}
	bravoAgent, err := prototype.resolve("org-bravo", "workspace-bravo", "agents", "agent-shared")
	if err != nil || bravoAgent.organizationID != "org-bravo" {
		t.Fatalf("Bravo should resolve its own resource: resource=%+v err=%v", bravoAgent, err)
	}

	cases := []struct {
		name           string
		organizationID string
		workspaceID    string
		kind           string
		id             string
	}{
		{"Alpha cannot resolve Bravo agent", "org-alpha", "workspace-alpha", "agents", "agent-bravo-only"},
		{"Bravo cannot resolve Alpha conversation", "org-bravo", "workspace-bravo", "conversations", "conversation-alpha-only"},
		{"Unknown ID has the same denial", "org-alpha", "workspace-alpha", "agents", "does-not-exist"},
		{"Workspace mismatch is denied", "org-alpha", "workspace-bravo", "agents", "agent-shared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := prototype.resolve(tc.organizationID, tc.workspaceID, tc.kind, tc.id)
			if !errors.Is(err, errTenantResourceNotFound) {
				t.Fatalf("expected indistinguishable not-found error, got %v", err)
			}
		})
	}

	if got := resourceName("org-alpha", "workspace-alpha", "agents", "agent-shared"); got != "organizations/org-alpha/workspaces/workspace-alpha/agents/agent-shared" {
		t.Fatalf("unexpected hierarchical resource name: %s", got)
	}
}
