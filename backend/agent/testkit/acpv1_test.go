package testkit

import (
	"context"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestFakeACPv1ImplementsAgent(_ *testing.T) {
	var _ acp.Agent = (*FakeACPv1)(nil)
}

func TestFakeACPv1SessionIDsAndPromptAreDeterministic(t *testing.T) {
	ctx := context.Background()
	fake := NewFakeACPv1("agent-a")

	init, err := fake.Initialize(ctx, acp.InitializeRequest{})
	require.NoError(t, err)
	require.Equal(t, acp.ProtocolVersion(acp.ProtocolVersionNumber), init.ProtocolVersion)

	first, err := fake.NewSession(ctx, acp.NewSessionRequest{Cwd: "/tmp", McpServers: []acp.McpServer{}})
	require.NoError(t, err)
	second, err := fake.NewSession(ctx, acp.NewSessionRequest{Cwd: "/tmp", McpServers: []acp.McpServer{}})
	require.NoError(t, err)
	require.Equal(t, acp.SessionId("agent-a-session-1"), first.SessionId)
	require.Equal(t, acp.SessionId("agent-a-session-2"), second.SessionId)

	resp, err := fake.Prompt(ctx, acp.PromptRequest{
		SessionId: first.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	})
	require.NoError(t, err)
	require.Equal(t, acp.StopReasonEndTurn, resp.StopReason)
	require.Equal(t, "agent-a: hello", resp.Meta["response"])
	require.Equal(t, []PromptCall{{SessionID: first.SessionId, Prompt: "hello", Response: "agent-a: hello"}}, fake.PromptCalls())
}

func TestFakeACPv1LifecycleMethodsAreSafeNoOps(t *testing.T) {
	ctx := context.Background()
	fake := NewFakeACPv1("agent-a")

	_, err := fake.ResumeSession(ctx, acp.ResumeSessionRequest{})
	require.NoError(t, err)
	_, err = fake.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{})
	require.NoError(t, err)
	_, err = fake.SetSessionMode(ctx, acp.SetSessionModeRequest{})
	require.NoError(t, err)
	require.NoError(t, fake.Cancel(ctx, acp.CancelNotification{}))
}
