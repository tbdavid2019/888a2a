package messageplane

import (
	"context"
	"errors"
	"strings"
)

type Capability string

const (
	CapabilityPresence        Capability = "presence"
	CapabilityTyping          Capability = "typing"
	CapabilityDeliveryReceipt Capability = "delivery_receipt"
	CapabilityReadReceipt     Capability = "read_receipt"
)

type CapabilityState string

const (
	CapabilitySupported   CapabilityState = "SUPPORTED"
	CapabilityUnsupported CapabilityState = "UNSUPPORTED"
	CapabilityDegraded    CapabilityState = "DEGRADED"
)

var ErrUnsupportedCapability = errors.New("message-plane capability is unsupported")

type CapabilityReport struct {
	OrganizationID string
	ConversationID string
	States         map[Capability]CapabilityState
}

type PresenceUpdate struct {
	OrganizationID string
	ConversationID string
	PrincipalID    string
	State          string
}

type TypingUpdate struct {
	OrganizationID string
	ConversationID string
	PrincipalID    string
	Typing         bool
}

type DeliveryReceipt struct {
	OrganizationID string
	ConversationID string
	MessageID      string
	PrincipalID    string
	State          string
}

type ReadReceipt struct {
	OrganizationID string
	ConversationID string
	MessageID      string
	PrincipalID    string
	MessageSeq     uint64
}

// CapabilityPlane makes realtime feature support explicit. Implementations
// must report unsupported operations instead of silently simulating success.
type CapabilityPlane interface {
	Capabilities(context.Context, string, string) (CapabilityReport, error)
	PublishPresence(context.Context, PresenceUpdate) error
	PublishTyping(context.Context, TypingUpdate) error
	RecordDeliveryReceipt(context.Context, DeliveryReceipt) error
	RecordReadReceipt(context.Context, ReadReceipt) error
}

func unsupportedCapabilityReport(ctx context.Context, organizationID, conversationID string) (CapabilityReport, error) {
	if err := requirePlaneTenant(ctx, organizationID); err != nil {
		return CapabilityReport{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return CapabilityReport{}, errors.New("conversation_id is required")
	}
	return CapabilityReport{OrganizationID: organizationID, ConversationID: conversationID, States: map[Capability]CapabilityState{
		CapabilityPresence: CapabilityUnsupported, CapabilityTyping: CapabilityUnsupported,
		CapabilityDeliveryReceipt: CapabilityUnsupported, CapabilityReadReceipt: CapabilityUnsupported,
	}}, nil
}

func unsupportedCapabilityError(Capability) error { return ErrUnsupportedCapability }

func (*PostgresPlane) Capabilities(ctx context.Context, organizationID, conversationID string) (CapabilityReport, error) {
	return unsupportedCapabilityReport(ctx, organizationID, conversationID)
}

func (*WuKongIMAdapter) Capabilities(ctx context.Context, organizationID, conversationID string) (CapabilityReport, error) {
	return unsupportedCapabilityReport(ctx, organizationID, conversationID)
}

func (*PostgresPlane) PublishPresence(context.Context, PresenceUpdate) error {
	return unsupportedCapabilityError(CapabilityPresence)
}

func (*PostgresPlane) PublishTyping(context.Context, TypingUpdate) error {
	return unsupportedCapabilityError(CapabilityTyping)
}

func (*PostgresPlane) RecordDeliveryReceipt(context.Context, DeliveryReceipt) error {
	return unsupportedCapabilityError(CapabilityDeliveryReceipt)
}

func (*PostgresPlane) RecordReadReceipt(context.Context, ReadReceipt) error {
	return unsupportedCapabilityError(CapabilityReadReceipt)
}

func (*WuKongIMAdapter) PublishPresence(context.Context, PresenceUpdate) error {
	return unsupportedCapabilityError(CapabilityPresence)
}

func (*WuKongIMAdapter) PublishTyping(context.Context, TypingUpdate) error {
	return unsupportedCapabilityError(CapabilityTyping)
}

func (*WuKongIMAdapter) RecordDeliveryReceipt(context.Context, DeliveryReceipt) error {
	return unsupportedCapabilityError(CapabilityDeliveryReceipt)
}

func (*WuKongIMAdapter) RecordReadReceipt(context.Context, ReadReceipt) error {
	return unsupportedCapabilityError(CapabilityReadReceipt)
}

var _ CapabilityPlane = (*PostgresPlane)(nil)
var _ CapabilityPlane = (*WuKongIMAdapter)(nil)
