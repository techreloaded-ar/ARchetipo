package web

import (
	"net/http"
	"slices"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

type conversationActivityView struct {
	template.ConversationActivity
	Kind              string `json:"kind"`
	Primary           bool   `json:"primary"`
	Scope             string `json:"scope,omitempty"`
	Runnable          bool   `json:"runnable"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type conversationActivitiesView struct {
	ConversationID string                     `json:"conversation_id"`
	SkillsKnown    bool                       `json:"skills_known"`
	Items          []conversationActivityView `json:"items"`
}

// This read does not open a session or start a turn. Process actions retain
// their existing offers and availability; native skills are enabled only by
// the catalog of this exact conversation, never by an installed directory.
func (s *Server) handleGetConversationActivities(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	id := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	var record conversationlog.Record
	var discovery execution.SessionDiscovery
	if id != "" {
		record, err = ws.conversationStore().GetMetadata(ctx, id)
		if err != nil {
			writeError(w, conversationNotFound(id, err))
			return
		}
		if record.Native() {
			// Catalog failure is an unknown availability, not an invented list
			// and not a failure of the independent process actions.
			_, discovery, _ = s.nativeConversationDiscovery(ctx, ws, id)
		}
	}
	availability := s.workspaceAvailability(ctx, ws)
	actions := workspaceActionViews(tpl.WorkspaceActions, availability)
	conversationTarget := s.conversationAvailabilityFor(ctx, ws)
	items := resolveConversationActivities(tpl, actions, record, discovery, conversationTarget.reason)
	if id != "" {
		if running, found, readErr := s.actionAlreadyRunningIn(ctx, ws, id); readErr == nil && found {
			for index := range items {
				if items[index].Kind == "action" {
					items[index].Runnable = false
					items[index].UnavailableReason = "Questa conversazione sta già seguendo l’attività " + running + ". Completala o annullala, oppure apri un’altra conversazione."
				}
			}
		}
	}
	// A starter can connect the native session to discover this activity. It
	// cannot invoke it until that session has actually announced its catalog.
	if id == "" {
		for index := range items {
			if items[index].Kind != "skill" {
				continue
			}
			items[index].Runnable = conversationTarget.reason == "" && conversationTarget.sessions != nil
			if items[index].Runnable {
				items[index].Kind = "connect"
				items[index].UnavailableReason = ""
			} else if conversationTarget.reason != "" {
				items[index].UnavailableReason = conversationTarget.reason
			} else {
				items[index].UnavailableReason = "L’agente configurato non espone un catalogo di attività."
			}
		}
	}
	writeJSON(w, http.StatusOK, conversationActivitiesView{
		ConversationID: id, SkillsKnown: discovery.SkillsKnown, Items: items,
	})
}

func resolveConversationActivities(tpl template.Template, actions []workspaceActionView, record conversationlog.Record, discovery execution.SessionDiscovery, conversationReason string) []conversationActivityView {
	items := make([]conversationActivityView, 0, len(tpl.ConversationActivities))
	for _, definition := range tpl.ConversationActivities {
		item := conversationActivityView{ConversationActivity: definition, Primary: true}
		switch {
		case definition.Action != "":
			index := slices.IndexFunc(actions, func(action workspaceActionView) bool { return action.ID == definition.Action })
			if index < 0 || !actions[index].Offered {
				continue
			}
			item.Kind, item.Scope = "action", template.ScopeWorkspace
			item.Runnable = actions[index].Runnable
			item.UnavailableReason = actions[index].UnavailableReason
			if remedy := actions[index].UnlockedBy; remedy != "" {
				if item.UnavailableReason != "" {
					item.UnavailableReason += ". "
				}
				item.UnavailableReason += remedy
			}
			if record.Native() {
				if reason := nativeActivityUnavailableReason(record); reason != "" {
					item.Runnable, item.UnavailableReason = false, reason
				}
			}
		case definition.Skill != "":
			item.Kind = "skill"
			item.UnavailableReason = "Apri una conversazione per verificare questa attività."
			if record.Native() {
				item.UnavailableReason = "Le attività dell’agente non sono ancora disponibili."
				if discovery.SkillsKnown {
					index := slices.IndexFunc(discovery.Skills, func(skill execution.SessionSkill) bool { return skill.Name == definition.Skill })
					if index < 0 {
						continue
					}
					item.Runnable = slices.Contains(discovery.Capabilities, execution.SessionCapabilitySkillInvocation)
					item.UnavailableReason = "L’agente non supporta questa attività."
					if item.Runnable {
						item.UnavailableReason = ""
					}
				}
				if reason := nativeActivityUnavailableReason(record); reason != "" {
					item.Runnable, item.UnavailableReason = false, reason
				}
			}
		default:
			item.Kind = "conversation"
			item.Runnable = conversationReason == ""
			item.UnavailableReason = conversationReason
		}
		items = append(items, item)
	}
	if discovery.SkillsKnown {
		for _, skill := range discovery.Skills {
			if strings.TrimSpace(skill.Name) == "" || slices.ContainsFunc(tpl.ConversationActivities, func(activity template.ConversationActivity) bool { return activity.Skill == skill.Name }) ||
				slices.ContainsFunc(tpl.WorkspaceActions, func(action template.WorkspaceAction) bool { return action.Skill == skill.Name }) {
				continue
			}
			item := conversationActivityView{
				ConversationActivity: template.ConversationActivity{ID: "native:" + skill.Name, Label: nativeActivityLabel(skill.Name), Skill: skill.Name},
				Kind:                 "skill", Runnable: slices.Contains(discovery.Capabilities, execution.SessionCapabilitySkillInvocation),
			}
			if !item.Runnable {
				item.UnavailableReason = "L’agente non supporta questa attività."
			} else if reason := nativeActivityUnavailableReason(record); reason != "" {
				item.Runnable, item.UnavailableReason = false, reason
			}
			items = append(items, item)
		}
	}
	return items
}

// Skill descriptions are instructions for the agent, not menu titles. Without
// presentation from the Template, preserve the skill's identity in a readable
// label, including for runtimes that announce names without descriptions.
func nativeActivityLabel(name string) string {
	label := strings.NewReplacer("-", " ", "_", " ", ":", " · ", "/", " · ").Replace(name)
	return strings.Join(strings.Fields(label), " ")
}

func nativeActivityUnavailableReason(record conversationlog.Record) string {
	if record.Archive == execution.ArchiveArchived || record.Recovery != execution.RecoveryResumable {
		return "Riapri la conversazione prima di scegliere un’attività."
	}
	if record.Work != execution.SessionIdle {
		return "Attendi la fine del turno prima di scegliere un’attività."
	}
	return ""
}
