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
