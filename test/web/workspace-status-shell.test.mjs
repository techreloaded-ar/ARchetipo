// test/web/workspace-status-shell.test.mjs
// Structural oracles for the workspace status strip (US-056).
// Run: node --test test/web/workspace-status-shell.test.mjs
//
// The decisions the strip takes live in a pure module and are tested there
// (workspace-status.test.mjs). What is checked here is everything the pure
// module cannot possibly express, because it is a property of *where* things
// are written rather than of what a function returns:
//
//   - the strip is a sibling that sits before the two-column shell, and the
//     only rule that hides it is the one for "no workspace open" — so opening
//     a spec never covers it (AC-1);
//   - there is exactly one way to start an execution in this application, the
//     mounted panel's `startURL`, and the strip reaches it through the very
//     function the board presses (AC-2);
//   - the strip is redrawn before the guards that freeze the board while a
//     window or a form is open (AC-3);
//   - closing the workspace forgets the step it recommended (AC-5).
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

// Extracts the balanced `{...}` block that opens after `marker`. Used to read a
// single function body, so an assertion about "what this function does" cannot
// be satisfied by a coincidence somewhere else in a 5000-line file.
function blockAfter(source, marker) {
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `app.js non contiene più \`${marker}\``);
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
// names the offending selector and not the paragraph written above it. Enough
// for this file: the sources use no nested at-rule bodies around the
// declarations these tests read.
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

describe("AC-1 — la striscia sta fuori dal guscio", () => {
	it("è un fratello che precede #workspace-shell, non un suo discendente", () => {
		const strip = html.indexOf('id="workspace-status"');
		const shell = html.indexOf('id="workspace-shell"');
		assert.notEqual(strip, -1, "index.html non contiene più #workspace-status");
		assert.notEqual(shell, -1, "index.html non contiene più #workspace-shell");
		assert.ok(
			strip < shell,
			"la striscia deve essere dichiarata prima del guscio: dentro o dopo il guscio seguirebbe la colonna primaria e il pannello spec la coprirebbe",
		);
		const between = html.slice(strip, shell);
		assert.ok(
			!between.includes("workspace-primary"),
			"fra la striscia e il guscio compare workspace-primary: la striscia è finita dentro la colonna che ospita il dettaglio spec, e aprire una spec la coprirebbe",
		);
	});

	it("è nascosta da una regola sola, quella del workspace non aperto", () => {
		const hiding = cssRules(css).filter(
			(rule) =>
				rule.selector.includes("#workspace-status") &&
				/display\s*:\s*none/.test(rule.body),
		);
		assert.equal(
			hiding.length,
			1,
			`#workspace-status deve essere nascosto da una regola sola; regole trovate: ${hiding
				.map((r) => JSON.stringify(r.selector))
				.join(", ")}`,
		);
		assert.ok(
			hiding[0].selector.includes("body.no-workspace"),
			`l'unica regola che nasconde la striscia deve essere quella del workspace non aperto; selettore intruso: ${JSON.stringify(
				hiding[0].selector,
			)} — con questa regola il passo suggerito può sparire mentre il revisore lavora`,
		);
	});
});

describe("AC-2 — il cammino di avvio è uno solo", () => {
	it("ogni URL di avvio esecuzione è un valore startURL: del pannello montato", () => {
		const startish = /\/api\/workspace\/execution|api\/spec\/[^\n]*\/execution/;
		const offenders = js
			.split("\n")
			.map((line, i) => ({ line, n: i + 1 }))
			.filter(({ line }) => startish.test(line))
			.filter(({ line }) => !line.includes("startURL:"));
		assert.deepEqual(
			offenders.map(({ n, line }) => `${n}: ${line.trim()}`),
			[],
			"è stato introdotto un secondo cammino di avvio: una rotta di esecuzione viene chiamata fuori da un `startURL:` di mountExecutionPanels. L'avvio deve passare da startPanelAction, che è ciò che rende l'avvio dalla striscia identico a quello dalla board",
		);
	});

	it("il gestore della striscia delega, non riscrive l'avvio", () => {
		const listener = blockAfter(js, 'workspaceStatusEl.addEventListener("click"');
		assert.ok(
			listener.includes("ws-status-next"),
			"il listener della striscia non riconosce più il controllo .ws-status-next: il passo suggerito non sarebbe più avviabile da dove è mostrato",
		);
		assert.ok(
			listener.includes("startNextStep"),
			"il gestore della striscia non delega più a startNextStep",
		);
		const startNextStep = blockAfter(js, "async function startNextStep(");
		assert.ok(
			startNextStep.includes("startPanelAction"),
			"startNextStep non chiama più startPanelAction: l'avvio dalla striscia smetterebbe di essere lo stesso avvio della board",
		);
	});
});

describe("AC-3 — la striscia si ridisegna prima delle guardie", () => {
	it("scheduleBoardReload aggiorna il passo suggerito anche a finestra o form aperti", () => {
		const body = blockAfter(js, "function scheduleBoardReload(");
		const status = body.indexOf("loadWorkspaceStatus()");
		const board = body.indexOf("loadBoard()");
		const firstGuard = body.indexOf("return;");
		const lastGuard = body.lastIndexOf("return;");
		assert.notEqual(
			status,
			-1,
			"scheduleBoardReload non ricarica più lo stato del workspace: il passo suggerito non si aggiornerebbe senza ricaricare la pagina",
		);
		assert.notEqual(board, -1, "scheduleBoardReload non ricarica più la board");
		assert.notEqual(
			firstGuard,
			-1,
			"scheduleBoardReload non ha più guardie: il confronto di questo test non ha più significato, va riletto il gestore",
		);
		assert.ok(
			status < firstGuard,
			"loadWorkspaceStatus() è finita dietro le guardie di scheduleBoardReload: il passo suggerito resterebbe congelato per tutto il tempo in cui una finestra o un form restano aperti, che è esattamente ciò che avviare un passo di workspace produce",
		);
		assert.ok(
			board > lastGuard,
			"loadBoard() è finita davanti alle guardie: ricaricare la board scarterebbe le modifiche in corso",
		);
	});
});

describe("AC-5 — senza workspace aperto nessun passo è suggerito", () => {
	it("enterNoWorkspaceMode azzera lo stato della striscia", () => {
		const body = blockAfter(js, "function enterNoWorkspaceMode(");
		assert.ok(
			body.includes("resetWorkspaceStatusState"),
			"enterNoWorkspaceMode non azzera più lo stato della striscia: l'ultimo passo suggerito sopravvivrebbe al workspace che lo ha prodotto e il redraw successivo potrebbe rimetterlo in scena",
		);
	});
});
