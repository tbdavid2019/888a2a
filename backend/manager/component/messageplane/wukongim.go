package messageplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// WuKongIMConfig configures the internal WuKongIM HTTP adapter. The server's
// HTTP API is an internal business API; it is never registered as a public
// 888a2a route.
type WuKongIMConfig struct {
	BaseURL     string
	HTTPClient  *http.Client
	ChannelType int
	LoginUID    func(organizationID, conversationID string) string
	Credentials func(context.Context, string, string) (ConnectionCredentials, error)
}

// WuKongIMAdapter implements MessagePlane using WuKongIM's internal HTTP API.
// Endpoint shapes follow the official message send, channel sync, subscriber,
// readiness, and user-token documentation.
type WuKongIMAdapter struct {
	baseURL     string
	client      *http.Client
	channelType int
	loginUID    func(string, string) string
	credentials func(context.Context, string, string) (ConnectionCredentials, error)
}

// NewWuKongIMAdapter validates and creates an internal-only adapter. Public
// IPs and unresolved hosts are rejected to prevent accidentally exposing or
// using the WuKongIM administration/business API over the public network.
func NewWuKongIMAdapter(config WuKongIMConfig) (*WuKongIMAdapter, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.New("WuKongIM base URL must be an authenticated-host-free HTTP(S) URL")
	}
	if err := requirePrivateHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	channelType := config.ChannelType
	if channelType == 0 {
		channelType = 2
	}
	if channelType < 1 || channelType > 2 {
		return nil, errors.New("WuKongIM channel type must be 1 or 2")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &WuKongIMAdapter{
		baseURL: strings.TrimRight(config.BaseURL, "/"), client: &clientCopy, channelType: channelType,
		loginUID: config.LoginUID, credentials: config.Credentials,
	}, nil
}

func requirePrivateHost(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil
		}
		return errors.New("WuKongIM adapter requires a private or loopback host")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		if err == nil {
			return errors.New("resolve WuKongIM internal host returned no addresses")
		}
		return errors.Wrap(err, "resolve WuKongIM internal host")
	}
	for _, ip := range ips {
		if !isPrivateIP(ip) {
			return errors.New("WuKongIM adapter requires a private or loopback host")
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// IssueCredentials delegates credential issuance to the 888a2a business
// layer. WuKongIM's internal API is not a public authentication endpoint.
func (a *WuKongIMAdapter) IssueCredentials(ctx context.Context, organizationID, conversationID string) (ConnectionCredentials, error) {
	if err := requirePlaneTenant(ctx, organizationID); err != nil {
		return ConnectionCredentials{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return ConnectionCredentials{}, errors.New("conversation_id is required")
	}
	if a.credentials == nil {
		return ConnectionCredentials{}, errors.New("WuKongIM credential provider is not configured")
	}
	credentials, err := a.credentials(ctx, organizationID, conversationID)
	if err != nil {
		return ConnectionCredentials{}, err
	}
	if credentials.OrganizationID != organizationID || credentials.ConversationID != conversationID || credentials.Token == "" {
		return ConnectionCredentials{}, errors.New("WuKongIM credential provider returned invalid tenant credentials")
	}
	return credentials, nil
}

type wuKongSendRequest struct {
	ClientMessageNo string `json:"client_msg_no"`
	FromUID         string `json:"from_uid"`
	ChannelID       string `json:"channel_id"`
	ChannelType     int    `json:"channel_type"`
	Payload         string `json:"payload"`
}

type wuKongSendResponse struct {
	MessageID       json.RawMessage `json:"message_id"`
	MessageSeq      json.RawMessage `json:"message_seq"`
	ClientMessageNo string          `json:"client_msg_no"`
	Data            *wuKongSendData `json:"data"`
}

type wuKongSendData struct {
	MessageID  json.RawMessage `json:"message_id"`
	MessageSeq json.RawMessage `json:"message_seq"`
}

// Append sends one message to WuKongIM. The vendor receives base64 payloads
// and owns its channel sequence and client-message deduplication.
func (a *WuKongIMAdapter) Append(ctx context.Context, input MessageInput) (Message, error) {
	if err := requirePlaneTenant(ctx, input.OrganizationID); err != nil {
		return Message{}, err
	}
	if input.ConversationID == "" || input.ClientMessageNo == "" || input.SenderID == "" {
		return Message{}, errors.New("conversation_id, client_message_no, and sender_id are required")
	}
	if len(input.Payload) == 0 {
		input.Payload = []byte(`{}`)
	}
	if !json.Valid(input.Payload) {
		return Message{}, errors.New("message payload must be valid JSON")
	}
	body := wuKongSendRequest{
		ClientMessageNo: input.ClientMessageNo, FromUID: input.SenderID,
		ChannelID: input.ConversationID, ChannelType: a.channelType,
		Payload: base64.StdEncoding.EncodeToString(input.Payload),
	}
	var response wuKongSendResponse
	if err := a.postJSON(ctx, "/message/send", body, &response); err != nil {
		return Message{}, err
	}
	messageIDRaw := response.MessageID
	messageSeqRaw := response.MessageSeq
	if response.Data != nil {
		if len(messageIDRaw) == 0 {
			messageIDRaw = response.Data.MessageID
		}
		if len(messageSeqRaw) == 0 {
			messageSeqRaw = response.Data.MessageSeq
		}
	}
	messageID, err := rawNumberString(messageIDRaw)
	if err != nil {
		return Message{}, errors.Wrap(err, "decode WuKongIM message_id")
	}
	messageSeq := uint64(0)
	if len(messageSeqRaw) > 0 {
		messageSeq, err = rawUint64(messageSeqRaw)
		if err != nil {
			return Message{}, errors.Wrap(err, "decode WuKongIM message_seq")
		}
	}
	if messageSeq == 0 {
		messageSeq, err = a.findMessageSequence(ctx, input, messageID)
		if err != nil {
			return Message{}, errors.Wrap(err, "resolve WuKongIM message_seq")
		}
	}
	if response.ClientMessageNo == "" {
		response.ClientMessageNo = input.ClientMessageNo
	}
	return Message{OrganizationID: input.OrganizationID, ConversationID: input.ConversationID, MessageID: messageID, ClientMessageNo: response.ClientMessageNo, MessageSeq: messageSeq, SenderID: input.SenderID, Payload: append([]byte(nil), input.Payload...)}, nil
}

type wuKongHistoryRequest struct {
	LoginUID        string `json:"login_uid"`
	ChannelID       string `json:"channel_id"`
	ChannelType     int    `json:"channel_type"`
	StartMessageSeq uint64 `json:"start_message_seq"`
	EndMessageSeq   uint64 `json:"end_message_seq"`
	Limit           int    `json:"limit"`
	PullMode        int    `json:"pull_mode"`
}

type wuKongHistoryMessage struct {
	MessageID       json.RawMessage `json:"message_id"`
	MessageSeq      uint64          `json:"message_seq"`
	ClientMessageNo string          `json:"client_msg_no"`
	FromUID         string          `json:"from_uid"`
	Payload         string          `json:"payload"`
}

// History syncs messages after a durable cursor using WuKongIM's channel
// message sync endpoint.
func (a *WuKongIMAdapter) History(ctx context.Context, request HistoryRequest) (HistoryResponse, error) {
	if err := requirePlaneTenant(ctx, request.OrganizationID); err != nil {
		return HistoryResponse{}, err
	}
	if request.ConversationID == "" || request.Limit <= 0 || request.Limit > 10000 {
		return HistoryResponse{}, errors.New("conversation_id and bounded positive limit are required")
	}
	if request.After.OrganizationID != "" && (request.After.OrganizationID != request.OrganizationID || request.After.ConversationID != request.ConversationID) {
		return HistoryResponse{}, errors.New("message cursor tenant or conversation mismatch")
	}
	if request.After.MessageSeq == ^uint64(0) {
		return HistoryResponse{NextCursor: request.After}, nil
	}
	loginUID := ""
	if a.loginUID != nil {
		loginUID = a.loginUID(request.OrganizationID, request.ConversationID)
	}
	if loginUID == "" {
		return HistoryResponse{}, errors.New("WuKongIM history login UID is not configured")
	}
	var rawResponse json.RawMessage
	if err := a.postJSON(ctx, "/channel/messagesync", wuKongHistoryRequest{
		LoginUID: loginUID, ChannelID: request.ConversationID, ChannelType: a.channelType,
		StartMessageSeq: request.After.MessageSeq + 1, Limit: request.Limit, PullMode: 1,
	}, &rawResponse); err != nil {
		return HistoryResponse{}, err
	}
	var messages []wuKongHistoryMessage
	if len(rawResponse) > 0 && rawResponse[0] == '[' {
		if err := json.Unmarshal(rawResponse, &messages); err != nil {
			return HistoryResponse{}, errors.Wrap(err, "decode WuKongIM history list")
		}
	} else {
		var envelope struct {
			Messages []wuKongHistoryMessage `json:"messages"`
		}
		if err := json.Unmarshal(rawResponse, &envelope); err != nil {
			return HistoryResponse{}, errors.Wrap(err, "decode WuKongIM history envelope")
		}
		messages = envelope.Messages
	}
	response := HistoryResponse{NextCursor: request.After}
	for _, raw := range messages {
		messageID, err := rawNumberString(raw.MessageID)
		if err != nil {
			return HistoryResponse{}, errors.Wrap(err, "decode WuKongIM history message_id")
		}
		payload, err := base64.StdEncoding.DecodeString(raw.Payload)
		if err != nil {
			return HistoryResponse{}, errors.Wrap(err, "decode WuKongIM history payload")
		}
		response.Messages = append(response.Messages, Message{OrganizationID: request.OrganizationID, ConversationID: request.ConversationID, MessageID: messageID, ClientMessageNo: raw.ClientMessageNo, MessageSeq: raw.MessageSeq, SenderID: raw.FromUID, Payload: payload})
	}
	if len(response.Messages) > 0 {
		last := response.Messages[len(response.Messages)-1]
		response.NextCursor = Cursor{OrganizationID: last.OrganizationID, ConversationID: last.ConversationID, MessageSeq: last.MessageSeq}
	}
	return response, nil
}

func (a *WuKongIMAdapter) findMessageSequence(ctx context.Context, input MessageInput, messageID string) (uint64, error) {
	const attempts = 10
	for attempt := 0; attempt < attempts; attempt++ {
		history, err := a.History(ctx, HistoryRequest{
			OrganizationID: input.OrganizationID, ConversationID: input.ConversationID,
			Limit: 10000,
		})
		if err != nil {
			return 0, err
		}
		for _, message := range history.Messages {
			if message.MessageID == messageID || message.ClientMessageNo == input.ClientMessageNo {
				return message.MessageSeq, nil
			}
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return 0, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return 0, errors.New("sent message was not returned by WuKongIM history")
}

// ProjectMembership updates channel subscribers through the internal channel
// API. Role semantics remain in 888a2a's tenant projection.
func (a *WuKongIMAdapter) ProjectMembership(ctx context.Context, projection MembershipProjection) error {
	if err := requirePlaneTenant(ctx, projection.OrganizationID); err != nil {
		return err
	}
	if projection.ConversationID == "" || projection.PrincipalID == "" || projection.Role == "" {
		return errors.New("conversation_id, principal_id, and role are required")
	}
	return a.postJSON(ctx, "/channel/subscriber_add", map[string]any{
		"channel_id": projection.ConversationID, "channel_type": a.channelType,
		"subscribers": []string{projection.PrincipalID}, "reset": 0, "temp_subscriber": 0,
	}, nil)
}

// Health checks WuKongIM readiness without exposing or calling its manager API.
func (a *WuKongIMAdapter) Health(ctx context.Context) (Health, error) {
	for _, endpoint := range []string{"/readyz", "/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+endpoint, nil)
		if err != nil {
			return Health{Healthy: false, Detail: err.Error()}, err
		}
		response, err := a.client.Do(req)
		if err != nil {
			return Health{Healthy: false, Detail: err.Error()}, err
		}
		status := response.StatusCode
		_ = response.Body.Close()
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			return Health{Healthy: true, Detail: "WuKongIM ready"}, nil
		}
		if status != http.StatusNotFound || endpoint != "/readyz" {
			return Health{Healthy: false, Detail: fmt.Sprintf("readiness returned HTTP %d", status)}, fmt.Errorf("WuKongIM readiness returned HTTP %d", status)
		}
	}
	return Health{Healthy: false, Detail: "WuKongIM readiness endpoint unavailable"}, errors.New("WuKongIM readiness endpoint unavailable")
}

func (a *WuKongIMAdapter) postJSON(ctx context.Context, path string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return errors.Wrap(err, "encode WuKongIM request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, strings.NewReader(string(data)))
	if err != nil {
		return errors.Wrap(err, "create WuKongIM request")
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "call WuKongIM internal API")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("WuKongIM %s returned HTTP %d", path, response.StatusCode)
	}
	if result == nil {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(result); err != nil {
		return errors.Wrap(err, "decode WuKongIM response")
	}
	return nil
}

func rawNumberString(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", err
		}
		value = decoded
	}
	if value == "" || value == "null" {
		return "", errors.New("empty numeric value")
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return "", err
	}
	return value, nil
}

func rawUint64(raw json.RawMessage) (uint64, error) {
	value, err := rawNumberString(raw)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(value, 10, 64)
}

var _ MessagePlane = (*WuKongIMAdapter)(nil)
