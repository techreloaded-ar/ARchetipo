// Package arcipelago implements the ARchetipo execution provider backed by an
// ARcipelago hub. It translates a spec.plan dispatch into an external task on
// the hub's machine-to-machine API and waits for that task to reach a terminal
// outcome, so a spec can be planned by a remote coding agent without opening a
// coding agent or the ARcipelago console by hand.
//
// The provider never touches the ARchetipo connector: the remote agent owns the
// persistence of the plan, and this package only starts the work, waits for it
// and reports the outcome. That separation is what guarantees that a remote
// failure cannot move the spec.
//
// It also bounds what this package can prove. The receipt it demands is a
// declaration of the remote agent, not an inspection of the connector, so it
// rules out an agent that simply terminated but not one that lied. Confirming
// that the spec really is PLANNED with a readable plan is done one layer up, in
// the CLI command, which already holds the connector.
//
// All HTTP goes through the injectable Doer, so tests exercise the real request
// building and status classification without a network.
package arcipelago

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// ProviderID is the registry id under which this provider is reachable.
const ProviderID = "arcipelago"

// Doer abstracts http.Client so tests can serve canned responses from an
// httptest server instead of the network. The real implementation is a plain
// *http.Client.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options carries the injectable seams of the provider. Every field is
// optional: New fills the zero values with the real implementations.
type Options struct {
	Doer Doer
	// StreamDoer performs the run event stream. It is separate from Doer
	// because the two have opposite needs, and defaults to Doer when a test
	// injects one.
	StreamDoer Doer
	Getenv     func(string) string
	Sleep      func(context.Context, time.Duration) error
	Now        func() time.Time
}

// Provider dispatches spec.plan actions to an ARcipelago hub.
type Provider struct {
	doer       Doer
	streamDoer Doer
	getenv     func(string) string
	sleep      func(context.Context, time.Duration) error
	now        func() time.Time

	// conversationsMu guards both maps below. One lock and not two: reserving
	// an id and registering its mirror are one moment, and two locks would let
	// a second open slip between them.
	conversationsMu sync.RWMutex
	conversations   map[string]*liveConversation
	// registry mirrors the conversations this provider holds, and is built on
	// first use: a provider that never holds one costs nothing for it.
	registry *localrun.Registry
}

var _ execution.Provider = (*Provider)(nil)

// New builds a provider, defaulting every unset seam to its real
// implementation. It never returns nil and never reads the environment: the
// secret is only looked up during Execute.
func New(options Options) *Provider {
	p := &Provider{
		doer:       options.Doer,
		streamDoer: options.StreamDoer,
		getenv:     options.Getenv,
		sleep:      options.Sleep,
		now:        options.Now,
	}
	if p.doer == nil {
		p.doer = &http.Client{Timeout: 30 * time.Second}
	}
	if p.streamDoer == nil {
		// A test that injects one client means it for every call it can observe,
		// so the stream follows Doer rather than reaching for the network.
		p.streamDoer = options.Doer
	}
	if p.streamDoer == nil {
		p.streamDoer = newStreamClient()
	}
	if p.getenv == nil {
		p.getenv = os.Getenv
	}
	if p.sleep == nil {
		p.sleep = sleepWithContext
	}
	if p.now == nil {
		p.now = time.Now
	}
	return p
}

// streamHeaderTimeout bounds how long the hub may take to answer the headers of
// the event stream. It is the only timeout a stream can carry.
const streamHeaderTimeout = 30 * time.Second

// newStreamClient builds the client the run event stream uses.
//
// It deliberately has no Client.Timeout. That field bounds the whole exchange,
// body included, so a client carrying it cannot hold a text/event-stream open
// at all: the connection is killed on the deadline no matter how healthy it is,
// and the follower above sees a perfectly good stream as a drop to reconnect
// from — for ever, at the deadline's period. The stream is bounded where a
// stream can be bounded: the header phase has its own timeout, and the rest is
// governed by the request context, which the caller already cancels.
func newStreamClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = streamHeaderTimeout
	return &http.Client{Transport: transport}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *Provider) ID() string { return ProviderID }

func (p *Provider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{
		execution.CapabilitySpecPlan,
		execution.CapabilitySpecImplement,
	}, nil
}

// ValidateConfig checks the shape of the non-secret configuration only. It must
// not read the environment: otherwise `execution provider set-default` would
// become unrunnable on a machine that does not hold the credential.
func (p *Provider) ValidateConfig(_ context.Context, raw map[string]any) error {
	_, err := parseConfig(raw)
	return err
}

// Execute creates the external task on the ARcipelago hub and waits for it to
// reach a terminal outcome. Success is granted only against a valid receipt: a
// remote task that closes `completed` without having produced a plan is a
// provider failure, never a success on a spec that is still TODO. The receipt
// is a necessary condition, not a sufficient one — the sufficient one is the
// state of the connector, which only the CLI layer can read.
func (p *Provider) Execute(ctx context.Context, req execution.Request) (execution.Result, error) {
	cfg, err := parseConfig(req.ProviderConfig)
	if err != nil {
		return execution.Result{}, err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return execution.Result{}, fmt.Errorf("the ARcipelago application credential is not available: export it in the %s environment variable", cfg.TokenEnv)
	}
	task, err := p.createTask(ctx, cfg, token, req)
	if err != nil {
		return execution.Result{}, err
	}
	// From here on the remote task exists: every failure must name it and say
	// how to find it again, because the local wait giving up never cancels it.
	final, err := p.awaitTerminal(ctx, cfg, token, req, task)
	if err != nil {
		return execution.Result{}, p.remoteFailure(cfg, req, task.ID, err)
	}
	result, err := p.resultFor(cfg, req, final)
	if err != nil {
		return execution.Result{}, p.remoteFailure(cfg, req, task.ID, err)
	}
	return result, nil
}

// remoteFailure decorates a failure that happened after the remote task was
// created. Timeout used to be the only path carrying the recovery route, yet a
// transient 5xx during a long poll leaves exactly the same remote task alive and
// is the more likely outcome. The identifier also travels in a structured field
// through execution.RemoteError, so the failed record names it outside prose.
func (p *Provider) remoteFailure(cfg settings, req execution.Request, taskID string, cause error) error {
	return &execution.RemoteError{
		ExternalID: taskID,
		Err: fmt.Errorf(
			"%w; the remote task %s was created and is not cancelled by this failure: follow it with GET %s%s/%s or, from the external reference, with GET %s%s (workspaceId=%s, source=%s, externalId=%s)",
			cause, taskID,
			cfg.BaseURL, pathTasks, taskID,
			cfg.BaseURL, byReferenceQuery(cfg, req.ExecutionID),
			cfg.WorkspaceID, sourceARchetipo, req.ExecutionID,
		),
	}
}

// createTask posts the external task. Both 201 and 200 are a successful
// creation: 200 means ARcipelago recognized the request as a repetition with an
// identical fingerprint and did not create a second task.
func (p *Provider) createTask(ctx context.Context, cfg settings, token string, req execution.Request) (remoteTask, error) {
	title, prompt, metadata := buildTask(req)
	body := createTaskRequest{
		WorkspaceID: cfg.WorkspaceID,
		Source:      sourceARchetipo,
		ExternalID:  req.ExecutionID,
		Title:       title,
		Prompt:      prompt,
		Metadata:    metadata,
	}
	var envelope taskEnvelope
	status, err := p.do(ctx, cfg, token, http.MethodPost, pathTasks, body, &envelope)
	if err != nil {
		if status == http.StatusConflict {
			err = p.describeConflict(ctx, cfg, token, req.ExecutionID, err)
		}
		return remoteTask{}, err
	}
	// A task without an id would make the poll build `GET /api/external/tasks/`,
	// whose 404 an operator would read as "workspace not granted" — a wrong
	// diagnosis. Reject it before any polling starts.
	if strings.TrimSpace(envelope.Task.ID) == "" {
		return remoteTask{}, fmt.Errorf("arcipelago answered the task creation with a task without an identifier, so the remote outcome cannot be followed")
	}
	return envelope.Task, nil
}

// describeConflict enriches an external identity conflict with the id of the
// task that already holds the reference. A failure to read it leaves the
// original message untouched rather than cascading a second error.
func (p *Provider) describeConflict(ctx context.Context, cfg settings, token, externalID string, cause error) error {
	var conflict *externalIdentityConflictError
	if !errors.As(cause, &conflict) {
		return cause
	}
	var envelope taskEnvelope
	if _, err := p.do(ctx, cfg, token, http.MethodGet, byReferenceQuery(cfg, externalID), nil, &envelope); err != nil {
		return cause
	}
	conflict.existingTaskID = strings.TrimSpace(envelope.Task.ID)
	return cause
}

// awaitTerminal polls the remote task until it reaches a terminal status or the
// configured timeout expires.
func (p *Provider) awaitTerminal(ctx context.Context, cfg settings, token string, req execution.Request, task remoteTask) (remoteTask, error) {
	deadline := p.now().Add(cfg.Timeout)
	for {
		if err := ctx.Err(); err != nil {
			return remoteTask{}, fmt.Errorf("waiting for arcipelago task %s was interrupted: %w", task.ID, err)
		}
		var envelope taskEnvelope
		if _, err := p.do(ctx, cfg, token, http.MethodGet, pathTasks+"/"+url.PathEscape(task.ID), nil, &envelope); err != nil {
			return remoteTask{}, err
		}
		current := envelope.Task
		if strings.TrimSpace(current.ID) == "" {
			current.ID = task.ID
		}
		switch current.Status {
		case statusCompleted:
			return current, nil
		case statusFailed, statusCancelled:
			return remoteTask{}, fmt.Errorf("arcipelago task %s ended %s%s", current.ID, current.Status, summarySuffix(current.ResultSummary))
		}
		if !p.now().Before(deadline) {
			return remoteTask{}, timeoutError(cfg, task.ID, current.Status)
		}
		if err := p.sleep(ctx, cfg.PollInterval); err != nil {
			return remoteTask{}, fmt.Errorf("waiting for arcipelago task %s was interrupted: %w", task.ID, err)
		}
	}
}

// timeoutError states that the local wait gave up. The recovery route is not
// spelled out here: remoteFailure appends it to every post-creation failure, of
// which the timeout is only one.
func timeoutError(cfg settings, taskID, lastStatus string) error {
	return fmt.Errorf("timed out after %s waiting for arcipelago task %s, last observed status %q", cfg.Timeout, taskID, lastStatus)
}

// resultFor accepts the completed task only against a valid receipt, then
// builds the compact payload the execution record carries.
//
// The fork is the same one buildTask makes, and it has to be: the task was
// dispatched asking for one receipt, so accepting the other would let an agent
// close an implementation by declaring a plan.
func (p *Provider) resultFor(cfg settings, req execution.Request, task remoteTask) (execution.Result, error) {
	if req.Action == execution.ActionImplement {
		return p.implementResultFor(cfg, req, task)
	}
	return p.planResultFor(cfg, req, task)
}

// planResultFor accepts a planning task.
func (p *Provider) planResultFor(cfg settings, req execution.Request, task remoteTask) (execution.Result, error) {
	// The acceptance rule is the shared one: a receipt this provider accepted
	// and another rejected would be a contract that exists twice. Its two
	// distinct causes collapse into one message here on purpose, because from
	// the hub's side both read the same way — the remote task ended `completed`
	// and no plan came out of it — and the summary is quoted in full anyway.
	got, err := execution.AcceptPlanReceipt(task.ResultSummary, req.SpecCode)
	if err != nil {
		return execution.Result{}, fmt.Errorf(
			"arcipelago task %s ended completed without having produced a plan for %s%s",
			task.ID, req.SpecCode, summarySuffix(task.ResultSummary),
		)
	}
	payload, err := json.Marshal(struct {
		TaskID        string `json:"task_id"`
		WorkspaceID   string `json:"workspace_id"`
		Status        string `json:"status"`
		ResultSummary string `json:"result_summary"`
		PlanTasks     int    `json:"plan_tasks"`
	}{
		TaskID:        task.ID,
		WorkspaceID:   cfg.WorkspaceID,
		Status:        task.Status,
		ResultSummary: task.ResultSummary,
		PlanTasks:     got.Tasks,
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the arcipelago execution payload: %w", err)
	}
	return execution.Result{Payload: payload, ExternalID: task.ID}, nil
}

// implementResultFor accepts an implementation task.
//
// TasksDone and Tests are the agent's own account of the work, carried into the
// record so a reviewer reads a summary without opening the run. They are never
// the authority on what happened: that is the connector, read one layer up.
func (p *Provider) implementResultFor(cfg settings, req execution.Request, task remoteTask) (execution.Result, error) {
	got, err := execution.AcceptImplementReceipt(task.ResultSummary, req.SpecCode)
	if err != nil {
		return execution.Result{}, fmt.Errorf(
			"arcipelago task %s ended completed without having implemented %s%s",
			task.ID, req.SpecCode, summarySuffix(task.ResultSummary),
		)
	}
	payload, err := json.Marshal(struct {
		TaskID        string `json:"task_id"`
		WorkspaceID   string `json:"workspace_id"`
		Status        string `json:"status"`
		ResultSummary string `json:"result_summary"`
		TasksDone     int    `json:"tasks_done"`
		Tests         string `json:"tests"`
	}{
		TaskID:        task.ID,
		WorkspaceID:   cfg.WorkspaceID,
		Status:        task.Status,
		ResultSummary: task.ResultSummary,
		TasksDone:     got.TasksDone,
		Tests:         got.Tests,
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the arcipelago execution payload: %w", err)
	}
	return execution.Result{Payload: payload, ExternalID: task.ID}, nil
}

func summarySuffix(resultSummary string) string {
	summary := strings.TrimSpace(resultSummary)
	if summary == "" {
		return " (the remote agent reported no result summary)"
	}
	return ": " + truncate([]byte(summary))
}
