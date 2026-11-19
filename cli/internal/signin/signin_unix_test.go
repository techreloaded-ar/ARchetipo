//go:build !windows

package signin

import (
	"context"
	"io"
	"testing"
	"time"
)

// A wrapper that leaves a child holding the output open is what the Codex
// launcher looks like. Cancelling must still end the sign-in.
func TestCancellingStopsAWrapperAndTheChildItLeftBehind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	process, err := startCommand(ctx, Command{Name: "sh", Args: []string{"-c", "sleep 300 & echo started; wait"}})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 7)
	if _, err = io.ReadFull(process.Output(), buf); err != nil || string(buf) != "started" {
		t.Fatalf("output %q %v", buf, err)
	}
	cancel()
	ended := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, process.Output())
		_ = process.Wait()
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the sign-in did not end: the child kept its output open")
	}
}
