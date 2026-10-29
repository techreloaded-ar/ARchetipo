package web

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherBoundsWatchesToWorkspaceArtifacts(t *testing.T) {
	root := t.TempDir()
	generated := []string{".venv/lib/packages", ".next/cache", "node_modules/example", ".archetipo/environment/pristine-run/lib", ".archetipo/increments/worktrees/inc-one/docs", ".archetipo/conversations/runtime"}
	for _, rel := range append(append([]string{}, generated...), "docs/wiki", ".archetipo/specs") {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b := NewBroker()
	defer b.Close()
	w, err := NewWatcher(root, b)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.fsw.Close() }()
	if err = w.addTree(root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range generated {
		// New-directory events must not re-enter excluded ancestors either.
		if err = w.addTree(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
		if w.relevant(fsnotify.Event{Name: filepath.Join(root, rel, "generated.md"), Op: fsnotify.Create}) {
			t.Fatalf("generated runtime artifact triggered a workspace refresh: %s", rel)
		}
	}
	watches := w.fsw.WatchList()
	for _, path := range watches {
		rel, _ := filepath.Rel(root, path)
		for _, excluded := range []string{".venv", ".next", "node_modules", ".archetipo/environment", ".archetipo/increments", ".archetipo/conversations"} {
			if rel == excluded || strings.HasPrefix(filepath.ToSlash(rel), excluded+"/") {
				t.Fatalf("watcher entered generated tree %s", rel)
			}
		}
	}
	for _, rel := range []string{"docs/wiki/page.md", ".archetipo/specs/US-001.yaml"} {
		if !w.relevant(fsnotify.Event{Name: filepath.Join(root, rel), Op: fsnotify.Write}) {
			t.Fatalf("workspace artifact no longer refreshes: %s", rel)
		}
	}
}

func TestWatcherSkipsRuntimeWhenBacklogWatchRootIsArchetipo(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".archetipo")
	for _, rel := range []string{"conversations/turn.lock", "environment/pristine/lib", "increments/worktrees/inc-one/src", "specs"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b := NewBroker()
	defer b.Close()
	w, err := NewWatcher(root, b)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.fsw.Close() }()
	if err = w.addTree(root); err != nil {
		t.Fatal(err)
	}
	for _, path := range w.fsw.WatchList() {
		rel, _ := filepath.Rel(root, path)
		if rel != "." && rel != "specs" {
			t.Fatalf("backlog watcher entered runtime tree: %s", rel)
		}
	}
	if w.relevant(fsnotify.Event{Name: filepath.Join(root, "environment/report.md"), Op: fsnotify.Write}) {
		t.Fatal("runtime report triggered the backlog watcher")
	}
}

func TestWatcherPublishesOnYamlChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	b := NewBroker()
	defer b.Close()
	ch, unsub := b.Subscribe()
	defer unsub()

	w, err := NewWatcher(root, b)
	if err != nil {
		t.Fatal(err)
	}
	// Shorten the debounce so the test does not have to wait long.
	w.debounce = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Small sleep so the watcher has time to register the directories before
	// we generate file events.
	time.Sleep(50 * time.Millisecond)

	path := filepath.Join(root, "specs", "US-001.yaml")
	if err := os.WriteFile(path, []byte("code: US-001\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected broker event within 2s")
	}
}

func TestWatcherIgnoresIrrelevantFiles(t *testing.T) {
	root := t.TempDir()

	b := NewBroker()
	defer b.Close()
	ch, unsub := b.Subscribe()
	defer unsub()

	w, err := NewWatcher(root, b)
	if err != nil {
		t.Fatal(err)
	}
	w.debounce = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	time.Sleep(50 * time.Millisecond)

	for _, name := range []string{".DS_Store", "backlog.yaml.swp", ".hidden", "scratch.tmp"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case <-ch:
		t.Fatal("did not expect an event for irrelevant files")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestWatcherCoalescesBurst(t *testing.T) {
	root := t.TempDir()

	b := NewBroker()
	defer b.Close()
	ch, unsub := b.Subscribe()
	defer unsub()

	w, err := NewWatcher(root, b)
	if err != nil {
		t.Fatal(err)
	}
	w.debounce = 80 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		path := filepath.Join(root, "backlog.yaml")
		if err := os.WriteFile(path, []byte("v: "+string(rune('0'+i))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Expect one event after the debounce window settles.
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected one coalesced event")
	}
	// And no second event right away.
	select {
	case <-ch:
		t.Fatal("did not expect a second coalesced event so soon")
	case <-time.After(150 * time.Millisecond):
	}
}
