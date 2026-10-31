package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctorAcceptsWritableStorage(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "hub.yaml")
	configBody := fmt.Sprintf("storage_root: %s\nrequire_smb: false\n", root)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunDoctor([]string{"--config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunDoctor() code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "doctor=ok") {
		t.Fatalf("stdout = %q, want doctor=ok", stdout.String())
	}
	if !strings.Contains(stdout.String(), "sqlite=ok schema_version=27") {
		t.Fatalf("stdout = %q, want sqlite schema check", stdout.String())
	}
}

func TestRunDoctorRequiresSMBWhenConfigured(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "hub.yaml")
	configBody := fmt.Sprintf("storage_root: %s\nrequire_smb: true\n", root)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunDoctor([]string{"--config", configPath}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "SMB") {
		t.Fatalf("RunDoctor() code = %d, stderr = %q, want SMB failure", code, stderr.String())
	}
}
