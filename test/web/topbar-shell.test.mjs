// test/web/topbar-shell.test.mjs
// Structural oracles for the topbar and the board counters (US-061).
// Run: node --test test/web/topbar-shell.test.mjs
//
// Nothing here can be expressed on a pure module, because nothing here is
// about what a function returns: it is about *where* a control is written.
// What US-061 asks for is a bar with four things one click away and one
// collector menu holding everything else, with no entry lost on the way — and
// three counters read in the board instead of in the bar.
//
//   - AC-5 the topbar exposes the workspace identity, one primary action, the
//     count of runs waiting and the theme; every other entry is inside
//     #topbar-more-menu, each exactly once, with its id and its markup
//     unchanged;
//   - AC-5 the collector is not workspace-scoped, because it holds
//     "New workspace", which must stay reachable with no workspace open;
//   - AC-6 the counters are gone from index.html and are emitted by
//     renderBoard, in both branches — an empty backlog included — and
//     updateStats resolves them by id on every call.
//
// These are facts a future refactor would break in silence: no unit test would
// go red, and the screen would only misbehave for a person. Hence the reading
// of the sources themselves. Nothing here asserts formatting, indentation or
// line numbers — only facts whose loss would break an acceptance criterion,
// and every failure message names the broken fact rather than a string diff.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const assetsDir = resolve(
	__dirname,
	"..",
	"..",
	"cli",
	"internal",
	"web",
	"assets",
);

const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");
const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");

// Extracts the balanced `{...}` block that opens after `marker`. Same helper
// as the sibling files: an assertion about "what this function does" cannot be
// satisfied by a coincidence somewhere else in a 6000-line file.
function blockAfter(source, marker) {
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `la sorgente non contiene più \`${marker}\``);
	const open = source.indexOf("{", at);
	assert.notEqual(open, -1, `nessun blocco dopo \`${marker}\``);
	let depth = 0;
	for (let i = open; i < source.length; i++) {
		if (source[i] === "{") depth++;
		else if (source[i] === "}") {
			depth--;
			if (depth === 0) return source.slice(open + 1, i);
		}
	}
	assert.fail(`blocco non bilanciato dopo \`${marker}\``);
}

// Every CSS rule as {selector, body}. Comments are dropped first so a failure
// names the offending selector and not the paragraph written above it.
function cssRules(text) {
	const stripped = text.replace(/\/\*[\s\S]*?\*\//g, "");
	const rules = [];
	const re = /([^{}]+)\{([^{}]*)\}/g;
	let m;
	while ((m = re.exec(stripped)) !== null) {
		rules.push({ selector: m[1].trim().replace(/\s+/g, " "), body: m[2] });
	}
	return rules;
}

// The markup between two known markers. Enough here, and deliberately simpler
// than an HTML parser: the repository asserts on sources with substrings, and
// both fragments this file reads are delimited by markers that appear once.
function fragment(source, from, to) {
	const start = source.indexOf(from);
	assert.notEqual(start, -1, `index.html non contiene più \`${from}\``);
	const end = source.indexOf(to, start);
	assert.notEqual(end, -1, `\`${to}\` non segue più \`${from}\``);
	return source.slice(start, end);
}

// The subtree of a <div> that opens at `marker`, closing tags counted. Needed
// because "one click away" is a fact about *direct* children: a fragment that
// swallows the collector menu would call every entry inside it a bar entry,
// which is the very regression AC-5 forbids.
function divSubtree(source, marker) {
	const start = source.indexOf(marker);
	assert.notEqual(start, -1, `index.html non contiene più \`${marker}\``);
	const re = /<div\b|<\/div>/g;
	re.lastIndex = start;
	let depth = 0;
	let m;
	while ((m = re.exec(source)) !== null) {
		depth += m[0] === "</div>" ? -1 : 1;
		if (depth === 0) return source.slice(start, m.index + m[0].length);
	}
	assert.fail(`il <div> aperto da \`${marker}\` non si chiude`);
}

// Every id declared in a markup fragment, in order.
function idsIn(markup) {
	return [...markup.matchAll(/id="([^"]+)"/g)].map((m) => m[1]);
}

// The seven entries the bar used to keep one click away, and that the menu
// must still hold. Losing one is losing a command, which no amount of tidying
// makes acceptable.
const COLLECTED = [
	"metrics-btn",
	"prd-btn",
	"mockups-dropdown",
	"new-workspace-btn",
	"workspaces-btn",
	"config-btn",
	"refresh-btn",
];

describe("AC-5 — la barra superiore espone quattro cose a un click", () => {
	it("identità, azione primaria, run in attesa e tema sono nella barra, e nient'altro", () => {
		const brand = fragment(html, '<div class="brand">', "</div>\n\n");
		assert.ok(
			brand.includes('id="workspace-indicator"'),
			"l'identità del workspace non è più nel blocco .brand della barra superiore",
		);

		const actions = fragment(html, '<div class="topbar-actions">', "</header>");
		assert.ok(
			actions.includes('id="topbar-more"'),
			"la barra superiore non espone più il menu di raccolta",
		);
		assert.ok(
			!actions.includes('id="workspace-indicator"'),
			"l'identità del workspace è finita fra le azioni: il suo posto è il blocco .brand",
		);

		// Ciò che la barra espone *a un click* è ciò che resta togliendo il
		// sottoalbero del menu: un id che vive dentro il menu non è a un click,
		// ed è esattamente la differenza che AC-5 chiede.
		const collector = divSubtree(html, '<div class="dropdown topbar-more"');
		const oneClick = idsIn(actions.replace(collector, ""));
		assert.deepEqual(
			oneClick.sort(),
			["new-spec-btn", "runs-attention", "theme-toggle"],
			`a un click nella barra superiore ci sono ${JSON.stringify(oneClick)} invece dell'unica azione primaria, del contatore delle run in attesa e del tema: o una voce è tornata affiancata, o una delle tre è finita dentro il menu`,
		);
	});

	it("nessuna delle altre voci è andata persa: sono tutte nel menu di raccolta", () => {
		const menu = divSubtree(html, '<div class="dropdown-menu topbar-more-menu');
		for (const id of COLLECTED) {
			assert.ok(
				menu.includes(`id="${id}"`),
				`#${id} non è più raggiungibile: non sta più nella barra e non è nel menu di raccolta`,
			);
		}
	});

	it("nessuna voce è duplicata: una voce doppia sarebbe premuta da due gestori", () => {
		for (const id of COLLECTED) {
			const occurrences = html.split(`id="${id}"`).length - 1;
			assert.equal(
				occurrences,
				1,
				`#${id} compare ${occurrences} volte in index.html: due elementi con lo stesso id sono due controlli che lo stesso listener crede uno solo`,
			);
		}
	});

	it("il menu di raccolta resta premibile a workspace chiuso", () => {
		const opener = fragment(html, '<div class="dropdown topbar-more"', ">");
		assert.ok(
			!opener.includes("data-workspace-scoped"),
			'#topbar-more è diventato data-workspace-scoped: "New workspace" vive dentro il menu e sarebbe irraggiungibile proprio quando serve, cioè senza workspace aperto',
		);
		const newWorkspace = fragment(html, '<button id="new-workspace-btn"', ">");
		assert.ok(
			!newWorkspace.includes("data-workspace-scoped"),
			"#new-workspace-btn è diventato data-workspace-scoped: sparirebbe insieme al workspace che non c'è",
		);
	});

	it("aprire il sottomenu dei mockup non chiude il menu che lo contiene", () => {
		const body = blockAfter(js, "function toggleTopbarMoreMenu(");
		assert.ok(
			body.includes("stopPropagation"),
			"il toggle del menu di raccolta non ferma la propagazione: il proprio click lo richiuderebbe subito",
		);
		assert.ok(
			js.includes("topbarMoreDropdown.contains(e.target)"),
			"la chiusura del menu di raccolta non è più ristretta al contenitore #topbar-more: il sottomenu dei mockup, che vive dentro di esso, lo chiuderebbe aprendosi",
		);
	});
});

describe("AC-6 — i contatori del backlog stanno nella board", () => {
	it("non sono più nella barra superiore", () => {
		assert.ok(
			!html.includes('id="topbar-stats"'),
			"i tre contatori sono tornati nella barra superiore",
		);
		assert.ok(
			!html.includes('class="topbar-stats"'),
			"la fascia dei contatori è tornata nella barra superiore",
		);
	});

	it("renderBoard li emette in entrambi i rami, backlog vuoto compreso", () => {
		const body = blockAfter(js, "function renderBoard(");
		for (const token of ["board-stats", "stat-total", "stat-progress", "stat-done"]) {
			assert.ok(
				body.includes(token) || blockAfter(js, "function boardStatsHeader(").includes(token),
				`la board non emette più ${token}: quel contatore non sarebbe leggibile da nessuna parte`,
			);
		}
		const header = body.indexOf("boardStatsHeader(");
		const empty = body.indexOf("empty-board");
		assert.notEqual(
			header,
			-1,
			"renderBoard non emette più l'intestazione dei contatori",
		);
		assert.notEqual(
			empty,
			-1,
			"renderBoard non ha più il ramo del backlog vuoto: il confronto di questo test non ha più significato, va riletto il disegno della board",
		);
		assert.ok(
			header < empty,
			"il messaggio di backlog assente precede l'intestazione: il ramo vuoto sta sostituendo i contatori invece di aggiungersi dopo di essi, e un backlog vuoto resterebbe senza numeri",
		);
		assert.ok(
			!/boardEl\.innerHTML\s*=/.test(body.slice(header)),
			"dopo l'intestazione renderBoard riassegna boardEl.innerHTML: l'intestazione appena emessa verrebbe cancellata, e il ramo del backlog vuoto resterebbe senza numeri",
		);
	});

	it("updateStats risolve i contatori per id a ogni chiamata", () => {
		const body = blockAfter(js, "function updateStats(");
		assert.ok(
			body.includes('getElementById("stat-total")'),
			"updateStats non risolve più i contatori per id: la board li ridisegna a ogni passata e i riferimenti tenuti in memoria punterebbero a nodi sostituiti",
		);
		assert.ok(
			!js.includes("const statTotal = document.getElementById"),
			"i contatori sono di nuovo risolti in costanti al boot: prima del primo disegno della board quei nodi non esistono",
		);
	});

	it("nessuna regola di app.css nasconde l'intestazione della board", () => {
		const hiding = cssRules(css).filter(
			(rule) =>
				/(^|[\s,>])\.board-stats\s*(,|$)/.test(rule.selector) &&
				/display\s*:\s*none/.test(rule.body),
		);
		assert.deepEqual(
			hiding.map((r) => r.selector),
			[],
			"una regola di app.css applica display:none a .board-stats: i contatori sparirebbero dalla board, che è l'unico posto in cui ora si leggono",
		);
	});
});
