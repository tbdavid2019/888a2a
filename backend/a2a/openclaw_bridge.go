package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// OpenClawBridgeConfig configures an authenticated local OpenClaw Gateway.
// Token is a callback so the secret is not retained in a bridge object or
// logged by the bridge.
type OpenClawBridgeConfig struct {
	ID      string
	BaseURL string
	AgentID string
	Token   func(context.Context) (string, error)
	Client  *http.Client
}

// OpenClawBridge invokes the documented OpenResponses endpoint exposed by an
// OpenClaw Gateway. It intentionally does not use /tools/invoke because that
// endpoint is a broad operator surface rather than a peer execution contract.
type OpenClawBridge struct {
	baseURL string
	agentID string
	token   func(context.Context) (string, error)
	client  *http.Client
	id      string
}

func NewOpenClawBridge(config OpenClawBridgeConfig) (*OpenClawBridge, error) {
	if strings.TrimSpace(config.ID) == "" || strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("OpenClaw bridge requires id and base URL")
	}
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("OpenClaw bridge requires an authenticated-host-free HTTP(S) URL")
	}
	if err := requirePrivateBridgeHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(config.AgentID)
	if agentID == "" {
		agentID = "default"
	}
	client := config.Client
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &OpenClawBridge{id: config.ID, baseURL: strings.TrimRight(config.BaseURL, "/"), agentID: agentID, token: config.Token, client: &clientCopy}, nil
}

func (b *OpenClawBridge) ID() string { return b.id }

func (b *OpenClawBridge) Preflight(_ context.Context, request BridgeRequest) error {
	if err := ValidateBridgeRequest(request, b.ID()); err != nil {
		return err
	}
	if b.token == nil {
		return fmt.Errorf("OpenClaw bridge token provider is not configured")
	}
	return nil
}

func (b *OpenClawBridge) Start(_ context.Context, _ BridgeRequest) (BridgeSession, error) {
	return &openClawBridgeSession{bridge: b}, nil
}

func (b *OpenClawBridge) Health(ctx context.Context) (BridgeHealth, error) {
	if b.token == nil {
		return BridgeHealth{Detail: "token provider is not configured"}, nil
	}
	token, err := b.token(ctx)
	if err != nil || token == "" {
		return BridgeHealth{Detail: "token provider failed"}, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/v1/models", nil)
	if err != nil {
		return BridgeHealth{Detail: "invalid OpenClaw health request"}, nil
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := b.client.Do(request)
	if err != nil {
		return BridgeHealth{Detail: "OpenClaw Gateway is unreachable"}, nil
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return BridgeHealth{Detail: fmt.Sprintf("OpenClaw Gateway returned HTTP %d", response.StatusCode)}, nil
	}
	return BridgeHealth{Ready: true, Detail: "OpenClaw OpenResponses endpoint is reachable"}, nil
}

type openClawBridgeSession struct{ bridge *OpenClawBridge }

func (s *openClawBridgeSession) Invoke(ctx context.Context, request BridgeRequest, emit func(BridgeEvent) error) (BridgeResult, error) {
	token, err := s.bridge.token(ctx)
	if err != nil || token == "" {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "OpenClaw token unavailable"}, fmt.Errorf("OpenClaw token unavailable")
	}
	body, err := json.Marshal(map[string]any{
		"model":  "openclaw/" + s.bridge.agentID,
		"input":  request.Input,
		"stream": false,
		"user":   request.ContextID,
		"metadata": map[string]string{
			"organization_id": request.OrganizationID,
			"caller_id":       request.CallerID,
			"task_id":         request.TaskID,
			"correlation_id":  request.CorrelationID,
		},
	})
	if err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "encode OpenClaw request"}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(runCtx, http.MethodPost, s.bridge.baseURL+"/v1/responses", strings.NewReader(string(body)))
	if err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "create OpenClaw request"}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := s.bridge.client.Do(httpRequest)
	if err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeNotDelivered, Reason: "OpenClaw Gateway request failed"}, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, int64(request.MaxOutputBytes)+1))
	if readErr != nil {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "read OpenClaw response failed"}, readErr
	}
	if len(data) > request.MaxOutputBytes {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "OpenClaw response exceeded output limit"}, fmt.Errorf("OpenClaw response exceeded output limit")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "OpenClaw authentication rejected"}, fmt.Errorf("OpenClaw authentication rejected")
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return BridgeResult{Outcome: DeliveryOutcomeNotDelivered, Reason: fmt.Sprintf("OpenClaw Gateway returned HTTP %d", response.StatusCode)}, fmt.Errorf("OpenClaw Gateway returned HTTP %d", response.StatusCode)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: fmt.Sprintf("OpenClaw Gateway returned HTTP %d", response.StatusCode)}, fmt.Errorf("OpenClaw Gateway returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "decode OpenClaw response failed"}, err
	}
	var parts []string
	for _, output := range payload.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	result := BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  strings.Join(parts, "\n"),
		Events: []BridgeEvent{
			{Sequence: 1, Kind: "output", Text: strings.Join(parts, "\n")},
			{Sequence: 2, Kind: "completed", Terminal: true},
		},
	}
	if err := ValidateBridgeResult(result); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "invalid OpenClaw response"}, err
	}
	for _, event := range result.Events {
		if emit != nil {
			if err := emit(event); err != nil {
				return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "OpenClaw event delivery failed"}, err
			}
		}
	}
	return result, nil
}

func (*openClawBridgeSession) Cancel(context.Context) error { return nil }
func (*openClawBridgeSession) Stop(context.Context) error   { return nil }

func requirePrivateBridgeHost(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateBridgeIP(ip) {
			return nil
		}
		return fmt.Errorf("bridge endpoint requires a private or loopback host")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("resolve bridge endpoint host: %w", err)
	}
	for _, ip := range ips {
		if !isPrivateBridgeIP(ip) {
			return fmt.Errorf("bridge endpoint requires a private or loopback host")
		}
	}
	return nil
}

func isPrivateBridgeIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
