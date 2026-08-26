package tenantqueue

import "testing"

func TestLimiterIsolatesTenantCapacity(t *testing.T) {
	limiter := NewLimiter(2, 3)
	releaseA1, ok := limiter.TryAcquire("org-a")
	if !ok {
		t.Fatal("first org-a unit should be admitted")
	}
	releaseA2, ok := limiter.TryAcquire("org-a")
	if !ok {
		t.Fatal("second org-a unit should be admitted")
	}
	if _, ok := limiter.TryAcquire("org-a"); ok {
		t.Fatal("org-a must be bounded at two active units")
	}
	releaseB, ok := limiter.TryAcquire("org-b")
	if !ok {
		t.Fatal("org-b must retain an independent worker slot")
	}
	releaseA1()
	releaseA1()
	releaseA2()
	releaseB()
	if got := limiter.Active("org-a"); got != 0 {
		t.Fatalf("org-a active units = %d, want 0", got)
	}
}
