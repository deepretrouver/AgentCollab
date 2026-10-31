package commands

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunServeStartsHealthEndpointAndStopsCleanly(t *testing.T) {
	root := t.TempDir()
	address := availableAddress(t)
	configPath := filepath.Join(t.TempDir(), "hub.yaml")
	configBody := fmt.Sprintf("storage_root: %s\nlisten_address: %s\nrequire_smb: false\n", root, address)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- RunServe(ctx, []string{"--config", configPath}, &stdout, &stderr)
	}()

	waitForHealthy(t, "http://"+address+"/healthz")
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("RunServe() code=%d stderr=%q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServe() did not stop after cancellation")
	}
	if !strings.Contains(stdout.String(), "hub_listening=") || !strings.Contains(stdout.String(), "hub_stopped=ok") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "data", "hub.lock")); !os.IsNotExist(err) {
		t.Fatalf("hub lock remains after shutdown: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "logs", "hub.jsonl")); err != nil {
		t.Fatalf("hub log missing: %v", err)
	}
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("health endpoint %s did not become ready", url)
}
