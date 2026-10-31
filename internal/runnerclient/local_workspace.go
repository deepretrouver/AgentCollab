package runnerclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentcollab.local/agentcollab/internal/domain"
)

type localProjectBindings struct {
	Version  int                            `json:"version"`
	Bindings map[string]localProjectBinding `json:"bindings"`
}

type localProjectBinding struct {
	StepID         string `json:"stepId"`
	WorkspaceID    string `json:"workspaceId"`
	DirectoryPath  string `json:"directoryPath"`
	DirectoryLabel string `json:"directoryLabel"`
	UpdatedAt      string `json:"updatedAt"`
}

func (c *Client) resolveLocalProjectWorkspace(task *domain.ClaimedTask) error {
	if task.LocalWorkspace == nil {
		return nil
	}
	path := filepath.Join(filepath.Dir(c.config.CredentialsFile), "project-bindings.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("负责人尚未在 Client 中为步骤“%s”绑定本机工作目录", task.LocalWorkspace.DirectoryLabel)
		}
		return fmt.Errorf("读取本机步骤目录绑定: %w", err)
	}
	var values localProjectBindings
	if err := json.Unmarshal(payload, &values); err != nil {
		return fmt.Errorf("本机步骤目录绑定已损坏，请在看板重新绑定: %w", err)
	}
	binding, ok := values.Bindings[string(task.LocalWorkspace.StepID)]
	if !ok || strings.TrimSpace(binding.DirectoryPath) == "" {
		return fmt.Errorf("负责人尚未在 Client 中为这个步骤绑定本机工作目录")
	}
	info, err := os.Stat(binding.DirectoryPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("已绑定的本机目录当前不可用，请负责人在看板重新绑定")
	}
	task.LocalWorkspace.LocalPath = filepath.Clean(binding.DirectoryPath)
	if strings.TrimSpace(task.LocalWorkspace.DirectoryLabel) == "" {
		task.LocalWorkspace.DirectoryLabel = binding.DirectoryLabel
	}
	return nil
}
