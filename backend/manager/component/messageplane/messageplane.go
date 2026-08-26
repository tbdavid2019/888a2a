// Package messageplane defines the internal boundary for realtime message
// delivery. Product services depend on this contract, not on a vendor's wire
// protocol or administration API.
package messageplane

import "context"

type ConnectionCredentials struct {
	OrganizationID string
	ConversationID string
	Token          string
}

type MessageInput struct {
	OrganizationID  string
	ConversationID  string
	ClientMessageNo string
	SenderID        string
	Payload         []byte
}

type Message struct {
	OrganizationID  string
	ConversationID  string
	MessageID       string
	ClientMessageNo string
	MessageSeq      uint64
	SenderID        string
	Payload         []byte
}

type Cursor struct {
	OrganizationID string
	ConversationID string
	MessageSeq     uint64
}

type HistoryRequest struct {
	OrganizationID string
	ConversationID string
	After          Cursor
	Limit          int
}

type HistoryResponse struct {
	Messages   []Message
	NextCursor Cursor
}

type MembershipProjection struct {
	OrganizationID string
	ConversationID string
	PrincipalID    string
	Role           string
}

type Health struct {
	Healthy bool
	Detail  string
}

// MessagePlane is the minimum internal contract required by collaboration
// services. All methods are tenant-scoped and must reject mismatched cursors or
// resources before touching a vendor engine.
type MessagePlane interface {
	IssueCredentials(context.Context, string, string) (ConnectionCredentials, error)
	Append(context.Context, MessageInput) (Message, error)
	History(context.Context, HistoryRequest) (HistoryResponse, error)
	ProjectMembership(context.Context, MembershipProjection) error
	Health(context.Context) (Health, error)
}
