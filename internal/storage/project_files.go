package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func projectSegment(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.Base(value) != value || value == "." || value == ".." {
		return "", fmt.Errorf("invalid project directory segment")
	}
	return value, nil
}

func WorkspaceDirectory(workspaceID string) (string, error) {
	id, err := projectSegment(workspaceID)
	if err != nil {
		return "", err
	}
	return filepath.Join("workspaces", id), nil
}

func RoomDirectory(workspaceID, roomID string) (string, error) {
	workspaceDir, err := WorkspaceDirectory(workspaceID)
	if err != nil {
		return "", err
	}
	id, err := projectSegment(roomID)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspaceDir, "rooms", id), nil
}

func EnsureWorkspaceTree(root, workspaceID string) error {
	base, err := WorkspaceDirectory(workspaceID)
	if err != nil {
		return err
	}
	for _, relative := range []string{base, filepath.Join(base, "knowledge"), filepath.Join(base, "rooms"), filepath.Join(base, "git"), filepath.Join(base, "git", "project.git")} {
		absolute, err := ResolveWithin(root, relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(absolute, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// WorkspaceGitDirectory is the Server-side bare repository location. It is a
// transport and collaboration hub, not a checked-out working tree.
func WorkspaceGitDirectory(workspaceID string) (string, error) {
	base, err := WorkspaceDirectory(workspaceID)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "git", "project.git"), nil
}

func EnsureRoomTree(root, workspaceID, roomID string) error {
	if err := EnsureWorkspaceTree(root, workspaceID); err != nil {
		return err
	}
	base, err := RoomDirectory(workspaceID, roomID)
	if err != nil {
		return err
	}
	for _, name := range []string{"", "knowledge", "uploads", "deliverables"} {
		absolute, err := ResolveWithin(root, filepath.Join(base, name))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(absolute, 0o700); err != nil {
			return err
		}
	}
	return nil
}
