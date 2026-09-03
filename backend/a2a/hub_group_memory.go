package a2a

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrHubGroupNotFound      = errors.New("Hub group not found")
	ErrHubGroupForbidden     = errors.New("operation not permitted for group")
	ErrHubGroupInvalidState  = errors.New("group state does not allow operation")
	ErrHubInvitationNotFound = errors.New("group invitation not found")
)

type MemoryHubGroupStore struct {
	mu          sync.Mutex
	groups      map[string]HubGroup
	members     map[string][]HubGroupMember
	invitations []HubGroupInvitation
	messages    []HubGroupMessage
	nextInvID   uint64
	nextMsgID   uint64
	mailbox     HubMailbox
}

func NewMemoryHubGroupStore(mailbox HubMailbox) *MemoryHubGroupStore {
	return &MemoryHubGroupStore{
		groups:      make(map[string]HubGroup),
		members:     make(map[string][]HubGroupMember),
		invitations: make([]HubGroupInvitation, 0),
		messages:    make([]HubGroupMessage, 0),
		nextInvID:   1,
		nextMsgID:   1,
		mailbox:     mailbox,
	}
}

func (s *MemoryHubGroupStore) CreateGroup(_ context.Context, group HubGroup) (HubGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if group.GroupID == "" {
		id, err := GenerateGroupID()
		if err != nil {
			return HubGroup{}, err
		}
		group.GroupID = id
	}
	if group.State == "" {
		group.State = HubGroupStateActive
	}
	if group.CreatedAt.IsZero() {
		group.CreatedAt = time.Now().UTC()
	}
	s.groups[group.GroupID] = group
	s.members[group.GroupID] = []HubGroupMember{
		{
			HubID:    group.HubID,
			GroupID:  group.GroupID,
			AgentID:  group.OwnerAgentID,
			Role:     HubGroupRoleOwner,
			State:    HubMembershipActive,
			JoinedAt: group.CreatedAt,
		},
	}
	return group, nil
}

func (s *MemoryHubGroupStore) FindGroup(_ context.Context, groupID string) (HubGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[groupID]
	if !exists {
		return HubGroup{}, ErrHubGroupNotFound
	}
	return group, nil
}

func (s *MemoryHubGroupStore) ListGroups(_ context.Context, agentID string) ([]HubGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HubGroup, 0)
	for gid, members := range s.members {
		for _, m := range members {
			if m.AgentID == agentID && m.State == HubMembershipActive {
				if g, ok := s.groups[gid]; ok && g.State == HubGroupStateActive {
					out = append(out, g)
				}
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryHubGroupStore) FindMember(_ context.Context, groupID, agentID string) (HubGroupMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.members[groupID]
	for _, m := range members {
		if m.AgentID == agentID {
			return m, nil
		}
	}
	return HubGroupMember{}, ErrHubGroupNotFound
}

func (s *MemoryHubGroupStore) ListMembers(_ context.Context, groupID string) ([]HubGroupMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, exists := s.members[groupID]
	if !exists {
		return nil, ErrHubGroupNotFound
	}
	out := make([]HubGroupMember, len(members))
	copy(out, members)
	return out, nil
}

func (s *MemoryHubGroupStore) CreateInvitation(_ context.Context, inv HubGroupInvitation) (HubGroupInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[inv.GroupID]
	if !exists || group.State != HubGroupStateActive {
		return HubGroupInvitation{}, ErrHubGroupInvalidState
	}
	activeCount := 0
	for _, m := range s.members[inv.GroupID] {
		if m.State == HubMembershipActive {
			activeCount++
		}
	}
	if activeCount >= MaxGroupMembers {
		return HubGroupInvitation{}, ErrHubGroupLimit
	}
	inv.ID = s.nextInvID
	s.nextInvID++
	if inv.State == "" {
		inv.State = HubInvitationPending
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = inv.CreatedAt.Add(24 * time.Hour)
	}
	s.invitations = append(s.invitations, inv)
	return inv, nil
}

func (s *MemoryHubGroupStore) FindInvitation(_ context.Context, id uint64) (HubGroupInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inv := range s.invitations {
		if inv.ID == id {
			return inv, nil
		}
	}
	return HubGroupInvitation{}, ErrHubInvitationNotFound
}

func (s *MemoryHubGroupStore) ListInvitations(_ context.Context, inviteeAgentID string) ([]HubGroupInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	out := make([]HubGroupInvitation, 0)
	for _, inv := range s.invitations {
		if inv.InviteeAgentID == inviteeAgentID && inv.State == HubInvitationPending && inv.ExpiresAt.After(now) {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (s *MemoryHubGroupStore) AcceptInvitation(_ context.Context, id uint64, agentID string, at time.Time) (HubGroupMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var target *HubGroupInvitation
	for i := range s.invitations {
		if s.invitations[i].ID == id {
			target = &s.invitations[i]
			break
		}
	}
	if target == nil {
		return HubGroupMember{}, ErrHubInvitationNotFound
	}
	if target.InviteeAgentID != agentID || target.State != HubInvitationPending || !target.ExpiresAt.After(at) {
		return HubGroupMember{}, ErrHubGroupInvalidState
	}
	group, exists := s.groups[target.GroupID]
	if !exists || group.State != HubGroupStateActive {
		return HubGroupMember{}, ErrHubGroupInvalidState
	}
	activeCount := 0
	for _, m := range s.members[target.GroupID] {
		if m.State == HubMembershipActive {
			activeCount++
		}
	}
	if activeCount >= MaxGroupMembers {
		return HubGroupMember{}, ErrHubGroupLimit
	}
	target.State = HubInvitationAccepted
	target.RespondedAt = &at

	members := s.members[target.GroupID]
	for i := range members {
		if members[i].AgentID == agentID {
			members[i].State = HubMembershipActive
			members[i].Role = HubGroupRoleMember
			members[i].JoinedAt = at
			return members[i], nil
		}
	}
	newMember := HubGroupMember{
		HubID:    target.HubID,
		GroupID:  target.GroupID,
		AgentID:  agentID,
		Role:     HubGroupRoleMember,
		State:    HubMembershipActive,
		JoinedAt: at,
	}
	s.members[target.GroupID] = append(s.members[target.GroupID], newMember)
	return newMember, nil
}

func (s *MemoryHubGroupStore) DeclineInvitation(_ context.Context, id uint64, agentID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.invitations {
		if s.invitations[i].ID == id {
			if s.invitations[i].InviteeAgentID != agentID || s.invitations[i].State != HubInvitationPending {
				return ErrHubGroupInvalidState
			}
			s.invitations[i].State = HubInvitationDeclined
			s.invitations[i].RespondedAt = &at
			return nil
		}
	}
	return ErrHubInvitationNotFound
}

func (s *MemoryHubGroupStore) RevokeInvitation(_ context.Context, id uint64, inviterID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.invitations {
		if s.invitations[i].ID == id {
			if s.invitations[i].InviterAgentID != inviterID || s.invitations[i].State != HubInvitationPending {
				return ErrHubGroupInvalidState
			}
			s.invitations[i].State = HubInvitationRevoked
			s.invitations[i].RespondedAt = &at
			return nil
		}
	}
	return ErrHubInvitationNotFound
}

func (s *MemoryHubGroupStore) SendGroupMessage(ctx context.Context, msg HubGroupMessage, maxFanout int) (HubGroupMessage, bool, error) {
	s.mu.Lock()
	// Check duplicate
	for _, existing := range s.messages {
		if existing.GroupID == msg.GroupID && existing.SenderAgentID == msg.SenderAgentID && existing.IdempotencyKey == msg.IdempotencyKey {
			s.mu.Unlock()
			return existing, true, nil
		}
	}
	group, exists := s.groups[msg.GroupID]
	if !exists || group.State != HubGroupStateActive {
		s.mu.Unlock()
		return HubGroupMessage{}, false, ErrHubGroupNotFound
	}
	members := s.members[msg.GroupID]
	isSenderMember := false
	recipients := make([]HubGroupMember, 0, len(members))
	for _, m := range members {
		if m.AgentID == msg.SenderAgentID && m.State == HubMembershipActive {
			isSenderMember = true
		}
		if m.AgentID != msg.SenderAgentID && m.State == HubMembershipActive {
			recipients = append(recipients, m)
		}
	}
	if !isSenderMember {
		s.mu.Unlock()
		return HubGroupMessage{}, false, ErrHubGroupForbidden
	}
	if len(recipients) == 0 || (maxFanout > 0 && len(recipients) > maxFanout) {
		s.mu.Unlock()
		return HubGroupMessage{}, false, ErrHubGroupInvalidState
	}
	msg.ID = s.nextMsgID
	s.nextMsgID++
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	msg.Trust = "UNTRUSTED_DATA"
	s.messages = append(s.messages, msg)
	mailbox := s.mailbox
	s.mu.Unlock()

	msg.Deliveries = make([]HubGroupDeliverySummary, 0, len(recipients))
	for _, recipient := range recipients {
		seq := uint64(0)
		if mailbox != nil {
			taskID := fmt.Sprintf("grp-%d-%s", msg.ID, recipient.AgentID)
			internalKey := "group:" + msg.GroupID + ":" + msg.IdempotencyKey
			res, err := mailbox.Enqueue(ctx, HubInboxItem{
				HubID:            msg.HubID,
				TargetAgentID:    recipient.AgentID,
				RequesterAgentID: msg.SenderAgentID,
				TaskID:           taskID,
				ContextID:        msg.ContextID,
				IdempotencyKey:   internalKey,
				Message:          msg.Message,
			})
			if err == nil {
				seq = res.Item.Sequence
			}
		}
		msg.Deliveries = append(msg.Deliveries, HubGroupDeliverySummary{
			TargetAgentID: recipient.AgentID,
			Sequence:      seq,
			State:         "PENDING",
		})
	}
	s.mu.Lock()
	for i := range s.messages {
		if s.messages[i].ID == msg.ID {
			s.messages[i].Deliveries = msg.Deliveries
			break
		}
	}
	s.mu.Unlock()
	return msg, false, nil
}

func (s *MemoryHubGroupStore) ListGroupMessages(_ context.Context, groupID, agentID string, afterID uint64, limit int) ([]HubGroupMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.members[groupID]
	var member *HubGroupMember
	for i := range members {
		if members[i].AgentID == agentID && members[i].State == HubMembershipActive {
			member = &members[i]
			break
		}
	}
	if member == nil {
		return nil, ErrHubGroupForbidden
	}
	if limit <= 0 || limit > MaxGroupHistoryPageSize {
		limit = 50
	}
	out := make([]HubGroupMessage, 0, limit)
	for _, msg := range s.messages {
		if msg.GroupID == groupID && msg.ID > afterID && !msg.CreatedAt.Before(member.JoinedAt) {
			out = append(out, msg)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryHubGroupStore) ArchiveGroup(_ context.Context, groupID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, exists := s.groups[groupID]
	if !exists {
		return ErrHubGroupNotFound
	}
	group.State = HubGroupStateArchived
	group.ArchivedAt = &at
	s.groups[groupID] = group
	return nil
}

func (s *MemoryHubGroupStore) LeaveGroup(_ context.Context, groupID, agentID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.members[groupID]
	for i := range members {
		if members[i].AgentID == agentID && members[i].State == HubMembershipActive {
			if members[i].Role == HubGroupRoleOwner {
				return ErrHubGroupForbidden
			}
			members[i].State = HubMembershipLeft
			members[i].LeftAt = &at
			return nil
		}
	}
	return ErrHubGroupNotFound
}

func (s *MemoryHubGroupStore) RemoveMember(_ context.Context, groupID, agentID, targetAgentID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.members[groupID]
	var requester *HubGroupMember
	var target *HubGroupMember
	for i := range members {
		if members[i].AgentID == agentID && members[i].State == HubMembershipActive {
			requester = &members[i]
		}
		if members[i].AgentID == targetAgentID && members[i].State == HubMembershipActive {
			target = &members[i]
		}
	}
	if requester == nil || !requester.CanManageMembers() {
		return ErrHubGroupForbidden
	}
	if target == nil {
		return ErrHubGroupNotFound
	}
	if target.Role == HubGroupRoleOwner {
		return ErrHubGroupForbidden
	}
	target.State = HubMembershipRemoved
	target.RemovedAt = &at
	return nil
}
