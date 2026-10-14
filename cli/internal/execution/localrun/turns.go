package localrun

import (
	"context"
	"errors"
)

type TurnOutcome struct {
	Completed bool
	Final     string
}

// TurnSource is implemented by each harness protocol. Only declared turn
// outcomes may complete work; a process disappearing is a separate event.
type TurnSource interface {
	Turns() <-chan TurnOutcome
	Gone() <-chan struct{}
	Completed() bool
	FinalMessage() string
}

var (
	ErrTurnFailed    = errors.New("the turn ended with an error")
	ErrProcessGone   = errors.New("the process ended")
	ErrRunTerminated = errors.New("the run was stopped")
)

// Converse follows questions and answers until a valid receipt arrives. A
// published receipt wins over simultaneous process exit or deadline expiry.
func Converse(ctx context.Context, source TurnSource, accept func(string) bool) (string, int, error) {
	turns := 0
	drain := func() (string, bool) {
		for {
			select {
			case outcome := <-source.Turns():
				turns++
				if !outcome.Completed {
					return "", false
				}
				if accept(outcome.Final) {
					return outcome.Final, true
				}
			default:
				return "", false
			}
		}
	}
	for {
		select {
		case outcome := <-source.Turns():
			turns++
			if !outcome.Completed {
				return "", turns, ErrTurnFailed
			}
			if accept(outcome.Final) {
				return outcome.Final, turns, nil
			}
		case <-source.Gone():
			if final, ok := drain(); ok {
				return final, turns, nil
			}
			return "", turns, ErrProcessGone
		case <-ctx.Done():
			if final, ok := drain(); ok {
				return final, turns, nil
			}
			return "", turns, ErrRunTerminated
		}
	}
}
