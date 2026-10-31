package runnerclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestResolveLocalProjectWorkspaceUsesPersistentResponsibleDirectory(t *testing.T) {
	configDirectory := t.TempDir()
	projectDirectory := filepath.Join(t.TempDir(), "current-strategy")
	if err := os.MkdirAll(projectDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := `{
  "version": 1,
  "bindings": {
    "step-market": {
      "stepId": "step-market",
      "workspaceId": "workspace-one",
      "directoryPath": ` + quotedJSON(projectDirectory) + `,
      "directoryLabel": "current-strategy"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(configDirectory, "project-bindings.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &Client{config: config.Runner{CredentialsFile: filepath.Join(configDirectory, "credentials.json")}}
	task := &domain.ClaimedTask{LocalWorkspace: &domain.LocalProjectWorkspace{StepID: "step-market"}}
	if err := client.resolveLocalProjectWorkspace(task); err != nil {
		t.Fatal(err)
	}
	if task.LocalWorkspace.LocalPath != projectDirectory || task.LocalWorkspace.DirectoryLabel != "current-strategy" {
		t.Fatalf("resolved workspace = %#v", task.LocalWorkspace)
	}
}

func TestResolveLocalProjectWorkspaceRejectsUnavailableBinding(t *testing.T) {
	configDirectory := t.TempDir()
	client := &Client{config: config.Runner{CredentialsFile: filepath.Join(configDirectory, "credentials.json")}}
	task := &domain.ClaimedTask{LocalWorkspace: &domain.LocalProjectWorkspace{StepID: "step-market"}}
	err := client.resolveLocalProjectWorkspace(task)
	if err == nil || !strings.Contains(err.Error(), "尚未") {
		t.Fatalf("error = %v, want missing binding guidance", err)
	}
}

func quotedJSON(value string) string {
	result := strings.ReplaceAll(value, `\`, `\\`)
	result = strings.ReplaceAll(result, `"`, `\"`)
	return `"` + result + `"`
}
