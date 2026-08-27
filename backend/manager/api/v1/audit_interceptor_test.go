package v1

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/observability"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestAuditPrincipalEvidenceUsesRequesterAndExecutor(t *testing.T) {
	ctx := common.SetOrganizationIDToContext(context.Background(), "org-a")
	ctx = common.SetRequesterPrincipalToContext(ctx, common.PrincipalIdentity{ID: "human-1", Type: "human"})
	ctx = common.SetExecutorPrincipalToContext(ctx, common.PrincipalIdentity{ID: "agent-1", Type: "agent"})
	if got := auditOrganizationID(ctx); got != "org-a" {
		t.Fatalf("audit organization = %q, want org-a", got)
	}
	if got := auditPrincipalID(ctx, true); got != "human-1" {
		t.Fatalf("audit requester = %q, want human-1", got)
	}
	if got := auditPrincipalID(ctx, false); got != "agent-1" {
		t.Fatalf("audit executor = %q, want agent-1", got)
	}
}

func TestAuditPrincipalEvidenceDefaultsOrganization(t *testing.T) {
	if got := auditOrganizationID(context.Background()); got != "default" {
		t.Fatalf("audit default organization = %q, want default", got)
	}
}

func TestObservabilityContextPropagatesCorrelationAndTenant(t *testing.T) {
	var handlerContext context.Context
	interceptor := &AuditInterceptor{}
	wrapped := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		handlerContext = ctx
		return connect.NewResponse(&emptypb.Empty{}), nil
	})
	request := connect.NewRequest(&emptypb.Empty{})
	request.Header().Set("X-Correlation-ID", "corr-test-1")
	response, err := wrapped(context.Background(), request)
	if err != nil {
		t.Fatalf("wrapped request returned error: %v", err)
	}
	if observability.CorrelationID(handlerContext) != "corr-test-1" {
		t.Fatalf("handler correlation id = %q", observability.CorrelationID(handlerContext))
	}
	if observability.Tenant(handlerContext) != "default" {
		t.Fatalf("handler tenant = %q", observability.Tenant(handlerContext))
	}
	if got := response.Header().Get("X-Correlation-ID"); got != "corr-test-1" {
		t.Fatalf("response correlation id = %q", got)
	}
}

func TestObservabilityContextRejectsHeaderInjection(t *testing.T) {
	ctx := withObservabilityContext(context.Background(), http.Header{"X-Correlation-ID": []string{"bad\nvalue"}})
	if observability.CorrelationID(ctx) == "bad\nvalue" || observability.CorrelationID(ctx) == "" {
		t.Fatalf("unsafe correlation id was accepted: %q", observability.CorrelationID(ctx))
	}
}
