package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestWorkspaceGitSmartHTTPCloneBranchPushAndProtectedMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	storageRoot := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(storageRoot, "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	if _, err := authService.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouterWithConfig(authService, store.NewCollabRepository(database), config.Hub{StorageRoot: storageRoot})
	server := httptest.NewServer(handler)
	defer server.Close()
	cookie := loginCookie(t, handler, "owner", "password-1234")
	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "LAN Project"}, cookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	repositoryResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+string(workspace.ID)+"/repository", nil, cookie)
	var repository struct {
		CloneURL string `json:"clone_url"`
	}
	decodeResponse(t, repositoryResponse, &repository)
	cloneURL := strings.Replace(repository.CloneURL, "http://example.com", server.URL, 1)
	authenticated := strings.Replace(cloneURL, "://", "://owner:password-1234@", 1)
	cloneDir := filepath.Join(t.TempDir(), "project")
	runGitCommand(t, "", "clone", authenticated, cloneDir)
	runGitCommand(t, cloneDir, "config", "user.name", "Owner")
	runGitCommand(t, cloneDir, "config", "user.email", "owner@example.test")
	runGitCommand(t, cloneDir, "switch", "-c", "issue/research")
	if err := os.WriteFile(filepath.Join(cloneDir, "finding.md"), []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, cloneDir, "add", "finding.md")
	runGitCommand(t, cloneDir, "commit", "-m", "Add research finding")
	runGitCommand(t, cloneDir, "push", "origin", "issue/research")
	runGitCommand(t, cloneDir, "switch", "main")
	if err := os.WriteFile(filepath.Join(cloneDir, "forbidden.md"), []byte("no direct main push\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, cloneDir, "add", "forbidden.md")
	runGitCommand(t, cloneDir, "commit", "-m", "Try direct main")
	command := exec.Command("git", "push", "origin", "main")
	command.Dir = cloneDir
	command.Env = gitTestEnvironment()
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "main is protected") {
		t.Fatalf("direct main push output=%s err=%v", output, err)
	}
	refs := exec.Command("git", "ls-remote", authenticated, "refs/heads/issue/research")
	refs.Env = gitTestEnvironment()
	output, err = refs.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "refs/heads/issue/research") {
		t.Fatalf("branch not visible output=%s err=%v", output, err)
	}

	unauthorized, _ := http.NewRequest(http.MethodGet, server.URL+"/git/"+string(workspace.ID)+"/project.git/info/refs?service=git-upload-pack", nil)
	response, err := http.DefaultClient.Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		payload, _ := json.Marshal(response.Header)
		t.Fatalf("unauthenticated git status=%d headers=%s", response.StatusCode, payload)
	}
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = gitTestEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s output=%s err=%v", strings.Join(args, " "), output, err)
	}
}

func gitTestEnvironment() []string {
	return append(os.Environ(), "NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.proxy", "GIT_CONFIG_VALUE_0=")
}
