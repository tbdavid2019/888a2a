package testkit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/coder/acp-go-sdk"
	"github.com/pkg/errors"

	a2atestkit "github.com/Ranxy/laelia/backend/a2a/testkit"
	"github.com/Ranxy/laelia/backend/agent/acp2"
)

func TestTwelveAgentDeterministicTopology(t *testing.T) {
	const agentCount = 12
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := make([]string, agentCount)
	kinds := make([]string, agentCount)
	errs := make([]error, agentCount)
	var wg sync.WaitGroup
	for index := range agentCount {
		wg.Go(func() {
			agentID := fmt.Sprintf("agent-%02d", index+1)
			prompt := fmt.Sprintf("work-%02d", index+1)
			switch {
			case index < 4:
				kinds[index] = "acp-v1"
				results[index], errs[index] = runACPv1(ctx, agentID, prompt)
			case index < 8:
				kinds[index] = "acp-v2"
				results[index], errs[index] = runACPv2(ctx, agentID, prompt)
			default:
				kinds[index] = "a2a-http-json"
				results[index], errs[index] = runA2A(ctx, agentID, prompt)
			}
		})
	}
	wg.Wait()

	kindCount := map[string]int{}
	for index := range agentCount {
		if errs[index] != nil {
			t.Fatalf("agent %d (%s): %v", index+1, kinds[index], errs[index])
		}
		want := fmt.Sprintf("agent-%02d: work-%02d", index+1, index+1)
		if results[index] != want {
			t.Errorf("result[%d] = %q, want %q", index, results[index], want)
		}
		kindCount[kinds[index]]++
	}
	for _, kind := range []string{"acp-v1", "acp-v2", "a2a-http-json"} {
		if kindCount[kind] != 4 {
			t.Errorf("%s agent count = %d, want 4", kind, kindCount[kind])
		}
	}
}

func runACPv1(ctx context.Context, agentID, prompt string) (string, error) {
	fake := NewFakeACPv1(agentID)
	session, err := fake.NewSession(ctx, acp.NewSessionRequest{Cwd: "/workspace"})
	if err != nil {
		return "", err
	}
	response, err := fake.Prompt(ctx, acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		return "", err
	}
	text, ok := response.Meta["response"].(string)
	if !ok {
		return "", errors.New("ACP v1 response metadata has no text")
	}
	return text, nil
}

func runACPv2(ctx context.Context, agentID, prompt string) (string, error) {
	want := agentID + ": " + prompt
	client, _, closePair := NewClient(WithTurn(agentID, prompt, want))
	defer closePair()
	if _, err := client.Initialize(ctx, agentID, "test"); err != nil {
		return "", err
	}
	if err := client.Initialized(); err != nil {
		return "", err
	}
	thread, err := client.StartThread(ctx, acp2.ThreadStartParams{Cwd: "/workspace"})
	if err != nil {
		return "", err
	}
	turn, err := client.StartTurn(ctx, thread.ID, prompt)
	if err != nil {
		return "", err
	}
	return AwaitTurn(ctx, client, turn.ResolvedID())
}

func runA2A(ctx context.Context, agentID, prompt string) (string, error) {
	peer, err := a2atestkit.NewPeer(ctx, agentID)
	if err != nil {
		return "", err
	}
	defer peer.Close()
	result, err := peer.Client.SendMessage(ctx, &a2a.SendMessageRequest{
		Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(prompt)),
	})
	if err != nil {
		return "", err
	}
	message, ok := result.(*a2a.Message)
	if !ok || len(message.Parts) == 0 {
		return "", errors.Errorf("A2A result type = %T", result)
	}
	return message.Parts[0].Text(), nil
}
