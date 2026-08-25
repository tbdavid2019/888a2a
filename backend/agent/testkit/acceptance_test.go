package testkit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/a2a/orchestration"
	"github.com/Ranxy/laelia/backend/a2a/tools"
	"github.com/Ranxy/laelia/backend/agent/client"
	"github.com/Ranxy/laelia/backend/agent/executor"
	"github.com/Ranxy/laelia/backend/agent/state"
	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
	"github.com/Ranxy/laelia/backend/manager/store"
)

type acceptanceMemoryWorkStore struct {
	mu        sync.RWMutex
	contexts  map[string]*store.WorkContextMessage
	works     map[string]*store.WorkMessage
	artifacts map[string][]*store.WorkArtifactMessage
	events    map[string][]*store.WorkEventMessage
}

func newAcceptanceMemoryWorkStore() *acceptanceMemoryWorkStore {
	return &acceptanceMemoryWorkStore{
		contexts:  make(map[string]*store.WorkContextMessage),
		works:     make(map[string]*store.WorkMessage),
		artifacts: make(map[string][]*store.WorkArtifactMessage),
		events:    make(map[string][]*store.WorkEventMessage),
	}
}

func (m *acceptanceMemoryWorkStore) EnsureWorkContext(_ context.Context, tenantID, contextID, rootWorkID string) (*store.WorkContextMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantID + ":" + contextID
	if c, ok := m.contexts[key]; ok {
		c.UpdatedAt = time.Now()
		return c, nil
	}
	c := &store.WorkContextMessage{
		TenantID:   tenantID,
		ContextID:  contextID,
		RootWorkID: sql.NullString{String: rootWorkID, Valid: rootWorkID != ""},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.contexts[key] = c
	return c, nil
}

func (m *acceptanceMemoryWorkStore) CreateWork(_ context.Context, work *store.WorkMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := work.TenantID + ":" + work.WorkID
	if _, exists := m.works[key]; exists {
		return pkgerrors.Errorf("work already exists: %s", work.WorkID)
	}
	work.CreatedAt = time.Now()
	work.UpdatedAt = time.Now()
	work.Version = 1
	m.works[key] = work
	return nil
}

func (m *acceptanceMemoryWorkStore) GetWork(_ context.Context, tenantID, workID string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	w, ok := m.works[key]
	if !ok {
		return nil, nil
	}
	return w, nil
}

func (m *acceptanceMemoryWorkStore) GetWorkByA2ATaskID(_ context.Context, tenantID, a2aTaskID string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.works {
		if w.TenantID == tenantID && w.A2ATaskID == a2aTaskID {
			return w, nil
		}
	}
	return nil, nil
}

func (m *acceptanceMemoryWorkStore) GetWorkByIdempotencyKey(_ context.Context, tenantID, requesterAgentID, idempotencyKey string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.works {
		if w.TenantID == tenantID && w.RequesterAgentID == requesterAgentID && w.IdempotencyKey == idempotencyKey {
			return w, nil
		}
	}
	return nil, nil
}

func (m *acceptanceMemoryWorkStore) UpdateWorkState(_ context.Context, tenantID, workID string, expectedVersion uint64, newState string, terminalReason string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantID + ":" + workID
	w, ok := m.works[key]
	if !ok {
		return 0, pkgerrors.Errorf("work not found: %s", workID)
	}
	if expectedVersion > 0 && w.Version != expectedVersion {
		return 0, pkgerrors.Errorf("version conflict: %d != %d", w.Version, expectedVersion)
	}
	w.State = newState
	w.TerminalReason = terminalReason
	w.Version++
	w.UpdatedAt = time.Now()
	return w.Version, nil
}

func (m *acceptanceMemoryWorkStore) ListWork(_ context.Context, filter store.ListWorkFilter) ([]*store.WorkMessage, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*store.WorkMessage
	for _, w := range m.works {
		if w.TenantID == filter.TenantID {
			if filter.ContextID != "" && w.ContextID != filter.ContextID {
				continue
			}
			list = append(list, w)
		}
	}
	return list, len(list), nil
}

func (m *acceptanceMemoryWorkStore) CreateWorkArtifact(_ context.Context, artifact *store.WorkArtifactMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := artifact.TenantID + ":" + artifact.WorkID
	artifact.CreatedAt = time.Now()
	m.artifacts[key] = append(m.artifacts[key], artifact)
	return nil
}

func (m *acceptanceMemoryWorkStore) ListWorkArtifacts(_ context.Context, tenantID, workID string) ([]*store.WorkArtifactMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	return m.artifacts[key], nil
}

func (m *acceptanceMemoryWorkStore) AppendWorkEvent(_ context.Context, event *store.WorkEventMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := event.TenantID + ":" + event.WorkID
	event.Sequence = uint64(len(m.events[key]) + 1)
	event.CreatedAt = time.Now()
	m.events[key] = append(m.events[key], event)
	return nil
}

func (m *acceptanceMemoryWorkStore) ListWorkEvents(_ context.Context, tenantID, workID string, afterSequence uint64, limit int) ([]*store.WorkEventMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	all := m.events[key]
	var res []*store.WorkEventMessage
	for _, e := range all {
		if e.Sequence > afterSequence {
			res = append(res, e)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (m *acceptanceMemoryWorkStore) GetLatestWorkEventSequence(_ context.Context, tenantID, workID string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	return uint64(len(m.events[key])), nil
}

func (m *acceptanceMemoryWorkStore) ListPendingWorkForRecovery(_ context.Context) ([]*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*store.WorkMessage
	for _, w := range m.works {
		if w.State == "SUBMITTED" || w.State == "WORKING" {
			list = append(list, w)
		}
	}
	return list, nil
}

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

	workStore := newAcceptanceMemoryWorkStore()
	eventMgr := a2a.NewEventManager(workStore)

	// Machine 1 hosts Coordinator (Agent 1) + Specialists 1-5
	// Machine 2 hosts Specialists 6-10 + Reviewer (Agent 12)
	agents := make([]*a2asdk.AgentCard, 12)
	cards := make(map[string]*a2asdk.AgentCard)

	// 1 Coordinator
	agents[0] = &a2asdk.AgentCard{
		Name:        "agents/coordinator-01",
		Description: "Coordinates 10 specialists and dispatches to reviewer",
		Version:     "1.0",
		Skills: []a2asdk.AgentSkill{
			{ID: "orchestrate", Name: "Orchestrate", Tags: []string{"coordination", "planner"}},
		},
	}

	// 10 Specialists
	for i := 1; i <= specialistCount; i++ {
		agentID := fmt.Sprintf("agents/specialist-%02d", i)
		agents[i] = &a2asdk.AgentCard{
			Name:        agentID,
			Description: fmt.Sprintf("Executes specialized subtask domain %02d", i),
			Version:     "1.0",
			Skills: []a2asdk.AgentSkill{
				{ID: fmt.Sprintf("domain-%02d", i), Name: fmt.Sprintf("Domain %02d", i), Tags: []string{"specialist", "compute"}},
			},
		}
	}

	// 1 Reviewer / Aggregator
	agents[11] = &a2asdk.AgentCard{
		Name:        "agents/reviewer-12",
		Description: "Aggregates and verifies results from all 10 specialists",
		Version:     "1.0",
		Skills: []a2asdk.AgentSkill{
			{ID: "review", Name: "Review & Validate", Tags: []string{"review", "aggregate"}},
		},
	}

	for _, card := range agents {
		cards[card.Name] = card
		require.NotEmpty(t, card.Name, "every agent card must have a name")
	}

	// =========================================================================
	// 8.2 Deterministic Fan-Out / Join Acceptance across 10 Specialists
	// =========================================================================
	rootWorkID := "work-root-coord-01"
	rootTask := &store.WorkMessage{
		WorkID:           rootWorkID,
		TenantID:         "tenant-default",
		ContextID:        "ctx-acceptance-01",
		RequesterAgentID: "agents/human-operator",
		ExecutorAgentID:  agents[0].Name,
		State:            "WORKING",
	}
	require.NoError(t, workStore.CreateWork(ctx, rootTask))

	orchestrator := orchestration.NewOrchestrator(orchestration.OrchestratorOptions{
		EventManager: eventMgr,
	})

	fanOutTasks := make([]orchestration.FanOutTaskSpec, specialistCount)
	for i := 0; i < specialistCount; i++ {
		idx := i
		specialistID := agents[idx+1].Name
		fanOutTasks[idx] = orchestration.FanOutTaskSpec{
			TaskID:          fmt.Sprintf("work-subtask-%02d", idx+1),
			ExecutorAgentID: specialistID,
			Input:           fmt.Sprintf("compute-input-%02d", idx+1),
			Budget: &a2a.WorkBudget{
				MaxDepth:    3,
				MaxChildren: 10,
				MaxFanOut:   10,
			},
			Executor: func(_ context.Context, _ *store.WorkMessage) (*orchestration.TaskOutput, error) {
				return &orchestration.TaskOutput{
					Output:        fmt.Sprintf("result-from-%s: compute-input-%02d", specialistID, idx+1),
					TokensUsed:    100 + int64(idx*10),
					WorkUnitsUsed: 1,
					Artifacts: []*store.WorkArtifactMessage{
						{
							ArtifactID: fmt.Sprintf("art-%02d", idx+1),
							Name:       fmt.Sprintf("domain-%02d-output.json", idx+1),
						},
					},
				}, nil
			},
		}
	}

	fanOutReq := orchestration.FanOutRequest{
		ParentWorkID:     rootWorkID,
		RequesterAgentID: agents[0].Name,
		TenantID:         "tenant-default",
		Tasks:            fanOutTasks,
		Policy:           orchestration.JoinPolicyAllSuccess,
		MaxConcurrency:   10,
		Timeout:          5 * time.Second,
	}

	fanOutResult, err := orchestrator.ExecuteFanOut(ctx, fanOutReq)
	require.NoError(t, err)
	require.NotNil(t, fanOutResult)
	assert.True(t, fanOutResult.Success, "10-specialist fanout must succeed")
	assert.Equal(t, specialistCount, fanOutResult.TotalTasks)
	assert.Equal(t, specialistCount, fanOutResult.CompletedCount)
	assert.Equal(t, 0, fanOutResult.FailedCount)

	// Verify deterministic index ordering (TaskResults[i].Index == i)
	for i := 0; i < specialistCount; i++ {
		assert.Equal(t, i, fanOutResult.TaskResults[i].Index)
		assert.Equal(t, fmt.Sprintf("work-subtask-%02d", i+1), fanOutResult.TaskResults[i].TaskID)
		assert.Equal(t, "COMPLETED", fanOutResult.TaskResults[i].State)
	}

	// Pass all 10 results to Reviewer (Agent 12)
	reviewWorkID := "work-review-12"
	reviewTask := &store.WorkMessage{
		WorkID:           reviewWorkID,
		TenantID:         "tenant-default",
		ContextID:        "ctx-acceptance-01",
		RequesterAgentID: agents[0].Name,
		ExecutorAgentID:  agents[11].Name,
		State:            "COMPLETED",
	}
	require.NoError(t, workStore.CreateWork(ctx, reviewTask))
	_, _ = workStore.UpdateWorkState(ctx, "tenant-default", rootWorkID, 1, "COMPLETED", "fan-out completed")

	// =========================================================================
	// 8.3 Restart Manager During Active Work & Reconnect Machine
	// =========================================================================
	interruptedWorkID := "work-interrupted-01"
	interruptedTask := &store.WorkMessage{
		WorkID:           interruptedWorkID,
		TenantID:         "tenant-default",
		ContextID:        "ctx-restart-01",
		RequesterAgentID: agents[0].Name,
		ExecutorAgentID:  agents[1].Name,
		State:            "WORKING",
	}
	require.NoError(t, workStore.CreateWork(ctx, interruptedTask))

	// Manager restart: RecoveryService scans in-flight working tasks and recovers them
	recoverySvc := a2a.NewRecoveryService(workStore)
	recoveryReport, err := recoverySvc.RecoverPendingWork(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, recoveryReport.Recovered, "interrupted task must be transitioned safely")

	recoveredRec, err := workStore.GetWork(ctx, "tenant-default", interruptedWorkID)
	require.NoError(t, err)
	assert.Equal(t, "SUBMITTED", recoveredRec.State, "recovered task is safely returned to SUBMITTED for re-dispatch")

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
	sendParams := tools.TaskSendInput{
		TenantID:         "tenant-default",
		ContextID:        "ctx-retry-01",
		RequesterAgentID: agents[0].Name,
		TargetAgentID:    agents[2].Name,
		Message:          "run idempotent task",
		IdempotencyKey:   idempKey,
	}

	// First send
	sendRes1, err := tools.TaskSend(ctx, workStore, eventMgr, sendParams)
	require.NoError(t, err)
	require.NotEmpty(t, sendRes1.WorkID)

	// Lost response retry with same idempotency key: must return existing task
	sendRes2, err := tools.TaskSend(ctx, workStore, eventMgr, sendParams)
	require.NoError(t, err)
	assert.Equal(t, sendRes1.WorkID, sendRes2.WorkID, "idempotent retry must return identical work ID")

	// Cancel task and verify terminal idempotency
	cancelRes, err := tools.TaskCancel(ctx, workStore, eventMgr, tools.TaskCancelInput{
		TenantID:      "tenant-default",
		WorkID:        sendRes1.WorkID,
		CallerAgentID: agents[0].Name,
		Reason:        "user requested cancellation",
	})
	require.NoError(t, err)
	assert.Equal(t, "CANCELED", cancelRes.State)

	// Repeated cancel on already canceled task is idempotent
	cancelRes2, err := tools.TaskCancel(ctx, workStore, eventMgr, tools.TaskCancelInput{
		TenantID:      "tenant-default",
		WorkID:        sendRes1.WorkID,
		CallerAgentID: agents[0].Name,
		Reason:        "repeat cancel",
	})
	require.NoError(t, err)
	assert.Equal(t, "CANCELED", cancelRes2.State)

	// =========================================================================
	// 8.5 Security Penetration Probes: Cross-Agent Workspace & Path Confinement
	// =========================================================================
	// Verify workspace isolation across all 12 agents
	var wg sync.WaitGroup
	isolationErrs := make([]error, 12)
	for i := 0; i < 12; i++ {
		agentIdx := i + 1
		agentID := agents[i].Name
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
			peerID := agents[peerIdx-1].Name
			traversalPath := filepath.Join("..", peerID, "workspace", "local.data")
			if _, err := client.ConfinePathToAgentWorkspace(machID, agentID, traversalPath); err == nil {
				isolationErrs[agentIdx-1] = pkgerrors.Errorf("agent %s traversal to peer %s succeeded", agentID, peerID)
				return
			}

			// 3. Cross-agent ownership probe
			if err := client.AssertAgentOwnership(agentID, peerID); err == nil {
				isolationErrs[agentIdx-1] = pkgerrors.Errorf("agent %s claimed unauthorized ownership of %s", agentID, peerID)
				return
			}
		})
	}
	wg.Wait()

	for idx, err := range isolationErrs {
		require.NoError(t, err, "Agent %d failed penetration probe", idx+1)
	}
}
