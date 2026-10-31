//go:build !windows

package runnerclient

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestCancellationClosesDescendantOutputPipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & wait")
	configureCommandCancellation(command)
	var output bytes.Buffer
	command.Stdout = &output
	started := time.Now()
	if err := command.Run(); err == nil {
		t.Fatal("cancelled process reported success")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("descendant kept pipes alive: %s", elapsed)
	}
}
