package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/config"
)

func TestDefaultRunnerConfigPath(t *testing.T) {
	path := defaultRunnerConfigPath()
	if filepath.Base(path) != "runner.yaml" {
		t.Fatalf("defaultRunnerConfigPath() = %q, want runner.yaml basename", path)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("defaultRunnerConfigPath() = %q, want absolute path", path)
	}
}

func TestPromptSetupValueUsesInputAndFallback(t *testing.T) {
	value, err := promptSetupValue(bufio.NewReader(strings.NewReader("  http://hub.example:8080  \n")), "Hub", "")
	if err != nil || value != "http://hub.example:8080" {
		t.Fatalf("promptSetupValue input = %q, %v", value, err)
	}
	value, err = promptSetupValue(bufio.NewReader(strings.NewReader("\n")), "Hub", "http://fallback:8080")
	if err != nil || value != "http://fallback:8080" {
		t.Fatalf("promptSetupValue fallback = %q, %v", value, err)
	}
}

func TestPrepareRunnerConfigCreatesPairingConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(t.TempDir(), "runner.yaml")
	cfg, err := prepareRunnerConfig(configPath, "https://hub.example")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HubURL != "https://hub.example" || cfg.CredentialsFile != filepath.Join(filepath.Dir(configPath), "runner", "device.json") {
		t.Fatalf("runner config=%#v", cfg)
	}
	if len(cfg.Resources) != 1 || cfg.Resources[0].Path != home {
		t.Fatalf("runner resources=%#v", cfg.Resources)
	}
	loaded, err := config.LoadRunner(configPath)
	if err != nil || loaded.HubURL != cfg.HubURL {
		t.Fatalf("saved config=%#v, err=%v", loaded, err)
	}
}

func TestNewRunnerConfigurationsDoNotShareState(t *testing.T) {
	root := t.TempDir()
	first, err := prepareRunnerConfig(filepath.Join(root, "v1", "runner.yaml"), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareRunnerConfig(filepath.Join(root, "v2", "runner.yaml"), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if first.LocalDir == second.LocalDir || first.CredentialsFile == second.CredentialsFile {
		t.Fatal("independent configurations share runtime state or device credentials")
	}
	// Reconnecting must preserve an already paired installation's paths.
	first.LocalDir = filepath.Join(root, "existing-runtime")
	first.CredentialsFile = filepath.Join(first.LocalDir, "existing-device.json")
	configPath := filepath.Join(root, "v1", "runner.yaml")
	if err := config.SaveRunner(configPath, first); err != nil {
		t.Fatal(err)
	}
	reloaded, err := prepareRunnerConfig(configPath, "http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.LocalDir != first.LocalDir || reloaded.CredentialsFile != first.CredentialsFile {
		t.Fatal("reconnection changed existing registration paths")
	}
}

func TestHasSavedRunnerRegistrationOnlyAfterOneTimePairingArtifactsExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".agentcollab", "runner.yaml")
	cfg, err := prepareRunnerConfig(configPath, "http://office-hub.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	if hasSavedRunnerRegistration(configPath) {
		t.Fatal("configuration without the one-time enrollment result must reopen pairing")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CredentialsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.CredentialsFile, []byte(`{"token":"saved-device-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasSavedRunnerRegistration(configPath) {
		t.Fatal("saved configuration and device token must restore the Client without pairing again")
	}
}
