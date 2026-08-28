package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type HubMode string

const (
	HubModeClosed HubMode = "closed"
	HubModeOpen   HubMode = "open"
	HubModePublic HubMode = "public"
)

type HubConfig struct {
	Mode                HubMode
	HubID               string
	BootstrapToken      string
	OperatorToken       string
	RegistrationEnabled bool
	PublicConfirmed     bool
	RegistrationTTL     time.Duration
	PeerLease           time.Duration
	MaxRegisteredAgents int32
	MaxTasksPerMinute   int32
	MaxConcurrentTasks  int32
	MaxPayloadBytes     int64
}

// LoadHubConfig reads only A2A888-prefixed settings. Public is the default
// when no mode is configured; closed or open can be selected explicitly.
func LoadHubConfig() (HubConfig, error) {
	mode, err := parseConfiguredHubMode(ReadEnv("A2A888_HUB_MODE"))
	if err != nil {
		return HubConfig{}, err
	}
	confirmed := strings.EqualFold(strings.TrimSpace(ReadEnv("A2A888_HUB_PUBLIC_CONFIRM")), "true")
	if strings.TrimSpace(ReadEnv("A2A888_HUB_MODE")) == "" && mode == HubModePublic {
		confirmed = true
	}
	registrationTTL, err := parseHubInt(ReadEnv("A2A888_HUB_REGISTRATION_TTL_SECONDS"), 24*60*60)
	if err != nil {
		return HubConfig{}, err
	}
	peerLease, err := parseHubInt(ReadEnv("A2A888_HUB_PEER_LEASE_SECONDS"), 90)
	if err != nil {
		return HubConfig{}, err
	}
	maxAgents, err := parseHubInt(ReadEnv("A2A888_HUB_MAX_REGISTERED_AGENTS"), 100)
	if err != nil {
		return HubConfig{}, err
	}
	maxTasks, err := parseHubInt(ReadEnv("A2A888_HUB_MAX_TASKS_PER_MINUTE"), 60)
	if err != nil {
		return HubConfig{}, err
	}
	maxConcurrent, err := parseHubInt(ReadEnv("A2A888_HUB_MAX_CONCURRENT_TASKS"), 4)
	if err != nil {
		return HubConfig{}, err
	}
	maxPayload, err := parseHubInt(ReadEnv("A2A888_HUB_MAX_PAYLOAD_BYTES"), 1<<20)
	if err != nil || maxPayload > 1<<20 {
		return HubConfig{}, errors.New("A2A888_HUB_MAX_PAYLOAD_BYTES must be between 1 and 1048576")
	}
	hubID := strings.TrimSpace(ReadEnv("A2A888_HUB_ID"))
	if hubID == "" && mode == HubModePublic {
		hubID = "public"
	}
	cfg := HubConfig{
		Mode:                mode,
		HubID:               hubID,
		BootstrapToken:      strings.TrimSpace(ReadEnv("A2A888_HUB_BOOTSTRAP_TOKEN")),
		OperatorToken:       strings.TrimSpace(ReadEnv("A2A888_HUB_OPERATOR_TOKEN")),
		RegistrationEnabled: mode != HubModeClosed,
		PublicConfirmed:     confirmed,
		RegistrationTTL:     time.Duration(registrationTTL) * time.Second,
		PeerLease:           time.Duration(peerLease) * time.Second,
		MaxRegisteredAgents: int32(maxAgents),
		MaxTasksPerMinute:   int32(maxTasks),
		MaxConcurrentTasks:  int32(maxConcurrent),
		MaxPayloadBytes:     maxPayload,
	}
	if mode == HubModeOpen && cfg.BootstrapToken == "" {
		return HubConfig{}, errors.New("A2A888_HUB_BOOTSTRAP_TOKEN is required in open mode")
	}
	if mode == HubModePublic && !confirmed {
		return HubConfig{}, errors.New("A2A888_HUB_PUBLIC_CONFIRM=true is required in public mode")
	}
	if mode != HubModeClosed && cfg.HubID == "" {
		return HubConfig{}, errors.New("A2A888_HUB_ID is required in open or public mode")
	}
	return cfg, nil
}

func parseConfiguredHubMode(value string) (HubMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return HubModePublic, nil
	case string(HubModeClosed):
		return HubModeClosed, nil
	case string(HubModeOpen):
		return HubModeOpen, nil
	case string(HubModePublic):
		return HubModePublic, nil
	default:
		return "", fmt.Errorf("unknown A2A888_HUB_MODE %q", value)
	}
}

func parseHubInt(value string, fallback int64) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid positive Hub limit %q", value)
	}
	return parsed, nil
}
