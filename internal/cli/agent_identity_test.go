package cli

import "testing"

func TestAgentIdentityPortableAndSessionSpecific(t *testing.T) {
	for _, key := range []string{"TIMEKEEPER_AGENT_ID", "TIMEKEEPER_SESSION_ID", "TIMEKEEPER_AGENT_RUNTIME", "CODEX_THREAD_ID", "CODEX_SESSION_ID", "HERMES_SESSION_ID", "HERMES_SESSION_THREAD_ID"} {
		t.Setenv(key, "")
	}
	if got := resolveAgentIdentity(); got.AgentID != "" {
		t.Fatalf("invented identity: %+v", got)
	}
	t.Setenv("CODEX_THREAD_ID", "thread-one")
	a := resolveAgentIdentity()
	t.Setenv("CODEX_THREAD_ID", "thread-two")
	b := resolveAgentIdentity()
	if a.AgentID == b.AgentID || a.SessionID != "thread-one" || a.Runtime != "codex" {
		t.Fatalf("sessions conflated: %+v %+v", a, b)
	}
	t.Setenv("TIMEKEEPER_AGENT_ID", "custom-agent")
	t.Setenv("TIMEKEEPER_SESSION_ID", "custom-session")
	t.Setenv("TIMEKEEPER_AGENT_RUNTIME", "anything")
	got := resolveAgentIdentity()
	if got.AgentID != "custom-agent" || got.SessionID != "custom-session" || got.Runtime != "anything" {
		t.Fatalf("explicit identity ignored: %+v", got)
	}
}

func TestAgentIdentityUsesHermesSessionContract(t *testing.T) {
	for _, key := range []string{"TIMEKEEPER_AGENT_ID", "TIMEKEEPER_SESSION_ID", "TIMEKEEPER_AGENT_RUNTIME", "CODEX_THREAD_ID", "CODEX_SESSION_ID", "HERMES_SESSION_ID", "HERMES_SESSION_THREAD_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("HERMES_SESSION_ID", "hermes-session-1")
	first := resolveAgentIdentity()
	if first.SessionID != "hermes-session-1" || first.Runtime != "hermes" || first.AgentID == "" {
		t.Fatalf("Hermes session was not resolved: %+v", first)
	}
	t.Setenv("HERMES_SESSION_ID", "hermes-session-2")
	second := resolveAgentIdentity()
	if second.AgentID == first.AgentID || second.SessionID != "hermes-session-2" {
		t.Fatalf("Hermes sessions were conflated: %+v %+v", first, second)
	}
}
