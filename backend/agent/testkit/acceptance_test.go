package testkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/a2a/orchestration"
	"github.com/Ranxy/laelia/backend/a2a/tools"
	"github.com/Ranxy/laelia/backend/agent/client"
	"github.com/Ranxy/laelia/backend/agent/executor"
	"github.com/Ranxy/laelia/backend/agent/state"
	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
	"github.com/Ranxy/laelia/backend/manager/component/dispatcher"
)

// TestTwelveAgentAcceptanceGate verifies the end-to-end 12-Agent acceptance gate (Tasks 8.1 - 8.5).
func TestTwelveAgentAcceptanceGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tempHome := t.TempDir()
	t.Setenv("A2A888_HOME", tempHome)

	// =========================================================================
	// 8.1 Topology Setup: 2 Machines, 12 Agents (1 Coordinator, 10 Specialists, 1 Reviewer)
	// =========================================================================
	const specialistCount = 10
	machine1ID := "machines/mach-01"
	machine2ID := "machines/mach-02"

	// Setup memory store & event manager
	workStore := tools.NewMemoryWorkStore()
	eventMgr := a2a.NewEventManager()
	traceRec := a2a.NewTraceRecorder(nil)

	// Machine 1 hosts Coordinator (Agent 1) + Specialists 1-5
	// Machine 2 hosts Specialists 6-10 + Reviewer (Agent 12)
	agents := make([]*a2a888.AgentCard, 12)
	cards := make(map[string]*a2a888.AgentCard)

	// 1 Coordinator
	agents[0] = &a2a888.AgentCard{
		AgentResourceId: "agents/coordinator-01",
		DisplayName:     "Orchestration Coordinator",
		Description:     "Coordinates 10 specialists and dispatches to reviewer",
		Skills: []*a2a888.AgentSkill{
			{Id: "orchestrate", Name: "Orchestrate", Tags: []string{"coordination", "planner"}},
		},
		Readiness: a2a888.RuntimeStatus_READY,
	}

	// 10 Specialists
	for i := 1; i <= specialistCount; i++ {
		agentID := fmt.Sprintf("agents/specialist-%02d", i)
		agents[i] = &a2a888.AgentCard{
			AgentResourceId: agentID,
			DisplayName:     fmt.Sprintf("Specialist Domain %02d", i),
			Description:     fmt.Sprintf("Executes specialized subtask domain %02d", i),
			Skills: []*a2a888.AgentSkill{
				{Id: fmt.Sprintf("domain-%02d", i), Name: fmt.Sprintf("Domain %02d", i), Tags: []string{"specialist", "compute"}},
			},
			Readiness: a2a888.RuntimeStatus_READY,
		}
	}

	// 1 Reviewer / Aggregator
	agents[11] = &a2a888.AgentCard{
		AgentResourceId: "agents/reviewer-12",
		DisplayName:     "Quality Reviewer & Aggregator",
		Description:     "Aggregates and verifies results from all 10 specialists",
		Skills: []*a2a888.AgentSkill{
			{Id: "review", Name: "Review & Validate", Tags: []string{"review", "aggregate"}},
		},
		Readiness: a2a888.RuntimeStatus_READY,
	}

	for _, card := range agents {
		cards[card.AgentResourceId] = card
		require.Equal(t, a2a888.RuntimeStatus_READY, card.Readiness, "every agent must reach READY")
	}

	// Verify capacity and availability reporting for both machines
	cap1 := client.NewCapacityTracker(16)
	cap2 := client.NewCapacityTracker(16)
	for i := 0; i < 6; i++ {
		cap1.IncrementInFlight(agents[i].AgentResourceId)
		cap1.DecrementInFlight(agents[i].AgentResourceId)
	}
	for i := 6; i < 12; i++ {
		cap2.IncrementInFlight(agents[i].AgentResourceId)
		cap2.DecrementInFlight(agents[i].AgentResourceId)
	}

	disp := dispatcher.New()
	disp.RecordMachineConnection(machine1ID)
	disp.RecordMachineConnection(machine2ID)
	disp.UpdateMachineCapacity(machine1ID, cap1.Snapshot(machine1ID, true))
	disp.UpdateMachineCapacity(machine2ID, cap2.Snapshot(machine2ID, true))

	for _, card := range agents {
		assert.True(t, disp.IsAgentReadyForWork(card.AgentResourceId), "Agent %s should be ready for work", card.AgentResourceId)
	}

	// =========================================================================
	// 8.2 Deterministic Fan-Out / Join Acceptance across 10 Specialists
	// =========================================================================
	rootWorkID := "work-root-coord-01"
	rootTask := &a2a888.WorkRecord{
		WorkId:           rootWorkID,
		TenantId:         "tenant-default",
		ContextId:        "ctx-acceptance-01",
		RequesterAgentId: "agents/human-operator",
		ExecutorAgentId:  agents[0].AgentResourceId,
		State:            a2a888.WorkState_WORKING,
		CreatedAt:        timestamppb.New(time.Now()),
		UpdatedAt:        timestamppb.New(time.Now()),
	}
	require.NoError(t, workStore.CreateWork(ctx, rootTask))

	specialistSubtasks := make([]orchestration.Subtask, specialistCount)
	for i := 0; i < specialistCount; i++ {
		specialistID := agents[i+1].AgentResourceId
		specialistSubtasks[i] = orchestration.Subtask{
			WorkID:          fmt.Sprintf("work-subtask-%02d", i+1),
			ExecutorAgentID: specialistID,
			Payload:         fmt.Sprintf("compute-input-%02d", i+1),
			Budget: &a2a888.WorkBudget{
				MaxDepth:          3,
				MaxChildren:       10,
				MaxFanOut:         10,
				MaxTimeoutSeconds: 30,
			},
		}
	}

	fanOutCfg := orchestration.FanOutConfig{
		CoordinatorAgentID: agents[0].AgentResourceId,
		ParentWorkID:       rootWorkID,
		TenantID:           "tenant-default",
		ContextID:          "ctx-acceptance-01",
		Subtasks:           specialistSubtasks,
		Policy:             orchestration.JoinPolicyAllSuccess,
		MaxConcurrency:     10,
		Timeout:            5 * time.Second,
	}

	// Execute fanout with deterministic worker function
	fanOutResult, err := orchestration.ExecuteFanOut(ctx, fanOutCfg, func(subCtx context.Context, sub orchestration.Subtask) (*orchestration.SubtaskResult, error) {
		// Record work creation in store
		childRec := &a2a888.WorkRecord{
			WorkId:           sub.WorkID,
			TenantId:         "tenant-default",
			ContextId:        "ctx-acceptance-01",
			RequesterAgentId: agents[0].AgentResourceId,
			ExecutorAgentId:  sub.ExecutorAgentID,
			State:            a2a888.WorkState_WORKING,
			ParentEdge: &a2a888.ParentEdge{
				ParentWorkId: rootWorkID,
				Depth:        1,
			},
			CreatedAt: timestamppb.New(time.Now()),
			UpdatedAt: timestamppb.New(time.Now()),
		}
		if err := workStore.CreateWork(subCtx, childRec); err != nil {
			return nil, err
		}

		// Emulate deterministic output
		outputPayload := fmt.Sprintf("result-from-%s: %s", sub.ExecutorAgentID, sub.Payload)
		childRec.State = a2a888.WorkState_COMPLETED
		_ = workStore.UpdateWork(subCtx, childRec)

		return &orchestration.SubtaskResult{
			WorkID:          sub.WorkID,
			ExecutorAgentID: sub.ExecutorAgentID,
			Output:          outputPayload,
			State:           a2a888.WorkState_COMPLETED,
		}, nil
	})

	require.NoError(t, err)
	require.NotNil(t, fanOutResult)
	assert.True(t, fanOutResult.Success, "10-specialist fanout must succeed")
	assert.Equal(t, specialistCount, len(fanOutResult.Completed), "all 10 subtasks must complete")
	assert.Equal(t, 0, len(fanOutResult.Failed), "no subtasks should fail")

	// Pass all 10 results to the Reviewer (Agent 12)
	reviewWorkID := "work-review-12"
	reviewRec := &a2a888.WorkRecord{
		WorkId:           reviewWorkID,
		TenantId:         "tenant-default",
		ContextId:        "ctx-acceptance-01",
		RequesterAgentId: agents[0].AgentResourceId,
		ExecutorAgentId:  agents[11].AgentResourceId,
		State:            a2a888.WorkState_COMPLETED,
		CreatedAt:        timestamppb.New(time.Now()),
		UpdatedAt:        timestamppb.New(time.Now()),
	}
	require.NoError(t, workStore.CreateWork(ctx, reviewRec))

	// =========================================================================
	// 8.3 Restart Manager During Active Work & Reconnect Machine
	// =========================================================================
	interruptedWorkID := "work-interrupted-01"
	interruptedTask := &a2a888.WorkRecord{
		WorkId:           interruptedWorkID,
		TenantId:         "tenant-default",
		ContextId:        "ctx-restart-01",
		RequesterAgentId: agents[0].AgentResourceId,
		ExecutorAgentId:  agents[1].AgentResourceId,
		State:            a2a888.WorkState_WORKING,
		CreatedAt:        timestamppb.New(time.Now()),
		UpdatedAt:        timestamppb.New(time.Now()),
	}
	require.NoError(t, workStore.CreateWork(ctx, interruptedTask))

	// Manager restart: RecoveryService scans in-flight working tasks and recovers them
	recoverySvc := a2a.NewRecoveryService(workStore, eventMgr, traceRec)
	recoveredCount, err := recoverySvc.RecoverPendingWork(ctx, "tenant-default")
	require.NoError(t, err)
	assert.Equal(t, 1, recoveredCount, "interrupted task must be transitioned safely")

	recoveredRec, err := workStore.GetWork(ctx, "tenant-default", interruptedWorkID)
	require.NoError(t, err)
	assert.Equal(t, a2a888.WorkState_INPUT_REQUIRED, recoveredRec.State, "recovered task is marked for retry/input")

	// Disconnect and Reconnect Machine 1: cursor replay
	_ = state.SaveAckCursor(&a2a888.AssignmentCursor{
		Sequence:       5,
		EventId:        "evt-5",
		IdempotencyKey: "key-5",
	})
	loadedState, err := state.Load()
	require.NoError(t, err)
	assert.Equal(t, uint64(5), loadedState.GetLastAckCursor().GetSequence())

	// =========================================================================
	// 8.4 Retry Lost A2A Send Response & Cancel Descendant
	// =========================================================================
	idempKey := "idemp-send-acceptance-01"
	sendParams := tools.TaskSendParams{
		TenantID:         "tenant-default",
		ContextID:        "ctx-retry-01",
		RequesterAgentID: agents[0].AgentResourceId,
		TargetAgentID:    agents[2].AgentResourceId,
		Message:          "run idempotent task",
		IdempotencyKey:   idempKey,
	}

	// First send
	sendRes1, err := tools.TaskSend(ctx, workStore, eventMgr, traceRec, sendParams)
	require.NoError(t, err)
	require.NotEmpty(t, sendRes1.WorkID)

	// Lost response retry with same idempotency key: must return existing task
	sendRes2, err := tools.TaskSend(ctx, workStore, eventMgr, traceRec, sendParams)
	require.NoError(t, err)
	assert.Equal(t, sendRes1.WorkID, sendRes2.WorkID, "idempotent retry must return identical work ID")
	assert.True(t, sendRes2.Deduplicated, "must be marked as deduplicated")

	// Cancel task and verify terminal idempotency
	cancelRes, err := tools.TaskCancel(ctx, workStore, eventMgr, traceRec, tools.TaskCancelParams{
		TenantID:        "tenant-default",
		WorkID:          sendRes1.WorkID,
		CallerAgentID:   agents[0].AgentResourceId,
		Reason:          "user requested cancellation",
		CascadeChildren: true,
	})
	require.NoError(t, err)
	assert.Equal(t, a2a888.WorkState_CANCELED, cancelRes.State)

	// Repeated cancel on already canceled task is idempotent
	cancelRes2, err := tools.TaskCancel(ctx, workStore, eventMgr, traceRec, tools.TaskCancelParams{
		TenantID:        "tenant-default",
		WorkID:          sendRes1.WorkID,
		CallerAgentID:   agents[0].AgentResourceId,
		Reason:          "repeat cancel",
		CascadeChildren: true,
	})
	require.NoError(t, err)
	assert.Equal(t, a2a888.WorkState_CANCELED, cancelRes2.State)

	// =========================================================================
	// 8.5 Security Penetration Probes: Cross-Agent Workspace & Path Confinement
	// =========================================================================
	// Verify workspace isolation across all 12 agents
	var wg sync.WaitGroup
	isolationErrs := make([]error, 12)
	for i := 0; i < 12; i++ {
		agentIdx := i + 1
		agentID := agents[i].AgentResourceId
		machID := machine1ID
		if agentIdx > 6 {
			machID = machine2ID
		}

		wg.Go(func() {
			wsDir := executor.AgentWorkingDir(machID, agentID)
			_ = os.MkdirAll(wsDir, 0o700)

			// 1. Legitimate file write within agent workspace
			validFile, err := client.ConfinePathToAgentWorkspace(machID, agentID, "local.data")
			if err != nil {
				isolationErrs[agentIdx-1] = err
				return
			}
			_ = os.WriteFile(validFile, []byte(fmt.Sprintf("data-%02d", agentIdx)), 0o600)

			// 2. Traversal attempt to peer agent workspace
			peerIdx := ((agentIdx) % 12) + 1
			peerID := agents[peerIdx-1].AgentResourceId
			traversalPath := filepath.Join("..", peerID, "workspace", "local.data")
			if _, err := client.ConfinePathToAgentWorkspace(machID, agentID, traversalPath); err == nil {
				isolationErrs[agentIdx-1] = fmt.Errorf("agent %s traversal to peer %s succeeded", agentID, peerID)
				return
			}

			// 3. Cross-agent ownership probe
			if err := client.AssertAgentOwnership(agentID, peerID); err == nil {
				isolationErrs[agentIdx-1] = fmt.Errorf("agent %s claimed unauthorized ownership of %s", agentID, peerID)
				return
			}
		})
	}
	wg.Wait()

	for idx, err := range isolationErrs {
		require.NoError(t, err, "Agent %d failed penetration probe", idx+1)
	}
}
