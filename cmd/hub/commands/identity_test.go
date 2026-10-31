package commands

import (
	"agentcollab.local/agentcollab/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestHubIdentitySurvivesRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	first := config.Hub{StorageRoot: root}
	if err := ensureHubIdentity(&first); err != nil {
		t.Fatal(err)
	}
	second := config.Hub{StorageRoot: root}
	if err := ensureHubIdentity(&second); err != nil {
		t.Fatal(err)
	}
	if first.HubID == "" || first.HubID != second.HubID {
		t.Fatal("identity not persisted")
	}
	explicit := config.Hub{StorageRoot: root, HubID: "configured-team"}
	if err := ensureHubIdentity(&explicit); err != nil || explicit.HubID != "configured-team" {
		t.Fatal("explicit identity changed", err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "hub-id"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	broken := config.Hub{StorageRoot: root}
	if err := ensureHubIdentity(&broken); err == nil {
		t.Fatal("empty identity silently replaced")
	}
}
