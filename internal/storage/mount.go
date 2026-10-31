package storage

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type MountInfo struct {
	Source     string
	MountPoint string
	Type       string
}

func DetectMount(root string) (MountInfo, error) {
	output, err := exec.Command("mount").Output()
	if err != nil {
		return MountInfo{}, fmt.Errorf("run mount: %w", err)
	}
	return ParseMountOutput(string(output), root)
}

func ParseMountOutput(output, root string) (MountInfo, error) {
	root = filepath.Clean(root)
	var best MountInfo
	for _, line := range strings.Split(output, "\n") {
		open := strings.LastIndex(line, " (")
		if open < 0 || !strings.HasSuffix(line, ")") {
			continue
		}
		prefix := line[:open]
		separator := strings.Index(prefix, " on ")
		if separator < 0 {
			continue
		}
		mountPoint := filepath.Clean(prefix[separator+4:])
		if !containsPath(mountPoint, root) || len(mountPoint) <= len(best.MountPoint) {
			continue
		}
		options := line[open+2 : len(line)-1]
		mountType := strings.TrimSpace(strings.SplitN(options, ",", 2)[0])
		best = MountInfo{
			Source:     prefix[:separator],
			MountPoint: mountPoint,
			Type:       mountType,
		}
	}
	if best.MountPoint == "" {
		return MountInfo{}, fmt.Errorf("no mount contains storage root %q", root)
	}
	return best, nil
}

func containsPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m MountInfo) IsSMB() bool {
	return strings.Contains(strings.ToLower(m.Type), "smb")
}
