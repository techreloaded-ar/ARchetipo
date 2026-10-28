package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// askUserQuestionTool is the built-in tool Claude Code calls to ask the person
// a question with options. With `--permission-prompt-tool stdio` the call
// reaches the host as an ordinary `can_use_tool` request, and the answers
// travel back inside the allow decision as `updatedInput.answers`, keyed by
// the question text — the contract the Agent SDK documents for canUseTool.
// Rendered as a plain permission it was unanswerable: the person could only
// consent to "a tool" whose arguments were the question itself.
const askUserQuestionTool = "AskUserQuestion"

const inputTitle = "Rispondi all’agente"

// askUserQuestionInput is the input of one AskUserQuestion call, field for
// field, kept whole so the answer can hand the questions back unchanged.
type askUserQuestionInput struct {
	Questions []struct {
		Question    string `json:"question"`
		Header      string `json:"header"`
		MultiSelect bool   `json:"multiSelect"`
		Options     []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
}

// inputQuestions projects the questions of an AskUserQuestion call into the
// provider-neutral form the viewer draws. The question text is the identity,
// because that is what the answer is keyed by on the way back. An input that
// does not read as questions answers nil, and the request falls back to the
// permission card rather than to an invented form.
func inputQuestions(payload json.RawMessage) []execution.InputQuestion {
	var native askUserQuestionInput
	if json.Unmarshal(payload, &native) != nil || len(native.Questions) == 0 {
		return nil
	}
	seen := map[string]bool{}
	questions := make([]execution.InputQuestion, 0, len(native.Questions))
	for _, question := range native.Questions {
		text := strings.TrimSpace(question.Question)
		if text == "" || seen[text] {
			return nil
		}
		seen[text] = true
		// Claude Code always offers "Other" beside the options, so free text is
		// always allowed; a multi-select question keeps its flag and the form
		// offers checkboxes instead of radios.
		projected := execution.InputQuestion{ID: text, Header: question.Header, Prompt: text, MultiSelect: question.MultiSelect, AllowText: true}
		for _, option := range question.Options {
			if strings.TrimSpace(option.Label) == "" {
				return nil
			}
			projected.Options = append(projected.Options, execution.InputOption{Value: option.Label, Label: option.Label, Description: option.Description})
		}
		questions = append(questions, projected)
	}
	return questions
}

// askInput records an AskUserQuestion call as a pending input. The caller
// holds mu. It answers false when the input does not read as questions, so
// the caller can keep the permission path.
func (s *streamSession) askInputLocked(id string, request canUseToolRequest) bool {
	questions := inputQuestions(request.Input)
	if questions == nil {
		return false
	}
	if _, already := s.inputs[id]; already {
		return true
	}
	s.inputs[id] = execution.PendingInput{ID: id, Title: inputTitle, Payload: localrun.RawOf(request.Input), Questions: questions, CreatedAt: s.now().UTC()}
	s.inputsIn = append(s.inputsIn, id)
	return true
}

// forgetInputLocked removes one pending input from both the lookup and the
// order. The caller holds mu.
func (s *streamSession) forgetInputLocked(id string) {
	if _, ok := s.inputs[id]; !ok {
		return
	}
	delete(s.inputs, id)
	for i, kept := range s.inputsIn {
		if kept == id {
			s.inputsIn = append(s.inputsIn[:i], s.inputsIn[i+1:]...)
			return
		}
	}
}

// PendingInputs lists the questions the process is waiting an answer to,
// oldest first.
func (s *streamSession) PendingInputs() []execution.PendingInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]execution.PendingInput, 0, len(s.inputsIn))
	for _, id := range s.inputsIn {
		if input, ok := s.inputs[id]; ok {
			out = append(out, input)
		}
	}
	return out
}

// encodeInputAnswers renders the person's answers as the `updatedInput` of an
// allow decision: the original questions, handed back as they were, and one
// answer per question keyed by its text. Several values — the options chosen
// on a multi-select question, and a free-text answer beside them — are joined
// with a comma, which is the documented encoding of a multi-select answer. A
// single-select question takes one value; a second one is refused rather than
// merged into an answer the person did not give.
func encodeInputAnswers(input execution.PendingInput, answers map[string][]string) (map[string]any, error) {
	if len(answers) != len(input.Questions) {
		return nil, fmt.Errorf("answer every question of the pending input request")
	}
	encoded := make(map[string]string, len(input.Questions))
	for _, question := range input.Questions {
		var values []string
		for _, value := range answers[question.ID] {
			if value = strings.TrimSpace(value); value != "" {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("answer question %q", question.ID)
		}
		if !question.MultiSelect && len(values) > 1 {
			return nil, fmt.Errorf("question %q takes one answer", question.ID)
		}
		encoded[question.ID] = strings.Join(values, ", ")
	}
	var original map[string]any
	if err := json.Unmarshal(input.Payload, &original); err != nil {
		return nil, fmt.Errorf("re-reading the questions of %q: %w", input.ID, err)
	}
	return map[string]any{"questions": original["questions"], "answers": encoded}, nil
}

// RespondInput answers one pending AskUserQuestion and gives it up, with the
// same take-before-write rule RespondApproval follows and for the same reason.
func (s *streamSession) RespondInput(ctx context.Context, inputID string, answers map[string][]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(inputID)
	s.mu.Lock()
	input, open := s.inputs[id]
	s.mu.Unlock()
	if !open {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedUnsupported,
			RunID:  s.session.RunID(),
			Err:    fmt.Errorf("the claude session is not waiting on the input %q", id),
		}
	}
	updated, err := encodeInputAnswers(input, answers)
	if err != nil {
		return &execution.RunCommandError{Reason: execution.RunRefusedUnsupported, RunID: s.session.RunID(), Err: err}
	}
	s.mu.Lock()
	s.forgetInputLocked(id)
	s.mu.Unlock()
	payload, err := json.Marshal(map[string]any{
		"type": frameControlResponse,
		"response": map[string]any{
			"subtype":    controlSuccess,
			"request_id": id,
			"response":   map[string]any{"behavior": "allow", "updatedInput": updated},
		},
	})
	if err != nil {
		return fmt.Errorf("encoding the answers for the claude session: %w", err)
	}
	if err := s.process.Send(payload); err != nil {
		if s.sessionOver() {
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude session is already over"),
			}
		}
		return fmt.Errorf("sending the answers to the claude session: %w", err)
	}
	return nil
}
