package verification

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Lifetime is an independent cleanup owner, rather than a Go defer in the
// viewer. Its private control pipe closes even when the caller is SIGKILLed.
// Only one preview/run may own a checkout's dependency services at a time.
type Lifetime struct {
	control   io.WriteCloser
	root      string
	candidate bool
	done      chan struct{}
	err       error
}
type lifetimeRequest struct {
	Root      string `json:"root"`
	Candidate bool   `json:"candidate,omitempty"`
}
type lifetimeReady struct {
	Error string `json:"error,omitempty"`
}

func OwnLifetime(root string, output io.Writer) (*Lifetime, error) {
	return ownLifetime(root, output, false)
}

func OwnCandidateLifetime(root string, output io.Writer) (*Lifetime, error) {
	return ownLifetime(root, output, true)
}
func ownLifetime(root string, output io.Writer, candidate bool) (*Lifetime, error) {
	service := New(root, "")
	service.candidate = candidate
	recipe, err := service.loadRecipe()
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
	lifetime := &Lifetime{control: control, root: root, candidate: candidate, done: make(chan struct{})}
	if err = json.NewEncoder(control).Encode(lifetimeRequest{Root: root, Candidate: candidate}); err == nil {
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

// Release hands a prepared environment back to the standalone contract without
// stopping its services. EOF or process death still means cleanup; retaining
// resources requires this explicit message on the owner's private pipe.
func (l *Lifetime) Release() error {
	if l == nil {
		return nil
	}
	err := json.NewEncoder(l.control).Encode(struct {
		Operation string `json:"operation"`
	}{Operation: "release"})
	_ = l.control.Close()
	<-l.done
	if err != nil {
		return err
	}
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
	stopping := filepath.Join(request.Root, ".archetipo", "environment", "stopping")
	if err = os.Remove(stopping); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = json.NewEncoder(output).Encode(lifetimeReady{}); err != nil {
		return err
	}
	released := make(chan bool, 1)
	go func() {
		var command struct {
			Operation string `json:"operation"`
		}
		err := json.NewDecoder(reader).Decode(&command)
		released <- err == nil && command.Operation == "release"
	}()
	select {
	case retain := <-released:
		if retain {
			return nil
		}
	case <-parent.Done():
	}
	if err = os.MkdirAll(filepath.Dir(stopping), 0o700); err != nil {
		return err
	}
	if err = os.WriteFile(stopping, []byte("stopping\n"), 0o600); err != nil {
		return err
	}
	// A command's own supervisor might still be terminating after parent death.
	// Retry only lock conflicts, bounded by the command supervisor's exit window.
	ctx, cancel := context.WithTimeout(context.Background(), 17*time.Minute)
	defer cancel()
	service := New(request.Root, "")
	service.lifetimeOwner = true
	service.candidate = request.Candidate
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
	service.candidate = l.candidate
	return service
}

func (l *Lifetime) Done() <-chan struct{} { return l.done }
