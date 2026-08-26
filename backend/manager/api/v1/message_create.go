package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	v1pb "github.com/tbdavid2019/888a2a/backend/generated-go/v1"
	"github.com/tbdavid2019/888a2a/backend/manager/component/messageplane"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// createMessageInput carries the sender-specific differences between the user
// SendMessage path and the agent PostMessage path, so both share one message
// creation pipeline.
type createMessageInput struct {
	convID      uuid.UUID
	content     string
	attachments []*v1pb.Attachment
	mentions    []*v1pb.Mention
	threadRoot  uuid.NullUUID
	asTask      bool
	commandID   uuid.NullUUID

	principalID     int
	principalName   string
	principalHandle string
	senderAgentID   sql.NullInt32
	role            int32
	senderType      int32

	// exceptAgentID excludes the posting agent from agent notifications; nil
	// notifies all agents.
	exceptAgentID *int
	// posterUserID is set for user senders so thread subscription can add the
	// posting user.
	posterUserID *int
}

// createMessage writes a chat message (or task message), bumps the
// conversation version, wakes agents/thread subscribers, and generates
// activity. It is the shared pipeline behind SendMessage (users) and
// PostMessage (agents).
func (s *CommandService) createMessage(ctx context.Context, in createMessageInput) (*store.ChatMessage, int64, error) {
	if mode := s.collaborationPathMode(ctx); mode == messageplane.PathModeMessagePlane && !in.asTask && !in.threadRoot.Valid && s.messagePlane != nil {
		return s.createMessageOnPlane(ctx, in)
	}
	var msg *store.ChatMessage
	var newVersion int64
	var err error

	if in.asTask {
		msg, newVersion, err = s.store.CreateTaskMessageBumpVersion(ctx, &store.ChatMessage{
			ConversationID:  in.convID,
			PrincipalID:     in.principalID,
			PrincipalName:   in.principalName,
			PrincipalHandle: in.principalHandle,
			Role:            in.role,
			Content:         in.content,
			SenderType:      in.senderType,
			Mentions:        in.mentions,
			Attachments:     in.attachments,
		})
	} else {
		msg, newVersion, err = s.store.CreateChatMessageBumpVersion(ctx, &store.ChatMessage{
			ConversationID:      in.convID,
			PrincipalID:         in.principalID,
			PrincipalName:       in.principalName,
			PrincipalHandle:     in.principalHandle,
			SenderAgentID:       in.senderAgentID,
			Role:                in.role,
			Content:             in.content,
			CommandID:           in.commandID,
			SenderType:          in.senderType,
			Mentions:            in.mentions,
			Attachments:         in.attachments,
			ThreadRootMessageID: in.threadRoot,
		})
	}
	if err != nil {
		return nil, 0, connect.NewError(connect.CodeInternal, errors.Wrapf(err, "failed to create message"))
	}

	if in.threadRoot.Valid {
		s.subscribeAndNotifyThread(ctx, in.convID, in.threadRoot.UUID, newVersion, in.mentions, in.exceptAgentID, in.posterUserID)
	} else {
		s.notifyConversationAgents(ctx, in.convID, newVersion, in.exceptAgentID)
	}

	// Generate per-user activity for this message (mention/task/reminder/thread).
	// Best-effort: failures are logged, never fatal.
	rootIsTask := in.asTask
	rootIsReminder := false
	if in.threadRoot.Valid {
		rootIsTask, rootIsReminder, err = s.store.RootMessageKinds(ctx, in.threadRoot.UUID)
		if err != nil {
			slog.Warn("failed to resolve thread root kinds for activity", "rootID", in.threadRoot.UUID, "error", err)
		}
	}
	s.store.GenerateActivityForMessage(msg, rootIsTask, rootIsReminder)

	if s.collaborationPathMode(ctx) == messageplane.PathModeDual && s.messagePlane != nil {
		if err := s.mirrorMessageToPlane(ctx, msg); err != nil {
			// Legacy remains authoritative during DUAL. Reconciliation repairs a
			// failed mirror, while the user request remains successful.
			slog.Warn("failed to mirror chat message to MessagePlane", "messageID", msg.ID, "error", err)
		}
	}

	return msg, newVersion, nil
}

func (s *CommandService) collaborationPathMode(ctx context.Context) messageplane.PathMode {
	if s.pathSelector == nil {
		return messageplane.PathModeLegacy
	}
	organizationID, ok := common.GetOrganizationIDFromContext(ctx)
	if !ok || organizationID == "" {
		organizationID = "default"
	}
	mode, err := s.pathSelector.Mode(ctx, organizationID)
	if err != nil {
		slog.Warn("failed to read collaboration rollout; using legacy path", "organizationID", organizationID, "error", err)
		return messageplane.PathModeLegacy
	}
	return mode
}

func (s *CommandService) planeMessagePayload(in createMessageInput) ([]byte, error) {
	payload := map[string]any{
		"content":          in.content,
		"mentions":         in.mentions,
		"attachments":      in.attachments,
		"principal_id":     in.principalID,
		"principal_name":   in.principalName,
		"principal_handle": in.principalHandle,
		"sender_type":      in.senderType,
	}
	return json.Marshal(payload)
}

func (s *CommandService) createMessageOnPlane(ctx context.Context, in createMessageInput) (*store.ChatMessage, int64, error) {
	organizationID := tenantIDForCommandContext(ctx)
	payload, err := s.planeMessagePayload(in)
	if err != nil {
		return nil, 0, connect.NewError(connect.CodeInternal, errors.Wrap(err, "encode MessagePlane payload"))
	}
	message, err := s.messagePlane.Append(ctx, messageplane.MessageInput{
		OrganizationID:  organizationID,
		ConversationID:  in.convID.String(),
		ClientMessageNo: uuid.NewString(),
		SenderID:        planeSenderID(in),
		Payload:         payload,
	})
	if err != nil {
		return nil, 0, connect.NewError(connect.CodeInternal, errors.Wrap(err, "append MessagePlane message"))
	}
	messageID, err := uuid.Parse(message.MessageID)
	if err != nil {
		return nil, 0, connect.NewError(connect.CodeInternal, errors.Wrap(err, "decode MessagePlane message id"))
	}
	msg := chatMessageFromPlane(message, messageID, in)
	newVersion := int64(message.MessageSeq)
	if in.threadRoot.Valid {
		s.subscribeAndNotifyThread(ctx, in.convID, in.threadRoot.UUID, newVersion, in.mentions, in.exceptAgentID, in.posterUserID)
	} else {
		s.notifyConversationAgents(ctx, in.convID, newVersion, in.exceptAgentID)
	}
	s.store.GenerateActivityForMessage(msg, false, false)
	return msg, newVersion, nil
}

func (s *CommandService) mirrorMessageToPlane(ctx context.Context, msg *store.ChatMessage) error {
	payload, err := json.Marshal(map[string]any{
		"content":          msg.Content,
		"mentions":         msg.Mentions,
		"attachments":      msg.Attachments,
		"principal_id":     msg.PrincipalID,
		"principal_name":   msg.PrincipalName,
		"principal_handle": msg.PrincipalHandle,
		"sender_type":      msg.SenderType,
	})
	if err != nil {
		return err
	}
	_, err = s.messagePlane.Append(ctx, messageplane.MessageInput{
		OrganizationID: tenantIDForCommandContext(ctx), ConversationID: msg.ConversationID.String(),
		ClientMessageNo: msg.ID.String(), SenderID: planeSenderIDForChatMessage(msg), Payload: payload,
	})
	return err
}

func tenantIDForCommandContext(ctx context.Context) string {
	if organizationID, ok := common.GetOrganizationIDFromContext(ctx); ok && organizationID != "" {
		return organizationID
	}
	return "default"
}

func planeSenderID(in createMessageInput) string {
	if in.principalHandle != "" {
		return in.principalHandle
	}
	return fmt.Sprintf("principal-%d", in.principalID)
}

func planeSenderIDForChatMessage(msg *store.ChatMessage) string {
	if msg.PrincipalHandle != "" {
		return msg.PrincipalHandle
	}
	return fmt.Sprintf("principal-%d", msg.PrincipalID)
}

func chatMessageFromPlane(message messageplane.Message, messageID uuid.UUID, in createMessageInput) *store.ChatMessage {
	return &store.ChatMessage{
		OrganizationID: message.OrganizationID, ID: messageID, ConversationID: in.convID,
		PrincipalID: in.principalID, PrincipalName: in.principalName, PrincipalHandle: in.principalHandle,
		Role: in.role, Content: in.content, SenderAgentID: in.senderAgentID, SenderType: in.senderType,
		Mentions: in.mentions, Attachments: in.attachments, CreatedAt: time.Now().UTC(), RoomVersion: int64(message.MessageSeq),
	}
}
