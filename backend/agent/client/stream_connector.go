package client

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/Ranxy/laelia/backend/agent/executor"
	v1pb "github.com/Ranxy/laelia/backend/generated-go/v1"
)

const cmdPingInterval = 15 * time.Second

// Start runs one command-stream lifecycle (mainLoop) and returns its terminal
// error. It deliberately does NOT retry internally: a dead bidi stream must
// surface to the caller (Client.Run's heartbeat loop, the "death fuse") so the
// whole agent connection is torn down and reconnected rather than the agent
// going deaf while its heartbeat stays healthy. The caller owns reconnect
// backoff.
func (c *commandStream) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	return c.mainLoop(ctx)
}

// mainLoop owns the connection lifecycle: it opens the AgentChannel, sends
// AgentReady, starts the receive pump (messageRouter) and the drain loop
// (drainRunner), and keeps the link alive with pings until the stream dies or
// the context is cancelled.
func (c *commandStream) mainLoop(ctx context.Context) error {
	token := c.getToken()
	if token == "" {
		_ = c.backoff.Wait(ctx)
		return nil
	}

	stream := c.client.AgentChannel(ctx)
	stream.RequestHeader().Set("Authorization", "Bearer "+token)

	ready := &v1pb.AgentStreamMessage{
		Message: &v1pb.AgentStreamMessage_AgentReady{
			AgentReady: &v1pb.AgentReady{
				AgentName: c.agentName,
				SessionId: c.getSessID(),
			},
		},
	}
	if state, err := executor.LoadLocalState(c.machineID, c.agentID); err != nil {
		slog.Warn("failed to load local command state", "error", err)
	} else if state != nil {
		ready.GetAgentReady().LastCommandId = state.CommandID
		ready.GetAgentReady().LastAckSeq = state.LastSeqSent
		ready.GetAgentReady().LastEventSeq = state.LastEventSeqSent
	}
	if err := stream.Send(ready); err != nil {
		return err
	}

	// Reset any stale in-flight session bookkeeping from a previous connection.
	// The previous connection's receive pump and drain loop have exited
	// (doneCh close / drainCancel) before this point, so replacing the fields
	// is safe.
	c.resetCrossConnectionState()

	// serializedSender guards Send: connect-go's Send is not safe to call
	// concurrently, and the workspace reply goroutines send alongside the ping
	// ticker and the drain loop.
	sender := &serializedSender{stream: stream}
	router := newMessageRouter(c)

	pingTicker := time.NewTicker(cmdPingInterval)
	defer pingTicker.Stop()

	var pingSeq int64

	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	defer close(doneCh)

	// Receive pump: dispatches manager messages. BeginSessionResponse goes to
	// the drain loop; NewMessages kicks the drain loop; Cancel acts on the
	// in-flight session.
	go func() {
		for {
			msg, err := stream.Receive()
			if err != nil {
				if err != io.EOF {
					select {
					case errCh <- err:
					case <-doneCh:
					}
				}
				return
			}
			router.route(ctx, sender, msg, doneCh)
		}
	}()

	// Drain loop: the agent-first autonomous engine. On wake it opens sessions
	// (BeginSession) and runs each until the manager reports idle. Wakes that
	// arrive during a session are coalesced — the post-session BeginSession
	// picks up anything new via the server-side cursor comparison.
	drainCtx, drainCancel := context.WithCancel(ctx)
	defer drainCancel()
	go c.drainLoop(drainCtx, sender, doneCh)

	// Kick the drain loop once on connect so missed-offline messages are
	// discovered immediately (AgentReady already told the manager we're back).
	c.wake()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-doneCh:
			return nil
		case err := <-errCh:
			return err
		case <-pingTicker.C:
			pingSeq++
			ping := &v1pb.AgentStreamMessage{
				Message: &v1pb.AgentStreamMessage_Ping{
					Ping: &v1pb.Ping{
						Seq:    pingSeq,
						SentAt: time.Now().UnixMilli(),
					},
				},
			}
			if err := sender.Send(ping); err != nil {
				return err
			}
		}
	}
}
