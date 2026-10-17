package preview

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

type commandRequest struct {
	Directory string   `json:"directory"`
	Argv      []string `json:"argv"`
}

// The child supervisor owns the command tree. Its private control pipe closes
// even on hard viewer death, so development servers do not become orphans.
func runCommand(ctx context.Context, dir string, argv []string, output io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "execution", "preview", "_run")
	cmd.Dir, cmd.Stdout, cmd.Stderr, cmd.WaitDelay = dir, output, output, 2*time.Second
	control, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer func() { _ = control.Close() }()
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = json.NewEncoder(control).Encode(commandRequest{dir, argv}); err != nil {
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
	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr, cmd.WaitDelay = req.Directory, output, stderr, 2*time.Second
	isolate(cmd)
	return cmd.Run()
}
