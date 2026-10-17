import { createRequire } from "node:module";
import { describe, it } from "node:test";
import assert from "node:assert/strict";

const require = createRequire(import.meta.url);
const { renderConversation } = require("../../cli/internal/web/assets/conversation.js");
const activity = {
	id: "catalog-entry", label: "Nuovo mockup", description: "Descrizione dal server",
	result: "Risultato dal server", placeholder: "Descrivi il bisogno",
	kind: "skill", skill: "archetipo-design", primary: true, runnable: true,
};
const view = {
	available: true, conversation: { id: "conv-test", state: "ACTIVE" },
	session: { work: "IDLE", recovery: "RESUMABLE", archive: "OPEN" }, events: [],
};
const visible = html => html.replace(/<[^>]*>/g, "");

describe("attività della conversazione", () => {
	it("ricompone i frammenti e tiene distinta la domanda dal messaggio successivo", () => {
		const html = renderConversation({ ...view, events: [
			{ id: 1, kind: "text", message_id: "question", text: "Quali ciclisti " },
			{ id: 2, kind: "text", message_id: "question", text: "vuoi aiutare?" },
			{ id: 3, kind: "text", message_id: "closing", text: "Attendo la tua risposta." },
		] }, "", {});
		assert.equal((html.match(/class="conv-event cev-agent"/g) || []).length, 2);
		assert.ok(visible(html).includes("Quali ciclisti vuoi aiutare?"));
		assert.ok(visible(html).includes("Attendo la tua risposta."));
	});
	it("mostra il progresso fornito dal server senza confonderlo con RUNNING", () => {
		const html = renderConversation({ ...view, runs: [{
			execution_id: "exec-draft", in_this_thread: true, label: "Proponi una spec",
			status: "RUNNING", progress_label: "In attesa della tua risposta", awaiting_response: true,
		}] }, "La risposta", {});
		assert.ok(visible(html).includes("In attesa della tua risposta"));
		assert.ok(!visible(html).includes("Proponi una spec"));
		const technical = renderConversation({ ...view, runs: [{ execution_id: "exec-draft", in_this_thread: true, label: "Proponi una spec", progress_label: "In attesa della tua risposta", awaiting_response: true }] }, "", { technicalAll: true });
		assert.ok(visible(technical).includes("Proponi una spec"));
		assert.match(technical, /class="conv-run is-waiting"/);
		assert.ok(!visible(html).includes("RUNNING"));
		assert.doesNotMatch(html, /class="conv-run is-waiting"/);
		assert.doesNotMatch(html, /<span class="conv-run-mark">run<\/span>/);
		assert.doesNotMatch(html, /<textarea[^>]* disabled/);
	});
	it("le approvazioni di un’azione nella sessione restano visibili senza il riepilogo tecnico", () => {
		const html = renderConversation({ ...view, runs: [{
			execution_id: "exec-draft", in_this_thread: true, label: "Proponi una spec",
			approvals: [{ id: "approval-visible", message: "Autorizzi il comando?", options: [{ id: "yes", label: "Autorizza" }] }],
		}] }, "", {});
		assert.match(html, /data-run-approval-id="approval-visible"/);
		assert.ok(!visible(html).includes("Proponi una spec"));
	});
	it("una domanda strutturata resta rispondibile senza indicare lavoro in corso", () => {
		const html = renderConversation({ ...view, session: { ...view.session, work: "WAITING_INPUT", pending_inputs: [{
			id: "question", title: "Rispondi all’agente", questions: [{
				id: "who", prompt: "Quali ciclisti vuoi aiutare?", allow_text: true, options: [],
			}],
		}] } }, "", {});
		assert.ok(visible(html).includes("Quali ciclisti vuoi aiutare?"));
		assert.ok(visible(html).includes("In attesa della tua risposta"));
		assert.match(html, /data-conversation-input-form="question"/);
		assert.doesNotMatch(html, /class="conv-working"/);
	});
	it("prepara la proposta senza aver ancora aperto una conversazione", () => {
		const selected = { ...activity, kind: "action", action: "azione-dal-server" };
		const html = renderConversation({ available: true, events: [] }, "La mia proposta", { activitySelection: selected });
		assert.match(html, /placeholder="Descrivi il bisogno"/);
		assert.match(html, />La mia proposta<\/textarea>/);
		assert.ok(visible(html).includes(activity.description));
		assert.ok(visible(html).includes(activity.result));
		assert.doesNotMatch(html, /<textarea[^>]* disabled/);
	});
	it("offre Ferma anche quando un’azione aspetta una risposta con il turno fermo", () => {
		const html = renderConversation({ ...view, runs: [{ id: "run-waiting", awaiting_response: true }] }, "", {});
		assert.match(html, /data-conversation-interrupt/);
		assert.ok(visible(html).includes("Ferma"));
	});
	it("le attività chiedono una descrizione prima di partire", () => {
		const html = renderConversation(view, "", { activities: { items: [
			activity, { ...activity, id: "start", kind: "action" },
		] } });
		assert.doesNotMatch(html, /Avvia attività/);
		assert.match(html, /Seleziona e descrivi la richiesta/);
	});
	it("mostra le attività dichiarate dal server senza esporre i nomi tecnici", () => {
		const html = renderConversation({ available: true }, "", { activities: { items: [activity] } });
		assert.match(html, /data-conversation-activity="catalog-entry"/);
		assert.ok(visible(html).includes(activity.label));
		assert.ok(visible(html).includes(activity.description));
		assert.ok(visible(html).includes(activity.result));
		assert.ok(!visible(html).includes(activity.skill));
	});

	it("mantiene il motivo di indisponibilità e non permette l’avvio", () => {
		const html = renderConversation({ available: true }, "", {
			activities: { items: [{ ...activity, runnable: false, unavailable_reason: "Catalogo non ancora disponibile" }] },
		});
		assert.match(html, /data-conversation-activity="catalog-entry" disabled/);
		assert.ok(visible(html).includes("Catalogo non ancora disponibile"));
	});

	it("la scelta resta fuori dal messaggio e usa l’etichetta e il placeholder del server", () => {
		const html = renderConversation(view, "Testo della persona", { activitySelection: activity });
		assert.match(html, /placeholder="Descrivi il bisogno"/);
		assert.match(html, />Testo della persona<\/textarea>/);
		assert.ok(visible(html).includes(activity.label));
		assert.ok(!visible(html).includes(activity.skill));
		assert.match(html, /data-conversation-activity-clear/);
	});

	it("mostra le attività e i motivi dichiarati anche senza un provider configurato", () => {
		const html = renderConversation({ available: false, unavailable_reason: "Configura un agente" }, "", {
			activities: { items: [{ ...activity, runnable: false, unavailable_reason: "Configura un agente" }] },
		});
		assert.ok(visible(html).includes(activity.label));
		assert.match(html, /data-conversation-activity="catalog-entry" disabled/);
		assert.ok(visible(html).includes("Configura un agente"));
		assert.ok(!html.includes("<textarea"));
	});

	it("le schede iniziali mantengono le attività principali senza un menu accanto al compositore", () => {
		const extra = { ...activity, id: "extra", label: "Attività secondaria", primary: false };
		const html = renderConversation(view, "", { activities: { items: [activity, extra] } });
		assert.ok(html.includes('data-conversation-activity="catalog-entry"'));
		assert.ok(!html.includes('data-conversation-activity="extra"'));
		assert.doesNotMatch(html, /data-conversation-activities-toggle|conv-activity-picker/);
	});

	it("non sostituisce una cronologia esistente con le schede iniziali", () => {
		const html = renderConversation({ ...view, events: [{ id: 1, kind: "text", text: "Risposta esistente" }] }, "", {
			activities: { items: [activity] },
		});
		assert.ok(visible(html).includes("Risposta esistente"));
		assert.ok(!html.includes('data-conversation-activity="catalog-entry"'));
	});

	it("escape di etichette, ragioni e identificatori provenienti dal catalogo", () => {
		const html = renderConversation({ available: true }, "", { activities: { items: [{
			...activity, id: '\" onmouseover=\"bad', label: '<script>bad</script>', runnable: false,
			unavailable_reason: '<img src=x onerror=bad>',
		}] } });
		assert.ok(!html.includes("<script>"));
		assert.ok(!html.includes("<img src=x"));
		assert.ok(html.includes("&lt;script&gt;"));
		assert.ok(html.includes("&quot; onmouseover=&quot;bad"));
	});
});
