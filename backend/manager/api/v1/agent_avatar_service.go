package v1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/common/log"
	v1pb "github.com/tbdavid2019/888a2a/backend/generated-go/v1"
	"github.com/tbdavid2019/888a2a/backend/manager/component/s3client"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// agentAvatarS3KeyPrefix is the S3 key namespace for agent avatar objects. Each
// avatar is stored at avatars/agents/{resource_id}/{content_hash}.{ext} so re-
// uploads with identical content reuse the key and updates produce a new
// cacheable key.
const agentAvatarS3KeyPrefix = "avatars/agents/"

// UploadAgentAvatar replaces an agent's avatar image. Requires laelia.agents.edit
// on the agent (enforced by the IAM interceptor). The bytes are stored in object storage and
// the agent's avatar_s3_key updated; the previous object (if any) is deleted.
func (s *AgentService) UploadAgentAvatar(ctx context.Context, req *connect.Request[v1pb.UploadAgentAvatarRequest]) (*connect.Response[v1pb.Agent], error) {
	resourceID, err := common.ParseAgentAvatarName(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Wrapf(err, "invalid avatar name"))
	}

	if int64(len(req.Msg.Data)) > MaxAvatarBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Errorf("avatar too large: max %d bytes", MaxAvatarBytes))
	}
	if len(req.Msg.Data) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("avatar data is required"))
	}

	mime := sniffMimeType(req.Msg.MimeType, req.Msg.Data)
	ext, ok := avatarExtensionFor(mime)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Errorf("unsupported avatar image type %q (allowed: png, jpeg, webp, gif)", mime))
	}

	objectStore, err := s.s3client.GetObjectStore(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	agent, err := s.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrapf(err, "failed to get agent"))
	}
	if agent == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.Errorf("agent %s not found", resourceID))
	}

	orgID, _ := common.GetOrganizationIDFromContext(ctx)
	if orgID == "" && agent.OrganizationID != "" {
		orgID = agent.OrganizationID
	}
	hash := sha256.Sum256(req.Msg.Data)
	contentHash := hex.EncodeToString(hash[:])
	rawKey := agentAvatarS3KeyPrefix + resourceID + "/" + contentHash + "." + ext
	newKey := s3client.TenantObjectKey(orgID, rawKey)

	if err := objectStore.Put(ctx, newKey, bytes.NewReader(req.Msg.Data), mime); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "object storage put failed"))
	}

	prevKey := agent.AvatarS3Key
	updated, err := s.store.UpdateAgent(ctx, agent, &store.UpdateAgentMessage{AvatarS3Key: &newKey})
	if err != nil {
		if delErr := deleteObject(ctx, objectStore, newKey); delErr != nil {
			slog.Warn("failed to clean up agent avatar after db update failure", log.WithError(delErr))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to persist agent avatar"))
	}

	if prevKey != "" && prevKey != newKey {
		if err := deleteObject(ctx, objectStore, prevKey); err != nil {
			slog.Warn("failed to delete previous agent avatar object", "key", prevKey, log.WithError(err))
		}
	}

	return connect.NewResponse(s.convertToAgent(ctx, updated, agentReachable(s.dispatcher, updated.ID, updated.MachineID))), nil
}

// DownloadAgentAvatar fetches an agent's avatar image bytes. Any authenticated user
// may download any agent's avatar (workspace-internal profile image).
func (s *AgentService) DownloadAgentAvatar(ctx context.Context, req *connect.Request[v1pb.DownloadAgentAvatarRequest]) (*connect.Response[v1pb.DownloadAgentAvatarResponse], error) {
	if _, ok := GetUserFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	resourceID, err := common.ParseAgentAvatarName(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Wrapf(err, "invalid avatar name"))
	}
	agent, err := s.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrapf(err, "failed to get agent"))
	}
	if agent == nil || agent.AvatarS3Key == "" {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("avatar not found"))
	}

	objectStore, err := s.s3client.GetObjectStore(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out, err := objectStore.Get(ctx, agent.AvatarS3Key)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "object storage get failed"))
	}
	defer out.Close()

	data, err := io.ReadAll(io.LimitReader(out, MaxAvatarBytes+1))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to read avatar body"))
	}

	hash := sha256.Sum256(data)
	return connect.NewResponse(&v1pb.DownloadAgentAvatarResponse{
		Data:     data,
		MimeType: sniffMimeType("", data),
		Etag:     hex.EncodeToString(hash[:]),
	}), nil
}

// DeleteAgentAvatar clears an agent's avatar, reverting to the pixel default.
// Requires laelia.agents.edit on the agent (enforced by the IAM interceptor).
func (s *AgentService) DeleteAgentAvatar(ctx context.Context, req *connect.Request[v1pb.DeleteAgentAvatarRequest]) (*connect.Response[v1pb.Agent], error) {
	resourceID, err := common.ParseAgentAvatarName(req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Wrapf(err, "invalid avatar name"))
	}
	agent, err := s.store.GetAgentByResourceID(ctx, resourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrapf(err, "failed to get agent"))
	}
	if agent == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.Errorf("agent %s not found", resourceID))
	}
	if agent.AvatarS3Key == "" {
		return connect.NewResponse(s.convertToAgent(ctx, agent, agentReachable(s.dispatcher, agent.ID, agent.MachineID))), nil
	}

	objectStore, err := s.s3client.GetObjectStore(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	prevKey := agent.AvatarS3Key
	emptyKey := ""
	updated, err := s.store.UpdateAgent(ctx, agent, &store.UpdateAgentMessage{AvatarS3Key: &emptyKey})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to clear agent avatar"))
	}
	if err := deleteObject(ctx, objectStore, prevKey); err != nil {
		slog.Warn("failed to delete agent avatar object", "key", prevKey, log.WithError(err))
	}

	return connect.NewResponse(s.convertToAgent(ctx, updated, agentReachable(s.dispatcher, updated.ID, updated.MachineID))), nil
}
