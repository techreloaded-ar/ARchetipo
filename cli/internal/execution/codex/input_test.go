package codex

import (
	"encoding/json"
	"testing"
)

func TestInputQuestionsAndAnswersPreserveNativeChoices(t *testing.T) {
	payload := json.RawMessage(`{"questions":[
		{"id":"database","header":"Database","question":"Quale database?","options":[{"label":"SQLite","description":"Locale"},{"label":"PostgreSQL","description":"Server"}]},
		{"id":"other","question":"Ambiente?","isOther":true,"options":[{"label":"Locale","description":"Sul computer"}]},
		{"id":"secret","question":"Valore riservato?","isSecret":true,"options":null}
	]}`)
	questions := inputQuestions(payload)
	if len(questions) != 3 || questions[0].AllowText || !questions[1].AllowText || !questions[2].Secret || !questions[2].AllowText {
		t.Fatalf("questions = %#v", questions)
	}
	if questions[0].Options[1].Value != "PostgreSQL" || questions[0].Options[1].Description != "Server" {
		t.Fatalf("choice identity changed: %#v", questions[0])
	}
	encoded, err := encodeInputAnswers(questions, map[string][]string{"database": {"SQLite"}, "other": {"Un altro ambiente"}, "secret": {"valore-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Answers["database"].Answers[0] != "SQLite" || decoded.Answers["other"].Answers[0] != "Un altro ambiente" {
		t.Fatalf("native response = %s, %v", encoded, err)
	}
	for name, answers := range map[string]map[string][]string{
		"missing":          {"database": {"SQLite"}},
		"unknown question": {"database": {"SQLite"}, "other": {"x"}, "unknown": {"x"}},
		"unknown option":   {"database": {"MySQL"}, "other": {"x"}, "secret": {"x"}},
		"blank":            {"database": {"SQLite"}, "other": {" "}, "secret": {"x"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := encodeInputAnswers(questions, answers); err == nil {
				t.Fatal("invalid answer accepted")
			}
		})
	}
}

func TestUnknownInputSchemaKeepsLegacyPayloadPath(t *testing.T) {
	for _, payload := range []string{`{"futureField":true}`, `{"questions":[{"id":"q"}]}`, `{"questions":[{"id":"q","question":"One?"},{"id":"q","question":"Two?"}]}`} {
		if len(inputQuestions(json.RawMessage(payload))) != 0 {
			t.Fatalf("invented questions for %s", payload)
		}
	}
}
