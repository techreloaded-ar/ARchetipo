// Package conversationlog is the on-disk home of the conversations held on a
// workspace. It is deliberately the same shape as internal/execution's record
// store — one small JSON file per record, read by scanning a directory — because
// it is the same kind of problem, and a second shape would be a second thing to
// learn for no gain.
package conversationlog

import (
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// Record is the whole history of one conversation, as it survives the process
// that held it. Events are kept as the execution.RunEvent they already are,
// with no translation: the viewer redraws a past conversation with the same
// renderer that draws the live one, so a past transcript cannot drift from a
// live one by way of a second, parallel implementation.
//
// SpecCode is empty for a free conversation, which is the default: a
// conversation is bound to a spec only when someone asked for that binding.
// ResumedFrom carries the id of the conversation this one was resumed from, and
// is empty for a conversation that started on its own.
//
// FinalState is the state the conversation was left in when it was released. It
// is history, not liveness: whether a record is the conversation running right
// now is decided by the process that holds it, never read back from disk, so a
// restart cannot resurrect a conversation that no longer exists.
type Record struct {
	ID            string               `json:"id"`
	SpecCode      string               `json:"spec_code"`
	Title         string               `json:"title"`
	WorkingDir    string               `json:"working_dir"`
	ProviderID    string               `json:"provider_id"`
	OpenedAt      time.Time            `json:"opened_at"`
	LastMessageAt time.Time            `json:"last_message_at"`
	MessageCount  int                  `json:"message_count"`
	ResumedFrom   string               `json:"resumed_from"`
	FinalState    string               `json:"final_state"`
	Events        []execution.RunEvent `json:"events"`
}
