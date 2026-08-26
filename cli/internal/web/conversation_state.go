package web

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// conversationCloseTimeout bounds how long stopping a workspace waits for the
// provider to release the agent processes behind its conversations.
//
// It exists because the close happens on a context of its own: the session's
// context is already cancelled by the time stop() gets there, and a close on a
// cancelled context would ask the provider to release nothing.
const conversationCloseTimeout = 5 * time.Second

// There is no ceiling on how many conversations a workspace may hold alive.
//
// There used to be one — three — and it was a guess dressed as a rule: it was
// meant to keep a person from running several agents over the same working
// directory without noticing, but nobody ever chose the number, and the cost it
// was guarding against is one the person is already choosing every time they
// open a thread. What the ceiling did instead was refuse: it turned "open one
// more" into an error to be read and a thread to be closed first, and once a
// step that finds no thread opens its own — see adoptStartedRun — that refusal
// would have started falling on runs nobody had asked to be refused.
//
// What replaces it is nothing at all. A workspace holds what it has been asked
// to hold, the index says how many that is, and closing a thread stays the
// person's gesture and not a toll the viewer collects.

// liveConversation is one conversation the workspace is holding right now.
//
// Everything that used to be a flat field of the holder lives here instead, and
// the decision watermark most of all: while decidedProposalID belonged to the
// holder, a proposal decided in one conversation marked as already decided the
// next proposal of another one, which is exactly the mixing a workspace with
// several live conversations must not do.
type liveConversation struct {
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
	// conversation — which is the default. It is a fact of *this* conversation
	// and not of the workspace: the one next to it may well be about nothing.
	specCode string
	// resumedFrom is the id of the past conversation this one was resumed from,
	// empty for a conversation that started on its own.
	resumedFrom string

	// decidedProposalID is the id of the last event of *this* conversation whose
	// action proposal has been decided. It is a watermark and not a flag: a
	// proposal is pending exactly while the last one the history carries is
	// *newer* than this id, which is what makes a decision survive the agent
	// going on talking, and a new proposal become pending again without anything
	// being cleared.
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

// conversationSet holds the conversations a workspace has alive.
//
// A map and no longer a single value: a person carrying two specs forward talks
// about one while an agent is still working on the other, and closing the first
// to open the second — which is what the singular holder forced the browser to
// do — threw away both the history and the work in flight. It is unbounded for
// the reason written where the old ceiling used to be.
//
// Every field is read and written under mu, and readers never receive the
// entries themselves: get() and list() hand back copies, so nobody can observe
// a conversation half-way through an open or a close.
type conversationSet struct {
	mu sync.Mutex

	// live is the conversations being held, keyed by conversation id.
	live map[string]*liveConversation
	// closed says the holder has been shut down for good. It is not "there are
	// no conversations right now": a stopped session must never accept a new
	// one, because nothing would be left to close it.
	closed bool
}

func newConversationSet() *conversationSet {
	return &conversationSet{live: map[string]*liveConversation{}}
}

// canOpen answers whether another conversation could be opened right now, and
// with which refusal when it could not.
//
// Now that no ceiling exists there is one refusal left, and it is the one that
// always mattered: a holder that has been shut down. It is asked *before* the
// provider is told to start an agent process, because a workspace that has been
// left would otherwise be handed a process nobody is left to close. It is not a
// guarantee — the holder can be shut down between this answer and the open —
// which is why open() asks again and the routes keep their recovery branch.
func (c *conversationSet) canOpen() error {
	if c == nil {
		return fmt.Errorf("this workspace cannot hold a conversation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("this workspace is no longer open")
	}
	return nil
}

// open records a conversation the provider has just started.
//
// It refuses an id it is already holding rather than replacing it: replacing
// would silently drop the handle of a live agent process, and the process would
// then outlive everything that could stop it. It refuses on a holder that has
// been closed for the same reason runFollowers refuses after closeAll — the
// workspace it belonged to is gone. It refuses for no other reason: how many
// are already alive is not one.
func (c *conversationSet) open(id, providerID string, provider execution.Conversationalist, collaborator execution.RunCollaborator, providerConfig map[string]any, workingDir string, openedAt time.Time, specCode, resumedFrom string) error {
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
	if c.live == nil {
		c.live = map[string]*liveConversation{}
	}
	if _, held := c.live[id]; held {
		return fmt.Errorf("conversation %s is already open for this workspace", id)
	}
	// A new conversation is a new story, and it starts with nothing decided: the
	// watermark, the outcome and the register are zero here and are never
	// inherited from a sibling, which is what keeps a decision taken in one
	// conversation out of every other one.
	c.live[id] = &liveConversation{
		id:                id,
		providerID:        providerID,
		provider:          provider,
		collaborator:      collaborator,
		providerConfig:    providerConfig,
		workingDir:        workingDir,
		openedAt:          openedAt,
		specCode:          specCode,
		resumedFrom:       resumedFrom,
		decidedProposalID: 0,
		outcome:           nil,
		outcomes:          nil,
	}
	return nil
}

// liveIDsLocked is the ids of the live conversations in the order list() uses,
// so every reader that walks the set walks it in the same order.
func (c *conversationSet) liveIDsLocked() []string {
	entries := make([]*liveConversation, 0, len(c.live))
	for _, entry := range c.live {
		entries = append(entries, entry)
	}
	sortLiveConversations(entries)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.id)
	}
	return ids
}

// sortLiveConversations orders by the moment of opening and, for two opened in
// the same instant, by id. The tie-break is not decoration: without it two
// successive reads of the same set could contradict each other, and the rail
// would reorder under the person reading it.
func sortLiveConversations(entries []*liveConversation) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].openedAt.Equal(entries[j].openedAt) {
			return entries[i].id < entries[j].id
		}
		return entries[i].openedAt.Before(entries[j].openedAt)
	})
}

// decide records that the proposal carried by proposalID has been answered in
// the conversation named by id.
//
// It refuses on a closed holder and on an id that is not alive rather than
// recording anyway: a decision belongs to one conversation, and there is no
// conversation to attach it to in either state. Both fields are written under
// the same lock, so no reader can ever observe a watermark that has moved
// without the outcome that explains it.
func (c *conversationSet) decide(id string, proposalID int64, outcome conversationOutcome) error {
	if c == nil {
		return fmt.Errorf("this workspace cannot hold a conversation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("this workspace is no longer open")
	}
	entry, held := c.live[id]
	if !held {
		return fmt.Errorf("the conversation %s is not open for this workspace", id)
	}
	entry.decidedProposalID = proposalID
	// The register keeps one entry per proposal. A decision on a proposal that
	// already has one replaces it — the watermark makes that happen only for a
	// refusal answered again by a confirmation of the same proposal, and two
	// entries for one proposal would be two blocks for a single gesture.
	replaced := false
	for i := range entry.outcomes {
		if entry.outcomes[i].ProposalID == outcome.ProposalID {
			entry.outcomes[i] = outcome
			replaced = true
			break
		}
	}
	if !replaced {
		entry.outcomes = append(entry.outcomes, outcome)
	}
	// outcome keeps pointing at the last decision taken, exactly as before. It
	// is a copy and not the address of the entry in the register: a reader that
	// received the pointer would otherwise be dereferencing a value a later
	// decision on the same proposal is free to rewrite underneath it.
	last := outcome
	entry.outcome = &last
	// The spec of the conversation follows the work it has actually started.
	// The code it was opened with is where it began, not what it is about
	// forever: a conversation opened on one card and then used to plan another
	// would otherwise keep being labelled — and grouped — by a spec it never
	// touched, while its title already says which one it did.
	//
	// Only a confirmation moves it, because only a confirmation is work: a
	// refusal started nothing, and letting it retarget the thread would rename
	// it after a step nobody took.
	retargetLocked(entry, outcome)
	return nil
}

// retargetLocked moves the conversation onto the spec an outcome worked on, with
// mu already held. It is a function of its own because two gestures start work
// in a conversation — confirming a proposal and pressing the recommended step —
// and the thread has to be labelled the same way by both.
func retargetLocked(entry *liveConversation, outcome conversationOutcome) {
	if outcome.Decision != conversationDecisionConfirmed {
		return
	}
	if code := strings.TrimSpace(outcome.SpecCode); code != "" {
		entry.specCode = code
	}
}

// adopt records that executionID was started *from* this conversation by a
// gesture that carried no proposal — the recommended step, pressed at the tail
// of the thread — anchored at the last event said before the press.
//
// It is not decide: nothing was proposed, so nothing was decided, and moving the
// watermark here would silently answer a proposal the agent may have just made
// and the person has not read. The register is written and the watermark is
// left exactly where it was.
//
// The outcome is appended and never replaces one already filed under the same
// anchor: two things really can be asked for at the same point of a history —
// a proposal confirmed on the last event, and a step pressed right after it —
// and each one started a run of its own that the thread has to keep showing.
// An execution already adopted is ignored, so a retried request adds nothing.
func (c *conversationSet) adopt(id string, anchorEventID int64, outcome conversationOutcome) error {
	if c == nil {
		return fmt.Errorf("this workspace cannot hold a conversation")
	}
	if strings.TrimSpace(outcome.ExecutionID) == "" {
		return fmt.Errorf("an adopted run needs an execution id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("this workspace is no longer open")
	}
	entry, held := c.live[id]
	if !held {
		return fmt.Errorf("the conversation %s is not open for this workspace", id)
	}
	outcome.ProposalID = anchorEventID
	for i := range entry.outcomes {
		if entry.outcomes[i].ExecutionID == outcome.ExecutionID {
			return nil
		}
	}
	entry.outcomes = append(entry.outcomes, outcome)
	// The last decision of the conversation is what it last set going, and this
	// gesture set something going: leaving `outcome` pointing at an older one
	// would make the thread report a step that has been superseded.
	last := outcome
	entry.outcome = &last
	retargetLocked(entry, outcome)
	return nil
}

// anchorOf answers with the conversation that started executionID and the id of
// the event that carried the proposal whose confirmation started it — the point
// of that conversation's history where the step was asked for.
//
// It names the conversation and not only the anchor because with several live
// conversations "which conversation is that anchor in" has a different answer
// for each of them, and a run rail that navigated to "the" conversation would
// be navigating to whichever one it happened to read.
//
// It answers false for an empty id, for a closed holder, and for an execution no
// live conversation started: none of those has a point in any discourse here,
// and a zero anchor would claim the top of a history.
func (c *conversationSet) anchorOf(executionID string) (string, int64, bool) {
	if c == nil || strings.TrimSpace(executionID) == "" {
		return "", 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", 0, false
	}
	for _, id := range c.liveIDsLocked() {
		entry := c.live[id]
		for i := range entry.outcomes {
			if entry.outcomes[i].ExecutionID == executionID {
				return entry.id, entry.outcomes[i].ProposalID, true
			}
		}
	}
	return "", 0, false
}

// get returns a copy of one live conversation, and false when the workspace is
// not holding it. Absence is an answer here, not a failure: an id that is not
// alive may perfectly well be a conversation that ended and lives on disk, and
// deciding that is the caller's business.
func (c *conversationSet) get(id string) (conversationSnapshot, bool) {
	if c == nil {
		return conversationSnapshot{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, held := c.live[id]
	if !held {
		return conversationSnapshot{}, false
	}
	return snapshotOf(entry), true
}

// list returns every live conversation, oldest first.
//
// The order is fixed rather than the map's own so two successive reads cannot
// contradict each other: an index whose order changed between polls would move
// the threads under the hand of whoever is clicking one.
func (c *conversationSet) list() []conversationSnapshot {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]*liveConversation, 0, len(c.live))
	for _, entry := range c.live {
		entries = append(entries, entry)
	}
	sortLiveConversations(entries)
	snapshots := make([]conversationSnapshot, 0, len(entries))
	for _, entry := range entries {
		snapshots = append(snapshots, snapshotOf(entry))
	}
	return snapshots
}

// snapshotOf copies one entry out of the set, with mu held.
//
// The register travels as a copy for the same reason the snapshot itself is
// one: the caller must not be holding the slice the next decision appends to or
// rewrites.
func snapshotOf(entry *liveConversation) conversationSnapshot {
	var outcomes []conversationOutcome
	if len(entry.outcomes) > 0 {
		outcomes = make([]conversationOutcome, len(entry.outcomes))
		copy(outcomes, entry.outcomes)
	}
	return conversationSnapshot{
		id:                entry.id,
		providerID:        entry.providerID,
		provider:          entry.provider,
		collaborator:      entry.collaborator,
		providerConfig:    entry.providerConfig,
		workingDir:        entry.workingDir,
		openedAt:          entry.openedAt,
		specCode:          entry.specCode,
		resumedFrom:       entry.resumedFrom,
		decidedProposalID: entry.decidedProposalID,
		outcome:           entry.outcome,
		outcomes:          outcomes,
	}
}

// closeOne releases the agent process behind one conversation and drops it from
// the set, leaving every other one exactly as it was.
//
// It is idempotent: closing an id the workspace is not holding succeeds and
// releases nothing, exactly like CloseConversation itself.
//
// The entry is dropped whatever the provider answers. A conversation the viewer
// kept after a failed close would be a handle on a process the viewer can no
// longer command, and the error is reported to the caller rather than hidden.
// The provider is asked *outside* the lock, because releasing a real agent is a
// cancel plus a process wait and holding mu across it would stop every other
// conversation of the workspace from being read.
func (c *conversationSet) closeOne(ctx context.Context, id string) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	entry, held := c.live[id]
	if held {
		delete(c.live, id)
	}
	c.mu.Unlock()
	if !held || entry.provider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return entry.provider.CloseConversation(ctx, entry.id)
}

// shutdown closes every conversation for good: after it, open refuses. It is
// what a session that is ending calls, so a request still in flight cannot
// install a conversation nobody is left to close.
//
// Emptying the map and marking the holder closed happen under the *same* lock,
// and that is the whole reason they are one step: releasing the processes is
// slow, and a shutdown that marked `closed` only afterwards would leave a window
// in which the holder reads as "not closed, room for another", which is exactly
// the state an open request accepts.
//
// It goes on after a provider that answers badly and joins the errors, because
// a process left alive because the previous one failed to close is precisely
// what closing a workspace must never leave behind.
func (c *conversationSet) shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	entries := make([]*liveConversation, 0, len(c.live))
	for _, id := range c.liveIDsLocked() {
		entries = append(entries, c.live[id])
	}
	c.live = map[string]*liveConversation{}
	c.closed = true
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for _, entry := range entries {
		if entry.provider == nil {
			continue
		}
		if err := entry.provider.CloseConversation(ctx, entry.id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// conversationOutcome is the record of a decision taken on a proposal: what was
// decided, about which proposed action, and — when the decision was to confirm
// — the execution that was born from it.
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

// conversationSnapshot is the read-only view of one live conversation at one
// instant. It is a copy on purpose: a caller that held the live struct would be
// reading fields the next open or close is free to rewrite underneath it.
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
