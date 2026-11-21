// Package health implements the filesystem heartbeat used by the container
// health check.
package health

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultFileName = "local-dns-drp.heartbeat"

// DefaultPath returns the heartbeat path inside the system temporary directory.
func DefaultPath() string {
	return filepath.Join(os.TempDir(), defaultFileName)
}

// Touch updates the heartbeat file with the current time. An empty path is a
// no-op so callers can disable heartbeats.
func Touch(path string) error {
	if path == "" {
		return nil
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600)
}

// Check verifies that the heartbeat file exists and is no older than maxAge.
func Check(path string, maxAge time.Duration) error {
	if path == "" {
		return errors.New("heartbeat path is empty")
	}
	if maxAge <= 0 {
		return errors.New("heartbeat max age must be positive")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read heartbeat: %w", err)
	}
	age := time.Since(info.ModTime())
	if age > maxAge {
		return fmt.Errorf("heartbeat is stale: %s old, maximum %s", age.Round(time.Second), maxAge)
	}
	return nil
}
