package httpapi

import (
	"net/http"
	"strings"

	"agentcollab.local/agentcollab/internal/config"
)

// HubInfoHandler is the public bootstrap document for browsers and desktop
// clients. It gives every installer one authoritative Hub identity and one set
// of server-derived links without exposing storage paths or credentials.
type HubInfoHandler struct {
	id         string
	name       string
	version    string
	publicURLs []string
}

func NewHubInfoHandler(cfg config.Hub, version string) *HubInfoHandler {
	name := strings.TrimSpace(cfg.HubName)
	if name == "" {
		name = "AgentCollab"
	}
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	return &HubInfoHandler{id: strings.TrimSpace(cfg.HubID), name: name, version: version, publicURLs: cfg.PublicURLs()}
}

func (h *HubInfoHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/hub/info", h.serve)
}

func (h *HubInfoHandler) serve(w http.ResponseWriter, r *http.Request) {
	baseURL := ""
	if len(h.publicURLs) > 0 {
		baseURL = h.publicURLs[0]
	} else {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		baseURL = scheme + "://" + r.Host
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":             "agentcollab-hub",
		"hub_id":              h.id,
		"name":                h.name,
		"version":             h.version,
		"canonical_url":       strings.TrimRight(baseURL, "/"),
		"candidate_urls":      h.publicURLs,
		"login_url":           strings.TrimRight(baseURL, "/") + "/",
		"client_download_url": strings.TrimRight(baseURL, "/") + "/downloads/client",
		"client_manifest_url": strings.TrimRight(baseURL, "/") + "/downloads/client/manifest",
		"agent_catalog_url":   strings.TrimRight(baseURL, "/") + "/downloads/agent-catalog",
		"admin_url":           strings.TrimRight(baseURL, "/") + "/?admin=1",
	})
}
