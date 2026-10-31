package agentcatalog

import (
	"strings"
	"testing"
)

func TestDefaultCatalogContainsCurrentAdapters(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"hermes": false, "codex": false, "pi": false, "workbuddy": false, "claude-code": false, "opencode": false, "openclaw": false, "codex-acp": false, "claude-code-acp": false}
	for _, adapter := range catalog.Adapters {
		if _, found := want[adapter.Type]; found {
			want[adapter.Type] = true
		}
	}
	for agentType, found := range want {
		if !found {
			t.Fatalf("default Agent adapter catalog missing %s", agentType)
		}
	}
}

func TestOpenCodeUsesSupportedPermissionFlag(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range catalog.Adapters {
		if adapter.Type != "opencode" {
			continue
		}
		for _, args := range [][]string{adapter.Args, adapter.ResumeArgs} {
			joined := " " + strings.Join(args, " ") + " "
			if strings.Contains(joined, "--dangerously-skip-permissions") || !strings.Contains(joined, " --auto ") {
				t.Fatalf("OpenCode 1.18 uses --auto for the explicit admin adapter: %v", args)
			}
		}
		return
	}
	t.Fatal("OpenCode adapter missing")
}

func TestCatalogRejectsUnknownOutputProtocol(t *testing.T) {
	payload := `{"schema_version":1,"revision":"test","adapters":[{"type":"future","display_name":"Future","transport":"native","catalog_tier":"native","launchers":[{"commands":["future"]}],"output_mode":"future-wire"}]}`
	if _, err := Decode(strings.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown output protocol was accepted: %v", err)
	}
}

func TestMergeReplacesAdapterByType(t *testing.T) {
	base, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	override, err := Decode(strings.NewReader(`{"schema_version":1,"revision":"local","adapters":[{"type":"pi","display_name":"Team Pi","transport":"native","catalog_tier":"native","launchers":[{"commands":["team-pi"]}],"output_mode":"text"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Merge(base, override)
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range merged.Adapters {
		if adapter.Type == "pi" && adapter.DisplayName != "Team Pi" {
			t.Fatalf("Pi override was not applied: %+v", adapter)
		}
	}
}
