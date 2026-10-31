package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHubPortableStorageRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.yaml")
	if err := os.WriteFile(path, []byte("storage_root: ./team-data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadHub(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorageRoot != filepath.Join(dir, "team-data") {
		t.Fatalf("unexpected root: %s", cfg.StorageRoot)
	}
}

func TestLoadHubRejectsUnknownPortableFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.yaml")
	if err := os.WriteFile(path, []byte("storage_root: ./data\nunknown_field: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHub(path); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}
