package a2a

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	GroupExtensionURI       = "https://github.com/tbdavid2019/888a2a/extensions/agent-groups/v1"
	MaxGroupNameLength      = 128
	MaxGroupMembers         = 32
	MaxGroupFanout          = 32
	MaxGroupHistoryPageSize = 100
)

type HubGroupState string

const (
	HubGroupStateActive   HubGroupState = "ACTIVE"
	HubGroupStateArchived HubGroupState = "ARCHIVED"
)

type HubGroupRole string

const (
	HubGroupRoleOwner  HubGroupRole = "OWNER"
	HubGroupRoleAdmin  HubGroupRole = "ADMIN"
	HubGroupRoleMember HubGroupRole = "MEMBER"
)

type HubMembershipState string

const (
	HubMembershipActive  HubMembershipState = "ACTIVE"
	HubMembershipLeft    HubMembershipState = "LEFT"
	HubMembershipRemoved HubMembershipState = "REMOVED"
)

type HubInvitationState string

const (
	HubInvitationPending  HubInvitationState = "PENDING"
	HubInvitationAccepted HubInvitationState = "ACCEPTED"
	HubInvitationDeclined HubInvitationState = "DECLINED"
	HubInvitationExpired  HubInvitationState = "EXPIRED"
	HubInvitationRevoked  HubInvitationState = "REVOKED"
)

type HubGroup struct {
	HubID        string        `json:"hubId"`
	GroupID      string        `json:"groupId"`
	Name         string        `json:"name"`
	State        HubGroupState `json:"state"`
	OwnerAgentID string        `json:"ownerAgentId"`
	CreatedAt    time.Time     `json:"createdAt"`
	ArchivedAt   *time.Time    `json:"archivedAt,omitempty"`
}

type HubGroupMember struct {
	HubID     string             `json:"hubId"`
	GroupID   string             `json:"groupId"`
	AgentID   string             `json:"agentId"`
	Role      HubGroupRole       `json:"role"`
	State     HubMembershipState `json:"state"`
	JoinedAt  time.Time          `json:"joinedAt"`
	LeftAt    *time.Time         `json:"leftAt,omitempty"`
	RemovedAt *time.Time         `json:"removedAt,omitempty"`
}

type HubGroupInvitation struct {
	ID             uint64             `json:"invitationId"`
	HubID          string             `json:"hubId"`
	GroupID        string             `json:"groupId"`
	InviterAgentID string             `json:"inviterAgentId"`
	InviteeAgentID string             `json:"inviteeAgentId"`
	State          HubInvitationState `json:"state"`
	CreatedAt      time.Time          `json:"createdAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	RespondedAt    *time.Time         `json:"respondedAt,omitempty"`
}

type HubCreateGroupInput struct {
	Name string `json:"name"`
}

type HubGroupMessageInput struct {
	ContextID      string `json:"contextId"`
	IdempotencyKey string `json:"idempotencyKey"`
	Message        string `json:"message"`
}

type HubInviteMemberInput struct {
	InviteeAgentID string `json:"inviteeAgentId"`
}

type HubGroupDeliverySummary struct {
	TargetAgentID string `json:"targetAgentId"`
	Sequence      uint64 `json:"sequence"`
	State         string `json:"state"`
}

type HubGroupMessage struct {
	ID             uint64                    `json:"groupMessageId"`
	HubID          string                    `json:"hubId"`
	GroupID        string                    `json:"groupId"`
	SenderAgentID  string                    `json:"senderAgentId"`
	ContextID      string                    `json:"contextId"`
	IdempotencyKey string                    `json:"idempotencyKey"`
	Message        string                    `json:"message"`
	Trust          string                    `json:"trust"`
	CreatedAt      time.Time                 `json:"createdAt"`
	Deliveries     []HubGroupDeliverySummary `json:"deliveries,omitempty"`
}

func (group HubGroup) IsActive() bool { return group.State == HubGroupStateActive }

func (member HubGroupMember) IsActive() bool {
	return member.State == HubMembershipActive
}

func (member HubGroupMember) CanManageMembers() bool {
	return member.IsActive() && (member.Role == HubGroupRoleOwner || member.Role == HubGroupRoleAdmin)
}

func GenerateGroupID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "grp-" + hex.EncodeToString(raw[:]), nil
}

func ValidateCreateGroup(input HubCreateGroupInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return errors.New("name must not be empty")
	}
	if len(name) > MaxGroupNameLength {
		return fmt.Errorf("name exceeds limit of %d characters", MaxGroupNameLength)
	}
	return nil
}

func ValidateGroupMessage(input HubGroupMessageInput, maxPayloadBytes int64) error {
	if strings.TrimSpace(input.ContextID) == "" {
		return errors.New("contextId must not be empty")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return errors.New("idempotencyKey must not be empty")
	}
	if len(input.IdempotencyKey) > MaxHubIdempotencyBytes {
		return fmt.Errorf("idempotencyKey exceeds limit of %d characters", MaxHubIdempotencyBytes)
	}
	if strings.TrimSpace(input.Message) == "" {
		return errors.New("message must not be empty")
	}
	if maxPayloadBytes > 0 && int64(len(input.Message)) > maxPayloadBytes {
		return fmt.Errorf("message exceeds payload limit of %d bytes", maxPayloadBytes)
	}
	return nil
}

type HubGroupStore interface {
	CreateGroup(ctx context.Context, group HubGroup) (HubGroup, error)
	FindGroup(ctx context.Context, groupID string) (HubGroup, error)
	ListGroups(ctx context.Context, agentID string) ([]HubGroup, error)
	FindMember(ctx context.Context, groupID, agentID string) (HubGroupMember, error)
	ListMembers(ctx context.Context, groupID string) ([]HubGroupMember, error)
	CreateInvitation(ctx context.Context, invitation HubGroupInvitation) (HubGroupInvitation, error)
	FindInvitation(ctx context.Context, id uint64) (HubGroupInvitation, error)
	ListInvitations(ctx context.Context, inviteeAgentID string) ([]HubGroupInvitation, error)
	AcceptInvitation(ctx context.Context, id uint64, agentID string, at time.Time) (HubGroupMember, error)
	DeclineInvitation(ctx context.Context, id uint64, agentID string, at time.Time) error
	RevokeInvitation(ctx context.Context, id uint64, inviterID string, at time.Time) error
	SendGroupMessage(ctx context.Context, message HubGroupMessage, maxFanout int) (HubGroupMessage, bool, error)
	ListGroupMessages(ctx context.Context, groupID, agentID string, afterID uint64, limit int) ([]HubGroupMessage, error)
	ArchiveGroup(ctx context.Context, groupID string, at time.Time) error
}
