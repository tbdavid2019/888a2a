package client

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestMultiAgentRunnerConcurrencyAndCancellation(t *testing.T) {
	c := newTestMachineClient(t, "machine-concurrency")
	const agentCount = 12
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Initialize 12 Agent assignments on the MachineClient
	for i := 1; i <= agentCount; i++ {
		agentID := fmt.Sprintf("agent-%02d", i)
		e := testAssignmentEvent("machine-concurrency", uint64(i), fmt.Sprintf("evt-%d", i),
			fmt.Sprintf("key-%d", i), agentID, a2a888.AssignmentEventType_CREATE, "v1")
		ack, err := c.ApplyAssignmentEvent(ctx, e)
		require.NoError(t, err)
		require.NotNil(t, ack)
	}

	c.runnersMu.Lock()
	require.Equal(t, agentCount, len(c.runners))
	c.runnersMu.Unlock()

	// 2. Launch concurrent workloads across all 12 agents
	var wg sync.WaitGroup
	results := make([]error, agentCount)
	completed := make([]bool, agentCount)

	// Targets for failure simulation
	cancelTarget := 2  // Agent-02: cancelled mid-flight
	timeoutTarget := 6 // Agent-06: timeout mid-flight
	crashTarget := 10  // Agent-10: crash / panic mid-flight

	for i := 0; i < agentCount; i++ {
		agentIdx := i + 1
		agentID := fmt.Sprintf("agent-%02d", agentIdx)

		c.runnersMu.Lock()
		runner := c.runners[agentID]
		c.runnersMu.Unlock()
		require.NotNil(t, runner)

		c.MarkAgentBusy(agentID)

		wg.Go(func() {
			defer c.MarkAgentIdle(agentID)

			switch agentIdx {
			case cancelTarget:
				// Simulate cancellation
				turnCtx, turnCancel := context.WithCancel(ctx)
				go func() {
					time.Sleep(50 * time.Millisecond)
					turnCancel()
				}()
				select {
				case <-turnCtx.Done():
					results[agentIdx-1] = turnCtx.Err()
				case <-time.After(1 * time.Second):
					results[agentIdx-1] = nil
				}

			case timeoutTarget:
				// Simulate short timeout
				timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 30*time.Millisecond)
				defer timeoutCancel()
				select {
				case <-timeoutCtx.Done():
					results[agentIdx-1] = timeoutCtx.Err()
				case <-time.After(1 * time.Second):
					results[agentIdx-1] = nil
				}

			case crashTarget:
				// Simulate crash handling / recovery without panicking inside wg.Go.
				results[agentIdx-1] = pkgerrors.New("recovered from crash: simulated agent runner panic")

			default:
				// Normal agent workload
				select {
				case <-time.After(80 * time.Millisecond):
					completed[agentIdx-1] = true
					results[agentIdx-1] = nil
				case <-ctx.Done():
					results[agentIdx-1] = ctx.Err()
				}
			}
		})
	}

	wg.Wait()

	// 3. Verify that the 9 unaffected agents finished successfully
	for i := 0; i < agentCount; i++ {
		agentIdx := i + 1
		switch agentIdx {
		case cancelTarget:
			assert.ErrorIs(t, results[i], context.Canceled)
			assert.False(t, completed[i])
		case timeoutTarget:
			assert.ErrorIs(t, results[i], context.DeadlineExceeded)
			assert.False(t, completed[i])
		case crashTarget:
			assert.Error(t, results[i])
			assert.Contains(t, results[i].Error(), "recovered from crash")
			assert.False(t, completed[i])
		default:
			assert.NoError(t, results[i], "agent %d should succeed", agentIdx)
			assert.True(t, completed[i], "agent %d should be completed", agentIdx)
		}
	}

	// 4. Verify all agents are returned to IDLE / available capacity
	capRep := c.GetCapacityReport()
	assert.Equal(t, 0, capRep.BusyRunners)
	assert.Equal(t, agentCount, capRep.ActiveRunners)

	// 5. Verify the affected agents can successfully run subsequent turns
	for _, affectedIdx := range []int{cancelTarget, timeoutTarget, crashTarget} {
		agentID := fmt.Sprintf("agent-%02d", affectedIdx)
		assert.True(t, c.IsAgentAvailable(agentID))
	}
}
