package runnerclient

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentcollab.local/agentcollab/internal/buildinfo"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
)

type Credentials struct {
	Token          string             `json:"token"`
	RunnerID       domain.RunnerID    `json:"runner_id"`
	DeviceID       domain.DeviceID    `json:"device_id"`
	WorkspaceID    domain.WorkspaceID `json:"workspace_id"`
	Agents         map[string]string  `json:"agents"`
	InstallationID string             `json:"installation_id,omitempty"`
}

type Client struct {
	config             config.Runner
	agents             []config.RunnerAgent
	metadata           map[string]discoveredAgent
	credentials        Credentials
	agentCredentialsMu sync.RWMutex
	catalogMu          sync.Mutex
	catalogRevision    string
	catalogETag        string
	lastCatalogCheck   time.Time
	http               *http.Client
	stdout             io.Writer
	stderr             io.Writer
}

func Enroll(ctx context.Context, cfg config.Runner, workspaceID, username, password, deviceName string) (Credentials, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Credentials{}, err
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	if err := postJSON(ctx, client, cfg.HubURL+"/api/v1/auth/login", "", map[string]string{"username": username, "password": password}, nil); err != nil {
		return Credentials{}, fmt.Errorf("login to Hub: %w", err)
	}
	_, _, _ = syncAgentCatalog(ctx, cfg.HubURL, cfg.LocalDir, client, "")
	return enroll(ctx, cfg, client, map[string]any{"workspace_id": workspaceID}, deviceName)
}

// EnrollWithInvite registers a remote Runner without exposing a Hub user's
// password. The invite is single-use and the returned device token is stored
// only on this machine.
func EnrollWithInvite(ctx context.Context, cfg config.Runner, enrollmentCode, deviceName string) (Credentials, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	_, _, _ = syncAgentCatalog(ctx, cfg.HubURL, cfg.LocalDir, client, "")
	return enroll(ctx, cfg, client, map[string]any{"enrollment_code": enrollmentCode}, deviceName)
}

func enroll(ctx context.Context, cfg config.Runner, client *http.Client, request map[string]any, deviceName string) (Credentials, error) {
	installationID, err := loadOrCreateInstallationID(cfg.CredentialsFile)
	if err != nil {
		return Credentials{}, err
	}
	catalog, catalogErr := loadDiscoveryCatalog(cfg.LocalDir)
	if catalogErr != nil {
		return Credentials{}, fmt.Errorf("load Agent adapter catalog: %w", catalogErr)
	}
	discovered := discoverRunnerAgentsWithCatalog(cfg, catalog)
	registrations := registrationsForDiscovery(discovered)
	var response struct {
		Enrollment domain.RunnerEnrollment `json:"enrollment"`
		Token      string                  `json:"token"`
	}
	request["device_name"] = deviceName
	request["device_key"] = installationID
	request["os"] = runtime.GOOS
	request["arch"] = runtime.GOARCH
	request["version"] = buildinfo.Version
	request["agents"] = registrations
	if err := postJSON(ctx, client, cfg.HubURL+"/api/v1/runner/enroll", "", request, &response); err != nil {
		return Credentials{}, fmt.Errorf("enroll Runner: %w", err)
	}
	credentials := Credentials{Token: response.Token, RunnerID: response.Enrollment.Runner.ID, DeviceID: response.Enrollment.Device.ID, WorkspaceID: response.Enrollment.Device.WorkspaceID, Agents: make(map[string]string), InstallationID: installationID}
	for _, agent := range response.Enrollment.Agents {
		credentials.Agents[agent.LocalID] = string(agent.AgentID)
	}
	if err := saveCredentials(cfg.CredentialsFile, credentials); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func registrationsForDiscovery(discovered []discoveredAgent) []domain.AgentRegistration {
	registrations := make([]domain.AgentRegistration, 0, len(discovered))
	for _, item := range discovered {
		agent := item.Agent
		permissionMode := agent.PermissionMode
		if permissionMode == "" {
			permissionMode = "admin"
		}
		nativeHistory := agent.NativeHistory
		if nativeHistory == "" {
			nativeHistory = "真实 CLI 调用；是否同步到桌面软件历史由该 Agent 自身决定"
		}
		skills := make([]domain.AgentSkill, 0, len(item.Skills))
		for _, skill := range item.Skills {
			skills = append(skills, skill.Capability)
		}
		resources := make([]domain.AgentResource, 0, len(item.Resources))
		for _, resource := range item.Resources {
			resources = append(resources, resource.Capability)
		}
		transport := agentTransport(agent)
		capabilityPayload := map[string]any{
			"execution": item.Available, "native_cli": transport == "native", "transport": transport,
			"catalog_tier":    agentCatalogTier(agent),
			"permission_mode": permissionMode, "output_mode": defaultOutputMode(agent.OutputMode),
			"channel": "runner/" + runtime.GOOS, "version": item.Version, "executable": item.Executable,
			"native_history": nativeHistory, "skills": skills, "resources": resources,
		}
		if transport == "acp" {
			capabilityPayload["protocol_version"] = acpProtocolVersion
		}
		capabilities, _ := json.Marshal(capabilityPayload)
		registrations = append(registrations, domain.AgentRegistration{LocalID: agent.ID, Type: agent.Type, DisplayName: agent.DisplayName, Capabilities: capabilities, Available: item.Available})
	}
	return registrations
}

func registrationsForMetadata(metadata map[string]discoveredAgent) []domain.AgentRegistration {
	discovered := make([]discoveredAgent, 0, len(metadata))
	for _, item := range metadata {
		discovered = append(discovered, item)
	}
	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].Agent.ID < discovered[j].Agent.ID
	})
	return registrationsForDiscovery(discovered)
}

func loadOrCreateInstallationID(credentialsFile string) (string, error) {
	path, err := installationIDPath()
	if err != nil {
		return "", err
	}
	if value, found, err := readInstallationID(path); err != nil {
		return "", err
	} else if found {
		return value, nil
	}

	// Migrate releases that stored the identity beside device.json. The new
	// fixed per-user location survives a different config path or Client folder.
	legacyPath := filepath.Join(filepath.Dir(credentialsFile), "installation-id")
	if legacyPath != path {
		if value, found, err := readInstallationID(legacyPath); err != nil {
			return "", err
		} else if found {
			if err := writeInstallationID(path, value); err != nil {
				return "", err
			}
			return value, nil
		}
	}

	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	value := hex.EncodeToString(bytes)
	if err := writeInstallationID(path, value); err != nil {
		return "", err
	}
	return value, nil
}

func installationIDPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENTCOLLAB_INSTALLATION_ID_FILE")); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("AGENTCOLLAB_INSTALLATION_ID_FILE must be absolute")
		}
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home for stable device identity: %w", err)
	}
	return filepath.Join(home, ".agentcollab", "installation-id"), nil
}

func readInstallationID(path string) (string, bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(string(payload))
	if len(value) < 16 || len(value) > 128 {
		return "", false, fmt.Errorf("stable device identity in %s is invalid", path)
	}
	return value, true, nil
}

func writeInstallationID(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(value+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func New(cfg config.Runner, stdout, stderr io.Writer) (*Client, error) {
	credentials, err := loadCredentials(cfg.CredentialsFile)
	if err != nil {
		return nil, err
	}
	if credentials.InstallationID == "" {
		credentials.InstallationID, err = loadOrCreateInstallationID(cfg.CredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("initialize stable device identity: %w", err)
		}
		if err := saveCredentials(cfg.CredentialsFile, credentials); err != nil {
			return nil, err
		}
	} else if err := preserveExistingInstallationID(credentials.InstallationID); err != nil {
		return nil, fmt.Errorf("preserve stable device identity: %w", err)
	}
	catalog, catalogErr := loadDiscoveryCatalog(cfg.LocalDir)
	if catalogErr != nil {
		return nil, fmt.Errorf("load Agent adapter catalog: %w", catalogErr)
	}
	discovered := discoverRunnerAgentsWithCatalog(cfg, catalog)
	agents := make([]config.RunnerAgent, 0, len(discovered))
	metadata := make(map[string]discoveredAgent)
	for _, item := range discovered {
		metadata[item.Agent.ID] = item
		if item.Available {
			agents = append(agents, item.Agent)
		}
	}
	return &Client{config: cfg, agents: agents, metadata: metadata, credentials: credentials, catalogRevision: catalog.Revision, http: &http.Client{Timeout: 45 * time.Second}, stdout: stdout, stderr: stderr}, nil
}

func (c *Client) refreshAdapterCatalog(ctx context.Context, force bool) error {
	c.catalogMu.Lock()
	defer c.catalogMu.Unlock()
	if !force && !c.lastCatalogCheck.IsZero() && time.Since(c.lastCatalogCheck) < agentCatalogSyncInterval {
		return nil
	}
	c.lastCatalogCheck = time.Now()
	previousETag := c.catalogETag
	catalog, etag, err := syncAgentCatalog(ctx, c.config.HubURL, c.config.LocalDir, c.http, c.catalogETag)
	if err != nil {
		return err
	}
	c.catalogETag = etag
	// Revision is operator-facing metadata; ETag is the actual content
	// identity. Apply changed content even when a publisher forgot to bump the
	// revision, otherwise connected Clients keep stale launch arguments in
	// memory until they are restarted.
	if catalog.Revision == c.catalogRevision && etag == previousETag {
		return nil
	}
	discovered := discoverRunnerAgentsWithCatalog(c.config, catalog)
	agents := make([]config.RunnerAgent, 0, len(discovered))
	metadata := make(map[string]discoveredAgent, len(discovered))
	for _, item := range discovered {
		metadata[item.Agent.ID] = item
		if item.Available {
			agents = append(agents, item.Agent)
		}
	}
	c.agentCredentialsMu.Lock()
	c.agents = agents
	c.metadata = metadata
	c.agentCredentialsMu.Unlock()
	c.catalogRevision = catalog.Revision
	fmt.Fprintf(c.stdout, "agent_catalog_updated=%s discovered=%d available=%d\n", catalog.Revision, len(discovered), len(agents))
	return nil
}

func preserveExistingInstallationID(value string) error {
	path, err := installationIDPath()
	if err != nil {
		return err
	}
	if _, found, err := readInstallationID(path); err != nil {
		return err
	} else if found {
		// An already enrolled Runner keeps its credential-bound identity. A
		// later explicit re-pair uses the canonical value and converges configs.
		return nil
	}
	if len(value) < 16 || len(value) > 128 {
		return fmt.Errorf("credential device identity is invalid")
	}
	return writeInstallationID(path, value)
}

func (c *Client) Serve(ctx context.Context, once bool) error {
	if err := os.MkdirAll(filepath.Join(c.config.LocalDir, "tasks"), 0o700); err != nil {
		return fmt.Errorf("create task directory: %w", err)
	}
	if err := os.MkdirAll(c.completionOutboxDir(), 0o700); err != nil {
		return fmt.Errorf("create completion outbox: %w", err)
	}
	if err := c.refreshAdapterCatalog(ctx, true); err != nil {
		fmt.Fprintf(c.stderr, "Agent adapter catalog: %v; keeping the last valid catalog\n", err)
	}
	if once {
		if err := c.prepareTaskStream(ctx); err != nil {
			return err
		}
		if err := c.heartbeat(ctx); err != nil {
			return err
		}
		task, err := c.nextTask(ctx)
		if err != nil {
			return err
		}
		if task != nil {
			if err := c.runTask(ctx, *task); err != nil {
				fmt.Fprintf(c.stderr, "task %s: %v\n", task.ID, err)
			}
		}
		return nil
	}

	if err := c.heartbeat(ctx); err != nil {
		fmt.Fprintf(c.stderr, "heartbeat: %v\n", err)
	}
	go c.heartbeatLoop(ctx)
	maxConcurrent := effectiveMaxConcurrentTasks(c.config.MaxConcurrentTasks, len(c.agents))
	slots := make(chan struct{}, maxConcurrent)
	var tasks sync.WaitGroup
	defer tasks.Wait()

	prepared := false
	for {
		if !prepared {
			if err := c.prepareTaskStream(ctx); err != nil {
				fmt.Fprintf(c.stderr, "prepare task stream: %v\n", err)
				if err := waitForNextPoll(ctx); err != nil {
					return err
				}
				continue
			}
			prepared = true
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		task, err := c.nextTask(ctx)
		if err != nil {
			<-slots
			fmt.Fprintf(c.stderr, "poll: %v\n", err)
			if err := waitForNextPoll(ctx); err != nil {
				return err
			}
			continue
		}
		if task == nil {
			<-slots
			if err := waitForNextPoll(ctx); err != nil {
				return err
			}
			continue
		}
		tasks.Add(1)
		go func(claimed domain.ClaimedTask) {
			defer tasks.Done()
			defer func() { <-slots }()
			if err := c.runTask(ctx, claimed); err != nil {
				fmt.Fprintf(c.stderr, "task %s: %v\n", claimed.ID, err)
			}
		}(*task)
	}
}

func effectiveMaxConcurrentTasks(configured, availableAgents int) int {
	if configured > 0 {
		return configured
	}
	if availableAgents > 0 {
		return availableAgents
	}
	return 1
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.heartbeat(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(c.stderr, "heartbeat: %v\n", err)
			}
		}
	}
}

func waitForNextPoll(ctx context.Context) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) heartbeat(ctx context.Context) error {
	if err := c.refreshAdapterCatalog(ctx, false); err != nil {
		fmt.Fprintf(c.stderr, "Agent adapter catalog: %v; keeping the last valid catalog\n", err)
	}
	c.agentCredentialsMu.RLock()
	agentIDs := make([]string, 0, len(c.credentials.Agents))
	agentTypes := make(map[string]bool)
	for localID, agentID := range c.credentials.Agents {
		if item, ok := c.metadata[localID]; ok && item.Available {
			agentIDs = append(agentIDs, agentID)
			if agentType := strings.TrimSpace(item.Agent.Type); agentType != "" {
				agentTypes[agentType] = true
			}
		}
	}
	registrations := registrationsForMetadata(c.metadata)
	c.agentCredentialsMu.RUnlock()
	types := make([]string, 0, len(agentTypes))
	for agentType := range agentTypes {
		types = append(types, agentType)
	}
	sort.Strings(types)
	var response struct {
		Agents []domain.EnrolledAgent `json:"agents"`
	}
	if err := c.post(ctx, "/api/v1/runner/heartbeat", map[string]any{
		"version": buildinfo.Version, "device_key": c.credentials.InstallationID,
		"agent_ids": agentIDs, "agent_types": types,
		"agents": registrations,
	}, &response); err != nil {
		return err
	}
	c.agentCredentialsMu.Lock()
	defer c.agentCredentialsMu.Unlock()
	changed := false
	for _, agent := range response.Agents {
		if c.credentials.Agents[agent.LocalID] != string(agent.AgentID) {
			c.credentials.Agents[agent.LocalID] = string(agent.AgentID)
			changed = true
		}
	}
	if changed {
		if err := saveCredentials(c.config.CredentialsFile, c.credentials); err != nil {
			return fmt.Errorf("save refreshed Agent catalog: %w", err)
		}
	}
	return nil
}

func (c *Client) nextTask(ctx context.Context) (*domain.ClaimedTask, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.HubURL+"/api/v1/runner/tasks/next", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("poll Hub: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("poll Hub: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var task domain.ClaimedTask
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("decode task: %w", err)
	}
	return &task, nil
}

func (c *Client) runTask(ctx context.Context, task domain.ClaimedTask) error {
	agent, ok := c.agentByServerID(string(task.TargetAgentID), string(task.TargetAgentSourceID), task.TargetAgentLocalKey, task.TargetAgentType)
	if !ok {
		return c.complete(ctx, task, false, "", "Runner does not have the target Agent configuration", 127, domain.ExecutionReceipt{})
	}
	if cancelled, err := c.taskCancellationRequested(ctx, task.ID); err == nil && cancelled {
		return nil
	}
	if err := c.post(ctx, "/api/v1/runner/tasks/"+string(task.ID)+"/start", map[string]any{}, nil); err != nil {
		if cancelled, checkErr := c.taskCancellationRequested(ctx, task.ID); checkErr == nil && cancelled {
			return nil
		}
		return err
	}
	timeout := time.Duration(agent.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stopWatcher, cancellation := c.watchTaskCancellation(runCtx, task.ID, cancel)
	defer stopWatcher()
	workdir := filepath.Join(c.config.LocalDir, "tasks", string(task.ID), fmt.Sprintf("attempt-%d", task.AttemptNo))
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		return c.complete(ctx, task, false, "", err.Error(), 1, domain.ExecutionReceipt{})
	}
	if err := writeTaskDefinition(workdir, task); err != nil {
		return c.complete(ctx, task, false, "", "写入任务定义失败："+err.Error(), 1, domain.ExecutionReceipt{})
	}
	if err := c.downloadTaskProjectWorkspace(runCtx, &task, workdir); err != nil {
		if cancellation.cancelled.Load() {
			fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
			return nil
		}
		return c.complete(ctx, task, false, "", "下载或校验共享项目目录失败："+err.Error(), 1, domain.ExecutionReceipt{})
	}
	if err := c.resolveLocalProjectWorkspace(&task); err != nil {
		if cancellation.cancelled.Load() {
			return nil
		}
		return c.complete(ctx, task, false, "", "读取负责人本机工作目录失败："+err.Error(), 1, domain.ExecutionReceipt{})
	}
	if err := c.downloadTaskAttachments(runCtx, &task, workdir); err != nil {
		if cancellation.cancelled.Load() {
			fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
			return nil
		}
		return c.complete(ctx, task, false, "", "下载或校验会话附件失败："+err.Error(), 1, domain.ExecutionReceipt{})
	}
	if err := c.downloadTaskKnowledge(runCtx, &task, workdir); err != nil {
		if cancellation.cancelled.Load() {
			fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
			return nil
		}
		return c.complete(ctx, task, false, "", "下载或校验项目知识资产失败："+err.Error(), 1, domain.ExecutionReceipt{})
	}
	agentWorkdir := workdir
	if task.ProjectWorkspace != nil && strings.TrimSpace(task.ProjectWorkspace.LocalPath) != "" {
		agentWorkdir = task.ProjectWorkspace.LocalPath
	}
	if task.LocalWorkspace != nil && strings.TrimSpace(task.LocalWorkspace.LocalPath) != "" {
		agentWorkdir = task.LocalWorkspace.LocalPath
		task.DeliveryDirectory = filepath.Join(workdir, "deliverables")
	}
	requestText := currentTaskRequest(task.Prompt)
	metadata := authorizedTaskMetadata(c.metadataForAgent(agent.ID), task)
	capabilityContext := localCapabilityPrompt(metadata, requestText)
	prompt := buildPrompt(task)
	if capabilityContext != "" {
		prompt += "\n\n" + capabilityContext
	}
	fmt.Fprintf(c.stdout, "task_started=%s agent=%s\n", task.ID, agent.ID)
	result := c.executeAgentCommand(runCtx, agent, task, prompt, agentWorkdir)
	if cancellation.cancelled.Load() {
		fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
		return nil
	}
	matchedSkills, matchedResources := matchedCapabilityNames(requestText, metadata.Skills, metadata.Resources)
	nativeSessionID := result.NativeSessionID
	if nativeSessionID == "" {
		nativeSessionID = ExtractNativeSessionID(result.Stdout)
	}
	receipt := domain.ExecutionReceipt{NativeAgentType: agent.Type, Executable: metadata.Executable, Version: metadata.Version, PID: result.PID, StartedAt: result.StartedAt.Format(time.RFC3339Nano), FinishedAt: result.FinishedAt.Format(time.RFC3339Nano), NativeSessionID: nativeSessionID, NativeHistory: agent.NativeHistory, SkillCount: len(metadata.Skills), ResourceCount: len(metadata.Resources), MatchedSkills: matchedSkills, MatchedResources: matchedResources}
	receipt.ExecutionSource = string(task.ExecutionSource)
	if task.ProjectWorkspace != nil {
		receipt.ProjectStepID = string(task.ProjectWorkspace.StepID)
		receipt.ProjectCommit = task.ProjectWorkspace.Commit
	}
	if task.LocalWorkspace != nil {
		receipt.ProjectStepID = string(task.LocalWorkspace.StepID)
		receipt.ProjectCommit = task.LocalWorkspace.BaselineCommit
		receipt.LocalDirectoryLabel = task.LocalWorkspace.DirectoryLabel
	}
	if receipt.NativeHistory == "" {
		receipt.NativeHistory = "真实 CLI 调用；是否同步到桌面软件历史由该 Agent 自身决定"
	}
	if result.Err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return c.complete(ctx, task, false, "", "Agent 执行超时，请检查本机 Agent 登录、模型连接和任务耗时后重试。 / Agent timed out; check local authentication and model connectivity.", result.ExitCode, receipt)
		}
		return c.complete(ctx, task, false, "", commandErrorSummary(result), result.ExitCode, receipt)
	}
	output := ParseOutput(defaultOutputMode(agent.OutputMode), result.Stdout)
	if output == "" {
		output = strings.TrimSpace(result.Stderr)
	}
	if StructuredOutputError(result.Stdout) {
		return c.complete(ctx, task, false, "", output, result.ExitCode, receipt)
	}
	if AuthenticationRequiredOutput(output) {
		return c.complete(ctx, task, false, "", authenticationRequiredSummary(agent), http.StatusUnauthorized, receipt)
	}
	if summary, failed := AgentExecutionFailureOutput(output); failed {
		return c.complete(ctx, task, false, "", summary, 1, receipt)
	}
	for continuation := 1; continuation <= 2 && NeedsCompletionFollowup(output); continuation++ {
		fmt.Fprintf(c.stdout, "task_continuation=%s agent=%s attempt=%d reason=incomplete_delivery\n", task.ID, agent.ID, continuation)
		continuationTask := task
		continuationTask.NativeSessionID = receipt.NativeSessionID
		continuationPrompt := completionContinuationPrompt(task, output, continuationTask.NativeSessionID == "" || !agentSupportsResume(agent), capabilityContext)
		continued := c.executeAgentCommand(runCtx, agent, continuationTask, continuationPrompt, agentWorkdir)
		if cancellation.cancelled.Load() {
			fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
			return nil
		}
		receipt.PID = continued.PID
		receipt.FinishedAt = continued.FinishedAt.Format(time.RFC3339Nano)
		if sessionID := continued.NativeSessionID; sessionID != "" {
			receipt.NativeSessionID = sessionID
		} else if sessionID := ExtractNativeSessionID(continued.Stdout); sessionID != "" {
			receipt.NativeSessionID = sessionID
		}
		if continued.Err != nil {
			return c.complete(ctx, task, false, "", commandErrorSummary(continued), continued.ExitCode, receipt)
		}
		output = ParseOutput(defaultOutputMode(agent.OutputMode), continued.Stdout)
		if output == "" {
			output = strings.TrimSpace(continued.Stderr)
		}
		if StructuredOutputError(continued.Stdout) {
			return c.complete(ctx, task, false, "", output, continued.ExitCode, receipt)
		}
		if AuthenticationRequiredOutput(output) {
			return c.complete(ctx, task, false, "", authenticationRequiredSummary(agent), http.StatusUnauthorized, receipt)
		}
		if summary, failed := AgentExecutionFailureOutput(output); failed {
			return c.complete(ctx, task, false, "", summary, 1, receipt)
		}
	}
	if NeedsCompletionFollowup(output) {
		return c.complete(ctx, task, false, "", "Agent 连续返回调查计划或过程状态，没有交付可验收的结论。", 2, receipt)
	}
	if err := c.uploadTaskArtifacts(runCtx, task, agentWorkdir); err != nil {
		return c.complete(ctx, task, false, "", "回传交付文件失败："+err.Error(), 1, receipt)
	}
	if cancellation.cancelled.Load() {
		fmt.Fprintf(c.stdout, "task_cancelled=%s agent=%s\n", task.ID, agent.ID)
		return nil
	}
	receipt.MatchedSkills, receipt.MatchedResources = matchedCapabilityNames(requestText+"\n"+output, metadata.Skills, metadata.Resources)
	if err := c.complete(ctx, task, true, output, "", 0, receipt); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "task_delivered=%s agent=%s\n", task.ID, agent.ID)
	return nil
}

func (c *Client) uploadTaskArtifacts(ctx context.Context, task domain.ClaimedTask, workdir string) error {
	directory := filepath.Join(workdir, "deliverables")
	if task.DeliveryDirectory != "" {
		directory = task.DeliveryDirectory
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(files)
	if len(files) > 5 {
		return fmt.Errorf("交付文件不能超过 5 个")
	}
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Size() > 50<<20 {
			return fmt.Errorf("%s 超过 50 MB", info.Name())
		}
		if err := c.uploadTaskArtifact(ctx, task.ID, path); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) uploadTaskArtifact(ctx context.Context, taskID domain.TaskID, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.HubURL+"/api/v1/runner/tasks/"+string(taskID)+"/artifacts", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Hub 返回 %d：%s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

type taskCancellationWatcher struct {
	cancelled atomic.Bool
	done      chan struct{}
}

// watchTaskCancellation turns the Hub's durable cancelled state into the
// local context cancellation that stops the actual CLI process. It is active
// only while a task is executing, and transient network errors intentionally
// do not terminate work on their own.
func (c *Client) watchTaskCancellation(ctx context.Context, taskID domain.TaskID, cancel context.CancelFunc) (func(), *taskCancellationWatcher) {
	watcher := &taskCancellationWatcher{done: make(chan struct{})}
	watchCtx, stop := context.WithCancel(ctx)
	go func() {
		defer close(watcher.done)
		check := func() bool {
			cancelled, err := c.taskCancellationRequested(watchCtx, taskID)
			if err != nil || !cancelled {
				return false
			}
			watcher.cancelled.Store(true)
			cancel()
			return true
		}
		if check() {
			return
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				if check() {
					return
				}
			}
		}
	}()
	return func() {
		stop()
		<-watcher.done
	}, watcher
}

func authorizedTaskMetadata(metadata discoveredAgent, task domain.ClaimedTask) discoveredAgent {
	if task.CapabilityScope != "shared_only" {
		return metadata
	}
	allowed := make(map[string]bool, len(task.AuthorizedSkills))
	for _, name := range task.AuthorizedSkills {
		allowed[name] = true
	}
	filtered := make([]localSkill, 0, len(metadata.Skills))
	for _, skill := range metadata.Skills {
		if allowed[skill.Capability.Name] {
			filtered = append(filtered, skill)
		}
	}
	metadata.Skills = filtered
	metadata.Resources = []localResource{}
	return metadata
}

type agentCommandResult struct {
	Stdout          string
	Stderr          string
	NativeSessionID string
	PID             int
	ExitCode        int
	StartedAt       time.Time
	FinishedAt      time.Time
	Err             error
}

func (c *Client) executeAgentCommand(ctx context.Context, agent config.RunnerAgent, task domain.ClaimedTask, prompt, workdir string) agentCommandResult {
	if agentTransport(agent) == "acp" {
		return c.executeACPAgent(ctx, agent, task, prompt, workdir)
	}
	template := agent.Args
	if task.NativeSessionID != "" && len(agent.ResumeArgs) > 0 {
		template = agent.ResumeArgs
	}
	args := argumentsForTask(agent, task, prompt, workdir)
	removed := make(map[string]bool)
	for {
		result := executeAgentCommandOnce(ctx, agent, prompt, workdir, args)
		if result.Err == nil || ctx.Err() != nil {
			return result
		}
		option := unsupportedCommandOption(result)
		if option == "" || removed[option] {
			return result
		}
		adapted, adaptedTemplate, ok := removeUnsupportedCommandOption(args, template, option)
		if !ok {
			return result
		}
		removed[option] = true
		args, template = adapted, adaptedTemplate
		fmt.Fprintf(c.stdout, "agent_cli_compatibility_retry=%s option=%s\n", agent.Type, option)
	}
}

func executeAgentCommandOnce(ctx context.Context, agent config.RunnerAgent, prompt, workdir string, args []string) agentCommandResult {
	command := exec.CommandContext(ctx, agent.Command, args...)
	configureCommandCancellation(command)
	command.Dir = workdir
	command.Env = os.Environ()
	for key, value := range agent.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	if agent.PromptStdin {
		command.Stdin = strings.NewReader(prompt)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	result := agentCommandResult{StartedAt: time.Now().UTC()}
	result.Err = command.Start()
	if command.Process != nil {
		result.PID = command.Process.Pid
	}
	if result.Err == nil {
		result.Err = command.Wait()
	}
	result.FinishedAt = time.Now().UTC()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if result.Err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(result.Err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	return result
}

var unsupportedOptionPattern = regexp.MustCompile(`(?i)(?:unknown|unrecognized|unsupported|invalid)\s+(?:option|argument)(?:\s*[:=]|\s+)\s*[\x60'\"]?(-{1,2}[a-z0-9][a-z0-9-]*)`)

func unsupportedCommandOption(result agentCommandResult) string {
	text := result.Stderr
	if strings.TrimSpace(text) == "" {
		text = result.Stdout
	}
	match := unsupportedOptionPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func removeUnsupportedCommandOption(args, template []string, option string) ([]string, []string, bool) {
	for index, argument := range args {
		if argument != option {
			continue
		}
		end := index + 1
		// Remove a following literal value (for example
		// "--setting-sources user"), but never consume a templated prompt,
		// work directory, session ID, or the next option.
		if end < len(args) && end < len(template) && !strings.HasPrefix(template[end], "-") && !strings.Contains(template[end], "{{") {
			end++
		}
		adaptedArgs := append([]string{}, args[:index]...)
		adaptedArgs = append(adaptedArgs, args[end:]...)
		adaptedTemplate := append([]string{}, template[:index]...)
		adaptedTemplate = append(adaptedTemplate, template[end:]...)
		return adaptedArgs, adaptedTemplate, true
	}
	return args, template, false
}

func agentSupportsResume(agent config.RunnerAgent) bool {
	return agentTransport(agent) == "acp" || len(agent.ResumeArgs) > 0
}

func commandErrorSummary(result agentCommandResult) string {
	// stderr can contain paths, credentials and tool-server configuration.
	// Classify it locally instead of publishing raw CLI diagnostics to a room.
	text := strings.ToLower(result.Stderr)
	if AuthenticationRequiredOutput(result.Stderr) {
		return "本机 Agent 需要登录，请由设备所有者完成登录后重试。 / Sign in to the local Agent and retry."
	}
	if strings.Contains(text, "unknown model") || strings.Contains(text, "unknown variant") {
		return "本机 Agent 与模型配置不兼容，请检查 CLI 版本和模型设置。 / Check CLI version and model configuration compatibility."
	}
	if strings.Contains(text, "timeout") || strings.Contains(text, "stream disconnected") || strings.Contains(text, "connection refused") {
		return "本机 Agent 连接失败或超时，请检查网络和模型服务。 / Check the local Agent network and model service."
	}
	return fmt.Sprintf("本机 Agent 执行失败（退出码 %d），请由设备所有者检查 CLI。 / Local Agent failed (exit code %d).", result.ExitCode, result.ExitCode)
}

func completionContinuationPrompt(task domain.ClaimedTask, previousOutput string, includeOriginal bool, capabilityContext string) string {
	var builder strings.Builder
	if includeOriginal {
		builder.WriteString(buildPrompt(task))
		if capabilityContext != "" {
			builder.WriteString("\n\n")
			builder.WriteString(capabilityContext)
		}
		builder.WriteString("\n\n")
	}
	builder.WriteString("你上一次回复的是调查计划或过程状态，尚未满足验收标准。不要再解释接下来打算做什么；请现在继续使用已有上下文和工具把任务做完，然后直接交付明确结论、核心依据和可执行建议。\n\n上一次未完成的回复：\n")
	builder.WriteString(previousOutput)
	return builder.String()
}

// NeedsCompletionFollowup detects the narrow, high-confidence case where a
// CLI exited successfully after emitting only a future-tense progress update.
// It deliberately avoids rejecting longer reports that happen to mention a
// next step among otherwise complete findings.
func NeedsCompletionFollowup(output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return true
	}
	if len([]rune(trimmed)) > 500 {
		return false
	}
	lower := strings.ToLower(trimmed)
	planMarkers := []string{"我会", "我将", "接下来", "下一步再", "准备继续", "会把范围", "i will", "i'll", "next i", "going to"}
	finalMarkers := []string{"结论", "发现", "建议", "证据", "评分", "风险", "综上", "因此", "已完成", "conclusion", "findings", "recommendation", "evidence", "completed"}
	hasPlan := false
	for _, marker := range planMarkers {
		hasPlan = hasPlan || strings.Contains(lower, marker)
	}
	if !hasPlan {
		return false
	}
	for _, marker := range finalMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

// AuthenticationRequiredOutput recognizes a native CLI's own login wall after
// its structured response has been reduced to display text. A zero process
// exit code does not make this a successful Agent delivery: the requested work
// was not performed until the owner completes login on the Agent's computer.
func AuthenticationRequiredOutput(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(lower, "authentication required") &&
		(strings.Contains(lower, "/login") || strings.Contains(lower, "sign in"))
}

func authenticationRequiredSummary(agent config.RunnerAgent) string {
	if agent.Type == "workbuddy" {
		return "WorkBuddy 本机尚未登录。请在这台电脑的 WorkBuddy / CodeBuddy 中完成登录；在 AgentCollab 群聊里发送 /login 不会登录 WorkBuddy。登录后重试。"
	}
	name := strings.TrimSpace(agent.DisplayName)
	if name == "" {
		name = agent.Type
	}
	return name + " 本机尚未登录。请在该 Agent 所在电脑完成登录后重试。"
}

// AgentExecutionFailureOutput recognizes native Agents that report a hard
// failure in ordinary text or JSON while still exiting with code 0. Such
// responses are not deliverables and must never advance a task or project
// board as if useful work had completed.
func AgentExecutionFailureOutput(output string) (string, bool) {
	trimmed := strings.TrimSpace(output)
	lower := strings.ToLower(trimmed)
	providerFailure := strings.Contains(lower, "insufficient balance") ||
		strings.Contains(lower, "billing or credits exhausted") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "http 402")
	if providerFailure {
		return "目标 Agent 的模型额度或计费不可用。请由 Agent 所有者在原软件中补充额度或切换可用模型后重试。\n\n原始错误：\n" + trimmed, true
	}
	missingRequest := strings.Contains(lower, "don't have the actual task content") ||
		strings.Contains(lower, "do not have the actual task content") ||
		strings.Contains(lower, "task content hasn't been provided") ||
		strings.Contains(lower, "task content has not been provided") ||
		strings.Contains(lower, "no task definition file") ||
		strings.Contains(lower, "任务内容还没下发") ||
		strings.Contains(lower, "任务内容没有下发") ||
		strings.Contains(lower, "没有实际任务内容")
	if missingRequest {
		return "目标 Agent 没有识别到真人发送的实际任务，平台已阻止把这次空响应标记为交付。请更新该电脑的 AgentCollab Client 后重试。\n\n原始响应：\n" + trimmed, true
	}
	return "", false
}

type completionPayload struct {
	Success      bool                    `json:"success"`
	Output       string                  `json:"output"`
	ErrorSummary string                  `json:"error_summary"`
	ExitCode     int                     `json:"exit_code"`
	Receipt      domain.ExecutionReceipt `json:"receipt"`
}

type pendingCompletion struct {
	TaskID    domain.TaskID     `json:"task_id"`
	AttemptNo int               `json:"attempt_no"`
	Payload   completionPayload `json:"payload"`
}

func (c *Client) complete(ctx context.Context, task domain.ClaimedTask, success bool, output, summary string, exitCode int, receipt domain.ExecutionReceipt) error {
	pending := pendingCompletion{
		TaskID: task.ID, AttemptNo: task.AttemptNo,
		Payload: completionPayload{Success: success, Output: output, ErrorSummary: summary, ExitCode: exitCode, Receipt: receipt},
	}
	path, err := c.persistPendingCompletion(pending)
	if err != nil {
		return fmt.Errorf("persist completion before delivery: %w", err)
	}
	completionCtx, cancel := completionRequestContext(ctx)
	defer cancel()
	if err := c.post(completionCtx, "/api/v1/runner/tasks/"+string(task.ID)+"/complete", pending.Payload, nil); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove delivered completion from outbox: %w", err)
	}
	return nil
}

func (c *Client) completionOutboxDir() string {
	return filepath.Join(c.config.LocalDir, "completion-outbox")
}

func (c *Client) pendingCompletionPath(taskID domain.TaskID, attemptNo int) string {
	safeID := strings.NewReplacer("/", "_", "\\", "_").Replace(string(taskID))
	return filepath.Join(c.completionOutboxDir(), fmt.Sprintf("%s-attempt-%d.json", safeID, attemptNo))
}

func (c *Client) persistPendingCompletion(pending pendingCompletion) (string, error) {
	if err := os.MkdirAll(c.completionOutboxDir(), 0o700); err != nil {
		return "", err
	}
	payload, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return "", err
	}
	path := c.pendingCompletionPath(pending.TaskID, pending.AttemptNo)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, path); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (c *Client) replayPendingCompletions(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(c.completionOutboxDir())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(c.completionOutboxDir(), entry.Name())
		payload, err := os.ReadFile(path)
		if err != nil {
			return delivered, err
		}
		var pending pendingCompletion
		if err := json.Unmarshal(payload, &pending); err != nil || pending.TaskID == "" {
			quarantine := path + ".invalid"
			_ = os.Rename(path, quarantine)
			fmt.Fprintf(c.stderr, "completion_outbox_invalid=%s\n", entry.Name())
			continue
		}
		if err := c.post(ctx, "/api/v1/runner/tasks/"+string(pending.TaskID)+"/complete", pending.Payload, nil); err != nil {
			return delivered, fmt.Errorf("replay completion for %s: %w", pending.TaskID, err)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

func (c *Client) prepareTaskStream(ctx context.Context) error {
	replayed, err := c.replayPendingCompletions(ctx)
	if err != nil {
		return err
	}
	if replayed > 0 {
		fmt.Fprintf(c.stdout, "completion_receipts_replayed=%d\n", replayed)
	}
	recovered, err := c.recoverInterruptedTasks(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	if recovered > 0 {
		fmt.Fprintf(c.stdout, "tasks_recovered=%d\n", recovered)
	}
	return nil
}

func (c *Client) recoverInterruptedTasks(ctx context.Context) (int, error) {
	var response struct {
		Requeued int `json:"requeued"`
	}
	if err := c.post(ctx, "/api/v1/runner/tasks/recover", map[string]any{}, &response); err != nil {
		return 0, err
	}
	return response.Requeued, nil
}

// completionRequestContext gives a Runner a short final window to report a
// task it has already claimed when its service context is being stopped. The
// native CLI is terminated with the Runner, but the Hub must not retain the
// task forever as "running" merely because the original request context was
// cancelled before the completion request could be sent.
func completionRequestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (c *Client) taskCancellationRequested(ctx context.Context, taskID domain.TaskID) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.HubURL+"/api/v1/runner/tasks/"+string(taskID)+"/cancellation", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
	response, err := c.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return false, fmt.Errorf("check task cancellation: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Cancelled bool `json:"cancelled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decode task cancellation: %w", err)
	}
	return payload.Cancelled, nil
}

func (c *Client) post(ctx context.Context, path string, body, target any) error {
	return postJSON(ctx, c.http, c.config.HubURL+path, c.credentials.Token, body, target)
}

func (c *Client) agentByServerID(serverID, sourceServerID, runtimeLocalKey, agentType string) (config.RunnerAgent, bool) {
	c.agentCredentialsMu.RLock()
	defer c.agentCredentialsMu.RUnlock()
	for _, agent := range c.agents {
		registeredID := c.credentials.Agents[agent.ID]
		if registeredID == serverID || (sourceServerID != "" && registeredID == sourceServerID) {
			return agent, true
		}
	}
	if runtimeLocalKey != "" {
		for _, agent := range c.agents {
			if agent.ID == runtimeLocalKey {
				return agent, true
			}
		}
	}
	// A workspace copy can outlive both the original server Agent ID and its
	// older scan-* local key. The claimed task still carries its real Agent
	// type, so use it only when it identifies one unambiguous local executable.
	// Multiple configured Agents of the same type remain deliberately unresolved
	// instead of running work through the wrong local profile.
	if agentType != "" {
		var matched config.RunnerAgent
		for _, agent := range c.agents {
			if agent.Type != agentType {
				continue
			}
			if matched.ID != "" {
				return config.RunnerAgent{}, false
			}
			matched = agent
		}
		if matched.ID != "" {
			return matched, true
		}
	}
	return config.RunnerAgent{}, false
}

func (c *Client) metadataForAgent(localID string) discoveredAgent {
	c.agentCredentialsMu.RLock()
	defer c.agentCredentialsMu.RUnlock()
	return c.metadata[localID]
}

func ParseOutput(mode, output string) string {
	output = strings.TrimSpace(output)
	if mode == "text" || output == "" {
		return output
	}
	if mode == "json-result" {
		var document any
		if json.Unmarshal([]byte(output), &document) == nil {
			if result := extractJSONResult(document); result != "" {
				return result
			}
		}
		lines := strings.Split(output, "\n")
		var last string
		for index := len(lines) - 1; index >= 0; index-- {
			var value any
			if json.Unmarshal([]byte(lines[index]), &value) == nil {
				if result := extractJSONResult(value); result != "" {
					last = result
					break
				}
			}
		}
		if last != "" {
			return last
		}
		return output
	}
	if mode == "pi-jsonl" {
		var last string
		scanner := bufio.NewScanner(strings.NewReader(output))
		for scanner.Scan() {
			var event map[string]any
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			if event["type"] != "message_end" && event["type"] != "turn_end" {
				continue
			}
			if message, ok := event["message"]; ok {
				if result := extractJSONResult(message); result != "" {
					last = result
				}
			}
		}
		if last != "" {
			return last
		}
		return output
	}
	var last string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type == "item.completed" && event.Item.Type == "agent_message" && strings.TrimSpace(event.Item.Text) != "" {
			last = strings.TrimSpace(event.Item.Text)
		}
	}
	if last != "" {
		return last
	}
	return output
}

func extractJSONResult(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if result, ok := typed["result"].(string); ok && strings.TrimSpace(result) != "" {
			return strings.TrimSpace(result)
		}
		if typed["role"] == "assistant" {
			if content, ok := typed["content"].([]any); ok {
				for index := len(content) - 1; index >= 0; index-- {
					if item, ok := content[index].(map[string]any); ok {
						if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
							return strings.TrimSpace(text)
						}
					}
				}
			}
		}
		if typed["type"] == "text" {
			if part, ok := typed["part"].(map[string]any); ok {
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	case []any:
		for index := len(typed) - 1; index >= 0; index-- {
			if result := extractJSONResult(typed[index]); result != "" {
				return result
			}
		}
	}
	return ""
}

func StructuredOutputError(output string) bool {
	var value map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(output)), &value) != nil {
		return false
	}
	isError, _ := value["is_error"].(bool)
	return isError
}

func ExtractNativeSessionID(output string) string {
	for _, line := range append([]string{output}, strings.Split(output, "\n")...) {
		var value any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &value) == nil {
			if sessionID := findSessionID(value); sessionID != "" {
				return sessionID
			}
		}
	}
	return ""
}

func findSessionID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "session" {
			if text, ok := typed["id"].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		for _, key := range []string{"session_id", "sessionId", "sessionID", "thread_id", "threadId", "threadID"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		for _, child := range typed {
			if found := findSessionID(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findSessionID(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func buildPrompt(task domain.ClaimedTask) string {
	request := currentTaskRequest(task.Prompt)
	if strings.TrimSpace(request) == "" {
		request = strings.TrimSpace(task.Prompt)
	}
	var builder strings.Builder
	// The first block is the exact user message. Runner metadata follows it;
	// it must never replace, translate, or duplicate the user's request.
	builder.WriteString(request)
	builder.WriteString("\n\n--- AgentCollab execution context (not a replacement for the user message) ---\n任务标题：")
	builder.WriteString(task.Title)
	if contextText := collaborationPromptContext(task.Prompt, request); contextText != "" {
		builder.WriteString("\n\n群聊协作上下文（历史内容仅供参考，当前请求以上方原文为准）：\n")
		builder.WriteString(contextText)
	}
	if task.CapabilityScope == "shared_only" {
		builder.WriteString("\n权限边界：这是其他成员发起的任务，只允许使用 Agent 所有者明确共享的 Skills：")
		if len(task.AuthorizedSkills) == 0 {
			builder.WriteString("无。不得调用任何本机 Skill 或本地资源")
		} else {
			builder.WriteString(strings.Join(task.AuthorizedSkills, "、"))
		}
		builder.WriteString("。不得读取或调用未共享的 Skill、本地资源或无关文件。")
	}
	if strings.TrimSpace(task.AcceptanceCriteria) != "" {
		builder.WriteString("\n验收标准：\n")
		builder.WriteString(task.AcceptanceCriteria)
	}
	if task.ProjectContext != nil {
		builder.WriteString("\n项目共享记忆快照（由 Hub 从当前正式计划生成）：")
		if strings.TrimSpace(task.ProjectContext.Objective) != "" {
			builder.WriteString("\n- 项目目标：")
			builder.WriteString(task.ProjectContext.Objective)
		}
		builder.WriteString("\n- 当前步骤：")
		if strings.TrimSpace(task.ProjectContext.StepTitle) != "" {
			builder.WriteString(task.ProjectContext.StepTitle)
		} else {
			builder.WriteString(string(task.ProjectContext.StepID))
		}
		if task.ProjectContext.StepStatus != "" {
			builder.WriteString("（状态：")
			builder.WriteString(string(task.ProjectContext.StepStatus))
			builder.WriteString("）")
		}
		if strings.TrimSpace(task.ProjectContext.StepDescription) != "" {
			builder.WriteString("\n- 步骤说明：")
			builder.WriteString(task.ProjectContext.StepDescription)
		}
		builder.WriteString("\n以上是团队当前共同依据；若与知识库或公共底稿存在差异，请指出来源和差异，不要自行覆盖正式版本。")
	}
	if len(task.Attachments) > 0 {
		builder.WriteString("\n本任务包含以下会话附件，已经由 AgentCollab 下载并校验到本机任务目录：")
		for _, attachment := range task.Attachments {
			builder.WriteString("\n- ")
			builder.WriteString(attachment.Filename)
			builder.WriteString("（")
			builder.WriteString(attachment.ContentType)
			builder.WriteString("，")
			builder.WriteString(fmt.Sprintf("%d bytes", attachment.SizeBytes))
			builder.WriteString("）：")
			builder.WriteString(attachment.LocalPath)
		}
		builder.WriteString("\n请直接读取上述真实本地文件，并按任务要求调用合适的本机 Skill；不要向用户重复索要已经提供的附件。")
	}
	if len(task.KnowledgeAssets) > 0 {
		builder.WriteString("\n本群项目知识库包含以下已共享参考资料，已经由 AgentCollab 下载并校验到本机任务目录：")
		for _, asset := range task.KnowledgeAssets {
			builder.WriteString("\n- [")
			if asset.Scope == domain.KnowledgeWorkspace {
				builder.WriteString("工作区共享")
			} else {
				builder.WriteString("本群资料")
			}
			builder.WriteString("] ")
			builder.WriteString(asset.Filename)
			builder.WriteString("：")
			builder.WriteString(asset.LocalPath)
		}
		builder.WriteString("\n这些文件是协作成员明确放入项目知识库的共同参考资产。需要时请直接读取并引用；不要臆测文件内容。")
	}
	if task.ProjectWorkspace != nil {
		builder.WriteString("\n本任务已经自动取得群内公共底稿的固定版本。这个目录是本次任务的临时副本，不是 Server 上的公共底稿本体。\n- 项目步骤：")
		builder.WriteString(string(task.ProjectWorkspace.StepID))
		builder.WriteString("\n- 版本：")
		builder.WriteString(task.ProjectWorkspace.Commit)
		builder.WriteString("\n- 本机项目目录：")
		builder.WriteString(task.ProjectWorkspace.LocalPath)
		builder.WriteString("\n请首先依据该目录只读分析。即使任务要求修改，也只允许修改这个任务临时副本；修改不会自动回写或覆盖公共底稿。需要交付修改成果时放入 deliverables，之后仍须由负责人提交、审查和合并。不得改动任务目录之外的同名工程。")
	}
	if task.LocalWorkspace != nil {
		builder.WriteString("\n本任务明确要求查询步骤负责人电脑上的实时工作目录。\n- 项目步骤：")
		builder.WriteString(string(task.LocalWorkspace.StepID))
		builder.WriteString("\n- 目录标识：")
		builder.WriteString(task.LocalWorkspace.DirectoryLabel)
		builder.WriteString("\n- 本机实际目录：")
		builder.WriteString(task.LocalWorkspace.LocalPath)
		builder.WriteString("\n请直接依据该目录当前实际内容检查进展，不要先下载或改用 Server 公共底稿。除非任务明确要求修改，否则只读检查；不得在回复中泄露绝对路径。当前结果只是进展汇报，不会自动提交、审查或合并。")
	}
	builder.WriteString("\n如果任务需要交付报告、表格、演示文稿、图片等文件，请将最终文件放到 AgentCollab 指定的 deliverables 文件夹")
	if task.DeliveryDirectory != "" {
		builder.WriteString("：")
		builder.WriteString(task.DeliveryDirectory)
	}
	builder.WriteString("；最多 5 个、单个不超过 50 MB。不要把输入附件、知识库资料或整个项目副本放入该文件夹。")
	builder.WriteString("\n完成后直接返回可交付的最终结果。")
	return builder.String()
}

// writeTaskDefinition gives CLIs that inspect their working directory (not
// only argv/stdin) a stable, human-readable task entry point. It deliberately
// contains the extracted user request rather than the internal routing
// wrapper, so the native Agent sees the same task a human sent.
func writeTaskDefinition(workdir string, task domain.ClaimedTask) error {
	request := strings.TrimSpace(currentTaskRequest(task.Prompt))
	if request == "" {
		request = strings.TrimSpace(task.Prompt)
	}
	var builder strings.Builder
	builder.WriteString("# AgentCollab task\n\n")
	builder.WriteString("## User request\n\n")
	builder.WriteString(request)
	builder.WriteString("\n\n## Task title\n\n")
	builder.WriteString(strings.TrimSpace(task.Title))
	if strings.TrimSpace(task.AcceptanceCriteria) != "" {
		builder.WriteString("\n\n## Acceptance criteria\n\n")
		builder.WriteString(strings.TrimSpace(task.AcceptanceCriteria))
	}
	builder.WriteString("\n")
	return os.WriteFile(filepath.Join(workdir, "AGENT_TASK.md"), []byte(builder.String()), 0o600)
}

// Keep the Hub's bounded history, role and delegation directory when moving the
// current user request to the first block. Never treat a plain task as a wrapper.
func collaborationPromptContext(prompt, request string) string {
	const currentMarker = "\n\n当前消息：\n"
	const directoryMarker = "\n\n可协作 Agent："
	if !strings.HasPrefix(prompt, "你是 AgentCollab 群聊中的 ") {
		return ""
	}
	start := strings.LastIndex(prompt, currentMarker)
	if start < 0 {
		return ""
	}
	prefix := prompt[:start]
	if firstLine := strings.IndexByte(prefix, '\n'); firstLine >= 0 {
		prefix = prefix[firstLine+1:]
	} else {
		prefix = ""
	}
	// The latest source message is also the last entry in recentRoomContextTx.
	// Its exact text is already first in the native prompt.
	prefix = strings.TrimSuffix(prefix, "群成员："+request)
	contextText := strings.TrimSpace(prefix)
	current := prompt[start+len(currentMarker):]
	if end := strings.Index(current, directoryMarker); end >= 0 {
		contextText += current[end:]
	}
	return strings.TrimSpace(contextText)
}

func (c *Client) downloadTaskProjectWorkspace(ctx context.Context, task *domain.ClaimedTask, workdir string) error {
	if task.ProjectWorkspace == nil {
		return nil
	}
	snapshot := task.ProjectWorkspace
	if strings.TrimSpace(snapshot.Commit) == "" || strings.TrimSpace(snapshot.DownloadURL) == "" {
		return fmt.Errorf("共享项目目录信息不完整")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.HubURL+snapshot.DownloadURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
	client := *c.http
	client.Timeout = 10 * time.Minute
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download shared project: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("download shared project: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if response.Header.Get("X-AgentCollab-Project-Commit") != snapshot.Commit || response.Header.Get("X-AgentCollab-Project-Step") != string(snapshot.StepID) {
		return fmt.Errorf("共享项目目录版本不一致")
	}
	archivePath := filepath.Join(workdir, ".agentcollab-project.zip")
	archiveFile, err := os.OpenFile(archivePath+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	maxArchiveBytes := snapshot.TotalBytes + 64<<20
	if maxArchiveBytes < 64<<20 {
		maxArchiveBytes = 64 << 20
	}
	written, copyErr := io.Copy(archiveFile, io.LimitReader(response.Body, maxArchiveBytes+1))
	closeErr := archiveFile.Close()
	if copyErr != nil || closeErr != nil || written > maxArchiveBytes {
		_ = os.Remove(archivePath + ".part")
		if written > maxArchiveBytes {
			return fmt.Errorf("共享项目压缩包超出预期大小")
		}
		return fmt.Errorf("save shared project: %v", firstNonNil(copyErr, closeErr))
	}
	if err := os.Rename(archivePath+".part", archivePath); err != nil {
		_ = os.Remove(archivePath + ".part")
		return err
	}
	defer os.Remove(archivePath)
	destination := filepath.Join(workdir, "project")
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open shared project archive: %w", err)
	}
	defer reader.Close()
	var extractedBytes int64
	var extractedFiles int
	for _, entry := range reader.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if name == "project" && entry.FileInfo().IsDir() {
			continue
		}
		prefix := "project" + string(filepath.Separator)
		if !strings.HasPrefix(name, prefix) || filepath.IsAbs(name) || strings.Contains(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("共享项目包含不安全路径")
		}
		relative := strings.TrimPrefix(name, prefix)
		target := filepath.Join(destination, relative)
		if target != destination && !strings.HasPrefix(target, destination+string(filepath.Separator)) {
			return fmt.Errorf("共享项目目标路径不安全")
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("共享项目不允许符号链接")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		extractedBytes += int64(entry.UncompressedSize64)
		if extractedBytes > snapshot.TotalBytes {
			return fmt.Errorf("共享项目内容超出冻结大小")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			source.Close()
			return err
		}
		copied, copyErr := io.Copy(destinationFile, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
		closeErr := destinationFile.Close()
		sourceErr := source.Close()
		if copyErr != nil || closeErr != nil || sourceErr != nil || copied != int64(entry.UncompressedSize64) {
			return fmt.Errorf("extract shared project file %s", relative)
		}
		extractedFiles++
	}
	if extractedFiles != snapshot.FileCount || extractedBytes != snapshot.TotalBytes {
		return fmt.Errorf("共享项目清单不一致：得到 %d 文件/%d 字节，应为 %d/%d", extractedFiles, extractedBytes, snapshot.FileCount, snapshot.TotalBytes)
	}
	metadata := []byte("step_id=" + string(snapshot.StepID) + "\ncommit=" + snapshot.Commit + "\nrevision=" + fmt.Sprintf("%d", snapshot.Revision) + "\n")
	if err := os.WriteFile(filepath.Join(workdir, ".agentcollab-project-version"), metadata, 0o600); err != nil {
		return err
	}
	snapshot.LocalPath = destination
	fmt.Fprintf(c.stdout, "task_project_workspace=%s step=%s commit=%s files=%d bytes=%d path=%s\n", task.ID, snapshot.StepID, snapshot.Commit, extractedFiles, extractedBytes, destination)
	return nil
}

func (c *Client) downloadTaskAttachments(ctx context.Context, task *domain.ClaimedTask, workdir string) error {
	if len(task.Attachments) == 0 {
		return nil
	}
	attachmentDir := filepath.Join(workdir, "attachments")
	if err := os.MkdirAll(attachmentDir, 0o700); err != nil {
		return fmt.Errorf("create attachment directory: %w", err)
	}
	client := *c.http
	client.Timeout = 5 * time.Minute
	for index := range task.Attachments {
		attachment := &task.Attachments[index]
		filename := fmt.Sprintf("%02d-%s", index+1, safeRunnerAttachmentFilename(attachment.Filename))
		finalPath := filepath.Join(attachmentDir, filename)
		temporaryPath := finalPath + ".part"
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.HubURL+"/api/v1/runner/tasks/"+string(task.ID)+"/attachments/"+string(attachment.ID), nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("download %s: %w", attachment.Filename, err)
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			response.Body.Close()
			return fmt.Errorf("download %s: status %d: %s", attachment.Filename, response.StatusCode, strings.TrimSpace(string(body)))
		}
		file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			response.Body.Close()
			return fmt.Errorf("create %s: %w", filename, err)
		}
		hasher := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, attachment.SizeBytes+1))
		response.Body.Close()
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("save %s: %v", attachment.Filename, firstNonNil(copyErr, closeErr))
		}
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if size != attachment.SizeBytes || !strings.EqualFold(actualHash, attachment.SHA256) {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("integrity check failed for %s", attachment.Filename)
		}
		if err := os.Rename(temporaryPath, finalPath); err != nil {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("finish %s: %w", attachment.Filename, err)
		}
		attachment.LocalPath = finalPath
		fmt.Fprintf(c.stdout, "task_attachment=%s file=%s bytes=%d sha256=%s\n", task.ID, filename, size, actualHash)
	}
	return nil
}

func (c *Client) downloadTaskKnowledge(ctx context.Context, task *domain.ClaimedTask, workdir string) error {
	if len(task.KnowledgeAssets) == 0 {
		return nil
	}
	knowledgeDir := filepath.Join(workdir, "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o700); err != nil {
		return fmt.Errorf("create knowledge directory: %w", err)
	}
	client := *c.http
	client.Timeout = 5 * time.Minute
	for index := range task.KnowledgeAssets {
		asset := &task.KnowledgeAssets[index]
		filename := fmt.Sprintf("%02d-%s", index+1, safeRunnerAttachmentFilename(asset.Filename))
		finalPath := filepath.Join(knowledgeDir, filename)
		temporaryPath := finalPath + ".part"
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.HubURL+"/api/v1/runner/tasks/"+string(task.ID)+"/knowledge/"+string(asset.ID), nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+c.credentials.Token)
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("download %s: %w", asset.Filename, err)
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			response.Body.Close()
			return fmt.Errorf("download %s: status %d: %s", asset.Filename, response.StatusCode, strings.TrimSpace(string(body)))
		}
		file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			response.Body.Close()
			return fmt.Errorf("create %s: %w", filename, err)
		}
		hasher := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, asset.SizeBytes+1))
		response.Body.Close()
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("save %s: %v", asset.Filename, firstNonNil(copyErr, closeErr))
		}
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if size != asset.SizeBytes || !strings.EqualFold(actualHash, asset.SHA256) {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("integrity check failed for %s", asset.Filename)
		}
		if err := os.Rename(temporaryPath, finalPath); err != nil {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("finish %s: %w", asset.Filename, err)
		}
		asset.LocalPath = finalPath
		fmt.Fprintf(c.stdout, "task_knowledge=%s file=%s bytes=%d sha256=%s\n", task.ID, filename, size, actualHash)
	}
	return nil
}

func safeRunnerAttachmentFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" || value == "." {
		return "attachment"
	}
	return value
}

func firstNonNil(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func replaceTemplates(value, prompt, workdir string, task domain.ClaimedTask) string {
	replacer := strings.NewReplacer("{{prompt}}", prompt, "{{workdir}}", workdir, "{{title}}", task.Title, "{{task_id}}", string(task.ID), "{{room_id}}", string(task.RoomID), "{{acceptance_criteria}}", task.AcceptanceCriteria, "{{session_id}}", task.NativeSessionID)
	return replacer.Replace(value)
}

func argumentsForTask(agent config.RunnerAgent, task domain.ClaimedTask, prompt, workdir string) []string {
	template := agent.Args
	if task.NativeSessionID != "" && len(agent.ResumeArgs) > 0 {
		template = agent.ResumeArgs
	}
	args := make([]string, len(template))
	for index, argument := range template {
		args[index] = replaceTemplates(argument, prompt, workdir, task)
	}
	return args
}

func defaultOutputMode(value string) string {
	if value == "" {
		return "text"
	}
	return value
}

func postJSON(ctx context.Context, client *http.Client, url, token string, body, target any) error {
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &payload)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if target != nil && response.StatusCode != http.StatusNoContent {
		return json.NewDecoder(response.Body).Decode(target)
	}
	return nil
}

func saveCredentials(path string, credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func loadCredentials(path string) (Credentials, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read Runner credentials: %w", err)
	}
	var credentials Credentials
	if err := json.Unmarshal(payload, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("decode Runner credentials: %w", err)
	}
	if credentials.Token == "" || credentials.RunnerID == "" || len(credentials.Agents) == 0 {
		return Credentials{}, fmt.Errorf("Runner credentials are incomplete")
	}
	return credentials, nil
}
