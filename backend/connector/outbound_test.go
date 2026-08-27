package connector

import (
	"testing"
	"time"
)

func TestDeliverySchedulerIsolatedPerInstallation(t *testing.T) {
	scheduler := NewDeliveryScheduler()
	now := time.Unix(100, 0)
	first, err := scheduler.Schedule("install-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scheduler.Schedule("install-b", now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !first.After(second) {
		t.Fatalf("installation schedules crossed: %v %v", first, second)
	}
}

func TestDeliveryFailureClassification(t *testing.T) {
	if result := ClassifyDeliveryFailure(429, time.Minute); result.Terminal || result.RetryAt.IsZero() {
		t.Fatal("rate limit must be retryable")
	}
	if result := ClassifyDeliveryFailure(400, 0); !result.Terminal {
		t.Fatal("bad request must be terminal")
	}
}
