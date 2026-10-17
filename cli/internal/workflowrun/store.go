// Package workflowrun persists agent-owned checkpoints. It neither interprets
// a skill's state nor dispatches workers or workflow phases.
package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

const (
	Running    = "running"
	Recovering = "recovering"
	Blocked    = "blocked"
	Complete   = "complete"
)

type Record struct {
	ID                  string          `json:"id"`
	Skill               string          `json:"skill"`
	Invocation          string          `json:"invocation"`
	Scope               string          `json:"scope"`
	ConversationID      string          `json:"conversation_id,omitempty"`
	Status              string          `json:"status"`
	Phase               string          `json:"phase,omitempty"`
	WorkerID            string          `json:"worker_id,omitempty"`
	State               json.RawMessage `json:"state"`
	Revision            int             `json:"revision"`
	RecoveryAttempts    int             `json:"recovery_attempts"`
	MaxRecoveryAttempts int             `json:"max_recovery_attempts"`
	RecoveryTurnID      string          `json:"recovery_turn_id,omitempty"`
	LastError           string          `json:"last_error,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type Start struct {
	Skill          string          `json:"skill"`
	Invocation     string          `json:"invocation"`
	Scope          string          `json:"scope"`
	ConversationID string          `json:"conversation_id,omitempty"`
	State          json.RawMessage `json:"state"`
}

type Checkpoint struct {
	Revision  int             `json:"revision"`
	Status    string          `json:"status"`
	Phase     string          `json:"phase,omitempty"`
	WorkerID  string          `json:"worker_id,omitempty"`
	State     json.RawMessage `json:"state"`
	LastError string          `json:"last_error,omitempty"`
}

type Store struct{ root string }

func New(root string) *Store { return &Store{root: root} }
func (s *Store) dir() string { return filepath.Join(s.root, ".archetipo", "workflow-runs") }

var skillName = regexp.MustCompile(`^archetipo-[a-z][a-z0-9-]*$`)

func (s *Store) Read(id string) (Record, error) {
	var r Record
	if !recordfile.ValidID(id) || !strings.HasPrefix(id, "flow-") {
		return r, iox.NewInvalidInput("invalid workflow run id", "use the id returned by execution workflow start", nil)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir(), id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return r, iox.NewNotFound("workflow run not found: "+id, "list existing workflow runs", err)
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, iox.NewInternal("reading workflow run "+id, err)
	}
	if r.ID != id || !skillName.MatchString(r.Skill) || !validInvocation(r.Skill, r.Invocation) || !object(r.State) || r.Revision < 1 || r.RecoveryAttempts < 0 || r.MaxRecoveryAttempts < 1 {
		return r, iox.NewInternal("invalid workflow run "+id, nil)
	}
	if r.Status != Running && r.Status != Recovering && r.Status != Blocked && r.Status != Complete {
		return r, iox.NewInternal("invalid workflow run status: "+id, nil)
	}
	return r, nil
}

func (s *Store) List() ([]Record, error) {
	out := []Record{}
	entries, err := os.ReadDir(s.dir())
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := s.Read(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) Start(ctx context.Context, input Start) (Record, error) {
	var r Record
	if !skillName.MatchString(input.Skill) || !validInvocation(input.Skill, input.Invocation) || strings.TrimSpace(input.Scope) == "" || strings.ContainsAny(input.Scope, "\r\n") || !object(input.State) {
		return r, iox.NewInvalidInput("skill, invocation, single-line scope and state object are required", "supply the controller skill, executable invocation, normalized scope and initial checkpoint", nil)
	}
	err := s.locked(ctx, func() error {
		all, err := s.List()
		if err != nil {
			return err
		}
		for _, existing := range all {
			if existing.Skill != input.Skill || existing.Status == Complete {
				continue
			}
			if existing.Scope != input.Scope || existing.ConversationID != input.ConversationID {
				return iox.NewConflict("another workflow run is unfinished: "+existing.ID, "resume that run before changing scope or conversation", nil)
			}
			r = existing
			return nil
		}
		id, err := execution.RandomID()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		r = Record{ID: "flow-" + strings.TrimPrefix(id, "exec-"), Skill: input.Skill, Invocation: input.Invocation, Scope: input.Scope, ConversationID: input.ConversationID, Status: Running, State: input.State, Revision: 1, MaxRecoveryAttempts: 3, CreatedAt: now, UpdatedAt: now}
		return s.save(r)
	})
	return r, err
}

func (s *Store) Update(ctx context.Context, id string, input Checkpoint) (Record, error) {
	var r Record
	err := s.locked(ctx, func() error {
		var err error
		r, err = s.Read(id)
		if err != nil {
			return err
		}
		if input.Revision != r.Revision || r.Status == Complete {
			return iox.NewConflict("workflow checkpoint is stale or complete", "reread the run before updating it", nil)
		}
		if (input.Status != Running && input.Status != Blocked && input.Status != Complete) || !object(input.State) {
			return iox.NewInvalidInput("invalid workflow checkpoint", "use running, blocked or complete and a state object", nil)
		}
		r.Status, r.Phase, r.WorkerID, r.State, r.LastError = input.Status, input.Phase, input.WorkerID, input.State, input.LastError
		return s.saveNext(&r)
	})
	return r, err
}

// Recover reserves one bounded continuation for an observed failed native turn.
// Retrying that observation is idempotent. The caller must first reconcile the
// native delivery and ensure no previous worker is still writing.
func (s *Store) Recover(ctx context.Context, id, turnID, reason string) (Record, error) {
	var r Record
	err := s.locked(ctx, func() error {
		var err error
		r, err = s.Read(id)
		if err != nil {
			return err
		}
		if r.Status == Complete || r.Status == Blocked || r.RecoveryTurnID == turnID {
			return nil
		}
		if turnID == "" {
			return iox.NewInvalidInput("failed turn id is required", "reconcile the native turn before recovery", nil)
		}
		r.RecoveryTurnID, r.LastError = turnID, reason
		if r.RecoveryAttempts >= r.MaxRecoveryAttempts {
			r.Status = Blocked
		} else {
			r.RecoveryAttempts++
			r.Status = Recovering
		}
		return s.saveNext(&r)
	})
	return r, err
}

func object(raw json.RawMessage) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(raw, &v) == nil && v != nil
}

func validInvocation(skill, invocation string) bool {
	return !strings.ContainsAny(invocation, "\r\n") && (invocation == "/"+skill || strings.HasPrefix(invocation, "/"+skill+" "))
}
func (s *Store) save(r Record) error {
	return recordfile.WriteAtomic(s.dir(), filepath.Join(s.dir(), r.ID+".json"), ".workflow-*.tmp", r)
}
func (s *Store) saveNext(r *Record) error {
	r.Revision++
	r.UpdatedAt = time.Now().UTC()
	return s.save(*r)
}
func (s *Store) locked(ctx context.Context, fn func() error) error {
	store, err := conversationlog.NewFileStore(s.root)
	if err != nil {
		return err
	}
	lock, err := store.Lock(ctx, "workflow-runs")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}
