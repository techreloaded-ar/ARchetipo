package preview

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/processtree"
)

type commandRequest struct {
	Root      string   `json:"root,omitempty"`
	Directory string   `json:"directory"`
	Argv      []string `json:"argv"`
	URL       string   `json:"url,omitempty"`
	RunID     string   `json:"run_id,omitempty"`
}

// The child supervisor owns the command tree. Its private control pipe closes
// even on hard viewer death, so development servers do not become orphans.
func runCommand(ctx context.Context, root, dir, address, runID string, argv []string, env []string, output io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "execution", "preview", "_run")
	processtree.SeparateSupervisor(cmd)
	cmd.Dir, cmd.Stdout, cmd.Stderr, cmd.WaitDelay = dir, output, output, 2*time.Second
	cmd.Env = env
	control, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer func() { _ = control.Close() }()
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = json.NewEncoder(control).Encode(commandRequest{Root: root, Directory: dir, URL: address, RunID: runID, Argv: argv}); err != nil {
		_ = control.Close()
		_ = cmd.Wait()
		return err
	}
	ended := make(chan error, 1)
	go func() { ended <- cmd.Wait() }()
	select {
	case err = <-ended:
		return err
	case <-ctx.Done():
		_ = control.Close()
		select {
		case <-ended: //nolint:errcheck // Cancellation takes precedence over the command exit status.
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			<-ended //nolint:errcheck // Return the cancellation error below.
		}
		return ctx.Err()
	}
}

func Supervise(parent context.Context, input io.Reader, output, stderr io.Writer) error {
	reader := bufio.NewReader(input)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	var req commandRequest
	if err = json.Unmarshal(line, &req); err != nil {
		return err
	}
	if len(req.Argv) == 0 {
		return invalid("missing preview command")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, reader); cancel() }()
	root := req.Root
	if root == "" {
		root = req.Directory // Older private invocations did not carry the checkout.
	}
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return err
	}
	lock, err := store.TryLock(ctx, "preview-command")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	var state ServerState
	if req.URL != "" {
		if _, err = (Recipe{Version: 1, Directory: ".", URL: req.URL}).Validate(root); err != nil {
			return err
		}
		if state, err = newServerState(req.URL); err != nil {
			return err
		}
		if req.RunID != "" {
			if nonce, decodeErr := hex.DecodeString(req.RunID); decodeErr != nil || len(nonce) != 16 {
				return invalid("invalid preview run identity")
			}
			state.RunID = req.RunID
		}
		if err = saveServerState(root, state); err != nil {
			return err
		}
		go watchServerStop(ctx, cancel, root, state.RunID)
	}
	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr, cmd.WaitDelay = req.Directory, output, stderr, 2*time.Second
	err = processtree.Run(cmd)
	if req.URL == "" {
		return err
	}
	state.State = "stopped"
	stopErr := waitStopped(req.URL)
	if stopErr != nil {
		state.State, state.Error = "error", stopErr.Error()
	}
	return errors.Join(err, stopErr, saveServerState(root, state))
}
