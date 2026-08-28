package a2a

import (
	"errors"
	"fmt"
	"strings"
)

// HubMode controls how external Agents may enroll.
type HubMode string

const (
	HubModeClosed HubMode = "closed"
	HubModeOpen   HubMode = "open"
	HubModePublic HubMode = "public"
)

const (
	MaxHubDisplayNameBytes = 128
	MaxHubProviderBytes    = 128
	MaxHubTransportBytes   = 128
	MaxHubIdempotencyBytes = 256
	MaxHubAgentCardBytes   = 256 * 1024
	MaxHubCapabilities     = 64
)

// ParseHubMode converts operator configuration to a mode. Empty configuration
// uses public mode; operators can select closed or open explicitly.
func ParseHubMode(value string) (HubMode, error) {
	switch HubMode(strings.ToLower(strings.TrimSpace(value))) {
	case "":
		return HubModePublic, nil
	case HubModeClosed:
		return HubModeClosed, nil
	case HubModeOpen:
		return HubModeOpen, nil
	case HubModePublic:
		return HubModePublic, nil
	default:
		return "", fmt.Errorf("unknown Hub mode %q", value)
	}
}

// HubPolicy is the in-memory policy used by registration and routing edges.
// BootstrapToken is intentionally not part of this public metadata type.
type HubPolicy struct {
	Mode                HubMode
	HubID               string
	RegistrationEnabled bool
	PublicConfirmed     bool
	RegistrationTTL     int64
	PeerLeaseSeconds    int64
	MaxRegisteredAgents int32
	MaxTasksPerMinute   int32
	MaxConcurrentTasks  int32
	MaxPayloadBytes     int64
}

func DefaultHubPolicy() HubPolicy {
	return HubPolicy{
		Mode:                HubModePublic,
		HubID:               "local-public",
		PublicConfirmed:     true,
		RegistrationEnabled: true,
		RegistrationTTL:     24 * 60 * 60,
		PeerLeaseSeconds:    90,
		MaxRegisteredAgents: 100,
		MaxTasksPerMinute:   60,
		MaxConcurrentTasks:  4,
		MaxPayloadBytes:     MaxBridgeInputBytes,
	}
}

func (p HubPolicy) Validate() error {
	if _, err := ParseHubMode(string(p.Mode)); err != nil {
		return err
	}
	if strings.TrimSpace(p.HubID) == "" || len(p.HubID) > MaxHubIdempotencyBytes || strings.ContainsAny(p.HubID, "\r\n") {
		return errors.New("Hub ID is invalid")
	}
	if p.Mode == HubModePublic && !p.PublicConfirmed {
		return errors.New("public Hub mode requires explicit confirmation")
	}
	if p.RegistrationTTL <= 0 || p.PeerLeaseSeconds <= 0 || p.MaxRegisteredAgents <= 0 || p.MaxTasksPerMinute <= 0 || p.MaxConcurrentTasks <= 0 || p.MaxPayloadBytes <= 0 || p.MaxPayloadBytes > MaxBridgeInputBytes {
		return errors.New("Hub policy limits must be positive and bounded")
	}
	return nil
}

// AgentDeclaration is the untrusted registration payload before validation.
type AgentDeclaration struct {
	DisplayName                string
	ProviderFamily             string
	TransportID                string
	Capabilities               []string
	AgentCardJSON              string
	RegistrationIdempotencyKey string
}

func (d AgentDeclaration) Validate() error {
	for name, value := range map[string]string{
		"display_name": d.DisplayName, "provider_family": d.ProviderFamily,
		"transport_id": d.TransportID, "registration_idempotency_key": d.RegistrationIdempotencyKey,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s contains a line break", name)
		}
	}
	if len([]byte(d.DisplayName)) > MaxHubDisplayNameBytes || len([]byte(d.ProviderFamily)) > MaxHubProviderBytes || len([]byte(d.TransportID)) > MaxHubTransportBytes || len([]byte(d.RegistrationIdempotencyKey)) > MaxHubIdempotencyBytes {
		return errors.New("Hub Agent declaration field is too large")
	}
	if len([]byte(d.AgentCardJSON)) > MaxHubAgentCardBytes {
		return errors.New("Agent Card is too large")
	}
	if len(d.Capabilities) > MaxHubCapabilities {
		return errors.New("too many Agent capabilities")
	}
	for _, capability := range d.Capabilities {
		if strings.TrimSpace(capability) == "" || len([]byte(capability)) > MaxHubTransportBytes || strings.ContainsAny(capability, "\r\n") {
			return errors.New("Agent capability is invalid")
		}
	}
	return nil
}
