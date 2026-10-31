package storage

import (
	"os"
	"testing"
)

func TestSMBIntegrationSingleHubLock(t *testing.T) {
	root := os.Getenv("AGENTCOLLAB_TEST_STORAGE_ROOT")
	if root == "" {
		t.Skip("AGENTCOLLAB_TEST_STORAGE_ROOT is not set")
	}
	if err := VerifyRoot(root); err != nil {
		t.Fatalf("VerifyRoot() error = %v", err)
	}
	mount, err := DetectMount(root)
	if err != nil {
		t.Fatalf("DetectMount() error = %v", err)
	}
	if !mount.IsSMB() {
		t.Fatalf("mount type = %q, want SMB", mount.Type)
	}
	if err := EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	first, err := AcquireHubLock(root)
	if err != nil {
		t.Fatalf("first AcquireHubLock() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if _, err := AcquireHubLock(root); err == nil {
		t.Fatal("second AcquireHubLock() error = nil, want SMB lock contention")
	}
}
