package runnerclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentcollab.local/agentcollab/internal/buildinfo"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
)

const acpProtocolVersion = 1

type acpWireMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpWireError   `json:"error,omitempty"`
}

type acpWireError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type acpPermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}

type acpConnection struct {
	writer  *bufio.Writer
	mu      sync.Mutex
	nextID  atomic.Int64
	input   chan acpWireMessage
	readErr chan error
	policy  string
}

func newACPConnection(stdin io.Writer, stdout io.Reader, policy string) *acpConnection {
	connection := &acpConnection{
		writer:  bufio.NewWriter(stdin),
		input:   make(chan acpWireMessage, 64),
		readErr: make(chan error, 1),
		policy:  effectiveACPPermission(policy),
	}
	go connection.readLoop(stdout)
	return connection
}

func effectiveACPPermission(value string) string {
	if value == "" {
		return "auto_allow"
	}
	return value
}

func (c *acpConnection) readLoop(reader io.Reader) {
	decoder := json.NewDecoder(reader)
	for {
		var message acpWireMessage
		if err := decoder.Decode(&message); err != nil {
			c.readErr <- err
			return
		}
		c.input <- message
	}
}

func (c *acpConnection) write(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := c.writer.Write(payload); err != nil {
		return err
	}
	if err := c.writer.WriteByte('\n'); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *acpConnection) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *acpConnection) call(ctx context.Context, method string, params any, target any, output *strings.Builder, sessionID string) error {
	id := c.nextID.Add(1)
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			if sessionID != "" {
				_ = c.notify("session/cancel", map[string]any{"sessionId": sessionID})
			}
			return ctx.Err()
		case err := <-c.readErr:
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("ACP Agent closed its output before responding to %s", method)
			}
			return fmt.Errorf("read ACP response for %s: %w", method, err)
		case message := <-c.input:
			if message.Method != "" {
				if len(message.ID) > 0 {
					if err := c.handleRequest(message); err != nil {
						return err
					}
				} else {
					c.handleNotification(message, output)
				}
				continue
			}
			responseID, ok := acpNumericID(message.ID)
			if !ok || responseID != id {
				continue
			}
			if message.Error != nil {
				return fmt.Errorf("ACP %s failed (%d): %s", method, message.Error.Code, message.Error.Message)
			}
			if target != nil && len(message.Result) > 0 {
				if err := json.Unmarshal(message.Result, target); err != nil {
					return fmt.Errorf("decode ACP %s result: %w", method, err)
				}
			}
			return nil
		}
	}
}

func acpNumericID(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, false
	}
	return id, true
}

func (c *acpConnection) handleNotification(message acpWireMessage, output *strings.Builder) {
	if message.Method != "session/update" || output == nil {
		return
	}
	var notification struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Content       struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(message.Params, &notification) != nil {
		return
	}
	if notification.Update.SessionUpdate == "agent_message_chunk" && notification.Update.Content.Type == "text" {
		output.WriteString(notification.Update.Content.Text)
	}
}

func (c *acpConnection) handleRequest(message acpWireMessage) error {
	if message.Method != "session/request_permission" {
		return c.write(map[string]any{
			"jsonrpc": "2.0", "id": message.ID,
			"error": map[string]any{"code": -32601, "message": "AgentCollab Runner does not advertise this client method"},
		})
	}
	var request struct {
		Options []acpPermissionOption `json:"options"`
	}
	if err := json.Unmarshal(message.Params, &request); err != nil {
		return c.write(map[string]any{
			"jsonrpc": "2.0", "id": message.ID,
			"error": map[string]any{"code": -32602, "message": "invalid permission request"},
		})
	}
	option, found := chooseACPPermissionOption(c.policy, request.Options)
	if !found {
		return c.write(map[string]any{
			"jsonrpc": "2.0", "id": message.ID,
			"result": map[string]any{"outcome": map[string]any{"outcome": "cancelled"}},
		})
	}
	return c.write(map[string]any{
		"jsonrpc": "2.0", "id": message.ID,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": option.OptionID}},
	})
}

func chooseACPPermissionOption(policy string, options []acpPermissionOption) (acpPermissionOption, bool) {
	preferred := []string{"allow_always", "allow_once"}
	if policy == "allow_once" {
		preferred = []string{"allow_once", "allow_always"}
	} else if policy == "deny" {
		preferred = []string{"reject_always", "reject_once"}
	}
	for _, kind := range preferred {
		for _, option := range options {
			if option.Kind == kind {
				return option, true
			}
		}
	}
	return acpPermissionOption{}, false
}

type acpInitializeResponse struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		LoadSession         bool                       `json:"loadSession"`
		SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities"`
	} `json:"agentCapabilities"`
}

func (r acpInitializeResponse) supportsSessionCapability(name string) bool {
	value, ok := r.AgentCapabilities.SessionCapabilities[name]
	return ok && string(value) != "null"
}

type acpSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type acpPromptResponse struct {
	StopReason string `json:"stopReason"`
}

func (c *Client) executeACPAgent(ctx context.Context, agent config.RunnerAgent, task domain.ClaimedTask, prompt, workdir string) agentCommandResult {
	result := agentCommandResult{StartedAt: time.Now().UTC()}
	args := argumentsForTask(agent, task, prompt, workdir)
	command := exec.Command(agent.Command, args...)
	command.Dir = workdir
	command.Env = os.Environ()
	for key, value := range agent.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		result.Err = err
		result.FinishedAt = time.Now().UTC()
		return result
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		result.Err = err
		result.FinishedAt = time.Now().UTC()
		return result
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		result.Err = err
		result.FinishedAt = time.Now().UTC()
		return result
	}
	result.PID = command.Process.Pid
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	connection := newACPConnection(stdin, stdout, agent.ACPPermission)
	var initialize acpInitializeResponse
	err = connection.call(ctx, "initialize", map[string]any{
		"protocolVersion": acpProtocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]string{
			"name": "agentcollab-runner", "title": "AgentCollab Runner", "version": buildinfo.Version,
		},
	}, &initialize, nil, "")
	if err == nil && initialize.ProtocolVersion != acpProtocolVersion {
		err = fmt.Errorf("ACP protocol mismatch: Agent supports v%d, Runner supports v%d", initialize.ProtocolVersion, acpProtocolVersion)
	}

	sessionID := ""
	if err == nil && task.NativeSessionID != "" {
		sessionID = task.NativeSessionID
		sessionParams := map[string]any{"sessionId": sessionID, "cwd": workdir, "mcpServers": []any{}}
		switch {
		case initialize.supportsSessionCapability("resume"):
			err = connection.call(ctx, "session/resume", sessionParams, nil, nil, sessionID)
		case initialize.AgentCapabilities.LoadSession:
			err = connection.call(ctx, "session/load", sessionParams, nil, nil, sessionID)
		default:
			sessionID = ""
		}
		// A stale native session must not make the whole room unusable. Starting
		// a fresh ACP session keeps the visible conversation moving while the
		// new session ID replaces the stale one in the Hub receipt.
		if err != nil && ctx.Err() == nil {
			sessionID = ""
			err = nil
		}
	}
	if err == nil && sessionID == "" {
		var session acpSessionResponse
		err = connection.call(ctx, "session/new", map[string]any{
			"cwd": workdir, "mcpServers": []any{},
		}, &session, nil, "")
		sessionID = session.SessionID
		if err == nil && sessionID == "" {
			err = errors.New("ACP session/new returned an empty sessionId")
		}
	}
	if err == nil && strings.TrimSpace(agent.ACPMode) != "" {
		err = connection.call(ctx, "session/set_mode", map[string]any{
			"sessionId": sessionID, "modeId": agent.ACPMode,
		}, nil, nil, sessionID)
	}
	var output strings.Builder
	var promptResponse acpPromptResponse
	if err == nil {
		err = connection.call(ctx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    []map[string]string{{"type": "text", "text": prompt}},
		}, &promptResponse, &output, sessionID)
	}
	if err == nil && promptResponse.StopReason != "end_turn" {
		err = fmt.Errorf("ACP turn stopped: %s", promptResponse.StopReason)
	}

	result.Stdout = output.String()
	result.Stderr = stderr.String()
	result.NativeSessionID = sessionID
	result.Err = err
	result.FinishedAt = time.Now().UTC()
	_ = stdin.Close()
	select {
	case processErr := <-wait:
		if result.Err == nil && processErr != nil {
			result.Err = processErr
		}
	case <-time.After(750 * time.Millisecond):
		_ = command.Process.Kill()
		<-wait
	}
	if result.Err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(result.Err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		if strings.TrimSpace(result.Stderr) == "" {
			result.Stderr = result.Err.Error()
		}
	}
	return result
}
