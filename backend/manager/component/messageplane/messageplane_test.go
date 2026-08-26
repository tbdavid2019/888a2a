package messageplane

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

type fakePlane struct {
	mu       sync.Mutex
	seq      map[string]uint64
	globalID uint64
	messages map[string][]Message
	byClient map[string]Message
}

func newFakePlane() *fakePlane {
	return &fakePlane{seq: map[string]uint64{}, messages: map[string][]Message{}, byClient: map[string]Message{}}
}

func (f *fakePlane) IssueCredentials(_ context.Context, org, conversation string) (ConnectionCredentials, error) {
	if org == "" || conversation == "" {
		return ConnectionCredentials{}, fmt.Errorf("tenant and conversation are required")
	}
	return ConnectionCredentials{OrganizationID: org, ConversationID: conversation, Token: org + ":" + conversation}, nil
}

func (f *fakePlane) Append(_ context.Context, input MessageInput) (Message, error) {
	if input.OrganizationID == "" || input.ConversationID == "" || input.ClientMessageNo == "" {
		return Message{}, fmt.Errorf("tenant, conversation, and client message identity are required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := input.OrganizationID + ":" + input.ConversationID + ":" + input.ClientMessageNo
	if existing, ok := f.byClient[key]; ok {
		return existing, nil
	}
	conversationKey := input.OrganizationID + ":" + input.ConversationID
	f.seq[conversationKey]++
	f.globalID++
	message := Message{
		OrganizationID: input.OrganizationID, ConversationID: input.ConversationID,
		MessageID: fmt.Sprintf("msg-%d", f.globalID), ClientMessageNo: input.ClientMessageNo,
		MessageSeq: f.seq[conversationKey], SenderID: input.SenderID, Payload: append([]byte(nil), input.Payload...),
	}
	f.messages[conversationKey] = append(f.messages[conversationKey], message)
	f.byClient[key] = message
	return message, nil
}

func (f *fakePlane) History(_ context.Context, request HistoryRequest) (HistoryResponse, error) {
	if request.OrganizationID == "" || request.ConversationID == "" || request.Limit <= 0 {
		return HistoryResponse{}, fmt.Errorf("invalid history request")
	}
	if request.After.OrganizationID != "" && request.After.OrganizationID != request.OrganizationID {
		return HistoryResponse{}, fmt.Errorf("cursor tenant mismatch")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := request.OrganizationID + ":" + request.ConversationID
	var result HistoryResponse
	for _, message := range f.messages[key] {
		if message.MessageSeq <= request.After.MessageSeq {
			continue
		}
		if len(result.Messages) == request.Limit {
			break
		}
		result.Messages = append(result.Messages, message)
	}
	if len(result.Messages) > 0 {
		last := result.Messages[len(result.Messages)-1]
		result.NextCursor = Cursor{OrganizationID: last.OrganizationID, ConversationID: last.ConversationID, MessageSeq: last.MessageSeq}
	}
	return result, nil
}

func (f *fakePlane) ProjectMembership(_ context.Context, projection MembershipProjection) error {
	if projection.OrganizationID == "" || projection.ConversationID == "" || projection.PrincipalID == "" {
		return fmt.Errorf("invalid membership projection")
	}
	return nil
}

func (*fakePlane) Health(context.Context) (Health, error) { return Health{Healthy: true}, nil }

func TestMessagePlaneContractDeduplicatesAndSequencesPerTenant(t *testing.T) {
	plane := newFakePlane()
	ctx := context.Background()
	input := MessageInput{OrganizationID: "org-a", ConversationID: "conv-1", ClientMessageNo: "client-1", SenderID: "user-1", Payload: []byte("hello")}
	first, err := plane.Append(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := plane.Append(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.MessageID != first.MessageID || duplicate.MessageSeq != first.MessageSeq {
		t.Fatalf("duplicate changed identity: first=%+v duplicate=%+v", first, duplicate)
	}
	second, err := plane.Append(ctx, MessageInput{OrganizationID: "org-a", ConversationID: "conv-1", ClientMessageNo: "client-2", SenderID: "user-1", Payload: []byte("world")})
	if err != nil {
		t.Fatal(err)
	}
	if second.MessageSeq != first.MessageSeq+1 {
		t.Fatalf("sequence = %d, want %d", second.MessageSeq, first.MessageSeq+1)
	}
	other, err := plane.Append(ctx, MessageInput{OrganizationID: "org-b", ConversationID: "conv-1", ClientMessageNo: "client-1", SenderID: "user-2", Payload: []byte("other")})
	if err != nil {
		t.Fatal(err)
	}
	if other.MessageSeq != 1 || other.MessageID == first.MessageID {
		t.Fatalf("tenant sequence or identity leaked: %+v", other)
	}
}

func TestMessagePlaneContractRejectsCrossTenantCursor(t *testing.T) {
	plane := newFakePlane()
	_, err := plane.History(context.Background(), HistoryRequest{
		OrganizationID: "org-b", ConversationID: "conv-1", Limit: 10,
		After: Cursor{OrganizationID: "org-a", ConversationID: "conv-1", MessageSeq: 1},
	})
	if err == nil {
		t.Fatal("expected cross-tenant cursor to be rejected")
	}
}
