package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

const serverJournalPath = ".archetipo/environment/preview-process.json"
const serverStopPath = ".archetipo/environment/preview-stop.json"

// ServerState retains the original endpoint and the verified shutdown outcome.
// It is evidence for recovery, never permission to kill a saved PID or listener.
type ServerState struct {
	Version     int       `json:"version"`
	RunID       string    `json:"run_id"`
	URL         string    `json:"url"`
	State       string    `json:"state"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
	OwnerActive bool      `json:"owner_active"`
}

func saveServerState(root string, state ServerState) error {
	state.UpdatedAt = time.Now().UTC()
	return recordfile.WriteAtomic(filepath.Join(root, ".archetipo", "environment"), filepath.Join(root, serverJournalPath), ".preview-process-", state)
}

func readServerState(root string) (*ServerState, error) {
	raw, err := os.ReadFile(filepath.Join(root, serverJournalPath))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state ServerState
	if err = json.Unmarshal(raw, &state); err != nil || state.Version != 1 || len(state.RunID) != 32 || (state.State != "running" && state.State != "stopped" && state.State != "error") {
		return nil, iox.NewPrecondition("invalid preview process journal", "preserve "+serverJournalPath+" and inspect its original evidence", err)
	}
	if _, err = (Recipe{Version: 1, Directory: ".", URL: state.URL}).Validate(root); err != nil {
		return nil, err
	}
	return &state, nil
}

func newServerState(address string) (ServerState, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ServerState{}, err
	}
	return ServerState{Version: 1, RunID: hex.EncodeToString(nonce[:]), URL: address, State: "running"}, nil
}

type serverStopRequest struct {
	RunID string `json:"run_id"`
}

func serverStopRequested(root, runID string) bool {
	raw, err := os.ReadFile(filepath.Join(root, serverStopPath))
	var request serverStopRequest
	return err == nil && json.Unmarshal(raw, &request) == nil && request.RunID == runID
}

// The live supervisor consumes a request addressed to its unique run, so a
// delayed request can never stop a subsequent server or a reused process ID.
func watchServerStop(ctx context.Context, cancel context.CancelFunc, root, runID string) {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if serverStopRequested(root, runID) {
				cancel()
				return
			}
		}
	}
}

func requestServerStop(root string) error {
	state, err := observedServerState(root)
	if err != nil || state == nil || !state.OwnerActive {
		return err
	}
	if err = recordfile.WriteAtomic(filepath.Join(root, ".archetipo", "environment"), filepath.Join(root, serverStopPath), ".preview-stop-", serverStopRequest{RunID: state.RunID}); err != nil {
		return err
	}
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		current, readErr := observedServerState(root)
		if readErr != nil {
			return readErr
		}
		if current == nil || current.RunID != state.RunID {
			return iox.NewConflict("preview ownership changed during stop", "inspect the current preview before retrying", nil)
		}
		if !current.OwnerActive {
			// Its parent may still be running registered dependency cleanup.
			lease, leaseErr := store.TryLock(context.Background(), "environment-lifetime")
			if leaseErr == nil {
				return lease.Unlock()
			}
			if !errors.Is(leaseErr, conversationlog.ErrLockHeld) {
				return leaseErr
			}
		}
		if time.Now().After(deadline) {
			return iox.NewConflict("preview stop is still pending", "inspect its retained server and resource journals, then retry", nil)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func observedServerState(root string) (*ServerState, error) {
	state, err := readServerState(root)
	if err != nil || state == nil {
		return state, err
	}
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return nil, err
	}
	lock, err := store.TryLock(context.Background(), "preview-command")
	if errors.Is(err, conversationlog.ErrLockHeld) {
		state.OwnerActive = true
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	_ = lock.Unlock()
	state.OwnerActive = false
	return state, nil
}

// ReconcileStopped confirms a retained shutdown before environment deletion or
// reuse. An active supervisor is refused. Unknown listeners are never killed.
func ReconcileStopped(root string) error {
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return err
	}
	lock, err := store.TryLock(context.Background(), "preview-command")
	if err != nil {
		return iox.NewConflict("a preview supervisor still owns this checkout", "stop it through its owning viewer or terminal", err)
	}
	defer func() { _ = lock.Unlock() }()
	return reconcileStopped(root)
}

// reconcileStopped is also used by deletion, which already holds the command
// lease together with all other preview leases.
func reconcileStopped(root string) error {
	state, err := readServerState(root)
	if err != nil || state == nil || state.State == "stopped" {
		return err
	}
	if err = waitStopped(state.URL); err != nil {
		state.State, state.Error = "error", err.Error()
		return iox.NewPrecondition("preview stop is not verified", "inspect "+serverJournalPath+"; checkout preserved; no unknown listener was terminated", errors.Join(err, saveServerState(root, *state)))
	}
	state.State, state.Error, state.OwnerActive = "stopped", "", false
	return saveServerState(root, *state)
}

// VerifyStopped is for the increment deletion caller already holding the
// preview-command lease. It performs no process mutation.
func VerifyStopped(root string) error { return reconcileStopped(root) }
