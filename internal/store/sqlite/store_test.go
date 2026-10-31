package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenConfiguresSQLiteAndMigrates(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var journal string
	var synchronous, foreignKeys, busyTimeout int
	err = store.Read(ctx, func(db *sql.DB) error {
		if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
			return err
		}
		if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
			return err
		}
		if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			return err
		}
		return db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout)
	})
	if err != nil {
		t.Fatal(err)
	}
	if journal != "delete" || synchronous != 2 || foreignKeys != 1 || busyTimeout != 5000 {
		t.Fatalf("pragmas journal=%q synchronous=%d foreign_keys=%d busy_timeout=%d", journal, synchronous, foreignKeys, busyTimeout)
	}
	if got, err := store.SchemaVersion(ctx); err != nil || got != 27 {
		t.Fatalf("SchemaVersion() = %d, %v; want 27", got, err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agentcollab.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got, err := second.SchemaVersion(ctx); err != nil || got != 27 {
		t.Fatalf("SchemaVersion() = %d, %v; want 27", got, err)
	}
}

func TestWriteSerializesConcurrentTransactions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const writers = 24
	var wg sync.WaitGroup
	errors := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errors <- store.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, role) VALUES (?, ?, ?, ?, 'member')`,
					fmt.Sprintf("user-%02d", index), fmt.Sprintf("name-%02d", index), fmt.Sprintf("User %02d", index), "hash")
				return err
			})
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	var count int
	if err := store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != writers {
		t.Fatalf("user count = %d, want %d", count, writers)
	}
}

func TestSchemaEnforcesForeignKeysAndAttemptUniqueness(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('missing-workspace', 'missing-user', 'member')`)
		return err
	})
	if err == nil {
		t.Fatal("foreign key insert succeeded, want failure")
	}

	err = store.Write(ctx, seedTaskAttempt)
	if err != nil {
		t.Fatalf("seedTaskAttempt() error = %v", err)
	}
	err = store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO task_attempts (id, task_id, attempt_no, state) VALUES ('attempt-duplicate', 'task-1', 1, 'queued')`)
		return err
	})
	if err == nil {
		t.Fatal("duplicate task attempt succeeded, want unique constraint failure")
	}
}

func seedTaskAttempt(tx *sql.Tx) error {
	statements := []string{
		`INSERT INTO users (id, username, display_name, password_hash, role) VALUES ('user-1', 'owner', 'Owner', 'hash', 'admin')`,
		`INSERT INTO workspaces (id, name, owner_user_id) VALUES ('workspace-1', 'Workspace', 'user-1')`,
		`INSERT INTO rooms (id, workspace_id, name) VALUES ('room-1', 'workspace-1', 'General')`,
		`INSERT INTO devices (id, workspace_id, owner_user_id, name, os, arch, status) VALUES ('device-1', 'workspace-1', 'user-1', 'Mac', 'darwin', 'arm64', 'online')`,
		`INSERT INTO runners (id, device_id, token_hash, version, status) VALUES ('runner-1', 'device-1', 'hash', 'dev', 'online')`,
		`INSERT INTO agents (id, workspace_id, runner_id, owner_user_id, type, display_name, status) VALUES ('agent-1', 'workspace-1', 'runner-1', 'user-1', 'custom-cli', 'Agent', 'online')`,
		`INSERT INTO tasks (id, workspace_id, room_id, creator_kind, creator_id, target_agent_id, title, prompt, state) VALUES ('task-1', 'workspace-1', 'room-1', 'user', 'user-1', 'agent-1', 'Task', 'Prompt', 'queued')`,
		`INSERT INTO task_attempts (id, task_id, attempt_no, state) VALUES ('attempt-1', 'task-1', 1, 'queued')`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
