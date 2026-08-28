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
	RegistrationEnabled bool
	PublicConfirmed     bool
	RegistrationTTL     time.Duration
	PeerLease           time.Duration
	MaxRegisteredAgents int32
	MaxTasksPerMinute   int32
	MaxConcurrentTasks  int32
	MaxPayloadBytes     int64
}

// LoadHubConfig reads only A2A888-prefixed settings. Closed is the fail-safe
// default; public mode requires an explicit confirmation flag.
func LoadHubConfig() (HubConfig, error) {
	mode, err := parseConfiguredHubMode(ReadEnv("A2A888_HUB_MODE"))
	if err != nil {
		return HubConfig{}, err
	}
	confirmed := strings.EqualFold(strings.TrimSpace(ReadEnv("A2A888_HUB_PUBLIC_CONFIRM")), "true")
	cfg := HubConfig{
		Mode:                mode,
		HubID:               strings.TrimSpace(ReadEnv("A2A888_HUB_ID")),
		BootstrapToken:      strings.TrimSpace(ReadEnv("A2A888_HUB_BOOTSTRAP_TOKEN")),
		RegistrationEnabled: mode != HubModeClosed,
		PublicConfirmed:     confirmed,
		RegistrationTTL:     24 * time.Hour,
		PeerLease:           90 * time.Second,
		MaxRegisteredAgents: 100,
		MaxTasksPerMinute:   60,
		MaxConcurrentTasks:  4,
		MaxPayloadBytes:     1 << 20,
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
	case "", string(HubModeClosed):
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
