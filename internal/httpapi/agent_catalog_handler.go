package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"

	"agentcollab.local/agentcollab/internal/agentcatalog"
)

// AgentCatalogHandler publishes the Hub's versioned, data-only CLI adapter
// catalog. Clients validate the schema before caching it and keep their last
// valid copy if the Hub publishes a malformed or future-schema document.
type AgentCatalogHandler struct {
	storageRoot string
}

func NewAgentCatalogHandler(storageRoot string) *AgentCatalogHandler {
	return &AgentCatalogHandler{storageRoot: storageRoot}
}

func (h *AgentCatalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.catalog()
	if err != nil {
		http.Error(w, "Agent adapter catalog is unavailable", http.StatusInternalServerError)
		return
	}
	payload, err := agentcatalog.Marshal(catalog)
	if err != nil {
		http.Error(w, "Agent adapter catalog is invalid", http.StatusInternalServerError)
		return
	}
	hash := sha256.Sum256(payload)
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-AgentCollab-Catalog-Revision", catalog.Revision)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(payload, '\n'))
}

func (h *AgentCatalogHandler) catalog() (agentcatalog.Catalog, error) {
	if strings.TrimSpace(h.storageRoot) != "" {
		path := filepath.Join(h.storageRoot, "artifacts", "agent-catalog", "catalog.json")
		if published, err := agentcatalog.Load(path); err == nil {
			return published, nil
		}
	}
	return agentcatalog.Default()
}
