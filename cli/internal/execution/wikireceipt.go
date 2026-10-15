package execution

import (
	"fmt"
	"strings"
)

var wikiReceiptFields = []string{"artifact", "operation", "status"}

type WikiReceipt struct {
	Artifact  string `json:"artifact"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
}

func WikiOperation(action ActionID) string {
	switch action {
	case ActionWikiCreate:
		return "bootstrap"
	case ActionWikiRefresh:
		return "refresh"
	case ActionWikiLint:
		return "lint"
	case ActionWikiQuery:
		return "query"
	}
	return ""
}

func AcceptWikiReceipt(message string, action ActionID) (WikiReceipt, error) {
	got, found := parseTrailingReceipt[WikiReceipt](message, wikiReceiptFields)
	if !found || got.Artifact != "wiki" || got.Operation != WikiOperation(action) || got.Operation == "" || got.Status != "COMPLETED" {
		return got, fmt.Errorf("the agent did not emit a completed receipt for the requested Wiki operation")
	}
	return got, nil
}

func WikiPrompt(opening string, req Request) string {
	operation := WikiOperation(req.Action)
	instruction := ""
	switch req.Action {
	case ActionWikiCreate:
		instruction = "Create the Wiki from the codebase using bootstrap. Complete the bootstrap validation gate before claiming completion."
	case ActionWikiRefresh:
		instruction = "Refresh the existing Wiki against current code and tests. Read Wiki status and inspect tracked sources of existing pages to discover changes, including uncommitted changes; use explicit --file inputs for affected discovery when no revision range is supplied. Preserve accurate pages and update obsolete knowledge."
	case ActionWikiLint:
		instruction = "Lint the Wiki: check structure, links, evidence and semantic consistency, apply structural repairs and report unresolved issues."
	case ActionWikiQuery:
		instruction = "Query the Wiki about the selected page whose exact Wiki ID is " + req.UserInput + ". Read that page and its relevant evidence, give a brief introduction, and invite the person to ask questions in this conversation. Do not modify the Wiki."
	}
	return strings.Join([]string{
		opening, questionGuidance, "Invoke /archetipo-wiki using its " + operation + " operation.", instruction,
		"Use the configured Wiki path and persist through the CLI exactly as the skill prescribes. Do not approve or reconfirm any page: revision is a separate human decision. Keep user-facing text about the Wiki, without naming the skill.",
		"After completing the operation, briefly describe the result and any unresolved issues, then close with this JSON receipt as the last line. A question is not completion; emit no receipt while waiting for an answer:",
		`{"artifact":"wiki","operation":"` + operation + `","status":"COMPLETED"}`,
	}, "\n")
}
