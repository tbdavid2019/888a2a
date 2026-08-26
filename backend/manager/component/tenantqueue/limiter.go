// Package tenantqueue provides bounded per-Organization execution admission.
package tenantqueue

import "sync"

type Item struct {
	OrganizationID string
	Value          any
}

type Queue struct {
	mu             sync.Mutex
	perTenantLimit int
	totalLimit     int
	items          map[string][]Item
	order          []string
	next           int
	total          int
}

func NewQueue(perTenantLimit, totalLimit int) *Queue {
	return &Queue{perTenantLimit: perTenantLimit, totalLimit: totalLimit, items: make(map[string][]Item)}
}

func (q *Queue) Enqueue(item Item) bool {
	if q == nil || item.OrganizationID == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.perTenantLimit <= 0 || q.totalLimit <= 0 || q.total >= q.totalLimit || len(q.items[item.OrganizationID]) >= q.perTenantLimit {
		return false
	}
	if len(q.items[item.OrganizationID]) == 0 {
		q.order = append(q.order, item.OrganizationID)
	}
	q.items[item.OrganizationID] = append(q.items[item.OrganizationID], item)
	q.total++
	return true
}

func (q *Queue) Dequeue() (Item, bool) {
	if q == nil {
		return Item{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.order) > 0 {
		if q.next >= len(q.order) {
			q.next = 0
		}
		org := q.order[q.next]
		items := q.items[org]
		if len(items) == 0 {
			q.order = append(q.order[:q.next], q.order[q.next+1:]...)
			continue
		}
		item := items[0]
		q.items[org] = items[1:]
		q.total--
		q.next++
		return item, true
	}
	return Item{}, false
}

type Limiter struct {
	mu           sync.Mutex
	perTenantMax int
	totalMax     int
	activeByOrg  map[string]int
	totalActive  int
}

func NewLimiter(perTenantMax, totalMax int) *Limiter {
	return &Limiter{
		perTenantMax: perTenantMax,
		totalMax:     totalMax,
		activeByOrg:  make(map[string]int),
	}
}

// TryAcquire admits one work unit and returns a release function. Admission
// never waits, so a saturated tenant cannot consume workers reserved for peers.
func (l *Limiter) TryAcquire(organizationID string) (func(), bool) {
	if l == nil || organizationID == "" {
		return nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.perTenantMax <= 0 || l.totalMax <= 0 ||
		l.activeByOrg[organizationID] >= l.perTenantMax || l.totalActive >= l.totalMax {
		return nil, false
	}
	l.activeByOrg[organizationID]++
	l.totalActive++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.activeByOrg[organizationID] > 0 {
				l.activeByOrg[organizationID]--
			}
			if l.totalActive > 0 {
				l.totalActive--
			}
		})
	}, true
}

func (l *Limiter) Active(organizationID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.activeByOrg[organizationID]
}
