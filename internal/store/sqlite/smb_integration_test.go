package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSMBIntegrationWriteAndBackup(t *testing.T) {
	root := os.Getenv("AGENTCOLLAB_TEST_STORAGE_ROOT")
	if root == "" {
		t.Skip("AGENTCOLLAB_TEST_STORAGE_ROOT is not set")
	}
	testRoot, err := os.MkdirTemp(filepath.Join(root, "data"), ".sqlite-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(testRoot) })

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(testRoot, "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, role) VALUES ('smb-user', 'smb-owner', 'SMB Owner', 'hash', 'admin')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	backupPath, err := store.Backup(ctx, filepath.Join(testRoot, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatal(err)
	}
}
