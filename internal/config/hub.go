package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Hub struct {
	StorageRoot      string   `yaml:"storage_root"`
	ListenAddress    string   `yaml:"listen_address"`
	HubID            string   `yaml:"hub_id"`
	HubName          string   `yaml:"hub_name"`
	PublicURL        string   `yaml:"public_url"`
	RunnerPublicURLs []string `yaml:"runner_public_urls"`
	DatabasePath     string   `yaml:"database_path"`
	LogPath          string   `yaml:"log_path"`
	RequireSMB       bool     `yaml:"require_smb"`
}

func DecodeHub(r io.Reader) (Hub, error) {
	cfg := Hub{
		ListenAddress: "127.0.0.1:8080",
		HubName:       "AgentCollab",
		DatabasePath:  "data/agentcollab.db",
		LogPath:       "logs/hub.jsonl",
	}
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Hub{}, fmt.Errorf("decode hub config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Hub{}, err
	}
	return cfg, nil
}

func LoadHub(path string) (Hub, error) {
	file, err := os.Open(path)
	if err != nil {
		return Hub{}, fmt.Errorf("open hub config %q: %w", path, err)
	}
	defer file.Close()
	// Resolve portable configurations relative to the config file, not the
	// launcher's working directory. DecodeHub still validates absolute roots.
	var raw map[string]interface{}
	if err := yaml.NewDecoder(file).Decode(&raw); err != nil {
		return Hub{}, fmt.Errorf("decode hub config: %w", err)
	}
	if root, ok := raw["storage_root"].(string); ok && strings.TrimSpace(root) != "" && !filepath.IsAbs(root) {
		absolute, err := filepath.Abs(filepath.Join(filepath.Dir(path), root))
		if err != nil {
			return Hub{}, err
		}
		raw["storage_root"] = absolute
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return Hub{}, err
	}
	return DecodeHub(strings.NewReader(string(data)))
}

func (c Hub) Validate() error {
	if strings.TrimSpace(c.StorageRoot) == "" {
		return fmt.Errorf("storage_root is required")
	}
	if !filepath.IsAbs(c.StorageRoot) {
		return fmt.Errorf("storage_root must be absolute")
	}
	if strings.TrimSpace(c.ListenAddress) == "" {
		return fmt.Errorf("listen_address is required")
	}
	if strings.TrimSpace(c.HubName) == "" {
		return fmt.Errorf("hub_name is required")
	}
	if c.PublicURL != "" {
		if err := validatePublicURL("public_url", c.PublicURL); err != nil {
			return err
		}
	}
	for index, value := range c.RunnerPublicURLs {
		if err := validatePublicURL(fmt.Sprintf("runner_public_urls[%d]", index), value); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{
		"database_path": c.DatabasePath,
		"log_path":      c.LogPath,
	} {
		if value == "" || filepath.IsAbs(value) || escapesRoot(value) {
			return fmt.Errorf("%s must be a relative path inside storage_root", name)
		}
	}
	return nil
}

// PublicURLs returns the canonical Hub address first, followed by any legacy
// Runner aliases. New clients only need the first URL; the aliases remain for
// older packages and temporary LAN migrations.
func (c Hub) PublicURLs() []string {
	values := make([]string, 0, 1+len(c.RunnerPublicURLs))
	seen := map[string]bool{}
	for _, value := range append([]string{c.PublicURL}, c.RunnerPublicURLs...) {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func validatePublicURL(field, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute http or https URL without credentials, query, or fragment", field)
	}
	return nil
}

func escapesRoot(path string) bool {
	cleaned := filepath.Clean(path)
	return cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator))
}
