package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// AskUserQuestion uses the same can_use_tool transport as permissions, but
// answers are product choices, never permission decisions. The native response
// preserves the original input and adds answers keyed by the question text.
// https://code.claude.com/docs/en/agent-sdk/user-input
func (s *streamSession) askInput(id string, payload json.RawMessage) {
	var native struct {
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
	if json.Unmarshal(payload, &native) != nil || len(native.Questions) == 0 {
		return
	}
	questions := make([]execution.InputQuestion, 0, len(native.Questions))
	seen := map[string]bool{}
	for index, question := range native.Questions {
		if strings.TrimSpace(question.Question) == "" || seen[question.Question] {
			return
		}
		seen[question.Question] = true
		projected := execution.InputQuestion{ID: strconv.Itoa(index), Header: question.Header, Prompt: question.Question, AllowText: true, Multiple: question.MultiSelect}
		for _, option := range question.Options {
			if strings.TrimSpace(option.Label) == "" {
				return
			}
			projected.Options = append(projected.Options, execution.InputOption{Value: option.Label, Label: option.Label, Description: option.Description})
		}
		questions = append(questions, projected)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inputs[id]; !exists {
		s.inputs[id] = execution.PendingInput{ID: id, Title: "Rispondi all’agente", Payload: append(json.RawMessage(nil), payload...), Questions: questions, CreatedAt: s.now().UTC()}
	}
}

func (s *streamSession) pendingInputs() []execution.PendingInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	inputs := make([]execution.PendingInput, 0, len(s.inputs))
	for _, input := range s.inputs {
		inputs = append(inputs, input)
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].CreatedAt.Equal(inputs[j].CreatedAt) {
			return inputs[i].ID < inputs[j].ID
		}
		return inputs[i].CreatedAt.Before(inputs[j].CreatedAt)
	})
	return inputs
}

func (s *streamSession) respondInput(ctx context.Context, id string, answers map[string][]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	input, exists := s.inputs[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("claude input %q is not pending", id)
	}
	payload, err := inputResponse(input, answers)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	// Claim before writing so concurrent tabs cannot answer the same request.
	delete(s.inputs, id)
	s.mu.Unlock()
	return s.process.Send(payload)
}

func inputResponse(input execution.PendingInput, answers map[string][]string) ([]byte, error) {
	if len(answers) != len(input.Questions) {
		return nil, fmt.Errorf("answer every question of the pending input request")
	}
	nativeAnswers := make(map[string]string, len(answers))
	for _, question := range input.Questions {
		values := answers[question.ID]
		if len(values) == 0 || (!question.Multiple && len(values) != 1) {
			return nil, fmt.Errorf("answer question %q", question.ID)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("answer question %q", question.ID)
			}
		}
		nativeAnswers[question.Prompt] = strings.Join(values, ", ")
	}
	var updated map[string]json.RawMessage
	if err := json.Unmarshal(input.Payload, &updated); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(nativeAnswers)
	if err != nil {
		return nil, err
	}
	updated["answers"] = encoded
	return json.Marshal(map[string]any{
		"type": frameControlResponse,
		"response": map[string]any{
			"subtype": controlSuccess, "request_id": input.ID,
			"response": map[string]any{"behavior": "allow", "updatedInput": updated},
		},
	})
}
