package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

type Step struct {
	Name       string    `json:"name"`
	Argv       []string  `json:"argv"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	ExitCode   int       `json:"exit_code"`
	LogPath    string    `json:"log_path"`
	LogSHA256  string    `json:"log_sha256,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Result struct {
	ID            string    `json:"id"`
	Operation     string    `json:"operation"`
	Status        string    `json:"status"`
	Passed        bool      `json:"passed"`
	Stable        bool      `json:"stable"`
	Reused        bool      `json:"reused"`
	InputKey      string    `json:"input_key"`
	EnvironmentID string    `json:"environment_id"`
	PreparationID string    `json:"preparation_id,omitempty"`
	ReusedReport  string    `json:"reused_report,omitempty"`
	ReportPath    string    `json:"report_path"`
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int64     `json:"duration_ms"`
	Steps         []Step    `json:"steps"`
	Invalidation  string    `json:"invalidation,omitempty"`
}

type state struct {
	PreparationKey string            `json:"preparation_key"`
	PreparationID  string            `json:"preparation_id"`
	Checks         map[string]string `json:"checks"`
}

type Runner interface {
	Run(context.Context, string, []string, Command, io.Writer) error
}

type Service struct {
	Root          string
	Results       string
	Runner        Runner
	env           []string
	lifetimeOwner bool
	previewOwner  bool
	candidate     bool
}

func New(root, results string) *Service {
	return &Service{Root: root, Results: results, Runner: SupervisorRunner{}}
}

func (s *Service) dir() string { return filepath.Join(s.Root, ".archetipo", "environment") }

func (s *Service) environmentID() string {
	hash := sha256.Sum256([]byte(s.Root))
	return "env-" + hex.EncodeToString(hash[:8])
}

func (s *Service) readState() (state, error) {
	value := state{Checks: map[string]string{}}
	raw, err := os.ReadFile(filepath.Join(s.dir(), "state.json"))
	if os.IsNotExist(err) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err = json.Unmarshal(raw, &value); err != nil || value.Checks == nil {
		return value, iox.NewPrecondition("invalid environment checkpoint", "preserve the record and run environment cleanup before preparing again", err)
	}
	return value, nil
}

func (s *Service) saveState(value state) error {
	return recordfile.WriteAtomic(s.dir(), filepath.Join(s.dir(), "state.json"), ".state-", value)
}

// Execute is serialized across processes. A second caller fails promptly rather
// than starting another installer, server or writer against the same checkout.
func (s *Service) Execute(ctx context.Context, operation, name, spec string, fresh bool) (Result, error) {
	var result Result
	if operation != "prepare" && operation != "check" && operation != "cleanup" {
		return result, invalid("unknown environment operation")
	}
	if spec != "" && !regexpSpec(spec) {
		return result, invalid("invalid verification spec code")
	}
	recipe, err := s.loadRecipe()
	if err != nil {
		if operation != "cleanup" {
			return result, err
		}
		// Managed resources retain their own original stop adapters.
		if resources, resourceErr := s.Resources(); resourceErr != nil || resources.Owner == "" {
			return result, err
		}
		recipe = Recipe{}
	}
	liveCheck := false
	watched := []string{}
	if !s.lifetimeOwner {
		leaseStore, leaseErr := conversationlog.NewFileStore(s.Root)
		if leaseErr != nil {
			return result, leaseErr
		}
		lease, leaseErr := leaseStore.TryLock(ctx, "environment-lifetime")
		if leaseErr != nil {
			if !errors.Is(leaseErr, conversationlog.ErrLockHeld) || operation != "check" || !recipe.Checks[name].DuringPreview {
				return result, iox.NewConflict("an active preview or environment run owns these services", "use a check explicitly declared during_preview, or stop the owning preview before changing its environment", leaseErr)
			}
			liveCheck = true
			watched = append(watched, "environment-lifetime")
		} else {
			defer func() { _ = lease.Unlock() }()
		}
		previewLease, previewErr := leaseStore.TryLock(ctx, "preview-environment")
		if previewErr == nil {
			_ = previewLease.Unlock()
		} else if errors.Is(previewErr, conversationlog.ErrLockHeld) && s.previewOwner {
			// The caller already holds the preview lease.
		} else if errors.Is(previewErr, conversationlog.ErrLockHeld) {
			if operation != "check" || !recipe.Checks[name].DuringPreview {
				return result, iox.NewConflict("a preview owns this checkout", "stop the preview before preparing or cleaning its environment", previewErr)
			}
			liveCheck = true
			watched = append(watched, "preview-environment")
		} else {
			return result, previewErr
		}
		if liveCheck {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			go monitorLifetime(ctx, cancel, leaseStore, s.Root, watched)
		}
	}
	store, err := conversationlog.NewFileStore(s.Root)
	if err != nil {
		return result, err
	}
	lock, err := store.TryLock(ctx, "verification-environment")
	if errors.Is(err, conversationlog.ErrLockHeld) {
		return result, iox.NewConflict("another environment operation is running", "inspect its durable run before retrying; do not start a second setup", err)
	}
	if err != nil {
		return result, err
	}
	defer func() { _ = lock.Unlock() }()
	// A dead CLI's supervisor may still be terminating its command. Never start
	// another writer until that separate command owner has exited as well.
	commandLock, err := store.TryLock(ctx, "verification-command")
	if errors.Is(err, conversationlog.ErrLockHeld) {
		return result, iox.NewConflict("the previous verification command is still ending", "wait for its owned command tree to terminate before retrying", err)
	}
	if err != nil {
		return result, err
	}
	if err = commandLock.Unlock(); err != nil {
		return result, err
	}
	if operation != "cleanup" && !liveCheck && !s.candidate {
		if err = environmentrecipe.Sync(s.Root); err != nil {
			return result, err
		}
	}
	if operation != "cleanup" {
		recipe, err = s.loadRecipe()
		if err != nil {
			return result, err
		}
	}
	check, found := recipe.Checks[name]
	if operation == "check" && !found {
		return result, invalid("unknown project check: " + name)
	}
	value, err := s.readState()
	if err != nil && operation != "cleanup" {
		return result, err
	}
	if operation == "cleanup" {
		// Invalidate before touching services, even if cleanup is interrupted.
		if err = s.saveState(state{Checks: map[string]string{}}); err != nil {
			return result, err
		}
		result, err = s.newResult(operation, "")
		if err == nil {
			result.Stable = true
			resources, resourceErr := s.Resources()
			if resourceErr != nil {
				return result, resourceErr
			}
			s.env = resourceEnv(resources, "")
			err = s.stopResources(ctx, &result, &resources)
			if err == nil && result.Passed {
				err = s.run(ctx, &result, recipe.Cleanup)
			}
		}
		return result, err
	}
	resources, resourceErr := s.Resources()
	if resourceErr != nil {
		return result, resourceErr
	}
	if liveCheck {
		s.env = resourceEnv(resources, "")
		if len(recipe.Resources) == 0 {
			s.env = nil
		}
		if err = s.prepared(recipe, value); err != nil {
			return result, err
		}
	}
	if !liveCheck && (len(recipe.Resources) > 0 || len(resources.Resources) > 0) {
		result, err = s.newResult("resources", "")
		if err != nil {
			return result, err
		}
		if err = s.ensureResources(ctx, &result, recipe); err != nil || !result.Passed {
			return result, err
		}
		value, err = s.readState()
		if err != nil {
			return result, err
		}
	}
	preparation := Result{Passed: true, Stable: true}
	if !liveCheck {
		if operation == "prepare" && fresh {
			value = state{Checks: map[string]string{}}
			if err = s.saveState(value); err != nil {
				return result, err
			}
		}
		preparation, err = s.prepare(ctx, recipe, &value)
	}
	if (err != nil || !preparation.Passed) && len(recipe.Resources) > 0 {
		// Compensate a failed install/project probe as well as a failed resource start.
		cleanup, cleanupErr := s.newResult("prepare-rollback", "")
		if cleanupErr != nil {
			return preparation, cleanupErr
		}
		current, cleanupErr := s.Resources()
		if cleanupErr != nil {
			return preparation, cleanupErr
		}
		cleanCtx, cancel := context.WithTimeout(context.Background(), time.Duration(len(current.Resources)*60+5)*time.Second)
		cleanupErr = s.stopResources(cleanCtx, &cleanup, &current)
		cancel()
		if cleanupErr != nil {
			return preparation, cleanupErr
		}
	}
	if err != nil || !preparation.Passed || operation == "prepare" {
		return preparation, err
	}
	inputs := append(append([]string{RecipePath}, recipe.Inputs...), check.Inputs...)
	key, err := fingerprint(s.Root, recipe, inputs, value.PreparationID)
	if err != nil {
		return result, err
	}
	result, err = s.newResult("check:"+name, spec)
	if err != nil {
		return result, err
	}
	result.InputKey, result.PreparationID = key, value.PreparationID
	if check.Cache && !fresh {
		if previous, ok := s.cached(value.Checks[name], key); ok {
			current, hashErr := fingerprint(s.Root, recipe, inputs, value.PreparationID)
			if hashErr != nil {
				return result, hashErr
			}
			if current == key {
				result.Passed, result.Stable, result.Reused = true, true, true
				result.ReusedReport = previous.ReportPath
				result.Status = "complete"
				return result, s.saveResult(&result)
			}
			key, result.InputKey = current, current
		}
	}
	// A fresh failure must revoke an earlier pass of the same candidate.
	delete(value.Checks, name)
	if err = s.saveState(value); err != nil {
		return result, err
	}
	if err = s.run(ctx, &result, []Command{check.Command}); err != nil {
		return result, err
	}
	after, err := fingerprint(s.Root, recipe, inputs, value.PreparationID)
	if err != nil {
		return result, err
	}
	result.Stable = after == key
	if !result.Stable {
		result.Passed = false
		result.Invalidation = "declared inputs changed during verification"
	}
	if err = s.saveResult(&result); err != nil {
		return result, err
	}
	if result.Passed && result.Stable {
		value.Checks[name] = result.ReportPath
		if err = s.saveState(value); err != nil {
			return result, err
		}
	}
	return result, nil
}

func regexpSpec(spec string) bool {
	parts := strings.Split(spec, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, c := range parts[0] {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Service) prepare(ctx context.Context, recipe Recipe, value *state) (Result, error) {
	// Adding a check does not reinstall dependencies. Setup identity includes
	// its own inputs, outputs, commands, toolchain and environment keys only.
	preparationRecipe := recipe
	preparationRecipe.Checks, preparationRecipe.Cleanup = nil, nil
	key, err := fingerprint(s.Root, preparationRecipe, recipe.Inputs, s.env)
	if err != nil {
		return Result{}, err
	}
	result, err := s.newResult("prepare", "")
	if err != nil {
		return result, err
	}
	result.InputKey = key
	original, retained := s.cachedEvidence(filepath.Join(s.dir(), "runs", filepath.Base(value.PreparationID), "report.json"), key, len(recipe.Prepare)+len(recipe.Probe))
	if value.PreparationKey == key && value.PreparationID != "" && retained && s.outputsExist(recipe.Outputs) {
		if err = s.run(ctx, &result, recipe.Probe); err != nil {
			return result, err
		}
		after, hashErr := s.preparationFingerprint()
		if hashErr != nil {
			return result, hashErr
		}
		if result.Passed && after == key && s.outputsExist(recipe.Outputs) {
			result.Stable = true
			result.Reused, result.PreparationID = true, value.PreparationID
			result.ReusedReport = original.ReportPath
			return result, s.saveResult(&result)
		}
		*value = state{Checks: map[string]string{}}
		if err = s.saveState(*value); err != nil {
			return result, err
		}
		if ctx.Err() != nil {
			return result, nil
		}
		key = after
		// The failed readiness run remains intact; a new run owns the repair.
		result, err = s.newResult("prepare", "")
		if err != nil {
			return result, err
		}
		result.InputKey = key
	}
	*value = state{Checks: map[string]string{}}
	if err = s.saveState(*value); err != nil {
		return result, err
	}
	commands := append(append([]Command{}, recipe.Prepare...), recipe.Probe...)
	if err = s.run(ctx, &result, commands); err != nil {
		return result, err
	}
	after, err := s.preparationFingerprint()
	if err != nil {
		return result, err
	}
	result.Stable = after == key
	if !result.Stable || !s.outputsExist(recipe.Outputs) {
		result.Passed = false
		result.Invalidation = "setup changed declared inputs or did not create local outputs; repair the recipe and retry"
	}
	if result.Passed {
		value.PreparationKey, value.PreparationID = key, result.ID
		result.PreparationID = result.ID
		if err = s.saveState(*value); err != nil {
			return result, err
		}
	}
	return result, s.saveResult(&result)
}

func (s *Service) outputsExist(outputs []string) bool {
	for _, output := range outputs {
		path, err := localPath(s.Root, output)
		if err != nil {
			return false
		}
		if _, err = os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) newResult(operation, spec string) (Result, error) {
	dir := filepath.Join(s.dir(), "runs")
	if spec != "" {
		dir = s.Results
		if dir == "" {
			dir = "docs/test-results"
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(s.Root, dir)
		}
		dir = filepath.Join(dir, spec)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, err
	}
	path, err := os.MkdirTemp(dir, "verification-")
	if err != nil {
		return Result{}, err
	}
	result := Result{ID: filepath.Base(path), Operation: operation, Status: "running", EnvironmentID: s.environmentID(), ReportPath: filepath.Join(path, "report.json"), StartedAt: time.Now().UTC(), Steps: []Step{}}
	return result, s.saveResult(&result)
}

func (s *Service) saveResult(result *Result) error {
	result.DurationMS = time.Since(result.StartedAt).Milliseconds()
	return recordfile.WriteAtomic(filepath.Dir(result.ReportPath), result.ReportPath, ".report-", result)
}

func (s *Service) run(ctx context.Context, result *Result, commands []Command) error {
	result.Passed = true
	for _, command := range commands {
		step := Step{Name: command.Name, Argv: command.Argv, StartedAt: time.Now().UTC(), ExitCode: -1, LogPath: filepath.Join(filepath.Dir(result.ReportPath), fmt.Sprintf("%02d-%s.log", len(result.Steps)+1, command.Name))}
		result.Status = "running"
		result.Steps = append(result.Steps, step)
		if err := s.saveResult(result); err != nil {
			return err
		}
		output, err := os.OpenFile(step.LogPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
		env := mergeEnv(runtimeBase(), append([]string{"CI=1", "ARCHETIPO_ENVIRONMENT_ID=" + s.environmentID(), "ARCHETIPO_ENVIRONMENT_ROOT=" + s.Root, "ARCHETIPO_ENVIRONMENT_STATE=" + s.dir()}, s.env...))
		runErr := s.Runner.Run(stepCtx, s.Root, env, command, output)
		if stepCtx.Err() != nil {
			runErr = stepCtx.Err()
		}
		cancel()
		closeErr := output.Close()
		if closeErr != nil {
			return closeErr
		}
		step.DurationMS = time.Since(step.StartedAt).Milliseconds()
		step.ExitCode = 0
		if runErr != nil {
			step.ExitCode, step.Error = -1, runErr.Error()
			var exit interface{ ExitCode() int }
			if errors.As(runErr, &exit) {
				step.ExitCode = exit.ExitCode()
			}
			result.Passed = false
		}
		step.LogSHA256, err = hashFile(step.LogPath)
		if err != nil {
			return err
		}
		result.Steps[len(result.Steps)-1] = step
		if err = s.saveResult(result); err != nil {
			return err
		}
		if !result.Passed {
			break
		}
	}
	result.Status = "complete"
	return s.saveResult(result)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) cached(path, key string) (Result, bool) {
	return s.cachedEvidence(path, key, 1)
}
func (s *Service) cachedEvidence(path, key string, steps int) (Result, bool) {
	return s.retainedEvidence(path, key, steps, false)
}
func (s *Service) retainedEvidence(path, key string, steps int, allowReuse bool) (Result, bool) {
	var result Result
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &result) != nil || result.EnvironmentID != s.environmentID() || result.Status != "complete" || !result.Passed || !result.Stable || (result.Reused && !allowReuse) || result.InputKey != key || len(result.Steps) != steps {
		return result, false
	}
	for _, step := range result.Steps {
		hash, err := hashFile(step.LogPath)
		if err != nil || hash != step.LogSHA256 || step.ExitCode != 0 {
			return result, false
		}
	}
	return result, true
}

// A shared recipe can be updated from another checkout while a command runs.
// Re-read its effective preparation contract before certifying the result.
// ForOwnedPreview is for a caller already holding preview-environment, such as
// the preview manager or confirmed removal. It never bypasses the lifetime lease.
func (s *Service) ForOwnedPreview() *Service { s.previewOwner = true; return s }

// ForCandidate keeps the shared bundle untouched while testing local recipe files.
// It retains the same checkout leases and resource ownership as normal execution.
func (s *Service) ForCandidate() *Service { s.candidate = true; return s }
func (s *Service) loadRecipe() (Recipe, error) {
	if s.candidate {
		return LoadLocal(s.Root)
	}
	return Load(s.Root)
}
func (s *Service) preparationFingerprint() (string, error) {
	recipe, err := s.loadRecipe()
	if err != nil {
		return "", err
	}
	recipe.Checks, recipe.Cleanup = nil, nil
	return fingerprint(s.Root, recipe, recipe.Inputs, s.env)
}
