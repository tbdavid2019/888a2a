package a2a

import (
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// SkillInput defines metadata for a skill provided to the projection.
type SkillInput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
	Private     bool     `json:"private,omitempty"`
}

// ProjectAgentCardOptions specifies inputs for projecting an Agent Card.
type ProjectAgentCardOptions struct {
	Agent    *store.AgentMessage
	Manifest *a2a888.ProviderManifest
	Skills   []SkillInput
	BaseURL  string
	Tenant   string
}

// ProjectAgentCard builds a standard A2A 1.0 AgentCard from 888a2a agent metadata,
// provider capabilities, and runtime status. Disabled and private skills are omitted.
func ProjectAgentCard(opts ProjectAgentCardOptions) (*a2a.AgentCard, error) {
	if opts.Agent == nil {
		return nil, errors.New("agent is required")
	}

	tenant := strings.TrimSpace(opts.Tenant)
	if tenant == "" {
		tenant = "default"
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8181"
	}

	name := strings.TrimSpace(opts.Agent.Name)
	if name == "" {
		name = opts.Agent.ResourceID
	}

	interfaceURL := fmt.Sprintf("%s/a2a/v1/%s/agents/%s", baseURL, tenant, opts.Agent.ResourceID)

	var streaming bool
	if opts.Manifest != nil && opts.Manifest.Capabilities != nil {
		streaming = opts.Manifest.Capabilities.Streaming
	} else if opts.Agent.Info != nil && opts.Agent.Info.Capability != nil {
		streaming = opts.Agent.Info.Capability.SupportsRawEvents
	}

	card := &a2a.AgentCard{
		Name:        name,
		Description: opts.Agent.Description,
		Version:     ProtocolVersion1_0,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(interfaceURL, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming:         streaming,
			ExtendedAgentCard: true,
		},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Provider: &a2a.AgentProvider{
			Org: "888a2a",
			URL: baseURL,
		},
	}

	var visibleSkills []a2a.AgentSkill
	for _, s := range opts.Skills {
		// Omit disabled or private skills
		if s.Disabled || s.Private {
			continue
		}
		if s.ID == "" {
			continue
		}
		skillName := s.Name
		if skillName == "" {
			skillName = s.ID
		}
		inModes := s.InputModes
		if len(inModes) == 0 {
			inModes = card.DefaultInputModes
		}
		outModes := s.OutputModes
		if len(outModes) == 0 {
			outModes = card.DefaultOutputModes
		}

		visibleSkills = append(visibleSkills, a2a.AgentSkill{
			ID:          s.ID,
			Name:        skillName,
			Description: s.Description,
			InputModes:  inModes,
			OutputModes: outModes,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}
	card.Skills = visibleSkills

	return card, nil
}
