package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunHealthcheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heartbeat")
	t.Setenv("HEARTBEAT_FILE", path)
	t.Setenv("CHECK_INTERVAL", "60")

	if code := runHealthcheck(); code != 1 {
		t.Fatalf("missing heartbeat: got exit %d, want 1", code)
	}

	if err := os.WriteFile(path, []byte("alive"), 0o600); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	if code := runHealthcheck(); code != 0 {
		t.Fatalf("fresh heartbeat: got exit %d, want 0", code)
	}

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("age heartbeat: %v", err)
	}
	if code := runHealthcheck(); code != 1 {
		t.Fatalf("stale heartbeat: got exit %d, want 1", code)
	}
}
