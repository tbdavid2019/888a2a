package testkit

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestPeerRoundTrip(t *testing.T) {
	peer, err := NewPeer(context.Background(), "reviewer-1")
	if err != nil {
		t.Fatalf("NewPeer: %v", err)
	}
	defer peer.Close()

	result, err := peer.Client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("inspect this patch")),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	message, ok := result.(*a2a.Message)
	if !ok {
		t.Fatalf("result type = %T, want *a2a.Message", result)
	}
	if got := message.Parts[0].Text(); got != "reviewer-1: inspect this patch" {
		t.Fatalf("response = %q", got)
	}
	if peer.Card.Name != "reviewer-1" {
		t.Fatalf("card name = %q", peer.Card.Name)
	}
	if got := peer.Card.SupportedInterfaces[0].ProtocolBinding; got != a2a.TransportProtocolHTTPJSON {
		t.Fatalf("protocol = %q", got)
	}
}

func TestPeerRejectsEmptyID(t *testing.T) {
	if _, err := NewPeer(context.Background(), ""); err == nil {
		t.Fatal("NewPeer accepted empty peer ID")
	}
}
