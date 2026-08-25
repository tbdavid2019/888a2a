package client

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ranxy/laelia/backend/agent/executor"
)

func TestConfinePathToAgentWorkspace(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("A2A888_HOME", tempHome)

	machineID := "mach-1"
	agentID := "agent-1"

	baseDir := executor.AgentWorkingDir(machineID, agentID)
	require.NoError(t, os.MkdirAll(baseDir, 0o700))

	// 1. Valid nested path
	target, err := ConfinePathToAgentWorkspace(machineID, agentID, "src/main.go")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(baseDir, "src/main.go"), target)

	// 2. Traversal attempt
	_, err = ConfinePathToAgentWorkspace(machineID, agentID, "../agent-2/workspace/secret.key")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathEscape)

	// 3. Absolute path to peer agent workspace
	peerDir := executor.AgentWorkingDir(machineID, "agent-2")
	_, err = ConfinePathToAgentWorkspace(machineID, agentID, peerDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathEscape)

	// 4. Symlink escape attempt
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "host_secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("supersecret"), 0o600))

	symlinkPath := filepath.Join(baseDir, "link_to_outside")
	require.NoError(t, os.Symlink(outsideFile, symlinkPath))

	_, err = ConfinePathToAgentWorkspace(machineID, agentID, "link_to_outside")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathEscape)
}

func TestBuildIsolatedEnvironment(t *testing.T) {
	env := BuildIsolatedEnvironment("agent-1", "/tmp/daemon.sock", "token-agent-1", "/tmp/bin", []string{
		"CUSTOM_KEY=custom_value",
		"A2A888_AGENT=tampered-agent", // must be ignored
	})

	assert.Contains(t, env, "A2A888_AGENT=agent-1")
	assert.Contains(t, env, "A2A888_DAEMON_SOCKET=/tmp/daemon.sock")
	assert.Contains(t, env, "A2A888_SESSION_TOKEN=token-agent-1")
	assert.Contains(t, env, "CUSTOM_KEY=custom_value")
	assert.NotContains(t, env, "A2A888_AGENT=tampered-agent")
}

func TestAssertAgentOwnership(t *testing.T) {
	assert.NoError(t, AssertAgentOwnership("agents/agent-1", "agents/agent-1"))
	assert.NoError(t, AssertAgentOwnership("agent-1", "agents/agent-1"))
	assert.ErrorIs(t, AssertAgentOwnership("agent-1", "agent-2"), ErrCrossAgentAccess)
}

func TestTwelveFakeAgentsIsolation(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("A2A888_HOME", tempHome)

	const agentCount = 12
	machineID := "machine-shared"

	var wg sync.WaitGroup
	errs := make([]error, agentCount)

	for i := 0; i < agentCount; i++ {
		agentIndex := i + 1
		agentID := fmt.Sprintf("fake-agent-%02d", agentIndex)
		wg.Go(func() {
			// Initialize workspace
			ws, err := ConfinePathToAgentWorkspace(machineID, agentID, "data.txt")
			if err != nil {
				errs[agentIndex-1] = err
				return
			}

			// Write private agent data
			secretPayload := fmt.Sprintf("secret-for-agent-%02d", agentIndex)
			if err := os.WriteFile(ws, []byte(secretPayload), 0o600); err != nil {
				errs[agentIndex-1] = err
				return
			}

			// Read back self data
			content, err := os.ReadFile(ws)
			if err != nil || string(content) != secretPayload {
				errs[agentIndex-1] = fmt.Errorf("self read mismatch: %v", err)
				return
			}

			// Attempt cross-agent read against every other agent
			for peerIndex := 1; peerIndex <= agentCount; peerIndex++ {
				if peerIndex == agentIndex {
					continue
				}
				peerID := fmt.Sprintf("fake-agent-%02d", peerIndex)

				// Verify ownership assertion rejects peer access
				if err := AssertAgentOwnership(agentID, peerID); err == nil {
					errs[agentIndex-1] = fmt.Errorf("ownership assertion permitted cross access to %s", peerID)
					return
				}

				// Verify path confinement prevents constructing peer path
				peerTargetPath := filepath.Join("..", peerID, "workspace", "data.txt")
				if _, err := ConfinePathToAgentWorkspace(machineID, agentID, peerTargetPath); err == nil {
					errs[agentIndex-1] = fmt.Errorf("path confinement allowed traversal to %s", peerID)
					return
				}
			}
		})
	}

	wg.Wait()

	for idx, err := range errs {
		if err != nil {
			t.Fatalf("agent %d failed isolation check: %v", idx+1, err)
		}
	}
}
