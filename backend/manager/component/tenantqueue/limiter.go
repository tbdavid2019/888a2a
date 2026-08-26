// Package tenantqueue provides bounded per-Organization execution admission.
package tenantqueue

import "sync"

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
