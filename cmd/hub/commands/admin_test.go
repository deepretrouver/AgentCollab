package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestRunAdminCreate(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "hub.yaml")
	configBody := fmt.Sprintf("storage_root: %s\nrequire_smb: false\n", root)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTCOLLAB_ADMIN_PASSWORD", "password-1234")

	var stdout, stderr bytes.Buffer
	code := RunAdmin([]string{"create", "--config", configPath, "--username", "owner", "--display-name", "Owner"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunAdmin() code = %d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "admin_created=") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	database, err := store.Open(context.Background(), filepath.Join(root, "data", "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := auth.NewService(store.NewAuthRepository(database))
	if _, err := service.Login(context.Background(), "owner", "password-1234"); err != nil {
		t.Fatalf("created admin login error = %v", err)
	}
}
