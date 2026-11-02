package web

import (
	"context"
	"sync"
	"time"
)

type incrementPRChecks struct {
	mu   sync.Mutex
	byID map[string]incrementPRCheck
}
type incrementPRCheck struct {
	at      time.Time
	running bool
	err     string
}

func (c *incrementPRChecks) begin(id string, every time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string]incrementPRCheck{}
	}
	last := c.byID[id]
	if last.running || time.Since(last.at) < every {
		return false
	}
	last.running = true
	c.byID[id] = last
	return true
}
func (c *incrementPRChecks) end(id string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	check := incrementPRCheck{at: time.Now()}
	if err != nil {
		check.err = err.Error()
	}
	if c.byID == nil {
		c.byID = map[string]incrementPRCheck{}
	}
	c.byID[id] = check
}
func (c *incrementPRChecks) last(id string) incrementPRCheck {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[id]
}

const incrementPRCheckInterval = time.Minute

// checkIncrementPR reconciles a published increment with GitHub in the
// background, at most once a minute, exactly as the refresh action does.
func (s *Server) checkIncrementPR(ws *workspaceSession, id string) {
	if !ws.prChecks.begin(id, incrementPRCheckInterval) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, err := s.incrementService(ws).Refresh(ctx, id)
		ws.prChecks.end(id, err)
	}()
}
