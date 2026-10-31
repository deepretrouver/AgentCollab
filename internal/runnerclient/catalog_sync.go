package runnerclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/agentcatalog"
)

const (
	agentCatalogRoute        = "/downloads/agent-catalog"
	agentCatalogSyncInterval = 5 * time.Minute
	maxAgentCatalogBytes     = 2 << 20
)

func syncAgentCatalog(ctx context.Context, cfgURL, localDir string, client *http.Client, etag string) (agentcatalog.Catalog, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfgURL, "/")+agentCatalogRoute, nil)
	if err != nil {
		return agentcatalog.Catalog{}, etag, err
	}
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := client.Do(request)
	if err != nil {
		return agentcatalog.Catalog{}, etag, fmt.Errorf("download Agent adapter catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		catalog, loadErr := loadDiscoveryCatalog(localDir)
		return catalog, etag, loadErr
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return agentcatalog.Catalog{}, etag, fmt.Errorf("download Agent adapter catalog: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	limited := io.LimitReader(response.Body, maxAgentCatalogBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return agentcatalog.Catalog{}, etag, err
	}
	if len(payload) > maxAgentCatalogBytes {
		return agentcatalog.Catalog{}, etag, fmt.Errorf("Agent adapter catalog exceeds %d bytes", maxAgentCatalogBytes)
	}
	remote, err := agentcatalog.Decode(strings.NewReader(string(payload)))
	if err != nil {
		return agentcatalog.Catalog{}, etag, err
	}
	if strings.TrimSpace(localDir) != "" {
		if err := saveAgentCatalog(filepath.Join(localDir, "adapter-catalog.json"), remote); err != nil {
			return agentcatalog.Catalog{}, etag, err
		}
	}
	resolved, err := loadDiscoveryCatalog(localDir)
	if err != nil {
		return agentcatalog.Catalog{}, etag, err
	}
	return resolved, response.Header.Get("ETag"), nil
}

func saveAgentCatalog(path string, catalog agentcatalog.Catalog) error {
	payload, err := agentcatalog.Marshal(catalog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Agent adapter catalog directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write Agent adapter catalog: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace Agent adapter catalog: %w", err)
	}
	return os.Chmod(path, 0o600)
}
