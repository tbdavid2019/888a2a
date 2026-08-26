package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/generated-go/v1/v1connect"
	"github.com/tbdavid2019/888a2a/backend/manager/config"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// fakeStore implements the auth.Store interface with just the three lookups
// the interceptor needs, demonstrating that tests no longer have to mock the
// whole *store.Store.
type fakeStore struct {
	user    *store.UserMessage
	agent   *store.AgentMessage
	machine *store.MachineMessage
}

func (f *fakeStore) GetUserByID(_ context.Context, _ int) (*store.UserMessage, error) {
	return f.user, nil
}

func (f *fakeStore) GetAgentByResourceID(_ context.Context, _ string) (*store.AgentMessage, error) {
	return f.agent, nil
}

func (f *fakeStore) GetMachineByResourceID(_ context.Context, _ string) (*store.MachineMessage, error) {
	return f.machine, nil
}

type fakeTokenExpireCache struct {
	expired map[string]bool
}

type membershipStoreFake struct {
	fakeStore
	allowed map[string]bool
}

var _ organizationMembershipStore = (*store.Store)(nil)

func (f *membershipStoreFake) GetMembership(_ context.Context, organizationID string, _ int) (*a2a888.OrganizationMembership, error) {
	if !f.allowed[organizationID] {
		return nil, store.ErrMembershipNotFound
	}
	return &a2a888.OrganizationMembership{
		OrganizationId: organizationID,
		State:          a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE,
	}, nil
}

func (f *fakeTokenExpireCache) Get(key string) (bool, bool) {
	value, ok := f.expired[key]
	return value, ok
}

func TestAuthenticateInjectsUserAndSourceIP(t *testing.T) {
	const secret = "test-secret"
	profile := &config.Profile{Mode: common.ReleaseModeDev}

	token, err := GenerateAccessToken("alice", 1, common.ReleaseModeDev, secret, time.Hour)
	require.NoError(t, err)

	in := New(
		&fakeStore{user: &store.UserMessage{ID: 1}},
		secret,
		&fakeTokenExpireCache{expired: map[string]bool{}},
		profile,
	)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	ctx, err := in.authenticate(
		context.Background(),
		header,
		connect.Peer{Addr: "203.0.113.7:1234"},
		"/"+v1connect.AgentServiceName+"/AgentHeartbeat",
	)
	require.NoError(t, err)

	assert.Equal(t, "203.0.113.7", ctx.Value(common.SourceIPContextKey))
	user, ok := ctx.Value(common.UserContextKey).(*store.UserMessage)
	require.True(t, ok)
	assert.Equal(t, 1, user.ID)
}

func TestAuthenticateRejectsExpiredCacheToken(t *testing.T) {
	const secret = "test-secret"
	profile := &config.Profile{Mode: common.ReleaseModeDev}

	token, err := GenerateAccessToken("alice", 1, common.ReleaseModeDev, secret, time.Hour)
	require.NoError(t, err)

	in := New(
		&fakeStore{user: &store.UserMessage{ID: 1}},
		secret,
		&fakeTokenExpireCache{expired: map[string]bool{token: true}},
		profile,
	)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	_, err = in.authenticate(
		context.Background(),
		header,
		connect.Peer{Addr: "203.0.113.7:1234"},
		"/"+v1connect.UserServiceName+"/GetUser",
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestAuthenticateRejectsUnmemberTenantSelection(t *testing.T) {
	const secret = "test-secret"
	profile := &config.Profile{Mode: common.ReleaseModeDev}
	token, err := GenerateAccessToken("alice", 1, common.ReleaseModeDev, secret, time.Hour)
	require.NoError(t, err)
	in := New(&membershipStoreFake{
		fakeStore: fakeStore{user: &store.UserMessage{ID: 1, DefaultOrganizationID: "org-b"}},
		allowed:   map[string]bool{"org-a": true},
	}, secret, &fakeTokenExpireCache{expired: map[string]bool{}}, profile)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Organization-ID", "org-b")
	_, err = in.authenticate(context.Background(), header, connect.Peer{}, "/"+v1connect.UserServiceName+"/GetUser")
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestAuthenticateInjectsTenantAndRequesterEvidence(t *testing.T) {
	const secret = "test-secret"
	profile := &config.Profile{Mode: common.ReleaseModeDev}
	token, err := GenerateAccessToken("alice", 1, common.ReleaseModeDev, secret, time.Hour)
	require.NoError(t, err)
	in := New(&membershipStoreFake{
		fakeStore: fakeStore{user: &store.UserMessage{ID: 1, DefaultOrganizationID: "org-b"}},
		allowed:   map[string]bool{"org-a": true},
	}, secret, &fakeTokenExpireCache{expired: map[string]bool{}}, profile)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Organization-ID", "org-a")
	ctx, err := in.authenticate(context.Background(), header, connect.Peer{}, "/"+v1connect.UserServiceName+"/GetUser")
	require.NoError(t, err)
	assert.Equal(t, "org-a", mustOrganizationID(ctx))
	requester, ok := common.GetRequesterPrincipalFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "1", requester.ID)
	assert.Equal(t, "human", requester.Type)
	assert.Equal(t, "org-a", requester.OrganizationID)
}

func mustOrganizationID(ctx context.Context) string {
	organizationID, _ := common.GetOrganizationIDFromContext(ctx)
	return organizationID
}

func TestValidateOrganizationSelectionRejectsAgentCrossTenant(t *testing.T) {
	in := &APIAuthInterceptor{}
	err := in.validateOrganizationSelection(context.Background(), &authResult{
		agent: &store.AgentMessage{OrganizationID: "org-a"},
	}, "org-b")
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestValidateWorkspaceSelectionRequiresMembershipBinding(t *testing.T) {
	in := New(&membershipStoreFake{
		fakeStore: fakeStore{user: &store.UserMessage{ID: 1}},
		allowed:   map[string]bool{"org-a": true},
	}, "secret", &fakeTokenExpireCache{expired: map[string]bool{}}, &config.Profile{Mode: common.ReleaseModeDev})
	result := &authResult{user: &store.UserMessage{ID: 1}}
	// The simple membership fake intentionally returns no workspace binding.
	if err := in.validateWorkspaceSelection(context.Background(), result, "org-a", "workspace-a"); err == nil {
		t.Fatal("workspace selection must require a membership workspace binding")
	}
}
