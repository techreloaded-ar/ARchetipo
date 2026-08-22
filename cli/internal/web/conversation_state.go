package web

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// conversationCloseTimeout bounds how long stopping a workspace waits for the
// provider to release the agent process behind its conversation.
//
// It exists because the close happens on a context of its own: the session's
// context is already cancelled by the time stop() gets there, and a close on a
// cancelled context would ask the provider to release nothing.
const conversationCloseTimeout = 5 * time.Second

// conversationState holds the *one* conversation of a workspace.
//
// One, not a map: a workspace admits a single conversation, and the identity of
// the conversation is therefore the identity of the workspace that opened it.
// Keeping a second one alive would mean keeping a second agent process rooted
// in a directory somebody has to close later, and nothing in the viewer would
// be holding the handle for it.
//
// Every field is read and written under mu, and readers never receive the
// struct itself: current() hands back a copy, so nobody can observe a state
// half-way through an open or a close.
type conversationState struct {
	mu sync.Mutex

	// id is the conversation id the provider registered the session under, and
	// therefore the run id the conversation is read and commanded with.
	id string
	// providerID is the default provider the conversation was opened with. It
	// travels with the conversation because the default can be changed in the
	// Execution panel while the conversation is alive, and the conversation
	// still belongs to the provider that is actually holding the process.
	providerID string
	// provider is what closes the agent process, and collaborator is what reads
	// and commands the session behind it. They are two interfaces rather than
	// one because a conversation borrows the run vocabulary without borrowing
	// the record: only the closing half is conversation-specific.
	provider     execution.Conversationalist
	collaborator execution.RunCollaborator
	// providerConfig is the configuration the conversation was opened with, kept
	// so a later command dispatches with the very configuration that was probed.
	providerConfig map[string]any
	// workingDir is the project root the conversation was opened about. It is a
	// fact of the workspace, not of the provider, which is shared.
	workingDir string
	openedAt   time.Time
	// specCode is the spec the conversation was opened about, empty for a free
	// conversation — which is the default. It lives in the holder because it is
	// a fact of *this* conversation and not of the workspace: the next one may
	// well be about nothing.
	specCode string
	// resumedFrom is the id of the past conversation this one was resumed from,
	// empty for a conversation that started on its own.
	resumedFrom string
	// closed says the holder has been shut down for good. It is not "there is
	// no conversation right now": a stopped session must never accept a new
	// one, because nothing would be left to close it.
	closed bool

	// decidedProposalID is the id of the last event whose action proposal has
	// been decided. It is a watermark and not a flag: a proposal is pending
	// exactly while the last one the history carries is *newer* than this id,
	// which is what makes a decision survive the agent going on talking, and a
	// new proposal become pending again without anything being cleared.
	decidedProposalID int64
	// outcome is what became of that decision, kept so a reader that arrives
	// after it — a reload, a second tab — still learns what was started and
	// under which id. It is a pointer because "nothing has been decided yet" is
	// an answer of its own, not a zero outcome.
	outcome *conversationOutcome
	// outcomes is every decision taken in this conversation, in the order they
	// were taken. It is a list and not a single value because one conversation
	// can start more than one step, and each of them has its own point in the
	// discourse: keeping only the last would lose the place of every earlier
	// one. A second decision on the *same* proposal replaces the entry instead
	// of adding one, because a refusal followed by a confirmation of the same
	// proposal is one gesture, not two.
	outcomes []conversationOutcome
}

// conversationOutcome is the record of the last decision taken on a proposal:
// what was decided, about which proposed action, and — when the decision was to
// confirm — the execution that was born from it.
//
// It carries the label, the scope and the spec of the proposal because the
// event that proposed them may well have been dropped from the retention window
// by the time somebody reads the outcome, and an outcome that could only say
// "confirmed" would then name nothing.
type conversationOutcome struct {
	// ProposalID is the id of the event that carried the decided proposal, so
	// the outcome can be tied back to the exact line of the history.
	ProposalID int64
	// Decision is what a person answered. The vocabulary is the routes' own,
	// never invented here.
	Decision string
	Action   string
	Label    string
	Scope    string
	SpecCode string
	// ExecutionID is the execution the confirmation started, empty when the
	// proposal was refused: a refusal starts nothing, and an id next to one
	// would be a record that does not exist.
	ExecutionID string
}

// conversationSnapshot is the read-only view of the holder at one instant. It
// is a copy on purpose: a caller that held the live struct would be reading
// fields the next open or close is free to rewrite underneath it.
type conversationSnapshot struct {
	id             string
	providerID     string
	provider       execution.Conversationalist
	collaborator   execution.RunCollaborator
	providerConfig map[string]any
	workingDir     string
	openedAt       time.Time
	specCode       string
	resumedFrom    string
	// decidedProposalID and outcome travel in the copy because every reader of
	// the conversation needs them together with its history: what is pending is
	// decided against the watermark, and what was decided is read from the
	// outcome.
	decidedProposalID int64
	outcome           *conversationOutcome
	// outcomes is the whole ordered register of decisions, copied like
	// providerConfig is, so a reader can place every started step at the point
	// of the history that asked for it.
	outcomes []conversationOutcome
}

func newConversationState() *conversationState {
	return &conversationState{}
}

// open records the conversation the provider has just started.
//
// It refuses when one is already open rather than replacing it: replacing would
// silently drop the handle of a live agent process, and the process would then
// outlive everything that could stop it. It also refuses on a holder that has
// been closed, for the same reason runFollowers refuses after closeAll — the
// workspace it belonged to is gone.
func (c *conversationState) open(id, providerID string, provider execution.Conversationalist, collaborator execution.RunCollaborator, providerConfig map[string]any, workingDir string, openedAt time.Time, specCode, resumedFrom string) error {
	if c == nil {
		return fmt.Errorf("this workspace cannot hold a conversation")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("a conversation needs an id")
	}
	if provider == nil {
		return fmt.Errorf("a conversation needs a provider that can close it")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("this workspace is no longer open")
	}
	if c.id != "" {
		return fmt.Errorf("conversation %s is already open for this workspace", c.id)
	}
	c.id = id
	c.providerID = providerID
	c.provider = provider
	c.collaborator = collaborator
	c.providerConfig = providerConfig
	c.workingDir = workingDir
	c.openedAt = openedAt
	c.specCode = specCode
	c.resumedFrom = resumedFrom
	// A new conversation is a new story, and it starts with nothing decided. An
	// inherited watermark would silently mark as already decided a proposal this
	// conversation has never made — the first one it makes would arrive with an
	// id below the watermark and would simply never be pending — and an
	// inherited outcome would show, next to a fresh history, the result of a
	// decision taken about a conversation that no longer exists.
	c.decidedProposalID = 0
	c.outcome = nil
	c.outcomes = nil
	return nil
}

// decide records that the proposal carried by proposalID has been answered.
//
// It refuses on an empty or closed holder rather than recording anyway: a
// decision belongs to one conversation, and there is no conversation to attach
// it to in either state. Both fields are written under the same lock, so no
// reader can ever observe a watermark that has moved without the outcome that
// explains it.
func (c *conversationState) decide(proposalID int64, outcome conversationOutcome) error {
	if c == nil {
		return fmt.Errorf("this workspace cannot hold a conversation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("this workspace is no longer open")
	}
	if c.id == "" {
		return fmt.Errorf("no conversation is open for this workspace")
	}
	c.decidedProposalID = proposalID
	// The register keeps one entry per proposal. A decision on a proposal that
	// already has one replaces it — the watermark makes that happen only for a
	// refusal answered again by a confirmation of the same proposal, and two
	// entries for one proposal would be two blocks for a single gesture.
	replaced := false
	for i := range c.outcomes {
		if c.outcomes[i].ProposalID == outcome.ProposalID {
			c.outcomes[i] = outcome
			replaced = true
			break
		}
	}
	if !replaced {
		c.outcomes = append(c.outcomes, outcome)
	}
	// outcome keeps pointing at the last decision taken, exactly as before. It
	// is a copy and not the address of the entry in the register: a reader that
	// received the pointer would otherwise be dereferencing a value a later
	// decision on the same proposal is free to rewrite underneath it.
	last := outcome
	c.outcome = &last
	return nil
}

// anchorOf answers with the id of the event that carried the proposal whose
// confirmation started executionID — the point of the history where that step
// was asked for.
//
// It answers false for an empty id, for a closed or empty holder, and for an
// execution this conversation never started: none of those has a point in this
// discourse, and a zero anchor would claim the top of the history.
func (c *conversationState) anchorOf(executionID string) (int64, bool) {
	if c == nil || strings.TrimSpace(executionID) == "" {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.id == "" {
		return 0, false
	}
	for i := range c.outcomes {
		if c.outcomes[i].ExecutionID == executionID {
			return c.outcomes[i].ProposalID, true
		}
	}
	return 0, false
}

// current returns a copy of the open conversation, and false when there is
// none. Absence is an answer here, not a failure: a workspace that has never
// opened a conversation is the ordinary case.
func (c *conversationState) current() (conversationSnapshot, bool) {
	if c == nil {
		return conversationSnapshot{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == "" {
		return conversationSnapshot{}, false
	}
	// The register travels as a copy for the same reason the snapshot itself is
	// one: the caller must not be holding the slice the next decision appends
	// to or rewrites.
	var outcomes []conversationOutcome
	if len(c.outcomes) > 0 {
		outcomes = make([]conversationOutcome, len(c.outcomes))
		copy(outcomes, c.outcomes)
	}
	return conversationSnapshot{
		id:                c.id,
		providerID:        c.providerID,
		provider:          c.provider,
		collaborator:      c.collaborator,
		providerConfig:    c.providerConfig,
		workingDir:        c.workingDir,
		openedAt:          c.openedAt,
		specCode:          c.specCode,
		resumedFrom:       c.resumedFrom,
		decidedProposalID: c.decidedProposalID,
		outcome:           c.outcome,
		outcomes:          outcomes,
	}, true
}

// close releases the agent process behind the conversation and empties the
// holder. It is idempotent: closing when there is nothing open succeeds and
// releases nothing, exactly like CloseConversation itself.
//
// The state is emptied whatever the provider answers. A conversation the viewer
// kept after a failed close would be a handle on a process the viewer can no
// longer command, and the error is reported to the caller rather than hidden.
func (c *conversationState) close(ctx context.Context) error {
	return c.releaseCurrent(ctx, false)
}

// releaseCurrent empties the holder and releases what was in it, marking the
// holder closed for good when markClosed is set.
//
// Emptying and marking happen under the *same* lock, and that is the whole
// reason this is one function and not two: releasing the process is slow — for
// a real agent it is a cancel plus a process wait — and a shutdown that marked
// `closed` only afterwards would leave a window in which the holder reads as
// "not closed, nothing open", which is precisely the state an open request
// accepts. A conversation installed in that window would belong to a session
// that is already gone, with nothing left to close it.
func (c *conversationState) releaseCurrent(ctx context.Context, markClosed bool) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	id := c.id
	provider := c.provider
	c.id = ""
	c.providerID = ""
	c.provider = nil
	c.collaborator = nil
	c.providerConfig = nil
	c.workingDir = ""
	c.openedAt = time.Time{}
	// Same reason the fields above are cleared: they described the conversation
	// being released, and a spec code left behind would bind the next one to a
	// spec nobody named.
	c.specCode = ""
	c.resumedFrom = ""
	// Same reason open clears them: the decision belonged to the conversation
	// being released. Left behind, the watermark would make the first proposal
	// of the next conversation look already decided, and the outcome would
	// describe a run started from a history nobody can read here any more.
	c.decidedProposalID = 0
	c.outcome = nil
	c.outcomes = nil
	if markClosed {
		c.closed = true
	}
	c.mu.Unlock()
	if id == "" || provider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return provider.CloseConversation(ctx, id)
}

// shutdown closes the conversation for good: after it, open refuses. It is what
// a session that is ending calls, so a request still in flight cannot install a
// conversation nobody is left to close.
func (c *conversationState) shutdown(ctx context.Context) error {
	return c.releaseCurrent(ctx, true)
}
