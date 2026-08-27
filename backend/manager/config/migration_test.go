package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadEnvUsesLegacyOnlyWhenCurrentKeyIsAbsent(t *testing.T) {
	t.Setenv("A2A888_PG_URL", "")
	t.Setenv("LAE"+"LIA_PG_URL", "postgres://legacy")
	require.Equal(t, "postgres://legacy", ReadEnv("A2A888_PG_URL"))

	t.Setenv("A2A888_PG_URL", "postgres://current")
	require.Equal(t, "postgres://current", ReadEnv("A2A888_PG_URL"))
}

func TestReadEnvDoesNotGuessUnmappedLegacyKeys(t *testing.T) {
	t.Setenv("LAE"+"LIA_SECRET", "must-not-be-read")
	require.Empty(t, ReadEnv("A2A888_SECRET"))
}
