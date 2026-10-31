package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func VerifyRoot(root string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("storage root must be absolute")
	}
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect storage root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("storage root must not be a symbolic link")
	}
	if !info.IsDir() {
		return fmt.Errorf("storage root is not a directory")
	}
	if info.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("storage root is not writable")
	}

	probe, err := os.CreateTemp(root, ".agentcollab-write-probe-")
	if err != nil {
		return fmt.Errorf("storage root write probe: %w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close storage write probe: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("remove storage write probe: %w", err)
	}
	return nil
}

func ResolveWithin(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be relative to storage root")
	}
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.Clean(relative))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve storage path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes storage root")
	}
	return candidate, nil
}

func FreeBytes(root string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return 0, fmt.Errorf("stat storage filesystem: %w", err)
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

type HubLock struct {
	mu     sync.Mutex
	path   string
	token  string
	closed bool
}

func AcquireHubLock(root string) (*HubLock, error) {
	lockPath, err := ResolveWithin(root, "data/hub.lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another Hub instance or stale lock holds %s", lockPath)
		}
		return nil, fmt.Errorf("create atomic hub lock directory: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("generate hub lock token: %w", err)
	}
	hostname, _ := os.Hostname()
	owner := lockOwner{
		Token:      token,
		PID:        os.Getpid(),
		Hostname:   hostname,
		AcquiredAt: time.Now().UTC(),
	}
	ownerBytes, err := json.Marshal(owner)
	if err != nil {
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("encode hub lock owner: %w", err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, "owner.json"), ownerBytes, 0o600); err != nil {
		_ = os.RemoveAll(lockPath)
		return nil, fmt.Errorf("write hub lock owner: %w", err)
	}
	return &HubLock{path: lockPath, token: token}, nil
}

func (l *HubLock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	ownerBytes, err := os.ReadFile(filepath.Join(l.path, "owner.json"))
	if err != nil {
		return fmt.Errorf("read hub lock owner before release: %w", err)
	}
	var owner lockOwner
	if err := json.Unmarshal(ownerBytes, &owner); err != nil {
		return fmt.Errorf("decode hub lock owner before release: %w", err)
	}
	if owner.Token != l.token {
		return fmt.Errorf("hub lock ownership changed; refusing to remove %s", l.path)
	}
	if err := os.RemoveAll(l.path); err != nil {
		return fmt.Errorf("remove hub lock directory: %w", err)
	}
	l.closed = true
	return nil
}

type lockOwner struct {
	Token      string    `json:"token"`
	PID        int       `json:"pid"`
	Hostname   string    `json:"hostname"`
	AcquiredAt time.Time `json:"acquired_at"`
}

func randomToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
