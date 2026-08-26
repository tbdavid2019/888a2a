// Package auth handles the auth of gRPC server.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	errs "github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	v1pb "github.com/tbdavid2019/888a2a/backend/generated-go/v1"
	"github.com/tbdavid2019/888a2a/backend/manager/config"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

const (
	issuer       = "888a2a"
	legacyIssuer = "lae" + "lia"
	keyID        = "v1"

	AccessTokenAudienceFmt              = "888a2a.user.access.%s"
	LegacyAccessTokenAudienceFmt        = "ll.user.access.%s"
	AgentAccessTokenAudienceFmt         = "888a2a.agent.access.%s"
	LegacyAgentAccessTokenAudienceFmt   = "ll.agent.access.%s"
	MachineAccessTokenAudienceFmt       = "888a2a.machine.access.%s"
	LegacyMachineAccessTokenAudienceFmt = "ll.machine.access.%s"

	apiTokenDuration     = 1 * time.Hour
	DefaultTokenDuration = 7 * 24 * time.Hour

	AccessTokenCookieName       = "888a2a-access-token"
	LegacyAccessTokenCookieName = "access-token"

	// DeclaredAgentHeader is the HTTP header a machine app sets on
	// agent-callable RPCs to declare which agent it is
	// acting on behalf of (agents/{agent}). A machine authenticates once with
	// its access token; per-agent identity is carried per-request by this
	// header. The auth interceptor resolves it, verifies the machine owns the
	// agent (agent.machine_id == machine.id), and injects the agent under
	// AgentContextKey so existing handlers resolve the caller unchanged.
	DeclaredAgentHeader       = "X-888a2a-Agent"
	LegacyDeclaredAgentHeader = "X-" + "Lae" + "lia-Agent"

	TokenTypeBootstrap = "BOOTSTRAP"
	TokenTypeAccess    = "ACCESS"
	TokenTypeRefresh   = "REFRESH"
)

// UserStore is the subset of *store.Store needed to authenticate user tokens.
type UserStore interface {
	GetUserByID(ctx context.Context, id int) (*store.UserMessage, error)
}

// AgentStore is the subset of *store.Store needed to authenticate agent tokens
// and resolve declared agents.
type AgentStore interface {
	GetAgentByResourceID(ctx context.Context, resourceID string) (*store.AgentMessage, error)
}

// MachineStore is the subset of *store.Store needed to authenticate machine tokens.
type MachineStore interface {
	GetMachineByResourceID(ctx context.Context, resourceID string) (*store.MachineMessage, error)
}

// Store is the auth package's view of the manager store. Keeping it to the
// three lookups above (instead of *store.Store) lets tests substitute a small
// fake without mocking the whole store.
type Store interface {
	UserStore
	AgentStore
	MachineStore
}

// organizationMembershipStore is an optional capability implemented by the
// production store. Keeping it optional preserves the deliberately small auth
// test double while allowing production to validate an explicitly selected
// organization before handing control to a handler.
type organizationMembershipStore interface {
	GetMembership(context.Context, string, int) (*a2a888.OrganizationMembership, error)
}

// TokenExpireCache is the subset of *state.State used to reject revoked/expired
// tokens before JWT verification.
type TokenExpireCache interface {
	Get(key string) (bool, bool)
}

// APIAuthInterceptor is the auth interceptor for gRPC server.
type APIAuthInterceptor struct {
	store            Store
	tokenExpireCache TokenExpireCache
	secret           string
	profile          *config.Profile
}

// New returns a new API auth interceptor.
func New(
	store Store,
	secret string,
	tokenExpireCache TokenExpireCache,
	profile *config.Profile,
) *APIAuthInterceptor {
	return &APIAuthInterceptor{
		store:            store,
		tokenExpireCache: tokenExpireCache,
		secret:           secret,
		profile:          profile,
	}
}

type authResult struct {
	user                 *store.UserMessage
	agent                *store.AgentMessage
	machine              *store.MachineMessage
	accessTokenExpiresAt int64
}

// WrapUnary implements the ConnectRPC interceptor interface for unary RPCs.
func (in *APIAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := in.authenticate(ctx, req.Header(), req.Peer(), req.Spec().Procedure)
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient implements the ConnectRPC interceptor interface for streaming clients.
func (*APIAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler implements the ConnectRPC interceptor interface for streaming handlers.
func (in *APIAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := in.authenticate(ctx, conn.RequestHeader(), conn.Peer(), conn.Spec().Procedure)
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// authenticate performs the shared Connect RPC authentication flow: it
// extracts the source IP, reads the bearer token, resolves the per-procedure
// auth context, authenticates the caller, and injects the resolved identity
// into the returned context. Unary and streaming handlers only adapt the
// result to their respective next functions.
func (in *APIAuthInterceptor) authenticate(
	ctx context.Context,
	header http.Header,
	peer connect.Peer,
	procedure string,
) (context.Context, error) {
	sourceIP := extractSourceIP(header, peerRemoteAddr(peer), in.profile.TrustProxy)
	ctx = context.WithValue(ctx, common.SourceIPContextKey, sourceIP)

	accessTokenStr, err := GetTokenFromHeaders(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	authContext, err := getAuthContext(procedure)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, common.AuthContextKey, authContext)

	result, err := in.getUserOrAgentConnect(ctx, accessTokenStr)
	if err != nil {
		if IsAuthenticationAllowed(procedure, authContext, in.profile.Mode == common.ReleaseModeDev) {
			return ctx, nil
		}
		return nil, err
	}

	return in.injectAuthResult(ctx, result, header)
}

// injectAuthResult puts the authenticated user/agent/machine (and, for a
// machine caller, the declared agent) into ctx. It is shared by the Connect
// interceptors and AuthenticateHTTP.
func (in *APIAuthInterceptor) injectAuthResult(
	ctx context.Context,
	result *authResult,
	header http.Header,
) (context.Context, error) {
	if result.user != nil {
		ctx = context.WithValue(ctx, common.UserContextKey, result.user)
	}
	if result.agent != nil {
		ctx = context.WithValue(ctx, common.AgentContextKey, result.agent)
	}
	var declared *store.AgentMessage
	if result.machine != nil {
		ctx = context.WithValue(ctx, common.MachineContextKey, result.machine)
		// A machine may act on behalf of an agent declared via the
		// X-Laelia-Agent header; resolve + ownership-check it here so
		// existing agent-callable handlers see the agent via
		// GetAgentFromContext unchanged.
		var derr error
		if declared, derr = in.resolveDeclaredAgent(ctx, result.machine, header); derr != nil {
			return nil, derr
		} else if declared != nil {
			ctx = context.WithValue(ctx, common.AgentContextKey, declared)
		}
	}
	if result.accessTokenExpiresAt > 0 {
		ctx = context.WithValue(ctx, common.AccessTokenExpiresAtContextKey, result.accessTokenExpiresAt)
	}

	// Resolve and inject active organization/tenant ID
	orgID := header.Get("X-Organization-ID")
	if orgID == "" {
		orgID = header.Get("X-Tenant-ID")
	}
	if orgID == "" {
		if result.user != nil && result.user.DefaultOrganizationID != "" {
			orgID = result.user.DefaultOrganizationID
		} else if declared != nil && declared.OrganizationID != "" {
			orgID = declared.OrganizationID
		} else if result.agent != nil && result.agent.OrganizationID != "" {
			orgID = result.agent.OrganizationID
		} else if result.machine != nil && result.machine.OrganizationID != "" {
			orgID = result.machine.OrganizationID
		} else {
			orgID = "default"
		}
	}
	if err := in.validateOrganizationSelection(ctx, result, orgID); err != nil {
		return nil, err
	}
	ctx = common.SetOrganizationIDToContext(ctx, orgID)
	if workspaceID := header.Get("X-Workspace-ID"); workspaceID != "" {
		if err := in.validateWorkspaceSelection(ctx, result, orgID, workspaceID); err != nil {
			return nil, err
		}
		ctx = common.SetWorkspaceIDToContext(ctx, workspaceID)
	}
	ctx = injectPrincipalEvidence(ctx, result, declared, orgID)
	return ctx, nil
}

// validateOrganizationSelection prevents a caller from selecting an
// arbitrary tenant with X-Organization-ID. A user may select only an active
// membership; machine and agent credentials are bound to their own tenant.
// The membership check is optional solely for the narrow auth test interface;
// the production *store.Store implements it.
func (in *APIAuthInterceptor) validateOrganizationSelection(ctx context.Context, result *authResult, orgID string) error {
	if result == nil || orgID == "" {
		return connect.NewError(connect.CodePermissionDenied, errs.New("organization access denied"))
	}
	if result.user != nil {
		membershipStore, ok := in.store.(organizationMembershipStore)
		if !ok {
			return nil
		}
		membership, err := membershipStore.GetMembership(ctx, orgID, result.user.ID)
		if err != nil || membership == nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
			return connect.NewError(connect.CodePermissionDenied, errs.New("organization access denied"))
		}
	}
	if result.agent != nil && result.agent.OrganizationID != "" && result.agent.OrganizationID != orgID {
		return connect.NewError(connect.CodePermissionDenied, errs.New("organization access denied"))
	}
	if result.machine != nil && result.machine.OrganizationID != "" && result.machine.OrganizationID != orgID {
		return connect.NewError(connect.CodePermissionDenied, errs.New("organization access denied"))
	}
	return nil
}

func (in *APIAuthInterceptor) validateWorkspaceSelection(ctx context.Context, result *authResult, organizationID, workspaceID string) error {
	if result == nil || workspaceID == "" {
		return connect.NewError(connect.CodePermissionDenied, errs.New("workspace access denied"))
	}
	if result.user != nil {
		membershipStore, ok := in.store.(organizationMembershipStore)
		if !ok {
			return nil
		}
		membership, err := membershipStore.GetMembership(ctx, organizationID, result.user.ID)
		if err != nil || membership == nil || !slices.Contains(membership.WorkspaceIds, workspaceID) {
			return connect.NewError(connect.CodePermissionDenied, errs.New("workspace access denied"))
		}
	}
	if result.agent != nil && result.agent.WorkspaceID != "" && result.agent.WorkspaceID != workspaceID {
		return connect.NewError(connect.CodePermissionDenied, errs.New("workspace access denied"))
	}
	return nil
}

func injectPrincipalEvidence(ctx context.Context, result *authResult, declared *store.AgentMessage, organizationID string) context.Context {
	if result == nil {
		return ctx
	}
	if result.user != nil {
		return common.SetRequesterPrincipalToContext(ctx, common.PrincipalIdentity{
			ID:             strconv.Itoa(result.user.ID),
			OrganizationID: organizationID,
			Type:           "human",
		})
	}
	if declared != nil {
		ctx = common.SetRequesterPrincipalToContext(ctx, common.PrincipalIdentity{
			ID:             strconv.Itoa(result.machine.ID),
			OrganizationID: result.machine.OrganizationID,
			Type:           "service_account",
		})
		return common.SetExecutorPrincipalToContext(ctx, common.PrincipalIdentity{
			ID:             declared.ResourceID,
			OrganizationID: declared.OrganizationID,
			Type:           "agent",
		})
	}
	if result.agent != nil {
		return common.SetExecutorPrincipalToContext(ctx, common.PrincipalIdentity{
			ID:             result.agent.ResourceID,
			OrganizationID: result.agent.OrganizationID,
			Type:           "agent",
		})
	}
	if result.machine != nil {
		return common.SetRequesterPrincipalToContext(ctx, common.PrincipalIdentity{
			ID:             result.machine.ResourceID,
			OrganizationID: result.machine.OrganizationID,
			Type:           "service_account",
		})
	}
	return ctx
}

// AuthenticateHTTP authenticates a plain HTTP request (used by browser-facing
// Echo routes that do not go through the Connect interceptor) using the same
// token/cookie rules as Connect RPCs. It returns a context carrying the
// caller's user/agent/machine identity so existing handlers can resolve it via
// apiv1.GetUserFromContext / GetAgentFromContext.
func (in *APIAuthInterceptor) AuthenticateHTTP(
	ctx context.Context,
	header http.Header,
	remoteAddr string,
) (context.Context, error) {
	sourceIP := extractSourceIP(header, remoteAddr, in.profile.TrustProxy)
	ctx = context.WithValue(ctx, common.SourceIPContextKey, sourceIP)

	accessTokenStr, err := GetTokenFromHeaders(header)
	if err != nil {
		return nil, err
	}

	result, err := in.getUserOrAgentConnect(ctx, accessTokenStr)
	if err != nil {
		return nil, err
	}

	return in.injectAuthResult(ctx, result, header)
}

// invalidTokenError reports an invalid bearer token. The token can be
// invalid without a jwt.ParseWithClaims error (untrusted claims or audience
// mismatch), so err may be nil; it is appended only when present.
func invalidTokenError(kind string, err error) error {
	if err == nil {
		return errs.New("invalid " + kind + " access token")
	}
	return errs.Errorf("invalid %s access token: %v", kind, err)
}

func (in *APIAuthInterceptor) getUserOrAgentConnect(ctx context.Context, accessTokenStr string) (*authResult, error) {
	if accessTokenStr == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("access token not found"))
	}
	if _, ok := in.tokenExpireCache.Get(accessTokenStr); ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("access token expired"))
	}

	keyFunc := func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, errs.Errorf("unexpected access token signing method=%v, expect %v", t.Header["alg"], jwt.SigningMethodHS256)
		}
		if kid, ok := t.Header["kid"].(string); ok {
			if kid == "v1" {
				return []byte(in.secret), nil
			}
		}
		return nil, errs.Errorf("unexpected access token kid=%v", t.Header["kid"])
	}

	// Branch on the audience so a request pays for exactly one signature
	// verification instead of three (user/agent/machine) parses. peekAudience
	// only decodes the unsigned payload to select the claims struct; the
	// signature is always verified below, and the audience is re-checked, so a
	// forged payload can only fall through to the generic invalid-token error.
	expected := []string{
		fmt.Sprintf(AgentAccessTokenAudienceFmt, in.profile.Mode),
		fmt.Sprintf(MachineAccessTokenAudienceFmt, in.profile.Mode),
		fmt.Sprintf(AccessTokenAudienceFmt, in.profile.Mode),
		fmt.Sprintf(LegacyAgentAccessTokenAudienceFmt, in.profile.Mode),
		fmt.Sprintf(LegacyMachineAccessTokenAudienceFmt, in.profile.Mode),
		fmt.Sprintf(LegacyAccessTokenAudienceFmt, in.profile.Mode),
	}
	kind := audienceKind(peekTokenAudience(accessTokenStr), expected)
	if kind < 0 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("invalid access token, audience mismatch, expected %q, %q or %q",
			expected[2], expected[0], expected[1]))
	}
	switch kind {
	case 0: // agent
		agentClaims := &agentClaimsMessage{}
		agentToken, err := jwt.ParseWithClaims(accessTokenStr, agentClaims, keyFunc)
		if err != nil || agentToken == nil || !agentToken.Valid || (!audienceContains(agentClaims.Audience, expected[0]) && !audienceContains(agentClaims.Audience, expected[3])) {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("access token expired"))
			}
			return nil, connect.NewError(connect.CodeUnauthenticated, invalidTokenError("agent", err))
		}
		agent, err := in.authenticateAgentByClaims(ctx, agentClaims)
		if err != nil {
			return nil, err
		}
		return &authResult{agent: agent, accessTokenExpiresAt: agentClaims.ExpiresAt.Unix()}, nil
	case 1: // machine
		machineClaims := &machineClaimsMessage{}
		machineToken, err := jwt.ParseWithClaims(accessTokenStr, machineClaims, keyFunc)
		if err != nil || machineToken == nil || !machineToken.Valid || (!audienceContains(machineClaims.Audience, expected[1]) && !audienceContains(machineClaims.Audience, expected[4])) {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("access token expired"))
			}
			return nil, connect.NewError(connect.CodeUnauthenticated, invalidTokenError("machine", err))
		}
		machine, err := in.authenticateMachineByClaims(ctx, machineClaims)
		if err != nil {
			return nil, err
		}
		return &authResult{machine: machine, accessTokenExpiresAt: machineClaims.ExpiresAt.Unix()}, nil
	case 2: // user
		userClaims := &claimsMessage{}
		userToken, err := jwt.ParseWithClaims(accessTokenStr, userClaims, keyFunc)
		if err != nil || userToken == nil || !userToken.Valid || (!audienceContains(userClaims.Audience, expected[2]) && !audienceContains(userClaims.Audience, expected[5])) {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("access token expired"))
			}
			return nil, connect.NewError(connect.CodeUnauthenticated, invalidTokenError("user", err))
		}
		user, err := in.authenticateUserByClaims(ctx, userClaims)
		if err != nil {
			return nil, err
		}
		return &authResult{user: user, accessTokenExpiresAt: userClaims.ExpiresAt.Unix()}, nil
	default:
		// Unreachable: audienceKind returns -1 only when no branch matches,
		// which is handled before the switch. Kept to satisfy the compiler.
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.New("invalid access token"))
	}
}

func (in *APIAuthInterceptor) authenticateUserByClaims(ctx context.Context, claims *claimsMessage) (*store.UserMessage, error) {
	principalID, err := strconv.Atoi(claims.Subject)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("malformed ID %s in the access token", claims.Subject))
	}
	user, err := in.store.GetUserByID(ctx, principalID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("failed to find user ID %d in the access token", principalID))
	}
	if user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("user ID %d not exists in the access token", principalID))
	}
	if user.MemberDeleted {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("user ID %d has been deactivated by administrators", user.ID))
	}

	in.profile.LastActiveTS.Store(time.Now().Unix())
	return user, nil
}

func (in *APIAuthInterceptor) authenticateAgentByClaims(ctx context.Context, claims *agentClaimsMessage) (*store.AgentMessage, error) {
	agent, err := in.store.GetAgentByResourceID(ctx, claims.Subject)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("failed to find agent %s", claims.Subject))
	}
	if agent == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("agent %s not exists", claims.Subject))
	}
	if agent.Deleted {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("agent %s has been deactivated", claims.Subject))
	}
	if agent.TokenVersion != claims.TokenVersion {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("agent token version mismatch"))
	}

	in.profile.LastActiveTS.Store(time.Now().Unix())
	return agent, nil
}

func (in *APIAuthInterceptor) authenticateMachineByClaims(ctx context.Context, claims *machineClaimsMessage) (*store.MachineMessage, error) {
	machine, err := in.store.GetMachineByResourceID(ctx, claims.Subject)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("failed to find machine %s", claims.Subject))
	}
	if machine == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("machine %s not exists", claims.Subject))
	}
	if machine.Deleted {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("machine %s has been deactivated", claims.Subject))
	}
	// Only ACCESS tokens may authenticate machine-side RPCs (MachineChannel,
	// MachineHeartbeat, agent-callable RPCs). REFRESH tokens are for
	// RefreshMachineToken only and BOOTSTRAP (registration) tokens are for
	// ConnectMachine's registration_token field — neither should be accepted as
	// a bearer access token, since all three share the same audience.
	if claims.TokenType != TokenTypeAccess {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("machine token type %q is not an access token", claims.TokenType))
	}
	if machine.TokenVersion != claims.TokenVersion {
		return nil, connect.NewError(connect.CodeUnauthenticated, errs.Errorf("machine token version mismatch"))
	}

	in.profile.LastActiveTS.Store(time.Now().Unix())
	return machine, nil
}

// resolveDeclaredAgent resolves the agent a machine caller is acting on behalf
// of, from the DeclaredAgentHeader (agents/{agent}). It verifies the machine
// owns the agent (agent.machine_id == machine.id) and that the agent is not
// deleted. Returns nil (no error) when the header is absent — the caller is a
// machine not acting on behalf of an agent (e.g. MachineHeartbeat). On a
// machine call to an agent-callable RPC the header is required, and the
// handler's GetAgentFromContext returning false yields Unauthenticated.
func (in *APIAuthInterceptor) resolveDeclaredAgent(ctx context.Context, machine *store.MachineMessage, headers http.Header) (*store.AgentMessage, error) {
	agentName := headers.Get(DeclaredAgentHeader)
	if agentName == "" {
		agentName = headers.Get(LegacyDeclaredAgentHeader)
	}
	if agentName == "" {
		return nil, nil
	}
	resourceID, err := common.GetAgentResourceID(agentName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errs.Wrapf(err, "invalid %s header", DeclaredAgentHeader))
	}
	agent, err := in.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errs.Errorf("failed to find declared agent %s", resourceID))
	}
	if agent == nil || agent.Deleted {
		return nil, connect.NewError(connect.CodePermissionDenied, errs.Errorf("declared agent %s not found", resourceID))
	}
	if agent.MachineID != machine.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, errs.Errorf("machine %s does not own agent %s", machine.ResourceID, resourceID))
	}
	return agent, nil
}

// GetTokenFromHeaders extracts the access token from HTTP headers for ConnectRPC.
func GetTokenFromHeaders(headers http.Header) (string, error) {
	// Check Authorization header first
	authHeader := headers.Get("Authorization")
	if authHeader != "" {
		authHeaderParts := strings.Fields(authHeader)
		if len(authHeaderParts) != 2 || strings.ToLower(authHeaderParts[0]) != "bearer" {
			return "", errs.Errorf("authorization header format must be Bearer {token}")
		}
		return authHeaderParts[1], nil
	}

	// Check HTTP cookies
	var accessToken string
	cookieHeaders := headers.Values("Cookie")
	for _, cookieHeader := range cookieHeaders {
		header := http.Header{}
		header.Add("Cookie", cookieHeader)
		request := http.Request{Header: header}
		if cookie, _ := request.Cookie(AccessTokenCookieName); cookie != nil {
			accessToken = cookie.Value
			break
		}
		if cookie, _ := request.Cookie(LegacyAccessTokenCookieName); cookie != nil {
			accessToken = cookie.Value
			break
		}
	}
	return accessToken, nil
}

// peekTokenAudience extracts the aud claim from a JWT payload WITHOUT
// verifying the signature. It only selects which claims struct to parse into
// (see getUserOrAgentConnect); the signature is always verified afterward and
// the audience re-checked, so a forged payload cannot lead to an unverified
// acceptance — at worst it falls through to the generic invalid-token error.
func peekTokenAudience(tokenStr string) jwt.ClaimStrings {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Audience jwt.ClaimStrings `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims.Audience
}

// audienceKind maps the token's audience to the branch index in
// getUserOrAgentConnect: 0=agent, 1=machine, 2=user, -1=none matched. The
// audience is part of the signed payload, so this selection is only as
// trustworthy as the verification that follows it.
func audienceKind(audience jwt.ClaimStrings, expected []string) int {
	for i, aud := range expected {
		if audienceContains(audience, aud) {
			return i % 3
		}
	}
	return -1
}

func audienceContains(audience jwt.ClaimStrings, token string) bool {
	for _, v := range audience {
		if v == token {
			return true
		}
	}
	return false
}

// extractSourceIP resolves the client source IP for a request.
//
// When trustProxy is true, the leftmost X-Forwarded-For entry (or X-Real-IP) is
// trusted — but only when the server sits behind a trusted reverse proxy that
// overwrites client-supplied values. Otherwise the raw TCP peer address
// (remoteAddr) is used, so the source IP is always populated for downstream IP
// allowlists and rate limiters. Client-supplied forwarding headers are ignored
// entirely when trustProxy is false, preventing spoofing.
func extractSourceIP(headers http.Header, remoteAddr string, trustProxy bool) string {
	if trustProxy {
		if xff := headers.Get("X-Forwarded-For"); xff != "" {
			ips := strings.SplitN(xff, ",", 2)
			return strings.TrimSpace(ips[0])
		}
		if xri := headers.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	return stripPort(remoteAddr)
}

// peerRemoteAddr returns the connect peer's remote address (the raw TCP
// "host:port" of the client), or "" when unavailable (e.g. an in-process call).
func peerRemoteAddr(peer connect.Peer) string {
	return peer.Addr
}

// stripPort removes the port from a "host:port" / "[host]:port" address,
// returning the host as-is when it is not an authority form.
func stripPort(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

type claimsMessage struct {
	Name string `json:"name"`
	jwt.RegisteredClaims
}

type agentClaimsMessage struct {
	Name         string `json:"name"`
	TokenVersion int    `json:"token_version"`
	TokenType    string `json:"token_type"`
	SessionID    string `json:"session_id,omitempty"`
	TokenFamily  string `json:"token_family,omitempty"`
	jwt.RegisteredClaims
}

// machineClaimsMessage mirrors agentClaimsMessage for machine tokens. A machine
// authenticates once with its access token; per-agent identity is declared
// in-stream (AgentChannel's AgentReady.agent_name), validated against
// agent.machine_id.
type machineClaimsMessage struct {
	Name         string `json:"name"`
	TokenVersion int    `json:"token_version"`
	TokenType    string `json:"token_type"`
	SessionID    string `json:"session_id,omitempty"`
	TokenFamily  string `json:"token_family,omitempty"`
	jwt.RegisteredClaims
}

// AgentClaims is the verified, exported view of an agent token's claims. It is
// returned by ParseAgentToken so callers outside the auth package (e.g. the
// refresh-token handler) can bind token_version / token_type to the operation
// without re-implementing signature verification.
type AgentClaims struct {
	Name         string
	Subject      string
	TokenVersion int
	TokenType    string
	SessionID    string
	TokenFamily  string
}

// ParseAgentToken parses an agent JWT and verifies its HS256 signature against
// secret. It does NOT enforce token_type or token_version — callers bind those
// to the operation (refresh expects REFRESH; the version must equal the
// agent's current TokenVersion). Verifying the signature here means a token
// minted with a tampered claim set or a rotated/different secret is rejected
// before the store is consulted, so a refresh token whose version was forged
// cannot silently "upgrade" to the current token_version via a hash lookup.
func ParseAgentToken(tokenStr string, secret string) (*AgentClaims, error) {
	claims := &agentClaimsMessage{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, errs.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		if kid, ok := t.Header["kid"].(string); ok && kid == keyID {
			return []byte(secret), nil
		}
		return nil, errs.Errorf("unexpected kid %v", t.Header["kid"])
	})
	if err != nil {
		return nil, errs.Wrap(err, "invalid agent token")
	}
	if !parsed.Valid {
		return nil, errs.New("agent token is invalid")
	}
	return &AgentClaims{
		Name:         claims.Name,
		Subject:      claims.Subject,
		TokenVersion: claims.TokenVersion,
		TokenType:    claims.TokenType,
		SessionID:    claims.SessionID,
		TokenFamily:  claims.TokenFamily,
	}, nil
}

// MachineClaims is the verified, exported view of a machine token's claims,
// parallel to AgentClaims. Returned by ParseMachineToken for the machine-side
// auth handlers (ConnectMachine / RefreshMachineToken).
type MachineClaims struct {
	Name         string
	Subject      string
	TokenVersion int
	TokenType    string
	SessionID    string
	TokenFamily  string
}

// ParseMachineToken parses a machine JWT and verifies its HS256 signature
// against secret. Like ParseAgentToken it does not enforce token_type or
// token_version — callers bind those to the operation.
func ParseMachineToken(tokenStr string, secret string) (*MachineClaims, error) {
	claims := &machineClaimsMessage{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, errs.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		if kid, ok := t.Header["kid"].(string); ok && kid == keyID {
			return []byte(secret), nil
		}
		return nil, errs.Errorf("unexpected kid %v", t.Header["kid"])
	})
	if err != nil {
		return nil, errs.Wrap(err, "invalid machine token")
	}
	if !parsed.Valid {
		return nil, errs.New("machine token is invalid")
	}
	return &MachineClaims{
		Name:         claims.Name,
		Subject:      claims.Subject,
		TokenVersion: claims.TokenVersion,
		TokenType:    claims.TokenType,
		SessionID:    claims.SessionID,
		TokenFamily:  claims.TokenFamily,
	}, nil
}

// GenerateAPIToken generates an API token.
func GenerateAPIToken(userName string, userID int, mode common.ReleaseMode, secret string) (string, error) {
	expirationTime := time.Now().Add(apiTokenDuration)
	return generateToken(userName, userID, fmt.Sprintf(AccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateAccessToken generates an access token for web.
func GenerateAccessToken(userName string, userID int, mode common.ReleaseMode, secret string, tokenDuration time.Duration) (string, error) {
	expirationTime := time.Now().Add(tokenDuration)
	return generateToken(userName, userID, fmt.Sprintf(AccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateAgentToken generates an agent token with the specified type and duration.
func GenerateAgentToken(agentName string, resourceID string, tokenVersion int, tokenType string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signAgentToken(agentName, resourceID, tokenVersion, tokenType, "", resourceID, fmt.Sprintf(AgentAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateAgentTokenWithFamily generates an agent token with a custom token family.
func GenerateAgentTokenWithFamily(agentName string, resourceID string, tokenVersion int, tokenType string, tokenFamily string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signAgentToken(agentName, resourceID, tokenVersion, tokenType, "", tokenFamily, fmt.Sprintf(AgentAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateAgentTokenWithSession generates an agent token with session ID.
func GenerateAgentTokenWithSession(agentName string, resourceID string, tokenVersion int, tokenType string, sessionID string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signAgentToken(agentName, resourceID, tokenVersion, tokenType, sessionID, resourceID, fmt.Sprintf(AgentAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateMachineToken generates a machine token with the specified type and duration.
func GenerateMachineToken(machineName string, resourceID string, tokenVersion int, tokenType string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signMachineToken(machineName, resourceID, tokenVersion, tokenType, "", resourceID, fmt.Sprintf(MachineAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateMachineTokenWithFamily generates a machine token with a custom token family.
func GenerateMachineTokenWithFamily(machineName string, resourceID string, tokenVersion int, tokenType string, tokenFamily string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signMachineToken(machineName, resourceID, tokenVersion, tokenType, "", tokenFamily, fmt.Sprintf(MachineAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

// GenerateMachineTokenWithSession generates a machine token with session ID.
func GenerateMachineTokenWithSession(machineName string, resourceID string, tokenVersion int, tokenType string, sessionID string, mode common.ReleaseMode, secret string, duration time.Duration) (string, error) {
	expirationTime := time.Now().Add(duration)
	return signMachineToken(machineName, resourceID, tokenVersion, tokenType, sessionID, resourceID, fmt.Sprintf(MachineAccessTokenAudienceFmt, mode), expirationTime, []byte(secret))
}

func signAgentToken(agentName string, resourceID string, tokenVersion int, tokenType string, sessionID string, tokenFamily string, aud string, expirationTime time.Time, secret []byte) (string, error) {
	claims := &agentClaimsMessage{
		Name:         agentName,
		TokenVersion: tokenVersion,
		TokenType:    tokenType,
		SessionID:    sessionID,
		TokenFamily:  tokenFamily,
		RegisteredClaims: jwt.RegisteredClaims{
			// jti makes every minted token unique. Without it, two tokens
			// minted for the same agent in the same second (e.g. a refresh
			// followed immediately by a connect) are byte-identical, hash to
			// the same idx_agent_token_hash, and violate the unique
			// constraint on insert.
			ID:        uuid.NewString(),
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
			Subject:   resourceID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func signMachineToken(machineName string, resourceID string, tokenVersion int, tokenType string, sessionID string, tokenFamily string, aud string, expirationTime time.Time, secret []byte) (string, error) {
	claims := &machineClaimsMessage{
		Name:         machineName,
		TokenVersion: tokenVersion,
		TokenType:    tokenType,
		SessionID:    sessionID,
		TokenFamily:  tokenFamily,
		RegisteredClaims: jwt.RegisteredClaims{
			// jti makes every minted token unique; see signAgentToken for why.
			ID:        uuid.NewString(),
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
			Subject:   resourceID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// Pay attention to this function. It holds the main JWT token generation logic.
func generateToken(userName string, userID int, aud string, expirationTime time.Time, secret []byte) (string, error) {
	// Create the JWT claims, which includes the username and expiry time.
	claims := &claimsMessage{
		Name: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{aud},
			// In JWT, the expiry time is expressed as unix milliseconds.
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
			Subject:   strconv.Itoa(userID),
		},
	}

	// Declare the token with the HS256 algorithm used for signing, and the claims.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = keyID

	// Create the JWT string.
	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// authContextCache memoizes getAuthContext per full method name. The
// descriptors backing it are registered once at startup from generated code
// and never mutate at runtime, so a successful result is valid for the process
// lifetime. Errors are not cached: they indicate a programming error (unknown
// method) that should surface loudly rather than be papered over.
var authContextCache sync.Map // map[string]*common.AuthContext

func getAuthContext(fullMethod string) (*common.AuthContext, error) {
	if cached, ok := authContextCache.Load(fullMethod); ok {
		ctx, ok := cached.(*common.AuthContext)
		if ok {
			return ctx, nil
		}
	}
	ctx, err := resolveAuthContext(fullMethod)
	if err != nil {
		return nil, err
	}
	authContextCache.Store(fullMethod, ctx)
	return ctx, nil
}

func resolveAuthContext(fullMethod string) (*common.AuthContext, error) {
	methodTokens := strings.Split(fullMethod, "/")
	if len(methodTokens) != 3 {
		return nil, errs.Errorf("invalid full method name %q", fullMethod)
	}
	rd, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(methodTokens[1]))
	if err != nil {
		return nil, errs.Wrapf(err, "invalid registry service descriptor, full method name %q", fullMethod)
	}
	sd, ok := rd.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, errs.Errorf("invalid service descriptor, full method name %q", fullMethod)
	}
	md, ok := sd.Methods().ByName(protoreflect.Name(methodTokens[2])).Options().(*descriptorpb.MethodOptions)
	if !ok {
		return nil, errs.Errorf("invalid method options, full method name %q", fullMethod)
	}
	allowWithoutCredentialAny := proto.GetExtension(md, v1pb.E_AllowWithoutCredential)
	allowWithoutCredential, ok := allowWithoutCredentialAny.(bool)
	if !ok {
		return nil, errs.Errorf("invalid allow without credential extension, full method name %q", fullMethod)
	}
	permissionAny := proto.GetExtension(md, v1pb.E_Permission)
	permission, ok := permissionAny.(string)
	if !ok {
		return nil, errs.Errorf("invalid permission extension, full method name %q", fullMethod)
	}
	authMethodAny := proto.GetExtension(md, v1pb.E_AuthMethod)
	am, ok := authMethodAny.(v1pb.AuthMethod)
	if !ok {
		return nil, errs.Errorf("invalid auth method extension, full method name %q", fullMethod)
	}
	var authMethod common.AuthMethod
	switch am {
	case v1pb.AuthMethod_AUTH_METHOD_UNSPECIFIED:
		authMethod = common.AuthMethodUnspecified
	case v1pb.AuthMethod_IAM:
		authMethod = common.AuthMethodIAM
	case v1pb.AuthMethod_CUSTOM:
		authMethod = common.AuthMethodCustom
	default:
		return nil, errs.Errorf("unknown auth method %v for full method name %q", am, fullMethod)
	}
	auditAny := proto.GetExtension(md, v1pb.E_Audit)
	audit, ok := auditAny.(bool)
	if !ok {
		return nil, errs.Errorf("invalid audit extension, full method name %q", fullMethod)
	}

	return &common.AuthContext{
		AllowWithoutCredential: allowWithoutCredential,
		Permission:             permission,
		AuthMethod:             authMethod,
		Audit:                  audit,
	}, nil
}
