// test/web/p1-audit-fixes.test.mjs
// Oracoli strutturali per le cinque criticità P1 dell'audit UX del visore.
// Esecuzione: node --test test/web/p1-audit-fixes.test.mjs
//
// Sono fatti che nessun test unitario può presidiare, perché non sono il
// risultato di una funzione ma una proprietà di *dove* le cose sono scritte:
//
//   - lingua: la pagina si dichiara italiana e ogni modulo tiene le sue parole
//     in una tabella sola, invece di scriverle a mano nel punto d'uso;
//   - dati: le modali che ospitano una bozza chiedono conferma prima di
//     scartarla, e il trascinamento Review → Done chiede la stessa conferma
//     del bottone Approva;
//   - accessibilità: il toast è annunciabile, ha durata leggibile, dismiss e
//     coda; le regioni ricostruite non sono più live; le modali trattengono e
//     restituiscono il fuoco; le card della board si aprono da tastiera.
//
// Un refactor che li perdesse non farebbe fallire nessun altro test: la
// schermata smetterebbe di funzionare soltanto per una persona. Da qui la
// lettura delle sorgenti. Nessuna asserzione riguarda formattazione o numeri di
// riga, e ogni messaggio di errore nomina il fatto perduto.

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

/** Il corpo bilanciato che si apre dopo `marker`: una funzione sola, non il file. */
function sectionOf(source, marker) {
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

/** Il tag di apertura dell'elemento che porta `id="<id>"`. */
function openingTagOf(source, id) {
	const at = source.indexOf(`id="${id}"`);
	assert.notEqual(at, -1, `index.html non contiene più #${id}`);
	const start = source.lastIndexOf("<", at);
	const end = source.indexOf(">", at);
	assert.ok(start !== -1 && end !== -1, `tag malformato attorno a #${id}`);
	return source.slice(start, end + 1);
}

// ---------------------------------------------------------------------------
// P1 · Lingua
// ---------------------------------------------------------------------------

describe("P1 — l'interfaccia parla una lingua sola", () => {
	it("la pagina si dichiara in italiano", () => {
		assert.match(
			html,
			/<html lang="it"/,
			'index.html non dichiara più lang="it": uno screen reader leggerebbe l\'italiano con fonetica inglese',
		);
	});

	it("ogni modulo che scrive testo ha la sua tabella di parole", () => {
		for (const file of [
			"app.js",
			"conversation.js",
			"conversation-index.js",
			"workspace-home.js",
		]) {
			const source = readFileSync(resolve(assetsDir, file), "utf8");
			assert.match(
				source,
				/const TEXT = \{/,
				`${file} non ha più la tabella TEXT: le stringhe tornerebbero sparse nel punto d'uso`,
			);
		}
		const runs = readFileSync(resolve(assetsDir, "workspace-runs.js"), "utf8");
		assert.match(
			runs,
			/const DEFAULT_TEXT = \{/,
			"workspace-runs.js non ha più la sua tabella di parole",
		);
	});

	it("ogni chiave usata da app.js esiste nella tabella", () => {
		const table = sectionOf(js, "const TEXT = {");
		const declared = new Set(
			[...table.matchAll(/^\t\t([A-Za-z][A-Za-z0-9]*)\s*:/gm)].map((m) => m[1]),
		);
		const used = new Set(
			[...js.matchAll(/\bTEXT\.([A-Za-z][A-Za-z0-9]*)/g)].map((m) => m[1]),
		);
		const missing = [...used].filter((k) => !declared.has(k)).sort();
		assert.deepEqual(
			missing,
			[],
			`app.js legge parole che la tabella non dichiara: ${missing.join(", ")}`,
		);
	});
});

// ---------------------------------------------------------------------------
// P1 · Dati — la bozza non si scarta per sbaglio
// ---------------------------------------------------------------------------

describe("P1 — chiudere una modale non scarta la bozza in silenzio", () => {
	it("ogni modale con contenuto interroga la propria sentinella", () => {
		const guarded = [
			["function closeNewSpec(", "newSpecGuard"],
			["function closeNewWorkspace(", "newWorkspaceGuard"],
			["function closeConfig(", "configGuard"],
			["function closePRD(", "prdGuard"],
		];
		for (const [marker, guard] of guarded) {
			const body = sectionOf(js, marker);
			assert.ok(
				body.includes(`${guard}.allowsClose()`),
				`\`${marker}\` non chiede più conferma: una bozza si perderebbe con Esc o un click sul fondale`,
			);
		}
	});

	it("annullare la modifica del PRD è una chiusura come le altre", () => {
		const body = sectionOf(js, "function exitPrdEditMode(");
		assert.ok(
			body.includes("prdGuard.allowsClose()"),
			"il comando Annulla del PRD scarta il testo scritto senza chiedere niente",
		);
	});

	it("una sentinella disarmata non fa domande", () => {
		const body = sectionOf(js, "function createDirtyGuard(");
		assert.ok(
			/isDirty\(\)\s*\{[\s\S]*baseline === null/.test(body),
			"la sentinella non distingue più lo stato non armato: chiederebbe conferma anche senza niente da perdere",
		);
	});
});

// ---------------------------------------------------------------------------
// P1 · Dati — l'approvazione da trascinamento
// ---------------------------------------------------------------------------

describe("P1 — approvare per trascinamento chiede quanto il bottone", () => {
	it("la domanda è scritta in un posto solo", () => {
		const occurrences = [...js.matchAll(/function confirmApproval\(/g)].length;
		assert.equal(
			occurrences,
			1,
			"la conferma dell'approvazione non è più una funzione sola: le due strade potrebbero divergere",
		);
	});

	it("il drop su Done e il bottone Approva passano dalla stessa domanda", () => {
		for (const marker of ["async function onDragMove(", "async function onApprove("]) {
			const body = sectionOf(js, marker);
			assert.ok(
				body.includes("confirmApproval("),
				`\`${marker}\` non chiede conferma: un'approvazione irreversibile partirebbe da sola`,
			);
		}
	});

	it("una conferma negata rimette la card dov'era", () => {
		const body = sectionOf(js, "async function onDragMove(");
		const at = body.indexOf("confirmApproval(");
		const after = body.slice(at, at + 400);
		assert.ok(
			after.includes("renderBoard(boardSnapshot)") || after.includes("loadBoard()"),
			"rifiutare la conferma lascia la card in Done: la board mentirebbe sullo stato della spec",
		);
	});
});

// ---------------------------------------------------------------------------
// P1 · Accessibilità — il feedback
// ---------------------------------------------------------------------------

describe("P1 — il toast è percepibile da tutti", () => {
	it("vive dentro una regione annunciabile che resta sempre nel DOM", () => {
		const region = openingTagOf(html, "toast-region");
		assert.match(
			region,
			/role="status"/,
			"la regione del toast non porta role=\"status\": nessuno screen reader annuncerebbe il messaggio",
		);
		assert.ok(
			html.indexOf('id="toast-region"') < html.indexOf('id="toast"'),
			"il toast non è più dentro la sua regione: la mutazione non verrebbe annunciata",
		);
	});

	it("si può chiudere", () => {
		assert.match(
			html,
			/id="toast-dismiss"/,
			"il toast non offre più un comando per chiuderlo",
		);
		assert.ok(
			js.includes('toastDismiss.addEventListener("click", dismissToast)'),
			"il comando di chiusura del toast non è collegato a niente",
		);
	});

	it("dura abbastanza da leggere il dettaglio del server", () => {
		const match = js.match(/const TOAST_DURATION = (\d+);/);
		assert.ok(match, "la durata del toast non è più dichiarata");
		assert.ok(
			Number(match[1]) >= 4000,
			`il toast dura ${match[1]}ms: un messaggio con il dettaglio del server non si fa in tempo a leggere`,
		);
	});

	it("un secondo messaggio si mette in coda invece di cancellare il primo", () => {
		const body = sectionOf(js, "function showToast(");
		assert.ok(
			body.includes("toastQueue.push"),
			"showToast non accoda più: un errore verrebbe cancellato dal messaggio successivo",
		);
	});
});

describe("P1 — aria-live sta sui messaggi, non sui contenitori ricostruiti", () => {
	it("le regioni riscritte da zero non sono più live", () => {
		for (const id of [
			"board",
			"workspace-conversation",
			"workspace-runs",
			"workspace-home",
		]) {
			const tag = openingTagOf(html, id);
			assert.ok(
				!tag.includes("aria-live"),
				`#${id} è di nuovo una regione live, ma viene ricostruita per intero: uno screen reader la riannuncerebbe di continuo. Tag: ${tag}`,
			);
		}
		assert.ok(
			!/id="board-stats"[^>]*aria-live/.test(js),
			"l'intestazione dei contatori è tornata live: viene riscritta a ogni ridisegno della board",
		);
	});

	it("le righe puntuali dei form restano live", () => {
		assert.match(
			html,
			/id="new-spec-status" aria-live="polite"/,
			"il messaggio di stato del form nuova spec non è più annunciato: è il posto giusto per aria-live",
		);
	});

	it("la decisione in attesa si annuncia, e la striscia non si riscrive se non cambia", () => {
		const runs = readFileSync(resolve(assetsDir, "workspace-runs.js"), "utf8");
		assert.match(
			runs,
			/class="ws-run-await" role="status"/,
			'la decisione in attesa è tornata role="note": è la cosa che aspetta una persona, non una postilla',
		);
		const body = sectionOf(js, "function renderWorkspaceRunsPanel(");
		assert.ok(
			body.includes("workspaceRunsHTML"),
			"la striscia si riscrive a ogni passata: il role=\"status\" al suo interno verrebbe riannunciato ogni pochi secondi",
		);
	});
});

// ---------------------------------------------------------------------------
// P1 · Accessibilità — fuoco e tastiera
// ---------------------------------------------------------------------------

describe("P1 — le modali mantengono la promessa di aria-modal", () => {
	it("ogni modale entra e esce dal trap del fuoco", () => {
		const opens = [
			["async function openPRD(", "prdModal"],
			["async function openMetrics(", "metricsModal"],
			["function openNewSpec(", "newSpecModal"],
			["async function openNewWorkspace(", "newWorkspaceModal"],
			["async function openWorkspaces(", "workspacesModal"],
			["async function openConfig(", "configModal"],
		];
		for (const [marker, root] of opens) {
			const body = sectionOf(js, marker);
			assert.ok(
				body.includes(`enterModal(${root}`),
				`\`${marker}\` non trattiene il fuoco: Tab uscirebbe nella board dietro`,
			);
		}
		const closes = [
			["function closePRD(", "prdModal"],
			["function closeMetrics(", "metricsModal"],
			["function closeNewSpec(", "newSpecModal"],
			["function closeNewWorkspace(", "newWorkspaceModal"],
			["function closeWorkspaces(", "workspacesModal"],
			["function closeConfig(", "configModal"],
		];
		for (const [marker, root] of closes) {
			const body = sectionOf(js, marker);
			assert.ok(
				body.includes(`leaveModal(${root})`),
				`\`${marker}\` non restituisce il fuoco al comando che aveva aperto la modale`,
			);
		}
	});

	it("lo sfondo diventa inerte e torna vivo", () => {
		assert.ok(
			sectionOf(js, "function enterModal(").includes('setAttribute("inert"'),
			"lo sfondo di una modale non è più inerte: resterebbe raggiungibile da Tab e dal puntatore",
		);
		assert.ok(
			sectionOf(js, "function leaveModal(").includes('removeAttribute("inert")'),
			"chiudere una modale non restituisce vita allo sfondo: la pagina resterebbe inerte per sempre",
		);
	});

	it("la regione del toast resta fuori dall'inerzia", () => {
		assert.ok(
			sectionOf(js, "function enterModal(").includes("toastRegion"),
			"il toast diventa inerte insieme allo sfondo: sopra una modale nessun errore verrebbe annunciato",
		);
	});

	it("Esc e il fondale rispondono solo per la modale in cima", () => {
		const guards = [...js.matchAll(/isTopModal\(/g)].length;
		assert.ok(
			guards >= 12,
			`solo ${guards} controlli sulla modale in cima: con due modali aperte Esc le chiuderebbe entrambe`,
		);
	});
});

describe("P1 — la card della board si apre da tastiera", () => {
	it("dichiara di essere un comando e riceve il fuoco", () => {
		const body = sectionOf(js, "function renderCard(");
		assert.ok(
			body.includes('el.setAttribute("role", "button")'),
			"la card non si dichiara più un comando",
		);
		assert.ok(
			body.includes('el.setAttribute("tabindex", "0")'),
			"la card non è più raggiungibile da tastiera: è l'oggetto interattivo primario del prodotto",
		);
		assert.ok(
			body.includes('"aria-label"'),
			"la card non dichiara il proprio nome accessibile: prenderebbe quello del comando di cancellazione che contiene",
		);
	});

	it("Invio e Spazio aprono il dettaglio", () => {
		const body = sectionOf(js, "function renderCard(");
		assert.ok(
			/keydown[\s\S]*"Enter"[\s\S]*" "/.test(body),
			"la card non risponde più a Invio e Spazio: il fuoco arriverebbe su un comando che non si può premere",
		);
	});

	it("il fuoco sulla card si vede", () => {
		assert.match(
			css,
			/\.card:focus-visible\s*\{/,
			"la card non ha più un anello di fuoco: si arriverebbe su una card senza sapere dove si è",
		);
	});
});
