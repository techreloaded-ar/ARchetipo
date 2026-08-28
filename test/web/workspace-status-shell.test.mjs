// test/web/workspace-status-shell.test.mjs
// Structural oracles for the recommended step (US-056, moved by US-061).
// Run: node --test test/web/workspace-status-shell.test.mjs
//
// The decisions the step takes live in a pure module and are tested there
// (workspace-status.test.mjs); how the block is drawn is tested on the pure
// renderer (conversation.test.mjs). What is checked here is everything neither
// can possibly express, because it is a property of *where* things are written
// rather than of what a function returns:
//
//   - the status strip that used to sit between the topbar and the shell does
//     not exist any more — not the element, not its rules, not its renderer
//     (US-061 AC-1);
//   - there is exactly one way to start an execution in this application, the
//     mounted panel's `startURL`, and the block in the thread reaches it
//     through the very function the board presses (AC-2);
//   - the panel is fed the recommended step from the status payload and from
//     no other source, and is redrawn whenever that payload changes (AC-2);
//   - closing the workspace forgets the step it recommended and takes it off
//     the screen (US-056 AC-5);
//   - the step is refreshed before the guards that freeze the board while a
//     window or a form is open (US-056 AC-3).
//
// The two oracles of US-056 that read the strip's position and its single
// hiding rule are superseded by US-061 AC-1, which removes the strip: they are
// rewritten below as the assertion that nothing of it is left.
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

describe("AC-1 — la fascia di stato non esiste più", () => {
	it("index.html non disegna più alcuna fascia sopra la conversazione", () => {
		assert.ok(
			!html.includes('id="workspace-status"'),
			"la fascia di stato è tornata fra la barra superiore e la conversazione",
		);
	});

	it("app.css non stila più nulla della fascia", () => {
		const leftovers = cssRules(css)
			.map((rule) => rule.selector)
			.filter((selector) => /\.workspace-status\b|\.ws-status/.test(selector));
		assert.deepEqual(
			leftovers,
			[],
			`sono rimaste regole della fascia rimossa: ${leftovers.join(", ")}`,
		);
		assert.ok(
			!css.includes("#workspace-status"),
			"app.css nomina ancora #workspace-status: un elemento che non esiste più",
		);
	});

	it("il modulo puro non disegna più la fascia", () => {
		const module = readFileSync(
			resolve(assetsDir, "workspace-status.js"),
			"utf8",
		);
		assert.ok(
			!module.includes("renderWorkspaceStatus"),
			"workspace-status.js disegna di nuovo la fascia: il passo raccomandato si disegna nel thread, in conversation.js",
		);
		assert.ok(
			!js.includes("workspaceStatusEl"),
			"app.js scrive di nuovo su un elemento della fascia rimossa",
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
			"è stato introdotto un secondo cammino di avvio: una rotta di esecuzione viene chiamata fuori da un `startURL:` di mountExecutionPanels. L'avvio deve passare da startPanelAction, che è ciò che rende l'avvio dal thread identico a quello dalla board",
		);
	});

	it("il gestore del thread delega, non riscrive l'avvio", () => {
		const listener = blockAfter(js, "function bindConversationPanel(");
		assert.ok(
			listener.includes("conv-nextstep-run"),
			"il pannello conversazione non riconosce più il controllo .conv-nextstep-run: il passo raccomandato non sarebbe più avviabile da dove è mostrato",
		);
		assert.ok(
			listener.includes("nextStepDispatch"),
			"il gestore del thread non chiede più al modulo puro se il passo è avviabile: il rifiuto di un passo bloccato tornerebbe a essere il solo attributo disabled del markup",
		);
		assert.ok(
			listener.includes("startNextStep"),
			"il gestore del thread non delega più a startNextStep",
		);
		const startNextStep = blockAfter(js, "async function startNextStep(");
		assert.ok(
			startNextStep.includes("startPanelAction"),
			"startNextStep non chiama più startPanelAction: l'avvio dal thread smetterebbe di essere lo stesso avvio della board",
		);
	});

	it("l'avvio non apre più un secondo thread: la run è il thread", () => {
		// Il buco che questo chiudeva era vero e resta chiuso, ma altrove. Un
		// passo avviato dai dettagli di una spec faceva lavorare un agente senza
		// che nell'elenco delle conversazioni comparisse niente. La risposta era
		// aprire un thread prima di avviare — e quel thread era un secondo
		// processo d'agente, inerte, aperto solo per raccontare il lavoro di un
		// altro. Ora la run *è* una conversazione: il server tiene la sua stessa
		// sessione sotto l'id dell'esecuzione, e il client la raggiunge con
		// quell'id invece di aprirne una accanto.
		const start = blockAfter(js, "async function startPanelAction(");
		assert.ok(
			!start.includes("threadForStart"),
			"startPanelAction apre ancora un thread prima di avviare: sarebbero di nuovo due processi d'agente per una pressione sola",
		);
		assert.ok(
			start.includes("revealThread(record"),
			"l'avvio non porta più sullo schermo il thread della run: chi preme resterebbe senza il posto in cui l'agente sta lavorando",
		);
		assert.ok(
			start.includes("body.conversation_id = from"),
			"l'avvio non nomina più la conversazione da cui viene la pressione: una run chiesta dentro un thread smetterebbe di essere ricordata lì",
		);
		assert.ok(
			start.includes("liveConversationEntries"),
			"l'avvio non guarda più se la conversazione che gli è stata nominata è viva: legherebbe la run a un thread già chiuso",
		);
		assert.ok(
			!js.includes("async function threadForStart("),
			"threadForStart esiste ancora: nessuno la chiama più",
		);
	});

	// Il numero di punti che avviano un'azione è un fatto, non uno stile: due
	// cammini di avvio sono due modi di partire che possono divergere. Sono la
	// chip del pannello, il passo raccomandato e la dichiarazione della
	// funzione stessa.
	it("nessun nuovo punto di avvio è comparso in app.js", () => {
		const occurrences = js.split("startPanelAction(").length - 1;
		assert.equal(
			occurrences,
			3,
			`i punti che nominano startPanelAction( sono ${occurrences} invece di 3: è comparso (o sparito) un cammino di avvio, e l'identità fra l'avvio dal thread e quello dalla board va riverificata`,
		);
	});

	it("il pannello conversazione riceve il passo dallo stato del workspace", () => {
		const body = blockAfter(js, "function renderConversationPanel(");
		assert.ok(
			body.includes("nextStep:"),
			"il pannello conversazione non passa più il passo raccomandato al renderer: in coda al thread non comparirebbe alcun blocco",
		);
		assert.ok(
			body.includes("nextStepStatusView("),
			"il passo raccomandato non passa più dalla sorgente unica nextStepStatusView: render e avvio potrebbero divergere",
		);
		// La sorgente unica legge sempre e solo i payload di
		// /api/workspace/status: quello scopato sulla spec della conversazione
		// quando c'è, quello del workspace altrimenti.
		const source = blockAfter(js, "function nextStepStatusView(");
		assert.ok(
			source.includes("conversationStatusSnapshot") &&
				source.includes("workspaceStatusSnapshot"),
			"il passo raccomandato non arriva più dal payload di /api/workspace/status: il thread lo starebbe inventando da un'altra fonte",
		);
		const load = blockAfter(js, "async function loadWorkspaceStatus(");
		assert.ok(
			load.includes("renderConversationPanel("),
			"leggere lo stato del workspace non ridisegna più il thread: il blocco resterebbe fermo sul passo di un momento fa",
		);
	});
});

describe("US-056 AC-3 — il passo si aggiorna prima delle guardie", () => {
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

describe("US-056 AC-5 — senza workspace aperto nessun passo è suggerito", () => {
	it("enterNoWorkspaceMode azzera lo stato del passo raccomandato", () => {
		const body = blockAfter(js, "function enterNoWorkspaceMode(");
		assert.ok(
			body.includes("resetWorkspaceStatusState"),
			"enterNoWorkspaceMode non azzera più lo stato del passo raccomandato: l'ultimo passo suggerito sopravvivrebbe al workspace che lo ha prodotto e il redraw successivo potrebbe rimetterlo in scena",
		);
	});

	it("dimenticare il passo lo toglie anche dallo schermo", () => {
		const body = blockAfter(js, "function resetWorkspaceStatusState(");
		assert.ok(
			/workspaceStatusSnapshot\s*=\s*null/.test(body),
			"resetWorkspaceStatusState non dimentica più il passo del workspace che si è appena chiuso",
		);
		assert.ok(
			body.includes("renderConversationPanel("),
			"dimenticare il passo non ridisegna più il thread: il blocco resterebbe sullo schermo con il passo di un workspace che non è più aperto",
		);
	});
});

// Il lavoro si disegna una volta sola.
//
// Dopo l'unificazione una run tenuta come conversazione *è* quella
// conversazione: una sessione, un processo d'agente. Un pannello disegnato
// accanto al thread sarebbe la stessa storia resa due volte, interrogata due
// volte, con due compositori che scrivono in un turno solo. Chi delle due sia
// una run lo sa il server — `thread_id` sulla proiezione — e la pagina non lo
// deduce: dedurlo vorrebbe dire indovinare una capability del provider.
//
// Non è un test unitario perché non è il risultato di una funzione: è una
// proprietà di *dove* la decisione è scritta, e un refactor che la perdesse non
// farebbe fallire nient'altro — tornerebbero solo a esserci due pannelli.
describe("il visore disegna la run una volta sola", () => {
	it("il pannello della run non si monta per una run che si legge in un thread", () => {
		const body = blockAfter(js, "async function resumeRun(");
		assert.ok(
			body.includes("view.thread_id"),
			"resumeRun non legge più thread_id: monterebbe il pannello anche per una run che il thread già mostra",
		);
		assert.ok(
			body.includes("executionThreadID"),
			"resumeRun non ricorda più dove la run si legge: renderExecution non avrebbe su cosa decidere",
		);
	});

	it("il pannello dell'esecuzione tace per una run che si legge in un thread", () => {
		const body = blockAfter(js, "function renderExecution(");
		assert.ok(
			body.includes("executionThreadID"),
			"renderExecution non guarda più se la run ha un thread: l'esito comparirebbe due volte, nel pannello e come esito del thread",
		);
	});

	it("prima si sa dove la run si legge, poi la si disegna", () => {
		// L'ordine non è stile: disegnare prima di sapere mostrerebbe per un
		// istante un pannello che sta per sparire.
		const follow = blockAfter(js, "async function followExecution(");
		const resumeAt = follow.indexOf("resumeRun(");
		const renderAt = follow.indexOf("renderExecution(");
		assert.ok(resumeAt >= 0, "followExecution non chiede più dove la run si legge");
		assert.ok(renderAt >= 0, "followExecution non disegna più l'esecuzione");
		assert.ok(
			resumeAt < renderAt,
			"followExecution disegna prima di sapere dove la run si legge",
		);
	});
});
