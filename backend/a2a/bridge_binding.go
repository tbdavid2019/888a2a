package a2a

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// BindingState describes the manager-owned lifecycle of a bridge invocation.
// Native provider session identifiers are intentionally not part of this type.
type BindingState string

const (
	BindingActive  BindingState = "ACTIVE"
	BindingStopped BindingState = "STOPPED"
	BindingStale   BindingState = "STALE"
)

// BridgeBinding is safe manager metadata for one bridge invocation. It may be
// persisted by a caller, but it contains no provider secret, process handle,
// or native runtime session identifier.
type BridgeBinding struct {
	BindingID      string       `json:"bindingId"`
	OrganizationID string       `json:"organizationId"`
	CallerID       string       `json:"callerId"`
	TaskID         string       `json:"taskId"`
	ContextID      string       `json:"contextId"`
	CorrelationID  string       `json:"correlationId"`
	BridgeID       string       `json:"bridgeId"`
	State          BindingState `json:"state"`
	CreatedAt      time.Time    `json:"createdAt"`
	LastSeenAt     time.Time    `json:"lastSeenAt"`
	LeaseExpiresAt time.Time    `json:"leaseExpiresAt"`
}

// BindingRegistry tracks short-lived bridge leases in manager memory. A
// process restart starts with an empty registry, so bindings from a crashed
// manager cannot be resumed as if their provider process still existed.
type BindingRegistry struct {
	mu       sync.Mutex
	bindings map[string]BridgeBinding
	now      func() time.Time
}

// NewBindingRegistry creates an empty registry with the given clock. A nil
// clock uses time.Now and is useful for production use.
func NewBindingRegistry(now func() time.Time) *BindingRegistry {
	if now == nil {
		now = time.Now
	}
	return &BindingRegistry{bindings: make(map[string]BridgeBinding), now: now}
}

// Start records a new lease for a validated bridge request. A task cannot have
// two active bindings for the same bridge, which prevents a retry from
// accidentally creating two local runtime sessions.
func (r *BindingRegistry) Start(request BridgeRequest, lease time.Duration) (BridgeBinding, error) {
	if r == nil {
		return BridgeBinding{}, errors.New("binding registry is required")
	}
	if err := ValidateBridgeRequest(request, request.BridgeID); err != nil {
		return BridgeBinding{}, err
	}
	if lease <= 0 || lease > maxBridgeTimeout {
		return BridgeBinding{}, fmt.Errorf("binding lease must be between 1ns and %s", maxBridgeTimeout)
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconcileLocked(now)
	for _, binding := range r.bindings {
		if binding.OrganizationID == request.OrganizationID && binding.TaskID == request.TaskID && binding.BridgeID == request.BridgeID && binding.State == BindingActive {
			return BridgeBinding{}, errors.New("active bridge binding already exists")
		}
	}
	binding := BridgeBinding{
		BindingID:      request.BridgeID + ":" + request.TaskID,
		OrganizationID: request.OrganizationID,
		CallerID:       request.CallerID,
		TaskID:         request.TaskID,
		ContextID:      request.ContextID,
		CorrelationID:  request.CorrelationID,
		BridgeID:       request.BridgeID,
		State:          BindingActive,
		CreatedAt:      now,
		LastSeenAt:     now,
		LeaseExpiresAt: now.Add(lease),
	}
	r.bindings[binding.BindingID] = binding
	return binding, nil
}

// Get returns an active binding only when its tenant and caller match. Stale
// bindings are marked before they are returned and cannot be reused.
func (r *BindingRegistry) Get(bindingID, organizationID, callerID string) (BridgeBinding, bool) {
	if r == nil {
		return BridgeBinding{}, false
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconcileLocked(now)
	binding, ok := r.bindings[bindingID]
	if !ok || binding.State != BindingActive || binding.OrganizationID != organizationID || binding.CallerID != callerID {
		return BridgeBinding{}, false
	}
	binding.LastSeenAt = now
	r.bindings[bindingID] = binding
	return binding, true
}

// Stop closes a binding idempotently when the tenant matches.
func (r *BindingRegistry) Stop(bindingID, organizationID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	binding, ok := r.bindings[bindingID]
	if !ok || binding.OrganizationID != organizationID {
		return false
	}
	if binding.State == BindingActive {
		binding.State = BindingStopped
		binding.LastSeenAt = r.now()
		r.bindings[bindingID] = binding
	}
	return true
}

// Reconcile marks expired leases stale and returns their public metadata for
// durable task reconciliation. It never attempts to restart a provider.
func (r *BindingRegistry) Reconcile() []BridgeBinding {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconcileLocked(r.now())
}

func (r *BindingRegistry) reconcileLocked(now time.Time) []BridgeBinding {
	var stale []BridgeBinding
	for id, binding := range r.bindings {
		if binding.State == BindingActive && !now.Before(binding.LeaseExpiresAt) {
			binding.State = BindingStale
			binding.LastSeenAt = now
			r.bindings[id] = binding
			stale = append(stale, binding)
		}
	}
	return stale
}
