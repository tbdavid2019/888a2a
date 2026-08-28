package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

func TestGatewayActiveSubscribeReceivesBridgeEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	taskStore := taskstore.NewInMemory(&taskstore.InMemoryStoreConfig{
		Authenticator: func(_ context.Context) (string, error) { return "bridge-caller", nil },
	})
	bridge := &fakeBridge{session: &fakeBridgeSession{result: BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  "subscribed bridge output",
		Events:  []BridgeEvent{{Sequence: 1, Kind: "delta", Text: "bridge delta"}},
	}}}
	bridge.waitForRelease = true
	gateway := NewGateway(GatewayOptions{
		TaskStore: taskStore,
		Authenticate: func(ctx context.Context, _ *http.Request, tenant, _ string) (context.Context, error) {
			return WithCaller(ctx, &fakeCaller{id: "bridge-caller", tenant: tenant, authenticated: true}), nil
		},
		ExecutorFactory: func(agentID string) a2asrv.AgentExecutor {
			executor, err := NewBridgeAgentExecutor(agentID, bridge)
			if err != nil {
				t.Fatalf("NewBridgeAgentExecutor: %v", err)
			}
			return executor
		},
	})
	server := httptest.NewServer(gateway)
	defer server.Close()
	client, err := a2aclient.NewFromCard(ctx, &a2a.AgentCard{
		Name: "bridge-agent", Version: "1.0",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(server.URL+"/a2a/v1/org-a/agents/bridge-agent", a2a.TransportProtocolHTTPJSON)},
		Capabilities:        a2a.AgentCapabilities{Streaming: true}, DefaultInputModes: []string{"text/plain"}, DefaultOutputModes: []string{"text/plain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Destroy() }()

	taskIDCh := make(chan a2a.TaskID, 1)
	sendDone := make(chan error, 1)
	go func() {
		for event, streamErr := range client.SendStreamingMessage(ctx, &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("subscribe while running"))}) {
			if streamErr != nil {
				sendDone <- streamErr
				return
			}
			if task, ok := event.(*a2a.Task); ok && task.ID != "" {
				taskIDCh <- task.ID
			}
		}
		sendDone <- nil
	}()
	taskID := <-taskIDCh

	subDone := make(chan int, 1)
	go func() {
		count := 0
		for event, streamErr := range client.SubscribeToTask(ctx, &a2a.SubscribeToTaskRequest{ID: taskID}) {
			if streamErr != nil {
				subDone <- -1
				return
			}
			if event != nil {
				count++
			}
		}
		subDone <- count
	}()
	time.Sleep(50 * time.Millisecond)
	bridge.Release()
	if count := <-subDone; count < 1 {
		t.Fatalf("active subscription event count = %d, want at least one", count)
	}
	if err := <-sendDone; err != nil {
		t.Fatalf("stream send: %v", err)
	}
}
