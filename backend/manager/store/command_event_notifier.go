package store

import "github.com/google/uuid"

// CommandEventNotifier wakes watchers after a command event is durably
// appended. Implementations may be local or backed by a shared transport.
// Notifications are hints; the command_event table remains authoritative.
type CommandEventNotifier interface {
	NotifyCommand(commandID uuid.UUID)
}

// SetCommandEventNotifier injects the shared command-event wake-up boundary.
func (s *Store) SetCommandEventNotifier(n CommandEventNotifier) {
	s.commandEventNotifier = n
}
