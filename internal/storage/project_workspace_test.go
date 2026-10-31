package storage

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCommitStepWorkspaceToMainAndTaskBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	makeUpload := func(path, body string) WorkspaceUpload {
		return WorkspaceUpload{RelativePath: path, Size: int64(len(body)), Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }}
	}
	main, err := CommitStepWorkspace(root, "ws-one", "step-one", "main", "Owner", "Import step workspace", []WorkspaceUpload{makeUpload("report/result.md", "main\n")})
	if err != nil || main.FileCount != 1 || main.Commit == "" {
		t.Fatalf("main=%+v err=%v", main, err)
	}
	branch, err := CommitStepWorkspace(root, "ws-one", "step-one", "issue/topic", "Member", "Submit task", []WorkspaceUpload{makeUpload("report/result.md", "branch\n")})
	if err != nil || branch.Commit == "" || branch.Commit == main.Commit {
		t.Fatalf("branch=%+v err=%v", branch, err)
	}
	relative, _ := WorkspaceGitDirectory("ws-one")
	if output, err := exec.Command("git", "--git-dir", root+"/"+relative, "show", "issue/topic:steps/step-one/report/result.md").CombinedOutput(); err != nil || string(output) != "branch\n" {
		t.Fatalf("branch file=%q err=%v", output, err)
	}
}

func TestCommitStepWorkspaceIgnoresLocalDependencyAndGitMetadata(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	makeUpload := func(path, body string) WorkspaceUpload {
		return WorkspaceUpload{RelativePath: path, Size: int64(len(body)), Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }}
	}
	snapshot, err := CommitStepWorkspace(root, "ws-ignore", "step-one", "main", "Owner", "Import", []WorkspaceUpload{
		makeUpload("src/main.go", "package main\n"),
		makeUpload(".git/config", "local metadata"),
		makeUpload("node_modules/pkg/index.js", "dependency"),
		makeUpload(".venv/bin/python", "dependency"),
		makeUpload("src/__pycache__/module.pyc", "cache"),
		makeUpload(".DS_Store", "finder"),
	})
	if err != nil || snapshot.FileCount != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
}

func TestCreateProjectStepArchiveExportsOnlyRequestedImmutableTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	makeUpload := func(path, body string) WorkspaceUpload {
		return WorkspaceUpload{RelativePath: path, Size: int64(len(body)), Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }}
	}
	snapshot, err := CommitStepWorkspace(root, "ws-archive", "step-one", "main", "Owner", "Import", []WorkspaceUpload{makeUpload("src/main.py", "print('one')\n")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitStepWorkspace(root, "ws-archive", "step-two", "main", "Owner", "Import", []WorkspaceUpload{makeUpload("secret.txt", "other step\n")}); err != nil {
		t.Fatal(err)
	}
	archive, err := CreateProjectStepArchive(root, "ws-archive", "step-one", snapshot.Commit)
	if err != nil {
		t.Fatal(err)
	}
	name := archive.Name()
	defer archive.Close()
	reader, err := zip.NewReader(archive.File, archive.Size)
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() {
			files = append(files, file.Name)
		}
	}
	if len(files) != 1 || files[0] != "project/src/main.py" {
		t.Fatalf("archive files=%v", files)
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("temporary archive missing before close: %v", err)
	}
}
