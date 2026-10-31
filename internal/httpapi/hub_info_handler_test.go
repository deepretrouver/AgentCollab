package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/config"
)

func TestHubInfoUsesConfiguredCanonicalAddress(t *testing.T) {
	handler := NewHubInfoHandler(config.Hub{HubID: "hub-research", HubName: "研究协作", PublicURL: "http://agentcollab.office:8080"}, "0.4.0")
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/hub/info", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"service":"agentcollab-hub"`, `"hub_id":"hub-research"`, `"name":"研究协作"`, `"canonical_url":"http://agentcollab.office:8080"`, `"candidate_urls":["http://agentcollab.office:8080"]`, `"client_download_url":"http://agentcollab.office:8080/downloads/client"`, `"version":"0.4.0"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body=%s missing %s", body, want)
		}
	}
}

func TestHubInfoFallsBackToRequestHost(t *testing.T) {
	handler := NewHubInfoHandler(config.Hub{}, "dev")
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "https://lan-hub.example/api/v1/hub/info", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"canonical_url":"https://lan-hub.example"`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}
