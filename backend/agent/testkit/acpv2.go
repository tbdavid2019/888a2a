// Package testkit contains deterministic in-memory protocol fakes for tests.
package testkit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/agent/acp2"
)

// TurnKey identifies a deterministic turn fixture.
type TurnKey struct {
	AgentID string
	Prompt  string
}

// TurnResult controls the response and terminal notification for a turn.
type TurnResult struct {
	ID     string
	Text   string
	Status string
}

// Config controls the fake server's handshake and turn fixtures.
type Config struct {
	UserAgent       string
	ProtocolVersion string
	AgentID         string
	Turns           map[TurnKey]TurnResult
}

// Option configures a fake app-server.
type Option func(*Config)

// WithTurn registers a deterministic response keyed by agent and prompt.
func WithTurn(agentID, prompt, text string) Option {
	return func(c *Config) {
		if c.Turns == nil {
			c.Turns = map[TurnKey]TurnResult{}
		}
		c.Turns[TurnKey{AgentID: agentID, Prompt: prompt}] = TurnResult{Text: text}
	}
}

// WithTurnResult registers a complete deterministic turn fixture.
func WithTurnResult(key TurnKey, result TurnResult) Option {
	return func(c *Config) {
		if c.Turns == nil {
			c.Turns = map[TurnKey]TurnResult{}
		}
		c.Turns[key] = result
	}
}

// Server is an ACP v2 JSON-RPC app-server bound to an input and output.
type Server struct {
	r     *bufio.Reader
	w     *acp2.Transport
	rawR  io.Reader
	rawW  io.Writer
	cfg   Config
	start sync.Once
	close sync.Once
	wg    sync.WaitGroup

	mu      sync.Mutex
	agentID string
}

func newConfig(options []Option) Config {
	c := Config{
		UserAgent:       "fake-acp-v2/1.0",
		ProtocolVersion: "2.0",
		Turns:           map[TurnKey]TurnResult{},
	}
	for _, option := range options {
		option(&c)
	}
	return c
}

// NewServer binds an ACP v2 fake to r and w. Start must be called before use.
func NewServer(r io.Reader, w io.Writer, options ...Option) *Server {
	return &Server{
		r:    bufio.NewReader(r),
		w:    acp2.NewTransport(w),
		rawR: r,
		rawW: w,
		cfg:  newConfig(options),
	}
}

// StartServer constructs and starts a fake bound to r and w.
func StartServer(r io.Reader, w io.Writer, options ...Option) *Server {
	s := NewServer(r, w, options...)
	s.Start()
	return s
}

// Start begins serving requests.
func (s *Server) Start() {
	s.start.Do(func() {
		s.wg.Add(1)
		go s.serve()
	})
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		msg, err := acp2.ReadMessage(s.r)
		if err != nil {
			return
		}
		if !msg.IsRequest() {
			continue
		}
		s.handle(msg)
	}
}

func (s *Server) handle(msg acp2.Message) {
	switch msg.Method {
	case "initialize":
		var params acp2.InitializeParams
		if json.Unmarshal(msg.Params, &params) == nil && params.ClientInfo.Name != "" {
			s.mu.Lock()
			s.agentID = params.ClientInfo.Name
			s.mu.Unlock()
		}
		s.result(msg.ID, map[string]any{
			"userAgent":       s.cfg.UserAgent,
			"protocolVersion": s.cfg.ProtocolVersion,
		})
	case "thread/start":
		s.result(msg.ID, map[string]any{"thread": map[string]any{"id": s.id("thread", "")}})
	case "turn/start":
		var params acp2.TurnStartParams
		if err := json.Unmarshal(msg.Params, &params); err != nil || len(params.Input) == 0 {
			s.errorResult(msg.ID, -32602, "turn/start requires text input")
			return
		}
		s.turn(msg.ID, params.Input[0].Text)
	default:
		s.errorResult(msg.ID, -32601, fmt.Sprintf("method not found: %s", msg.Method))
	}
}

func (s *Server) turn(requestID json.RawMessage, prompt string) {
	s.mu.Lock()
	agentID := s.agentID
	if agentID == "" {
		agentID = s.cfg.AgentID
	}
	result, ok := s.cfg.Turns[TurnKey{AgentID: agentID, Prompt: prompt}]
	s.mu.Unlock()
	if !ok {
		result = TurnResult{Text: fmt.Sprintf("reply from %s: %s", agentID, prompt)}
	}
	if result.ID == "" {
		result.ID = s.idFor("turn", agentID, prompt)
	}
	if result.Status == "" {
		result.Status = "completed"
	}
	s.result(requestID, map[string]any{"turn": map[string]any{"id": result.ID}})
	_ = s.w.Send(map[string]any{"jsonrpc": "2.0", "method": "turn/started", "params": map[string]any{"turn": map[string]any{"id": result.ID}}})
	if result.Text != "" {
		_ = s.w.Send(map[string]any{"jsonrpc": "2.0", "method": "item/agentMessage/delta", "params": map[string]any{"itemId": result.ID + "-message", "phase": "final_answer", "delta": result.Text}})
	}
	_ = s.w.Send(map[string]any{"jsonrpc": "2.0", "method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": result.ID, "status": result.Status}}})
}

func (s *Server) result(id json.RawMessage, result any) {
	_ = s.w.Send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (s *Server) errorResult(id json.RawMessage, code int, message string) {
	_ = s.w.Send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": map[string]any{"code": code, "message": message}})
}

func (s *Server) id(kind, prompt string) string {
	s.mu.Lock()
	agentID := s.agentID
	if agentID == "" {
		agentID = s.cfg.AgentID
	}
	s.mu.Unlock()
	return s.idFor(kind, agentID, prompt)
}

func (*Server) idFor(kind, agentID, prompt string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + agentID + "\x00" + prompt))
	return kind + "-" + hex.EncodeToString(sum[:])[:16]
}

// Close stops the server and closes bound closers when available.
func (s *Server) Close() {
	s.close.Do(func() {
		if c, ok := s.rawR.(io.Closer); ok {
			_ = c.Close()
		}
		if c, ok := s.rawW.(io.Closer); ok {
			_ = c.Close()
		}
	})
	s.wg.Wait()
}

// NewClient returns a real acp2.Client connected to an in-memory fake server.
// The returned close function is idempotent and owns both pipe ends.
func NewClient(options ...Option) (*acp2.Client, *Server, func()) {
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()
	server := StartServer(serverR, serverW, options...)
	client := acp2.NewClient(acp2.NewTransport(clientW), clientR, nil)
	client.Start()
	var once sync.Once
	closePair := func() {
		once.Do(func() {
			client.Close()
			_ = clientW.Close()
			_ = clientR.Close()
			server.Close()
		})
	}
	return client, server, closePair
}

// AwaitTurn collects text deltas until the requested turn completes or ctx is
// canceled.
func AwaitTurn(ctx context.Context, client *acp2.Client, turnID string) (string, error) {
	var response string
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case notification, ok := <-client.Notifications():
			if !ok {
				return "", errors.New("ACP v2 notification stream closed")
			}
			switch notification.Method {
			case "item/agentMessage/delta":
				var params struct {
					Delta string `json:"delta"`
				}
				if err := json.Unmarshal(notification.Params, &params); err != nil {
					return "", err
				}
				response += params.Delta
			case "turn/completed":
				var params struct {
					Turn struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"turn"`
				}
				if err := json.Unmarshal(notification.Params, &params); err != nil {
					return "", err
				}
				if params.Turn.ID != turnID {
					return "", errors.Errorf("ACP v2 completed turn %q, want %q", params.Turn.ID, turnID)
				}
				if params.Turn.Status != "completed" {
					return "", errors.Errorf("ACP v2 turn %q status = %q", turnID, params.Turn.Status)
				}
				return response, nil
			default:
				continue
			}
		}
	}
}
