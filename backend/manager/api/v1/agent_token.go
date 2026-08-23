package v1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"connectrpc.com/connect"
	"github.com/Ranxy/laelia/backend/common"
	storepb "github.com/Ranxy/laelia/backend/generated-go/store"
	v1pb "github.com/Ranxy/laelia/backend/generated-go/v1"
	"github.com/Ranxy/laelia/backend/manager/api/auth"
	"github.com/Ranxy/laelia/backend/manager/store"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *AgentService) RotateAgentToken(ctx context.Context, req *connect.Request[v1pb.RotateAgentTokenRequest]) (*connect.Response[v1pb.RotateAgentTokenResponse], error) {
	resourceID, err := common.GetAgentResourceID(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	agent, err := s.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to get agent, error: %v", err))
	}
	if agent == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.Errorf("agent %s not found", resourceID))
	}
	user, _ := GetUserFromContext(ctx)
	if !s.canEditAgent(ctx, user, agent) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the agent's owner or a holder of laelia.agents.edit can rotate this agent's token"))
	}

	newTokenVersion := agent.TokenVersion + 1
	newTokenFamily := fmt.Sprintf("%s:v%d", agent.ResourceID, newTokenVersion)

	bootstrapToken, err := auth.GenerateAgentTokenWithFamily(agent.Name, agent.ResourceID, newTokenVersion, auth.TokenTypeBootstrap, newTokenFamily, s.profile.Mode, s.secret, bootstrapTokenDuration)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to generate agent token, error: %v", err))
	}

	nowRotated := time.Now()
	if _, err := s.store.UpdateAgent(ctx, agent, &store.UpdateAgentMessage{
		TokenVersion:       &newTokenVersion,
		LastTokenRotatedAt: &nowRotated,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to update agent token version, error: %v", err))
	}

	if err := s.store.RevokeAllAgentTokens(ctx, agent.ID); err != nil {
		// Abort the rotation: leaving old tokens live while minting a new
		// bootstrap token means a previously-issued (possibly leaked) refresh
		// token would keep working under the old token_version until it
		// expired. Failing closed forces the admin to retry and keeps the
		// "rotation revokes everything" invariant intact. The version bump
		// above is contained: old refresh tokens embed the prior version and
		// RefreshAgentToken rejects version mismatches.
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke old tokens during rotation, error: %v", err))
	}

	tokenHash := hashToken(bootstrapToken)
	if err := s.store.CreateAgentToken(ctx, &store.AgentTokenMessage{
		AgentID:     agent.ID,
		TokenHash:   tokenHash,
		TokenType:   storepb.AgentTokenType_BOOTSTRAP,
		TokenFamily: newTokenFamily,
		State:       storepb.AgentTokenState_ACTIVE,
		ExpiresAt:   time.Now().Add(bootstrapTokenDuration),
		CreatedBy:   "system",
	}); err != nil {
		slog.Error("failed to store new token after rotation — agent has no valid bootstrap token", "agent", resourceID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to store new agent token, error: %v", err))
	}

	terminateReason := "token_rotated"
	if req.Msg.Reason != "" {
		terminateReason = req.Msg.Reason
	}
	if err := s.store.TerminateAllAgentSessions(ctx, agent.ID, terminateReason); err != nil {
		slog.Warn("failed to terminate agent sessions after rotation", "agent", resourceID, "error", err)
	}

	return connect.NewResponse(&v1pb.RotateAgentTokenResponse{
		BootstrapToken: bootstrapToken,
	}), nil
}

func (s *AgentService) RevokeAgentToken(ctx context.Context, req *connect.Request[v1pb.RevokeAgentTokenRequest]) (*connect.Response[v1pb.RevokeAgentTokenResponse], error) {
	resourceID, err := common.GetAgentResourceID(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	agent, err := s.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to get agent, error: %v", err))
	}
	if agent == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.Errorf("agent %s not found", resourceID))
	}
	user, _ := GetUserFromContext(ctx)
	if !s.canEditAgent(ctx, user, agent) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the agent's owner or a holder of laelia.agents.edit can revoke this agent's tokens"))
	}

	newTokenVersion := agent.TokenVersion + 1
	nowRotated := time.Now()
	if _, err := s.store.UpdateAgent(ctx, agent, &store.UpdateAgentMessage{
		TokenVersion:       &newTokenVersion,
		LastTokenRotatedAt: &nowRotated,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to update agent token version, error: %v", err))
	}

	if err := s.store.RevokeAllAgentTokens(ctx, agent.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke agent tokens, error: %v", err))
	}

	terminateReason := "token_revoked"
	if req.Msg.Reason != "" {
		terminateReason = req.Msg.Reason
	}
	if err := s.store.TerminateAllAgentSessions(ctx, agent.ID, terminateReason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to terminate agent sessions, error: %v", err))
	}

	return connect.NewResponse(&v1pb.RevokeAgentTokenResponse{}), nil
}

func (s *AgentService) RefreshAgentToken(ctx context.Context, req *connect.Request[v1pb.RefreshAgentTokenRequest]) (*connect.Response[v1pb.RefreshAgentTokenResponse], error) {
	refreshTokenStr := req.Msg.RefreshToken
	if refreshTokenStr == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("refresh token is required"))
	}

	// Verify the JWT signature before trusting any claim. Without this the
	// handler relied solely on a hash lookup: a refresh token whose
	// token_version was forged (or minted under a since-rotated secret) could
	// pass the hash check and be "upgraded" to the current token_version.
	claims, err := auth.ParseAgentToken(refreshTokenStr, s.secret)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.Wrap(err, "invalid refresh token"))
	}
	if claims.TokenType != auth.TokenTypeRefresh {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.Errorf("expected refresh token, got %s", claims.TokenType))
	}

	tokenHash := hashToken(refreshTokenStr)
	storedToken, err := s.store.GetAgentTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to look up refresh token, error: %v", err))
	}
	if storedToken == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}

	switch action := refreshReuseAction(storedToken.State); action {
	case refreshActionProceed:
		// Fresh token: proceed to rotate it below.
	case refreshActionRevokeFamily:
		// Reuse detected: a refresh token that was already exchanged (CONSUMED)
		// or revoked is being presented again. Revoke the entire family so
		// every token derived from the same bootstrap/rotation is invalidated,
		// then reject. Reusing a CONSUMED token previously only revoked the
		// single row and still issued a new token — i.e. it silently
		// succeeded, defeating reuse detection.
		if err := s.store.RevokeTokenFamily(ctx, storedToken.TokenFamily); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke token family, error: %v", err))
		}
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("refresh token reuse detected, token family revoked"))
	default:
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token state"))
	}

	if time.Now().After(storedToken.ExpiresAt) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("refresh token expired"))
	}

	if req.Msg.Fingerprint != "" && storedToken.Fingerprint != "" && req.Msg.Fingerprint != storedToken.Fingerprint {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("fingerprint mismatch, possible token theft detected"))
	}

	agent, err := s.store.GetAgent(ctx, storedToken.AgentID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to get agent, error: %v", err))
	}
	if agent == nil || agent.Deleted {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("agent not found or deactivated"))
	}

	// Bind the token's version to the agent's current version. After a
	// RotateAgentToken/RevokeAgentToken the version increments and the old
	// family is revoked; if a refresh token from the old family survived the
	// revoke (e.g. a partial failure), its embedded version no longer matches
	// and we reject rather than minting a new token under the current version.
	if claims.TokenVersion != agent.TokenVersion {
		if err := s.store.RevokeTokenFamily(ctx, storedToken.TokenFamily); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke stale token family, error: %v", err))
		}
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("refresh token version mismatch"))
	}

	if storedToken.State == storepb.AgentTokenState_ACTIVE {
		consumedAt := time.Now()
		if err := s.store.UpdateAgentTokenState(ctx, storedToken.ID, storepb.AgentTokenState_CONSUMED, &consumedAt); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to mark refresh token as consumed, error: %v", err))
		}
		s.scheduleTokenRevoke(storedToken.ID, storedToken.TokenFamily)
	}

	accessToken, err := auth.GenerateAgentTokenWithSession(agent.Name, agent.ResourceID, agent.TokenVersion, auth.TokenTypeAccess, "", s.profile.Mode, s.secret, accessTokenDuration)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to generate access token, error: %v", err))
	}

	newRefreshToken, err := auth.GenerateAgentTokenWithSession(agent.Name, agent.ResourceID, agent.TokenVersion, auth.TokenTypeRefresh, "", s.profile.Mode, s.secret, refreshTokenDuration)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to generate refresh token, error: %v", err))
	}

	newTokenHash := hashToken(newRefreshToken)
	if err := s.store.CreateAgentToken(ctx, &store.AgentTokenMessage{
		AgentID:     agent.ID,
		TokenHash:   newTokenHash,
		TokenType:   storepb.AgentTokenType_REFRESH,
		TokenFamily: storedToken.TokenFamily,
		State:       storepb.AgentTokenState_ACTIVE,
		Fingerprint: req.Msg.Fingerprint,
		ExpiresAt:   time.Now().Add(refreshTokenDuration),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to store new refresh token, error: %v", err))
	}

	return connect.NewResponse(&v1pb.RefreshAgentTokenResponse{
		AccessToken:          accessToken,
		RefreshToken:         newRefreshToken,
		AccessTokenExpiresAt: timestamppb.New(time.Now().Add(accessTokenDuration)),
	}), nil
}

func (s *AgentService) scheduleTokenRevoke(tokenID int, _ string) {
	timer := time.AfterFunc(refreshTokenReuseWindow, func() {
		if err := s.store.UpdateAgentTokenState(context.Background(), tokenID, storepb.AgentTokenState_REVOKED, nil); err != nil {
			slog.Error("failed to revoke consumed token", "token_id", tokenID, "error", err)
		}
		s.consumedMu.Lock()
		delete(s.consumedTimers, tokenID)
		s.consumedMu.Unlock()
	})
	s.consumedMu.Lock()
	s.consumedTimers[tokenID] = timer
	s.consumedMu.Unlock()
}

// refreshReuseAction is the pure decision for how RefreshAgentToken should
// treat a stored refresh token given its current state. Extracted so the
// reuse-detection matrix is unit-testable without a DB.
//
//   - ACTIVE        → proceed (rotate the token)
//   - CONSUMED/REVOKED → reuse: revoke the whole family and reject
//   - anything else → reject as invalid
//
// CONSUMED is treated as reuse (not as a valid second use) because a refresh
// token is single-use: once exchanged it must never be accepted again.
type refreshAction int

const (
	refreshActionProceed refreshAction = iota
	refreshActionRevokeFamily
	refreshActionInvalid
)

func refreshReuseAction(state storepb.AgentTokenState) refreshAction {
	switch state {
	case storepb.AgentTokenState_ACTIVE:
		return refreshActionProceed
	case storepb.AgentTokenState_CONSUMED, storepb.AgentTokenState_REVOKED:
		return refreshActionRevokeFamily
	default:
		return refreshActionInvalid
	}
}

func (*AgentService) Hello(_ context.Context, _ *connect.Request[v1pb.HelloRequest]) (*connect.Response[v1pb.HelloResponse], error) {
	return connect.NewResponse(&v1pb.HelloResponse{
		CurrentTime:   time.Now().Unix(),
		ServerVersion: "0.1.0",
	}), nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)[:length]
}
