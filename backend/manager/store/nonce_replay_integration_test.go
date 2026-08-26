package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConsumeNonceIsSharedAndIdempotent(t *testing.T) {
	stores := requireIntegrationStore(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Minute)

	first, err := stores.ConsumeNonce(ctx, "agents/shared", "nonce-shared", expiresAt)
	require.NoError(t, err)
	require.True(t, first)

	second, err := stores.ConsumeNonce(ctx, "agents/shared", "nonce-shared", expiresAt)
	require.NoError(t, err)
	require.False(t, second)
}
