package runnerclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agentcollab.local/agentcollab/internal/agentcatalog"
	"agentcollab.local/agentcollab/internal/config"
)

func TestClientRefreshesHubCatalogAndRediscoversWithoutReinstall(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "team-agent")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	remote := agentcatalog.Catalog{SchemaVersion: agentcatalog.SchemaVersion, Revision: "team-2026-08-05", Adapters: []agentcatalog.Adapter{{
		Type: "team-agent", DisplayName: "Team Agent", Transport: "native", CatalogTier: "native",
		Launchers: []agentcatalog.Launcher{{CommandPaths: []string{executable}}}, OutputMode: "text",
	}}}
	payload, err := agentcatalog.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"team-catalog"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	base, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	client := &Client{
		config: config.Runner{HubURL: server.URL, LocalDir: root}, metadata: make(map[string]discoveredAgent),
		http: server.Client(), stdout: stdout, stderr: &bytes.Buffer{}, catalogRevision: base.Revision,
	}
	if err := client.refreshAdapterCatalog(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if client.catalogRevision != remote.Revision || len(client.agents) != 1 || client.agents[0].Type != "team-agent" {
		t.Fatalf("Client did not adopt remote catalog: revision=%q agents=%+v", client.catalogRevision, client.agents)
	}
	if _, err := os.Stat(filepath.Join(root, "adapter-catalog.json")); err != nil {
		t.Fatalf("remote catalog was not cached: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("agent_catalog_updated=team-2026-08-05")) {
		t.Fatalf("catalog refresh was not observable: %s", stdout.String())
	}
}

func TestInvalidHubCatalogDoesNotReplaceLastValidCache(t *testing.T) {
	root := t.TempDir()
	base, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "adapter-catalog.json")
	if err := saveAgentCatalog(cachePath, base); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":99,"revision":"future","adapters":[]}`))
	}))
	defer server.Close()
	if _, _, err := syncAgentCatalog(context.Background(), server.URL, root, server.Client(), ""); err == nil {
		t.Fatal("future-schema catalog was accepted")
	}
	retained, err := agentcatalog.Load(cachePath)
	if err != nil || retained.Revision != base.Revision {
		t.Fatalf("last valid catalog was replaced: revision=%q err=%v", retained.Revision, err)
	}
}

func TestClientAppliesChangedCatalogContentWhenRevisionWasNotBumped(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "team-agent")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	base, err := agentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	remote := agentcatalog.Catalog{SchemaVersion: agentcatalog.SchemaVersion, Revision: base.Revision, Adapters: []agentcatalog.Adapter{{
		Type: "team-agent", DisplayName: "Team Agent", Transport: "native", CatalogTier: "native",
		Launchers: []agentcatalog.Launcher{{CommandPaths: []string{executable}}}, Args: []string{"--new-compatible-flag", "{{prompt}}"}, OutputMode: "text",
	}}}
	payload, err := agentcatalog.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"changed-content"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	stdout := &bytes.Buffer{}
	client := &Client{
		config: config.Runner{HubURL: server.URL, LocalDir: root}, metadata: make(map[string]discoveredAgent),
		http: server.Client(), stdout: stdout, stderr: &bytes.Buffer{}, catalogRevision: base.Revision, catalogETag: `"old-content"`,
	}
	if err := client.refreshAdapterCatalog(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(client.agents) != 1 || client.agents[0].Type != "team-agent" {
		t.Fatalf("same-revision changed content was not applied: %+v", client.agents)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("agent_catalog_updated="+base.Revision)) {
		t.Fatalf("same-revision refresh was not observable: %s", stdout.String())
	}
}
