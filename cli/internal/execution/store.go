package execution

import (
	"context"
	"fmt"
)

type Store interface {
	Create(context.Context, Execution) error
	Update(context.Context, Execution) error
	Get(context.Context, string) (Execution, error)
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
