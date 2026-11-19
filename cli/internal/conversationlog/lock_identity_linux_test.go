//go:build linux

package conversationlog

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// A lock left on a volume by a container's previous main process names a pid
// the next main process reuses. The recorded identity tells them apart.
func TestLockOfAnEarlierProcessWithTheSamePidIsRecovered(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.TryLock(context.Background(), "preview-environment")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.TryLock(context.Background(), "preview-environment"); err != ErrLockHeld {
		t.Fatalf("a lock of this very process is held: %v", err)
	}
	owner := filepath.Join(held.path, "owner")
	stale := strconv.Itoa(os.Getpid()) + "\n2026-01-01T00:00:00Z\nanother-boot:1\n"
	if err = os.WriteFile(owner, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := store.TryLock(context.Background(), "preview-environment")
	if err != nil {
		t.Fatalf("a lock of an earlier process with the same pid must be recovered: %v", err)
	}
	_ = again.Unlock()
}
