package a2a

import (
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicy_DefaultDenyOnHighRiskOperations(t *testing.T) {
	workspace := t.TempDir()
	policy := DefaultRuntimePolicy("agent-1", []string{workspace})

	// 1. Unapproved Shell -> DENIED
	res := policy.Evaluate(PermissionRequest{
		ActionKind: ActionShell,
		Command:    "rm -rf /",
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "shell execution denied")

	// 2. Unapproved Filesystem Write -> DENIED
	res = policy.Evaluate(PermissionRequest{
		ActionKind: ActionWrite,
		TargetPath: filepath.Join(workspace, "test.txt"),
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "filesystem write denied")

	// 3. Network Egress -> DENIED
	res = policy.Evaluate(PermissionRequest{
		ActionKind: ActionNetwork,
		ToolName:   "http_client",
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "network access denied")

	// 4. Secret Access -> DENIED
	res = policy.Evaluate(PermissionRequest{
		ActionKind: ActionSecret,
		ToolName:   "get_api_key",
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "secret and credential access denied")

	// 5. Unapproved side-effecting MCP -> DENIED
	res = policy.Evaluate(PermissionRequest{
		ActionKind: ActionMCP,
		MCPServer:  "filesystem",
		MCPTool:    "delete_file",
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "side-effecting MCP action denied")

	// 6. Unclassified action -> DENIED
	res = policy.Evaluate(PermissionRequest{
		ActionKind: ActionUnclassified,
		ToolName:   "unknown_tool",
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "unclassified high-risk action denied")
}

func TestPolicy_WorkspaceReadAllowedWithinConfinement(t *testing.T) {
	workspace := t.TempDir()
	targetFile := filepath.Join(workspace, "data.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	policy := DefaultRuntimePolicy("agent-1", []string{workspace})

	// Reading inside workspace -> ALLOWED
	res := policy.Evaluate(PermissionRequest{
		ActionKind: ActionRead,
		TargetPath: targetFile,
	})
	assert.Equal(t, DecisionAllow, res.Decision)
	assert.Contains(t, res.Reason, "read permitted within workspace root")
}

func TestPolicy_RejectsSymlinkEscapeAndCrossAgentAccess(t *testing.T) {
	agent1Workspace := t.TempDir()
	agent2Workspace := t.TempDir()

	agent2File := filepath.Join(agent2Workspace, "secret.key")
	require.NoError(t, os.WriteFile(agent2File, []byte("peer_secret"), 0o600))

	// Agent 1 tries to read Agent 2's workspace directly
	policy1 := DefaultRuntimePolicy("agent-1", []string{agent1Workspace})
	res := policy1.Evaluate(PermissionRequest{
		ActionKind: ActionRead,
		TargetPath: agent2File,
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "outside allowed workspace roots")

	// Agent 1 has a symlink inside its workspace pointing to Agent 2
	symlinkPath := filepath.Join(agent1Workspace, "symlink_to_agent2")
	require.NoError(t, os.Symlink(agent2Workspace, symlinkPath))

	res = policy1.Evaluate(PermissionRequest{
		ActionKind: ActionRead,
		TargetPath: filepath.Join(symlinkPath, "secret.key"),
	})
	assert.Equal(t, DecisionDeny, res.Decision)
	assert.Contains(t, res.Reason, "outside allowed workspace roots")

	// Dangling symlink pointing outside root
	danglingSymlink := filepath.Join(agent1Workspace, "dangling")
	require.NoError(t, os.Symlink("/nonexistent/evil", danglingSymlink))
	res = policy1.Evaluate(PermissionRequest{
		ActionKind: ActionRead,
		TargetPath: danglingSymlink,
	})
	assert.Equal(t, DecisionDeny, res.Decision)
}

func TestPolicy_EvaluateACPPermission(t *testing.T) {
	workspace := t.TempDir()
	insideFile := filepath.Join(workspace, "inside.txt")
	require.NoError(t, os.WriteFile(insideFile, []byte("ok"), 0o644))

	policy := DefaultRuntimePolicy("agent-1", []string{workspace})

	// Allowed Read: should pick allow option
	readKind := acp.ToolKindRead
	readTitle := "Read file"
	optID, decision, _ := policy.EvaluateACPPermission(acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject", Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "allow", Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow"},
		},
		ToolCall: acp.ToolCallUpdate{
			Kind:     &readKind,
			Title:    &readTitle,
			RawInput: map[string]any{"path": insideFile},
		},
	})
	assert.Equal(t, DecisionAllow, decision)
	assert.Equal(t, acp.PermissionOptionId("allow"), optID)

	// Denied Shell: should pick reject option
	execKind := acp.ToolKindExecute
	execTitle := "Bash command"
	optID, decision, _ = policy.EvaluateACPPermission(acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject", Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "allow", Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow"},
		},
		ToolCall: acp.ToolCallUpdate{
			Kind:     &execKind,
			Title:    &execTitle,
			RawInput: map[string]any{"command": "curl evil.com"},
		},
	})
	assert.Equal(t, DecisionDeny, decision)
	assert.Equal(t, acp.PermissionOptionId("reject"), optID)
}

