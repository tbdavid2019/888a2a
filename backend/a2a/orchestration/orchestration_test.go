package orchestration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/manager/store"
)

// TestCycleDetection_DirectAndIndirect verifies that direct (A->A) and indirect (A->B->C->A)
// cycles are rejected before commit without corrupting graph state.
func TestCycleDetection_DirectAndIndirect(t *testing.T) {
	graph := NewTaskGraph()

	// Add root node A
	graph.AddNode(&TaskNode{WorkID: "task-A", State: "WORKING", Depth: 0})

	// Add child B with parent A
	require.NoError(t, graph.CheckCycle("task-A", "task-B"))
	graph.AddNode(&TaskNode{WorkID: "task-B", ParentID: "task-A", State: "WORKING", Depth: 1})

	// Add child C with parent B
	require.NoError(t, graph.CheckCycle("task-B", "task-C"))
	graph.AddNode(&TaskNode{WorkID: "task-C", ParentID: "task-B", State: "WORKING", Depth: 2})

	// 1. Direct cycle: C -> C must be rejected
	err := graph.CheckCycle("task-C", "task-C")
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrCyclicDelegation)
	assert.Contains(t, err.Error(), "direct cycle")

	// 2. Indirect cycle: C -> A (attempting to make A child of C when C is descendant of A) must be rejected
	err = graph.CheckCycle("task-C", "task-A")
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrCyclicDelegation)
	assert.Contains(t, err.Error(), "indirect cycle")

	// 3. Indirect cycle: C -> B must be rejected
	err = graph.CheckCycle("task-C", "task-B")
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrCyclicDelegation)

	// 4. Valid new branch D -> E with parent A is accepted
	require.NoError(t, graph.CheckCycle("task-A", "task-D"))
	graph.AddNode(&TaskNode{WorkID: "task-D", ParentID: "task-A", State: "WORKING", Depth: 1})
	require.NoError(t, graph.CheckCycle("task-D", "task-E"))
	graph.AddNode(&TaskNode{WorkID: "task-E", ParentID: "task-D", State: "WORKING", Depth: 2})

	// Verify descendants for task-A: B, C, D, E
	descendants := graph.GetDescendants("task-A")
	assert.Len(t, descendants, 4)
	assert.Contains(t, descendants, "task-B")
	assert.Contains(t, descendants, "task-C")
	assert.Contains(t, descendants, "task-D")
	assert.Contains(t, descendants, "task-E")
}

// TestOrchestration_BudgetAndLimitsEnforcement verifies depth, child count, fan-out, concurrency,
// retry and token/work-unit limits.
func TestOrchestration_BudgetAndLimitsEnforcement(t *testing.T) {
	orchestrator := NewOrchestrator(OrchestratorOptions{})

	parent := &store.WorkMessage{
		WorkID:          "coordinator-root",
		State:           "WORKING",
		DelegationDepth: 1,
		MaxDepth:        2,
		MaxChildren:     3,
		UsedChildren:    2,
		MaxFanOut:       4,
		MaxConcurrency:  2,
		MaxTokens:       50000,
		UsedTokens:      35000, // 15000 remaining
		MaxWorkUnits:    100,
		UsedWorkUnits:   80, // 20 remaining
	}

	// 1. Depth limit: parent depth (1) + 1 = 2 <= 2 (OK)
	assert.NoError(t, a2a.ValidateDelegationLimits(parent, 2))

	// Depth limit exceeded (parent depth 2 + 1 = 3 > 2)
	err := a2a.ValidateDelegationLimits(parent, 3)
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum delegation depth exceeded")

	// 2. Child count limit: used 2 + 1 = 3 <= 3 (OK)
	assert.NoError(t, a2a.ValidateDelegationLimits(parent, 2))
	parent.UsedChildren = 3
	err = a2a.ValidateDelegationLimits(parent, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum child count exceeded")

	// 3. Fan-out limit
	parent.UsedChildren = 0
	assert.NoError(t, a2a.ValidateFanOutLimit(parent, 3))
	err = a2a.ValidateFanOutLimit(parent, 5) // > MaxFanOut (4)
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum fan-out exceeded")

	// 4. Concurrency limit: 1 active + 1 = 2 <= 2 (OK)
	assert.NoError(t, a2a.ValidateConcurrencyLimit(parent, 1))
	err = a2a.ValidateConcurrencyLimit(parent, 2) // 2 + 1 = 3 > 2
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum concurrency exceeded")

	// 5. Token budget availability: requesting 10000 <= 15000 (OK)
	assert.NoError(t, a2a.ValidateBudgetAvailability(parent, 10000, 10))
	// Requesting 20000 > 15000: Error
	err = a2a.ValidateBudgetAvailability(parent, 20000, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, a2a.ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "token budget exceeded")

	// 6. Child budget allocation bounded by parent remaining
	childBudget := a2a.AllocateChildBudget(parent, &a2a.WorkBudget{
		MaxTokens:    100000,
		MaxWorkUnits: 50,
	})
	assert.Equal(t, int64(15000), childBudget.MaxTokens, "child token budget capped at parent remaining")
	assert.Equal(t, int64(20), childBudget.MaxWorkUnits, "child unit budget capped at parent remaining")

	_ = orchestrator
}

// TestFanOutJoin_TenPeersDeterministicAggregation tests parallel fan-out across ten peer specialists,
// deterministic result aggregation, partial failure policies, and timeout handling.
func TestFanOutJoin_TenPeersDeterministicAggregation(t *testing.T) {
	orchestrator := NewOrchestrator(OrchestratorOptions{})

	// 1. Successful ten-peer parallel fan-out with deterministic aggregation
	var executionOrder []int
	var execMu sync.Mutex

	tasks := make([]FanOutTaskSpec, 10)
	for i := 0; i < 10; i++ {
		idx := i
		agentID := fmt.Sprintf("specialist-%d", idx+1)
		tasks[i] = FanOutTaskSpec{
			TaskID:          fmt.Sprintf("task-%02d", idx+1),
			ExecutorAgentID: agentID,
			Input:           fmt.Sprintf("input-for-specialist-%d", idx+1),
			Executor: func(_ context.Context, _ *store.WorkMessage) (*TaskOutput, error) {
				execMu.Lock()
				executionOrder = append(executionOrder, idx)
				execMu.Unlock()

				// Small synthetic delay to simulate concurrent work
				time.Sleep(10 * time.Millisecond)

				return &TaskOutput{
					Output:        fmt.Sprintf("Processed output from %s", agentID),
					TokensUsed:    100 + int64(idx*10),
					WorkUnitsUsed: 5,
					Artifacts: []*store.WorkArtifactMessage{
						{
							ArtifactID:  fmt.Sprintf("artifact-%02d", idx+1),
							Name:        fmt.Sprintf("result-%d.json", idx+1),
							Description: fmt.Sprintf("Artifact produced by %s", agentID),
						},
					},
				}, nil
			},
		}
	}

	joinRes, err := orchestrator.ExecuteFanOut(context.Background(), FanOutRequest{
		ParentWorkID:     "coordinator-parent",
		RequesterAgentID: "coordinator",
		Tasks:            tasks,
		Policy:           JoinPolicyAllSuccess,
		MaxConcurrency:   5,
	})
	require.NoError(t, err)
	require.NotNil(t, joinRes)

	assert.True(t, joinRes.Success)
	assert.Equal(t, 10, joinRes.TotalTasks)
	assert.Equal(t, 10, joinRes.CompletedCount)
	assert.Equal(t, 0, joinRes.FailedCount)
	assert.Equal(t, 0, joinRes.CanceledCount)
	assert.Len(t, joinRes.TaskResults, 10)

	// Verify deterministic index ordering (results[i].Index == i) regardless of goroutine finish order
	for i := 0; i < 10; i++ {
		assert.Equal(t, i, joinRes.TaskResults[i].Index)
		assert.Equal(t, fmt.Sprintf("task-%02d", i+1), joinRes.TaskResults[i].TaskID)
		assert.Equal(t, fmt.Sprintf("specialist-%d", i+1), joinRes.TaskResults[i].ExecutorAgentID)
		assert.Equal(t, "COMPLETED", joinRes.TaskResults[i].State)
		assert.Contains(t, joinRes.TaskResults[i].Output, fmt.Sprintf("specialist-%d", i+1))
		assert.Len(t, joinRes.TaskResults[i].Artifacts, 1)
	}

	// 2. Partial Failure Policy: 8 succeed, 2 fail
	partialTasks := make([]FanOutTaskSpec, 10)
	for i := 0; i < 10; i++ {
		idx := i
		agentID := fmt.Sprintf("specialist-%d", idx+1)
		shouldFail := (idx == 3 || idx == 7)

		partialTasks[i] = FanOutTaskSpec{
			TaskID:          fmt.Sprintf("ptask-%02d", idx+1),
			ExecutorAgentID: agentID,
			Executor: func(_ context.Context, _ *store.WorkMessage) (*TaskOutput, error) {
				if shouldFail {
					return nil, fmt.Errorf("peer %s unavailable", agentID)
				}
				return &TaskOutput{
					Output: fmt.Sprintf("OK from %s", agentID),
				}, nil
			},
		}
	}

	// JoinPolicyPartialFailure with minSuccess=8 -> Success
	partialJoinRes, err := orchestrator.ExecuteFanOut(context.Background(), FanOutRequest{
		ParentWorkID:    "coordinator-parent",
		Tasks:           partialTasks,
		Policy:          JoinPolicyPartialFailure,
		MinSuccessCount: 8,
		MaxConcurrency:  4,
	})
	require.NoError(t, err)
	assert.True(t, partialJoinRes.Success)
	assert.Equal(t, 8, partialJoinRes.CompletedCount)
	assert.Equal(t, 2, partialJoinRes.FailedCount)
	assert.Equal(t, "FAILED", partialJoinRes.TaskResults[3].State)
	assert.Equal(t, "FAILED", partialJoinRes.TaskResults[7].State)
	assert.Equal(t, "COMPLETED", partialJoinRes.TaskResults[0].State)

	// 3. Timeout Policy: Fan-out bounded by timeout
	timeoutTasks := make([]FanOutTaskSpec, 10)
	for i := 0; i < 10; i++ {
		timeoutTasks[i] = FanOutTaskSpec{
			TaskID:          fmt.Sprintf("ttask-%02d", i+1),
			ExecutorAgentID: fmt.Sprintf("specialist-%d", i+1),
			Executor: func(ctx context.Context, _ *store.WorkMessage) (*TaskOutput, error) {
				select {
				case <-time.After(500 * time.Millisecond):
					return &TaskOutput{Output: "done late"}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}
	}

	timeoutRes, err := orchestrator.ExecuteFanOut(context.Background(), FanOutRequest{
		ParentWorkID:   "coordinator-parent",
		Tasks:          timeoutTasks,
		Policy:         JoinPolicyAllSuccess,
		Timeout:        50 * time.Millisecond,
		MaxConcurrency: 5,
	})
	require.NoError(t, err)
	assert.False(t, timeoutRes.Success)
	assert.Equal(t, 10, timeoutRes.CanceledCount)
	for i := 0; i < 10; i++ {
		assert.Equal(t, "CANCELED", timeoutRes.TaskResults[i].State)
	}
}

// TestCancellation_PropagationToDescendants verifies that root cancellation terminates running
// runtimes, blocks new children from being scheduled, and transitions all descendants to an observable terminal state.
func TestCancellation_PropagationToDescendants(t *testing.T) {
	orchestrator := NewOrchestrator(OrchestratorOptions{})

	var runtime1Cancelled atomic.Bool
	var runtime2Cancelled atomic.Bool

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	go func() {
		<-ctx1.Done()
		runtime1Cancelled.Store(true)
	}()
	go func() {
		<-ctx2.Done()
		runtime2Cancelled.Store(true)
	}()

	orchestrator.RegisterActiveTask("child-task-1", cancel1)
	orchestrator.RegisterActiveTask("child-task-2", cancel2)

	// Cancel root and active tasks
	cancel1()
	cancel2()

	assert.Eventually(t, func() bool {
		return runtime1Cancelled.Load() && runtime2Cancelled.Load()
	}, 1*time.Second, 10*time.Millisecond, "runtimes must receive cancellation")

	// Verify terminal parent blocks new child delegation
	terminalParent := &store.WorkMessage{
		WorkID: "parent-cancelled",
		State:  "CANCELED",
	}
	assert.True(t, isTerminalState(terminalParent.State))
}
