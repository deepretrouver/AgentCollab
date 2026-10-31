package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) Backup(ctx context.Context, backupDir string) (string, error) {
	return s.backupWithHook(ctx, backupDir, nil)
}

func (s *Store) backupWithHook(ctx context.Context, backupDir string, afterVacuum func(string) error) (string, error) {
	if !filepath.IsAbs(backupDir) {
		return "", fmt.Errorf("backup directory must be absolute")
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	finalPath := filepath.Join(backupDir, "agentcollab-"+stamp+".sqlite")
	temporaryPath := filepath.Join(backupDir, ".agentcollab-"+stamp+".partial")
	published := false
	defer func() {
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()

	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	quotedPath := strings.ReplaceAll(temporaryPath, "'", "''")
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO '"+quotedPath+"'"); err != nil {
		return "", fmt.Errorf("create sqlite backup: %w", err)
	}
	if afterVacuum != nil {
		if err := afterVacuum(temporaryPath); err != nil {
			return "", fmt.Errorf("backup interrupted after vacuum: %w", err)
		}
	}
	if err := checkDatabaseIntegrity(ctx, temporaryPath); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return "", fmt.Errorf("publish sqlite backup: %w", err)
	}
	published = true
	return finalPath, nil
}

func checkDatabaseIntegrity(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("open backup for integrity check: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("check backup integrity: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup integrity_check returned %q", result)
	}
	return nil
}
