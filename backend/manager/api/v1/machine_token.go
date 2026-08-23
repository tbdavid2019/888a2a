package v1

import (
	"context"
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

func (s *MachineService) RevokeMachineToken(ctx context.Context, req *connect.Request[v1pb.RevokeMachineTokenRequest]) (*connect.Response[v1pb.RevokeMachineTokenResponse], error) {
	resourceID, err := common.GetMachineResourceID(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	machine, err := s.store.GetMachineByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to get machine, error: %v", err))
	}
	if machine == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.Errorf("machine %s not found", resourceID))
	}
	user, _ := GetUserFromContext(ctx)
	if !isMachineAdmin(ctx, s.iam, user, machine) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the machine's creator or a workspace admin can revoke this machine's tokens"))
	}

	newTokenVersion := machine.TokenVersion + 1
	nowRotated := time.Now()
	if _, err := s.store.UpdateMachine(ctx, machine, &store.UpdateMachineMessage{
		TokenVersion:       &newTokenVersion,
		LastTokenRotatedAt: &nowRotated,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to update machine token version, error: %v", err))
	}

	if err := s.store.RevokeAllMachineTokens(ctx, machine.ID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke machine tokens, error: %v", err))
	}

	terminateReason := "token_revoked"
	if req.Msg.Reason != "" {
		terminateReason = req.Msg.Reason
	}
	if err := s.store.TerminateAllMachineSessions(ctx, machine.ID, terminateReason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to terminate machine sessions, error: %v", err))
	}
	if s.dispatcher != nil {
		s.dispatcher.UnregisterMachine(machine.ID)
	}

	return connect.NewResponse(&v1pb.RevokeMachineTokenResponse{}), nil
}
func (s *MachineService) RefreshMachineToken(ctx context.Context, req *connect.Request[v1pb.RefreshMachineTokenRequest]) (*connect.Response[v1pb.RefreshMachineTokenResponse], error) {
	refreshTokenStr := req.Msg.RefreshToken
	if refreshTokenStr == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("refresh token is required"))
	}

	claims, err := auth.ParseMachineToken(refreshTokenStr, s.secret)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.Wrap(err, "invalid refresh token"))
	}
	if claims.TokenType != auth.TokenTypeRefresh {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.Errorf("expected refresh token, got %s", claims.TokenType))
	}

	tokenHash := hashToken(refreshTokenStr)
	storedToken, err := s.store.GetMachineTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to look up refresh token, error: %v", err))
	}
	if storedToken == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}

	switch action := machineRefreshReuseAction(storedToken.State); action {
	case refreshActionProceed:
	case refreshActionRevokeFamily:
		// A CONSUMED or REVOKED refresh token being presented again. The
		// multi-use flow never consumes a refresh token, so reaching here means
		// either an artifact of the old single-use flow, an admin
		// Revoke/RotateMachineToken, or genuine theft — revoke the family and
		// reject in all cases.
		if err := s.store.RevokeMachineTokenFamily(ctx, storedToken.TokenFamily); err != nil {
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

	machine, err := s.store.GetMachine(ctx, storedToken.MachineID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to get machine, error: %v", err))
	}
	if machine == nil || machine.Deleted {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine not found or deactivated"))
	}
	if claims.TokenVersion != machine.TokenVersion {
		if err := s.store.RevokeMachineTokenFamily(ctx, storedToken.TokenFamily); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to revoke stale token family, error: %v", err))
		}
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("refresh token version mismatch"))
	}

	accessToken, err := auth.GenerateMachineTokenWithSession(machine.Name, machine.ResourceID, machine.TokenVersion, auth.TokenTypeAccess, "", s.profile.Mode, s.secret, accessTokenDuration)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to generate access token, error: %v", err))
	}

	// Multi-use: reuse the same refresh token across reconnects. Mint a
	// replacement only when the current one is within the rolling-renewal
	// window of expiry, so the credential self-renews without manual rotation
	// and the machine never dies from expiry as long as it reconnects within
	// the window. The old token is left ACTIVE and expires on its own within
	// the window (a pre-renewal thief is bounded by it); it is not consumed, so
	// a lost renewal response is safely retryable.
	newRefreshToken := ""
	if time.Until(storedToken.ExpiresAt) < machineRefreshRotateWindow {
		newRefreshToken, err = auth.GenerateMachineTokenWithSession(machine.Name, machine.ResourceID, machine.TokenVersion, auth.TokenTypeRefresh, "", s.profile.Mode, s.secret, machineRefreshTokenDuration)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to generate refresh token, error: %v", err))
		}
		if err := s.store.CreateMachineToken(ctx, &store.MachineTokenMessage{
			MachineID:   machine.ID,
			TokenHash:   hashToken(newRefreshToken),
			TokenType:   storepb.MachineTokenType_MACHINE_REFRESH,
			TokenFamily: storedToken.TokenFamily,
			State:       storepb.MachineTokenState_MACHINE_TOKEN_ACTIVE,
			Fingerprint: req.Msg.Fingerprint,
			ExpiresAt:   time.Now().Add(machineRefreshTokenDuration),
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Errorf("failed to store new refresh token, error: %v", err))
		}
	}

	return connect.NewResponse(&v1pb.RefreshMachineTokenResponse{
		AccessToken:          accessToken,
		RefreshToken:         newRefreshToken,
		AccessTokenExpiresAt: timestamppb.New(time.Now().Add(accessTokenDuration)),
	}), nil
}

func machineRefreshReuseAction(state storepb.MachineTokenState) refreshAction {
	switch state {
	case storepb.MachineTokenState_MACHINE_TOKEN_ACTIVE:
		return refreshActionProceed
	case storepb.MachineTokenState_MACHINE_TOKEN_CONSUMED, storepb.MachineTokenState_MACHINE_TOKEN_REVOKED:
		return refreshActionRevokeFamily
	default:
		return refreshActionInvalid
	}
}
