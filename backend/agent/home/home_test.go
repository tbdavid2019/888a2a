package home

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirDefaultsToDot888a2aUnderHome(t *testing.T) {
	t.Setenv(EnvDir, "")
	t.Setenv(LegacyEnvDir, "")
	t.Setenv("HOME", "/home/test-user")

	got := Dir()
	assert.Equal(t, filepath.Join("/home/test-user", ".888a2a"), got)
}

func TestDirUsesEnvOverride(t *testing.T) {
	t.Setenv(EnvDir, "/var/lib/888a2a")
	t.Setenv("HOME", "/home/test-user")

	assert.Equal(t, "/var/lib/888a2a", Dir())
}

func TestDirConvertsRelativeEnvToAbsolute(t *testing.T) {
	t.Setenv(EnvDir, "relative/laelia")
	t.Setenv("HOME", "/home/test-user")

	abs, err := filepath.Abs("relative/laelia")
	require.NoError(t, err)
	assert.Equal(t, abs, Dir())
}

func TestJoinUsesEnvRoot(t *testing.T) {
	t.Setenv(EnvDir, "/data/laelia")

	assert.Equal(t, filepath.Join("/data/laelia", "machine.json"), Join("machine.json"))
	assert.Equal(t, filepath.Join("/data/laelia", "m", "a", "state.json"), Join("m", "a", "state.json"))
}

func TestDirCopiesLegacyHomeToNewRoot(t *testing.T) {
	homeDir := t.TempDir()
	legacyDir := filepath.Join(homeDir, ".lae"+"lia")
	require.NoError(t, os.MkdirAll(filepath.Join(legacyDir, "agent"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "machine.json"), []byte("state"), 0o600))

	t.Setenv(EnvDir, "")
	t.Setenv(LegacyEnvDir, "")
	t.Setenv("HOME", homeDir)

	target := Dir()
	require.Equal(t, filepath.Join(homeDir, ".888a2a"), target)
	data, err := os.ReadFile(filepath.Join(target, "machine.json"))
	require.NoError(t, err)
	assert.Equal(t, "state", string(data))
}
