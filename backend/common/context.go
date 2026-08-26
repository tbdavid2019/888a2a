//nolint:revive
package common

import (
	"context"

	"google.golang.org/protobuf/types/known/anypb"
)

// ContextKey is the key type of context value.
type ContextKey int

const (
	// UserContextKey is the key name used to store user message in the context.
	UserContextKey ContextKey = iota
	AuthContextKey
	ServiceDataKey
	AgentContextKey
	MachineContextKey
	SessionContextKey
	SourceIPContextKey
	AccessTokenExpiresAtContextKey
	OrganizationContextKey
	WorkspaceContextKey
	RequesterPrincipalContextKey
	ExecutorPrincipalContextKey
)

type AuthMethod int

const (
	AuthMethodUnspecified AuthMethod = iota
	AuthMethodIAM
	AuthMethodCustom
)

type Resource struct {
	Type      string
	Name      string
	ProjectID string
	Workspace bool
}

type AuthContext struct {
	Audit                  bool
	AllowWithoutCredential bool
	Permission             string
	AuthMethod             AuthMethod
	Resources              []*Resource
}

// PrincipalIdentity is the immutable identity evidence captured for a
// tenant-scoped request. Requester and executor are intentionally separate so
// delegation cannot collapse into an ambiguous actor string in audit records.
type PrincipalIdentity struct {
	ID             string
	OrganizationID string
	Type           string
}

func GetAuthContextFromContext(ctx context.Context) (*AuthContext, bool) {
	authCtx, ok := ctx.Value(AuthContextKey).(*AuthContext)
	return authCtx, ok
}

func (c *AuthContext) HasWorkspaceResource() bool {
	for _, r := range c.Resources {
		if r.Workspace {
			return true
		}
	}
	return false
}

func (c *AuthContext) GetProjectResources() []string {
	projectIDMap := make(map[string]bool)
	for _, r := range c.Resources {
		if r.ProjectID != "" {
			projectIDMap[r.ProjectID] = true
		}
	}
	var projectIDs []string
	for projectID := range projectIDMap {
		projectIDs = append(projectIDs, projectID)
	}
	return projectIDs
}

// WithSetServiceData registers a callback that handlers use to attach
// request-scoped structured data (e.g. the binding deltas of a SetIamPolicy
// call) to the audit record the interceptor writes after the RPC returns.
func WithSetServiceData(ctx context.Context, setServiceData func(a *anypb.Any)) context.Context {
	return context.WithValue(ctx, ServiceDataKey, setServiceData)
}

// GetSetServiceDataFromContext returns the service-data callback registered by
// the audit interceptor, if any.
func GetSetServiceDataFromContext(ctx context.Context) (func(a *anypb.Any), bool) {
	setServiceData, ok := ctx.Value(ServiceDataKey).(func(*anypb.Any))
	return setServiceData, ok
}

func GetSessionIDFromContext(ctx context.Context) (string, bool) {
	sessionID, ok := ctx.Value(SessionContextKey).(string)
	return sessionID, ok
}

// SetSessionIDToContext attaches the authenticated device/session identity.
// User message cursors use it to keep read progress independent per device.
func SetSessionIDToContext(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionContextKey, sessionID)
}

func GetSourceIPFromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(SourceIPContextKey).(string)
	return ip, ok
}

func GetAccessTokenExpiresAtFromContext(ctx context.Context) (int64, bool) {
	exp, ok := ctx.Value(AccessTokenExpiresAtContextKey).(int64)
	return exp, ok
}

func GetOrganizationIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(OrganizationContextKey).(string)
	return orgID, ok
}

func SetOrganizationIDToContext(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, OrganizationContextKey, orgID)
}

func GetWorkspaceIDFromContext(ctx context.Context) (string, bool) {
	workspaceID, ok := ctx.Value(WorkspaceContextKey).(string)
	return workspaceID, ok
}

func SetWorkspaceIDToContext(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, WorkspaceContextKey, workspaceID)
}

func GetRequesterPrincipalFromContext(ctx context.Context) (PrincipalIdentity, bool) {
	identity, ok := ctx.Value(RequesterPrincipalContextKey).(PrincipalIdentity)
	return identity, ok
}

func SetRequesterPrincipalToContext(ctx context.Context, identity PrincipalIdentity) context.Context {
	return context.WithValue(ctx, RequesterPrincipalContextKey, identity)
}

func GetExecutorPrincipalFromContext(ctx context.Context) (PrincipalIdentity, bool) {
	identity, ok := ctx.Value(ExecutorPrincipalContextKey).(PrincipalIdentity)
	return identity, ok
}

func SetExecutorPrincipalToContext(ctx context.Context, identity PrincipalIdentity) context.Context {
	return context.WithValue(ctx, ExecutorPrincipalContextKey, identity)
}
