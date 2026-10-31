package buildinfo

import (
	"strings"
	"testing"
)

func TestInfoStringContainsBuildIdentity(t *testing.T) {
	info := Info{
		Name:    "agentcollab-hub",
		Version: "v0.1.0-test",
		Commit:  "abc1234",
		BuiltAt: "2026-07-16T00:00:00Z",
	}

	got := info.String()
	for _, want := range []string{info.Name, info.Version, info.Commit, info.BuiltAt} {
		if !strings.Contains(got, want) {
			t.Fatalf("Info.String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestCurrentUsesLinkerValues(t *testing.T) {
	oldVersion, oldCommit, oldBuiltAt := Version, Commit, BuiltAt
	t.Cleanup(func() {
		Version, Commit, BuiltAt = oldVersion, oldCommit, oldBuiltAt
	})

	Version = "v-test"
	Commit = "commit-test"
	BuiltAt = "time-test"

	got := Current("agentcollab-runner")
	if got.Name != "agentcollab-runner" || got.Version != Version || got.Commit != Commit || got.BuiltAt != BuiltAt {
		t.Fatalf("Current() = %#v, want linker values and supplied name", got)
	}
}
