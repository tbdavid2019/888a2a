package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ranxy/laelia/backend/common"
)

// TestParseAgentToken_VerifiesSignature is the core T12 guard: RefreshAgentToken
// trusts claims only after ParseAgentToken verifies the HS256 signature. A
// token whose payload is tampered (here: re-signed with a different secret, or
// with a forged token_version under the wrong key) must be rejected — otherwise
// a refresh token could "upgrade" itself to the current token_version purely
// via a hash lookup.
func TestParseAgentToken_VerifiesSignature(t *testing.T) {
	const secret = "test-secret"
	const agentName = "agent-1"
	const resourceID = "agents/agent-1"
	const tokenVersion = 3

	tok, err := GenerateAgentTokenWithSession(agentName, resourceID, tokenVersion, TokenTypeRefresh, "sess-1", common.ReleaseModeDev, secret, time.Hour)
	require.NoError(t, err)

	claims, err := ParseAgentToken(tok, secret)
	require.NoError(t, err)
	assert.Equal(t, TokenTypeRefresh, claims.TokenType)
	assert.Equal(t, tokenVersion, claims.TokenVersion)
	assert.Equal(t, resourceID, claims.Subject)
	assert.Equal(t, "sess-1", claims.SessionID)
}

func TestParseAgentToken_RejectsWrongSecret(t *testing.T) {
	tok, err := GenerateAgentTokenWithSession("agent-1", "agents/agent-1", 3, TokenTypeRefresh, "", common.ReleaseModeDev, "real-secret", time.Hour)
	require.NoError(t, err)

	_, err = ParseAgentToken(tok, "different-secret")
	assert.Error(t, err, "token signed with a different secret must not verify")
}

func TestParseAgentToken_RejectsTampered(t *testing.T) {
	tok, err := GenerateAgentTokenWithSession("agent-1", "agents/agent-1", 3, TokenTypeRefresh, "", common.ReleaseModeDev, "secret", time.Hour)
	require.NoError(t, err)

	// Flip a character in the payload segment to break the signature. Changing
	// the final Base64URL character of a SHA-256 signature can leave the
	// decoded bytes unchanged because that character has unused trailing bits.
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	payload := []byte(parts[1])
	if payload[0] == 'A' {
		payload[0] = 'B'
	} else {
		payload[0] = 'A'
	}
	parts[1] = string(payload)
	tampered := strings.Join(parts, ".")
	_, err = ParseAgentToken(tampered, "secret")
	assert.Error(t, err, "a token whose signature no longer matches its payload must not verify")
}
