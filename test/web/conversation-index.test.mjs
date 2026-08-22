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
//   - a group with no members leaves no orphan heading
//   - the live conversation is listed as such even when it has a spec code
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

const { renderConversationIndex, relativeTime, renderResumeBanner } =
	loadConversationIndex();

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
