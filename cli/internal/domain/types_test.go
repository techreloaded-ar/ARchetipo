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
