package a2a

import (
	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/manager/store"
)

// Default safety limits for bounded orchestration.
const (
	DefaultMaxDepth       int32 = 8
	DefaultMaxChildren    int32 = 32
	DefaultMaxFanOut      int32 = 16
	DefaultMaxConcurrency int32 = 10
	DefaultMaxRuntimeMs   int64 = 300000 // 5 minutes
	DefaultMaxRetries     int32 = 3
	DefaultMaxTokens      int64 = 1000000
	DefaultMaxWorkUnits   int64 = 1000
)

var (
	// ErrPolicyLimitExceeded is the general policy limit error.
	ErrPolicyLimitExceeded = errors.New("policy limit exceeded")
	// ErrMaxDepthExceeded indicates maximum delegation depth was reached.
	ErrMaxDepthExceeded = errors.Wrap(ErrPolicyLimitExceeded, "maximum delegation depth exceeded")
	// ErrMaxChildrenExceeded indicates maximum child count was reached.
	ErrMaxChildrenExceeded = errors.Wrap(ErrPolicyLimitExceeded, "maximum child count exceeded")
	// ErrMaxFanOutExceeded indicates maximum fan-out width was reached.
	ErrMaxFanOutExceeded = errors.Wrap(ErrPolicyLimitExceeded, "maximum fan-out exceeded")
	// ErrMaxConcurrencyExceeded indicates coordinator concurrency limit was reached.
	ErrMaxConcurrencyExceeded = errors.Wrap(ErrPolicyLimitExceeded, "maximum concurrency exceeded")
	// ErrMaxRetriesExceeded indicates maximum retry attempts was reached.
	ErrMaxRetriesExceeded = errors.Wrap(ErrPolicyLimitExceeded, "maximum retries exceeded")
	// ErrTokenBudgetExceeded indicates token budget was exhausted.
	ErrTokenBudgetExceeded = errors.Wrap(ErrPolicyLimitExceeded, "token budget exceeded")
	// ErrWorkUnitBudgetExceeded indicates work unit budget was exhausted.
	ErrWorkUnitBudgetExceeded = errors.Wrap(ErrPolicyLimitExceeded, "work unit budget exceeded")
	// ErrCyclicDelegation indicates a cyclic delegation dependency was detected.
	ErrCyclicDelegation = errors.New("cyclic delegation detected")
)

// WorkBudget defines the execution limits for a work record or task tree.
type WorkBudget struct {
	MaxDepth       int32 `json:"max_depth"`
	MaxChildren    int32 `json:"max_children"`
	MaxFanOut      int32 `json:"max_fan_out"`
	MaxConcurrency int32 `json:"max_concurrency"`
	MaxRuntimeMs   int64 `json:"max_runtime_ms"`
	MaxRetries     int32 `json:"max_retries"`
	MaxTokens      int64 `json:"max_tokens"`
	MaxWorkUnits   int64 `json:"max_work_units"`
}

// WorkUsage tracks consumed budget resources.
type WorkUsage struct {
	Depth     int32 `json:"depth"`
	Children  int32 `json:"children"`
	FanOut    int32 `json:"fan_out"`
	RuntimeMs int64 `json:"runtime_ms"`
	Retries   int32 `json:"retries"`
	Tokens    int64 `json:"tokens"`
	WorkUnits int64 `json:"work_units"`
}

// DefaultBudget returns a WorkBudget populated with safe standard defaults.
func DefaultBudget() *WorkBudget {
	return &WorkBudget{
		MaxDepth:       DefaultMaxDepth,
		MaxChildren:    DefaultMaxChildren,
		MaxFanOut:      DefaultMaxFanOut,
		MaxConcurrency: DefaultMaxConcurrency,
		MaxRuntimeMs:   DefaultMaxRuntimeMs,
		MaxRetries:     DefaultMaxRetries,
		MaxTokens:      DefaultMaxTokens,
		MaxWorkUnits:   DefaultMaxWorkUnits,
	}
}

// EffectiveBudget merges explicit limits with defaults for unconfigured (0) fields.
func EffectiveBudget(b *WorkBudget) *WorkBudget {
	def := DefaultBudget()
	if b == nil {
		return def
	}
	res := *b
	if res.MaxDepth <= 0 {
		res.MaxDepth = def.MaxDepth
	}
	if res.MaxChildren <= 0 {
		res.MaxChildren = def.MaxChildren
	}
	if res.MaxFanOut <= 0 {
		res.MaxFanOut = def.MaxFanOut
	}
	if res.MaxConcurrency <= 0 {
		res.MaxConcurrency = def.MaxConcurrency
	}
	if res.MaxRuntimeMs <= 0 {
		res.MaxRuntimeMs = def.MaxRuntimeMs
	}
	if res.MaxRetries <= 0 {
		res.MaxRetries = def.MaxRetries
	}
	if res.MaxTokens <= 0 {
		res.MaxTokens = def.MaxTokens
	}
	if res.MaxWorkUnits <= 0 {
		res.MaxWorkUnits = def.MaxWorkUnits
	}
	return &res
}

// ValidateDelegationLimits verifies that creating a child does not violate parent depth or child count limits.
func ValidateDelegationLimits(parent *store.WorkMessage, proposedChildDepth int32) error {
	if parent == nil {
		return nil
	}

	maxDepth := parent.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	if proposedChildDepth > maxDepth {
		return errors.Wrapf(ErrMaxDepthExceeded, "depth %d exceeds maximum depth %d", proposedChildDepth, maxDepth)
	}

	maxChildren := parent.MaxChildren
	if maxChildren <= 0 {
		maxChildren = DefaultMaxChildren
	}
	if parent.UsedChildren+1 > maxChildren {
		return errors.Wrapf(ErrMaxChildrenExceeded, "used children %d + 1 exceeds maximum %d", parent.UsedChildren, maxChildren)
	}

	return nil
}

// ValidateFanOutLimit verifies that a batch fan-out request fits within the parent's fan-out and child count limits.
func ValidateFanOutLimit(parent *store.WorkMessage, count int) error {
	if parent == nil || count <= 0 {
		return nil
	}

	maxFanOut := parent.MaxFanOut
	if maxFanOut <= 0 {
		maxFanOut = DefaultMaxFanOut
	}
	if int32(count) > maxFanOut {
		return errors.Wrapf(ErrMaxFanOutExceeded, "requested fan-out %d exceeds maximum %d", count, maxFanOut)
	}

	maxChildren := parent.MaxChildren
	if maxChildren <= 0 {
		maxChildren = DefaultMaxChildren
	}
	if parent.UsedChildren+int32(count) > maxChildren {
		return errors.Wrapf(ErrMaxChildrenExceeded, "spawning %d children would exceed remaining limit (%d/%d)", count, parent.UsedChildren, maxChildren)
	}

	return nil
}

// ValidateConcurrencyLimit checks if running one more concurrent task violates max concurrency.
func ValidateConcurrencyLimit(parent *store.WorkMessage, activeCount int) error {
	if parent == nil {
		return nil
	}

	maxConc := parent.MaxConcurrency
	if maxConc <= 0 {
		maxConc = DefaultMaxConcurrency
	}
	if int32(activeCount+1) > maxConc {
		return errors.Wrapf(ErrMaxConcurrencyExceeded, "active children %d + 1 exceeds maximum concurrency %d", activeCount, maxConc)
	}
	return nil
}

// ValidateRetryLimit checks if another retry attempt is permitted.
func ValidateRetryLimit(work *store.WorkMessage) error {
	if work == nil {
		return nil
	}

	maxRetries := work.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	if work.RetryCount+1 > maxRetries {
		return errors.Wrapf(ErrMaxRetriesExceeded, "retry %d + 1 exceeds maximum retries %d", work.RetryCount, maxRetries)
	}
	return nil
}

// ValidateBudgetAvailability verifies that child requested tokens and work units do not exceed remaining parent budget.
func ValidateBudgetAvailability(parent *store.WorkMessage, requestedTokens, requestedUnits int64) error {
	if parent == nil {
		return nil
	}

	if parent.MaxTokens > 0 && requestedTokens > 0 {
		remainingTokens := parent.MaxTokens - parent.UsedTokens
		if remainingTokens < 0 {
			remainingTokens = 0
		}
		if requestedTokens > remainingTokens {
			return errors.Wrapf(ErrTokenBudgetExceeded, "requested tokens %d exceeds parent remaining tokens %d", requestedTokens, remainingTokens)
		}
	}

	if parent.MaxWorkUnits > 0 && requestedUnits > 0 {
		remainingUnits := parent.MaxWorkUnits - parent.UsedWorkUnits
		if remainingUnits < 0 {
			remainingUnits = 0
		}
		if requestedUnits > remainingUnits {
			return errors.Wrapf(ErrWorkUnitBudgetExceeded, "requested work units %d exceeds parent remaining units %d", requestedUnits, remainingUnits)
		}
	}

	return nil
}

// AllocateChildBudget creates a safe child budget bounded by the parent's remaining budget.
func AllocateChildBudget(parent *store.WorkMessage, requested *WorkBudget) *WorkBudget {
	req := EffectiveBudget(requested)
	if parent == nil {
		return req
	}

	parentBudget := EffectiveBudget(&WorkBudget{
		MaxDepth:       parent.MaxDepth,
		MaxChildren:    parent.MaxChildren,
		MaxFanOut:      parent.MaxFanOut,
		MaxConcurrency: parent.MaxConcurrency,
		MaxRuntimeMs:   parent.MaxRuntimeMs,
		MaxRetries:     parent.MaxRetries,
		MaxTokens:      parent.MaxTokens,
		MaxWorkUnits:   parent.MaxWorkUnits,
	})

	child := &WorkBudget{
		MaxDepth:       parentBudget.MaxDepth,
		MaxChildren:    minInt32(req.MaxChildren, parentBudget.MaxChildren),
		MaxFanOut:      minInt32(req.MaxFanOut, parentBudget.MaxFanOut),
		MaxConcurrency: minInt32(req.MaxConcurrency, parentBudget.MaxConcurrency),
		MaxRuntimeMs:   minInt64(req.MaxRuntimeMs, parentBudget.MaxRuntimeMs),
		MaxRetries:     minInt32(req.MaxRetries, parentBudget.MaxRetries),
	}

	if parent.MaxTokens > 0 {
		remainingTokens := parent.MaxTokens - parent.UsedTokens
		if remainingTokens < 0 {
			remainingTokens = 0
		}
		child.MaxTokens = minInt64(req.MaxTokens, remainingTokens)
	} else {
		child.MaxTokens = req.MaxTokens
	}

	if parent.MaxWorkUnits > 0 {
		remainingUnits := parent.MaxWorkUnits - parent.UsedWorkUnits
		if remainingUnits < 0 {
			remainingUnits = 0
		}
		child.MaxWorkUnits = minInt64(req.MaxWorkUnits, remainingUnits)
	} else {
		child.MaxWorkUnits = req.MaxWorkUnits
	}

	return child
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
