// test/web/conversation.test.mjs
// Tests for the pure conversation renderer used by the ARchetipo web viewer.
// Run: node --test test/web/conversation.test.mjs
//
// Same discipline as workspace-status.test.mjs: the oracles are on the
// *visible text* of the rendered HTML, not on the shape of the module. What
// the person reading the panel actually sees — the reason a conversation is
// not offered, the declaration that a history is partial, whether a composer
// is there at all — is what the acceptance criteria are about. The one
// exception is the source check, where the criterion is the absence of process
// knowledge in the module itself.
//
// Verifies:
//   - AC-3 a partial history is declared at the head of the timeline, and a
//     whole one declares nothing
//   - AC-4 without availability the reason is shown verbatim and no composer
//     is offered
//   - AC-6 a closed conversation is read only, and a live one has a close
//     control
//   - US-054 AC-1 a proposed action is named with its target and nothing has
//     started yet
//   - US-054 AC-3 an action the process does not admit carries the server's
//     reason and offers no confirmation
//   - US-054 AC-4 without a proposal there is no control left to press
//   - US-054 AC-5 from an accepted outcome the run can be reached
//   - the module carries no process rules: no capability, no provider, no
//     action identifier

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

const __dirname = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(
	__dirname,
	"..",
	"..",
	"cli",
	"internal",
	"web",
	"assets",
	"conversation.js",
);

// Same minimal virtual-machine loader as workspace-status.test.mjs and
// provider-fields.test.mjs: the UMD module detects `module.exports` first, so
// the Node branch is enough — and a module that reached for `document`,
// `window` or `fetch` would fail here, because this realm has none of them.
function loadConversation() {
	const src = readFileSync(helperPath, "utf8");
	const mod = { exports: {} };
	const ctx = createContext({
		module: mod,
		// No `window` — the UMD will detect `module` and use that path.
		window: undefined,
	});
	runInContext(src, ctx);
	return mod.exports;
}

const { renderConversation } = loadConversation();

// Strip every attribute from the markup, leaving only what a reader sees.
// A sentence that survives this is visible text; one that does not was only
// ever hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

const LIVE = {
	available: true,
	provider_id: "provider-inventato",
	conversation: {
		id: "conv-1",
		state: "ACTIVE",
		working_dir: "/tmp/DIRECTORY-X",
		opened_at: "2026-08-21T10:00:00.000Z",
	},
	events: [
		{ id: 1, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "user_message", text: "PRIMO-EVENTO" },
		{ id: 2, seq: 1, at: "2026-08-21T10:00:02.000Z", kind: "text", text: "RISPOSTA-AGENTE" },
	],
	last_id: 2,
	truncated: false,
};

function withConversation(overrides) {
	return Object.assign({}, LIVE, overrides);
}

describe("renderConversation", () => {
	it("senza disponibilità mostra la ragione e non offre il compositore", () => {
		const html = renderConversation(
			{
				available: false,
				unavailable_reason:
					"the default provider does not declare workspace.converse",
				conversation: null,
				events: [],
				last_id: 0,
			},
			"",
		);

		assert.ok(
			visibleText(html).includes(
				"the default provider does not declare workspace.converse",
			),
			"la ragione ricevuta non è testo visibile",
		);
		assert.ok(
			!html.includes("conv-composer"),
			"senza disponibilità non deve esserci un compositore",
		);
		assert.ok(
			!html.includes("data-conversation-open"),
			"senza disponibilità non deve esserci il bottone di apertura",
		);
	});

	it("il renderer non nomina da sé alcuna capacità né alcun provider", () => {
		// The reason travels through untouched: an identifier the module has
		// never seen is echoed as it is.
		const html = renderConversation(
			{ available: false, unavailable_reason: "CAPACITA-INVENTATA mancante" },
			"",
		);
		assert.ok(visibleText(html).includes("CAPACITA-INVENTATA"));

		// And the source itself declares no capability, no provider and no
		// process action identifier.
		const source = readFileSync(helperPath, "utf8").toLowerCase();
		const forbidden = [
			"workspace.converse",
			"converse",
			"claude",
			"codex",
			"arcipelago",
			"inception",
			"backlog",
			"spec-draft",
			'"plan"',
			'"implement"',
			'"review"',
			// US-054: the proposal card resolves nothing by itself — it draws a
			// label, a code and a sentence the server has already decided.
			"workspace.execute",
			"autopilot",
			"worktree",
			"epic",
			"planned",
			"wiki",
		];
		for (const token of forbidden) {
			assert.ok(
				!source.includes(token),
				`il renderer non deve contenere l'identificativo ${token}`,
			);
		}
	});

	it("una storia parziale è dichiarata in testa alla timeline", () => {
		const html = renderConversation(
			withConversation({
				truncated: true,
				notice: "STORIA-PARZIALE: 7 eventi non sono più mostrati",
			}),
			"",
		);

		const text = visibleText(html);
		assert.ok(
			text.includes("STORIA-PARZIALE: 7 eventi non sono più mostrati"),
			"la dichiarazione di storia parziale non è testo visibile",
		);
		assert.ok(
			html.includes("conv-history-partial"),
			"manca la riga che dichiara la storia parziale",
		);
		assert.ok(
			html.indexOf("STORIA-PARZIALE") < html.indexOf("PRIMO-EVENTO"),
			"la dichiarazione deve precedere il primo evento della timeline",
		);
		// And it is inside the timeline, not floating above it.
		const timeline = html.slice(html.indexOf('<ol class="conv-timeline'));
		assert.ok(
			timeline.includes("STORIA-PARZIALE"),
			"la dichiarazione deve stare in testa alla timeline",
		);
	});

	it("una storia intera non dichiara nulla", () => {
		const html = renderConversation(
			withConversation({
				truncated: false,
				notice: "STORIA-PARZIALE: 7 eventi non sono più mostrati",
			}),
			"",
		);

		assert.ok(
			!html.includes("conv-history-partial"),
			"una storia intera non deve dichiararsi parziale",
		);
		const timeline = html.slice(html.indexOf('<ol class="conv-timeline'));
		assert.ok(
			!timeline.includes("STORIA-PARZIALE"),
			"nulla deve essere dichiarato in testa alla timeline di una storia intera",
		);
	});

	it("una conversazione attiva mostra timeline, compositore e chiusura", () => {
		const html = renderConversation(LIVE, "");
		const text = visibleText(html);

		assert.ok(text.includes("PRIMO-EVENTO"), "il primo evento non è visibile");
		assert.ok(text.includes("RISPOSTA-AGENTE"), "la risposta non è visibile");
		assert.ok(
			text.includes("/tmp/DIRECTORY-X"),
			"la directory di lavoro non è testo visibile",
		);
		assert.ok(html.includes("conv-composer"), "manca il compositore");
		assert.ok(
			!/<textarea[^>]*\sdisabled/.test(html),
			"il compositore di una conversazione attiva non deve essere disabled",
		);
		assert.ok(
			html.includes("data-conversation-close-open"),
			"manca il comando di chiusura",
		);
		assert.ok(
			!html.includes("data-conversation-close-confirm"),
			"la conferma non deve comparire prima di essere armata",
		);
	});

	it("il comando di chiusura chiede conferma prima di chiudere", () => {
		const armed = renderConversation(LIVE, "", { closeArmed: true });
		assert.ok(
			armed.includes("data-conversation-close-confirm"),
			"armato, deve offrire la conferma",
		);
		assert.ok(
			armed.includes("data-conversation-close-abort"),
			"armato, deve offrire di annullare",
		);
	});

	it("una conversazione non più attiva mostra la sola lettura e dichiara che l'agente non è più impegnato", () => {
		const html = renderConversation(
			withConversation({
				conversation: {
					id: "conv-1",
					state: "CLOSED",
					working_dir: "/tmp/DIRECTORY-X",
					opened_at: "2026-08-21T10:00:00.000Z",
				},
			}),
			"",
		);

		const text = visibleText(html);
		assert.ok(
			text.includes("PRIMO-EVENTO"),
			"la storia deve restare leggibile dopo la chiusura",
		);
		assert.ok(
			/no longer engaged/i.test(text),
			"deve dichiarare che l'agente non è più impegnato",
		);
		assert.ok(
			/<textarea[^>]*\sdisabled/.test(html),
			"una conversazione chiusa non deve restare scrivibile",
		);
		assert.ok(
			!html.includes("data-conversation-close-open"),
			"una conversazione già chiusa non deve offrire di chiudersi di nuovo",
		);
		assert.ok(
			html.includes("data-conversation-open"),
			"deve restare possibile aprirne una nuova",
		);
	});

	it("una conversazione aperta resta leggibile e chiudibile anche se non è più offerta", () => {
		// The state the server contract describes: the default provider was
		// changed in the Execution panel while the conversation was live, so
		// available has turned false while the conversation is still ACTIVE and
		// still holding an agent process. Throwing the panel away here would
		// leave the operator without any way to let that agent go.
		const html = renderConversation(
			withConversation({
				available: false,
				unavailable_reason:
					"the default execution provider does not declare workspace.converse",
			}),
			"",
		);
		const text = visibleText(html);

		assert.ok(
			text.includes("the default execution provider does not declare workspace.converse"),
			"la ragione del rifiuto non è testo visibile",
		);
		assert.ok(
			text.includes("PRIMO-EVENTO") && text.includes("RISPOSTA-AGENTE"),
			"la storia della conversazione aperta è stata buttata via",
		);
		assert.ok(
			html.includes("data-conversation-close-open"),
			"la conversazione aperta deve restare chiudibile",
		);
		assert.ok(
			/<textarea[^>]*\sdisabled/.test(html),
			"senza disponibilità il compositore non deve restare scrivibile",
		);
		assert.ok(
			!html.includes("data-conversation-open"),
			"non essendo offerta, non deve invitare ad aprirne un'altra",
		);
	});

	it("lo stato vuoto invita ad aprirne una", () => {
		const html = renderConversation(
			{ available: true, conversation: null, events: [], last_id: 0 },
			"",
		);

		assert.ok(
			html.includes("data-conversation-open"),
			"lo stato vuoto deve offrire il bottone di apertura",
		);
		assert.ok(
			!html.includes("conv-composer"),
			"senza conversazione non deve esserci un compositore",
		);
	});

	it("non lancia su payload parziali", () => {
		const views = [
			null,
			undefined,
			{},
			{ available: true },
			{ available: true, conversation: null },
			{ available: true, conversation: {}, events: null },
			{ available: true, conversation: { state: "ACTIVE" }, events: [null, {}, 3] },
			{ available: false },
			{ available: true, conversation: { state: "STATO-INVENTATO" }, events: [] },
		];
		for (const view of views) {
			const html = renderConversation(view, undefined);
			assert.equal(typeof html, "string");
			assert.ok(html.length > 0);
		}
	});

	it("neutralizza l'HTML che arriva dal payload", () => {
		const injected = '<img src=x onerror="alert(1)">';
		const html = renderConversation(
			withConversation({
				events: [{ id: 1, kind: "text", text: injected }],
			}),
			"",
		);
		assert.ok(html.includes("&lt;img"), "il testo dell'evento non è neutralizzato");
		assert.ok(!html.includes("<img"), "il payload ha prodotto un tag reale");

		const refused = renderConversation(
			{ available: false, unavailable_reason: injected },
			"",
		);
		assert.ok(
			refused.includes("&lt;img"),
			"la ragione non è stata neutralizzata",
		);
		assert.ok(!refused.includes("<img"), "la ragione ha prodotto un tag reale");
	});

	it("la bozza del compositore sopravvive alla resa", () => {
		const html = renderConversation(LIVE, 'BOZZA-X <b>"grassetto"</b>');
		assert.ok(
			html.includes("BOZZA-X"),
			"la bozza non compare nel campo del compositore",
		);
		assert.ok(
			html.includes("&lt;b&gt;") && html.includes("&quot;grassetto&quot;"),
			"la bozza non è stata neutralizzata",
		);
		assert.ok(!html.includes("<b>"), "la bozza ha prodotto un tag reale");
	});
});

// US-054 — the proposal channel. The agent declares what it *would* start; the
// server resolves that declaration against the workspace; a person decides. The
// oracles stay on the visible text, with the single exception of the two
// `data-*` attributes that are the contract with app.js: they are the handles
// the wiring binds to, so their presence — and their absence — is behaviour.
describe("renderConversation — proposta", () => {
	it("propone l'azione nominando bersaglio e azione", () => {
		const html = renderConversation(
			withConversation({
				proposal: {
					event_id: 7,
					runnable: true,
					label: "AZIONE-PROPOSTA",
					spec_code: "XX-999",
					spec_title: "TITOLO-DELLA-SPEC",
				},
			}),
			"",
		);
		const text = visibleText(html);

		assert.ok(
			text.includes("AZIONE-PROPOSTA"),
			"la label della proposta non è testo visibile",
		);
		assert.ok(
			text.includes("XX-999"),
			"il codice della spec proposta non è testo visibile",
		);
		assert.ok(
			text.includes("TITOLO-DELLA-SPEC"),
			"il titolo della spec proposta non è testo visibile",
		);
		assert.ok(
			html.includes("data-conversation-proposal-accept"),
			"manca il comando di conferma della proposta",
		);
		assert.ok(
			html.includes("data-conversation-proposal-decline"),
			"manca il comando di rifiuto della proposta",
		);
		assert.ok(
			/nothing has started yet/i.test(text),
			"il pannello non dichiara che nulla è ancora partito",
		);
	});

	it("non offre conferma per un'azione che il processo non ammette", () => {
		const html = renderConversation(
			withConversation({
				proposal: {
					event_id: 7,
					runnable: false,
					label: "AZIONE-PROPOSTA",
					spec_code: "XX-999",
					unavailable_reason: "RAGIONE-DEL-RIFIUTO dal processo",
					unlocked_by: "SBLOCCATA-DA un passo precedente",
				},
			}),
			"",
		);
		const text = visibleText(html);

		assert.ok(
			text.includes("RAGIONE-DEL-RIFIUTO dal processo"),
			"la ragione del rifiuto non è testo visibile",
		);
		assert.ok(
			text.includes("SBLOCCATA-DA un passo precedente"),
			"il rimedio che sbloccherebbe l'azione non è testo visibile",
		);
		assert.ok(
			!html.includes("data-conversation-proposal-accept"),
			"un'azione che il processo non ammette non deve offrire conferma",
		);
	});

	it("non disegna nulla senza proposta", () => {
		const html = renderConversation(LIVE, "");

		assert.ok(
			!html.includes("data-conversation-proposal-accept"),
			"senza proposta non deve restare un comando di conferma",
		);
		assert.ok(
			!html.includes("data-conversation-proposal-decline"),
			"senza proposta non deve restare un comando di rifiuto",
		);
	});

	it("neutralizza l'HTML che arriva dal payload della proposta", () => {
		const injected = '<script>alert(1)</script>';
		const html = renderConversation(
			withConversation({
				proposal: {
					event_id: 7,
					runnable: false,
					label: "AZIONE-PROPOSTA",
					spec_title: injected,
					unavailable_reason: injected,
				},
			}),
			"",
		);

		assert.ok(
			html.includes("&lt;script&gt;"),
			"il testo della proposta non è neutralizzato",
		);
		assert.ok(
			!html.includes("<script"),
			"il payload della proposta ha prodotto un tag reale",
		);
	});
});

describe("renderConversation — esito", () => {
	it("dall'esito accettato si raggiunge la run", () => {
		const html = renderConversation(
			withConversation({
				outcome: {
					decision: "DECISIONE-ACCETTATA",
					label: "AZIONE-PROPOSTA",
					execution_id: "exec-123",
					scope: "AMBITO-X",
					spec_code: "XX-999",
				},
			}),
			"",
		);
		const text = visibleText(html);

		assert.ok(
			text.includes("DECISIONE-ACCETTATA"),
			"la decisione non è testo visibile",
		);
		assert.ok(
			html.includes("data-conversation-reach-run"),
			"manca il comando per raggiungere la run avviata",
		);
		assert.ok(
			html.includes('data-scope="AMBITO-X"'),
			"il comando non porta lo scope del payload",
		);
		assert.ok(
			html.includes('data-code="XX-999"'),
			"il comando non porta il codice del payload",
		);
	});

	it("un esito rifiutato non offre nulla da raggiungere", () => {
		const html = renderConversation(
			withConversation({
				outcome: {
					decision: "DECISIONE-RIFIUTATA",
					label: "AZIONE-PROPOSTA",
					spec_code: "XX-999",
				},
			}),
			"",
		);

		assert.ok(
			visibleText(html).includes("DECISIONE-RIFIUTATA"),
			"la decisione di rifiuto non è testo visibile",
		);
		assert.ok(
			!html.includes("data-conversation-reach-run"),
			"un rifiuto non ha avviato nulla: non deve esserci nulla da raggiungere",
		);
	});

	it("neutralizza l'HTML che arriva dal payload dell'esito", () => {
		const html = renderConversation(
			withConversation({
				outcome: {
					decision: '<script>alert(1)</script>',
					label: '<img src=x onerror="alert(1)">',
					execution_id: "exec-123",
					scope: '"><script>alert(1)</script>',
					spec_code: "XX-999",
				},
			}),
			"",
		);

		assert.ok(
			html.includes("&lt;script&gt;"),
			"il testo dell'esito non è neutralizzato",
		);
		assert.ok(!html.includes("<script"), "l'esito ha prodotto un tag reale");
		assert.ok(!html.includes("<img"), "l'esito ha prodotto un tag reale");
	});
});
