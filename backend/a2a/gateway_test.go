package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

func TestGateway_ProtocolVersionNegotiation(t *testing.T) {
	gw := NewGateway(GatewayOptions{
		BaseURL: "http://localhost:8181",
	})
	server := httptest.NewServer(gw)
	defer server.Close()

	client := server.Client()

	t.Run("default version 1.0", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/.well-known/agent-card.json", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		if v := resp.Header.Get(a2a.SvcParamVersion); v != "1.0" {
			t.Errorf("expected A2A-Version header '1.0', got %q", v)
		}
	})

	t.Run("explicit version 1.0 supported", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/.well-known/agent-card.json", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set(a2a.SvcParamVersion, "1.0")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		if v := resp.Header.Get(a2a.SvcParamVersion); v != "1.0" {
			t.Errorf("expected A2A-Version header '1.0', got %q", v)
		}
	})

	t.Run("incompatible version 2.0 rejected", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/.well-known/agent-card.json", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set(a2a.SvcParamVersion, "2.0")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})
}

func TestGateway_ExternalSDKConnection(t *testing.T) {
	ctx := context.Background()

	taskStore := taskstore.NewInMemory(&taskstore.InMemoryStoreConfig{
		Authenticator: func(_ context.Context) (string, error) {
			return "test-user", nil
		},
	})
	gw := NewGateway(GatewayOptions{
		TaskStore: taskStore,
		BaseURL:   "http://localhost:8181",
	})
	server := httptest.NewServer(gw)
	defer server.Close()

	// Build standard A2A AgentCard pointing to the server
	card := &a2a.AgentCard{
		Name:        "test-agent",
		Description: "A2A Gateway Test Agent",
		Version:     "1.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(server.URL, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}

	// Create official A2A SDK client from card
	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		t.Fatalf("a2aclient.NewFromCard failed: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	// 1. Send message via official SDK client
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello peer agent"))
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("client.SendMessage failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil SendMessageResult")
	}

	// Extract Task from result
	task, ok := result.(*a2a.Task)
	if !ok {
		t.Fatalf("expected result to be *a2a.Task, got %T", result)
	}

	if task.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected task state COMPLETED, got %s", task.Status.State)
	}

	// 2. GetTask via official SDK client
	fetchedTask, err := client.GetTask(ctx, &a2a.GetTaskRequest{ID: task.ID})
	if err != nil {
		t.Fatalf("client.GetTask failed: %v", err)
	}
	if fetchedTask.ID != task.ID {
		t.Errorf("expected fetched task ID %q, got %q", task.ID, fetchedTask.ID)
	}

	// 3. ListTasks via official SDK client
	listResp, err := client.ListTasks(ctx, &a2a.ListTasksRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("client.ListTasks failed: %v", err)
	}
	if len(listResp.Tasks) == 0 {
		t.Fatalf("expected at least 1 task in ListTasks response")
	}

	// 4. CancelTask on an active working task
	workingTaskID := a2a.TaskID("task-cancel-test")
	_, err = taskStore.Create(ctx, &a2a.Task{
		ID:        workingTaskID,
		ContextID: "ctx-cancel",
		Status: a2a.TaskStatus{
			State: a2a.TaskStateWorking,
		},
	})
	if err != nil {
		t.Fatalf("taskStore.Create working task failed: %v", err)
	}

	cancelTask, err := client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: workingTaskID})
	if err != nil {
		t.Fatalf("client.CancelTask failed: %v", err)
	}
	if cancelTask.Status.State != a2a.TaskStateCanceled {
		t.Errorf("expected state CANCELED on cancel, got %s", cancelTask.Status.State)
	}
}

func TestGateway_TenantNamespacedRouting(t *testing.T) {
	ctx := context.Background()

	taskStore := taskstore.NewInMemory(nil)
	gw := NewGateway(GatewayOptions{
		TaskStore: taskStore,
		BaseURL:   "http://localhost:8181",
	})
	server := httptest.NewServer(gw)
	defer server.Close()

	card := &a2a.AgentCard{
		Name:        "specialist-agent",
		Description: "Specialist in Tenant Beta",
		Version:     "1.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(server.URL+"/a2a/v1/tenant-beta/agents/specialist-1", a2a.TransportProtocolHTTPJSON),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}

	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		t.Fatalf("NewFromCard: %v", err)
	}
	defer func() { _ = client.Destroy() }()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("tenant task test"))
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("SendMessage on namespaced route failed: %v", err)
	}

	task, ok := result.(*a2a.Task)
	if !ok {
		t.Fatalf("expected *a2a.Task, got %T", result)
	}
	if task.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected state COMPLETED, got %s", task.Status.State)
	}
	if task.Status.Message != nil {
		txt := task.Status.Message.Parts[0].Text()
		if !strings.Contains(txt, "specialist-1") {
			t.Errorf("expected response to reflect specialist-1 executor, got %q", txt)
		}
	}
}
