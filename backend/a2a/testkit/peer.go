// Package testkit provides deterministic official A2A peers for local tests.
package testkit

import (
	"context"
	"fmt"
	"iter"
	"net/http/httptest"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/pkg/errors"
)

// Peer is a local A2A 1.0 HTTP+JSON test peer and its official SDK client.
type Peer struct {
	ID     string
	Card   *a2a.AgentCard
	Client *a2aclient.Client

	server *httptest.Server
}

// NewPeer starts a deterministic peer whose response is "<peerID>: <input>".
func NewPeer(ctx context.Context, peerID string) (*Peer, error) {
	if strings.TrimSpace(peerID) == "" {
		return nil, errors.New("peer ID is required")
	}

	// Official HTTP+JSON server/client composition:
	// https://github.com/a2aproject/a2a-go/blob/v2.4.0/a2asrv/example_test.go
	handler := a2asrv.NewHandler(&executor{peerID: peerID})
	server := httptest.NewServer(a2asrv.NewRESTHandler(handler))
	card := &a2a.AgentCard{
		Name:                peerID,
		Description:         "deterministic A2A test peer",
		Version:             "test",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(server.URL, a2a.TransportProtocolHTTPJSON)},
		Capabilities:        a2a.AgentCapabilities{},
		DefaultInputModes:   []string{"text/plain"},
		DefaultOutputModes:  []string{"text/plain"},
	}
	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		server.Close()
		return nil, errors.Wrap(err, "create A2A client")
	}
	return &Peer{ID: peerID, Card: card, Client: client, server: server}, nil
}

// Close stops the test peer and releases its HTTP resources.
func (p *Peer) Close() {
	if p == nil {
		return
	}
	if p.Client != nil {
		_ = p.Client.Destroy()
		p.Client = nil
	}
	if p.server != nil {
		p.server.Close()
		p.server = nil
	}
}

type executor struct {
	peerID string
}

func (e *executor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		input := ""
		if execCtx.Message != nil {
			var parts []string
			for _, part := range execCtx.Message.Parts {
				if text := part.Text(); text != "" {
					parts = append(parts, text)
				}
			}
			input = strings.Join(parts, " ")
		}
		response := a2a.NewMessageForTask(
			a2a.MessageRoleAgent,
			execCtx,
			a2a.NewTextPart(fmt.Sprintf("%s: %s", e.peerID, input)),
		)
		yield(response, nil)
	}
}

func (*executor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}
