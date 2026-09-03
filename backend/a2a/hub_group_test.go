package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHubGroupLifecycleAndBroadcast(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-grp-test"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	mailbox := NewMemoryHubMailbox()
	groupStore := NewMemoryHubGroupStore(mailbox)
	handler := HubHTTPHandler{
		Registry: registry,
		Mailbox:  mailbox,
		Groups:   groupStore,
	}

	// 1. Register Agent 1
	reg1Body, _ := json.Marshal(validAgentDeclaration("agent-one"))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(reg1Body)))
	var res1 struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(rec1.Body.Bytes(), &res1)
	agent1ID := res1.Identity.AgentID
	token1 := res1.Identity.AgentToken

	// 2. Register Agent 2
	reg2Body, _ := json.Marshal(validAgentDeclaration("agent-two"))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(reg2Body)))
	var res2 struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &res2)
	agent2ID := res2.Identity.AgentID
	token2 := res2.Identity.AgentToken

	// 3. Agent 1 creates group
	createGroupBody, _ := json.Marshal(HubCreateGroupInput{Name: "Alpha Team"})
	createReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups", bytes.NewReader(createGroupBody))
	createReq.Header.Set("X-Agent-ID", agent1ID)
	createReq.Header.Set("Authorization", "Bearer "+token1)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create group failed: %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createdGroup HubGroup
	_ = json.Unmarshal(createRec.Body.Bytes(), &createdGroup)
	if createdGroup.GroupID == "" || createdGroup.OwnerAgentID != agent1ID {
		t.Fatalf("unexpected group: %+v", createdGroup)
	}

	// 4. Agent 1 invites Agent 2
	inviteBody, _ := json.Marshal(HubInviteMemberInput{InviteeAgentID: agent2ID})
	inviteReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups/"+createdGroup.GroupID+"/invitations", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("X-Agent-ID", agent1ID)
	inviteReq.Header.Set("Authorization", "Bearer "+token1)
	inviteRec := httptest.NewRecorder()
	handler.ServeHTTP(inviteRec, inviteReq)
	if inviteRec.Code != http.StatusOK {
		t.Fatalf("invite failed: %d body=%s", inviteRec.Code, inviteRec.Body.String())
	}
	var inviteRes struct {
		Invitation HubGroupInvitation `json:"invitation"`
	}
	_ = json.Unmarshal(inviteRec.Body.Bytes(), &inviteRes)

	// 5. Agent 2 lists invitations and accepts
	listInvReq := httptest.NewRequest(http.MethodGet, "/hub/v1/groups/invitations", nil)
	listInvReq.Header.Set("X-Agent-ID", agent2ID)
	listInvReq.Header.Set("Authorization", "Bearer "+token2)
	listInvRec := httptest.NewRecorder()
	handler.ServeHTTP(listInvRec, listInvReq)
	if listInvRec.Code != http.StatusOK {
		t.Fatalf("list invitations failed: %d body=%s", listInvRec.Code, listInvRec.Body.String())
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups/invitations/1/accept", nil)
	acceptReq.Header.Set("X-Agent-ID", agent2ID)
	acceptReq.Header.Set("Authorization", "Bearer "+token2)
	acceptRec := httptest.NewRecorder()
	handler.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept failed: %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	// 6. Agent 1 broadcasts group message
	msgBody, _ := json.Marshal(HubGroupMessageInput{
		ContextID:      "ctx-grp-1",
		IdempotencyKey: "key-grp-1",
		Message:        "Hello Alpha Team!",
	})
	sendReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups/"+createdGroup.GroupID+"/messages", bytes.NewReader(msgBody))
	sendReq.Header.Set("X-Agent-ID", agent1ID)
	sendReq.Header.Set("Authorization", "Bearer "+token1)
	sendRec := httptest.NewRecorder()
	handler.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("send group message failed: %d body=%s", sendRec.Code, sendRec.Body.String())
	}

	// 7. Verify Agent 2 received message in inbox
	inboxItems, err := mailbox.Poll(context.Background(), policy.HubID, agent2ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(inboxItems) != 1 {
		t.Fatalf("expected 1 inbox item for agent 2, got %d", len(inboxItems))
	}
	if inboxItems[0].Message != "Hello Alpha Team!" {
		t.Fatalf("unexpected inbox message: %s", inboxItems[0].Message)
	}

	// 8. Agent 1 archives the group
	archiveReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups/"+createdGroup.GroupID+"/archive", nil)
	archiveReq.Header.Set("X-Agent-ID", agent1ID)
	archiveReq.Header.Set("Authorization", "Bearer "+token1)
	archiveRec := httptest.NewRecorder()
	handler.ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("archive group failed: %d body=%s", archiveRec.Code, archiveRec.Body.String())
	}
}
