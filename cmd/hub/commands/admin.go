package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func RunAdmin(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(stderr, "admin: expected 'create' subcommand")
		return 2
	}
	flags := flag.NewFlagSet("admin create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config/hub.yaml", "path to Hub YAML configuration")
	username := flags.String("username", "", "administrator username")
	displayName := flags.String("display-name", "", "administrator display name")
	passwordEnv := flags.String("password-env", "AGENTCOLLAB_ADMIN_PASSWORD", "environment variable containing the password")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	password := os.Getenv(*passwordEnv)
	if password == "" {
		fmt.Fprintf(stderr, "admin: password environment variable %s is empty\n", *passwordEnv)
		return 2
	}

	cfg, err := config.LoadHub(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "admin: %v\n", err)
		return 1
	}
	if err := storage.EnsureLayout(cfg.StorageRoot); err != nil {
		fmt.Fprintf(stderr, "admin: storage check failed: %v\n", err)
		return 1
	}
	lock, err := storage.AcquireHubLock(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "admin: lock check failed: %v\n", err)
		return 1
	}
	defer lock.Close()
	databasePath, err := storage.ResolveWithin(cfg.StorageRoot, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(stderr, "admin: database path: %v\n", err)
		return 1
	}
	database, err := store.Open(context.Background(), databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "admin: open database: %v\n", err)
		return 1
	}
	defer database.Close()
	service := auth.NewService(store.NewAuthRepository(database))
	user, err := service.CreateUser(context.Background(), *username, *displayName, password, domain.UserAdmin)
	if err != nil {
		fmt.Fprintf(stderr, "admin: create user: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "admin_created=%s username=%s\n", user.ID, user.Username)
	return 0
}
