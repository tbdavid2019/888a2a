// Package connector defines the versioned boundary between the collaboration
// service and external messaging platforms.
package connector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const ContractVersion = "v1"

type Capability string

const (
	CapabilityInstall     Capability = "install"
	CapabilityVerify      Capability = "verify"
	CapabilityNormalize   Capability = "normalize"
	CapabilityText        Capability = "text"
	CapabilityMedia       Capability = "media"
	CapabilityReplies     Capability = "replies"
	CapabilityThreads     Capability = "threads"
	CapabilityEdits       Capability = "edits"
	CapabilityRecalls     Capability = "recalls"
	CapabilityReactions   Capability = "reactions"
	CapabilityReceipts    Capability = "receipts"
	CapabilityInteractive Capability = "interactive"
	CapabilityOutbound    Capability = "outbound"
)

type CapabilityMatrix map[Capability]bool

func (m CapabilityMatrix) Supports(capability Capability) bool { return m != nil && m[capability] }

type Manifest struct {
	Kind         string
	Contract     string
	Capabilities CapabilityMatrix
}

func (m Manifest) Validate() error {
	if m.Kind == "" || m.Contract != ContractVersion {
		return errors.New("connector kind and supported contract version are required")
	}
	if !m.Capabilities.Supports(CapabilityVerify) || !m.Capabilities.Supports(CapabilityNormalize) {
		return errors.New("connector must declare verify and normalize capabilities")
	}
	return nil
}

type Installation struct {
	OrganizationID string
	InstallationID string
	Kind           string
}

func (i Installation) Validate() error {
	if i.OrganizationID == "" || i.InstallationID == "" || i.Kind == "" {
		return errors.New("connector installation must be organization-scoped")
	}
	return nil
}

type VerifiedInbound struct {
	Installation Installation
	ExternalID   string
	ReceivedAt   time.Time
	Headers      http.Header
	Raw          json.RawMessage
}

type Envelope struct {
	OrganizationID       string
	InstallationID       string
	ExternalEventID      string
	ExternalConversation string
	ExternalSender       string
	EventType            string
	OccurredAt           time.Time
	Text                 string
	VendorExtension      json.RawMessage
}

type Outbound struct {
	Installation   Installation
	ConversationID string
	Text           string
	ReplyTo        string
	Media          []MediaPart
	Interactive    []json.RawMessage
}

type MediaPart struct {
	Type       string
	URL        string
	PreviewURL string
}

type DeliveryResult struct {
	ExternalID string
	RetryAt    time.Time
	Terminal   bool
	Reason     string
}

// Connector is the stable contract implemented by each platform adapter.
// Authentication and normalization happen before routing; delivery remains
// installation-scoped so one tenant cannot consume another's credentials.
type Connector interface {
	Manifest() Manifest
	VerifyInbound(context.Context, Installation, http.Header, []byte) (VerifiedInbound, error)
	Normalize(context.Context, VerifiedInbound) (Envelope, error)
	Deliver(context.Context, Installation, Outbound) (DeliveryResult, error)
}
