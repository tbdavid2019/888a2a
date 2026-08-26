package testkit

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/tbdavid2019/888a2a/backend/agent/acp2"
)

func TestACPv2ClientPair(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, closePair := NewClient(WithTurn("agent-a", "hello", "fixed reply"))
	defer closePair()

	init, err := client.Initialize(ctx, "agent-a", "test")
	if err != nil || init.UserAgent == "" {
		t.Fatalf("initialize: result=%+v err=%v", init, err)
	}
	if err := client.Initialized(); err != nil {
		t.Fatalf("initialized: %v", err)
	}
	thread, err := client.StartThread(ctx, acp2.ThreadStartParams{Cwd: "/workspace"})
	if err != nil {
		t.Fatalf("thread/start: %v", err)
	}
	turn, err := client.StartTurn(ctx, thread.ID, "hello")
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	if turn.ResolvedID() == "" {
		t.Fatal("turn/start returned an empty id")
	}

	reply, err := AwaitTurn(ctx, client, turn.ResolvedID())
	if err != nil {
		t.Fatalf("AwaitTurn: %v", err)
	}
	if reply != "fixed reply" {
		t.Fatalf("unexpected deterministic reply: %q", reply)
	}
}

func TestACPv2BoundServerIsDeterministic(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	server := StartServer(serverConn, serverConn, WithTurn("agent-a", "same", "same reply"))
	client := acp2.NewClient(acp2.NewTransport(clientConn), clientConn, nil)
	client.Start()
	defer func() {
		client.Close()
		_ = clientConn.Close()
		server.Close()
	}()

	if _, err := client.Initialize(context.Background(), "agent-a", "test"); err != nil {
		t.Fatal(err)
	}
	thread, err := client.StartThread(context.Background(), acp2.ThreadStartParams{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.StartTurn(context.Background(), thread.ID, "same")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.StartTurn(context.Background(), thread.ID, "same")
	if err != nil {
		t.Fatal(err)
	}
	if first.ResolvedID() != second.ResolvedID() {
		t.Fatalf("turn id is not deterministic: %q != %q", first.ResolvedID(), second.ResolvedID())
	}
}
