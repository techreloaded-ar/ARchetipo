package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// inputQuestions projects the installed app-server's request_user_input schema.
// Unknown payloads retain the legacy response path instead of inventing fields.
func inputQuestions(payload json.RawMessage) []execution.InputQuestion {
	var native struct {
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			IsOther  bool   `json:"isOther"`
			IsSecret bool   `json:"isSecret"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(payload, &native) != nil {
		return nil
	}
	seen := map[string]bool{}
	questions := make([]execution.InputQuestion, 0, len(native.Questions))
	for _, question := range native.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Question) == "" || seen[question.ID] {
			return nil
		}
		seen[question.ID] = true
		projected := execution.InputQuestion{ID: question.ID, Header: question.Header, Prompt: question.Question, AllowText: question.IsOther || len(question.Options) == 0, Secret: question.IsSecret}
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

func encodeInputAnswers(questions []execution.InputQuestion, answers map[string][]string) (json.RawMessage, error) {
	if len(questions) == 0 || len(answers) != len(questions) {
		return nil, fmt.Errorf("answer every question of the pending input request")
	}
	type answer struct {
		Answers []string `json:"answers"`
	}
	encoded := make(map[string]answer, len(questions))
	for _, question := range questions {
		values := answers[question.ID]
		if len(values) == 0 {
			return nil, fmt.Errorf("answer question %q", question.ID)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("answer question %q", question.ID)
			}
			allowed := question.AllowText
			for _, option := range question.Options {
				allowed = allowed || value == option.Value
			}
			if !allowed {
				return nil, fmt.Errorf("choose an offered option for question %q", question.ID)
			}
		}
		encoded[question.ID] = answer{Answers: values}
	}
	return json.Marshal(struct {
		Answers map[string]answer `json:"answers"`
	}{encoded})
}
