package domain

import "testing"

func TestTaskStateTransitions(t *testing.T) {
	tests := []struct {
		from TaskState
		to   TaskState
		want bool
	}{
		{TaskQueued, TaskDispatched, true},
		{TaskQueued, TaskBlocked, true},
		{TaskQueued, TaskCancelled, true},
		{TaskDispatched, TaskRunning, true},
		{TaskDispatched, TaskCancelled, true},
		{TaskRunning, TaskDelivered, true},
		{TaskRunning, TaskFailed, true},
		{TaskRunning, TaskBlocked, true},
		{TaskRunning, TaskCancelled, true},
		{TaskBlocked, TaskQueued, true},
		{TaskFailed, TaskQueued, true},
		{TaskDelivered, TaskQueued, false},
		{TaskCancelled, TaskQueued, false},
		{TaskQueued, TaskDelivered, false},
		{TaskRunning, TaskRunning, false},
	}

	for _, test := range tests {
		t.Run(string(test.from)+"_to_"+string(test.to), func(t *testing.T) {
			if got := test.from.CanTransitionTo(test.to); got != test.want {
				t.Fatalf("CanTransitionTo() = %v, want %v", got, test.want)
			}
			if err := ValidateTaskTransition(test.from, test.to); (err == nil) != test.want {
				t.Fatalf("ValidateTaskTransition() error = %v, want allowed=%v", err, test.want)
			}
		})
	}
}

func TestTaskStateTerminal(t *testing.T) {
	if !TaskDelivered.Terminal() || !TaskCancelled.Terminal() {
		t.Fatal("delivered and cancelled must be terminal")
	}
	if TaskFailed.Terminal() || TaskBlocked.Terminal() || TaskRunning.Terminal() {
		t.Fatal("failed, blocked, and running must remain retryable or actionable")
	}
}

func TestTaskEnvelopeKeepsStableRoutingAndReplyContract(t *testing.T) {
	root := MessageID("message-root")
	source := MessageID("message-source")
	task := Task{ID: "task-1", RootMessageID: &root, SourceMessageID: &source, CreatorID: "user-1", TargetAgentID: "agent-1", CollaborationDepth: 2}
	envelope := task.Envelope()
	if envelope.ID != task.ID || envelope.Conversation != string(root) || envelope.InReplyTo == nil || *envelope.InReplyTo != source {
		t.Fatalf("unexpected correlation fields: %+v", envelope)
	}
	if envelope.Act != "request" || !envelope.RequiresReply || envelope.Hops != 2 || envelope.To != task.TargetAgentID {
		t.Fatalf("unexpected routing contract: %+v", envelope)
	}
	if retry := task.Envelope(); retry.ID != envelope.ID || retry.Conversation != envelope.Conversation {
		t.Fatalf("retry changed idempotency key: before=%+v after=%+v", envelope, retry)
	}
}
