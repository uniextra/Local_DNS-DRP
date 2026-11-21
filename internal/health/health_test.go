package health

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heartbeat")

	if err := Check(path, time.Minute); err == nil {
		t.Fatal("expected an error for a missing heartbeat")
	}
	if err := Touch(path); err != nil {
		t.Fatalf("Touch() error: %v", err)
	}
	if err := Check(path, time.Minute); err != nil {
		t.Fatalf("Check() fresh heartbeat error: %v", err)
	}

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes() error: %v", err)
	}
	if err := Check(path, time.Minute); err == nil {
		t.Fatal("expected an error for a stale heartbeat")
	}
}

func TestTouchEmptyPathIsNoop(t *testing.T) {
	if err := Touch(""); err != nil {
		t.Fatalf("Touch(\"\") error: %v", err)
	}
}

func TestCheckRejectsInvalidArguments(t *testing.T) {
	if err := Check("", time.Minute); err == nil {
		t.Fatal("expected an error for an empty path")
	}
	if err := Check(filepath.Join(t.TempDir(), "missing"), 0); err == nil {
		t.Fatal("expected an error for a non-positive max age")
	}
}
