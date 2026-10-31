package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrGitBranchNotFound = errors.New("git branch not found")
	ErrGitMergeConflict  = errors.New("git merge conflict")
)

type GitRepositoryInfo struct {
	Initialized   bool   `json:"initialized"`
	DefaultBranch string `json:"default_branch"`
	StoragePath   string `json:"storage_path"`
	Transport     string `json:"transport"`
}

type WorkspaceReviewFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type WorkspaceBranchReview struct {
	Branch       string                `json:"branch"`
	BaseCommit   string                `json:"base_commit"`
	HeadCommit   string                `json:"head_commit"`
	Mergeable    bool                  `json:"mergeable"`
	FileCount    int                   `json:"file_count"`
	Additions    int                   `json:"additions"`
	Deletions    int                   `json:"deletions"`
	ChangedFiles []WorkspaceReviewFile `json:"changed_files"`
}

// ReviewWorkspaceGitBranch returns the server-calculated change summary shown
// before a reviewer can merge. Only files belonging to the linked project step
// are included in the product review surface.
func ReviewWorkspaceGitBranch(root, workspaceID, stepID, branch string) (WorkspaceBranchReview, error) {
	if err := EnsureWorkspaceGitRepository(root, workspaceID); err != nil {
		return WorkspaceBranchReview{}, err
	}
	if _, err := projectSegment(stepID); err != nil {
		return WorkspaceBranchReview{}, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "main" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
		return WorkspaceBranchReview{}, ErrGitBranchNotFound
	}
	relative, _ := WorkspaceGitDirectory(workspaceID)
	repository, err := ResolveWithin(root, relative)
	if err != nil {
		return WorkspaceBranchReview{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return WorkspaceBranchReview{}, err
	}
	branchRef := "refs/heads/" + branch
	head, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", branchRef)
	if err != nil {
		return WorkspaceBranchReview{}, ErrGitBranchNotFound
	}
	base, err := runGit(git, nil, "--git-dir", repository, "merge-base", "refs/heads/main", branchRef)
	if err != nil {
		return WorkspaceBranchReview{}, err
	}
	review := WorkspaceBranchReview{Branch: branch, BaseCommit: strings.TrimSpace(base), HeadCommit: strings.TrimSpace(head), Mergeable: true}
	stepPath := filepath.ToSlash(filepath.Join("steps", stepID))
	statusOutput, err := runGit(git, nil, "--git-dir", repository, "diff", "--name-status", "refs/heads/main..."+branchRef, "--", stepPath)
	if err != nil {
		return WorkspaceBranchReview{}, err
	}
	byPath := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(statusOutput), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		path := fields[len(fields)-1]
		path = strings.TrimPrefix(path, stepPath+"/")
		review.ChangedFiles = append(review.ChangedFiles, WorkspaceReviewFile{Path: path, Status: fields[0]})
		byPath[path] = len(review.ChangedFiles) - 1
	}
	numstat, err := runGit(git, nil, "--git-dir", repository, "diff", "--numstat", "refs/heads/main..."+branchRef, "--", stepPath)
	if err != nil {
		return WorkspaceBranchReview{}, err
	}
	for _, line := range strings.Split(strings.TrimSpace(numstat), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		path := strings.TrimPrefix(fields[len(fields)-1], stepPath+"/")
		index, exists := byPath[path]
		if !exists {
			continue
		}
		if value, parseErr := strconv.Atoi(fields[0]); parseErr == nil {
			review.ChangedFiles[index].Additions = value
			review.Additions += value
		}
		if value, parseErr := strconv.Atoi(fields[1]); parseErr == nil {
			review.ChangedFiles[index].Deletions = value
			review.Deletions += value
		}
	}
	review.FileCount = len(review.ChangedFiles)
	mainCommit, _ := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", "refs/heads/main")
	if _, mergeErr := runGit(git, nil, "--git-dir", repository, "merge-tree", "--write-tree", strings.TrimSpace(mainCommit), review.HeadCommit); mergeErr != nil {
		review.Mergeable = false
	}
	return review, nil
}

func EnsureWorkspaceGitRepository(root, workspaceID string) error {
	if err := EnsureWorkspaceTree(root, workspaceID); err != nil {
		return err
	}
	relative, err := WorkspaceGitDirectory(workspaceID)
	if err != nil {
		return err
	}
	absolute, err := ResolveWithin(root, relative)
	if err != nil {
		return err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git executable not found: %w", err)
	}
	if _, err := os.Stat(filepath.Join(absolute, "HEAD")); os.IsNotExist(err) {
		if _, err := runGit(git, nil, "init", "--bare", "--initial-branch=main", absolute); err != nil {
			return fmt.Errorf("initialize workspace git repository: %w", err)
		}
	}
	if _, err := runGit(git, nil, "--git-dir", absolute, "rev-parse", "--verify", "refs/heads/main"); err != nil {
		tree, err := runGit(git, bytes.NewReader(nil), "--git-dir", absolute, "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return fmt.Errorf("create initial git tree: %w", err)
		}
		command := exec.Command(git, "--git-dir", absolute, "commit-tree", strings.TrimSpace(tree), "-m", "Initialize AgentCollab workspace")
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=AgentCollab Server", "GIT_AUTHOR_EMAIL=server@agentcollab.local",
			"GIT_COMMITTER_NAME=AgentCollab Server", "GIT_COMMITTER_EMAIL=server@agentcollab.local",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("create initial git commit: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if _, err := runGit(git, nil, "--git-dir", absolute, "update-ref", "refs/heads/main", strings.TrimSpace(string(output))); err != nil {
			return fmt.Errorf("set initial main branch: %w", err)
		}
	}
	if _, err := runGit(git, nil, "--git-dir", absolute, "config", "http.receivepack", "true"); err != nil {
		return fmt.Errorf("enable authenticated git pushes: %w", err)
	}
	if err := installProtectedMainHook(absolute); err != nil {
		return err
	}
	return nil
}

func WorkspaceGitInfo(root, workspaceID string) (GitRepositoryInfo, error) {
	relative, err := WorkspaceGitDirectory(workspaceID)
	if err != nil {
		return GitRepositoryInfo{}, err
	}
	absolute, err := ResolveWithin(root, relative)
	if err != nil {
		return GitRepositoryInfo{}, err
	}
	info := GitRepositoryInfo{DefaultBranch: "main", StoragePath: filepath.ToSlash(relative), Transport: "server_managed"}
	if _, err := os.Stat(filepath.Join(absolute, "HEAD")); err == nil {
		git, lookupErr := exec.LookPath("git")
		if lookupErr == nil {
			_, verifyErr := runGit(git, nil, "--git-dir", absolute, "rev-parse", "--verify", "refs/heads/main")
			info.Initialized = verifyErr == nil
		}
	}
	return info, nil
}

// MergeWorkspaceGitBranch creates an explicit two-parent merge commit in the
// protected Server repository. It never checks out files on the Hub machine.
func MergeWorkspaceGitBranch(root, workspaceID, branch, actorName string) (string, error) {
	if err := EnsureWorkspaceGitRepository(root, workspaceID); err != nil {
		return "", err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "main" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
		return "", ErrGitBranchNotFound
	}
	relative, _ := WorkspaceGitDirectory(workspaceID)
	repository, err := ResolveWithin(root, relative)
	if err != nil {
		return "", err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "", err
	}
	branchRef := "refs/heads/" + branch
	branchCommit, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", branchRef)
	if err != nil {
		return "", ErrGitBranchNotFound
	}
	mainCommit, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", "refs/heads/main")
	if err != nil {
		return "", err
	}
	mergeTree, err := runGit(git, nil, "--git-dir", repository, "merge-tree", "--write-tree", strings.TrimSpace(mainCommit), strings.TrimSpace(branchCommit))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrGitMergeConflict, err)
	}
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "AgentCollab Reviewer"
	}
	command := exec.Command(git, "--git-dir", repository, "commit-tree", strings.TrimSpace(mergeTree), "-p", strings.TrimSpace(mainCommit), "-p", strings.TrimSpace(branchCommit), "-m", "Merge "+branch+" after AgentCollab review")
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+actorName, "GIT_AUTHOR_EMAIL=review@agentcollab.local",
		"GIT_COMMITTER_NAME="+actorName, "GIT_COMMITTER_EMAIL=review@agentcollab.local",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("create reviewed merge commit: %w: %s", err, strings.TrimSpace(string(output)))
	}
	commit := strings.TrimSpace(string(output))
	if _, err := runGit(git, nil, "--git-dir", repository, "update-ref", "refs/heads/main", commit, strings.TrimSpace(mainCommit)); err != nil {
		return "", err
	}
	return commit, nil
}

func runGit(git string, stdin *bytes.Reader, args ...string) (string, error) {
	command := exec.Command(git, args...)
	if stdin != nil {
		command.Stdin = stdin
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func installProtectedMainHook(repository string) error {
	hook := filepath.Join(repository, "hooks", "pre-receive")
	content := []byte(`#!/bin/sh
while read old new ref; do
  if [ "$ref" = "refs/heads/main" ] && [ "$AGENTCOLLAB_SERVER_MERGE" != "1" ]; then
    echo "AgentCollab: main is protected; push a branch and request review." >&2
    exit 1
  fi
done
exit 0
`)
	if err := os.WriteFile(hook, content, 0o700); err != nil {
		return fmt.Errorf("install protected main hook: %w", err)
	}
	return nil
}
