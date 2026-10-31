package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"agentcollab.local/agentcollab/cmd/hub/commands"
	"agentcollab.local/agentcollab/internal/buildinfo"
)

func main() {
	if commands.IsCommand(os.Args[1:], "serve") {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		os.Exit(commands.RunServe(ctx, os.Args[2:], os.Stdout, os.Stderr))
	}
	if commands.IsCommand(os.Args[1:], "doctor") {
		os.Exit(commands.RunDoctor(os.Args[2:], os.Stdout, os.Stderr))
	}
	if commands.IsCommand(os.Args[1:], "backup") {
		os.Exit(commands.RunBackup(os.Args[2:], os.Stdout, os.Stderr))
	}
	if commands.IsCommand(os.Args[1:], "admin") {
		os.Exit(commands.RunAdmin(os.Args[2:], os.Stdout, os.Stderr))
	}
	fmt.Println(buildinfo.Current("agentcollab-hub"))
}
