package messageplane

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
)

// TestMessagePlaneHotChannelLoadGate is an opt-in capacity gate. It exercises
// concurrent append and idempotent retry against a real PostgreSQL plane; it
// does not claim to represent a production capacity result until run with a
// declared target and a controlled environment.
func TestMessagePlaneHotChannelLoadGate(t *testing.T) {
	if os.Getenv("A2A888_RUN_LOAD_TESTS") != "1" {
		t.Skip("set A2A888_RUN_LOAD_TESTS=1 to run the MessagePlane load gate")
	}
	count := loadGateInt(t, "A2A888_LOAD_COUNT", 1000)
	workers := loadGateInt(t, "A2A888_LOAD_WORKERS", 32)
	maxSeconds := loadGateInt(t, "A2A888_LOAD_MAX_SECONDS", 30)
	require.LessOrEqual(t, count, 1000, "A2A888_LOAD_COUNT must fit one MessagePlane history page")

	db := requireMessagePlaneDatabase(t)
	plane, err := NewPostgresPlane(db)
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "default")
	conversationID := fmt.Sprintf("load-%d", time.Now().UnixNano())

	started := time.Now()
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	errorsCh := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, appendErr := plane.Append(ctx, MessageInput{
				OrganizationID: "default", ConversationID: conversationID,
				ClientMessageNo: fmt.Sprintf("load-client-%d", index), SenderID: "load-user",
				Payload: []byte(fmt.Sprintf(`{"index":%d}`, index)),
			})
			if appendErr != nil {
				errorsCh <- appendErr
			}
		}(i)
	}
	wg.Wait()
	close(errorsCh)
	for appendErr := range errorsCh {
		require.NoError(t, appendErr)
	}

	retry, err := plane.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: conversationID,
		ClientMessageNo: "load-client-0", SenderID: "load-user", Payload: []byte(`{"index":0}`),
	})
	require.NoError(t, err)
	require.NotEmpty(t, retry.MessageID)
	history, err := plane.History(ctx, HistoryRequest{OrganizationID: "default", ConversationID: conversationID, Limit: count})
	require.NoError(t, err)
	require.Len(t, history.Messages, count)
	require.LessOrEqual(t, time.Since(started), time.Duration(maxSeconds)*time.Second)
}

func loadGateInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	require.NoErrorf(t, err, "%s must be an integer", name)
	require.Positive(t, parsed)
	return parsed
}
