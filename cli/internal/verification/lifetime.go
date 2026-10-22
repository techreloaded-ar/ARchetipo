package verification

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Lifetime is an independent cleanup owner, rather than a Go defer in the
// viewer. Its private control pipe closes even when the caller is SIGKILLed.
// Only one preview/run may own a checkout's dependency services at a time.
type Lifetime struct {
	control io.Closer
	root    string
	done    chan struct{}
	err     error
}
type lifetimeRequest struct {
	Root string `json:"root"`
}
type lifetimeReady struct {
	Error string `json:"error,omitempty"`
}

func OwnLifetime(root string, output io.Writer) (*Lifetime, error) {
	recipe, err := Load(root)
	if err != nil {
		return nil, err
	}
	if len(recipe.Resources) == 0 {
		return nil, nil
	} // Legacy recipes retain their explicit lifecycle.
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, "execution", "environment", "_guard")
	cmd.Dir, cmd.Stderr = root, output
	control, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	status, err := cmd.StdoutPipe()
	if err != nil {
		_ = control.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = control.Close()
		return nil, err
	}
	lifetime := &Lifetime{control: control, root: root, done: make(chan struct{})}
	if err = json.NewEncoder(control).Encode(lifetimeRequest{root}); err == nil {
		var ready lifetimeReady
		err = json.NewDecoder(status).Decode(&ready)
		if err == nil && ready.Error != "" {
			err = iox.NewConflict(ready.Error, "stop the active environment owner before starting another preview", nil)
		}
	}
	if err != nil {
		_ = control.Close()
		_ = cmd.Wait()
		return nil, err
	}
	go func() { lifetime.err = cmd.Wait(); close(lifetime.done) }()
	return lifetime, nil
}
func (l *Lifetime) Close() error {
	if l == nil {
		return nil
	}
	_ = l.control.Close()
	<-l.done
	return l.err
}

// Guard holds the environment lock through cleanup. Logs/reports and resource
// errors survive both a failed cleanup and deletion of the current recipe.
func Guard(parent context.Context, input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(io.LimitReader(input, 1<<20))
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	var request lifetimeRequest
	if err = json.Unmarshal(line, &request); err != nil {
		return err
	}
	store, err := conversationlog.NewFileStore(request.Root)
	if err != nil {
		return err
	}
	lock, err := store.TryLock(parent, "environment-lifetime")
	if err != nil {
		_ = json.NewEncoder(output).Encode(lifetimeReady{Error: "another preview or environment run owns this checkout"})
		return err
	}
	defer func() { _ = lock.Unlock() }()
	if err = json.NewEncoder(output).Encode(lifetimeReady{}); err != nil {
		return err
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, reader); close(closed) }()
	select {
	case <-closed:
	case <-parent.Done():
	}
	// A command's own supervisor might still be terminating after parent death.
	// Retry only lock conflicts, bounded by the command supervisor's exit window.
	ctx, cancel := context.WithTimeout(context.Background(), 17*time.Minute)
	defer cancel()
	service := New(request.Root, "")
	service.lifetimeOwner = true
	deadline := time.Now().Add(6 * time.Second)
	for {
		result, cleanupErr := service.Execute(ctx, "cleanup", "", "", false)
		if cleanupErr == nil {
			if !result.Passed {
				return resourceFailure(result)
			}
			return nil
		}
		coded, ok := cleanupErr.(*iox.CodedError)
		if !ok || coded.Code != iox.CodeConflict || time.Now().After(deadline) {
			return fmt.Errorf("environment cleanup: %w", cleanupErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Service is for the caller holding this live lifetime handle. Other processes
// must acquire the lifetime lock before operating on the same environment.
func (l *Lifetime) Service(results string) *Service {
	service := New(l.root, results)
	service.lifetimeOwner = true
	return service
}

func (l *Lifetime) Done() <-chan struct{} { return l.done }
