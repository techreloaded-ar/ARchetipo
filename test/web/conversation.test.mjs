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
//   - ogni avviso porta il comando per chiuderlo, e un avviso che il lettore ha
//     chiuso non viene disegnato affatto
//   - una conversazione finita dice sopra al campo, e non accanto, che la
//     risposta arriverà in una conversazione nuova
//   - il dettaglio tecnico — strumenti, ganci, ragionamento — è ripiegato per
//     difetto, la piega dice quanti passaggi contiene e quali strumenti ha
//     toccato, e aperta rende esattamente le righe di prima
//   - il testo dell'agente e la frase di chi scrive non finiscono mai in una
//     piega; i ganci di apertura e ogni altra riga che il pannello non sa
//     leggere sì, e la piega li nomina
//   - una piega che contiene un errore lo dichiara da chiusa
//   - mentre l'agente lavora la conversazione lo dichiara in coda, dice a che
//     cosa sta lavorando e da quanto, e la riga sparisce a turno chiuso

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

const { renderConversation, formatElapsed } = loadConversation();

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
	// La riga agente sta sulla riga del comando che la usa, e non in un riquadro
	// sopra di esso: nello stato vuoto è il pulsante che apre.
	it("prima dell'apertura mostra la scelta di modello sulla riga del comando", () => {
		const choice = '<div class="conv-agent-row">MODELLO-CONVERSAZIONE</div>';
		const html = renderConversation(
			{ available: true, conversation: null, events: [] },
			"",
			{ modelChoiceHtml: choice, openingSpecCode: "US-777" },
		);
		const text = visibleText(html);

		assert.ok(text.includes("MODELLO-CONVERSAZIONE"));
		assert.ok(text.includes("US-777"));
		assert.ok(html.indexOf(choice) < html.indexOf("data-conversation-open"));
		assert.ok(
			html.indexOf('class="conv-empty-command"') < html.indexOf(choice),
			"la scelta non è sulla riga del comando che la usa",
		);
	});

	// A conversazione conclusa riprenderla apre una sessione nuova, e la riga
	// agente è dove si sceglie con che modello parte: sta nel compositore,
	// all'altezza del pulsante che manda.
	it("a conversazione conclusa la scelta sta dentro al compositore", () => {
		const choice = '<div class="conv-agent-row">MODELLO-CONVERSAZIONE</div>';
		const html = renderConversation(
			withConversation({
				conversation: { ...LIVE.conversation, state: "CLOSED" },
			}),
			"",
			{ modelChoiceHtml: choice },
		);

		assert.ok(html.includes(choice));
		assert.ok(
			html.indexOf('class="conv-composer"') < html.indexOf(choice),
			"la scelta non è dentro al compositore",
		);
	});

	// Prima il blocco spariva e con lui l'informazione: ora la riga resta,
	// inerte e con un lucchetto.
	it("una conversazione attiva dice il modello fissato e non lo offre", () => {
		const html = renderConversation(
			withConversation({
				model: "MODELLO-FISSATO",
				model_options: { sforzo: "LIVELLO-X" },
			}),
			"",
			{ modelChoiceHtml: "MODELLO-DA-NON-MOSTRARE" },
		);
		const text = visibleText(html);

		assert.ok(!html.includes("MODELLO-DA-NON-MOSTRARE"));
		assert.ok(!html.includes("data-conversation-pill"), "la scelta è ancora premibile");
		assert.ok(html.includes("conv-agent-lock"), "la riga fissata non porta il lucchetto");
		assert.ok(
			html.includes('title="Modello e ragionamento sono stati fissati all&#39;apertura"'),
			"la ragione per cui non si sceglie più non è nel title",
		);
		assert.ok(
			text.includes("MODELLO-FISSATO · LIVELLO-X"),
			"il modello fissato e il suo sforzo non si leggono sulla riga",
		);
	});

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

	it("il comando di chiusura sta nella testata e non nella riga del compositore", () => {
		// L'unico comando irreversibile del pannello non condivide la riga con
		// l'unico comando ordinario: sta accanto allo stato su cui agisce, in
		// testa, e la riga sotto al campo resta a scrivere e inviare.
		const html = renderConversation(LIVE, "", {});
		const idxHead = html.indexOf('class="conv-head"');
		const idxClose = html.indexOf("data-conversation-close-open");
		const idxComposer = html.indexOf("conv-composer");

		assert.ok(
			idxHead !== -1 && idxClose !== -1 && idxComposer !== -1,
			"testata, comando di chiusura o compositore non sono disegnati",
		);
		assert.ok(
			idxHead < idxClose && idxClose < idxComposer,
			"il comando di chiusura non è più nella testata: sta fuori dalla banda che intesta la conversazione",
		);

		const armed = renderConversation(LIVE, "", { closeArmed: true });
		assert.ok(
			armed.indexOf("data-conversation-close-confirm") <
				armed.indexOf("conv-composer"),
			"armata, la conferma è scesa nella riga del compositore",
		);
	});

	it("il comando di chiusura si annuncia per esteso anche quando l'etichetta è corta", () => {
		// L'etichetta visibile è breve perché il controllo è quieto; il nome
		// accessibile dice per intero che cosa si sta per chiudere.
		const html = renderConversation(LIVE, "", {});
		const button = html.match(/<button[^>]*data-conversation-close-open[^>]*>/);
		assert.ok(button, "il comando di chiusura non è un bottone");
		assert.match(
			button[0],
			/aria-label="Chiudi la conversazione"/,
			"il comando di chiusura non dichiara il proprio nome accessibile",
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
			/non è più impegnato/i.test(text),
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
		// Aprirne una nuova si comanda dall'elenco delle conversazioni: il
		// pannello non ripete in coda un comando che ha già il suo posto, e la
		// fascia che gli serviva torna alla conversazione.
		assert.ok(
			!html.includes("data-conversation-open"),
			"il pannello ripete in coda il comando di apertura invece di lasciarlo all'elenco",
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
			/non è ancora partito niente/i.test(text),
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
	it("l'esito accettato resta compatto e non offre una vecchia run", () => {
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
			!html.includes("data-conversation-reach-run"),
			"l'esito offre ancora il comando Vai alla run",
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
		// Gli eventi text 2 e 3 sono adiacenti e la timeline li fonde in una
		// riga sola, la cui colonnina porta l'intervallo degli id.
		const mergedRail = railOf("2–3");
		assert.ok(
			html.indexOf(railOf(1)) < first && first < html.indexOf(mergedRail),
			"il primo blocco non è ancorato al proprio evento",
		);
		assert.ok(
			html.indexOf(mergedRail) < second,
			"il secondo blocco non è ancorato al proprio evento",
		);
	});

	// US: la risposta dell'agente è Markdown, e con un parser iniettato la
	// timeline la interpreta invece di mostrarla cruda. Senza parser — il vm
	// nudo di questi test — resta il testo escapato di sempre: gli altri test
	// di questo file sono la prova che il modulo non lancia e non cambia.
	describe("markdown della risposta dell'agente", () => {
		it("il testo dell'agente passa dal parser iniettato, quello di chi scrive no", () => {
			const calls = [];
			const html = renderConversation(
				withConversation({
					events: [
						{ id: 1, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "user_message", text: "# TESTO-TUO" },
						{ id: 2, seq: 2, at: "2026-08-21T10:00:02.000Z", kind: "text", text: "# TESTO-AGENTE" },
					],
					last_id: 2,
				}),
				"",
				{
					markedParse: (md) => {
						calls.push(md);
						return `<b data-md-reso>${md}</b>`;
					},
				},
			);

			assert.deepEqual(calls, ["# TESTO-AGENTE"], "il parser deve vedere solo il testo dell'agente");
			assert.ok(
				html.includes('<b data-md-reso># TESTO-AGENTE</b>'),
				"l'HTML del parser deve arrivare non escapato",
			);
			assert.ok(
				html.includes("# TESTO-TUO"),
				"il messaggio di chi scrive resta testo, mai markup",
			);
			assert.ok(
				html.includes("conv-event-markdown"),
				"il blocco interpretato deve dichiararsi tale, per lo stile che toglie il pre-wrap",
			);
		});

		it("i frammenti adiacenti diventano una chiamata sola, e un messaggio in mezzo li separa", () => {
			const calls = [];
			const parse = (md) => {
				calls.push(md);
				return `<x>${md}</x>`;
			};
			renderConversation(
				withConversation({
					events: [
						{ id: 1, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "text", text: "## Tit" },
						{ id: 2, seq: 2, at: "2026-08-21T10:00:02.000Z", kind: "text", text: "olo\n- voce" },
						{ id: 3, seq: 3, at: "2026-08-21T10:00:03.000Z", kind: "user_message", text: "FRAPPOSTO" },
						{ id: 4, seq: 4, at: "2026-08-21T10:00:04.000Z", kind: "text", text: "coda" },
					],
					last_id: 4,
				}),
				"",
				{ markedParse: parse },
			);

			assert.deepEqual(
				calls,
				["## Titolo\n- voce", "coda"],
				"i delta si ricompongono senza separatore, e il messaggio frapposto chiude la riga",
			);
		});

		it("la colonnina di una riga fusa porta l'intervallo degli id", () => {
			const html = renderConversation(
				withConversation({
					events: [
						{ id: 5, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "text", text: "a" },
						{ id: 6, seq: 2, at: "2026-08-21T10:00:02.000Z", kind: "text", text: "b" },
					],
					last_id: 6,
				}),
				"",
				{ markedParse: (md) => md },
			);
			assert.ok(html.includes(railOf("5–6")), "la riga fusa deve dichiarare #5–6");
		});

		it("senza parser il testo resta escapato e visibile", () => {
			const html = renderConversation(
				withConversation({
					events: [
						{ id: 1, seq: 1, at: "2026-08-21T10:00:01.000Z", kind: "text", text: "**non** <interpretato>" },
					],
					last_id: 1,
				}),
				"",
			);
			assert.ok(
				html.includes("**non** &lt;interpretato&gt;"),
				"senza marked la risposta è il testo escapato di sempre",
			);
			assert.ok(!html.includes("conv-event-markdown"));
		});
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

	it("un'approvazione risolta scompare anche se il payload la elenca ancora", () => {
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

		assert.ok(!html.includes("run-approval-title"), "la carta risolta occupa ancora spazio");
		assert.ok(
			!visibleText(html).includes("git worktree prune --verbose"),
			"il payload della decisione risolta resta nella conversazione",
		);
	});

	it("un'approvazione risolta non viene ricostruita quando non è più pendente", () => {
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

		assert.ok(!html.includes("run-approval-title"), "la carta risolta è stata ricostruita");
		assert.ok(!visibleText(html).includes("OPZIONE-RIFIUTO"));
	});

	it("un'approvazione risolta non lascia copie nel blocco", () => {
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
		assert.equal(cards.length, 0, "la carta risolta è ancora disegnata");
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

	it("un passo già in corso non offre una navigazione alla vecchia run", () => {
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
			!block.includes("data-conversation-reach-run"),
			"il passo già in corso offre ancora Vai alla run",
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

// La testata nomina per intero chi risponde: provider, modello e i valori delle
// opzioni che quel modello dichiara — l'effort fra queste. Il solo
// identificativo del provider diceva il terzo meno interessante del fatto.
describe("l'agente nominato in testata", () => {
	it("nomina il solo provider anche quando il payload porta modello e opzioni", () => {
		// Modello ed effort stanno già sulla riga agente accanto al compositore:
		// ripeterli in testata sarebbe dirli due volte nello stesso pannello.
		const html = renderConversation(
			withConversation({
				provider_id: "fornitore",
				model: "modello",
				model_options: { effort: "LIVELLO-X" },
			}),
			"",
		);

		assert.match(
			html,
			/class="conv-provider"[^>]*>Fornitore</,
			"la testata non nomina il provider",
		);
		assert.ok(
			!/class="conv-provider"[^>]*>[^<]*(Modello|LIVELLO-X)/.test(html),
			"la testata ripete modello o opzioni accanto al provider",
		);
	});

	it("un identificativo composto resta come è scritto", () => {
		// Maiuscolare un identificativo con cifre o trattini produrrebbe un nome
		// che nessuno usa: si maiuscola solo la parola sola.
		const html = renderConversation(
			withConversation({ provider_id: "fornitore-2", model: "modello.9" }),
			"",
		);

		assert.match(
			html,
			/class="conv-provider"[^>]*>fornitore-2</,
			"un identificativo composto è stato riscritto",
		);
	});

	it("senza modello resta il solo provider", () => {
		// Un provider che non dichiara catalogo non lascia una casella vuota:
		// la testata dice quello che sa e nient'altro.
		const html = renderConversation(
			withConversation({ provider_id: "fornitore", model: "" }),
			"",
		);

		assert.match(
			html,
			/class="conv-provider"[^>]*>Fornitore</,
			"senza modello la testata non nomina più nemmeno il provider",
		);
	});
});

// ---------------------------------------------------------------------------
// Gli avvisi si chiudono
//
// Un avviso è una nota sopra alla conversazione: una volta letta, lo spazio che
// occupa appartiene alla conversazione. Quindi ognuno porta la sua X, e chiudere
// significa non disegnarlo — non lasciarne il posto vuoto.
//
// Che la scelta duri è del chiamante: il pannello si ridisegna a ogni lettura,
// e il renderer la riceve come tabella di chiavi. Qui si verifica il patto fra i
// due: la chiave dichiarata dal bottone è la stessa che, passata come chiusa,
// fa sparire il riquadro.
// ---------------------------------------------------------------------------

const CHIUDIBILI = [
	{
		chiave: "over",
		testo: "non è più impegnato",
		view: withConversation({
			conversation: {
				id: "conv-1",
				state: "CLOSED",
				working_dir: "/tmp/DIRECTORY-X",
				opened_at: "2026-08-21T10:00:00.000Z",
			},
		}),
		ui: {},
	},
	{
		chiave: "note",
		testo: "NOTA-DEL-SERVER",
		view: withConversation({ notice: "NOTA-DEL-SERVER", truncated: false }),
		ui: {},
	},
	{
		chiave: "channel",
		testo: "CANALE-X",
		view: LIVE,
		ui: { link: "CANALE-X" },
	},
	{
		chiave: "refusal",
		testo: "RIFIUTO-X",
		view: LIVE,
		ui: { refusal: "RIFIUTO-X" },
	},
	{
		chiave: "error",
		testo: "ERRORE-X",
		view: withConversation({
			conversation: {
				id: "conv-1",
				state: "ACTIVE",
				working_dir: "/tmp/DIRECTORY-X",
				opened_at: "2026-08-21T10:00:00.000Z",
				error: "ERRORE-X",
			},
		}),
		ui: {},
	},
	{
		chiave: "not-offered",
		testo: "RAGIONE-X",
		view: {
			available: false,
			unavailable_reason: "RAGIONE-X",
			conversation: null,
			events: [],
			last_id: 0,
		},
		ui: {},
	},
];

describe("ogni avviso si può chiudere", () => {
	for (const caso of CHIUDIBILI) {
		it(`l'avviso ${caso.chiave} porta il comando per chiuderlo`, () => {
			const html = renderConversation(caso.view, "", caso.ui);
			assert.ok(
				visibleText(html).includes(caso.testo),
				`l'avviso ${caso.chiave} non è nemmeno disegnato: il caso non prova niente`,
			);
			assert.ok(
				html.includes(
					`data-conversation-notice-dismiss="${caso.chiave}"`,
				),
				`l'avviso ${caso.chiave} non offre la X per chiuderlo`,
			);
		});

		it(`l'avviso ${caso.chiave} chiuso non viene disegnato`, () => {
			const chiuso = renderConversation(
				caso.view,
				"",
				Object.assign({}, caso.ui, { dismissed: { [caso.chiave]: true } }),
			);
			assert.ok(
				!visibleText(chiuso).includes(caso.testo),
				`l'avviso ${caso.chiave} torna sullo schermo dopo essere stato chiuso`,
			);
			assert.ok(
				!chiuso.includes(
					`data-conversation-notice-dismiss="${caso.chiave}"`,
				),
				`del riquadro ${caso.chiave} chiuso resta il posto vuoto`,
			);
		});
	}

	it("chiudere un avviso non ne chiude un altro", () => {
		const html = renderConversation(LIVE, "", {
			link: "CANALE-X",
			refusal: "RIFIUTO-X",
			dismissed: { channel: true },
		});
		const text = visibleText(html);
		assert.ok(!text.includes("CANALE-X"), "l'avviso chiuso è ancora lì");
		assert.ok(
			text.includes("RIFIUTO-X"),
			"chiudere un avviso ne ha portato via un altro",
		);
	});

	it("senza tabella degli avvisi chiusi li disegna tutti", () => {
		// Il chiamante può non passarla affatto: il pannello deve dire tutto
		// quello che sa, non tacere per via di un argomento mancante.
		const html = renderConversation(LIVE, "", { link: "CANALE-X" });
		assert.ok(visibleText(html).includes("CANALE-X"));
	});
});

// ---------------------------------------------------------------------------
// Dove arriva la risposta a conversazione finita
//
// Una conversazione finita non è muta: ci si scrive per riprenderla, e ciò che
// ne esce è una conversazione nuova. È il fatto da sapere prima di premere
// Invio, quindi sta su una riga propria sopra al campo — non stretto accanto,
// dove divideva la riga con ciò che si scrive ed era il primo a essere troncato.
// ---------------------------------------------------------------------------

describe("la ripresa di una conversazione finita", () => {
	const CHIUSA = withConversation({
		conversation: {
			id: "conv-1",
			state: "CLOSED",
			working_dir: "/tmp/DIRECTORY-X",
			opened_at: "2026-08-21T10:00:00.000Z",
		},
	});

	it("dice che la risposta arriverà in una conversazione nuova", () => {
		const text = visibleText(renderConversation(CHIUSA, "", {}));
		assert.match(
			text,
			/conversazione nuova/,
			"non dice dove andrà a finire ciò che si sta per scrivere",
		);
	});

	it("lo dice sopra al campo e non nella riga del campo", () => {
		const html = renderConversation(CHIUSA, "", {});
		const idxNota = html.indexOf("conv-resume-note");
		const idxRiga = html.indexOf("conv-composer-row");
		assert.ok(idxNota !== -1, "la nota di ripresa non è disegnata");
		assert.ok(idxRiga !== -1, "la riga del compositore non è disegnata");
		assert.ok(
			idxNota < idxRiga,
			"la nota è finita dentro la riga del campo invece di stare sopra",
		);
		// La riga del campo un suggerimento ce l'ha sempre, ma è il suo — dice
		// che non si scrive — e non una seconda copia della nota qui sopra.
		const riga = html.slice(idxRiga);
		assert.ok(
			!/conv-composer-hint[^]*conversazione nuova/.test(riga),
			"la nota di ripresa è ripetuta anche nella riga del campo",
		);
	});

	it("la nota porta il comando per chiuderla, e chiusa non viene disegnata", () => {
		const aperta = renderConversation(CHIUSA, "", {});
		assert.ok(
			aperta.includes('data-conversation-notice-dismiss="resume-note"'),
			"la nota di ripresa non offre la X per chiuderla",
		);
		const chiusa = renderConversation(CHIUSA, "", {
			dismissed: { "resume-note": true },
		});
		assert.ok(
			!chiusa.includes("conv-resume-note"),
			"la nota di ripresa torna sullo schermo dopo essere stata chiusa",
		);
		assert.ok(
			chiusa.includes("conv-composer-row"),
			"chiudere la nota si è portata via anche il compositore",
		);
	});

	it("una conversazione viva non porta la nota di ripresa", () => {
		const html = renderConversation(LIVE, "", {});
		assert.ok(
			!html.includes("conv-resume-note"),
			"la nota di ripresa compare in una conversazione che non è finita",
		);
	});

	it("il suggerimento dei tasti nomina invio e maiusc+invio", () => {
		// La scorciatoia è cambiata: Invio manda, Maiusc+Invio va a capo. Il
		// suggerimento accanto al campo deve dire quella, non un'altra.
		const text = visibleText(renderConversation(LIVE, "", {}));
		assert.match(text, /invio: invia/);
		assert.match(text, /maiusc\+invio: a capo/);
	});

	it("il suggerimento del campo c'è anche a conversazione finita", () => {
		// Non è la stessa frase — su una conversazione chiusa non si manda
		// niente — ma la riga non resta mai muta: dice che è di sola lettura.
		const html = renderConversation(
			Object.assign({}, LIVE, {
				conversation: Object.assign({}, LIVE.conversation, {
					state: "CLOSED",
				}),
			}),
			"",
			{},
		);
		assert.ok(
			html.includes("conv-composer-hint"),
			"la riga del compositore resta senza suggerimento",
		);
		assert.match(visibleText(html), /sola lettura/);
	});
	// ---- Il dettaglio tecnico, ripiegato ---------------------------------
	// Una conversazione la si legge per le risposte dell'agente. Gli strumenti
	// che ha aperto e il ragionamento con cui c'è arrivato restano leggibili,
	// ma un passo indietro: gli oracoli qui sotto guardano il testo visibile,
	// cioè quello che si legge senza premere niente.

	const CON_STRUMENTI = withConversation({
		events: [
			{ id: 1, at: "2026-08-21T10:00:01.000Z", kind: "user_message", text: "DOMANDA-MIA" },
			{ id: 2, at: "2026-08-21T10:00:02.000Z", kind: "thinking", text: "RAGIONAMENTO-INTERNO" },
			{ id: 3, at: "2026-08-21T10:00:03.000Z", kind: "tool_start", tool: "STRUMENTO-UNO", text: "AVVIO-STRUMENTO" },
			{ id: 4, at: "2026-08-21T10:00:04.000Z", kind: "tool_end", tool: "STRUMENTO-UNO", text: "ESITO-STRUMENTO" },
			{ id: 5, at: "2026-08-21T10:00:05.000Z", kind: "text", text: "RISPOSTA-AGENTE" },
		],
		last_id: 5,
	});

	it("i passaggi tecnici sono ripiegati e la risposta dell'agente no", () => {
		const text = visibleText(renderConversation(CON_STRUMENTI, "", {}));
		assert.match(text, /RISPOSTA-AGENTE/, "la risposta dell'agente non si legge");
		assert.match(text, /DOMANDA-MIA/, "la frase di chi scrive non si legge");
		assert.ok(
			!text.includes("RAGIONAMENTO-INTERNO"),
			"il ragionamento è in chiaro invece che dietro la piega",
		);
		assert.ok(
			!text.includes("ESITO-STRUMENTO"),
			"l'esito dello strumento è in chiaro invece che dietro la piega",
		);
	});

	it("la piega dice quanti passaggi contiene e quali strumenti ha toccato", () => {
		const text = visibleText(renderConversation(CON_STRUMENTI, "", {}));
		assert.match(text, /3 passaggi tecnici/, "la piega non conta i passaggi");
		assert.match(text, /STRUMENTO-UNO/, "la piega non nomina lo strumento toccato");
		// Lo strumento è nominato una volta sola anche se compare due volte:
		// avvio e fine sono lo stesso strumento.
		assert.equal(
			(text.match(/STRUMENTO-UNO/g) || []).length,
			1,
			"lo stesso strumento è nominato più di una volta nella riga di comando",
		);
	});

	it("aperta, la piega rende esattamente le righe che teneva", () => {
		const chiusa = renderConversation(CON_STRUMENTI, "", {});
		const chiave = (chiusa.match(/data-conversation-technical-toggle="([^"]+)"/) || [])[1];
		assert.ok(chiave, "la piega non dichiara la chiave con cui si apre");
		const aperta = visibleText(
			renderConversation(CON_STRUMENTI, "", { technicalOpen: { [chiave]: true } }),
		);
		assert.match(aperta, /RAGIONAMENTO-INTERNO/, "aperta, la piega non mostra il ragionamento");
		assert.match(aperta, /ESITO-STRUMENTO/, "aperta, la piega non mostra l'esito dello strumento");
		assert.match(aperta, /RISPOSTA-AGENTE/, "aprire la piega si è portata via la risposta");
	});

	it("il comando della testata apre il dettaglio di tutta la conversazione", () => {
		const html = renderConversation(CON_STRUMENTI, "", {});
		assert.ok(
			html.includes("data-conversation-technical-all"),
			"la testata non offre il comando del dettaglio tecnico",
		);
		const tutto = visibleText(
			renderConversation(CON_STRUMENTI, "", { technicalAll: true }),
		);
		assert.match(tutto, /RAGIONAMENTO-INTERNO/);
		assert.match(tutto, /ESITO-STRUMENTO/);
	});

	it("il comando della testata dice il proprio nome e l'azione che compie", () => {
		// Era un'icona sola, e di che cosa parlasse lo diceva soltanto il titolo
		// del browser: chi non ha un mouse non lo legge mai. Il nome sta scritto
		// sul pulsante, e il titolo dice che cosa succede a premerlo — che è
		// l'opposto a seconda di come sta.
		const chiusa = renderConversation(CON_STRUMENTI, "", {});
		assert.match(
			visibleText(chiusa),
			/Dettaglio tecnico/,
			"il comando della testata non porta scritto di che cosa parla",
		);
		assert.match(
			chiusa,
			/data-conversation-technical-all aria-pressed="false" title="Mostra sempre il dettaglio tecnico"/,
			"da chiuso il comando non dice che aprirebbe il dettaglio",
		);
		const aperta = renderConversation(CON_STRUMENTI, "", { technicalAll: true });
		assert.match(
			visibleText(aperta),
			/Dettaglio tecnico/,
			"aperto, il comando perde il proprio nome",
		);
		assert.match(
			aperta,
			/data-conversation-technical-all aria-pressed="true" title="Ripiega il dettaglio tecnico"/,
			"da aperto il comando non dice che ripiegherebbe il dettaglio",
		);
	});

	it("una piega che contiene un errore lo dichiara da chiusa", () => {
		const html = renderConversation(
			withConversation({
				events: [
					{ id: 1, kind: "tool_error", tool: "STRUMENTO-DUE", text: "DETTAGLIO-ERRORE" },
					{ id: 2, kind: "text", text: "RISPOSTA-AGENTE" },
				],
				last_id: 2,
			}),
			"",
			{},
		);
		const text = visibleText(html);
		assert.match(text, /con errore/, "la piega chiusa tace l'errore che contiene");
		assert.ok(
			!text.includes("DETTAGLIO-ERRORE"),
			"il dettaglio dell'errore è in chiaro invece che dietro la piega",
		);
	});

	it("un errore della conversazione non è un passaggio tecnico", () => {
		// Il vocabolario porta anche `error`, che non è la lavorazione ma una
		// risposta: si legge senza premere niente.
		const text = visibleText(
			renderConversation(
				withConversation({
					events: [{ id: 1, kind: "error", text: "ERRORE-DELLA-CONVERSAZIONE" }],
					last_id: 1,
				}),
				"",
				{},
			),
		);
		assert.match(text, /ERRORE-DELLA-CONVERSAZIONE/);
	});

	it("i ganci di apertura sono ripiegati, e la piega li nomina", () => {
		// Ogni conversazione vera si apre con due righe che nessun provider
		// traduce — i ganci scattati all'avvio dell'agente — e prima stavano in
		// chiaro sopra alla prima cosa detta. Sono la lavorazione con un nome
		// che il pannello non sa leggere: ripiegate, ma nominate, altrimenti la
		// piega non fa sapere se val la pena aprirla.
		const html = renderConversation(
			withConversation({
				events: [
					{ id: 1, kind: "system/hook_started" },
					{ id: 2, kind: "system/hook_response" },
					{ id: 3, kind: "user_message", text: "DOMANDA-MIA" },
					{ id: 4, kind: "text", text: "RISPOSTA-AGENTE" },
				],
				last_id: 4,
			}),
			"",
			{},
		);
		const text = visibleText(html);
		assert.match(text, /2 passaggi tecnici/, "i due ganci non sono in una piega sola");
		assert.match(
			text,
			/system\/hook_started · system\/hook_response/,
			"la piega non dice quali righe tiene",
		);
		assert.match(text, /DOMANDA-MIA/);
		assert.match(text, /RISPOSTA-AGENTE/);
	});

	it("un tipo sconosciuto è ripiegato ma non perso", () => {
		const events = [{ id: 1, kind: "tipo-mai-visto", text: "TESTO-MAI-VISTO" }];
		const chiusa = visibleText(
			renderConversation(withConversation({ events, last_id: 1 }), "", {}),
		);
		assert.ok(
			!chiusa.includes("TESTO-MAI-VISTO"),
			"un tipo sconosciuto sta in chiaro fra le risposte",
		);
		assert.match(chiusa, /tipo-mai-visto/, "la piega non nomina il tipo che tiene");
		const aperta = visibleText(
			renderConversation(withConversation({ events, last_id: 1 }), "", {
				technicalAll: true,
			}),
		);
		assert.match(aperta, /TESTO-MAI-VISTO/, "aperta, la piega non rende il testo");
	});

	it("anche il log di una run e' ripiegato, e la decisione che aspetta no", () => {
		// Il log di una run e' la stessa cosa dei passaggi tecnici della
		// timeline: righe che dicono come si e' lavorato. Cio' che le sta
		// intorno no — una run ferma su una domanda deve restare visibile per
		// intero, altrimenti la piega nasconde la cosa da fare.
		const CON_RUN = withConversation({
			events: [{ id: 1, kind: "user_message", text: "DOMANDA-MIA" }],
			runs: [
				{
					execution_id: "ESECUZIONE-UNO",
					anchor_event_id: 1,
					action: "azione-inventata",
					label: "AZIONE-INVENTATA",
					scope: "spec",
					spec_code: "CODICE-X",
					run: { id: "run-1", state: "ACTIVE" },
					events: [
						{ id: 1, kind: "tool_start", tool: "STRUMENTO-QUATTRO", text: "RIGA-DI-LOG" },
					],
					approvals: [
						{
							id: "decisione-1",
							tool_name: "STRUMENTO-QUATTRO",
							args: { comando: "COMANDO-DA-APPROVARE" },
							options: [{ id: "allow", label: "ETICHETTA-CONSENSO", kind: "allow" }],
						},
					],
					awaiting_response: true,
				},
			],
			last_id: 1,
		});

		const chiusa = renderConversation(CON_RUN, "", {});
		assert.ok(
			!chiusa.includes('<pre class="conv-run-log"'),
			"il log della run e' aperto per difetto",
		);
		const testo = visibleText(chiusa);
		assert.match(testo, /1 riga di log/, "la piega non conta le righe del log");
		assert.match(testo, /STRUMENTO-QUATTRO/, "la piega non nomina lo strumento");
		assert.ok(
			!testo.includes("RIGA-DI-LOG"),
			"la riga di log e' in chiaro invece che dietro la piega",
		);
		// Cio' che la run aspetta resta in chiaro: il comando e la risposta che
		// si puo' dare.
		assert.match(testo, /COMANDO-DA-APPROVARE/, "il comando in attesa e' stato ripiegato");
		assert.match(testo, /ETICHETTA-CONSENSO/, "la risposta da dare e' stata ripiegata");

		const aperta = renderConversation(CON_RUN, "", {
			technicalOpen: { "rESECUZIONE-UNO": true },
		});
		assert.ok(
			aperta.includes('<pre class="conv-run-log"'),
			"aperta, la piega non mostra il log della run",
		);
		assert.match(visibleText(aperta), /RIGA-DI-LOG/);
	});

	// ---- L'attesa dell'agente ---------------------------------------------

	it("mentre l'agente lavora la conversazione lo dichiara, e dice a che cosa", () => {
		const text = visibleText(
			renderConversation(LIVE, "", {
				working: { active: true, kind: "tool_start", tool: "STRUMENTO-TRE", seconds: 5 },
			}),
		);
		assert.match(text, /sta usando STRUMENTO-TRE/);
		assert.match(text, /5s/, "l'attesa non dice da quanto dura");
	});

	it("senza attività ancora, l'attesa dice comunque che si sta lavorando", () => {
		// Fra il messaggio consegnato e la prima riga dell'agente non si sa a
		// che cosa stia lavorando, e la frase generica dice esattamente quello.
		// Qui prima c'era «ha ricevuto il messaggio», che è una frase finita: si
		// leggeva come un lavoro concluso proprio quando il lavoro comincia.
		const text = visibleText(
			renderConversation(LIVE, "", { working: { active: true, kind: "" } }),
		);
		assert.match(text, /sta lavorando/);
		assert.ok(
			!text.includes("ricevuto il messaggio"),
			"l'attesa si dichiara con una frase finita",
		);
	});

	it("il ragionamento e la scrittura hanno parole proprie", () => {
		const ragiona = visibleText(
			renderConversation(LIVE, "", { working: { active: true, kind: "thinking" } }),
		);
		assert.match(ragiona, /sta ragionando/);
		const scrive = visibleText(
			renderConversation(LIVE, "", { working: { active: true, kind: "text" } }),
		);
		assert.match(scrive, /sta scrivendo/);
	});

	it("a turno chiuso la riga dell'attesa non c'è", () => {
		assert.ok(
			!renderConversation(LIVE, "", {}).includes("conv-working"),
			"la riga dell'attesa è disegnata senza che nessuno stia aspettando",
		);
		assert.ok(
			!renderConversation(LIVE, "", { working: { active: false } }).includes(
				"conv-working",
			),
			"un'attesa dichiarata finita resta sullo schermo",
		);
	});

	it("l'attesa si legge in secondi e, superato il minuto, in minuti", () => {
		assert.equal(formatElapsed(0), "0s");
		assert.equal(formatElapsed(42), "42s");
		assert.equal(formatElapsed(60), "1m 00s");
		assert.equal(formatElapsed(125), "2m 05s");
	});
});

// La decisione della conversazione stessa: l'agente si è fermato a chiedere il
// permesso di usare uno strumento, e non c'è nessuna run su cui rispondere.
// Senza una carta nel thread la conversazione resterebbe ferma per sempre —
// l'agente aspetta una risposta che il visore non ha mai offerto.
describe("renderConversation — la decisione della conversazione", () => {
	it("un permesso in attesa si legge in fondo al thread, senza run", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, approvals: [APPROVAL] }),
			"",
		);

		assert.ok(
			html.includes("conv-approvals"),
			"la decisione della conversazione non è disegnata",
		);
		assert.ok(
			visibleText(html).includes("git worktree prune --verbose"),
			"il comando da giudicare non si legge in chiaro",
		);
		// Nessuna run: i bottoni non nominano nessuna esecuzione, ed è così che
		// il chiamante sa di dover rispondere sulla conversazione.
		const buttons = html.match(/<button[^>]*data-run-approval-id="appr-1"[^>]*>/g) || [];
		assert.equal(buttons.length, 2, "le due opzioni dichiarate non ci sono");
		for (const button of buttons) {
			assert.ok(
				/data-execution-id=""/.test(button),
				`un permesso della conversazione nomina una run: ${button}`,
			);
		}
	});

	it("il compositore dichiara l'attesa anche senza run", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, approvals: [APPROVAL] }),
			"",
		);
		assert.ok(
			html.includes("conv-composer-await"),
			"il compositore non dice che si sta aspettando una risposta",
		);
	});

	it("la decisione presa scompare quando non è più pendente", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, approvals: [] }),
			"",
			{
				answeredApprovals: {
					"appr-1": {
						optionID: "deny-once",
						label: "OPZIONE-RIFIUTO",
						denied: true,
						executionID: "",
						conversation: true,
						approval: APPROVAL,
					},
				},
			},
		);

		assert.ok(!html.includes("conv-approvals"), "la decisione risolta occupa ancora spazio");
		assert.ok(!visibleText(html).includes("OPZIONE-RIFIUTO"));
	});

	it("una decisione della conversazione non compare dentro il blocco di una run", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				approvals: [],
				runs: [runAt(2, { approvals: [] })],
			}),
			"",
			{
				answeredApprovals: {
					"appr-1": {
						optionID: "allow-once",
						label: "OPZIONE-CONSENSO",
						executionID: "",
						conversation: true,
						approval: APPROVAL,
					},
				},
			},
		);

		const cards = html.match(/run-approval-title/g) || [];
		assert.equal(cards.length, 0, "la decisione risolta è ancora disegnata");
	});

	it("senza permessi in attesa non c'è nessuna carta e nessuna attesa", () => {
		const html = renderConversation(
			withConversation({ events: RUN_EVENTS, last_id: 3, approvals: [] }),
			"",
		);
		assert.ok(!html.includes("conv-approvals"), "una carta è disegnata senza decisione");
		assert.ok(
			!html.includes("conv-composer-await"),
			"il compositore dichiara un'attesa che non c'è",
		);
	});
});

// Un passo che si legge in una conversazione sua non si cita qui.
//
// Dopo l'unificazione la run *è* la conversazione: una sessione, un agente. Un
// blocco che ne quotasse tutto il log dentro la conversazione che l'ha chiesto
// disegnerebbe lo stesso agente due volte nella stessa pagina, con due
// compositori che scrivono in un turno solo. Quello che il blocco tiene è ciò
// che questa conversazione sa davvero: di aver chiesto il passo, e dove il
// passo sta accadendo.
describe("renderConversation — il passo che ha una conversazione sua", () => {
	it("il blocco nomina la conversazione invece di citarne il log", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [
					runAt(2, {
						thread_id: "exec-abc",
						events: [],
						approvals: [],
					}),
				],
			}),
			"",
		);

		assert.ok(
			html.includes('data-conversation-reach-thread="exec-abc"'),
			"il blocco non offre la strada verso la conversazione del passo",
		);
		assert.ok(
			!html.includes("data-conversation-reach-run"),
			"il blocco porta ancora alla vecchia strada: il pannello della run per un passo che ha una conversazione sua è vuoto",
		);
		assert.ok(
			!html.includes("conv-run-log-toggle"),
			"il blocco disegna ancora la piega del log: sarebbe lo stesso agente reso due volte",
		);
	});

	it("un passo senza conversazione sua tiene la piega del log e la vecchia strada", () => {
		const html = renderConversation(
			withConversation({
				events: RUN_EVENTS,
				last_id: 3,
				runs: [runAt(2, { events: [{ id: 1, kind: "text", text: "X" }] })],
			}),
			"",
		);

		assert.ok(
			html.includes("data-conversation-reach-run"),
			"un passo dispatch-based ha perso la strada verso il suo pannello",
		);
		assert.ok(
			html.includes("conv-run-log-toggle"),
			"un passo dispatch-based ha perso la piega del log: non si leggerebbe da nessuna parte",
		);
	});
});
