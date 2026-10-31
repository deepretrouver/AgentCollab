package agentcatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const SchemaVersion = 1

//go:embed default.json
var defaultPayload []byte

type Catalog struct {
	SchemaVersion int       `json:"schema_version"`
	Revision      string    `json:"revision"`
	Adapters      []Adapter `json:"adapters"`
}

type Adapter struct {
	Type              string      `json:"type"`
	DisplayName       string      `json:"display_name"`
	Transport         string      `json:"transport"`
	CatalogTier       string      `json:"catalog_tier"`
	Launchers         []Launcher  `json:"launchers"`
	Args              []string    `json:"args,omitempty"`
	ResumeArgs        []string    `json:"resume_args,omitempty"`
	VersionArgs       []string    `json:"version_args,omitempty"`
	OutputMode        string      `json:"output_mode"`
	PermissionMode    string      `json:"permission_mode,omitempty"`
	ACPPermission     string      `json:"acp_permission_policy,omitempty"`
	ACPMode           string      `json:"acp_mode,omitempty"`
	PromptStdin       bool        `json:"prompt_stdin,omitempty"`
	TimeoutSeconds    int         `json:"timeout_seconds,omitempty"`
	NativeHistory     string      `json:"native_history,omitempty"`
	HideIfUnavailable bool        `json:"hide_if_unavailable,omitempty"`
	SkillRoots        []SkillRoot `json:"skill_roots,omitempty"`
}

// Launcher describes one executable discovery strategy. EntrypointPath is
// prepended to Args, ResumeArgs, and VersionArgs, which supports CLIs bundled
// inside a desktop app or a private Node runtime without adding Go code.
type Launcher struct {
	OS             []string `json:"os,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	CommandPaths   []string `json:"command_paths,omitempty"`
	EntrypointPath string   `json:"entrypoint_path,omitempty"`
}

type SkillRoot struct {
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
}

var safeType = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func Default() (Catalog, error) {
	return Decode(strings.NewReader(string(defaultPayload)))
}

func Decode(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode Agent adapter catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func Load(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer file.Close()
	return Decode(file)
}

func (c Catalog) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("Agent adapter catalog schema %d is unsupported; Client supports %d", c.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(c.Revision) == "" {
		return fmt.Errorf("Agent adapter catalog revision is required")
	}
	if len(c.Adapters) == 0 || len(c.Adapters) > 500 {
		return fmt.Errorf("Agent adapter catalog must contain 1-500 adapters")
	}
	seen := make(map[string]bool, len(c.Adapters))
	for index, adapter := range c.Adapters {
		if !safeType.MatchString(adapter.Type) {
			return fmt.Errorf("adapters[%d].type is invalid", index)
		}
		if seen[adapter.Type] {
			return fmt.Errorf("adapters[%d].type %q is duplicated", index, adapter.Type)
		}
		seen[adapter.Type] = true
		if strings.TrimSpace(adapter.DisplayName) == "" {
			return fmt.Errorf("adapters[%d].display_name is required", index)
		}
		if adapter.Transport != "native" && adapter.Transport != "acp" {
			return fmt.Errorf("adapters[%d].transport must be native or acp", index)
		}
		if adapter.CatalogTier != "native" && adapter.CatalogTier != "preset" && adapter.CatalogTier != "custom" {
			return fmt.Errorf("adapters[%d].catalog_tier is invalid", index)
		}
		if adapter.Transport == "native" && adapter.CatalogTier != "native" {
			return fmt.Errorf("adapters[%d] native transport requires native catalog_tier", index)
		}
		if adapter.Transport == "acp" && adapter.CatalogTier == "native" {
			return fmt.Errorf("adapters[%d] ACP transport requires preset or custom catalog_tier", index)
		}
		if len(adapter.Launchers) == 0 || len(adapter.Launchers) > 16 {
			return fmt.Errorf("adapters[%d] must contain 1-16 launchers", index)
		}
		for launcherIndex, launcher := range adapter.Launchers {
			if len(launcher.Commands) == 0 && len(launcher.CommandPaths) == 0 {
				return fmt.Errorf("adapters[%d].launchers[%d] requires a command or command_path", index, launcherIndex)
			}
			for _, command := range launcher.Commands {
				if strings.TrimSpace(command) == "" || strings.ContainsAny(command, `/\\`) {
					return fmt.Errorf("adapters[%d].launchers[%d] command must be a bare executable name", index, launcherIndex)
				}
			}
		}
		switch adapter.OutputMode {
		case "text", "codex-jsonl", "json-result", "pi-jsonl":
		default:
			return fmt.Errorf("adapters[%d].output_mode %q is unsupported by this Client", index, adapter.OutputMode)
		}
		if adapter.TimeoutSeconds < 0 || adapter.TimeoutSeconds > 86400 {
			return fmt.Errorf("adapters[%d].timeout_seconds must be between 0 and 86400", index)
		}
		for rootIndex, root := range adapter.SkillRoots {
			if strings.TrimSpace(root.Path) == "" {
				return fmt.Errorf("adapters[%d].skill_roots[%d].path is required", index, rootIndex)
			}
		}
	}
	return nil
}

// Merge applies a trusted local override by adapter type. It is deliberately
// data-only: entries select executables and argv; no shell command is evaluated.
func Merge(base, override Catalog) (Catalog, error) {
	if err := base.Validate(); err != nil {
		return Catalog{}, err
	}
	if err := override.Validate(); err != nil {
		return Catalog{}, err
	}
	merged := base
	merged.Revision = base.Revision + "+" + override.Revision
	indexes := make(map[string]int, len(merged.Adapters))
	for index, adapter := range merged.Adapters {
		indexes[adapter.Type] = index
	}
	for _, adapter := range override.Adapters {
		if index, found := indexes[adapter.Type]; found {
			merged.Adapters[index] = adapter
		} else {
			indexes[adapter.Type] = len(merged.Adapters)
			merged.Adapters = append(merged.Adapters, adapter)
		}
	}
	if err := merged.Validate(); err != nil {
		return Catalog{}, err
	}
	return merged, nil
}

func Marshal(c Catalog) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(c, "", "  ")
}
