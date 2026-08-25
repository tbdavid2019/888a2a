package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	storepb "github.com/Ranxy/laelia/backend/generated-go/store"
)

func TestProviderAvailable_Gating(t *testing.T) {
	available := []*storepb.AgentProviderInfo{
		{
			ProviderId:         "opencode",
			DisplayName:        "OpenCode",
			RuntimeStatus:      "READY",
			CompatibilityLevel: "FULL_LOOP_VERIFIED",
		},
		{
			ProviderId:         "claude-code",
			DisplayName:        "Claude Code",
			RuntimeStatus:      "QUARANTINED",
			CompatibilityLevel: "DETECTED",
			FailureMessage:     "tampered binary",
		},
		{
			ProviderId:         "broken-provider",
			DisplayName:        "Broken Provider",
			RuntimeStatus:      "BROKEN",
			CompatibilityLevel: "DETECTED",
			FailureMessage:     "npm install failed",
		},
		{
			ProviderId:         "unverified-provider",
			DisplayName:        "Unverified Provider",
			RuntimeStatus:      "DETECTED",
			CompatibilityLevel: "DETECTED",
			FailureMessage:     "probe failed",
		},
		{
			ProviderId:         "detection-only-provider",
			DisplayName:        "Detection Only Provider",
			RuntimeStatus:      "DETECTED",
			CompatibilityLevel: "DETECTED",
		},
	}

	assert.True(t, providerAvailable("opencode", available), "READY provider should be available")
	assert.False(t, providerAvailable("claude-code", available), "QUARANTINED provider must not be available")
	assert.False(t, providerAvailable("broken-provider", available), "BROKEN provider must not be available")
	assert.False(t, providerAvailable("unverified-provider", available), "unverified DETECTED provider must not be available")
	assert.False(t, providerAvailable("detection-only-provider", available), "detection-only provider must not be available")
	assert.False(t, providerAvailable("legacy-without-status", append(available, &storepb.AgentProviderInfo{ProviderId: "legacy-without-status"})), "provider without verification status must not be available")
	assert.False(t, providerAvailable("non-existent", available), "missing provider must not be available")
}
