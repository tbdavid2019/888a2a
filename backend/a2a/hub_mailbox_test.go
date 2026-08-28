package a2a

import (
	"context"
	"testing"
)

func TestMemoryHubMailboxIsIdempotentAndReplaysUnacknowledgedItems(t *testing.T) {
	mailbox := NewMemoryHubMailbox()
	item := HubInboxItem{HubID: "hub", TargetAgentID: "agt-b", RequesterAgentID: "agt-a", TaskID: "task", ContextID: "ctx", IdempotencyKey: "idem", Message: "hello"}
	first, err := mailbox.Enqueue(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mailbox.Enqueue(context.Background(), item)
	if err != nil || !second.Duplicate || second.Item.Sequence != first.Item.Sequence {
		t.Fatalf("duplicate enqueue = %+v, err=%v", second, err)
	}
	items, err := mailbox.Poll(context.Background(), "hub", "agt-b", 0, 10)
	if err != nil || len(items) != 1 || items[0].Message != "hello" {
		t.Fatalf("poll = %+v, err=%v", items, err)
	}
	if err := mailbox.Acknowledge(context.Background(), "hub", "agt-b", first.Item.Sequence); err != nil {
		t.Fatal(err)
	}
	items, err = mailbox.Poll(context.Background(), "hub", "agt-b", 0, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("acknowledged poll = %+v, err=%v", items, err)
	}
}
