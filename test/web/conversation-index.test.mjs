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
//   - AC-5 a conversation tied to no spec is listed apart from the tied ones
//     and carries no spec code
//   - AC-7 a workspace with no past conversation says so and offers to open
//     the first one
//   - AC-4 a resumed conversation declares that it is a new one carrying the
//     earlier one as context, and names it
//   - ogni nota del pannello — la dichiarazione di ripresa e il rifiuto della
//     colonna — porta il comando per chiuderla
//   - a group with no members leaves no orphan heading
//   - the live conversation is listed as such even when it has a spec code
//   - US-059 AC-3 several live conversations stand together under "In corso"
//     while a closed one stays out of that group
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
	const buttons = html.split("<button").slice(1);
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

		const visible = visibleText(html);
		assert.match(visible, /Spec/);
		assert.match(visible, /Progetto/);

		const free = threadWithTitle(html, "Conversazione di progetto");
		assert.match(free, /is-free/);
		assert.doesNotMatch(free, /US-\d/);

		const tied = threadWithTitle(html, "Conversazione della spec");
		assert.match(tied, /US-058/);
		assert.doesNotMatch(tied, /is-free/);
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

		assert.match(html, /Progetto/);
		assert.doesNotMatch(html, /Spec/);
		assert.doesNotMatch(html, /In corso/);
	});

	it("elenca sotto «In corso» la conversazione viva, anche se ha un codice spec", () => {
		const html = renderConversationIndex(
			{
				conversations: [
					{
						id: "conv-viva",
						title: "Quella aperta adesso",
						last_message_at: ago(5 * 60 * 1000),
						spec_code: "US-058",
						live: true,
					},
					{
						id: "conv-vecchia",
						title: "Quella di ieri",
						last_message_at: ago(30 * HOUR),
						spec_code: "US-057",
					},
				],
			},
			{ now: NOW },
		);

		const visible = visibleText(html);
		assert.match(visible, /In corso/);

		const live = threadWithTitle(html, "Quella aperta adesso");
		assert.match(visibleText(live), /in corso/);

		// The live one is drawn first, under its own heading, and the spec
		// group opens only after it.
		assert.ok(
			html.indexOf("In corso") < html.indexOf("Quella aperta adesso"),
		);
		assert.ok(
			html.indexOf("Quella aperta adesso") < html.indexOf("Quella di ieri"),
		);
		assert.doesNotMatch(
			visibleText(threadWithTitle(html, "Quella di ieri")),
			/in corso/,
		);
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

describe("renderConversationIndex con più conversazioni vive", () => {
	// The payload of a workspace holding two live conversations — one tied to a
	// spec, one free — beside a third that has ended. It is the state US-059
	// introduces, and the one the singular rail could not describe.
	const twoLiveAndOneClosed = {
		conversations: [
			{
				id: "conv-viva-spec",
				title: "Implementiamo US-059",
				last_message_at: ago(2 * 60 * 1000),
				spec_code: "US-059",
				live: true,
			},
			{
				id: "conv-viva-libera",
				title: "Che ne pensi della rail",
				last_message_at: ago(10 * 60 * 1000),
				spec_code: "",
				live: true,
			},
			{
				id: "conv-chiusa",
				title: "Conversazione finita ieri",
				last_message_at: ago(30 * HOUR),
				spec_code: "US-058",
				state: "CLOSED",
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

	it("tiene due conversazioni vive insieme sotto «In corso» e la chiusa fuori (AC-3)", () => {
		const html = renderConversationIndex(twoLiveAndOneClosed, { now: NOW });

		const live = groupBody(html, "In corso");
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
			"a conversation that has ended must not stand under In corso",
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
		// And the one that ended is still listed, under the group its spec code
		// puts it in: closed is not hidden, it is elsewhere.
		assert.ok(
			groupBody(html, "Spec").includes("conv-chiusa"),
			"the closed conversation must still be listed under its own group",
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
