package codex

import (
	"encoding/json"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type nativeApprovalChoice struct {
	option   execution.ApprovalOption
	decision any
}

// approvalChoices is adapter-owned. Opaque decisions never come back from the
// browser: it returns only a declared option id, resolved against the original
// pending native request here.
func approvalChoices(request rpcMessage) []nativeApprovalChoice {
	var params struct {
		AvailableDecisions []json.RawMessage `json:"availableDecisions"`
		Amendment          []string          `json:"proposedExecpolicyAmendment"`
	}
	if json.Unmarshal(request.Params, &params) != nil {
		return nil
	}
	command := request.Method == "item/commandExecution/requestApproval"
	decisions := params.AvailableDecisions
	if decisions == nil {
		decisions = []json.RawMessage{json.RawMessage(`"accept"`), json.RawMessage(`"acceptForSession"`), json.RawMessage(`"decline"`)}
		if command && len(params.Amendment) > 0 {
			rule, _ := json.Marshal(map[string]any{"acceptWithExecpolicyAmendment": map[string]any{"execpolicy_amendment": params.Amendment}})
			decisions = append(decisions, rule)
		}
	}
	choices := make([]nativeApprovalChoice, 0, len(decisions))
	seen := map[string]bool{}
	for _, raw := range decisions {
		var option execution.ApprovalOption
		var decision any
		if json.Unmarshal(raw, &decision) != nil {
			continue
		}
		switch decision {
		case "accept":
			option = execution.ApprovalOption{ID: localrun.ApprovalAllow, Label: "Consenti una volta", Kind: "allow", Scope: "once"}
		case "acceptForSession":
			option = execution.ApprovalOption{ID: "allow_session", Label: "Consenti per la sessione", Kind: "allow", Scope: "session", Description: "Il runtime ricorda questa autorizzazione per le richieste corrispondenti nella sessione nativa."}
		case "decline":
			option = execution.ApprovalOption{ID: localrun.ApprovalDeny, Label: "Nega", Kind: "deny", Scope: "once"}
		case "cancel":
			option = execution.ApprovalOption{ID: "cancel", Label: "Nega e interrompi il turno", Kind: "deny", Scope: "once"}
		default:
			if !command {
				continue
			}
			var amendment struct {
				Rule *struct {
					Prefix []string `json:"execpolicy_amendment"`
				} `json:"acceptWithExecpolicyAmendment"`
			}
			if json.Unmarshal(raw, &amendment) != nil || amendment.Rule == nil || len(amendment.Rule.Prefix) == 0 {
				continue
			}
			option = execution.ApprovalOption{ID: "allow_rule", Label: "Ricorda la regola del comando", Kind: "allow", Scope: "persistent", Description: "Il runtime salva la regola proposta, che può valere anche in altri progetti. Prefisso: " + strings.Join(amendment.Rule.Prefix, " ")}
		}
		if !seen[option.ID] {
			choices = append(choices, nativeApprovalChoice{option: option, decision: decision})
			seen[option.ID] = true
		}
	}
	return choices
}

func approvalOptions(request rpcMessage) []execution.ApprovalOption {
	options := []execution.ApprovalOption{}
	for _, choice := range approvalChoices(request) {
		options = append(options, choice.option)
	}
	return options
}
