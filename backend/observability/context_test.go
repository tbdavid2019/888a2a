package observability

import (
	"context"
	"testing"
)

func TestCorrelationContextIsTenantScoped(t *testing.T) {
	ctx := WithCorrelationID(WithTenant(context.Background(), "org-a"), "corr-a")
	if Tenant(ctx) != "org-a" || CorrelationID(ctx) != "corr-a" {
		t.Fatalf("context = tenant %q correlation %q", Tenant(ctx), CorrelationID(ctx))
	}
	if len(Attributes(ctx)) != 2 {
		t.Fatalf("attributes = %d, want 2", len(Attributes(ctx)))
	}
	if id := NewCorrelationID(); len(id) != 32 {
		t.Fatalf("correlation id length = %d", len(id))
	}
}
