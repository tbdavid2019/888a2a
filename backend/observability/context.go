// Package observability defines tenant-safe correlation fields shared by
// request logs, traces, and durable event metadata.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"log/slog"
)

type contextKey string

const (
	tenantKey      contextKey = "observability.tenant"
	correlationKey contextKey = "observability.correlation"
)

func WithTenant(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, tenantKey, strings.TrimSpace(tenant))
}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationKey, strings.TrimSpace(correlationID))
}

func Tenant(ctx context.Context) string {
	value, _ := ctx.Value(tenantKey).(string)
	return value
}

func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey).(string)
	return value
}

func NewCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

func Attributes(ctx context.Context) []slog.Attr {
	attributes := make([]slog.Attr, 0, 2)
	if tenant := Tenant(ctx); tenant != "" {
		attributes = append(attributes, slog.String("organization_id", tenant))
	}
	if correlationID := CorrelationID(ctx); correlationID != "" {
		attributes = append(attributes, slog.String("correlation_id", correlationID))
	}
	return attributes
}
