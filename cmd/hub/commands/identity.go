package commands

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentcollab.local/agentcollab/internal/config"
)

// Called after acquiring the Hub lock. Identity follows the data directory,
// not the host IP, so reconnecting Clients recognize a restarted/moved Server.
func ensureHubIdentity(cfg *config.Hub) error {
	if strings.TrimSpace(cfg.HubID) != "" {
		return nil
	}
	file := filepath.Join(cfg.StorageRoot, "data", "hub-id")
	data, err := os.ReadFile(file)
	if err == nil {
		cfg.HubID = strings.TrimSpace(string(data))
		if cfg.HubID == "" {
			return fmt.Errorf("saved Hub identity is empty")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	identity := "hub-" + hex.EncodeToString(random[:])
	if err := os.WriteFile(file+".tmp", []byte(identity+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(file+".tmp", file); err != nil {
		return err
	}
	cfg.HubID = identity
	return nil
}
