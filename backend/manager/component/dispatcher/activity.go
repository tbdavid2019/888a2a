package dispatcher

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	v1pb "github.com/Ranxy/laelia/backend/generated-go/v1"
	"github.com/Ranxy/laelia/backend/manager/store"
)

// activityAggregator computes per-agent conversation activity from the session
// registry and the store. It is extracted from Dispatcher so this aggregation
// can be tested and evolved independently.
type activityAggregator struct {
	store    *store.Store
	registry *sessionRegistry
}

func (a *activityAggregator) FetchConversationActivity(ctx context.Context, conversationID string) ([]*v1pb.AgentActivity, error) {
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid conversation id")
	}

	members, err := a.store.ListConversationMembers(ctx, convUUID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list conversation members")
	}

	// Collect agent members: member_id is the agent resource ID.
	type agentEntry struct {
		resourceID string
		name       string
		id         int
	}
	var agents []agentEntry
	var agentIDs []int
	for _, m := range members {
		if m.MemberType != store.MemberTypeAgent {
			continue
		}
		ag, agErr := a.store.GetAgentByResourceID(ctx, m.MemberID)
		if agErr != nil || ag == nil {
			continue
		}
		agents = append(agents, agentEntry{resourceID: ag.ResourceID, name: ag.Name, id: ag.ID})
		agentIDs = append(agentIDs, ag.ID)
	}

	// Batch-query running commands for these agents in this conversation.
	running, runErr := a.store.GetRunningCommandsForConversation(ctx, agentIDs, convUUID)
	if runErr != nil {
		return nil, errors.Wrapf(runErr, "failed to get running commands")
	}
	runningByAgent := make(map[int]*store.RunningCommandInfo, len(running))
	for _, r := range running {
		runningByAgent[r.AgentID] = r
	}

	// Build activity entries.
	activities := make([]*v1pb.AgentActivity, 0, len(agents))
	for _, ag := range agents {
		act := &v1pb.AgentActivity{
			AgentId:     ag.resourceID,
			DisplayName: ag.name,
			Status:      "idle",
		}

		sess, connected := a.registry.getAgent(ag.id)
		if !connected {
			act.Status = "offline"
			activities = append(activities, act)
			continue
		}

		rci, hasRunning := runningByAgent[ag.id]
		if !hasRunning {
			activities = append(activities, act) // stays "idle"
			continue
		}

		// Derive status from the latest command event.
		switch rci.EventType {
		case 0:
			act.Status = "starting"
		case int32(v1pb.CommandEventType_LIFECYCLE):
			act.Status = "starting"
		case int32(v1pb.CommandEventType_TEXT_DELTA):
			act.Status = "output"
		case int32(v1pb.CommandEventType_TOOL_CALL_STARTED):
			if rci.Summary.Valid {
				act.Status = rci.Summary.String
				act.ToolName = rci.Summary.String
			} else {
				act.Status = "tool"
			}
		case int32(v1pb.CommandEventType_TOOL_CALL_FINISHED):
			act.Status = "thinking"
		case int32(v1pb.CommandEventType_CONTEXT_COMPACTION_STARTED):
			act.Status = "compacting"
		case int32(v1pb.CommandEventType_CONTEXT_COMPACTION_FINISHED), int32(v1pb.CommandEventType_CONTEXT_USAGE_UPDATE):
			act.Status = "thinking"
		default:
			act.Status = "starting"
		}

		// Suppress idle for active agents that might have a stale session.
		sess.mu.Lock()
		if sess.currentCmdID == "" {
			act.Status = "idle"
		}
		sess.mu.Unlock()

		activities = append(activities, act)
	}

	return activities, nil
}
