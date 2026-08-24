package web

// A run started from inside a conversation belongs to it whether or not the
// agent proposed it.
//
// Until this file existed, only one gesture tied a run to a thread: confirming
// a proposal the agent had made. The other one — pressing the recommended step
// at the tail of the thread — went through the board's own start route, which
// knows nothing of conversations, so the run it started was born anchored to
// nothing: absent from the conversation that asked for it, and reachable only
// from the workspace strip. The person who pressed it in a thread was then
// reading a thread that did not mention it.
//
// What is added here is only the tie. The start itself stays where it was, in
// startSpecAction and startWorkspaceAction, and this file runs *after* it: an
// adoption that failed must never be able to unstart, duplicate or refuse a run
// that already exists.

import (
	"context"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// conversationAnchorOf is the last event said in a conversation, which is the
// point of its history a gesture pressed now belongs to.
//
// A conversation whose transcript cannot be read answers 0, and so does one in
// which nothing has been said yet: both mean "before anything", which is where
// the flow already draws a block whose anchor precedes every event.
func conversationAnchorOf(snapshot conversationSnapshot) int64 {
	session, found := conversationSessionOf(snapshot)
	if !found {
		return 0
	}
	events := session.Events(0)
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].ID
}

// adoptStartedRun ties a run just started to the conversation it was started
// from.
//
// An empty id is not an attempt: a start from the board carries none, and is
// anchored to nothing on purpose. Every failure is returned and none is ever
// raised into the response: the execution exists and is running by the time
// this runs, so refusing the answer that announces it would leave the person
// with a run they were never told about — the very fault this file exists to
// remove. A run that could not be tied is still the workspace's, and the runs
// strip still shows it.
func (s *Server) adoptStartedRun(
	ctx context.Context,
	ws *workspaceSession,
	conversationID string,
	started *execution.Execution,
) error {
	id := strings.TrimSpace(conversationID)
	if id == "" || started == nil {
		return nil
	}
	snapshot, live := ws.conversation.get(id)
	if !live {
		return fmt.Errorf("the conversation %s is no longer open for this workspace", id)
	}
	outcome := conversationOutcome{
		Decision:    conversationDecisionConfirmed,
		Action:      string(started.Action),
		Label:       s.actionLabelOf(ws, started.Action, started.SpecCode),
		Scope:       template.ScopeWorkspace,
		SpecCode:    started.SpecCode,
		ExecutionID: started.ID,
	}
	if outcome.SpecCode != "" {
		outcome.Scope = template.ScopeSpec
	}
	if err := ws.conversation.adopt(id, conversationAnchorOf(snapshot), outcome); err != nil {
		return err
	}
	// The thread's own label follows the work, exactly as it does after a
	// confirmed proposal, and the record the index reads is written with it.
	adopted, _ := ws.conversation.get(id)
	_ = ws.journal.retarget(ctx, id, adopted.specCode)
	return nil
}

// actionLabelOf is the process's own word for an action, or the action id when
// the process does not name it. It reads the spec-scoped table for a run on a
// spec and the workspace-scoped one otherwise, which is the same split every
// other reader of the template makes.
func (s *Server) actionLabelOf(ws *workspaceSession, action execution.ActionID, specCode string) string {
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		return string(action)
	}
	if strings.TrimSpace(specCode) != "" {
		for _, candidate := range tpl.Actions {
			if candidate.ID == string(action) {
				return candidate.Label
			}
		}
		return string(action)
	}
	for _, candidate := range tpl.WorkspaceActions {
		if candidate.ID == string(action) {
			return candidate.Label
		}
	}
	return string(action)
}
