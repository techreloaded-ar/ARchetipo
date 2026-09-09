package execution

import (
	"encoding/json"
	"fmt"
)

// AcceptActionReceipt applies the acceptance rule of one action, whichever
// action it is, to the message a turn ended on.
//
// It exists for the same reason ActionPrompt does. An action carried out inside
// a native session is chosen at run time, so the caller holds an ActionID and a
// transcript, not a compiled-in call to one of the six Accept* gates. A switch
// written in the viewer would be a second table of which receipt belongs to
// which action, free to drift from the one that renders the prompts.
//
// The three answers are deliberately distinct, because they are three different
// situations:
//   - found=false means the message carries no receipt line at all. For an
//     action running inside a session that is not a failure: the agent has said
//     something and is waiting — a question, a clarification — and the action
//     goes on into the next turn. It is the caller's business to decide when
//     waiting has become failing.
//   - found=true with an error means a receipt was emitted and it does not
//     declare the work that was asked for. That is a closing message, and it
//     closes the action as failed.
//   - found=true with no error is the declaration, returned as the JSON object
//     it was, so the caller can record it without re-encoding a typed value it
//     does not need to understand.
//
// Accepting a receipt is a necessary condition and never a sufficient one: it
// is the agent's word, not an inspection of the workspace. VerifyActionEffect
// is what turns it into a verified success, one layer up, where the connector
// is held.
func AcceptActionReceipt(action ActionID, specCode, message string) (json.RawMessage, bool, error) {
	switch action {
	case ActionPlan:
		receipt, err := AcceptPlanReceipt(message, specCode)
		return encodeReceipt(receiptPresence[PlanReceipt](message, planReceiptFields), receipt, err)
	case ActionImplement:
		receipt, err := AcceptImplementReceipt(message, specCode)
		return encodeReceipt(receiptPresence[ImplementReceipt](message, implementReceiptFields), receipt, err)
	case ActionReview:
		receipt, err := AcceptReviewReceipt(message, specCode)
		return encodeReceipt(receiptPresence[ReviewReceipt](message, reviewReceiptFields), receipt, err)
	case ActionInception:
		receipt, err := AcceptPRDReceipt(message)
		return encodeReceipt(receiptPresence[PRDReceipt](message, prdReceiptFields), receipt, err)
	case ActionBacklog:
		receipt, err := AcceptBacklogReceipt(message)
		return encodeReceipt(receiptPresence[BacklogReceipt](message, backlogReceiptFields), receipt, err)
	case ActionSpecDraft:
		receipt, err := AcceptSpecDraftReceipt(message)
		return encodeReceipt(receiptPresence[SpecDraftReceipt](message, specDraftReceiptFields), receipt, err)
	}
	return nil, false, fmt.Errorf("no receipt is defined for the %q action", action)
}

// receiptPresence reports whether the message carries a line shaped like this
// receipt at all, which is the question that separates "the agent is still
// working" from "the agent closed on something unacceptable". It asks the very
// same extractor the Accept* gates ask, so the two can never disagree about
// which line is the receipt.
func receiptPresence[T any](message string, fields []string) bool {
	_, ok := parseTrailingReceipt[T](message, fields)
	return ok
}

// encodeReceipt turns the (receipt, error) pair every Accept* gate returns into
// the three-valued answer of AcceptActionReceipt, encoding an accepted receipt
// back into the JSON object the record carries.
//
// It takes the presence separately because a gate reports "no receipt" and
// "wrong receipt" through the same error: only the extractor can tell them
// apart.
func encodeReceipt[T any](found bool, receipt T, err error) (json.RawMessage, bool, error) {
	if err != nil {
		return nil, found, err
	}
	encoded, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		return nil, true, fmt.Errorf("encoding the receipt of the action: %w", marshalErr)
	}
	return encoded, true, nil
}
