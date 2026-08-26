package v1

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/common/permission"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/component/iam"
	"github.com/tbdavid2019/888a2a/backend/manager/migration"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func requireOrganizationIntegrationStore(t *testing.T) *store.Store {
	t.Helper()
	if os.Getenv("A2A888_RUN_MIGRATION_TESTS") != "1" {
		t.Skip("set A2A888_RUN_MIGRATION_TESTS=1 to run Organization tenancy integration tests")
	}
	rootURL := os.Getenv("A2A888_TEST_PG_URL")
	if rootURL == "" {
		t.Skip("set A2A888_TEST_PG_URL to a PostgreSQL URL")
	}
	root, err := sql.Open("pgx", rootURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	dbName := fmt.Sprintf("a2a888_tenant_%d", time.Now().UnixNano())
	if _, err := root.ExecContext(context.Background(), `CREATE DATABASE "`+dbName+`"`); err != nil {
		t.Skipf("test user cannot create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = root.ExecContext(context.Background(), `DROP DATABASE IF EXISTS "`+dbName+`" WITH (FORCE)`)
	})
	databaseURL, err := url.Parse(rootURL)
	require.NoError(t, err)
	databaseURL.Path = "/" + dbName
	db, err := sql.Open("pgx", databaseURL.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, migration.MigrateSchema(context.Background(), db))
	services, err := store.New(context.Background(), databaseURL.String(), true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = services.Close() })
	return services
}

func TestOrganizationTenancyIsolationAndLifecycle(t *testing.T) {
	services := requireOrganizationIntegrationStore(t)
	ctx := context.Background()
	orgStore := store.NewOrganizationStore(services.GetDB())
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orgAID, orgBID := "tenant-a-"+suffix, "tenant-b-"+suffix
	wsAID, wsBID := "workspace-a-"+suffix, "workspace-b-"+suffix

	_, err := orgStore.CreateOrganization(ctx, &a2a888.Organization{Id: orgAID, Name: "Tenant A", Slug: "tenant-a-" + suffix})
	require.NoError(t, err)
	_, err = orgStore.CreateOrganization(ctx, &a2a888.Organization{Id: orgBID, Name: "Tenant B", Slug: "tenant-b-" + suffix})
	require.NoError(t, err)
	_, err = orgStore.CreateWorkspace(ctx, &a2a888.Workspace{Id: wsAID, OrganizationId: orgAID, Name: "Workspace A", Slug: "workspace-a-" + suffix, IsDefault: true})
	require.NoError(t, err)
	_, err = orgStore.CreateWorkspace(ctx, &a2a888.Workspace{Id: wsBID, OrganizationId: orgBID, Name: "Workspace B", Slug: "workspace-b-" + suffix, IsDefault: true})
	require.NoError(t, err)

	user, err := services.CreateUser(ctx, &store.UserMessage{
		Email: "tenant-user-" + suffix + "@example.test", Name: "Tenant User", Type: models.PrincipalType_END_USER,
		PasswordHash: "test", EmailVerifiedAt: timePtr(time.Now().UTC()),
	})
	require.NoError(t, err)
	serviceAccount, err := services.CreateUser(ctx, &store.UserMessage{
		Email: "tenant-service-" + suffix + "@example.test", Name: "Tenant Service", Type: models.PrincipalType_SERVICE_ACCOUNT,
		PasswordHash: "test", EmailVerifiedAt: timePtr(time.Now().UTC()),
	})
	require.NoError(t, err)
	for _, membership := range []*a2a888.OrganizationMembership{
		{OrganizationId: orgAID, PrincipalId: fmt.Sprint(user.ID), Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER, State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, WorkspaceIds: []string{wsAID}},
		{OrganizationId: orgBID, PrincipalId: fmt.Sprint(user.ID), Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER, State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, WorkspaceIds: []string{wsBID}},
		{OrganizationId: orgAID, PrincipalId: fmt.Sprint(serviceAccount.ID), Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER, State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, WorkspaceIds: []string{wsAID}},
	} {
		_, err = orgStore.AddMembership(ctx, membership)
		require.NoError(t, err)
	}
	require.NoError(t, services.SetDefaultOrganizationForPrincipal(ctx, user.ID, orgAID))
	user, err = services.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, orgAID, user.DefaultOrganizationID)

	// 2.5: the real OrganizationService switches the authenticated human and
	// persists the selected tenant for the next request.
	authCtx := context.WithValue(ctx, common.UserContextKey, user)
	service := NewOrganizationService(services, iam.NewManager(services))
	listResponse, err := service.ListOrganizations(authCtx, connect.NewRequest(&a2a888.ListOrganizationsRequest{}))
	require.NoError(t, err)
	require.Len(t, listResponse.Msg.Organizations, 2)
	require.Equal(t, orgAID, listResponse.Msg.ActiveOrganizationId)
	switchResponse, err := service.SwitchOrganization(authCtx, connect.NewRequest(&a2a888.SwitchOrganizationRequest{OrganizationId: orgBID}))
	require.NoError(t, err)
	require.Equal(t, orgBID, switchResponse.Msg.Organization.Id)
	refreshed, err := services.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, orgBID, refreshed.DefaultOrganizationID)

	// 2.7: group grants are evaluated live, and removal takes effect without
	// copying permissions into the membership row.
	ctxA := common.SetOrganizationIDToContext(ctx, orgAID)
	group, err := services.CreateGroup(ctxA, &store.GroupMessage{
		OrganizationID: orgAID, Title: "Tenant A operators",
		Payload: &models.GroupPayload{Members: []*models.GroupMember{{Member: user.GetResourceID()}}},
	})
	require.NoError(t, err)
	_, err = orgStore.SetGroupBinding(ctx, &a2a888.OrganizationGroupBinding{OrganizationId: orgAID, GroupId: group.ID, Role: "roles/workspaceAdmin"})
	require.NoError(t, err)
	iamManager := iam.NewManager(services)
	allowed, err := iamManager.CheckTenantPermission(ctxA, orgAID, permission.AgentsEdit, user.ID)
	require.NoError(t, err)
	require.True(t, allowed)
	_, err = services.UpdateGroup(ctxA, group.ID, &store.UpdateGroupMessage{Payload: &models.GroupPayload{}})
	require.NoError(t, err)
	allowed, err = iamManager.CheckTenantPermission(ctxA, orgAID, permission.AgentsEdit, user.ID)
	require.NoError(t, err)
	require.False(t, allowed)
	err = orgStore.UpdateMembership(ctx, &a2a888.OrganizationMembership{
		OrganizationId: orgAID, PrincipalId: fmt.Sprint(user.ID),
		Role:  a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER,
		State: a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED, WorkspaceIds: []string{wsAID},
	})
	require.NoError(t, err)
	allowed, err = iamManager.CheckTenantPermission(ctxA, orgAID, permission.AgentsEdit, user.ID)
	require.NoError(t, err)
	require.False(t, allowed)
	membership, err := orgStore.GetMembership(ctx, orgAID, user.ID)
	require.NoError(t, err)
	require.Equal(t, a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED, membership.State)
	err = orgStore.UpdateMembership(ctx, &a2a888.OrganizationMembership{
		OrganizationId: orgAID, PrincipalId: fmt.Sprint(user.ID),
		Role:  a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER,
		State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, WorkspaceIds: []string{wsAID},
	})
	require.NoError(t, err)

	// 2.6 and 2.8: requester/executor evidence is tenant-scoped, and a resource
	// with the same identifier cannot be resolved from the other organization.
	agent, err := services.CreateAgent(ctxA, &store.AgentMessage{
		Name: "Tenant A agent", CreatedBy: user.ID, OwnerID: user.ID, OrganizationID: orgAID, WorkspaceID: wsAID, TokenVersion: 1,
	})
	require.NoError(t, err)
	gotAgent, err := services.GetAgentByResourceID(ctxA, agent.ResourceID)
	require.NoError(t, err)
	require.NotNil(t, gotAgent)
	gotAgent, err = services.GetAgentByResourceID(common.SetOrganizationIDToContext(ctx, orgBID), agent.ResourceID)
	require.NoError(t, err)
	require.Nil(t, gotAgent)
	denied, err := iamManager.CheckPermission(common.SetOrganizationIDToContext(ctx, orgBID), permission.AgentsEdit, user, nil, &iam.ResourceRef{ResourceType: models.Policy_AGENT, Name: "agents/" + agent.ResourceID})
	require.NoError(t, err)
	require.False(t, denied)

	require.NoError(t, services.CreateAuditLogs(ctx, []*store.AuditLogMessage{
		{OrganizationID: orgAID, RequesterID: fmt.Sprintf("users/%s", user.Handle), ExecutorID: "agents/" + agent.ResourceID, Method: "Delegate", ActorType: "USER", ActorID: user.Handle, Status: "ok", Resource: "agents/" + agent.ResourceID, CreatedAt: time.Now().UTC()},
		{OrganizationID: orgBID, RequesterID: fmt.Sprintf("users/%s", user.Handle), ExecutorID: "agents/other", Method: "Delegate", ActorType: "USER", ActorID: user.Handle, Status: "ok", Resource: "agents/other", CreatedAt: time.Now().UTC()},
	}))
	auditA, err := services.ListAuditLogs(ctxA, &store.FindAuditLogMessage{OrderAsc: true})
	require.NoError(t, err)
	require.Len(t, auditA, 1)
	require.Equal(t, fmt.Sprintf("users/%s", user.Handle), auditA[0].RequesterID)
	require.Equal(t, "agents/"+agent.ResourceID, auditA[0].ExecutorID)

	// 2.9: identical local IDs remain distinct in both cache and projection
	// namespaces.
	require.NotEqual(t, store.TenantCacheKey(orgAID, "agent", agent.ResourceID), store.TenantCacheKey(orgBID, "agent", agent.ResourceID))
	require.NotEqual(t, store.TenantProjectionKey(orgAID, "conversation", "same"), store.TenantProjectionKey(orgBID, "conversation", "same"))

	// 2.10: a suspended Organization blocks human, connector, A2A, and runtime
	// writes through their actual store entry points.
	require.NoError(t, orgStore.UpdateOrganizationState(ctx, orgAID, a2a888.OrganizationState_ORGANIZATION_STATE_SUSPENDED))
	require.Error(t, services.RequireOrganizationActive(ctx, orgAID))
	_, err = services.CreateAgent(ctxA, &store.AgentMessage{Name: "blocked agent", CreatedBy: user.ID, OwnerID: user.ID, OrganizationID: orgAID, WorkspaceID: wsAID, TokenVersion: 1})
	require.Error(t, err)
	_, err = services.RecordConnectorInbox(ctx, store.ConnectorInboxEvent{OrganizationID: orgAID, InstallationID: "install", ExternalEventID: uuid.NewString(), ExternalEventType: "message", RawPayload: []byte(`{"ok":true}`)})
	require.Error(t, err)
	_, err = services.EnsureWorkContext(ctx, orgAID, "blocked-context", "")
	require.Error(t, err)
	err = services.CreateWork(ctx, &store.WorkMessage{TenantID: orgAID, WorkID: "blocked-work", A2ATaskID: "blocked-task", ContextID: "blocked-context", RequesterAgentID: agent.ResourceID, ExecutorAgentID: "peer", IdempotencyKey: "blocked-key"})
	require.Error(t, err)
	_, err = services.GetOrCreateDirectConversation(ctxA, agent.ID, user.ID)
	require.Error(t, err)
	err = services.CreateAgentSession(ctx, &store.AgentSessionMessage{SessionID: "blocked-session-" + suffix, AgentID: agent.ID, TokenFamily: "family", State: "ACTIVE", ConnectedAt: time.Now().UTC()})
	require.Error(t, err)

	// Closed is terminal and remains blocked as well.
	require.NoError(t, orgStore.UpdateOrganizationState(ctx, orgAID, a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED))
	active, err := iamManager.CheckOrganizationActive(ctx, orgAID)
	require.NoError(t, err)
	require.False(t, active)
}

func timePtr(value time.Time) *time.Time { return &value }

func TestOrganizationTenancyServiceDeniesUnknownAndNonMemberEqually(t *testing.T) {
	services := requireOrganizationIntegrationStore(t)
	ctx := context.Background()
	orgStore := store.NewOrganizationStore(services.GetDB())
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orgAID, orgBID := "deny-a-"+suffix, "deny-b-"+suffix
	for _, orgID := range []string{orgAID, orgBID} {
		_, err := orgStore.CreateOrganization(ctx, &a2a888.Organization{Id: orgID, Name: orgID, Slug: orgID})
		require.NoError(t, err)
	}
	user, err := services.CreateUser(ctx, &store.UserMessage{Email: "deny-" + suffix + "@example.test", Name: "Deny User", Type: models.PrincipalType_END_USER, PasswordHash: "test"})
	require.NoError(t, err)
	_, err = orgStore.AddMembership(ctx, &a2a888.OrganizationMembership{OrganizationId: orgBID, PrincipalId: fmt.Sprint(user.ID), Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER, State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE})
	require.NoError(t, err)
	service := NewOrganizationService(services, iam.NewManager(services))
	authCtx := context.WithValue(ctx, common.UserContextKey, user)
	_, memberErr := service.GetOrganization(authCtx, connect.NewRequest(&a2a888.GetOrganizationRequest{OrganizationId: orgAID}))
	_, unknownErr := service.GetOrganization(authCtx, connect.NewRequest(&a2a888.GetOrganizationRequest{OrganizationId: "not-a-real-org"}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(memberErr))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(unknownErr))
}
