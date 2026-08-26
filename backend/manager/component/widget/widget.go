// Package widget contains the tenant-scoped public Web Widget boundary.
package widget

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const defaultSessionTTL = 15 * time.Minute

// Config controls one Organization's embeddable widget.
type Config struct {
	OrganizationID string
	WidgetID       string
	Enabled        bool
	SessionTTL     time.Duration
}

// BootstrapResponse is safe for an unauthenticated browser. It contains only
// the tenant/widget identity and a short-lived visitor session token.
type BootstrapResponse struct {
	OrganizationID string
	WidgetID       string
	SessionToken   string
	ExpiresAt      time.Time
}

type sessionClaims struct {
	OrganizationID string `json:"organization_id"`
	WidgetID       string `json:"widget_id"`
	SessionID      string `json:"session_id"`
	ExpiresAt      int64  `json:"expires_at"`
}

// OrganizationStateLookup and WidgetConfigLookup make the security boundary
// testable without requiring a live database. Production constructors provide
// PostgreSQL-backed implementations.
type OrganizationStateLookup func(context.Context, string) (string, error)
type WidgetConfigLookup func(context.Context, string, string) (Config, error)

// Service issues and verifies short-lived visitor sessions. It never accepts
// a caller-supplied Organization as proof of access: the configured lookup
// must confirm the Organization exists and is ACTIVE first.
type Service struct {
	secret       []byte
	now          func() time.Time
	lookupState  OrganizationStateLookup
	lookupConfig WidgetConfigLookup
}

func New(db *sql.DB, secret string) (*Service, error) {
	if db == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("widget database and signing secret are required")
	}
	return &Service{
		secret: []byte(secret), now: time.Now,
		lookupState: func(ctx context.Context, organizationID string) (string, error) {
			var state string
			err := db.QueryRowContext(ctx, `SELECT state FROM organizations WHERE id = $1`, organizationID).Scan(&state)
			if errors.Is(err, sql.ErrNoRows) {
				return "", errors.New("organization not found")
			}
			return state, err
		},
		lookupConfig: func(ctx context.Context, organizationID, widgetID string) (Config, error) {
			config := Config{OrganizationID: organizationID, WidgetID: widgetID}
			var enabled bool
			var ttlSeconds int
			err := db.QueryRowContext(ctx, `
				SELECT enabled, session_ttl_seconds
				FROM a2a888_web_widget_config
				WHERE organization_id = $1 AND widget_id = $2
			`, organizationID, widgetID).Scan(&enabled, &ttlSeconds)
			if errors.Is(err, sql.ErrNoRows) {
				return Config{}, errors.New("widget configuration not found")
			}
			if err != nil {
				return Config{}, err
			}
			config.Enabled = enabled
			config.SessionTTL = time.Duration(ttlSeconds) * time.Second
			return config, nil
		},
	}, nil
}

func (s *Service) validateConfig(ctx context.Context, organizationID, widgetID string) (Config, error) {
	if s == nil || len(s.secret) == 0 || s.lookupState == nil || s.lookupConfig == nil {
		return Config{}, errors.New("widget service is not configured")
	}
	if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(widgetID) == "" {
		return Config{}, errors.New("organization_id and widget_id are required")
	}
	state, err := s.lookupState(ctx, organizationID)
	if err != nil {
		return Config{}, errors.Wrap(err, "lookup widget organization")
	}
	if state != "ACTIVE" {
		return Config{}, errors.New("organization is not active")
	}
	config, err := s.lookupConfig(ctx, organizationID, widgetID)
	if err != nil {
		return Config{}, errors.Wrap(err, "lookup widget configuration")
	}
	if !config.Enabled {
		return Config{}, errors.New("widget is disabled")
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = defaultSessionTTL
	}
	return config, nil
}

// Bootstrap starts a visitor session or validates the supplied existing
// session. An expired or tenant-mismatched token is always rejected.
func (s *Service) Bootstrap(ctx context.Context, organizationID, widgetID, existingToken string) (BootstrapResponse, error) {
	config, err := s.validateConfig(ctx, organizationID, widgetID)
	if err != nil {
		return BootstrapResponse{}, err
	}
	if existingToken != "" {
		claims, err := s.VerifySession(existingToken, organizationID, widgetID)
		if err != nil {
			return BootstrapResponse{}, err
		}
		return BootstrapResponse{OrganizationID: claims.OrganizationID, WidgetID: claims.WidgetID, SessionToken: existingToken, ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC()}, nil
	}
	return s.issueSession(config)
}

func (s *Service) issueSession(config Config) (BootstrapResponse, error) {
	sessionID := make([]byte, 18)
	if _, err := rand.Read(sessionID); err != nil {
		return BootstrapResponse{}, errors.Wrap(err, "generate widget session")
	}
	expiresAt := s.now().UTC().Add(config.SessionTTL)
	claims := sessionClaims{OrganizationID: config.OrganizationID, WidgetID: config.WidgetID, SessionID: base64.RawURLEncoding.EncodeToString(sessionID), ExpiresAt: expiresAt.Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return BootstrapResponse{}, errors.Wrap(err, "encode widget session")
	}
	token := s.sign(payload)
	return BootstrapResponse{OrganizationID: config.OrganizationID, WidgetID: config.WidgetID, SessionToken: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) sign(payload []byte) string {
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	hash := hmac.New(sha256.New, s.secret)
	_, _ = hash.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
	return encoded + "." + signature
}

// VerifySession validates signature, tenant/widget binding, and expiry.
func (s *Service) VerifySession(token, organizationID, widgetID string) (sessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return sessionClaims{}, errors.New("invalid widget session")
	}
	hash := hmac.New(sha256.New, s.secret)
	_, _ = hash.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return sessionClaims{}, errors.New("invalid widget session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, errors.Wrap(err, "decode widget session")
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return sessionClaims{}, errors.Wrap(err, "decode widget session claims")
	}
	if claims.OrganizationID != organizationID || claims.WidgetID != widgetID || claims.SessionID == "" {
		return sessionClaims{}, errors.New("widget session tenant mismatch")
	}
	if s.now().UTC().Unix() >= claims.ExpiresAt {
		return sessionClaims{}, errors.New("widget session expired")
	}
	return claims, nil
}
