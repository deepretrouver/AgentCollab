package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrEmptyWorkspace       = errors.New("workspace directory has no files")
	ErrIgnoredWorkspacePath = errors.New("ignored workspace path")
)

type WorkspaceUpload struct {
	RelativePath string
	Open         func() (io.ReadCloser, error)
	Size         int64
}

type WorkspaceSnapshot struct {
	Commit     string
	FileCount  int
	TotalBytes int64
}

// InspectProjectStepSnapshot returns the exact file count and byte total for a
// project-step tree at one immutable commit. Runner downloads use these values
// to reject partial or mismatched review/verification archives.
func InspectProjectStepSnapshot(root, workspaceID, stepID, commit string) (WorkspaceSnapshot, error) {
	if err := EnsureWorkspaceGitRepository(root, workspaceID); err != nil {
		return WorkspaceSnapshot{}, err
	}
	if _, err := projectSegment(stepID); err != nil {
		return WorkspaceSnapshot{}, err
	}
	commit = strings.TrimSpace(commit)
	if commit == "" || strings.HasPrefix(commit, "-") || strings.ContainsAny(commit, "\x00\r\n:") {
		return WorkspaceSnapshot{}, ErrGitBranchNotFound
	}
	relative, _ := WorkspaceGitDirectory(workspaceID)
	repository, err := ResolveWithin(root, relative)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	verified, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return WorkspaceSnapshot{}, ErrGitBranchNotFound
	}
	verified = strings.TrimSpace(verified)
	treeish := verified + ":" + filepath.ToSlash(filepath.Join("steps", stepID))
	output, err := runGit(git, nil, "--git-dir", repository, "ls-tree", "-r", "-l", treeish)
	if err != nil {
		return WorkspaceSnapshot{}, ErrGitBranchNotFound
	}
	snapshot := WorkspaceSnapshot{Commit: verified}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != "blob" {
			continue
		}
		size, parseErr := strconv.ParseInt(fields[3], 10, 64)
		if parseErr != nil {
			return WorkspaceSnapshot{}, parseErr
		}
		snapshot.FileCount++
		snapshot.TotalBytes += size
	}
	if snapshot.FileCount == 0 {
		return WorkspaceSnapshot{}, ErrEmptyWorkspace
	}
	return snapshot, nil
}

// CommitStepWorkspace replaces one step directory from a browser/desktop
// directory selection and commits it to main or to a task branch. Git remains
// a Server implementation detail for the default product flow.
func CommitStepWorkspace(root, workspaceID, stepID, branch, actorName, message string, uploads []WorkspaceUpload) (WorkspaceSnapshot, error) {
	if len(uploads) == 0 {
		return WorkspaceSnapshot{}, ErrEmptyWorkspace
	}
	if err := EnsureWorkspaceGitRepository(root, workspaceID); err != nil {
		return WorkspaceSnapshot{}, err
	}
	if _, err := projectSegment(stepID); err != nil {
		return WorkspaceSnapshot{}, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	if branch != "main" && (strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") || strings.Contains(branch, "..")) {
		return WorkspaceSnapshot{}, ErrGitBranchNotFound
	}
	relative, _ := WorkspaceGitDirectory(workspaceID)
	repository, err := ResolveWithin(root, relative)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	checkout, err := os.MkdirTemp("", "agentcollab-workspace-")
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer os.RemoveAll(checkout)
	if _, err := runGit(git, nil, "clone", "--quiet", repository, checkout); err != nil {
		return WorkspaceSnapshot{}, err
	}
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "AgentCollab Contributor"
	}
	for _, args := range [][]string{{"-C", checkout, "config", "user.name", actorName}, {"-C", checkout, "config", "user.email", "contributor@agentcollab.local"}} {
		if _, err := runGit(git, nil, args...); err != nil {
			return WorkspaceSnapshot{}, err
		}
	}
	if branch != "main" {
		if _, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", "refs/heads/"+branch); err == nil {
			if _, err := runGit(git, nil, "-C", checkout, "switch", "--quiet", "-c", branch, "origin/"+branch); err != nil {
				return WorkspaceSnapshot{}, err
			}
		} else if _, err := runGit(git, nil, "-C", checkout, "switch", "--quiet", "-c", branch, "main"); err != nil {
			return WorkspaceSnapshot{}, err
		}
	}
	stepDir := filepath.Join(checkout, "steps", stepID)
	if err := os.RemoveAll(stepDir); err != nil {
		return WorkspaceSnapshot{}, err
	}
	if err := os.MkdirAll(stepDir, 0o700); err != nil {
		return WorkspaceSnapshot{}, err
	}
	snapshot := WorkspaceSnapshot{}
	seen := map[string]struct{}{}
	for _, upload := range uploads {
		rel, err := cleanUploadPath(upload.RelativePath)
		if errors.Is(err, ErrIgnoredWorkspacePath) {
			continue
		}
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		if _, duplicate := seen[rel]; duplicate {
			return WorkspaceSnapshot{}, fmt.Errorf("duplicate workspace path %q", rel)
		}
		seen[rel] = struct{}{}
		target, err := ResolveWithin(stepDir, rel)
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return WorkspaceSnapshot{}, err
		}
		source, err := upload.Open()
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		destination, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			source.Close()
			return WorkspaceSnapshot{}, err
		}
		written, copyErr := io.Copy(destination, source)
		closeErr := destination.Close()
		sourceErr := source.Close()
		if copyErr != nil {
			return WorkspaceSnapshot{}, copyErr
		}
		if closeErr != nil {
			return WorkspaceSnapshot{}, closeErr
		}
		if sourceErr != nil {
			return WorkspaceSnapshot{}, sourceErr
		}
		snapshot.FileCount++
		snapshot.TotalBytes += written
	}
	if snapshot.FileCount == 0 {
		return WorkspaceSnapshot{}, ErrEmptyWorkspace
	}
	if _, err := runGit(git, nil, "-C", checkout, "add", "-A", "--", filepath.ToSlash(filepath.Join("steps", stepID))); err != nil {
		return WorkspaceSnapshot{}, err
	}
	diff := exec.Command(git, "-C", checkout, "diff", "--cached", "--quiet")
	if err := diff.Run(); err != nil {
		if message = strings.TrimSpace(message); message == "" {
			message = "Update project step workspace"
		}
		if _, err := runGit(git, nil, "-C", checkout, "commit", "--quiet", "-m", message); err != nil {
			return WorkspaceSnapshot{}, err
		}
	} else if branch != "main" {
		if message = strings.TrimSpace(message); message == "" {
			message = "Submit project task for review"
		}
		if _, err := runGit(git, nil, "-C", checkout, "commit", "--quiet", "--allow-empty", "-m", message); err != nil {
			return WorkspaceSnapshot{}, err
		}
	}
	commit, err := runGit(git, nil, "-C", checkout, "rev-parse", "HEAD")
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	commit = strings.TrimSpace(commit)
	push := exec.Command(git, "-C", checkout, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch)
	push.Env = append(os.Environ(), "AGENTCOLLAB_SERVER_MERGE=1")
	if output, err := push.CombinedOutput(); err != nil {
		return WorkspaceSnapshot{}, fmt.Errorf("push workspace snapshot: %w: %s", err, strings.TrimSpace(string(output)))
	}
	snapshot.Commit = commit
	return snapshot, nil
}

func cleanUploadPath(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid workspace path")
	}
	for _, segment := range strings.Split(filepath.ToSlash(cleaned), "/") {
		switch segment {
		case ".git", ".agentcollab", "node_modules", ".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".DS_Store":
			return "", ErrIgnoredWorkspacePath
		}
	}
	return cleaned, nil
}

// ProjectStepArchive materializes one immutable step tree as a temporary ZIP.
// The caller must Close it; Close also removes the temporary file.
type ProjectStepArchive struct {
	*os.File
	Size int64
}

func (archive *ProjectStepArchive) Close() error {
	if archive == nil || archive.File == nil {
		return nil
	}
	name := archive.File.Name()
	err := archive.File.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return err
	}
	return removeErr
}

// CreateProjectStepArchive exports only steps/<stepID> at the requested commit
// and rewrites that subtree to project/ inside the ZIP. A Runner therefore
// receives neither other project steps nor Server Git metadata.
func CreateProjectStepArchive(root, workspaceID, stepID, commit string) (*ProjectStepArchive, error) {
	if err := EnsureWorkspaceGitRepository(root, workspaceID); err != nil {
		return nil, err
	}
	if _, err := projectSegment(stepID); err != nil {
		return nil, err
	}
	commit = strings.TrimSpace(commit)
	if commit == "" || strings.HasPrefix(commit, "-") || strings.ContainsAny(commit, "\x00\r\n:") {
		return nil, ErrGitBranchNotFound
	}
	relative, _ := WorkspaceGitDirectory(workspaceID)
	repository, err := ResolveWithin(root, relative)
	if err != nil {
		return nil, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	verified, err := runGit(git, nil, "--git-dir", repository, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil || strings.TrimSpace(verified) != commit {
		return nil, ErrGitBranchNotFound
	}
	treeish := commit + ":" + filepath.ToSlash(filepath.Join("steps", stepID))
	if _, err := runGit(git, nil, "--git-dir", repository, "cat-file", "-e", treeish); err != nil {
		return nil, ErrGitBranchNotFound
	}
	temporary, err := os.CreateTemp("", "agentcollab-project-*.zip")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()
	command := exec.Command(git, "--git-dir", repository, "archive", "--format=zip", "--prefix=project/", treeish)
	command.Stdout = temporary
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("archive project step: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	info, err := temporary.Stat()
	if err != nil {
		return nil, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	cleanup = false
	return &ProjectStepArchive{File: temporary, Size: info.Size()}, nil
}
