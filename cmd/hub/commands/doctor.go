package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func RunDoctor(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config/hub.yaml", "path to Hub YAML configuration")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadHub(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	if err := storage.VerifyRoot(cfg.StorageRoot); err != nil {
		fmt.Fprintf(stderr, "doctor: storage check failed: %v\n", err)
		return 1
	}
	if err := storage.EnsureLayout(cfg.StorageRoot); err != nil {
		fmt.Fprintf(stderr, "doctor: layout check failed: %v\n", err)
		return 1
	}

	mountInfo, err := storage.DetectMount(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: mount check failed: %v\n", err)
		return 1
	}
	if cfg.RequireSMB && !mountInfo.IsSMB() {
		fmt.Fprintf(stderr, "doctor: storage root must be on SMB; detected %s at %s\n", mountInfo.Type, mountInfo.MountPoint)
		return 1
	}

	freeBytes, err := storage.FreeBytes(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: free-space check failed: %v\n", err)
		return 1
	}
	lock, err := storage.AcquireHubLock(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: lock check failed: %v\n", err)
		return 1
	}

	databasePath, err := storage.ResolveWithin(cfg.StorageRoot, cfg.DatabasePath)
	if err != nil {
		_ = lock.Close()
		fmt.Fprintf(stderr, "doctor: database path failed: %v\n", err)
		return 1
	}
	ctx := context.Background()
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		_ = lock.Close()
		fmt.Fprintf(stderr, "doctor: sqlite open failed: %v\n", err)
		return 1
	}
	if err := database.IntegrityCheck(ctx); err != nil {
		_ = database.Close()
		_ = lock.Close()
		fmt.Fprintf(stderr, "doctor: sqlite integrity failed: %v\n", err)
		return 1
	}
	schemaVersion, err := database.SchemaVersion(ctx)
	if err != nil {
		_ = database.Close()
		_ = lock.Close()
		fmt.Fprintf(stderr, "doctor: sqlite schema failed: %v\n", err)
		return 1
	}
	if err := database.Close(); err != nil {
		_ = lock.Close()
		fmt.Fprintf(stderr, "doctor: sqlite close failed: %v\n", err)
		return 1
	}
	if err := lock.Close(); err != nil {
		fmt.Fprintf(stderr, "doctor: lock release failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "storage_root=%s status=ok\n", cfg.StorageRoot)
	fmt.Fprintf(stdout, "mount_type=%s mount_point=%s source=%s\n", mountInfo.Type, mountInfo.MountPoint, mountInfo.Source)
	fmt.Fprintf(stdout, "free_bytes=%d\n", freeBytes)
	fmt.Fprintln(stdout, "lock=available")
	fmt.Fprintf(stdout, "sqlite=ok schema_version=%d\n", schemaVersion)
	fmt.Fprintln(stdout, "doctor=ok")
	return 0
}

func IsCommand(args []string, name string) bool {
	return len(args) > 0 && strings.EqualFold(args[0], name)
}
