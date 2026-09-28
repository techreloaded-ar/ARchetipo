// test/web/conversation-index.test.mjs
// Tests for the pure conversation-index renderer used by the ARchetipo web viewer.
// Run: node --test test/web/conversation-index.test.mjs
//
// Same discipline as conversation.test.mjs: the oracles are on the *visible
// text* of the rendered HTML, not on the shape of the module. What the person
// reading the thread rail actually sees — the title of a past conversation,
// how long ago it was last spoken in, the spec it came from, whether the rail
// says out loud that there is nothing yet — is what the acceptance criteria
// are about. The one exception is the purity check, where the criterion is the
// absence of DOM, network and timers in the module itself.
//
// Verifies:
//   - AC-2 every entry shows its title, when it was last spoken in, and the
//     code of the spec it was born from
//   - AC-5 a conversation tied to no spec carries no spec code, and no
//     placeholder in its place
//   - AC-7 a workspace with no past conversation says so and offers to open
//     the first one
//   - AC-4 a resumed conversation declares that it is a new one carrying the
//     earlier one as context, and names it
//   - ogni nota del pannello — la dichiarazione di ripresa e il rifiuto della
//     colonna — porta il comando per chiuderla
//   - a group with no members leaves no orphan heading
//   - a conversation whose turn is in flight is listed under "Al lavoro" even
//     when it has a spec code; one that is idle, or waiting for an answer, is
//     listed under "Aperte" — whether or not a process is attached to it
//   - US-059 AC-3 several working conversations stand together under "Al
//     lavoro" while an archived one stays out of that group
//   - a thread waiting for the person says so, and a thread with a reply not
//     yet read carries the mark — and neither is declared "in corso"
//   - US-059 AC-3 only the conversation on screen is marked current, even when
//     more than one is live
//   - US-059 AC-5 the limit notice renders the server's refusal verbatim, knows
//     no number of its own, and escapes what it is given
//   - the conversation on screen is the only one marked as current
//   - payload text is escaped, never interpreted as markup
//   - the module is pure: no DOM, no network, no timers

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
	"conversation-index.js",
);
const appPath = resolve(dirname(helperPath), "app.js");

// Same minimal virtual-machine loader as conversation.test.mjs: the UMD module
// detects `module.exports` first, so the Node branch is enough — and a module
// that reached for `document`, `window` or `fetch` would fail here, because
// this realm has none of them. Loading the very file the viewer serves is the
// point: the test proves the shipped asset, not a copy of it.
function loadConversationIndex() {
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

const {
	renderConversationIndex,
	relativeTime,
	renderResumeBanner,
	renderLimitNotice,
} = loadConversationIndex();

// Strip every attribute from the markup, leaving only what a reader sees.
// A sentence that survives this is visible text; one that does not was only
// ever hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

// The markup of the single thread whose title is `title`. Lets an assertion
// speak about one entry — "this one carries no code" — instead of about the
// whole rail, where another entry's code would satisfy it by accident.
function threadWithTitle(html, title) {
	// Only the thread button itself: the trash beside it names the same
	// conversation in its label, and counting it too would make every
	// "exactly one" assertion below fail on a second copy of the same thread.
	const buttons = html
		.split("<button")
		.slice(1)
		.filter((chunk) => chunk.includes("data-conversation-id"));
	const found = buttons.filter((chunk) => chunk.includes(title));
	assert.equal(
		found.length,
		1,
		`expected exactly one thread titled ${title}`,
	);
	return found[0];
}

const NOW = new Date("2026-08-22T12:00:00.000Z");
const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

function ago(ms) {
	return new Date(NOW.getTime() - ms).toISOString();
}

describe("renderConversationIndex", () => {
	it("mostra titolo, momento dell'ultimo messaggio e codice della spec (AC-2)", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-1",
						title: "Perché la board ha due colonne",
						last_message_at: ago(2 * HOUR),
						spec_code: "US-058",
					},
				],
			},
			{ now: NOW },
		);

		assert.match(html, /Perché la board ha due colonne/);
		assert.match(html, /2 ore fa/);
		assert.match(html, /US-058/);
		// All three are readable, not buried in attributes.
		const visible = visibleText(html);
		assert.match(visible, /Perché la board ha due colonne/);
		assert.match(visible, /2 ore fa/);
		assert.match(visible, /US-058/);
	});

	it("distingue la conversazione libera da quella legata a una spec (AC-5)", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-legata",
						title: "Conversazione della spec",
						last_message_at: ago(2 * HOUR),
						spec_code: "US-058",
					},
					{
						id: "conv-libera",
						title: "Conversazione di progetto",
						last_message_at: ago(3 * HOUR),
					},
				],
			},
			{ now: NOW },
		);

		// Le due stanno nello stesso gruppo: quello che le distingue è il codice
		// sopra al titolo, non un'intestazione che le separa.
		assert.match(visibleText(html), /Aperte/);
		assert.doesNotMatch(visibleText(html), /Progetto/);

		const free = threadWithTitle(html, "Conversazione di progetto");
		assert.match(free, /is-free/);
		assert.doesNotMatch(free, /US-\d/);
		// E soprattutto: niente riquadro al posto del codice che non c'è.
		assert.doesNotMatch(free, /thread-code/);

		const tied = threadWithTitle(html, "Conversazione della spec");
		assert.match(tied, /US-058/);
		assert.doesNotMatch(tied, /is-free/);
	});

	it("mette il codice della spec sopra al titolo, non accanto (AC-5)", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-legata",
						title: "Conversazione della spec",
						last_message_at: ago(2 * HOUR),
						spec_code: "US-058",
					},
				],
			},
			{ now: NOW },
		);

		const tied = threadWithTitle(html, "Conversazione della spec");
		assert.ok(
			tied.indexOf("thread-code") < tied.indexOf("thread-title"),
			"the spec code must be rendered before the title it stands over",
		);
	});

	it("non lascia intestazioni orfane sopra un gruppo vuoto", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-a",
						title: "Prima libera",
						last_message_at: ago(2 * HOUR),
					},
					{
						id: "conv-b",
						title: "Seconda libera",
						last_message_at: ago(4 * HOUR),
					},
				],
			},
			{ now: NOW },
		);

		assert.match(html, /Aperte/);
		assert.doesNotMatch(html, /Al lavoro/);
	});

	it("elenca sotto «Al lavoro» la conversazione con un turno in corso, anche se ha un codice spec", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-viva",
						title: "Quella che lavora adesso",
						last_message_at: ago(5 * 60 * 1000),
						spec_code: "US-058",
						work: "TURN_ACTIVE",
						live: true,
					},
					{
						id: "conv-ferma",
						title: "Quella ferma da ieri",
						last_message_at: ago(30 * HOUR),
						spec_code: "US-057",
						work: "IDLE",
						live: true,
					},
				],
			},
			{ now: NOW },
		);

		const visible = visibleText(html);
		assert.match(visible, /Al lavoro/);

		const working = threadWithTitle(html, "Quella che lavora adesso");
		assert.match(visibleText(working), /sta lavorando/);

		// The working one is drawn first, under its own heading, and the open
		// group opens only after it.
		assert.ok(
			html.indexOf("Al lavoro") < html.indexOf("Quella che lavora adesso"),
		);
		assert.ok(
			html.indexOf("Quella che lavora adesso") < html.indexOf("Quella ferma da ieri"),
		);
		// Ferma con un processo attaccato: e' aperta, non «in corso». Il
		// processo e' un fatto dell'infrastruttura, non della conversazione.
		const idle = threadWithTitle(html, "Quella ferma da ieri");
		assert.doesNotMatch(visibleText(idle), /sta lavorando|in corso/);
		assert.ok(
			html.indexOf("Aperte") < html.indexOf("Quella ferma da ieri"),
			"an idle thread stands under Aperte even while a runtime is attached to it",
		);
	});

	it("una conversazione ferma senza processo sta fra le aperte, non fra le archiviate", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{ id: "conv-rilasciata", title: "Rilasciata da ferma", work: "IDLE", connection: "RELEASED", live: false },
				],
			},
			{ now: NOW },
		);
		assert.match(visibleText(html), /Aperte/);
		assert.doesNotMatch(visibleText(html), /Archiviate/);
		assert.doesNotMatch(threadWithTitle(html, "Rilasciata da ferma"), /is-closed/);
	});

	it("dice quando una conversazione aspetta chi legge, senza dichiararla al lavoro", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{ id: "conv-domanda", title: "Ha una domanda", work: "WAITING_INPUT", live: true },
					{ id: "conv-permesso", title: "Chiede un permesso", work: "WAITING_APPROVAL", live: true },
				],
			},
			{ now: NOW },
		);
		assert.doesNotMatch(visibleText(html), /Al lavoro/);
		assert.match(visibleText(threadWithTitle(html, "Ha una domanda")), /aspetta te/);
		assert.match(visibleText(threadWithTitle(html, "Chiede un permesso")), /aspetta te/);
		assert.ok(html.indexOf("Aperte") < html.indexOf("Ha una domanda"));
	});

	it("segna con il puntino la conversazione con una risposta da leggere", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{ id: "conv-nuova", title: "Ha risposto", work: "IDLE", message_count: 4 },
					{ id: "conv-letta", title: "Letta ieri", work: "IDLE", message_count: 2 },
				],
			},
			{ now: NOW, unread: { "conv-nuova": true } },
		);
		const fresh = threadWithTitle(html, "Ha risposto");
		assert.match(fresh, /is-unread/);
		assert.match(fresh, /aria-label="da leggere"/);
		assert.doesNotMatch(threadWithTitle(html, "Letta ieri"), /is-unread/);
		// Da leggere non vuol dire in corso: nessuna etichetta di lavoro.
		assert.doesNotMatch(visibleText(fresh), /sta lavorando|in corso/);
	});

	it("rende compatte le archiviate e lascia prominenti le aperte", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{id: "open", title: "Conversazione aperta", work: "IDLE"},
					{id: "closed", title: "Archiviata", archived: true},
				],
			},
			{ now: NOW },
		);
		assert.match(threadWithTitle(html, "Conversazione aperta"), /is-live/);
		assert.doesNotMatch(threadWithTitle(html, "Conversazione aperta"), /is-closed/);
		assert.match(threadWithTitle(html, "Archiviata"), /is-closed/);
	});

	it("offre di caricare le precedenti soltanto quando il server ne dichiara altre", () => {
		const entry = { id: "closed", title: "Conclusa", live: false };
		const more = renderConversationIndex(
			{ conversations: [entry], has_more_closed: true },
			{ now: NOW },
		);
		assert.match(more, /data-conversation-more/);
		assert.match(visibleText(more), /Carica conversazioni precedenti/);

		const complete = renderConversationIndex(
			{ conversations: [entry], has_more_closed: false },
			{ now: NOW },
		);
		assert.doesNotMatch(complete, /data-conversation-more/);
	});

	it("dichiara che non c'è ancora nessuna conversazione e offre di avviarne una (AC-7)", () => {
		const html = renderConversationIndex({ conversations: [] }, { now: NOW });

		assert.match(
			visibleText(html),
			/Su questo workspace non c'è ancora nessuna conversazione\./,
		);
		assert.match(html, /data-conversation-new/);
	});

	it("marca come corrente solo la conversazione mostrata", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-1",
						title: "Quella aperta",
						last_message_at: ago(2 * HOUR),
					},
					{
						id: "conv-2",
						title: "Quella accanto",
						last_message_at: ago(3 * HOUR),
					},
				],
			},
			{ currentId: "conv-1", now: NOW },
		);

		assert.match(threadWithTitle(html, "Quella aperta"), /is-current/);
		assert.doesNotMatch(threadWithTitle(html, "Quella accanto"), /is-current/);
	});

	it("scrive il testo del payload, non lo interpreta come markup", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-1",
						title: "<script>alert(1)</script>",
						last_message_at: ago(2 * HOUR),
					},
				],
			},
			{ now: NOW },
		);

		assert.match(html, /&lt;script&gt;alert\(1\)&lt;\/script&gt;/);
		assert.doesNotMatch(html, /<script>/);
	});
});

describe("cablaggio della rail", () => {
	it("preserva lo scroll quando il polling ridisegna l'indice", () => {
		const source = readFileSync(appPath, "utf8");
		assert.match(source, /const previousTop = previousList \? previousList\.scrollTop : 0/);
		assert.match(source, /nextList\.scrollTop = previousTop/);
		assert.match(source, /closed_limit=\$\{conversationsClosedLimit\}/);
	});
});

describe("renderResumeBanner", () => {
	it("dichiara la ripresa e nomina la conversazione ripresa (AC-4)", () => {
		const html = renderResumeBanner({
			conversation: { resumed_from: "conv-vecchia" },
		});

		const visible = visibleText(html);
		assert.match(visible, /riprende/);
		assert.match(visible, /conv-vecchia/);
		assert.match(visible, /contesto/);
	});

	it("resta muto su una conversazione ordinaria", () => {
		assert.equal(renderResumeBanner({ conversation: { id: "conv-1" } }), "");
		assert.equal(renderResumeBanner(null), "");
	});

	it("porta il comando per chiuderlo, come ogni altra nota del pannello", () => {
		// È una dichiarazione scritta per essere letta una volta: dopo, lo
		// spazio che occupa appartiene alla conversazione. La chiave è quella
		// con cui il pannello ricorda che è stata chiusa.
		const html = renderResumeBanner({
			conversation: { resumed_from: "conv-vecchia" },
		});

		assert.match(html, /data-conversation-notice-dismiss="resume"/);
		assert.match(html, /class="resume-banner-dismiss"/);
		assert.match(html, /aria-label="Chiudi questo avviso"/);
	});
});

describe("renderConversationIndex con più conversazioni al lavoro", () => {
	// The payload of a workspace holding two working conversations — one tied
	// to a spec, one free — beside a third that was archived. It is the state
	// US-059 introduces, and the one the singular rail could not describe.
	const twoLiveAndOneClosed = {
		conversations: [
			{
				id: "conv-viva-spec",
				title: "Implementiamo US-059",
				last_message_at: ago(2 * 60 * 1000),
				spec_code: "US-059",
				work: "TURN_ACTIVE",
				live: true,
			},
			{
				id: "conv-viva-libera",
				title: "Che ne pensi della rail",
				last_message_at: ago(10 * 60 * 1000),
				spec_code: "",
				work: "TURN_ACTIVE",
				live: true,
			},
			{
				id: "conv-chiusa",
				title: "Conversazione finita ieri",
				last_message_at: ago(30 * HOUR),
				spec_code: "US-058",
				state: "ARCHIVED",
				archived: true,
				live: false,
			},
		],
	};

	// The markup of one group, from its heading to the next heading or the end.
	// It lets an assertion speak about "what stands under In corso" instead of
	// about the whole rail, where an entry of another group would satisfy it by
	// accident.
	function groupBody(html, label) {
		const opening = `<p class="rail-group">${label}</p>`;
		const start = html.indexOf(opening);
		assert.notEqual(start, -1, `the group ${label} is not rendered`);
		const after = html.slice(start + opening.length);
		const next = after.indexOf(`<p class="rail-group">`);
		return next === -1 ? after : after.slice(0, next);
	}

	it("tiene due conversazioni al lavoro insieme sotto «Al lavoro» e l'archiviata fuori (AC-3)", () => {
		const html = renderConversationIndex(twoLiveAndOneClosed, { now: NOW });

		const live = groupBody(html, "Al lavoro");
		assert.ok(
			live.includes("conv-viva-spec"),
			"the live conversation tied to a spec is missing from the live group",
		);
		assert.ok(
			live.includes("conv-viva-libera"),
			"the free live conversation is missing from the live group",
		);
		assert.ok(
			!live.includes("conv-chiusa"),
			"a conversation that has ended must not stand under Al lavoro",
		);
		// Both of them carry the live flag, and only them.
		assert.equal(
			(live.match(/thread-flag is-live/g) || []).length,
			2,
			"both live conversations must carry the is-live flag",
		);
		assert.equal(
			(html.match(/thread-flag is-live/g) || []).length,
			2,
			"no entry outside the live group may carry the is-live flag",
		);
		// And the one that ended is still listed, under the group of what is
		// over: closed is not hidden, it is elsewhere.
		assert.ok(
			groupBody(html, "Archiviate").includes("conv-chiusa"),
			"the archived conversation must still be listed under Archiviate",
		);
	});

	it("marca come corrente solo la conversazione mostrata, anche fra due vive (AC-3)", () => {
		const html = renderConversationIndex(twoLiveAndOneClosed, {
			now: NOW,
			currentId: "conv-viva-libera",
		});

		const current = threadWithTitle(html, "Che ne pensi della rail");
		assert.match(current, /is-current/);
		assert.match(current, /aria-current="true"/);

		const other = threadWithTitle(html, "Implementiamo US-059");
		assert.ok(
			!other.includes("is-current"),
			"a live conversation that is not the one on screen must not be marked current",
		);
		assert.ok(
			!other.includes("aria-current"),
			"only the conversation on screen carries aria-current",
		);
		assert.equal(
			(html.match(/aria-current="true"/g) || []).length,
			1,
			"exactly one entry is the current one",
		);
	});
});

describe("renderLimitNotice", () => {
	// The very shape of sentence the server sends: it declares the limit and
	// names the conversations holding the places.
	const serverRefusal =
		"this workspace already holds 3 live conversations: conv-a (Implementiamo US-059), conv-b (Che ne pensi della rail), conv-c (Rivediamo il piano). Close one of them before opening another.";

	it("rende il rifiuto del server alla lettera (AC-5)", () => {
		const html = renderLimitNotice(serverRefusal);
		assert.match(html, /class="rail-notice"/);
		assert.match(html, /role="status"/);
		// Verbatim: every word of it, including the number and all three ids.
		assert.ok(
			visibleText(html).includes(serverRefusal),
			`the notice must carry the server sentence unchanged, got ${html}`,
		);
	});

	it("porta il comando per chiuderlo", () => {
		const html = renderLimitNotice(serverRefusal);
		assert.match(html, /data-rail-notice-dismiss/);
		assert.match(html, /aria-label="Chiudi questo avviso"/);
	});

	it("tace quando non c'è nulla da dire", () => {
		assert.equal(renderLimitNotice(""), "");
		assert.equal(renderLimitNotice("   "), "");
		assert.equal(renderLimitNotice(null), "");
		assert.equal(renderLimitNotice(undefined), "");
	});

	it("non inventa il numero: il limite non è scritto nel modulo (AC-5)", () => {
		// The module composes nothing about the limit, so it can hold no digit
		// of its own: a number written here would be a second truth about how
		// many conversations may live at once, and the two would drift.
		const withoutDigits = renderLimitNotice(
			"this workspace is already holding every conversation it can hold",
		);
		assert.ok(
			!/\d/.test(visibleText(withoutDigits)),
			`a refusal with no number must produce a notice with no number, got ${withoutDigits}`,
		);
	});

	it("sfugge il motivo invece di interpretarlo", () => {
		const html = renderLimitNotice('<script>alert("x")</script> troppe vive');
		assert.ok(
			!html.includes("<script>"),
			`the notice must escape markup, got ${html}`,
		);
		assert.match(html, /&lt;script&gt;/);
		assert.match(html, /troppe vive/);
	});
});

describe("il comando di cancellazione di un thread", () => {
	const rail = () =>
		renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-viva",
						title: "Quella in corso",
						last_message_at: ago(HOUR),
						work: "TURN_ACTIVE",
						live: true,
					},
					{
						id: "conv-ferma",
						title: "Quella ferma",
						last_message_at: ago(HOUR),
						work: "IDLE",
					},
					{
						id: "conv-chiusa",
						title: "Quella conclusa",
						last_message_at: ago(3 * DAY),
						archived: true,
					},
				],
			},
			{ now: NOW },
		);

	it("sta su ogni conversazione archiviata e dice quale cancella", () => {
		const html = rail();
		assert.match(html, /data-conversation-delete="conv-chiusa"/);
		assert.match(html, /Elimina la conversazione Quella conclusa/);
	});

	it("non compare su una conversazione ancora aperta, al lavoro o ferma che sia", () => {
		const html = rail();
		assert.ok(
			!html.includes('data-conversation-delete="conv-viva"'),
			`a working conversation must carry no trash, got ${html}`,
		);
		assert.ok(
			!html.includes('data-conversation-delete="conv-ferma"'),
			`an open conversation must carry no trash, got ${html}`,
		);
		assert.equal(html.match(/data-conversation-delete=/g).length, 1);
	});

	it("sta accanto al thread e non dentro: niente bottoni annidati", () => {
		const html = rail();
		// Il thread apre la conversazione, il comando la cancella: se il secondo
		// fosse dentro il primo il markup sarebbe invalido e una pressione
		// finirebbe per aprire quello che si sta cancellando.
		const insideThread = /data-conversation-id="[^"]*"[^>]*>(?:(?!<\/button>)[\s\S])*data-conversation-delete/;
		assert.ok(
			!insideThread.test(html),
			`the trash must be a sibling of the thread, got ${html}`,
		);
	});
});

describe("l'annullamento dell'ultima eliminazione", () => {
	const withUndo = (conversations) =>
		renderConversationIndex(
			{ conversations },
			{ now: NOW, undo: { id: "conv-tolta", title: "Quella tolta" } },
		);

	it("sta in fondo alla lista e dice quale conversazione rimette a posto", () => {
		const html = withUndo([
			{ id: "conv-resta", title: "Quella che resta", last_message_at: ago(HOUR) },
		]);
		assert.match(html, /data-conversation-undo="conv-tolta"/);
		assert.match(visibleText(html), /Annulla l'eliminazione/);
		// Nel nome accessibile l'apostrofo è sfuggito, come ogni altro testo che
		// finisce in un attributo.
		assert.match(
			html,
			/aria-label="Annulla l&#39;eliminazione di Quella tolta"/,
		);
		// In fondo vuol dire dopo l'ultimo thread, non prima.
		assert.ok(
			html.indexOf("data-conversation-undo") >
				html.lastIndexOf("data-conversation-id"),
			`the undo must come after the threads, got ${html}`,
		);
		// E fuori dalla lista che scorre: sta nella barra ferma in fondo alla
		// colonna, altrimenti una lista lunga lo porterebbe via con sé.
		assert.ok(
			html.indexOf("rail-bottom") > html.indexOf('class="rail-list"') &&
				html.indexOf("data-conversation-undo") > html.indexOf("rail-bottom"),
			`the undo must live in the docked bar after the list, got ${html}`,
		);
	});

	it("c'è anche quando la lista è rimasta vuota", () => {
		// Cancellare l'unica conversazione del workspace è esattamente il caso
		// in cui serve: senza il bottone qui, quella cancellazione non si
		// potrebbe più annullare.
		const html = withUndo([]);
		assert.match(html, /data-conversation-undo="conv-tolta"/);
		assert.match(visibleText(html), /non c'è ancora nessuna conversazione/);
	});

	it("non compare quando non c'è niente da annullare", () => {
		const html = renderConversationIndex(
			{ conversations: [{ id: "conv-1", title: "Una", last_message_at: ago(HOUR) }] },
			{ now: NOW },
		);
		assert.ok(
			!html.includes("data-conversation-undo"),
			`no deletion, no undo, got ${html}`,
		);
	});
});

describe("relativeTime", () => {
	it("dice da quanto tempo, sui confini, con un orologio fisso", () => {
		assert.equal(relativeTime(ago(10 * 1000), NOW), "adesso");
		assert.equal(relativeTime(ago(5 * 60 * 1000), NOW), "5 min fa");
		assert.equal(relativeTime(ago(HOUR), NOW), "1 ora fa");
		assert.equal(relativeTime(ago(3 * HOUR), NOW), "3 ore fa");
		assert.equal(relativeTime(ago(30 * HOUR), NOW), "ieri");
		assert.equal(relativeTime(ago(4 * DAY), NOW), "4 giorni fa");
		assert.equal(relativeTime(ago(40 * DAY), NOW), "13/07/2026");
	});

	it("tace su un momento che non conosce", () => {
		assert.equal(relativeTime(null, NOW), "");
		assert.equal(relativeTime("", NOW), "");
		assert.equal(relativeTime("non-una-data", NOW), "");
	});
});

describe("il modulo dell'indice", () => {
	it("è puro: nessun DOM, nessuna rete, nessun timer", () => {
		const source = readFileSync(helperPath, "utf8");
		for (const forbidden of ["document", "fetch(", "setInterval"]) {
			assert.ok(
				!source.includes(forbidden),
				`conversation-index.js must not reference ${forbidden}`,
			);
		}
	});
});
