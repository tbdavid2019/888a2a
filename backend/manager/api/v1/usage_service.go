package v1

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888/a2a888connect"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// UsageService exposes tenant-wide usage only to Organization owners and
// billing admins. It never returns credentials or payment-provider fields.
type UsageService struct {
	a2a888connect.UnimplementedUsageServiceHandler
	store *store.Store
}

func NewUsageService(s *store.Store) *UsageService { return &UsageService{store: s} }

func (s *UsageService) GetUsageSummary(ctx context.Context, req *connect.Request[a2a888.GetUsageSummaryRequest]) (*connect.Response[a2a888.GetUsageSummaryResponse], error) {
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	organizationID := req.Msg.OrganizationId
	if organizationID == "" {
		organizationID, ok = common.GetOrganizationIDFromContext(ctx)
		if !ok || organizationID == "" {
			organizationID = user.DefaultOrganizationID
		}
	}
	if organizationID == "" {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("usage access denied"))
	}
	membership, err := s.store.GetMembership(ctx, organizationID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE || !canViewOrganizationUsage(membership.Role) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("organization usage access denied"))
	}
	now := time.Now().UTC()
	start, end := now.Truncate(24*time.Hour), now.Truncate(24*time.Hour).Add(24*time.Hour)
	if req.Msg.PeriodStart != nil && req.Msg.PeriodStart.IsValid() {
		start = req.Msg.PeriodStart.AsTime().UTC()
	}
	if req.Msg.PeriodEnd != nil && req.Msg.PeriodEnd.IsValid() {
		end = req.Msg.PeriodEnd.AsTime().UTC()
	}
	if !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("usage period must be positive and at most one year"))
	}
	aggregates, err := s.store.ListUsageAggregates(ctx, organizationID, start, end)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "list organization usage"))
	}
	entitlements, err := s.store.ListEntitlements(ctx, organizationID, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "list organization entitlements"))
	}
	subscription, err := s.store.GetCurrentSubscription(ctx, organizationID, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "get organization subscription"))
	}
	readOnly := subscription != nil && (subscription.State == a2a888.SubscriptionState_SUBSCRIPTION_STATE_READ_ONLY || subscription.State == a2a888.SubscriptionState_SUBSCRIPTION_STATE_SUSPENDED || subscription.State == a2a888.SubscriptionState_SUBSCRIPTION_STATE_CANCELLED || (subscription.State == a2a888.SubscriptionState_SUBSCRIPTION_STATE_GRACE && strings.EqualFold(subscription.GracePolicy, "READ_ONLY")))
	return connect.NewResponse(&a2a888.GetUsageSummaryResponse{Aggregates: aggregates, Entitlements: entitlements, Subscription: subscription, ReadOnly: readOnly}), nil
}

func canViewOrganizationUsage(role a2a888.OrganizationRole) bool {
	return role == a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER || role == a2a888.OrganizationRole_ORGANIZATION_ROLE_BILLING_ADMIN
}
