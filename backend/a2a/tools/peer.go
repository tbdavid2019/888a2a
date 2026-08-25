package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/a2a"
)

// PeerListInput defines input parameters for listing peers.
type PeerListInput struct {
	Tenant     string `json:"tenant,omitempty"`
	SkillTag   string `json:"skillTag,omitempty"`
	SkillName  string `json:"skillName,omitempty"`
	InputMode  string `json:"inputMode,omitempty"`
	OutputMode string `json:"outputMode,omitempty"`
	ReadyOnly  bool   `json:"readyOnly,omitempty"`
}

// SkillSummary provides human and machine readable skill details.
type SkillSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

// CapabilitiesSummary provides summary of Agent Card capabilities.
type CapabilitiesSummary struct {
	Streaming         bool `json:"streaming"`
	ExtendedAgentCard bool `json:"extendedAgentCard"`
}

// PeerSummary describes an agent peer with its Agent Card and verified readiness.
type PeerSummary struct {
	AgentResourceID string               `json:"agentResourceId"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Tenant          string               `json:"tenant"`
	Readiness       a2a.RuntimeReadiness `json:"readiness"`
	Enabled         bool                 `json:"enabled"`
	Version         string               `json:"version"`
	Capabilities    CapabilitiesSummary  `json:"capabilities"`
	Skills          []SkillSummary       `json:"skills,omitempty"`
	Interfaces      []string             `json:"interfaces,omitempty"`
}

// PeerListOutput contains the results of a peer listing.
type PeerListOutput struct {
	Peers      []PeerSummary `json:"peers"`
	TotalCount int           `json:"totalCount"`
}

// PeerGetInput defines input parameters for fetching a specific peer.
type PeerGetInput struct {
	Tenant          string `json:"tenant,omitempty"`
	AgentResourceID string `json:"agentResourceId"`
}

// PeerGetOutput contains the detailed peer projection.
type PeerGetOutput struct {
	Peer PeerSummary `json:"peer"`
}

// PeerList queries the directory for peer agents matching the filter.
func PeerList(ctx context.Context, ds *a2a.DirectoryService, caller a2a.CallerPrincipal, in PeerListInput) (*PeerListOutput, error) {
	if ds == nil {
		return nil, errors.New("directory service is required")
	}

	filter := a2a.PeerFilter{
		SkillTag:   in.SkillTag,
		SkillName:  in.SkillName,
		InputMode:  in.InputMode,
		OutputMode: in.OutputMode,
		ReadyOnly:  in.ReadyOnly,
	}

	projections, err := ds.ListPeers(ctx, caller, in.Tenant, filter)
	if err != nil {
		return nil, errors.Wrap(err, "list peers from directory")
	}

	peers := make([]PeerSummary, 0, len(projections))
	for _, p := range projections {
		peers = append(peers, projectToPeerSummary(p))
	}

	return &PeerListOutput{
		Peers:      peers,
		TotalCount: len(peers),
	}, nil
}

// PeerGet retrieves an Agent Card and verified readiness for a specific peer.
func PeerGet(ctx context.Context, ds *a2a.DirectoryService, caller a2a.CallerPrincipal, in PeerGetInput) (*PeerGetOutput, error) {
	if ds == nil {
		return nil, errors.New("directory service is required")
	}
	if in.AgentResourceID == "" {
		return nil, errors.New("agentResourceId is required")
	}

	proj, err := ds.GetPeer(ctx, caller, in.Tenant, in.AgentResourceID)
	if err != nil {
		return nil, errors.Wrap(err, "get peer from directory")
	}

	return &PeerGetOutput{
		Peer: projectToPeerSummary(proj),
	}, nil
}

func projectToPeerSummary(p *a2a.PeerProjection) PeerSummary {
	if p == nil {
		return PeerSummary{}
	}

	summary := PeerSummary{
		AgentResourceID: p.AgentResourceID,
		Tenant:          p.Tenant,
		Readiness:       p.Readiness,
		Enabled:         p.Enabled,
	}

	if p.Card != nil {
		summary.Name = p.Card.Name
		summary.Description = p.Card.Description
		summary.Version = p.Card.Version
		summary.Capabilities = CapabilitiesSummary{
			Streaming:         p.Card.Capabilities.Streaming,
			ExtendedAgentCard: p.Card.Capabilities.ExtendedAgentCard,
		}

		for _, iface := range p.Card.SupportedInterfaces {
			if iface != nil {
				summary.Interfaces = append(summary.Interfaces, fmt.Sprintf("%s (%s)", iface.URL, iface.ProtocolBinding))
			}
		}

		for _, s := range p.Card.Skills {
			summary.Skills = append(summary.Skills, SkillSummary{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				Tags:        s.Tags,
				InputModes:  s.InputModes,
				OutputModes: s.OutputModes,
				Examples:    s.Examples,
			})
		}
	}

	return summary
}

// FormatPeerList formats a PeerListOutput into human-readable text.
func FormatPeerList(out *PeerListOutput) string {
	if out == nil || len(out.Peers) == 0 {
		return "No peer agents discovered.\n"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Discovered %d peer agent(s):\n", out.TotalCount)
	for _, p := range out.Peers {
		sb.WriteString(formatPeerSummaryLine(p))
	}
	return sb.String()
}

// FormatPeerGet formats a PeerGetOutput into human-readable text.
func FormatPeerGet(out *PeerGetOutput) string {
	if out == nil {
		return "Peer agent not found.\n"
	}
	return formatPeerSummaryDetailed(out.Peer)
}

func formatPeerSummaryLine(p PeerSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- **%s** (`%s`) [Readiness: %s]\n", p.Name, p.AgentResourceID, p.Readiness)
	if p.Description != "" {
		fmt.Fprintf(&sb, "  Description: %s\n", p.Description)
	}
	fmt.Fprintf(&sb, "  Capabilities: Streaming=%t, ExtendedCard=%t\n", p.Capabilities.Streaming, p.Capabilities.ExtendedAgentCard)
	if len(p.Skills) > 0 {
		var skillNames []string
		for _, s := range p.Skills {
			name := s.Name
			if len(s.Tags) > 0 {
				name += fmt.Sprintf(" [%s]", strings.Join(s.Tags, ", "))
			}
			skillNames = append(skillNames, name)
		}
		fmt.Fprintf(&sb, "  Skills (%d): %s\n", len(p.Skills), strings.Join(skillNames, "; "))
	}
	return sb.String()
}

func formatPeerSummaryDetailed(p PeerSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### Agent Card: %s (`%s`)\n", p.Name, p.AgentResourceID)
	fmt.Fprintf(&sb, "- **Tenant**: `%s`\n", p.Tenant)
	fmt.Fprintf(&sb, "- **Readiness**: `%s` (Enabled: %t)\n", p.Readiness, p.Enabled)
	fmt.Fprintf(&sb, "- **Protocol Version**: `%s`\n", p.Version)
	fmt.Fprintf(&sb, "- **Capabilities**: Streaming=%t, ExtendedAgentCard=%t\n", p.Capabilities.Streaming, p.Capabilities.ExtendedAgentCard)
	if p.Description != "" {
		fmt.Fprintf(&sb, "- **Description**: %s\n", p.Description)
	}
	if len(p.Interfaces) > 0 {
		sb.WriteString("- **Interfaces**:\n")
		for _, iface := range p.Interfaces {
			fmt.Fprintf(&sb, "  - %s\n", iface)
		}
	}
	if len(p.Skills) > 0 {
		fmt.Fprintf(&sb, "- **Skills** (%d):\n", len(p.Skills))
		for _, s := range p.Skills {
			fmt.Fprintf(&sb, "  - **%s** (`%s`): %s\n", s.Name, s.ID, s.Description)
			if len(s.Tags) > 0 {
				fmt.Fprintf(&sb, "    Tags: %s\n", strings.Join(s.Tags, ", "))
			}
			if len(s.InputModes) > 0 {
				fmt.Fprintf(&sb, "    Input Modes: %s\n", strings.Join(s.InputModes, ", "))
			}
			if len(s.OutputModes) > 0 {
				fmt.Fprintf(&sb, "    Output Modes: %s\n", strings.Join(s.OutputModes, ", "))
			}
		}
	}
	return sb.String()
}
