package a2a

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func TestBridgeAgentExecutorTranslatesBridgeOutputToA2AEvents(t *testing.T) {
	bridge := &fakeBridge{session: &fakeBridgeSession{result: BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  "bridge result",
		Events:  []BridgeEvent{{Sequence: 1, Kind: "delta", Text: "partial"}},
	}}}
	executor, err := NewBridgeAgentExecutor("agent-a", bridge)
	if err != nil {
		t.Fatalf("NewBridgeAgentExecutor: %v", err)
	}
	ctx := WithCaller(context.Background(), &fakeCaller{id: "caller-a", tenant: "org-a", authenticated: true})
	execCtx := &a2asrv.ExecutorContext{
		Message:   &sdk.Message{ID: "message-a", Parts: sdk.ContentParts{sdk.NewTextPart("hello")}},
		TaskID:    sdk.TaskID("task-a"),
		ContextID: "context-a",
		Tenant:    "org-a",
	}
	var events []sdk.Event
	for event, err := range executor.Execute(ctx, execCtx) {
		if err != nil {
			t.Fatalf("Execute event error: %v", err)
		}
		events = append(events, event)
	}
	if len(events) != 4 {
		t.Fatalf("events = %d, want submitted, working, artifact, completed", len(events))
	}
	if _, ok := events[0].(*sdk.Task); !ok {
		t.Fatalf("event 0 = %T, want submitted task", events[0])
	}
	if _, ok := events[2].(*sdk.TaskArtifactUpdateEvent); !ok {
		t.Fatalf("event 2 = %T, want artifact update", events[2])
	}
	completed, ok := events[3].(*sdk.TaskStatusUpdateEvent)
	if !ok || completed.Status.State != sdk.TaskStateCompleted || completed.Status.Message == nil || completed.Status.Message.Parts[0].Text() != "bridge result" {
		t.Fatalf("completed event = %#v", events[3])
	}
}

func TestBridgeAgentExecutorRejectsUnauthenticatedCallerBeforeBridge(t *testing.T) {
	bridge := &fakeBridge{session: &fakeBridgeSession{}, startErr: errTestBridgeStart}
	executor, err := NewBridgeAgentExecutor("agent-a", bridge)
	if err != nil {
		t.Fatalf("NewBridgeAgentExecutor: %v", err)
	}
	execCtx := &a2asrv.ExecutorContext{
		Message:   &sdk.Message{ID: "message-a", Parts: sdk.ContentParts{sdk.NewTextPart("hello")}},
		TaskID:    sdk.TaskID("task-a"),
		ContextID: "context-a",
		Tenant:    "org-a",
	}
	for event, err := range executor.Execute(context.Background(), execCtx) {
		if err == nil || !strings.Contains(err.Error(), "authenticated A2A caller") {
			t.Fatalf("unexpected unauthenticated execution result: event=%v err=%v", event, err)
		}
	}
}

func TestBridgeAgentExecutorCancelTargetsOnlyTaskSession(t *testing.T) {
	bridge := &fakeBridge{session: &fakeBridgeSession{result: BridgeResult{Outcome: DeliveryOutcomeUnknown}}}
	executor, err := NewBridgeAgentExecutor("agent-a", bridge)
	if err != nil {
		t.Fatalf("NewBridgeAgentExecutor: %v", err)
	}
	request := validBridgeRequest()
	session, err := bridge.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	executor.remember(sdk.TaskID(request.TaskID), session)
	execCtx := &a2asrv.ExecutorContext{TaskID: sdk.TaskID(request.TaskID), ContextID: request.ContextID}
	var events []sdk.Event
	for event, err := range executor.Cancel(context.Background(), execCtx) {
		if err != nil {
			t.Fatalf("Cancel event error: %v", err)
		}
		events = append(events, event)
	}
	if len(events) != 1 || events[0].(*sdk.TaskStatusUpdateEvent).Status.State != sdk.TaskStateCanceled {
		t.Fatalf("cancel events = %#v", events)
	}
}

var errTestBridgeStart = errTestBridge{}

type errTestBridge struct{}

func (errTestBridge) Error() string { return "bridge start failed" }
