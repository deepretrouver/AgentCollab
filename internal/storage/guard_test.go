package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLayoutCreatesRequiredDirectories(t *testing.T) {
	root := t.TempDir()
	if err := EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	for _, name := range RequiredDirectories {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("directory %q: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", name)
		}
	}
}

func TestVerifyRootRejectsRelativePath(t *testing.T) {
	if err := VerifyRoot("relative/path"); err == nil {
		t.Fatal("VerifyRoot() error = nil, want relative path rejection")
	}
}

func TestVerifyRootRejectsSymlink(t *testing.T) {
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRoot(linkRoot); err == nil {
		t.Fatal("VerifyRoot() error = nil, want symlink rejection")
	}
}

func TestVerifyRootRejectsReadOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if err := VerifyRoot(root); err == nil {
		t.Fatal("VerifyRoot() error = nil, want read-only rejection")
	}
}

func TestResolveWithinRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveWithin(root, "../outside"); err == nil {
		t.Fatal("ResolveWithin() error = nil, want escape rejection")
	}
	got, err := ResolveWithin(root, "data/agentcollab.db")
	if err != nil {
		t.Fatalf("ResolveWithin() error = %v", err)
	}
	want := filepath.Join(root, "data", "agentcollab.db")
	if got != want {
		t.Fatalf("ResolveWithin() = %q, want %q", got, want)
	}
}

func TestAcquireHubLockRejectsSecondHolder(t *testing.T) {
	root := t.TempDir()
	if err := EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireHubLock(root)
	if err != nil {
		t.Fatalf("first AcquireHubLock() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	if _, err := AcquireHubLock(root); err == nil {
		t.Fatal("second AcquireHubLock() error = nil, want lock contention")
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first.Close() error = %v", err)
	}
	second, err := AcquireHubLock(root)
	if err != nil {
		t.Fatalf("AcquireHubLock() after release error = %v", err)
	}
	_ = second.Close()
}

func TestParseMountOutputChoosesLongestContainingMount(t *testing.T) {
	output := strings.Join([]string{
		"/dev/disk3s1 on / (apfs, local)",
		"//user@server/share on /Volumes/share (smbfs, nodev, nosuid)",
	}, "\n")

	got, err := ParseMountOutput(output, "/Volumes/share/AgentCollab")
	if err != nil {
		t.Fatalf("ParseMountOutput() error = %v", err)
	}
	if got.MountPoint != "/Volumes/share" || got.Type != "smbfs" {
		t.Fatalf("ParseMountOutput() = %#v", got)
	}
}
