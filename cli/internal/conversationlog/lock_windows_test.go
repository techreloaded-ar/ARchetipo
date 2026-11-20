package conversationlog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func holdWindowsOwnerFile(t *testing.T, lock *ConversationLock, sharing uint32) func() {
	t.Helper()
	name, err := windows.UTF16PtrFromString(filepath.Join(lock.path, "owner"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, sharing, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closeReader := sync.OnceFunc(func() { _ = windows.CloseHandle(handle) })
	t.Cleanup(closeReader)
	return closeReader
}

func TestConversationUnlockWaitsForWindowsReader(t *testing.T) {
	store := newStore(t, t.TempDir())
	lock, err := store.Lock(context.Background(), "conv-reader")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
	closeReader := holdWindowsOwnerFile(t, lock, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); done <- lock.Unlock() }()
	<-started
	select {
	case err := <-done:
		t.Fatalf("unlock returned while deletion was denied: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	closeReader()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unlock after reader closed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unlock did not finish after reader closed")
	}
	next, err := store.TryLock(context.Background(), "conv-reader")
	if err != nil {
		t.Fatalf("released lock left a live-owner directory: %v", err)
	}
	if err := next.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestConversationLockReadFailurePreservesLiveOwner(t *testing.T) {
	store := newStore(t, t.TempDir())
	lock, err := store.Lock(context.Background(), "conv-live")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
	closeReader := holdWindowsOwnerFile(t, lock, windows.FILE_SHARE_DELETE)
	if _, err := store.TryLock(context.Background(), "conv-live"); err == nil {
		t.Fatal("an unreadable live owner was treated as a stale lock")
	}
	closeReader()
	if _, err := store.TryLock(context.Background(), "conv-live"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("reading failure removed the live owner's lock: %v", err)
	}
}

// A Windows process object outlives its process for as long as anyone holds a
// handle to it — the parent that started it, an antivirus inspecting it — and
// its pid stays openable meanwhile. The lock of a killed owner must be
// recovered all the same: being openable is not being alive.
func TestConversationLockOfAnExitedOwnerIsRecoveredWhileItsProcessHandleIsHeld(t *testing.T) {
	exitedOwner := exec.Command("cmd", "/c", "exit", "0")
	if err := exitedOwner.Start(); err != nil {
		t.Fatal(err)
	}
	exitedOwnerPID := exitedOwner.Process.Pid
	heldHandle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(exitedOwnerPID))
	if err != nil {
		_ = exitedOwner.Wait()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(heldHandle) })
	if err = exitedOwner.Wait(); err != nil {
		t.Fatal(err)
	}
	stillOpenable, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(exitedOwnerPID))
	if err != nil {
		t.Fatalf("the held handle does not keep the exited process openable, so this test proves nothing: %v", err)
	}
	_ = windows.CloseHandle(stillOpenable)

	if ProcessAlive(exitedOwnerPID) {
		t.Fatal("an exited process is reported alive while a handle to it is held")
	}
	store := newStore(t, t.TempDir())
	lock, err := store.Lock(context.Background(), "conv-exited-owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
	ownerOfExitedProcess := []byte(strconv.Itoa(exitedOwnerPID) + "\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n")
	if err = os.WriteFile(filepath.Join(lock.path, "owner"), ownerOfExitedProcess, 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.TryLock(context.Background(), "conv-exited-owner")
	if err != nil {
		t.Fatalf("the lock of an exited owner was not recovered: %v", err)
	}
	if err = recovered.Unlock(); err != nil {
		t.Fatal(err)
	}
}
