package storage

import (
	"fmt"
	"os"
)

var RequiredDirectories = []string{
	"app",
	"config",
	"data",
	"shared",
	"inbox",
	"artifacts",
	"task-files",
	"workspaces",
	"logs",
	"backups",
	"docs",
}

func EnsureLayout(root string) error {
	if err := VerifyRoot(root); err != nil {
		return err
	}
	for _, name := range RequiredDirectories {
		path, err := ResolveWithin(root, name)
		if err != nil {
			return err
		}
		if err := ensureDirectory(path); err != nil {
			return fmt.Errorf("ensure storage directory %q: %w", name, err)
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path is a symbolic link")
		}
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0o700)
}
