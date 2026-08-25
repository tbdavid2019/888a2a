package executor

import (
	"strings"
	"testing"
)

func TestBuildPromptOmitsPersonaWhenEmpty(t *testing.T) {
	got := BuildPrompt("alice", "", "")
	if strings.Contains(got, "Your persona") {
		t.Fatalf("prompt should not contain persona section when empty, got:\n%s", got)
	}
	if !strings.Contains(got, agentIdentityText("alice")) {
		t.Fatal("prompt must still contain identity section")
	}
	if !strings.Contains(got, AgentCommunicationPrompt) {
		t.Fatal("prompt must still contain communication section")
	}
}

func TestBuildPromptInjectsPersonaAfterIdentity(t *testing.T) {
	got := BuildPrompt("alice", "", "  Be concise and prefer Go.  ")
	identityIdx := strings.Index(got, agentIdentityText("alice"))
	personaIdx := strings.Index(got, "## Your persona")
	commIdx := strings.Index(got, AgentCommunicationPrompt)
	if personaIdx < 0 || !strings.Contains(got, "Be concise and prefer Go.") {
		t.Fatalf("prompt must contain trimmed persona text, got:\n%s", got)
	}
	if identityIdx >= personaIdx || personaIdx >= commIdx {
		t.Fatalf("persona must appear after identity and before communication: identity=%d persona=%d comm=%d", identityIdx, personaIdx, commIdx)
	}
}

func TestBuildPromptInjectsOwnershipAfterPersona(t *testing.T) {
	got := BuildPrompt("alice", "Alice Owner", "persona text")
	if !strings.Contains(got, "## Ownership & Safety") {
		t.Fatalf("prompt must contain ownership section when owner is set, got:\n%s", got)
	}
	if !strings.Contains(got, "Your owner is Alice Owner") {
		t.Fatalf("prompt must name the owner, got:\n%s", got)
	}
	if !strings.Contains(got, "dm:@Alice Owner") {
		t.Fatalf("prompt must give the dm:@<owner> address for high-risk approval, got:\n%s", got)
	}
	// The approval request must be a self-contained, detailed message (who, where,
	// what, impact), not a terse template.
	for _, want := range []string{"WHO requested it", "WHERE the request came from", "WHAT they want", "the IMPACT", `"Approve or deny?"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt must instruct a detailed approval request mentioning %q, got:\n%s", want, got)
		}
	}
	personaIdx := strings.Index(got, "## Your persona")
	ownerIdx := strings.Index(got, "## Ownership & Safety")
	commIdx := strings.Index(got, AgentCommunicationPrompt)
	if ownerIdx < 0 || personaIdx >= ownerIdx || ownerIdx >= commIdx {
		t.Fatalf("ownership must appear after persona and before communication: persona=%d owner=%d comm=%d", personaIdx, ownerIdx, commIdx)
	}
}

func TestBuildPromptOmitsOwnershipWhenOwnerEmpty(t *testing.T) {
	if got := BuildPrompt("alice", "", "persona"); strings.Contains(got, "Ownership & Safety") {
		t.Fatalf("prompt must omit ownership section for a legacy agent with no owner, got:\n%s", got)
	}
}

func TestBuildReanchorPrompt(t *testing.T) {
	got := BuildReanchorPrompt("bob", "")
	if strings.Contains(got, "{{name}}") {
		t.Fatalf("re-anchor template must not contain unrendered placeholders:\n%s", got)
	}
	if !strings.Contains(got, `You are "bob"`) || !strings.Contains(got, "@bob") {
		t.Fatalf("re-anchor prompt must render the agent name, got:\n%s", got)
	}
	if !strings.Contains(got, "Read MEMORY.md first") {
		t.Fatalf("re-anchor prompt must keep the MEMORY.md recovery instruction, got:\n%s", got)
	}
}

func TestBuildReanchorPromptCarriesOwner(t *testing.T) {
	withOwner := BuildReanchorPrompt("bob", "Bob Owner")
	if !strings.Contains(withOwner, "Bob Owner") || !strings.Contains(withOwner, "dm:@Bob Owner") {
		t.Fatalf("re-anchor must carry the owner line when owner is set, got:\n%s", withOwner)
	}
	if got := BuildReanchorPrompt("bob", ""); strings.Contains(got, "Owner:") {
		t.Fatalf("re-anchor must omit the owner line for a legacy agent, got:\n%s", got)
	}
}

func TestBuildPrompt_A2ATaskDelegationAndDirectProcessRejection(t *testing.T) {
	got := BuildPrompt("alice", "Alice Owner", "persona text")

	// Verify A2A task delegation instructions are present
	if !strings.Contains(got, "a2a_task_send") {
		t.Fatalf("prompt must instruct A2A task delegation via a2a_task_send, got:\n%s", got)
	}
	if !strings.Contains(got, "a2a_peer_list") {
		t.Fatalf("prompt must instruct peer discovery via a2a_peer_list, got:\n%s", got)
	}

	// Verify Channel/DM retained for collaboration context
	if !strings.Contains(got, "Retain Channel and DM conversations for conversational collaboration") &&
		!strings.Contains(got, "Retain Channel/DM for collaboration context") {
		t.Fatalf("prompt must retain Channel/DM for conversational context, got:\n%s", got)
	}

	// Verify direct process assumptions are rejected
	for _, rejectedAssumption := range []string{
		"do NOT poll",
		"shared memory",
		"direct process control",
	} {
		if !strings.Contains(got, rejectedAssumption) {
			t.Fatalf("prompt must explicitly reject direct process assumption %q, got:\n%s", rejectedAssumption, got)
		}
	}
}

func TestBuildReanchorPrompt_A2ADelegation(t *testing.T) {
	got := BuildReanchorPrompt("bob", "")
	if !strings.Contains(got, "Use A2A tasks for work delegation") {
		t.Fatalf("re-anchor prompt must include A2A work delegation instruction, got:\n%s", got)
	}
	if !strings.Contains(got, "never assume direct process control or poll peer processes") {
		t.Fatalf("re-anchor prompt must reject direct process assumptions, got:\n%s", got)
	}
}
