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
		const html = renderConversation(view, "", { activitiesOpen: true, activities: { items: [
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

	it("il menu include anche attività secondarie senza aggiungerle allo stato iniziale", () => {
		const extra = { ...activity, id: "extra", label: "Attività secondaria", primary: false };
		const closed = renderConversation(view, "", { activities: { items: [activity, extra] } });
		assert.ok(!closed.includes('data-conversation-activity="extra"'));
		const open = renderConversation(view, "", { activities: { items: [activity, extra] }, activitiesOpen: true });
		assert.ok(open.includes('data-conversation-activity="extra"'));
		assert.match(open, /aria-expanded="true"/);
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
