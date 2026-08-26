package a2a

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func TestValidateDelegationLimits(t *testing.T) {
	parent := &store.WorkMessage{
		MaxDepth:     3,
		MaxChildren:  5,
		UsedChildren: 4,
	}

	// Valid depth and child count
	err := ValidateDelegationLimits(parent, 2)
	assert.NoError(t, err)

	// Exceeds depth
	err = ValidateDelegationLimits(parent, 4)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum delegation depth exceeded")

	// Exceeds max children
	parent.UsedChildren = 5
	err = ValidateDelegationLimits(parent, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum child count exceeded")
}

func TestValidateFanOutLimit(t *testing.T) {
	parent := &store.WorkMessage{
		MaxFanOut:    10,
		MaxChildren:  20,
		UsedChildren: 12,
	}

	// Valid fan out (8 tasks, remaining capacity 8)
	err := ValidateFanOutLimit(parent, 8)
	assert.NoError(t, err)

	// Exceeds max fan out (12 > 10)
	err = ValidateFanOutLimit(parent, 12)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum fan-out exceeded")

	// Exceeds child limit (9 <= 10 fan out, but 12 + 9 > 20 children)
	err = ValidateFanOutLimit(parent, 9)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum child count exceeded")
}

func TestValidateConcurrencyLimit(t *testing.T) {
	parent := &store.WorkMessage{
		MaxConcurrency: 4,
	}

	// 3 active + 1 = 4 <= 4: OK
	assert.NoError(t, ValidateConcurrencyLimit(parent, 3))

	// 4 active + 1 = 5 > 4: Error
	err := ValidateConcurrencyLimit(parent, 4)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum concurrency exceeded")
}

func TestValidateRetryLimit(t *testing.T) {
	work := &store.WorkMessage{
		MaxRetries: 2,
		RetryCount: 1,
	}

	// 1 + 1 = 2 <= 2: OK
	assert.NoError(t, ValidateRetryLimit(work))

	// 2 + 1 = 3 > 2: Error
	work.RetryCount = 2
	err := ValidateRetryLimit(work)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "maximum retries exceeded")
}

func TestValidateBudgetAvailability(t *testing.T) {
	parent := &store.WorkMessage{
		MaxTokens:     10000,
		UsedTokens:    6000, // remaining 4000
		MaxWorkUnits:  100,
		UsedWorkUnits: 80, // remaining 20
	}

	// Requesting 3000 tokens, 15 units: OK
	assert.NoError(t, ValidateBudgetAvailability(parent, 3000, 15))

	// Requesting 5000 tokens (> 4000): Error
	err := ValidateBudgetAvailability(parent, 5000, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "token budget exceeded")

	// Requesting 25 work units (> 20): Error
	err = ValidateBudgetAvailability(parent, 1000, 25)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyLimitExceeded)
	assert.Contains(t, err.Error(), "work unit budget exceeded")
}

func TestAllocateChildBudget_BoundedByParent(t *testing.T) {
	parent := &store.WorkMessage{
		MaxDepth:       4,
		MaxChildren:    10,
		UsedChildren:   3,
		MaxFanOut:      6,
		MaxConcurrency: 4,
		MaxRuntimeMs:   60000,
		MaxRetries:     2,
		MaxTokens:      50000,
		UsedTokens:     20000, // 30000 remaining
		MaxWorkUnits:   200,
		UsedWorkUnits:  50, // 150 remaining
	}

	requested := &WorkBudget{
		MaxTokens:    100000, // Requests more than remaining
		MaxWorkUnits: 100,
	}

	childBudget := AllocateChildBudget(parent, requested)
	assert.Equal(t, int32(4), childBudget.MaxDepth)
	assert.Equal(t, int32(6), childBudget.MaxFanOut)
	assert.Equal(t, int64(30000), childBudget.MaxTokens, "child tokens must be capped by parent remaining")
	assert.Equal(t, int64(100), childBudget.MaxWorkUnits)
}
