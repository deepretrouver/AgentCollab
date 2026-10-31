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

type Runner struct {
	HubURL             string           `yaml:"hub_url"`
	LocalDir           string           `yaml:"local_dir"`
	CredentialsFile    string           `yaml:"credentials_file"`
	MaxConcurrentTasks int              `yaml:"max_concurrent_tasks"`
	Resources          []RunnerResource `yaml:"resources"`
	Agents             []RunnerAgent    `yaml:"agents"`
}

type RunnerResource struct {
	Name        string `yaml:"name"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
	Access      string `yaml:"access"`
}

type RunnerAgent struct {
	ID             string            `yaml:"id"`
	DisplayName    string            `yaml:"display_name"`
	Type           string            `yaml:"type"`
	Transport      string            `yaml:"transport,omitempty"`
	CatalogTier    string            `yaml:"catalog_tier,omitempty"`
	Command        string            `yaml:"command"`
	Args           []string          `yaml:"args"`
	ResumeArgs     []string          `yaml:"resume_args"`
	Environment    map[string]string `yaml:"environment"`
	PermissionMode string            `yaml:"permission_mode"`
	ACPPermission  string            `yaml:"acp_permission_policy,omitempty"`
	ACPMode        string            `yaml:"acp_mode,omitempty"`
	OutputMode     string            `yaml:"output_mode"`
	PromptStdin    bool              `yaml:"prompt_stdin"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	VersionArgs    []string          `yaml:"version_args"`
	NativeHistory  string            `yaml:"native_history"`
	SkillRoots     []string          `yaml:"skill_roots"`
}

func DecodeRunner(r io.Reader) (Runner, error) {
	var cfg Runner
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Runner{}, fmt.Errorf("decode runner config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Runner{}, err
	}
	return cfg, nil
}

func LoadRunner(path string) (Runner, error) {
	file, err := os.Open(path)
	if err != nil {
		return Runner{}, fmt.Errorf("open runner config %q: %w", path, err)
	}
	defer file.Close()
	return DecodeRunner(file)
}

func SaveRunner(path string, cfg Runner) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	payload, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode runner config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create runner config directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return fmt.Errorf("write runner config: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace runner config: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func (c Runner) Validate() error {
	parsed, err := url.Parse(c.HubURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("hub_url must be an absolute http or https URL")
	}
	if !filepath.IsAbs(c.LocalDir) {
		return fmt.Errorf("local_dir must be absolute")
	}
	if !filepath.IsAbs(c.CredentialsFile) {
		return fmt.Errorf("credentials_file must be absolute")
	}
	if c.MaxConcurrentTasks < 0 || c.MaxConcurrentTasks > 64 {
		return fmt.Errorf("max_concurrent_tasks must be between 0 and 64")
	}
	resourceNames := make(map[string]bool)
	for index, resource := range c.Resources {
		if strings.TrimSpace(resource.Name) == "" || strings.TrimSpace(resource.Path) == "" {
			return fmt.Errorf("resources[%d] requires name and path", index)
		}
		if resourceNames[resource.Name] {
			return fmt.Errorf("resources[%d].name must be unique", index)
		}
		resourceNames[resource.Name] = true
		if !filepath.IsAbs(resource.Path) {
			return fmt.Errorf("resources[%d].path must be absolute", index)
		}
		if resource.Access != "" && resource.Access != "read_only" && resource.Access != "read_write" {
			return fmt.Errorf("resources[%d].access must be read_only or read_write", index)
		}
	}
	for index, agent := range c.Agents {
		if strings.TrimSpace(agent.ID) == "" || strings.TrimSpace(agent.Type) == "" || strings.TrimSpace(agent.Command) == "" {
			return fmt.Errorf("agents[%d] requires id, type, and command", index)
		}
		if strings.TrimSpace(agent.DisplayName) == "" {
			return fmt.Errorf("agents[%d] requires display_name", index)
		}
		if !filepath.IsAbs(agent.Command) {
			return fmt.Errorf("agents[%d].command must be absolute", index)
		}
		if agent.Transport != "" && agent.Transport != "native" && agent.Transport != "acp" {
			return fmt.Errorf("agents[%d].transport must be native or acp", index)
		}
		if agent.CatalogTier != "" && agent.CatalogTier != "native" && agent.CatalogTier != "preset" && agent.CatalogTier != "custom" {
			return fmt.Errorf("agents[%d].catalog_tier must be native, preset, or custom", index)
		}
		transport := agent.Transport
		if transport == "" {
			transport = "native"
		}
		if transport == "native" && agent.CatalogTier != "" && agent.CatalogTier != "native" {
			return fmt.Errorf("agents[%d].catalog_tier must be native for native transport", index)
		}
		if transport == "acp" && agent.CatalogTier == "native" {
			return fmt.Errorf("agents[%d].catalog_tier must be preset or custom for ACP transport", index)
		}
		if agent.PermissionMode != "" && agent.PermissionMode != "admin" {
			return fmt.Errorf("agents[%d].permission_mode only supports admin in the current test release", index)
		}
		if agent.ACPPermission != "" && agent.ACPPermission != "auto_allow" && agent.ACPPermission != "allow_once" && agent.ACPPermission != "deny" {
			return fmt.Errorf("agents[%d].acp_permission_policy must be auto_allow, allow_once, or deny", index)
		}
		if agent.Transport != "acp" && (agent.ACPPermission != "" || agent.ACPMode != "") {
			return fmt.Errorf("agents[%d] ACP settings require transport: acp", index)
		}
		if agent.OutputMode != "" && agent.OutputMode != "text" && agent.OutputMode != "codex-jsonl" && agent.OutputMode != "json-result" && agent.OutputMode != "pi-jsonl" {
			return fmt.Errorf("agents[%d].output_mode must be text, codex-jsonl, json-result, or pi-jsonl", index)
		}
		for rootIndex, root := range agent.SkillRoots {
			if !filepath.IsAbs(root) {
				return fmt.Errorf("agents[%d].skill_roots[%d] must be absolute", index, rootIndex)
			}
		}
	}
	return nil
}
