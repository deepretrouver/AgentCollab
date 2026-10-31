package httpapi

import (
	"context"
	"net/http"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/buildinfo"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/realtime"
)

func NewRouter(authService *auth.Service, collabRepository CollabRepository, brokers ...*realtime.Broker) http.Handler {
	return newRouter(authService, collabRepository, config.Hub{}, brokers...)
}

func NewRouterWithConfig(authService *auth.Service, collabRepository CollabRepository, cfg config.Hub, brokers ...*realtime.Broker) http.Handler {
	return newRouter(authService, collabRepository, cfg, brokers...)
}

func newRouter(authService *auth.Service, collabRepository CollabRepository, cfg config.Hub, brokers ...*realtime.Broker) http.Handler {
	mux := http.NewServeMux()
	publicURLs := cfg.PublicURLs()
	NewAuthHandlersWithRunnerURLs(authService, publicURLs).Register(mux)
	NewHubInfoHandler(cfg, buildinfo.Version).Register(mux)
	mux.Handle("GET /downloads/agent-catalog", NewAgentCatalogHandler(cfg.StorageRoot))
	if cfg.StorageRoot != "" {
		clientDownloads := NewClientDownloadHandler(cfg.StorageRoot, buildinfo.Version)
		runnerDownloads := NewRunnerDownloadHandler(cfg.StorageRoot, buildinfo.Version)
		mux.Handle("GET /downloads/runner", runnerDownloads)
		mux.HandleFunc("GET /downloads/runner/manifest", runnerDownloads.ServeManifest)
		mux.Handle("GET /downloads/client", clientDownloads)
		mux.HandleFunc("GET /downloads/client/manifest", clientDownloads.ServeManifest)
		if attachmentRepository, ok := collabRepository.(AttachmentRepository); ok {
			NewAttachmentHandlers(authService, attachmentRepository, cfg.StorageRoot).Register(mux)
		}
		if knowledgeRepository, ok := collabRepository.(KnowledgeRepository); ok {
			knowledgeHandlers := NewKnowledgeHandlers(authService, knowledgeRepository, cfg.StorageRoot)
			_ = knowledgeHandlers.Reconcile(context.Background())
			knowledgeHandlers.Register(mux)
		}
	}
	var publisher EventPublisher
	if len(brokers) > 0 {
		broker := brokers[0]
		publisher = broker
		NewCollabHandlers(authService, collabRepository, broker).WithStorageRoot(cfg.StorageRoot).Register(mux)
		if realtimeRepository, ok := collabRepository.(realtime.Repository); ok {
			realtime.NewHandler(authService, realtimeRepository, broker).Register(mux)
		}
	} else {
		NewCollabHandlers(authService, collabRepository).WithStorageRoot(cfg.StorageRoot).Register(mux)
	}
	if automationRepository, ok := collabRepository.(AutomationRepository); ok {
		NewAutomationHandlers(authService, automationRepository, publisher, cfg.StorageRoot).Register(mux)
	}
	if projectRepository, ok := collabRepository.(ProjectRepository); ok {
		NewProjectHandlers(authService, projectRepository, cfg.StorageRoot).Register(mux)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "agentcollab-hub", "hub_id": cfg.HubID, "version": buildinfo.Version})
	})
	mux.Handle("/", NewWebHandler())
	return mux
}
