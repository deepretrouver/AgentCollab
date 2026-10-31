package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeHubAppliesDefaults(t *testing.T) {
	got, err := DecodeHub(strings.NewReader("storage_root: /Volumes/share/AgentCollab\n"))
	if err != nil {
		t.Fatalf("DecodeHub() error = %v", err)
	}
	if got.StorageRoot != "/Volumes/share/AgentCollab" {
		t.Fatalf("StorageRoot = %q", got.StorageRoot)
	}
	if got.ListenAddress != "127.0.0.1:8080" {
		t.Fatalf("ListenAddress = %q, want default", got.ListenAddress)
	}
	if got.DatabasePath != "data/agentcollab.db" {
		t.Fatalf("DatabasePath = %q, want default", got.DatabasePath)
	}
	if got.HubName != "AgentCollab" {
		t.Fatalf("HubName = %q, want default", got.HubName)
	}
}

func TestDecodeHubExposesCanonicalURLBeforeAliases(t *testing.T) {
	got, err := DecodeHub(strings.NewReader("storage_root: /tmp/AgentCollab\nhub_id: hub-research\nhub_name: Research Hub\npublic_url: http://agentcollab.office:8080\nrunner_public_urls: [http://192.168.1.20:8080, http://agentcollab.office:8080]\n"))
	if err != nil {
		t.Fatalf("DecodeHub() error = %v", err)
	}
	urls := got.PublicURLs()
	if got.HubID != "hub-research" || got.HubName != "Research Hub" || len(urls) != 2 || urls[0] != "http://agentcollab.office:8080" || urls[1] != "http://192.168.1.20:8080" {
		t.Fatalf("Hub metadata = %#v URLs=%#v", got, urls)
	}
}

func TestDecodeHubRejectsPublicURLQuery(t *testing.T) {
	_, err := DecodeHub(strings.NewReader("storage_root: /tmp/AgentCollab\npublic_url: https://agents.example.com/?secret=1\n"))
	if err == nil || !strings.Contains(err.Error(), "public_url") {
		t.Fatalf("DecodeHub() error = %v, want public_url rejection", err)
	}
}

func TestDecodeHubRejectsUnknownField(t *testing.T) {
	_, err := DecodeHub(strings.NewReader("storage_root: /tmp/AgentCollab\nmystery: true\n"))
	if err == nil || !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("DecodeHub() error = %v, want unknown field error", err)
	}
}

func TestDecodeHubAcceptsRunnerPublicURLs(t *testing.T) {
	got, err := DecodeHub(strings.NewReader("storage_root: /tmp/AgentCollab\nrunner_public_urls: [http://192.168.1.20:8080, https://agents.example.com]\n"))
	if err != nil {
		t.Fatalf("DecodeHub() error = %v", err)
	}
	if len(got.RunnerPublicURLs) != 2 || got.RunnerPublicURLs[0] != "http://192.168.1.20:8080" {
		t.Fatalf("RunnerPublicURLs = %#v", got.RunnerPublicURLs)
	}
}

func TestDecodeHubRejectsRunnerURLCredentials(t *testing.T) {
	_, err := DecodeHub(strings.NewReader("storage_root: /tmp/AgentCollab\nrunner_public_urls: [https://user:secret@agents.example.com]\n"))
	if err == nil || !strings.Contains(err.Error(), "runner_public_urls") {
		t.Fatalf("DecodeHub() error = %v, want URL credentials rejection", err)
	}
}

func TestDecodeRunnerRejectsStorageRoot(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
storage_root: /Volumes/share/AgentCollab
`
	_, err := DecodeRunner(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "storage_root") {
		t.Fatalf("DecodeRunner() error = %v, want storage_root rejection", err)
	}
}

func TestDecodeRunnerRequiresLocalFields(t *testing.T) {
	_, err := DecodeRunner(strings.NewReader("hub_url: http://127.0.0.1:8080\n"))
	if err == nil {
		t.Fatal("DecodeRunner() error = nil, want missing local fields error")
	}
}

func TestDecodeRunnerAcceptsLocalSkillsAndResources(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
resources:
  - name: research-db
    path: /tmp/research.db
    access: read_write
agents:
  - id: local-hermes
    display_name: Hermes
    type: hermes
    command: /usr/bin/true
    skill_roots: [/tmp/hermes-skills]
`
	got, err := DecodeRunner(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DecodeRunner() error = %v", err)
	}
	if len(got.Resources) != 1 || got.Resources[0].Name != "research-db" || len(got.Agents[0].SkillRoots) != 1 {
		t.Fatalf("local capabilities = %#v / %#v", got.Resources, got.Agents[0].SkillRoots)
	}
}

func TestDecodeRunnerRejectsRelativeCapabilityPaths(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
resources:
  - name: research-db
    path: data/research.db
`
	_, err := DecodeRunner(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "resources[0].path") {
		t.Fatalf("DecodeRunner() error = %v, want relative resource rejection", err)
	}
}

func TestDecodeRunnerAcceptsCustomACPAgent(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
agents:
  - id: research-acp
    display_name: Research ACP
    type: research-agent
    transport: acp
    catalog_tier: custom
    command: /usr/bin/true
    acp_permission_policy: allow_once
    acp_mode: code
`
	got, err := DecodeRunner(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DecodeRunner() error = %v", err)
	}
	if got.Agents[0].Transport != "acp" || got.Agents[0].CatalogTier != "custom" || got.Agents[0].ACPPermission != "allow_once" {
		t.Fatalf("ACP Agent = %#v", got.Agents[0])
	}
}

func TestDecodeRunnerRejectsACPSettingsOnNativeAgent(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
agents:
  - id: native-agent
    display_name: Native
    type: native-agent
    command: /usr/bin/true
    acp_permission_policy: auto_allow
`
	_, err := DecodeRunner(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "ACP settings require transport") {
		t.Fatalf("DecodeRunner() error = %v, want native ACP settings rejection", err)
	}
}

func TestDecodeRunnerRejectsNativeCatalogTierForACP(t *testing.T) {
	input := `
hub_url: http://127.0.0.1:8080
local_dir: /tmp/runner
credentials_file: /tmp/runner/device.token
agents:
  - id: invalid-acp
    display_name: Invalid ACP
    type: invalid-acp
    transport: acp
    catalog_tier: native
    command: /usr/bin/true
`
	_, err := DecodeRunner(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "preset or custom") {
		t.Fatalf("DecodeRunner() error = %v, want ACP catalog tier rejection", err)
	}
}

func TestSaveRunnerWritesPrivatePortableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agentcollab", "runner.yaml")
	cfg := Runner{
		HubURL: "http://192.168.1.20:8080", LocalDir: filepath.Join(t.TempDir(), "runner"), CredentialsFile: filepath.Join(t.TempDir(), "device.json"),
		Resources: []RunnerResource{{Name: "user-home", Path: t.TempDir(), Access: "read_write"}},
	}
	if err := SaveRunner(path, cfg); err != nil {
		t.Fatalf("SaveRunner() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("runner config mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := LoadRunner(path)
	if err != nil || loaded.HubURL != cfg.HubURL || len(loaded.Resources) != 1 {
		t.Fatalf("LoadRunner() = %#v, %v", loaded, err)
	}
}
