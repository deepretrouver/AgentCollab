package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBackupPrintsCreatedBackupPath(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "hub.yaml")
	configBody := fmt.Sprintf("storage_root: %s\nrequire_smb: false\n", root)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunBackup([]string{"--config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunBackup() code = %d, stderr = %q", code, stderr.String())
	}
	backupPath := strings.TrimSpace(stdout.String())
	if filepath.Dir(backupPath) != filepath.Join(root, "backups") {
		t.Fatalf("backup path = %q", backupPath)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file: %v", err)
	}
}
