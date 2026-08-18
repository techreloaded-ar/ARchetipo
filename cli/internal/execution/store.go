package execution

import (
	"context"
	"fmt"
)

type Store interface {
	Create(context.Context, Execution) error
	Update(context.Context, Execution) error
	Get(context.Context, string) (Execution, error)
	// ListBySpec returns the executions of one spec, most recent first by
	// CreatedAt, with the ID breaking ties so the order is deterministic when two
	// records share an instant. A spec with no execution yields an empty slice,
	// never an error and never a nil one: absence is an answer, not a failure.
	//
	// It exists because a client that keeps no identifier — a browser that was
	// just reloaded — must still be able to find the execution it started.
	ListBySpec(ctx context.Context, specCode string) ([]Execution, error)
}

type StoreErrorKind string

const (
	StoreNotFound     StoreErrorKind = "not_found"
	StoreAlreadyExist StoreErrorKind = "already_exists"
	StoreInvalidID    StoreErrorKind = "invalid_id"
)

type StoreError struct {
	Kind StoreErrorKind
	ID   string
	Err  error
}

func (e *StoreError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("execution store %s for %q: %v", e.Kind, e.ID, e.Err)
	}
	return fmt.Sprintf("execution store %s for %q", e.Kind, e.ID)
}

func (e *StoreError) Unwrap() error { return e.Err }
