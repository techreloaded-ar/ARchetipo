package verification

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
)

// SupervisorRunner gives the command a parent-death signal on every supported
// platform: closing the private control pipe cancels the command tree. The pipe
// also closes when the CLI is killed without a chance to run Go defers.
type SupervisorRunner struct{}

type supervisedCommand struct {
	Root    string  `json:"root"`
	Command Command `json:"command"`
}

func (SupervisorRunner) Run(ctx context.Context, root string, env []string, command Command, output io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "execution", "environment", "_run")
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = root, env, output, output
	cmd.WaitDelay = 2 * time.Second
	control, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer func() { _ = control.Close() }()
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = json.NewEncoder(control).Encode(supervisedCommand{Root: root, Command: command}); err != nil {
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
		case <-ended:
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			<-ended
		}
		return ctx.Err()
	}
}

// Supervise is the private subprocess entry point. Only its caller owns the
// control-pipe writer. Target commands never inherit that writer or read stdin.
func Supervise(parent context.Context, input io.Reader, output, stderr io.Writer) (int, error) {
	reader := bufio.NewReader(io.LimitReader(input, 1<<20))
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return 1, err
	}
	var request supervisedCommand
	if err = json.Unmarshal(line, &request); err != nil {
		return 1, err
	}
	if err = validateCommands([]Command{request.Command}); err != nil {
		return 1, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(request.Command.TimeoutSeconds)*time.Second)
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		cancel()
	}()
	store, err := conversationlog.NewFileStore(request.Root)
	if err != nil {
		return 1, err
	}
	lock, err := store.TryLock(ctx, "verification-command")
	if err != nil {
		return 1, err
	}
	defer func() { _ = lock.Unlock() }()
	cmd := exec.CommandContext(ctx, request.Command.Argv[0], request.Command.Argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr, cmd.Env = request.Root, output, stderr, os.Environ()
	cmd.WaitDelay = 2 * time.Second
	isolateProcess(cmd)
	err = cmd.Run()
	if ctx.Err() != nil {
		return 124, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), err
	}
	return 1, err
}
