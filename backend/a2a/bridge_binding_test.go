package a2a

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBindingRegistryScopesAndStopsBindings(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	clock := now
	registry := NewBindingRegistry(func() time.Time { return clock })
	request := validBridgeRequest()
	binding, err := registry.Start(request, time.Minute)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := registry.Get(binding.BindingID, "other-org", request.CallerID); ok {
		t.Fatal("cross-tenant binding lookup must fail")
	}
	if _, ok := registry.Get(binding.BindingID, request.OrganizationID, "other-caller"); ok {
		t.Fatal("cross-caller binding lookup must fail")
	}
	if _, ok := registry.Get(binding.BindingID, request.OrganizationID, request.CallerID); !ok {
		t.Fatal("matching binding lookup must succeed")
	}
	if !registry.Stop(binding.BindingID, request.OrganizationID) || !registry.Stop(binding.BindingID, request.OrganizationID) {
		t.Fatal("stopping a binding must be idempotent")
	}
	if _, ok := registry.Get(binding.BindingID, request.OrganizationID, request.CallerID); ok {
		t.Fatal("stopped binding must not be returned as active")
	}
}

func TestBindingRegistryMarksExpiredBindingsStale(t *testing.T) {
	clock := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	registry := NewBindingRegistry(func() time.Time { return clock })
	request := validBridgeRequest()
	binding, err := registry.Start(request, time.Minute)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	clock = clock.Add(time.Minute)
	stale := registry.Reconcile()
	if len(stale) != 1 || stale[0].State != BindingStale {
		t.Fatalf("stale bindings = %+v", stale)
	}
	if _, ok := registry.Get(binding.BindingID, request.OrganizationID, request.CallerID); ok {
		t.Fatal("expired binding must not be reusable")
	}
}

func TestBindingRegistryRestartDoesNotRestoreProviderSession(t *testing.T) {
	request := validBridgeRequest()
	first := NewBindingRegistry(nil)
	binding, err := first.Start(request, time.Minute)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	second := NewBindingRegistry(nil)
	if _, ok := second.Get(binding.BindingID, request.OrganizationID, request.CallerID); ok {
		t.Fatal("new registry must not restore a provider session after restart")
	}

	encoded, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{"token", "secret", "password", "session_id", "native_session"} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("binding metadata leaked %q: %s", forbidden, serialized)
		}
	}
}
