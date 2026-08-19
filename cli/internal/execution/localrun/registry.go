package localrun

import "sync"

// Registry holds the sessions of this process, keyed by run id.
//
// A finished session is deliberately **not** removed. Two things depend on it:
// the history of a run that has ended stays readable, and a command sent to it
// can be refused with the reason that is true — "this run is no longer active"
// — instead of the reason that would be merely convenient, "no such run".
//
// Nothing here is persisted. A local run does not outlive the process that
// started it, so a registry that survived it would only be able to lie about
// runs that no longer exist.
type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewRegistry() *Registry {
	return &Registry{sessions: make(map[string]*Session)}
}

// Register stores a session under its run id, replacing any session previously
// registered under the same id.
func (r *Registry) Register(session *Session) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = make(map[string]*Session)
	}
	r.sessions[session.RunID()] = session
}

// Lookup finds a session by run id.
func (r *Registry) Lookup(runID string) (*Session, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[runID]
	return session, ok
}
