package messageplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
)

func TestEvaluateModerationEnforcesAuthorWindowsAndLegalHold(t *testing.T) {
	createdAt := time.Unix(100, 0)
	now := createdAt.Add(3 * time.Minute)
	message := ModerationMessage{OrganizationID: "org-1", MessageID: "message-1", AuthorID: "user-1", CreatedAt: createdAt}
	policy := ModerationPolicy{EditWindow: 5 * time.Minute, RecallWindow: 15 * time.Minute, LegalHold: true}

	decision, err := EvaluateModeration(policy, "org-1", message, ModerationEdit, "user-1", "edited", now)
	require.NoError(t, err)
	require.Equal(t, EventMessageEdited, decision.Event.Type)
	decision, err = EvaluateModeration(policy, "org-1", message, ModerationRecall, "user-1", "", now)
	require.NoError(t, err)
	require.True(t, decision.Audit.LegalHoldPreserved)

	events := []CollaborationEvent{
		{OrganizationID: "org-1", MessageID: "message-1", Type: EventMessageCreated, Payload: []byte(`{"content":"edited"}`)},
		decision.Event,
	}
	normal, err := ProjectEvents(events)
	require.NoError(t, err)
	require.Empty(t, normal["message-1"].Content)
	held, err := ProjectEventsForLegalHold(events)
	require.NoError(t, err)
	require.Equal(t, "edited", held["message-1"].Content)

	_, err = EvaluateModeration(policy, "org-1", message, ModerationEdit, "user-1", "too late", createdAt.Add(20*time.Minute))
	require.Error(t, err)
	_, err = EvaluateModeration(policy, "org-2", message, ModerationRecall, "user-1", "", now)
	require.Error(t, err)
}

func TestEvaluateModerationAllowsModeratorRedactionWithoutLeakingBody(t *testing.T) {
	message := ModerationMessage{OrganizationID: "org-1", MessageID: "message-1", AuthorID: "user-1", CreatedAt: time.Unix(100, 0)}
	decision, err := EvaluateModeration(ModerationPolicy{ModeratorIDs: []string{"moderator-1"}, LegalHold: true}, "org-1", message, ModerationRedact, "moderator-1", "secret body must not enter audit", time.Unix(200, 0))
	require.NoError(t, err)
	require.Equal(t, EventMessageRedacted, decision.Event.Type)
	require.NotContains(t, string(decision.Event.Payload), "secret body")
	require.NotContains(t, decision.Audit.Reason, "secret body")
	require.True(t, decision.Audit.LegalHoldPreserved)
	_, err = EvaluateModeration(ModerationPolicy{ModeratorIDs: []string{"moderator-1"}}, "org-1", message, ModerationRedact, "user-1", "", time.Unix(200, 0))
	require.Error(t, err)
}

func TestCapabilityPlaneReportsUnsupportedInsteadOfSuccess(t *testing.T) {
	ctx := common.SetOrganizationIDToContext(context.Background(), "org-1")
	planes := []CapabilityPlane{&PostgresPlane{}, &WuKongIMAdapter{}}
	for _, plane := range planes {
		report, err := plane.Capabilities(ctx, "org-1", "conversation-1")
		require.NoError(t, err)
		require.Len(t, report.States, 4)
		for _, state := range report.States {
			require.Equal(t, CapabilityUnsupported, state)
		}
		err = plane.PublishPresence(ctx, PresenceUpdate{OrganizationID: "org-1", ConversationID: "conversation-1", PrincipalID: "user-1", State: "online"})
		require.True(t, errors.Is(err, ErrUnsupportedCapability))
	}
}
