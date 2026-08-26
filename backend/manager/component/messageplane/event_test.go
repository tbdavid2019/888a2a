package messageplane

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectEventsAppliesAppendOnlyVisibleState(t *testing.T) {
	events := []CollaborationEvent{
		{MessageID: "message-1", Type: EventMessageCreated, Payload: []byte(`{"content":"before"}`)},
		{MessageID: "message-1", Type: EventMessageEdited, Payload: []byte(`{"content":"after"}`)},
		{MessageID: "message-1", ActorID: "user-1", Type: EventReactionAdded, Payload: []byte(`{"emoji":"👍"}`)},
		{MessageID: "message-1", Type: EventMessageRecalled},
		{MessageID: "message-1", Type: EventMessageEdited, Payload: []byte(`{"content":"must stay hidden"}`)},
	}
	views, err := ProjectEvents(events)
	require.NoError(t, err)
	require.True(t, views["message-1"].Recalled)
	require.Empty(t, views["message-1"].Content)
	require.True(t, views["message-1"].Reactions["👍"]["user-1"])
}

func TestProjectEventsRejectsUnknownOrMalformedMutation(t *testing.T) {
	_, err := ProjectEvents([]CollaborationEvent{{MessageID: "message-1", Type: EventType("UNKNOWN")}})
	require.Error(t, err)
	_, err = ProjectEvents([]CollaborationEvent{{MessageID: "message-1", Type: EventMessageCreated, Payload: []byte("not-json")}})
	require.Error(t, err)
}
