package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCreatesConsistentDatabase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "data", "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, role) VALUES ('user-1', 'owner', 'Owner', 'hash', 'admin')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	backupPath, err := store.Backup(ctx, filepath.Join(root, "backups"))
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file: %v", err)
	}

	backupDB, err := sql.Open("sqlite3", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	var integrity string
	if err := backupDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q", integrity)
	}
	var count int
	if err := backupDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("backup user count = %d, want 1", count)
	}
}

func TestBackupFailureDoesNotPublishPartialFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "data", "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.backupWithHook(ctx, filepath.Join(root, "backups"), func(string) error {
		return errors.New("simulated interruption")
	})
	if err == nil {
		t.Fatal("backupWithHook() error = nil, want interruption")
	}
	entries, err := os.ReadDir(filepath.Join(root, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("backup directory contains %d files after failure", len(entries))
	}
}
