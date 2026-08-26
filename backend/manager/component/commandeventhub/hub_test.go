package commandeventhub

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHubNotifiesOnlyMatchingCommand(t *testing.T) {
	hub := New()
	commandID := uuid.New()
	otherID := uuid.New()
	waiter := hub.Subscribe(commandID)
	other := hub.Subscribe(otherID)

	hub.NotifyCommand(commandID)
	select {
	case <-waiter:
	case <-time.After(time.Second):
		t.Fatal("matching command waiter was not notified")
	}
	select {
	case <-other:
		t.Fatal("different command waiter was notified")
	default:
	}
	hub.Unsubscribe(commandID, waiter)
	require.NotPanics(t, func() { hub.NotifyCommand(commandID) })
}
