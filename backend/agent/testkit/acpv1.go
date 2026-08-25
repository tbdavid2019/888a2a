// Package testkit contains deterministic in-process protocol test doubles.
package testkit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
)

// PromptCall is the stable, text-only record of a prompt sent to a fake agent.
type PromptCall struct {
	SessionID acp.SessionId
	Prompt    string
	Response  string
}

// FakeACPv1 is a deterministic, in-process ACP v1 Agent.
//
// It has no authentication, filesystem, network, or process behavior. When a
// connection is attached, Prompt emits the response as one session update.
type FakeACPv1 struct {
	AgentID string

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions uint64
	calls    []PromptCall
}

var _ acp.Agent = (*FakeACPv1)(nil)

// NewFakeACPv1 creates a fake whose response and session IDs include agentID.
func NewFakeACPv1(agentID string) *FakeACPv1 { return &FakeACPv1{AgentID: agentID} }

// SetAgentConnection attaches the client connection used for session updates.
func (f *FakeACPv1) SetAgentConnection(conn *acp.AgentSideConnection) {
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
}

// PromptCalls returns a snapshot of all prompts received so far.
func (f *FakeACPv1) PromptCalls() []PromptCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]PromptCall, len(f.calls))
	copy(calls, f.calls)
	return calls
}

// ResponseFor returns the deterministic response for prompt.
func (f *FakeACPv1) ResponseFor(prompt string) string {
	return fmt.Sprintf("%s: %s", f.AgentID, prompt)
}

func (*FakeACPv1) Initialize(_ context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber}, nil
}

func (f *FakeACPv1) NewSession(ctx context.Context, _ acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if err := ctx.Err(); err != nil {
		return acp.NewSessionResponse{}, err
	}
	f.mu.Lock()
	f.sessions++
	id := fmt.Sprintf("%s-session-%d", f.AgentID, f.sessions)
	if f.AgentID == "" {
		id = fmt.Sprintf("session-%d", f.sessions)
	}
	f.mu.Unlock()
	return acp.NewSessionResponse{SessionId: acp.SessionId(id)}, nil
}

func (f *FakeACPv1) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	if err := ctx.Err(); err != nil {
		return acp.PromptResponse{}, err
	}
	var text strings.Builder
	for _, block := range params.Prompt {
		if block.Text != nil {
			text.WriteString(block.Text.Text)
		}
	}
	prompt := text.String()
	response := f.ResponseFor(prompt)
	f.mu.Lock()
	f.calls = append(f.calls, PromptCall{SessionID: params.SessionId, Prompt: prompt, Response: response})
	conn := f.conn
	f.mu.Unlock()
	if conn != nil {
		if err := conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update:    acp.UpdateAgentMessageText(response),
		}); err != nil {
			return acp.PromptResponse{}, err
		}
	}
	return acp.PromptResponse{
		Meta:       map[string]any{"response": response},
		StopReason: acp.StopReasonEndTurn,
	}, nil
}

func (*FakeACPv1) ResumeSession(_ context.Context, _ acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (*FakeACPv1) SetSessionConfigOption(_ context.Context, _ acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{ConfigOptions: []acp.SessionConfigOption{}}, nil
}

func (*FakeACPv1) SetSessionMode(_ context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (*FakeACPv1) Cancel(_ context.Context, _ acp.CancelNotification) error { return nil }

func (*FakeACPv1) Authenticate(_ context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (*FakeACPv1) Logout(_ context.Context, _ acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (*FakeACPv1) CloseSession(_ context.Context, _ acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*FakeACPv1) ListSessions(_ context.Context, _ acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{Sessions: []acp.SessionInfo{}}, nil
}
