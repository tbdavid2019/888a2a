// Package line implements the LINE Messaging API connector boundary.
package line

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tbdavid2019/888a2a/backend/connector"
)

const defaultAPIBaseURL = "https://api.line.me"

type Adapter struct {
	ChannelSecret string
	AccessToken   string
	APIBaseURL    string
	HTTPClient    *http.Client
}

func (a Adapter) Manifest() connector.Manifest {
	return connector.Manifest{Kind: "line", Contract: connector.ContractVersion, Capabilities: connector.CapabilityMatrix{
		connector.CapabilityInstall: true, connector.CapabilityVerify: true, connector.CapabilityNormalize: true,
		connector.CapabilityText: true, connector.CapabilityMedia: true, connector.CapabilityReplies: true,
		connector.CapabilityRecalls: true, connector.CapabilityOutbound: true,
	}}
}

func (a Adapter) VerifyInbound(_ context.Context, installation connector.Installation, headers http.Header, raw []byte) (connector.VerifiedInbound, error) {
	if strings.TrimSpace(a.ChannelSecret) == "" {
		return connector.VerifiedInbound{}, errors.New("LINE channel secret is not configured")
	}
	signature, err := base64.StdEncoding.DecodeString(headers.Get("X-Line-Signature"))
	if err != nil || len(signature) == 0 {
		return connector.VerifiedInbound{}, errors.New("LINE webhook signature is missing or invalid")
	}
	mac := hmac.New(sha256.New, []byte(a.ChannelSecret))
	_, _ = mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return connector.VerifiedInbound{}, errors.New("LINE webhook signature verification failed")
	}
	var envelope struct {
		Events []struct {
			WebhookEventID string `json:"webhookEventId"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return connector.VerifiedInbound{}, errors.New("LINE webhook JSON is invalid")
	}
	externalID := ""
	if len(envelope.Events) > 0 {
		externalID = envelope.Events[0].WebhookEventID
	}
	if externalID == "" {
		sum := sha256.Sum256(raw)
		externalID = "verification-" + fmt.Sprintf("%x", sum[:])
	}
	return connector.VerifiedInbound{Installation: installation, ExternalID: externalID, ReceivedAt: time.Now().UTC(), Headers: headers, Raw: append([]byte(nil), raw...)}, nil
}

type webhook struct {
	Events []event `json:"events"`
}

type event struct {
	Type           string `json:"type"`
	WebhookEventID string `json:"webhookEventId"`
	Timestamp      int64  `json:"timestamp"`
	ReplyToken     string `json:"replyToken"`
	Mode           string `json:"mode"`
	Source         struct {
		Type    string `json:"type"`
		UserID  string `json:"userId"`
		GroupID string `json:"groupId"`
		RoomID  string `json:"roomId"`
	} `json:"source"`
	Message struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"message"`
	Unsended struct {
		MessageID string `json:"messageId"`
	} `json:"unsend"`
}

type lineMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type lineOutboundPayload struct {
	To         string        `json:"to,omitempty"`
	ReplyToken string        `json:"replyToken,omitempty"`
	Messages   []lineMessage `json:"messages"`
}

func (a Adapter) Normalize(_ context.Context, inbound connector.VerifiedInbound) (connector.Envelope, error) {
	var payload webhook
	if err := json.Unmarshal(inbound.Raw, &payload); err != nil {
		return connector.Envelope{}, errors.New("LINE webhook JSON is invalid")
	}
	if len(payload.Events) == 0 {
		return connector.Envelope{OrganizationID: inbound.Installation.OrganizationID, InstallationID: inbound.Installation.InstallationID, ExternalEventID: inbound.ExternalID, EventType: "verification"}, nil
	}
	e := payload.Events[0]
	if e.WebhookEventID == "" || e.WebhookEventID != inbound.ExternalID {
		return connector.Envelope{}, errors.New("LINE webhook event identity does not match verified body")
	}
	conversationID := e.Source.UserID
	if e.Source.GroupID != "" {
		conversationID = e.Source.GroupID
	} else if e.Source.RoomID != "" {
		conversationID = e.Source.RoomID
	}
	eventType := e.Type
	text := e.Message.Text
	if e.Type == "unsend" {
		eventType = "recall"
		text = e.UnendedMessageID()
	}
	if conversationID == "" {
		return connector.Envelope{}, errors.New("LINE webhook source conversation is missing")
	}
	return connector.Envelope{OrganizationID: inbound.Installation.OrganizationID, InstallationID: inbound.Installation.InstallationID, ExternalEventID: e.WebhookEventID, ExternalConversation: conversationID, ExternalSender: e.Source.UserID, EventType: eventType, OccurredAt: time.UnixMilli(e.Timestamp).UTC(), Text: text}, nil
}

func (e event) UnendedMessageID() string { return e.Unsended.MessageID }

func (a Adapter) Deliver(ctx context.Context, installation connector.Installation, outbound connector.Outbound) (connector.DeliveryResult, error) {
	if err := installation.Validate(); err != nil {
		return connector.DeliveryResult{Terminal: true, Reason: err.Error()}, err
	}
	if strings.TrimSpace(a.AccessToken) == "" || outbound.ConversationID == "" || outbound.Text == "" {
		return connector.DeliveryResult{Terminal: true, Reason: "LINE delivery credentials and message are required"}, errors.New("LINE delivery credentials and message are required")
	}
	payload := lineOutboundPayload{To: outbound.ConversationID, Messages: []lineMessage{{Type: "text", Text: outbound.Text}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return connector.DeliveryResult{Terminal: true, Reason: "LINE message encoding failed"}, err
	}
	path := "/v2/bot/message/push"
	if outbound.ReplyTo != "" {
		path = "/v2/bot/message/reply"
	}
	if outbound.ReplyTo != "" {
		payload.To = ""
		payload.ReplyToken = outbound.ReplyTo
		body, err = json.Marshal(payload)
		if err != nil {
			return connector.DeliveryResult{Terminal: true, Reason: "LINE reply encoding failed"}, err
		}
	}
	baseURL := a.APIBaseURL
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return connector.DeliveryResult{Terminal: true, Reason: "LINE request creation failed"}, err
	}
	request.Header.Set("Authorization", "Bearer "+a.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Line-Retry-Key", uuid.NewString())
	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return connector.DeliveryResult{RetryAt: time.Now().UTC().Add(time.Minute), Reason: "LINE transport failure"}, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return connector.DeliveryResult{}, nil
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout {
		delay := time.Minute
		if value, parseErr := strconv.Atoi(response.Header.Get("Retry-After")); parseErr == nil && value > 0 {
			delay = time.Duration(value) * time.Second
		}
		return connector.DeliveryResult{RetryAt: time.Now().UTC().Add(delay), Reason: "LINE retryable delivery failure"}, fmt.Errorf("LINE delivery returned HTTP %d", response.StatusCode)
	}
	return connector.DeliveryResult{Terminal: true, Reason: "LINE terminal delivery failure"}, fmt.Errorf("LINE delivery returned HTTP %d", response.StatusCode)
}
