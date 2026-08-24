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
//   - US-061 AC-2 the recommended step is drawn at the tail of the thread, it
//     names the action and the spec it acts on, and carries the target in its
//     attributes
//   - US-061 AC-3 a step that cannot be taken is not pressable and its reason
//     is read inside the block itself
//   - US-061 AC-4 with nothing pending no block and no placeholder is drawn
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

// US-060 — the run inside the flow. A run started from a conversation belongs
// where it was asked for, its consent request must read the command in the
// clear, a refusal must stay on the page after the provider stops listing it,
// and none of this may take the word away from the person: the composer stays
// writable while a run waits.
//
// Verifies:
//   - AC-1 the block sits right after the event it names as its anchor, and a
//     run whose anchor has left the history is still present, at the tail
//   - AC-3 the command is textual in the HTML and the buttons are exactly the
//     options the payload declares
//   - AC-4 an answered decision stays readable with the option that was chosen
//   - AC-5 the composer is not disabled while a run is waiting

const RUN_EVENTS = [
	{ id: 1, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "user_message", text: "EVENTO-UNO" },
	{ id: 2, seq: 2, at: "2026-08-21T10:00:02.000Z", kind: "text", text: "EVENTO-DUE" },
	{ id: 3, seq: 3, at: "2026-08-21T10:00:03.000Z", kind: "text", text: "EVENTO-TRE" },
];

// The rail of an event ends with `#<id></div>`: matching the whole tail keeps
// the oracle away from any other `#` the escaping may have produced.
function railOf(id) {
	return `#${id}</div>`;
}

function runAt(anchor, overrides) {
	return Object.assign(
		{
			execution_id: "exec-1",
			anchor_event_id: anchor,
			spec_code: "XX-999",
			scope: "AMBITO-X",
			label: "AZIONE-AVVIATA",
			run: { state: "ACTIVE" },
			events: [],
			approvals: [],
		},
		overrides,
	);
}

const APPROVAL = {
	id: "appr-1",
	tool_name: "Bash",
	title: "TITOLO-DELLA-RICHIESTA",
	created_at: "2026-08-21T10:00:04.000Z",
	args: { command: "git worktree prune --verbose" },
	options: [
		{ id: "allow-once", kind: "allow", label: "OPZIONE-CONSENSO" },
		{ id: "deny-once", kind: "deny", label: "OPZIONE-RIFIUTO" },
	],
};

function timelineOf(html) {
	const from = html.indexOf('<ol class="conv-timeline');
	return html.slice(from, html.indexOf("</ol>", from));
}

describe("renderConversation — la run dentro il flusso", () => {
	it("il blocco della run sta subito dopo l'evento che l'ha chiesta", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, runs: [runAt(2)] }),
			"",
		);

		const block = html.indexOf('data-conversation-run-anchor="2"');
		assert.ok(block > 0, "il blocco della run non è stato disegnato");
		assert.ok(
			html.indexOf(railOf(2)) < block,
			"il blocco deve seguire l'evento che lo ha chiesto",
		);
		assert.ok(
			block < html.indexOf(railOf(3)),
			"il blocco deve precedere l'evento successivo",
		);
	});

	it("una run la cui ancora non è più nella storia va in coda", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, runs: [runAt(99)] }),
			"",
		);

		const block = html.indexOf('data-conversation-run-anchor="99"');
		assert.ok(
			block > 0,
			"una run la cui ancora è uscita dalla storia non deve sparire",
		);
		assert.ok(
			html.indexOf(railOf(3)) < block,
			"senza la sua ancora il blocco deve stare dopo l'ultimo evento",
		);
		assert.ok(
			timelineOf(html).includes('data-conversation-run-anchor="99"'),
			"il blocco deve comunque restare dentro la timeline",
		);
	});

	it("due run producono due blocchi ancorati ai propri eventi", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [
					runAt(3, { execution_id: "exec-tardi", label: "AZIONE-SECONDA" }),
					runAt(1, { execution_id: "exec-presto", label: "AZIONE-PRIMA" }),
				],
			}),
			"",
		);

		const first = html.indexOf('data-conversation-run-anchor="1"');
		const second = html.indexOf('data-conversation-run-anchor="3"');
		assert.ok(first > 0 && second > 0, "entrambi i blocchi devono esserci");
		assert.ok(
			first < second,
			"l'ordine dei blocchi è quello delle ancore, non quello del payload",
		);
		assert.ok(
			html.indexOf(railOf(1)) < first && first < html.indexOf(railOf(2)),
			"il primo blocco non è ancorato al proprio evento",
		);
		assert.ok(
			html.indexOf(railOf(3)) < second,
			"il secondo blocco non è ancorato al proprio evento",
		);
	});

	it("il comando di un consenso si legge in chiaro", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [runAt(2, { awaiting_response: true, approvals: [APPROVAL] })],
			}),
			"",
		);

		const from = html.indexOf('<pre class="run-approval-args">');
		assert.ok(from > 0, "manca il blocco degli argomenti della richiesta");
		const args = html.slice(from, html.indexOf("</pre>", from));
		assert.ok(
			args.includes("git worktree prune --verbose"),
			"il comando non si legge in chiaro dentro run-approval-args",
		);
		assert.ok(
			args.includes("&quot;command&quot;"),
			"gli argomenti non sono stati neutralizzati dove serviva",
		);
		assert.ok(
			visibleText(html).includes("TITOLO-DELLA-RICHIESTA"),
			"il titolo della richiesta non è testo visibile",
		);

		const buttons = html.match(/data-run-approval-id="appr-1"/g) || [];
		assert.equal(
			buttons.length,
			APPROVAL.options.length,
			"i bottoni devono essere esattamente le opzioni dichiarate dal payload",
		);
		assert.ok(
			html.includes("OPZIONE-CONSENSO") && html.includes("OPZIONE-RIFIUTO"),
			"le etichette delle opzioni dichiarate non compaiono",
		);
		assert.ok(
			html.includes('data-run-option-id="allow-once"') &&
				html.includes('data-run-option-id="deny-once"'),
			"i bottoni non portano gli identificativi delle opzioni del payload",
		);
	});

	it("un'approvazione risolta resta leggibile", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [runAt(2, { approvals: [APPROVAL] })],
			}),
			"",
			{
				answeredApprovals: {
					"appr-1": {
						optionID: "deny-once",
						label: "OPZIONE-RIFIUTO",
						denied: true,
					},
				},
			},
		);

		assert.ok(
			html.includes("is-answered"),
			"una decisione già data deve essere disegnata come risolta",
		);
		assert.ok(
			visibleText(html).includes("OPZIONE-RIFIUTO"),
			"l'opzione scelta non resta leggibile",
		);
		const enabled = (html.match(/<button[^>]*data-run-approval-id[^>]*>/g) || [])
			.filter((button) => !/\sdisabled/.test(button));
		assert.equal(
			enabled.length,
			0,
			"una decisione già data non deve lasciare bottoni premibili",
		);
	});

	it("il rifiuto resta leggibile quando l'approvazione non è più pendente", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				// Il provider ha smesso di elencare l'approvazione: è esattamente il
				// momento in cui la carta sparirebbe portandosi via il rifiuto.
				runs: [runAt(2, { approvals: [] })],
			}),
			"",
			{
				answeredApprovals: {
					"appr-1": {
						optionID: "deny-once",
						label: "OPZIONE-RIFIUTO",
						denied: true,
						approval: APPROVAL,
					},
				},
			},
		);

		assert.ok(
			html.includes("is-denied"),
			"un rifiuto già dato deve restare disegnato come rifiuto",
		);
		assert.ok(
			visibleText(html).includes("OPZIONE-RIFIUTO"),
			"l'opzione rifiutata non resta leggibile nella conversazione",
		);
		assert.ok(
			visibleText(html).includes("git worktree prune --verbose"),
			"il comando rifiutato non resta leggibile",
		);
		const enabled = (html.match(/<button[^>]*data-run-approval-id[^>]*>/g) || [])
			.filter((button) => !/\sdisabled/.test(button));
		assert.equal(
			enabled.length,
			0,
			"una decisione già data non deve lasciare bottoni premibili",
		);
	});

	it("un'approvazione risolta non viene disegnata due volte", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				// Finché il provider la elenca ancora, la carta del payload è
				// l'unica: la copia locale non ne aggiunge una seconda.
				runs: [runAt(2, { approvals: [APPROVAL] })],
			}),
			"",
			{
				answeredApprovals: {
					"appr-1": {
						optionID: "deny-once",
						label: "OPZIONE-RIFIUTO",
						denied: true,
						approval: APPROVAL,
					},
				},
			},
		);

		const cards = html.match(/run-approval-title/g) || [];
		assert.equal(cards.length, 1, "la carta è stata disegnata due volte");
	});

	it("il composer resta scrivibile mentre una run attende", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [runAt(2, { awaiting_response: true, approvals: [APPROVAL] })],
			}),
			"",
		);

		assert.ok(
			!/<textarea[^>]*\sdisabled/.test(html),
			"un'attesa non deve togliere la parola a chi scrive",
		);
		assert.ok(
			html.includes("conv-composer-await"),
			"manca il suggerimento sull'attesa sotto il compositore",
		);
	});

	it("un payload senza runs non cambia nulla", () => {
		const baseline = timelineOf(
			renderConversation(withConversation({ events: RUN_EVENTS, last_id: 3 }), ""),
		);
		const empty = timelineOf(
			renderConversation(
				withConversation({ events: RUN_EVENTS, last_id: 3, runs: [] }),
				"",
			),
		);

		assert.equal(empty, baseline, "un elenco di run vuoto ha cambiato la timeline");
		assert.ok(
			!baseline.includes("data-conversation-run-anchor"),
			"senza run non deve restare alcun blocco",
		);
		assert.ok(
			!baseline.includes("conv-composer-await"),
			"senza run non deve esserci alcun suggerimento d'attesa",
		);
	});

	it("un payload malformato non fa lanciare", () => {
		const views = [
			withConversation({ events: RUN_EVENTS, runs: "RUNS-INVENTATE" }),
			withConversation({ events: RUN_EVENTS, runs: null }),
			withConversation({ events: RUN_EVENTS, runs: [null] }),
			withConversation({ events: RUN_EVENTS, runs: [null, 3, "x", {}] }),
			withConversation({
				events: RUN_EVENTS,
				runs: [{ anchor_event_id: 2, approvals: "X", events: 7, run: null }],
			}),
			withConversation({
				events: RUN_EVENTS,
				runs: [runAt(2, { approvals: [null, {}, { id: "appr-2", options: "X" }] })],
			}),
		];
		for (const view of views) {
			const html = renderConversation(view, "");
			assert.equal(typeof html, "string");
			assert.ok(html.includes("conv-timeline"));
		}
	});
});

// The recommended step of the workspace, hosted at the tail of the thread.
// Same discipline as the proposal block above: the oracles are on the visible
// text and on the `data-*` attributes that are the contract with app.js — the
// handles the wiring binds to, so their presence and their absence are
// behaviour. The step arrives as local, non-payload state of the panel
// (`ui.nextStep`), because it is a fact of the workspace and not of this
// conversation.
describe("renderConversation — passo successivo", () => {
	const RUNNABLE = {
		scope: "spec",
		action: "implement",
		label: "Implementa",
		runnable: true,
		spec: { code: "US-002" },
	};

	// The markup of the block alone, opening tag to matching close, div depth
	// counted: what is asserted "inside the block" must be inside the block and
	// not merely somewhere in the panel. Slicing to the end of the string would
	// swallow the composer, and a reason drawn *under* the composer would pass.
	function nextStepBlock(html) {
		const start = html.indexOf('<div class="conv-nextstep');
		assert.notEqual(start, -1, "il blocco del passo successivo non è disegnato");
		const re = /<div\b|<\/div>/g;
		re.lastIndex = start;
		let depth = 0;
		let m;
		while ((m = re.exec(html)) !== null) {
			depth += m[0] === "</div>" ? -1 : 1;
			if (depth === 0) return html.slice(start, m.index + m[0].length);
		}
		assert.fail("il blocco del passo successivo non si chiude");
	}

	it("nomina il passo e la spec su cui agisce", () => {
		const html = renderConversation(LIVE, "", { nextStep: RUNNABLE });
		const text = visibleText(html);

		assert.ok(
			text.includes("Implementa"),
			"l'etichetta del passo successivo non è testo visibile",
		);
		assert.ok(
			text.includes("US-002"),
			"il codice della spec su cui agisce il passo non è testo visibile",
		);
		assert.ok(
			html.includes('class="conv-nextstep-run"'),
			"il passo successivo non offre alcun comando da premere",
		);
		assert.ok(
			html.includes('data-next-action="implement"'),
			"il comando del passo non porta l'azione da avviare",
		);
		assert.ok(
			html.includes('data-next-scope="spec"'),
			"il comando del passo non porta l'ambito su cui avviare",
		);
		assert.ok(
			html.includes('data-next-spec="US-002"'),
			"il comando del passo non porta la spec bersaglio",
		);
	});

	it("un passo di workspace non nomina nessuna spec", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "workspace",
				action: "prd",
				label: "Scrivi il PRD",
				runnable: true,
			},
		});

		assert.ok(
			html.includes('data-next-spec=""'),
			"un passo di workspace dichiara comunque una spec bersaglio",
		);
		assert.ok(
			!html.includes("conv-nextstep-code"),
			"un passo di workspace disegna il posto di un codice spec che non esiste",
		);
	});

	it("sta in coda alla conversazione, prima del compositore", () => {
		const html = renderConversation(LIVE, "", { nextStep: RUNNABLE });
		const idxTimeline = html.indexOf("conv-timeline");
		const idxNextStep = html.indexOf("conv-nextstep");
		const idxComposer = html.indexOf("conv-composer");

		assert.ok(
			idxTimeline !== -1 && idxNextStep !== -1 && idxComposer !== -1,
			"timeline, passo successivo o compositore non sono disegnati",
		);
		assert.ok(
			idxTimeline < idxNextStep,
			"il blocco non è più in coda alla conversazione: precede la storia invece di seguirla",
		);
		assert.ok(
			idxNextStep < idxComposer,
			"il blocco non è più in coda alla conversazione: segue il compositore invece di precederlo",
		);
	});

	it("un passo bloccato non è premibile e dice perché", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "spec",
				action: "review",
				label: "Rivedi",
				runnable: false,
				spec: { code: "US-002" },
				unavailable_reason: "RAGIONE-DEL-RIFIUTO dal processo",
				unlocked_by: "SBLOCCATO-DA un passo precedente",
			},
		});
		const block = nextStepBlock(html);

		assert.ok(
			/<button[^>]*conv-nextstep-run[^>]*disabled/.test(block),
			"il comando di un passo bloccato è premibile",
		);
		assert.ok(
			visibleText(block).includes("SBLOCCATO-DA un passo precedente"),
			"il motivo del blocco non si legge dentro il blocco del passo",
		);
	});

	it("senza unlocked_by mostra la ragione del rifiuto", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "spec",
				action: "review",
				label: "Rivedi",
				runnable: false,
				spec: { code: "US-002" },
				unavailable_reason: "RAGIONE-DEL-RIFIUTO dal processo",
			},
		});

		assert.ok(
			visibleText(nextStepBlock(html)).includes("RAGIONE-DEL-RIFIUTO dal processo"),
			"senza rimedio dichiarato il blocco tace sul motivo del rifiuto",
		);
	});

	it("un passo già in corso porta alla sua run invece di offrire un avvio inerte", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "spec",
				action: "plan",
				label: "Pianifica",
				runnable: false,
				spec: { code: "US-002" },
				unavailable_reason: "RAGIONE-DEL-RIFIUTO dal processo",
				running_execution_id: "exec-abc",
			},
		});
		const block = nextStepBlock(html);

		assert.ok(
			block.includes("data-conversation-reach-run"),
			"il passo già in corso non offre alcun modo di raggiungere la run che lo blocca",
		);
		assert.ok(
			block.includes('data-code="US-002"'),
			"il comando di raggiungimento non porta la spec su cui la run è in corso",
		);
		assert.ok(
			!/<button[^>]*conv-nextstep-run/.test(block),
			"il passo già in corso offre ancora l'avvio di ciò che sta già avvenendo",
		);
		assert.ok(
			visibleText(block).includes("RAGIONE-DEL-RIFIUTO dal processo"),
			"il motivo del blocco non si legge più dentro il blocco del passo",
		);
	});

	it("un passo bloccato da altro resta un avvio non premibile", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "spec",
				action: "review",
				label: "Rivedi",
				runnable: false,
				spec: { code: "US-002" },
				unavailable_reason: "RAGIONE-DEL-RIFIUTO dal processo",
			},
		});
		const block = nextStepBlock(html);

		assert.ok(
			/<button[^>]*conv-nextstep-run[^>]*disabled/.test(block),
			"un passo bloccato da una condizione da soddisfare non è più un avvio inerte",
		);
		assert.ok(
			!block.includes("data-conversation-reach-run"),
			"un passo bloccato da altro promette una run da raggiungere che nessuno ha nominato",
		);
	});

	it("neutralizza l'HTML che arriva dal payload del passo", () => {
		const injected = '<img src=x onerror=1>';
		const html = renderConversation(LIVE, "", {
			nextStep: {
				scope: "spec",
				action: "review",
				label: injected,
				runnable: false,
				unavailable_reason: injected,
			},
		});

		assert.ok(
			html.includes("&lt;img"),
			"il testo del passo non è neutralizzato",
		);
		assert.ok(
			!html.includes("<img"),
			"il payload del passo ha prodotto un tag reale",
		);
	});

	it("non lancia su un passo parziale", () => {
		const partials = [{}, { action: "plan" }, { runnable: true }];
		for (const nextStep of partials) {
			const html = renderConversation(LIVE, "", { nextStep });
			assert.equal(typeof html, "string");
			assert.ok(html.includes("conv-timeline"));
		}
	});

	it("senza passo raccomandato non c'è alcun blocco", () => {
		const html = renderConversation(LIVE, "", {});

		assert.ok(
			!html.includes("conv-nextstep"),
			"in coda alla conversazione compare un blocco anche senza passo in sospeso",
		);
	});

	it("ogni forma di assenza produce la stessa assenza di blocco", () => {
		const absences = [
			{ nextStep: null },
			{ nextStep: undefined },
			{ nextStep: "plan" },
		];
		for (const ui of absences) {
			const html = renderConversation(LIVE, "", ui);
			assert.ok(
				!html.includes("conv-nextstep"),
				`in coda alla conversazione compare un blocco con nextStep = ${JSON.stringify(ui.nextStep)}`,
			);
		}
	});

	it("un passo senza nome non viene proposto", () => {
		const html = renderConversation(LIVE, "", {
			nextStep: { scope: "spec", runnable: true, spec: { code: "US-002" } },
		});

		assert.ok(
			!html.includes("conv-nextstep"),
			"un passo che non ha un nome da mostrare viene comunque proposto",
		);
	});

	it("senza conversazione da mostrare non c'è coda a cui accodarsi", () => {
		const html = renderConversation({}, "", { nextStep: RUNNABLE });

		assert.ok(
			html.includes("conv-empty"),
			"senza conversazione manca l'invito ad aprirne una",
		);
		assert.ok(
			!html.includes("conv-nextstep"),
			"il blocco del passo compare dove non c'è alcuna conversazione a cui accodarlo",
		);
	});

	// The assumption this spec registered, made explicit so a later change
	// cannot break it in silence: the block follows *a* conversation being
	// shown, not a live one. Tied to ACTIVE alone, the recommended step would
	// be unreachable from the workspace home every time the last conversation
	// has ended.
	it("una conversazione conclusa mostra comunque il passo", () => {
		const html = renderConversation(
			withConversation({
				conversation: Object.assign({}, LIVE.conversation, { state: "CLOSED" }),
			}),
			"",
			{ nextStep: RUNNABLE },
		);

		assert.ok(
			html.includes("conv-nextstep"),
			"una conversazione conclusa nasconde il passo raccomandato",
		);
	});
});
