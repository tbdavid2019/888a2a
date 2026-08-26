package messageplane

import (
	"context"
	"testing"
)

func TestPathModeValuesAndDefault(t *testing.T) {
	if PathModeLegacy == PathModeDual || PathModeDual == PathModeMessagePlane {
		t.Fatal("collaboration rollout modes must be distinct")
	}
	selector, err := NewPathSelector(nil)
	if err == nil {
		// A selector with no database is useful for unit-tested fail-safe callers,
		// but construction itself must reject it so production wiring is explicit.
		t.Fatal("expected nil database to be rejected")
	}
	if selector != nil {
		t.Fatal("selector must be nil when construction fails")
	}
	if _, err := (&PathSelector{}).Mode(context.Background(), "org-1"); err != nil {
		t.Fatalf("nil selector mode should fail safe without an error: %v", err)
	}
}
