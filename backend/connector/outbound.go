package connector

import (
	"errors"
	"sync"
	"time"
)

// DeliveryScheduler keeps rate-limit state per installation. The returned
// time is persisted as an outbox availability time by the caller.
type DeliveryScheduler struct {
	mu       sync.Mutex
	nextByID map[string]time.Time
}

func NewDeliveryScheduler() *DeliveryScheduler {
	return &DeliveryScheduler{nextByID: make(map[string]time.Time)}
}

func (s *DeliveryScheduler) Schedule(installationID string, now time.Time, retryAfter time.Duration) (time.Time, error) {
	if s == nil || installationID == "" {
		return time.Time{}, errors.New("installation id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if retryAfter < 0 {
		return time.Time{}, errors.New("retry delay cannot be negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	availableAt := now.UTC()
	if previous := s.nextByID[installationID]; previous.After(availableAt) {
		availableAt = previous
	}
	if retryAfter > 0 {
		candidate := now.UTC().Add(retryAfter)
		if candidate.After(availableAt) {
			availableAt = candidate
		}
	}
	s.nextByID[installationID] = availableAt
	return availableAt, nil
}

func ClassifyDeliveryFailure(statusCode int, retryAfter time.Duration) DeliveryResult {
	if statusCode >= 200 && statusCode < 300 {
		return DeliveryResult{}
	}
	if statusCode == 408 || statusCode == 429 || statusCode >= 500 {
		return DeliveryResult{RetryAt: time.Now().UTC().Add(retryAfter), Reason: "retryable connector delivery failure"}
	}
	return DeliveryResult{Terminal: true, Reason: "terminal connector delivery failure"}
}
