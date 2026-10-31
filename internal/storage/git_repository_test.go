package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureWorkspaceGitRepositoryCreatesBareMainRepository(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspaceGitRepository(root, "workspace-git"); err != nil {
		t.Fatal(err)
	}
	relative, _ := WorkspaceGitDirectory("workspace-git")
	head, err := os.ReadFile(filepath.Join(root, relative, "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	if string(head) != "ref: refs/heads/main\n" {
		t.Fatalf("HEAD=%q", head)
	}
	repository := filepath.Join(root, relative)
	if output, err := exec.Command("git", "--git-dir", repository, "rev-list", "--count", "main").CombinedOutput(); err != nil || string(output) != "1\n" {
		t.Fatalf("initial main history output=%q err=%v", output, err)
	}
	info, err := WorkspaceGitInfo(root, "workspace-git")
	if err != nil || !info.Initialized || info.DefaultBranch != "main" || info.Transport != "server_managed" {
		t.Fatalf("repository info=%+v err=%v", info, err)
	}
	if err := EnsureWorkspaceGitRepository(root, "workspace-git"); err != nil {
		t.Fatalf("idempotent ensure: %v", err)
	}
}

func TestMergeWorkspaceGitBranchCreatesReviewedMergeCommit(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspaceGitRepository(root, "workspace-merge"); err != nil {
		t.Fatal(err)
	}
	relative, _ := WorkspaceGitDirectory("workspace-merge")
	repository := filepath.Join(root, relative)
	clone := filepath.Join(t.TempDir(), "clone")
	for _, command := range [][]string{{"clone", repository, clone}, {"-C", clone, "config", "user.name", "Contributor"}, {"-C", clone, "config", "user.email", "contributor@example.test"}, {"-C", clone, "switch", "-c", "issue/topic"}} {
		if output, err := exec.Command("git", command...).CombinedOutput(); err != nil {
			t.Fatalf("git %v output=%s err=%v", command, output, err)
		}
	}
	if err := os.WriteFile(filepath.Join(clone, "result.md"), []byte("accepted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"-C", clone, "add", "result.md"}, {"-C", clone, "commit", "-m", "Add result"}, {"-C", clone, "push", "origin", "issue/topic"}} {
		cmd := exec.Command("git", command...)
		if command[len(command)-1] == "issue/topic" {
			cmd.Env = append(os.Environ(), "AGENTCOLLAB_SERVER_MERGE=1")
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v output=%s err=%v", command, output, err)
		}
	}
	commit, err := MergeWorkspaceGitBranch(root, "workspace-merge", "issue/topic", "Reviewer")
	if err != nil || commit == "" {
		t.Fatalf("merge commit=%q err=%v", commit, err)
	}
	parents, err := exec.Command("git", "--git-dir", repository, "show", "-s", "--format=%P", "main").CombinedOutput()
	if err != nil || len(string(parents)) < 81 {
		t.Fatalf("merge parents=%q err=%v", parents, err)
	}
	if output, err := exec.Command("git", "--git-dir", repository, "show", "main:result.md").CombinedOutput(); err != nil || string(output) != "accepted\n" {
		t.Fatalf("merged file=%q err=%v", output, err)
	}
}
