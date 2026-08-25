package a2a

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMetadata_StripsCredentialsAndHiddenReasoning(t *testing.T) {
	raw := map[string]string{
		"agent_id":             "specialist-1",
		"auth_token":           "secret_token_12345",
		"api_key":              "sk-abcdefg1234567890",
		"bearer_credential":    "Bearer my-jwt-token-string",
		"thought":              "I should try to bypass the safety check...",
		"thinking_process":     "Step 1: formulate hidden plan...",
		"hidden_reasoning":     "private internal reasoning here",
		"prompt":               "internal system prompt containing sensitive instruction",
		"regular_action":       "delegate_task",
		"authorization_header": "Bearer secret_header_value",
		"custom_field":         "safe text with API_KEY=abc embedded",
	}

	sanitized := SanitizeMetadata(raw)

	// Safe keys remain
	assert.Equal(t, "specialist-1", sanitized["agent_id"])
	assert.Equal(t, "delegate_task", sanitized["regular_action"])

	// Sensitive keys are removed
	assert.NotContains(t, sanitized, "auth_token")
	assert.NotContains(t, sanitized, "api_key")
	assert.NotContains(t, sanitized, "bearer_credential")
	assert.NotContains(t, sanitized, "authorization_header")

	// Hidden reasoning keys are removed
	assert.NotContains(t, sanitized, "thought")
	assert.NotContains(t, sanitized, "thinking_process")
	assert.NotContains(t, sanitized, "hidden_reasoning")
	assert.NotContains(t, sanitized, "prompt")

	// Embedded secrets in allowed keys are redacted
	assert.Contains(t, sanitized["custom_field"], "API_KEY=[REDACTED]")
}

func TestTraceRecorder_RecordWithoutStore(t *testing.T) {
	recorder := NewTraceRecorder(nil, nil)

	event := &TraceEvent{
		TenantID:       "tenant-1",
		WorkID:         "work-1",
		EventType:      TraceEventDiscovery,
		PolicyDecision: "ALLOWED",
		Metadata: map[string]string{
			"agent_id": "coordinator",
			"secret":   "leaked-password",
		},
	}

	recorded, err := recorder.Record(context.Background(), event)
	require.NoError(t, err)
	assert.NotEmpty(t, recorded.EventID)
	assert.NotContains(t, recorded.Metadata, "secret")
	assert.Equal(t, "coordinator", recorded.Metadata["agent_id"])
}
