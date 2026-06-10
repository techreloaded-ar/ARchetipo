// Package github implements the Connector interface against GitHub Issues
// and GitHub Projects v2. The implementation shells out to the `gh` CLI for
// auth and basic operations and uses `gh api graphql` for batch mutations
// that need aliased fields.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Runner abstracts execution of `gh` so tests can inject a fake. The real
// implementation forks a process; tests pass a recorded mock.
type Runner interface {
	// Run executes `gh args...` with the given stdin and returns stdout
	// and stderr. err is non-nil only when the process fails to spawn or
	// exits with a non-zero code; in that case stdout/stderr are still
	// returned for diagnostics.
	Run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr []byte, err error)
}

// realRunner shells out to the actual gh binary on $PATH.
type realRunner struct{}

// NewRealRunner returns a Runner that forks `gh`.
func NewRealRunner() Runner { return realRunner{} }

func (realRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.Bytes(), errOut.Bytes(), err
}

// runJSON runs gh with --jq passthrough and decodes stdout into v.
func runJSON(ctx context.Context, r Runner, v any, args ...string) error {
	stdout, stderr, err := r.Run(ctx, nil, args...)
	if err != nil {
		return classify(err, stderr)
	}
	if err := json.Unmarshal(stdout, v); err != nil {
		return iox.NewConnector(iox.CodeConnectorBackend, "decoding gh JSON output",
			"check that the gh CLI is up-to-date", err)
	}
	return nil
}

// runGraphQL runs `gh api graphql` with the given query (as a single -f query=
// argument) and decodes the data field of the response into v. vars is encoded
// to one -F flag per key with JSON values.
func runGraphQL(ctx context.Context, r Runner, query string, vars map[string]string, v any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, val := range vars {
		args = append(args, "-F", k+"="+val)
	}
	stdout, stderr, err := r.Run(ctx, nil, args...)
	if err != nil {
		return classify(err, stderr)
	}
	if v == nil {
		return nil
	}
	// gh api wraps the response under {"data": ...} only when the response
	// has no top-level "errors". Decode permissively.
	var envelope struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		return iox.NewConnector(iox.CodeConnectorBackend, "decoding GraphQL response", "", err)
	}
	if len(envelope.Errors) > 0 {
		return iox.NewConnector(iox.CodeConnectorBackend, "GraphQL errors",
			fmt.Sprintf("%s", envelope.Errors), nil)
	}
	if len(envelope.Data) == 0 {
		// Some gh versions return the data object at the top-level.
		return json.Unmarshal(stdout, v)
	}
	return json.Unmarshal(envelope.Data, v)
}

// classify maps an exec error + stderr text into a typed connector error so
// the CLI surfaces a stable code.
func classify(err error, stderr []byte) error {
	if err == nil {
		return nil
	}
	msg := string(stderr)
	if msg == "" {
		msg = err.Error()
	}
	switch {
	case bytes.Contains(stderr, []byte("authentication required")) ||
		bytes.Contains(stderr, []byte("Resource not accessible by integration")):
		return iox.NewConnector(iox.CodeConnectorAuth,
			"gh authentication or scope is missing",
			"run `gh auth refresh -s read:project -s project`", err)
	case bytes.Contains(stderr, []byte("could not resolve to a")) ||
		bytes.Contains(stderr, []byte("Not Found")):
		return iox.NewConnector(iox.CodeNotFound, msg, "", err)
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return iox.NewConnector(iox.CodeConnectorBackend, msg, "", err)
		}
		return iox.NewConnector(iox.CodeConnectorBackend, "running gh: "+err.Error(), "", err)
	}
}
