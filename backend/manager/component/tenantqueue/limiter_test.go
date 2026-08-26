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

func TestQueueProvidesTenantFairnessAndBounds(t *testing.T) {
	queue := NewQueue(2, 3)
	if !queue.Enqueue(Item{OrganizationID: "org-a", Value: "a1"}) ||
		!queue.Enqueue(Item{OrganizationID: "org-a", Value: "a2"}) {
		t.Fatal("org-a items should be queued")
	}
	if queue.Enqueue(Item{OrganizationID: "org-a", Value: "a3"}) {
		t.Fatal("org-a queue must be bounded")
	}
	if !queue.Enqueue(Item{OrganizationID: "org-b", Value: "b1"}) {
		t.Fatal("org-b must retain an independent queue slot")
	}
	first, ok := queue.Dequeue()
	if !ok || first.OrganizationID != "org-a" {
		t.Fatalf("first dequeue = %+v, want org-a", first)
	}
	second, ok := queue.Dequeue()
	if !ok || second.OrganizationID != "org-b" {
		t.Fatalf("second dequeue = %+v, want org-b", second)
	}
}
