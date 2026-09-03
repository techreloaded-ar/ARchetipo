package domain

import "testing"

func TestNormalizeTaskBody(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want string
	}{
		{
			name: "copies legacy description into body",
			task: Task{Description: "## Descrizione\n\nContenuto legacy"},
			want: "## Descrizione\n\nContenuto legacy",
		},
		{
			name: "keeps explicit body",
			task: Task{Body: "## Descrizione\n\nBody canonico", Description: "legacy"},
			want: "## Descrizione\n\nBody canonico",
		},
		{
			name: "ignores fully empty task",
			task: Task{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := tc.task
			NormalizeTaskBody(&task)
			if task.Body != tc.want {
				t.Fatalf("NormalizeTaskBody() body = %q, want %q", task.Body, tc.want)
			}
		})
	}
}

func TestNormalizePlanInput(t *testing.T) {
	input := PlanInput{Tasks: []Task{{ID: "TASK-01", Description: "legacy body"}, {ID: "TASK-02", Body: "canonical body"}}}

	NormalizePlanInput(&input)

	if input.Tasks[0].Body != "legacy body" {
		t.Fatalf("expected legacy description copied into body, got %q", input.Tasks[0].Body)
	}
	if input.Tasks[1].Body != "canonical body" {
		t.Fatalf("expected canonical body preserved, got %q", input.Tasks[1].Body)
	}
}

func TestReworkFeedbackItems(t *testing.T) {
	dossier := &ReviewDossier{
		Criteria: []ReviewCriterion{
			{ID: "AC-1", Verdict: ReviewCriterionMet, Note: "verificato"},
			{ID: "AC-2", Verdict: ReviewCriterionUnclear, Note: "nessun test copre il caso vuoto"},
			{ID: "AC-3", Verdict: ReviewCriterionNotVerifiable},
		},
		Blockers: []string{"il build non passa", "  "},
	}
	comment := ReviewComment{File: "app.js", Line: 12, Side: "new", Body: "nome poco chiaro"}

	tests := []struct {
		name     string
		review   Review
		freeText string
		want     []string
	}{
		{
			name:   "solo commenti inline",
			review: Review{Comments: []ReviewComment{comment}},
			want:   []string{"nome poco chiaro"},
		},
		{
			name:   "solo i blocker del dossier",
			review: Review{Dossier: &ReviewDossier{Blockers: []string{"il build non passa"}}},
			want:   []string{"il build non passa"},
		},
		{
			name:   "criteri non soddisfatti, con e senza nota",
			review: Review{Dossier: &ReviewDossier{Criteria: dossier.Criteria}},
			want:   []string{"AC-2: nessun test copre il caso vuoto", "AC-3"},
		},
		{
			name:     "solo il testo libero",
			review:   Review{},
			freeText: "  manca la migrazione  ",
			want:     []string{"manca la migrazione"},
		},
		{
			name:     "ordine: commenti, blocker, criteri, testo libero",
			review:   Review{Comments: []ReviewComment{comment}, Dossier: dossier},
			freeText: "manca la migrazione",
			want: []string{
				"nome poco chiaro",
				"il build non passa",
				"AC-2: nessun test copre il caso vuoto",
				"AC-3",
				"manca la migrazione",
			},
		},
		{
			name:   "niente da riportare",
			review: Review{Dossier: &ReviewDossier{Criteria: []ReviewCriterion{{ID: "AC-1", Verdict: ReviewCriterionMet}}}},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := ReworkFeedbackItems(tt.review, tt.freeText)
			if len(items) != len(tt.want) {
				t.Fatalf("got %d items %v, want %d %v", len(items), items, len(tt.want), tt.want)
			}
			for i, want := range tt.want {
				if items[i].Body != want {
					t.Errorf("item %d: got %q, want %q", i, items[i].Body, want)
				}
			}
		})
	}
}

// Un commento inline conserva la sua ancora attraverso l'assemblaggio: è la
// differenza fra un bullet puntato a una riga e uno generico.
func TestReworkFeedbackItemsKeepsTheAnchorOfInlineComments(t *testing.T) {
	items := ReworkFeedbackItems(Review{Comments: []ReviewComment{
		{File: "app.js", Line: 12, Side: "new", Body: "nome poco chiaro"},
	}}, "")
	if len(items) != 1 || items[0].File != "app.js" || items[0].Line != 12 {
		t.Fatalf("ancora persa: %+v", items)
	}
}
