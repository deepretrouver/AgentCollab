package runnerclient

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/agentcatalog"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestDefaultAgentCatalogIncludesSupportedTypes(t *testing.T) {
	catalog, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"hermes": false, "codex": false, "pi": false, "workbuddy": false, "claude-code": false, "opencode": false, "openclaw": false}
	for _, adapter := range catalog.Adapters {
		if _, ok := want[adapter.Type]; ok {
			want[adapter.Type] = true
		}
	}
	for agentType, found := range want {
		if !found {
			t.Fatalf("builtin Agent catalog missing %s", agentType)
		}
	}
}

func TestCatalogDiscoversBundledPiOutsideShellPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	node := filepath.Join(home, ".hermes", "node", "bin", "node")
	cli := filepath.Join(home, ".hermes", "node", "lib", "node_modules", "@earendil-works", "pi-coding-agent", "dist", "cli.js")
	for _, path := range []string{node, cli} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("placeholder"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	var agent config.RunnerAgent
	for _, adapter := range catalog.Adapters {
		if adapter.Type == "pi" {
			agent = resolveCatalogAdapter(adapter)
		}
	}
	if agent.Command != node || len(agent.Args) == 0 || agent.Args[0] != cli {
		t.Fatalf("bundled Pi was not discovered from catalog: agent=%+v", agent)
	}
	if !containsArgumentPair(agent.Args, "--mode", "json") || !containsArgumentPair(agent.ResumeArgs, "--fork", "{{session_id}}") {
		t.Fatalf("Pi catalog lost structured output or resume args: %+v", agent)
	}
}

func TestWorkBuddyCatalogAvoidsVersionSpecificSettingSourceFlag(t *testing.T) {
	catalog, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range catalog.Adapters {
		if adapter.Type != "workbuddy" {
			continue
		}
		seenLongPermissionFlag := false
		for _, arg := range append(adapter.Args, adapter.ResumeArgs...) {
			if arg == "--setting-sources" || arg == "user" {
				t.Fatalf("WorkBuddy catalog still contains version-specific setting source argument: %+v", adapter.Args)
			}
			if arg == "-y" {
				t.Fatalf("WorkBuddy catalog still contains unsupported short permission argument: %+v", adapter.Args)
			}
			if arg == "--dangerously-skip-permissions" {
				seenLongPermissionFlag = true
			}
		}
		if !seenLongPermissionFlag {
			t.Fatalf("WorkBuddy catalog must use the compatible long permission argument: %+v", adapter.Args)
		}
		return
	}
	t.Fatal("workbuddy adapter missing")
}

func TestWindowsCatalogCandidatesCoverNpmAndDesktopInstallations(t *testing.T) {
	home := `C:\Users\test`
	appData := filepath.Join(home, "AppData", "Roaming")
	localAppData := filepath.Join(home, "AppData", "Local")
	candidates := catalogCommandCandidatesForOS("codebuddy", "windows", home, appData, localAppData, `C:\\Program Files`, `C:\\Program Files (x86)`)
	want := []string{
		filepath.Join(appData, "npm", "codebuddy.cmd"),
		filepath.Join(localAppData, "Programs", "WorkBuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy"),
		filepath.Join(`C:\\Program Files`, "WorkBuddy", "resources", "app.asar.unpacked", "cli", "bin", "codebuddy.exe"),
	}
	for _, expected := range want {
		found := false
		for _, candidate := range candidates {
			if candidate == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Windows discovery candidate missing %q in %v", expected, candidates)
		}
	}
}

func TestDefaultCatalogIncludesKnownACPAdapters(t *testing.T) {
	catalog, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"codex-acp": false, "claude-code-acp": false}
	for _, adapter := range catalog.Adapters {
		if _, ok := want[adapter.Type]; ok {
			want[adapter.Type] = true
		}
	}
	for agentType, found := range want {
		if !found {
			t.Fatalf("builtin ACP catalog missing %s", agentType)
		}
	}
}

func TestCatalogLauncherPrependsBundledEntrypoint(t *testing.T) {
	root := t.TempDir()
	command := filepath.Join(root, "node")
	entrypoint := filepath.Join(root, "app", "cli.js")
	for _, path := range []string{command, entrypoint} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("placeholder"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	adapter := agentcatalog.Adapter{Type: "example", DisplayName: "Example", Transport: "native", CatalogTier: "native", Launchers: []agentcatalog.Launcher{{CommandPaths: []string{command}, EntrypointPath: entrypoint}}, Args: []string{"run", "{{prompt}}"}, VersionArgs: []string{"--version"}, OutputMode: "text"}
	agent := resolveCatalogAdapter(adapter)
	if agent.Command != command || len(agent.Args) < 2 || agent.Args[0] != entrypoint || agent.VersionArgs[0] != entrypoint {
		t.Fatalf("catalog launcher did not prepend bundled entrypoint: %+v", agent)
	}
}

func containsArgumentPair(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

func TestInstallationIDPersistsAcrossDeviceRenames(t *testing.T) {
	root := t.TempDir()
	credentialsPath := filepath.Join(root, "runner", "device.json")
	installationPath := filepath.Join(root, "identity", "installation-id")
	t.Setenv("AGENTCOLLAB_INSTALLATION_ID_FILE", installationPath)
	first, err := loadOrCreateInstallationID(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateInstallationID(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("installation ID changed: first=%q second=%q", first, second)
	}
	info, err := os.Stat(installationPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("installation ID permissions = %o, want private", info.Mode().Perm())
	}
}

func TestInstallationIDMigratesLegacyFileAndIgnoresConfigDirectory(t *testing.T) {
	root := t.TempDir()
	canonicalPath := filepath.Join(root, "identity", "installation-id")
	t.Setenv("AGENTCOLLAB_INSTALLATION_ID_FILE", canonicalPath)
	legacyCredentials := filepath.Join(root, "old-config", "device.json")
	legacyPath := filepath.Join(filepath.Dir(legacyCredentials), "installation-id")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	const legacyID = "1234567890abcdef1234567890abcdef"
	if err := os.WriteFile(legacyPath, []byte(legacyID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadOrCreateInstallationID(legacyCredentials)
	if err != nil || got != legacyID {
		t.Fatalf("migrated installation ID = %q, %v", got, err)
	}
	fromDifferentConfig, err := loadOrCreateInstallationID(filepath.Join(root, "new-config", "device.json"))
	if err != nil || fromDifferentConfig != legacyID {
		t.Fatalf("installation ID changed with config directory: %q, %v", fromDifferentConfig, err)
	}
}

func TestNewBackfillsStableIdentityForExistingCredentials(t *testing.T) {
	root := t.TempDir()
	installationPath := filepath.Join(root, "identity", "installation-id")
	t.Setenv("AGENTCOLLAB_INSTALLATION_ID_FILE", installationPath)
	credentialsPath := filepath.Join(root, "runner", "device.json")
	if err := saveCredentials(credentialsPath, Credentials{Token: "token", RunnerID: "runner", DeviceID: "device", WorkspaceID: "workspace", Agents: map[string]string{"codex": "agent"}}); err != nil {
		t.Fatal(err)
	}
	client, err := New(config.Runner{CredentialsFile: credentialsPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if client.credentials.InstallationID == "" {
		t.Fatal("existing credentials did not receive a stable installation ID")
	}
	reloaded, err := loadCredentials(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.InstallationID != client.credentials.InstallationID {
		t.Fatalf("stable identity was not saved: client=%q file=%q", client.credentials.InstallationID, reloaded.InstallationID)
	}
	stored, err := os.ReadFile(installationPath)
	if err != nil || strings.TrimSpace(string(stored)) != client.credentials.InstallationID {
		t.Fatalf("canonical installation ID = %q, %v", strings.TrimSpace(string(stored)), err)
	}
}

func TestNewPreservesExistingCredentialIdentityAtCanonicalPath(t *testing.T) {
	root := t.TempDir()
	installationPath := filepath.Join(root, "identity", "installation-id")
	t.Setenv("AGENTCOLLAB_INSTALLATION_ID_FILE", installationPath)
	credentialsPath := filepath.Join(root, "runner", "device.json")
	const existingID = "abcdef1234567890abcdef1234567890"
	if err := saveCredentials(credentialsPath, Credentials{Token: "token", RunnerID: "runner", DeviceID: "device", WorkspaceID: "workspace", Agents: map[string]string{"codex": "agent"}, InstallationID: existingID}); err != nil {
		t.Fatal(err)
	}
	client, err := New(config.Runner{CredentialsFile: credentialsPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if client.credentials.InstallationID != existingID {
		t.Fatalf("existing credential identity changed: %q", client.credentials.InstallationID)
	}
	stored, err := os.ReadFile(installationPath)
	if err != nil || strings.TrimSpace(string(stored)) != existingID {
		t.Fatalf("canonical installation ID = %q, %v", strings.TrimSpace(string(stored)), err)
	}
}

func TestParseOutput(t *testing.T) {
	codex := `{"type":"item.completed","item":{"type":"agent_message","text":"CODEX_OK"}}`
	if got := ParseOutput("codex-jsonl", codex); got != "CODEX_OK" {
		t.Fatalf("Codex output = %q", got)
	}
	workbuddy := `[{"type":"message","role":"user","content":[{"type":"input_text","text":"prompt"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"WORKBUDDY_OK"}]}]`
	if got := ParseOutput("json-result", workbuddy); got != "WORKBUDDY_OK" {
		t.Fatalf("WorkBuddy output = %q", got)
	}
	opencode := "{\"type\":\"step_start\",\"sessionID\":\"ses_123\"}\n{\"type\":\"text\",\"sessionID\":\"ses_123\",\"part\":{\"type\":\"text\",\"text\":\"OPENCODE_OK\"}}"
	if got := ParseOutput("json-result", opencode); got != "OPENCODE_OK" {
		t.Fatalf("OpenCode output = %q", got)
	}
	if got := ExtractNativeSessionID(opencode); got != "ses_123" {
		t.Fatalf("OpenCode session = %q", got)
	}
	pi := "{\"type\":\"session\",\"version\":3,\"id\":\"pi-session-123\"}\n{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"PI_OK\"}]}}\n{\"type\":\"agent_end\",\"messages\":[]}"
	if got := ParseOutput("pi-jsonl", pi); got != "PI_OK" {
		t.Fatalf("Pi output = %q", got)
	}
	if got := ExtractNativeSessionID(pi); got != "pi-session-123" {
		t.Fatalf("Pi session = %q", got)
	}
	if !StructuredOutputError(`{"is_error":true,"result":"authentication failed"}`) {
		t.Fatal("Claude structured error was not detected")
	}
	if !AuthenticationRequiredOutput("Authentication required. Please use /login command to sign in to your account") {
		t.Fatal("native authentication response was not detected")
	}
	if AuthenticationRequiredOutput("Please sign in to review the report, then provide a conclusion.") {
		t.Fatal("ordinary instruction was incorrectly treated as a native authentication response")
	}
}

func TestCompletionRequestContextSurvivesRunnerShutdown(t *testing.T) {
	stopped, stop := context.WithCancel(context.Background())
	stop()
	completionCtx, cancel := completionRequestContext(stopped)
	defer cancel()
	if err := completionCtx.Err(); err != nil {
		t.Fatalf("completion context inherited shutdown cancellation: %v", err)
	}
	if _, found := completionCtx.Deadline(); !found {
		t.Fatal("completion context has no bounded shutdown deadline")
	}
}

func TestArgumentsForTaskUsesNativeSessionResume(t *testing.T) {
	agent := config.RunnerAgent{Args: []string{"new", "{{prompt}}"}, ResumeArgs: []string{"resume", "{{session_id}}", "{{prompt}}"}}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-1", RoomID: "room-1"}, NativeSessionID: "session-123"}
	args := argumentsForTask(agent, task, "continue", "/tmp/work")
	if len(args) != 3 || args[0] != "resume" || args[1] != "session-123" || args[2] != "continue" {
		t.Fatalf("resume args = %#v", args)
	}
}

func TestWatchTaskCancellationCancelsLocalExecutionContext(t *testing.T) {
	var checks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/runner/tasks/task-1/cancellation" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer runner-token" {
			t.Fatalf("missing Runner authorization")
		}
		cancelled := checks.Add(1) >= 2
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cancelled":` + strconv.FormatBool(cancelled) + `}`))
	}))
	defer server.Close()

	client := &Client{config: config.Runner{HubURL: server.URL}, credentials: Credentials{Token: "runner-token"}, http: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, watcher := client.watchTaskCancellation(ctx, "task-1", cancel)
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation signal did not stop local context")
	}
	if !watcher.cancelled.Load() {
		t.Fatal("watcher did not record cancellation")
	}
}

func TestExecuteAgentCommandStopsWhenContextIsCancelled(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep command is unavailable on this platform")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	result := (&Client{}).executeAgentCommand(ctx, config.RunnerAgent{Command: sleep, Args: []string{"10"}}, domain.ClaimedTask{Task: domain.Task{ID: "task-cancel"}}, "", t.TempDir())
	if result.Err == nil {
		t.Fatal("cancelled CLI command unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancelled CLI command ran for %s", elapsed)
	}
}

func TestUnsupportedCommandOptionRecognizesCommonCLIErrors(t *testing.T) {
	for message, want := range map[string]string{
		"error: unknown option '-y'":                    "-y",
		"error: unknown option '--setting-sources'":     "--setting-sources",
		"unrecognized argument: --legacy-output-format": "--legacy-output-format",
	} {
		got := unsupportedCommandOption(agentCommandResult{Stderr: message, Err: errors.New("exit status 1")})
		if got != want {
			t.Fatalf("unsupported option from %q = %q, want %q", message, got, want)
		}
	}
}

func TestRemoveUnsupportedCommandOptionKeepsPromptAndRemovesLiteralValue(t *testing.T) {
	template := []string{"-p", "--setting-sources", "user", "--output-format", "json", "{{prompt}}"}
	args := []string{"-p", "--setting-sources", "user", "--output-format", "json", "@WorkBuddy 介绍下自己"}
	got, gotTemplate, ok := removeUnsupportedCommandOption(args, template, "--setting-sources")
	if !ok {
		t.Fatal("unsupported option was not removed")
	}
	want := []string{"-p", "--output-format", "json", "@WorkBuddy 介绍下自己"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("adapted args = %#v, want %#v", got, want)
	}
	if len(gotTemplate) != 4 || gotTemplate[len(gotTemplate)-1] != "{{prompt}}" {
		t.Fatalf("adapted template lost prompt: %#v", gotTemplate)
	}

	got, _, ok = removeUnsupportedCommandOption([]string{"-y", "{{already-expanded}}"}, []string{"-y", "{{prompt}}"}, "-y")
	if !ok || len(got) != 1 || got[0] != "{{already-expanded}}" {
		t.Fatalf("boolean option removal consumed prompt: %#v", got)
	}
}

func TestAgentExecutionFailureOutputRejectsFalseDeliveries(t *testing.T) {
	for _, output := range []string{
		"Billing or credits exhausted: HTTP 402: Insufficient Balance",
		"The task directory is empty — there's no task definition file. I don't have the actual task content.",
		"AgentCollab 上下文已就位，但任务内容还没下发。",
	} {
		if summary, failed := AgentExecutionFailureOutput(output); !failed || strings.TrimSpace(summary) == "" {
			t.Fatalf("false delivery was accepted: %q summary=%q", output, summary)
		}
	}
	if summary, failed := AgentExecutionFailureOutput("结论：已完成核验，结果为 42。"); failed || summary != "" {
		t.Fatalf("valid delivery was rejected: summary=%q", summary)
	}
}

func TestAgentByServerIDUsesCanonicalRegistrationForWorkspaceReuseAfterLocalKeyRename(t *testing.T) {
	client := &Client{
		agents:      []config.RunnerAgent{{ID: "codex-local", Type: "codex"}},
		credentials: Credentials{Agents: map[string]string{"codex-local": "source-agent"}},
	}
	agent, found := client.agentByServerID("workspace-copy", "source-agent", "scan-codex", "codex")
	if !found || agent.ID != "codex-local" {
		t.Fatalf("reused Agent lookup=%+v, found=%v", agent, found)
	}
}

func TestAgentByServerIDFallsBackToUniqueConfiguredTypeAfterMigration(t *testing.T) {
	client := &Client{agents: []config.RunnerAgent{
		{ID: "codex-local", Type: "codex"},
		{ID: "hermes-chengong", Type: "hermes"},
	}}
	agent, found := client.agentByServerID("workspace-copy", "deleted-source", "scan-codex", "codex")
	if !found || agent.ID != "codex-local" {
		t.Fatalf("type fallback lookup=%+v, found=%v", agent, found)
	}
}

func TestAgentByServerIDDoesNotGuessBetweenSameTypeProfiles(t *testing.T) {
	client := &Client{agents: []config.RunnerAgent{
		{ID: "research-hermes", Type: "hermes"},
		{ID: "ops-hermes", Type: "hermes"},
	}}
	if _, found := client.agentByServerID("workspace-copy", "deleted-source", "scan-hermes", "hermes"); found {
		t.Fatal("ambiguous Agent type unexpectedly resolved")
	}
}

func TestDownloadTaskAttachmentsVerifiesAndAddsLocalPathToPrompt(t *testing.T) {
	payload := []byte("investment committee report")
	digest := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tasks/task-1/attachments/attachment-1" || r.Header.Get("Authorization") != "Bearer runner-token" {
			http.Error(w, "unexpected request", http.StatusForbidden)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	var output bytes.Buffer
	client := &Client{config: config.Runner{HubURL: server.URL}, credentials: Credentials{Token: "runner-token"}, http: server.Client(), stdout: &output}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-1", RoomID: "room-1", Title: "投委会审查", Prompt: "请审查报告"}, Attachments: []domain.Attachment{{
		ID: "attachment-1", Filename: "授信报告.pdf", ContentType: "application/pdf", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:]),
	}}}
	workdir := t.TempDir()
	if err := client.downloadTaskAttachments(context.Background(), &task, workdir); err != nil {
		t.Fatal(err)
	}
	if len(task.Attachments) != 1 || task.Attachments[0].LocalPath == "" {
		t.Fatalf("attachments=%#v", task.Attachments)
	}
	stored, err := os.ReadFile(task.Attachments[0].LocalPath)
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
	prompt := buildPrompt(task)
	if !strings.Contains(prompt, task.Attachments[0].LocalPath) || !strings.Contains(prompt, "不要向用户重复索要") {
		t.Fatalf("prompt=%q", prompt)
	}
}

func TestDownloadTaskKnowledgeAddsSharedAssetsToPrompt(t *testing.T) {
	payload := []byte("shared research policy")
	digest := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tasks/task-knowledge/knowledge/knowledge-1" || r.Header.Get("Authorization") != "Bearer runner-token" {
			http.Error(w, "unexpected request", http.StatusForbidden)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	var output bytes.Buffer
	client := &Client{config: config.Runner{HubURL: server.URL}, credentials: Credentials{Token: "runner-token"}, http: server.Client(), stdout: &output}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-knowledge", RoomID: "room-1", Title: "研究复核", Prompt: "根据项目资料复核"}, KnowledgeAssets: []domain.KnowledgeAsset{{
		ID: "knowledge-1", Scope: domain.KnowledgeWorkspace, Filename: "研究规范.md", ContentType: "text/markdown", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:]),
	}}}
	if err := client.downloadTaskKnowledge(context.Background(), &task, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if task.KnowledgeAssets[0].LocalPath == "" {
		t.Fatalf("knowledge=%#v", task.KnowledgeAssets)
	}
	prompt := buildPrompt(task)
	if !strings.Contains(prompt, "工作区共享") || !strings.Contains(prompt, task.KnowledgeAssets[0].LocalPath) || !strings.Contains(prompt, "不要臆测文件内容") {
		t.Fatalf("prompt=%q", prompt)
	}
}

func TestBuildPromptIncludesFactualProjectMemoryAfterOriginalRequest(t *testing.T) {
	task := domain.ClaimedTask{
		Task: domain.Task{ID: "task-context", RoomID: "room-1", Title: "仓位研究", Prompt: "请复核最新仓位方案"},
		ProjectContext: &domain.ProjectContextSnapshot{
			Objective: "形成红利交易增强策略", StepID: "step-position", StepTitle: "配置仓位管理",
			StepDescription: "确定仓位上限和再平衡规则", StepStatus: domain.ProjectStepInProgress,
		},
	}
	prompt := buildPrompt(task)
	if !strings.HasPrefix(prompt, "请复核最新仓位方案") {
		t.Fatalf("original request is not first: %q", prompt)
	}
	for _, want := range []string{"项目共享记忆快照", "形成红利交易增强策略", "配置仓位管理", "确定仓位上限和再平衡规则"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %q", want, prompt)
		}
	}
}

func TestDownloadTaskProjectWorkspaceExtractsFrozenSharedDirectory(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("project/src/strategy.py")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("print('shared')\n")
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tasks/task-project/project-workspace" || r.Header.Get("Authorization") != "Bearer runner-token" {
			http.Error(w, "unexpected request", http.StatusForbidden)
			return
		}
		w.Header().Set("X-AgentCollab-Project-Commit", "abc123")
		w.Header().Set("X-AgentCollab-Project-Step", "step-strategy")
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	var output bytes.Buffer
	client := &Client{config: config.Runner{HubURL: server.URL}, credentials: Credentials{Token: "runner-token"}, http: server.Client(), stdout: &output}
	task := domain.ClaimedTask{Task: domain.Task{ID: "task-project", RoomID: "room-1", Title: "修改策略", Prompt: "检查共享工程"}, ProjectWorkspace: &domain.ProjectWorkspaceSnapshot{
		WorkspaceID: "workspace-red", StepID: "step-strategy", Commit: "abc123", Revision: 1, FileCount: 1, TotalBytes: int64(len(content)), DownloadURL: "/api/v1/runner/tasks/task-project/project-workspace",
	}}
	workdir := t.TempDir()
	if err := client.downloadTaskProjectWorkspace(context.Background(), &task, workdir); err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(workdir, "project")
	if task.ProjectWorkspace.LocalPath != wantPath {
		t.Fatalf("local path=%q want=%q", task.ProjectWorkspace.LocalPath, wantPath)
	}
	stored, err := os.ReadFile(filepath.Join(wantPath, "src", "strategy.py"))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
	prompt := buildPrompt(task)
	if !strings.Contains(prompt, wantPath) || !strings.Contains(prompt, "abc123") || !strings.Contains(prompt, "公共底稿") || !strings.Contains(prompt, "临时副本") || !strings.Contains(prompt, "不能") && !strings.Contains(prompt, "不得") {
		t.Fatalf("prompt=%q", prompt)
	}
	version, err := os.ReadFile(filepath.Join(workdir, ".agentcollab-project-version"))
	if err != nil || !strings.Contains(string(version), "commit=abc123") {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestNeedsCompletionFollowupRejectsProgressOnlyOutput(t *testing.T) {
	progressOnly := "第一轮搜索里大量命中是第三方包和旧资料，真正可用的线索不多。我会把范围收窄到 Hermes/AgentCollab 配置和定时任务脚本。"
	if !NeedsCompletionFollowup(progressOnly) {
		t.Fatal("progress-only output was accepted as a final delivery")
	}
	completed := "结论：工程链路存在超时风险。证据是近三次下午任务均超时。建议先分拆 prompt 并增加降级路径；下一步补回放测试。"
	if NeedsCompletionFollowup(completed) {
		t.Fatal("completed report was rejected because it also contained a next step")
	}
}

func TestEffectiveMaxConcurrentTasks(t *testing.T) {
	if got := effectiveMaxConcurrentTasks(0, 5); got != 5 {
		t.Fatalf("default concurrency = %d, want 5", got)
	}
	if got := effectiveMaxConcurrentTasks(3, 5); got != 3 {
		t.Fatalf("configured concurrency = %d, want 3", got)
	}
	if got := effectiveMaxConcurrentTasks(0, 0); got != 1 {
		t.Fatalf("empty concurrency = %d, want 1", got)
	}
}

func TestDiscoverLocalCapabilitiesKeepsPathsOnRunner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(root, "stock-analysis")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Analyze local stock data\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resourcePath := filepath.Join(t.TempDir(), "research.db")
	if err := os.WriteFile(resourcePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent := config.RunnerAgent{Type: "custom", SkillRoots: []string{root}}
	roots, skills, resources := discoverLocalCapabilities(agent, []config.RunnerResource{{Name: "research-db", Path: resourcePath, Description: "Local research database", Access: "read_write"}})
	if len(roots) != 1 || len(skills) != 1 || skills[0].Capability.Name != "stock-analysis" || skills[0].Path != filepath.Join(skillDir, "SKILL.md") {
		t.Fatalf("discovered skills = %#v roots=%#v", skills, roots)
	}
	if skills[0].Capability.Source != "自定义目录" {
		t.Fatalf("skill source = %q", skills[0].Capability.Source)
	}
	if len(resources) != 2 || resources[1].Capability.Name != "research-db" || resources[1].Path != resourcePath {
		t.Fatalf("discovered resources = %#v", resources)
	}
	item := discoveredAgent{Agent: agent, SkillRoots: roots, Skills: skills, Resources: resources}
	prompt := localCapabilityPrompt(item, "请调用 stock-analysis Skill")
	if !strings.Contains(prompt, filepath.Join(skillDir, "SKILL.md")) || !strings.Contains(prompt, resourcePath) || !strings.Contains(prompt, "stock-analysis") || strings.Contains(prompt, "Analyze local stock data") {
		t.Fatalf("capability prompt = %q", prompt)
	}
	matchedSkills, matchedResources := matchedCapabilityNames("请用 stock-analysis 查询 research-db", skills, resources)
	if len(matchedSkills) != 1 || matchedSkills[0] != "stock-analysis" || len(matchedResources) != 0 {
		t.Fatalf("matches without explicit resource marker = %#v / %#v", matchedSkills, matchedResources)
	}
	matchedSkills, matchedResources = matchedCapabilityNames("请调用 stock-analysis Skill，并使用 research-db 本地资源", skills, resources)
	if len(matchedSkills) != 1 || matchedSkills[0] != "stock-analysis" || len(matchedResources) != 1 || matchedResources[0] != "research-db" {
		t.Fatalf("matches = %#v / %#v", matchedSkills, matchedResources)
	}
}

func TestMatchedCapabilitiesDoesNotTreatAgentNameAsSkillUse(t *testing.T) {
	skills := []localSkill{{Capability: domain.AgentSkill{Name: "codex"}}, {Capability: domain.AgentSkill{Name: "stock-analysis"}}}
	matched, _ := matchedCapabilityNames("Codex Local 是群聊成员；请调用 stock-analysis Skill", skills, nil)
	if len(matched) != 1 || matched[0] != "stock-analysis" {
		t.Fatalf("matched skills = %#v", matched)
	}
}

func TestCurrentTaskRequestExcludesPriorCapabilityReceipts(t *testing.T) {
	prompt := "最近群聊上下文：\n任务/交付提及 Skill：codex\n\n当前消息：\n@Hermes 请调用 hermes-skills-architecture Skill\n\n可协作 Agent：@Codex Local"
	current := currentTaskRequest(prompt)
	if strings.Contains(current, "Skill：codex") || !strings.Contains(current, "hermes-skills-architecture") {
		t.Fatalf("current task request = %q", current)
	}
	skills := []localSkill{{Capability: domain.AgentSkill{Name: "codex"}}, {Capability: domain.AgentSkill{Name: "hermes-skills-architecture"}}}
	matched, _ := matchedCapabilityNames(current, skills, nil)
	if len(matched) != 1 || matched[0] != "hermes-skills-architecture" {
		t.Fatalf("matched current skills = %#v", matched)
	}
}

func TestWriteTaskDefinitionUsesUserRequestInsteadOfRoutingWrapper(t *testing.T) {
	workdir := t.TempDir()
	task := domain.ClaimedTask{Task: domain.Task{
		Title:              "报数",
		Prompt:             "你是 AgentCollab 群聊中的 WorkBuddy。\n\n最近群聊上下文：\n旧消息\n\n当前消息：\n@郑博文 WorkBuddy 报数\n\n可协作 Agent：@pi",
		AcceptanceCriteria: "给出实际数字",
	}}
	if err := writeTaskDefinition(workdir, task); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(workdir, "AGENT_TASK.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "@郑博文 WorkBuddy 报数") || strings.Contains(text, "你是 AgentCollab 群聊中的 WorkBuddy") {
		t.Fatalf("task definition leaked routing wrapper or lost request: %q", text)
	}
}

func TestBuildPromptKeepsUserMessageVerbatimAndDoesNotDuplicateIt(t *testing.T) {
	const userMessage = "@杨凯翔 WorkBuddy 介绍下自己"
	task := domain.ClaimedTask{Task: domain.Task{
		Title:  "群聊点名：杨凯翔 WorkBuddy",
		Prompt: "你是 AgentCollab 群聊中的 WorkBuddy。\n\n当前消息：\n" + userMessage + "\n\n可协作 Agent：@Codex",
	}}
	prompt := buildPrompt(task)
	if !strings.HasPrefix(prompt, userMessage+"\n\n") {
		t.Fatalf("prompt does not begin with the verbatim user message: %q", prompt)
	}
	if strings.Count(prompt, userMessage) != 1 {
		t.Fatalf("user message was translated or duplicated: count=%d prompt=%q", strings.Count(prompt, userMessage), prompt)
	}
	if strings.Contains(prompt, "你是 AgentCollab 群聊中的 WorkBuddy") || strings.Contains(prompt, "任务要求：\n"+userMessage) {
		t.Fatalf("routing wrapper leaked into the Agent request: %q", prompt)
	}
}

func TestBuildPromptPreservesCollaborationContext(t *testing.T) {
	request := "请复述此前的确认串"
	task := domain.ClaimedTask{Task: domain.Task{Prompt: "你是 AgentCollab 群聊中的 Hermes（hermes）。\n你的群内角色：复核\n\n最近群聊上下文：\nOpenCode：TEAM_OK_123\n\n群成员：" + request + "\n\n当前消息：\n" + request + "\n\n可协作 Agent：@OpenCode"}}
	prompt := buildPrompt(task)
	for _, expected := range []string{"TEAM_OK_123", "你的群内角色：复核", "可协作 Agent：@OpenCode"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("lost collaboration context %q", expected)
		}
	}
	if !strings.HasPrefix(prompt, request+"\n\n") || strings.Count(prompt, request) != 1 {
		t.Fatalf("current request must remain first and not be duplicated: %q", prompt)
	}
}

func TestCollaborationPromptContextDoesNotInventHistory(t *testing.T) {
	for _, prompt := range []string{"ordinary task", "用户引用\n\n当前消息：\nquoted", "你是 AgentCollab 群聊中的 Hermes。"} {
		if got := collaborationPromptContext(prompt, "quoted"); got != "" {
			t.Fatalf("plain or incomplete prompt acquired context: %q", got)
		}
	}
	prompt := "你是 AgentCollab 群聊中的 Hermes。\n这是平台汇总回调：请整合专家结果。\n最近群聊上下文：\nReviewer：结论 A\n\n当前消息：\n总结"
	got := collaborationPromptContext(prompt, "总结")
	if !strings.Contains(got, "平台汇总回调") || !strings.Contains(got, "Reviewer：结论 A") {
		t.Fatalf("callback instructions or expert result lost: %q", got)
	}
}
