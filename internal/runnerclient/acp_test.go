package runnerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestExecuteACPAgentUsesProtocolSessionStreamingAndPermissions(t *testing.T) {
	workdir := t.TempDir()
	agent := config.RunnerAgent{
		Command: os.Args[0], Args: []string{"-test.run=TestACPHelperProcess"},
		Transport: "acp", ACPPermission: "auto_allow",
		Environment: map[string]string{"GO_WANT_ACP_HELPER": "1"},
	}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-acp", RoomID: "room-acp"}}
	result := (&Client{}).executeAgentCommand(context.Background(), agent, task, "return the result", workdir)
	if result.Err != nil {
		t.Fatalf("ACP execution failed: %v; stderr=%s", result.Err, result.Stderr)
	}
	if result.Stdout != "ACP_RESULT" {
		t.Fatalf("ACP output = %q", result.Stdout)
	}
	if result.NativeSessionID != "acp-session-1" {
		t.Fatalf("ACP session ID = %q", result.NativeSessionID)
	}
}

// TestACPHelperProcess runs as a child test binary and speaks the small ACP v1
// surface used by the Runner. Keeping the fake agent out-of-process verifies
// the real stdio framing rather than only testing JSON helper functions.
func TestACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_HELPER") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if decoder.Decode(&message) != nil {
			return
		}
		switch message.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "id": message.ID,
				"result": map[string]any{
					"protocolVersion": acpProtocolVersion,
					"agentCapabilities": map[string]any{
						"loadSession": false, "sessionCapabilities": map[string]any{"resume": map[string]any{}},
					},
				},
			})
		case "session/new":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "id": message.ID,
				"result": map[string]any{"sessionId": "acp-session-1"},
			})
		case "session/prompt":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "id": "permission-1", "method": "session/request_permission",
				"params": map[string]any{
					"sessionId": "acp-session-1",
					"toolCall":  map[string]any{"toolCallId": "tool-1"},
					"options": []map[string]string{
						{"optionId": "allow-once", "name": "Allow", "kind": "allow_once"},
						{"optionId": "allow-always", "name": "Always allow", "kind": "allow_always"},
					},
				},
			})
			var permissionResponse map[string]any
			if decoder.Decode(&permissionResponse) != nil {
				os.Exit(4)
			}
			result, _ := permissionResponse["result"].(map[string]any)
			outcome, _ := result["outcome"].(map[string]any)
			if outcome["optionId"] != "allow-always" {
				os.Exit(5)
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "acp-session-1",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]string{"type": "text", "text": "ACP_RESULT"},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "id": message.ID,
				"result": map[string]string{"stopReason": "end_turn"},
			})
		}
	}
}

func TestCompletionOutboxReplaysBeforeTaskRecovery(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/api/v1/runner/tasks/task-outbox/complete") {
			http.NotFound(w, request)
			return
		}
		if calls.Add(1) == 1 {
			http.Error(w, "temporary disconnect", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg-existing"}`))
	}))
	defer server.Close()

	root := t.TempDir()
	client := &Client{
		config:      config.Runner{HubURL: server.URL, LocalDir: root},
		credentials: Credentials{Token: "runner-token"},
		http:        server.Client(), stdout: os.Stdout, stderr: os.Stderr,
	}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-outbox"}, AttemptNo: 2}
	if err := client.complete(context.Background(), task, true, "done", "", 0, domain.ExecutionReceipt{}); err == nil {
		t.Fatal("first completion unexpectedly succeeded")
	}
	path := client.pendingCompletionPath(task.ID, task.AttemptNo)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("completion was not saved in outbox: %v", err)
	}
	replayed, err := client.replayPendingCompletions(context.Background())
	if err != nil || replayed != 1 {
		t.Fatalf("replayed=%d err=%v", replayed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("delivered outbox file still exists: %v", err)
	}
	if filepath.Dir(path) != client.completionOutboxDir() {
		t.Fatalf("outbox path escaped local directory: %s", path)
	}
}
