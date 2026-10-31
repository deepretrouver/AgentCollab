package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func RunBackup(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config/hub.yaml", "path to Hub YAML configuration")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadHub(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "backup: %v\n", err)
		return 1
	}
	if err := storage.EnsureLayout(cfg.StorageRoot); err != nil {
		fmt.Fprintf(stderr, "backup: storage check failed: %v\n", err)
		return 1
	}
	lock, err := storage.AcquireHubLock(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "backup: lock check failed: %v\n", err)
		return 1
	}
	defer lock.Close()

	databasePath, err := storage.ResolveWithin(cfg.StorageRoot, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(stderr, "backup: database path: %v\n", err)
		return 1
	}
	backupDir, err := storage.ResolveWithin(cfg.StorageRoot, "backups")
	if err != nil {
		fmt.Fprintf(stderr, "backup: backup path: %v\n", err)
		return 1
	}
	ctx := context.Background()
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "backup: open database: %v\n", err)
		return 1
	}
	defer database.Close()
	backupPath, err := database.Backup(ctx, backupDir)
	if err != nil {
		fmt.Fprintf(stderr, "backup: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, backupPath)
	return 0
}
