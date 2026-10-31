package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCatalogHandlerPublishesDefaultAndSupportsETag(t *testing.T) {
	handler := NewAgentCatalogHandler("")
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/downloads/agent-catalog", nil))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"schema_version": 1`) {
		t.Fatalf("default catalog response = %d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	secondRequest := httptest.NewRequest(http.MethodGet, "/downloads/agent-catalog", nil)
	secondRequest.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional catalog response = %d", second.Code)
	}
}

func TestAgentCatalogHandlerUsesValidHubPublishedCatalog(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifacts", "agent-catalog", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := `{"schema_version":1,"revision":"team-2","adapters":[{"type":"team-cli","display_name":"Team CLI","transport":"native","catalog_tier":"native","launchers":[{"commands":["team-cli"]}],"output_mode":"text"}]}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewAgentCatalogHandler(root).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/downloads/agent-catalog", nil))
	if response.Code != http.StatusOK || response.Header().Get("X-AgentCollab-Catalog-Revision") != "team-2" || !strings.Contains(response.Body.String(), "team-cli") {
		t.Fatalf("published catalog response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestAgentCatalogHandlerFallsBackFromInvalidPublishedCatalog(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifacts", "agent-catalog", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewAgentCatalogHandler(root).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/downloads/agent-catalog", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"type": "codex"`) {
		t.Fatalf("invalid published catalog did not fall back safely: %d %s", response.Code, response.Body.String())
	}
}
