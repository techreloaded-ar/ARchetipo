package execution

import "context"

// ConversationRequest asks a provider to open one free conversation about a
// directory. It is deliberately not a Request: a conversation is not an action
// of the process, has no spec, no capability to satisfy and no execution record
// behind it, so it carries only the three facts a provider needs to start.
type ConversationRequest struct {
	// ConversationID is the id under which the provider registers the session.
	// It is therefore the run id the conversation will later be read and
	// commanded with, through the very same RunCollaborator methods a run uses:
	// a conversation borrows the run vocabulary without borrowing the record.
	ConversationID string `json:"conversation_id"`
	// WorkingDir is the project root of the open workspace. It travels on the
	// request and not on the provider for the same reason Request.WorkingDir
	// does: the provider is shared by every workspace a long-lived process
	// serves, while where a conversation has to execute is a fact of the
	// workspace that opened it.
	WorkingDir string `json:"working_dir,omitempty"`
	// ProviderConfig is the non-secret configuration of the provider for this
	// workspace, exactly as it travels on an execution request.
	ProviderConfig map[string]any `json:"provider_config,omitempty"`
	// ProcessActions is the vocabulary of the process of this workspace: the
	// steps it offers, in the words of the process itself.
	//
	// It travels on the request because the provider does not know the process
	// and must never learn it. An agent that may propose an action has to name
	// one that exists, and the only way to keep invented ids out of a proposal
	// without teaching the provider the process is to hand it the list. An
	// empty list is a legitimate state and means the agent has nothing to
	// propose.
	ProcessActions []ConversationAction `json:"process_actions,omitempty"`
}

// ConversationAction is one step of the process as a conversation agent sees
// it: the id a proposal has to name, the label a person reads, and the scope
// that says whether the step is about a spec or about the workspace as a whole.
//
// It is deliberately a type of this package and not of the process one: what
// travels to a provider is the vocabulary, not the model behind it — no skill,
// no admissible statuses, nothing a provider could act on. Naming an action is
// all a proposal ever does.
type ConversationAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Scope string `json:"scope"`
}

// Conversationalist is implemented by providers that can hold a free
// conversation about a directory. It is deliberately optional and separate from
// Provider, like ConfigDescriber and RunCollaborator: Provider is a stable
// contract and adding a method to it would break every existing implementation
// for the sake of one caller.
type Conversationalist interface {
	// OpenConversation starts the agent process, makes the conversation
	// followable under ConversationRequest.ConversationID and returns. It does
	// not block: a conversation has no outcome to wait for, and a caller that
	// waited for one would wait until somebody closed it.
	OpenConversation(ctx context.Context, req ConversationRequest) error
	// CloseConversation releases the agent process behind the conversation. It
	// is idempotent: closing a conversation that is already closed, or one that
	// was never open, succeeds and releases nothing.
	CloseConversation(ctx context.Context, conversationID string) error
}

// ConversationalistFor discovers the optional capability on a provider. It
// returns (nil, false) for a provider that cannot converse and for a nil
// provider, so a caller never has to test for nil before asking.
func ConversationalistFor(provider Provider) (Conversationalist, bool) {
	if provider == nil {
		return nil, false
	}
	conversationalist, ok := provider.(Conversationalist)
	if !ok {
		return nil, false
	}
	return conversationalist, true
}
