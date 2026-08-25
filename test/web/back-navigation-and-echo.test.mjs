// test/web/back-navigation-and-echo.test.mjs
// Oracoli per le tre correzioni di percorribilità del visore.
// Esecuzione: node --test test/web/back-navigation-and-echo.test.mjs
//
// Sono tre cose che una persona nota subito e che nessun altro test presidia:
//
//   1. aprire la card di una spec è una navigazione, quindi il tasto Indietro
//      del browser deve riportare alla board e l'indirizzo deve dire quale spec
//      è aperta;
//   2. il dettaglio spec è un documento, non una colonna di testo appoggiata a
//      sinistra con mezzo riquadro bianco alla sua destra;
//   3. premendo invio il messaggio si vede subito, invece che dopo l'attesa
//      dell'agente.
//
// Il primo e il secondo sono proprietà di *dove* le cose sono scritte — la
// cronologia si guida da app.js, la misura da app.css — e si presidiano
// leggendo la sorgente, con la stessa disciplina di p1-audit-fixes.test.mjs. Il
// terzo è invece una funzione pura, e si prova chiamandola.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

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

const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");
const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");

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

function loadConversationRenderer() {
	const src = readFileSync(resolve(assetsDir, "conversation.js"), "utf8");
	const mod = { exports: {} };
	const ctx = createContext({ module: mod, window: undefined });
	runInContext(src, ctx);
	return mod.exports;
}

const { renderConversation } = loadConversationRenderer();

/** Toglie ogni attributo: resta solo ciò che una persona legge. */
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
		opened_at: "2026-08-25T10:00:00.000Z",
	},
	events: [
		{
			id: 1,
			seq: 1,
			at: "2026-08-25T10:00:01.000Z",
			kind: "text",
			text: "RISPOSTA-PRECEDENTE",
		},
	],
	last_id: 1,
	truncated: false,
};

describe("il tasto Indietro riporta dalla spec alla board", () => {
	it("aprire una spec aggiunge una voce di cronologia e la scrive nell'indirizzo", () => {
		const body = sectionOf(js, "async function openEditor(");
		assert.ok(
			body.includes("rememberSpecInHistory(code)"),
			"aprire una spec non tocca più la cronologia: il tasto Indietro non avrebbe niente da togliere e riporterebbe fuori dall'applicazione",
		);
		const remember = sectionOf(js, "function rememberSpecInHistory(");
		assert.ok(
			remember.includes("pushState"),
			"rememberSpecInHistory non aggiunge più una voce: aprire una card smetterebbe di essere una navigazione",
		);
		assert.ok(
			remember.includes("locationWithSpec(code)"),
			"la voce non porta più l'indirizzo della spec: ricaricare la pagina o condividere il link non arriverebbe più allo stesso posto",
		);
	});

	it("passare da una card all'altra non accumula voci", () => {
		const remember = sectionOf(js, "function rememberSpecInHistory(");
		assert.ok(
			remember.includes("historyHoldsSpec()") &&
				remember.includes("replaceState"),
			"con un dettaglio già aperto la voce non viene più riscritta: tornare alla board costerebbe tanti Indietro quante card sono state guardate",
		);
	});

	it("chi lascia il dettaglio passa sempre dalla cronologia", () => {
		assert.ok(
			js.includes('modalClose.addEventListener("click", leaveSpecDetail)'),
			"il comando di ritorno chiude di nuovo per conto proprio: la cronologia resterebbe a descrivere un dettaglio che non è più su schermo",
		);
		const shellView = sectionOf(js, "function setShellView(");
		assert.ok(
			shellView.includes("leaveSpecDetail()") &&
				!shellView.includes("closeModal()"),
			"la linguetta Board chiude di nuovo per conto proprio invece di tornare indietro di una voce",
		);
		const leave = sectionOf(js, "function leaveSpecDetail(");
		assert.ok(
			leave.includes("window.history.back()"),
			"leaveSpecDetail non torna più indietro: il comando sullo schermo e il tasto del browser prenderebbero due strade diverse",
		);
		assert.ok(
			leave.includes("closeModal()"),
			"senza History API leaveSpecDetail non chiude più niente: il dettaglio resterebbe aperto per sempre",
		);
	});

	it("popstate è ciò che apre e chiude davvero il dettaglio", () => {
		assert.ok(
			js.includes('window.addEventListener("popstate"'),
			"nessuno ascolta più popstate: il tasto Indietro non avrebbe alcun effetto sulla schermata",
		);
		const listener = sectionOf(js, 'window.addEventListener("popstate"');
		assert.ok(
			listener.includes("openEditor(code)"),
			"Avanti non riapre più la spec che la voce descrive",
		);
		assert.ok(
			listener.includes("closeModal()"),
			"Indietro non chiude più il dettaglio",
		);
		assert.ok(
			listener.includes("navigatingFromHistory = true"),
			"la navigazione dalla cronologia non è più distinta da quella comandata: aprire da popstate aggiungerebbe una voce che nessuno ha chiesto",
		);
	});

	it("chi arriva da un link ha sempre dove tornare", () => {
		const restore = sectionOf(js, "function restoreSpecFromLocation(");
		assert.ok(
			restore.includes("replaceState") && restore.includes("openEditor(code)"),
			"la voce d'ingresso non viene più riscritta come board prima di aprire il dettaglio: il primo Indietro porterebbe fuori dall'applicazione",
		);
		const boot = sectionOf(js, "async function boot(");
		assert.ok(
			boot.includes("restoreSpecFromLocation()"),
			"il boot non legge più la spec dall'indirizzo: un link a una spec aprirebbe una board qualunque",
		);
	});

	it("il comando di ritorno si legge, e apre la testata", () => {
		const header = html.slice(
			html.indexOf('<header class="modal-header">'),
			html.indexOf("</header>", html.indexOf('<header class="modal-header">')),
		);
		const backAt = header.indexOf('id="modal-close"');
		const titleAt = header.indexOf('id="story-editor-title"');
		assert.notEqual(backAt, -1, "il comando di ritorno non è più nella testata");
		assert.ok(
			backAt < titleAt,
			"il comando di ritorno è tornato in coda alla riga: si cerca all'inizio, non alla fine",
		);
		assert.ok(
			header.includes('id="modal-close-label"'),
			"il comando di ritorno è tornato una sola icona: una ✕ dice «chiudi la finestra», e il dettaglio non è una finestra",
		);
		assert.ok(
			js.includes("modalCloseLabel.textContent = layout.back.label"),
			"la parola sul comando non viene più dal modulo del layout: sarebbe una seconda scritta a mano, libera di dire un'altra cosa",
		);
	});

	it("chiudere il dettaglio senza navigare toglie la spec dall'indirizzo", () => {
		const close = sectionOf(js, "function closeModal(");
		assert.ok(
			close.includes("forgetSpecInHistory()"),
			"chiudere il dettaglio non ripulisce più la cronologia: un Indietro successivo riaprirebbe una spec eliminata",
		);
		const forget = sectionOf(js, "function forgetSpecInHistory(");
		assert.ok(
			forget.includes("navigatingFromHistory") && forget.includes("replaceState"),
			"forgetSpecInHistory non riscrive più la voce, o la riscrive anche quando a chiudere è stato il browser",
		);
	});
});

describe("il dettaglio spec ha una colonna di contenuto, centrata", () => {
	it("le linguette impaginano il loro contenuto su una misura sola", () => {
		assert.ok(
			css.includes("--spec-content-width:"),
			"il token della colonna del dettaglio non c'è più: la misura tornerebbe scritta a mano dove serve",
		);
		const rule = css.slice(
			css.indexOf(".spec-pane-body > .tab-panel > * {"),
		);
		assert.ok(
			rule.startsWith(".spec-pane-body > .tab-panel > * {"),
			"la regola che impagina il contenuto delle linguette non c'è più",
		);
		const block = rule.slice(0, rule.indexOf("}"));
		assert.ok(
			block.includes("max-width: var(--spec-content-width)"),
			`il contenuto delle linguette non ha più una misura. Regola trovata: ${block}`,
		);
		assert.ok(
			block.includes("margin-inline: auto"),
			`la colonna non è più centrata: il testo tornerebbe appoggiato a sinistra con lo spazio vuoto tutto a destra. Regola trovata: ${block}`,
		);
	});

	it("dentro quella colonna la prosa la riempie", () => {
		const at = css.indexOf(".spec-pane .markdown-rendered");
		assert.notEqual(
			at,
			-1,
			"la prosa del dettaglio non è più liberata dalla misura di lettura: dentro la colonna si ripeterebbe il vuoto a destra un livello più in dentro",
		);
		const block = css.slice(at, css.indexOf("}", at));
		assert.ok(
			block.includes("max-width: none"),
			`la prosa del dettaglio ha di nuovo un tetto proprio. Regola trovata: ${block}`,
		);
	});

	it("il diff resta largo quanto il riquadro", () => {
		const at = css.indexOf(".spec-pane-body > .tab-panel > .review-diff");
		assert.notEqual(
			at,
			-1,
			"il diff non è più eccezione alla colonna: le sue righe sono codice, e stringerle vuol dire mandarle a capo",
		);
		const block = css.slice(at, css.indexOf("}", at));
		assert.ok(
			block.includes("max-width: none"),
			`il diff è tornato dentro la colonna del testo. Regola trovata: ${block}`,
		);
	});
});

describe("il messaggio inviato si vede subito", () => {
	it("il renderer disegna in coda il messaggio ancora in consegna", () => {
		const html = renderConversation(LIVE, "", {
			pendingMessage: "QUELLO-CHE-HO-APPENA-SCRITTO",
		});
		const readable = visibleText(html);
		assert.ok(
			readable.includes("QUELLO-CHE-HO-APPENA-SCRITTO"),
			"il messaggio appena inviato non è testo visibile: chi ha premuto invio non vedrebbe niente finché l'agente non risponde",
		);
		assert.ok(
			readable.includes("in consegna"),
			"la riga non dichiara più di essere in consegna: si spaccerebbe per storia della conversazione",
		);
		const pendingAt = html.indexOf("conv-event-pending");
		const composerAt = html.indexOf("conv-composer");
		assert.ok(pendingAt !== -1, "la riga in consegna non ha più la sua classe");
		assert.ok(
			pendingAt < composerAt,
			"la riga in consegna non sta più fra la storia e il campo di scrittura",
		);
		assert.ok(
			html.indexOf("RISPOSTA-PRECEDENTE") < pendingAt,
			"la riga in consegna non è più in coda a ciò che è già stato detto",
		);
	});

	it("senza niente in consegna non resta nessun posto vuoto", () => {
		const html = renderConversation(LIVE, "", {});
		assert.ok(
			!html.includes("conv-event-pending"),
			"la riga in consegna viene disegnata anche quando non c'è niente da consegnare",
		);
		const onlySpaces = renderConversation(LIVE, "", { pendingMessage: "   \n" });
		assert.ok(
			!onlySpaces.includes("conv-event-pending"),
			"un messaggio fatto di soli spazi disegna comunque una riga",
		);
	});

	it("il messaggio in consegna non è markup", () => {
		const html = renderConversation(LIVE, "", {
			pendingMessage: '<img src=x onerror="alert(1)">',
		});
		assert.ok(
			!html.includes("<img"),
			"il testo scritto da una persona arriva al parser: sarebbe markup, non una frase",
		);
		assert.ok(
			html.includes("&lt;img"),
			"il testo non è più leggibile dopo l'escape",
		);
	});

	it("l'invio svuota il campo e mette il messaggio in coda prima di partire", () => {
		const body = sectionOf(js, "async function sendConversationMessage(");
		const pendingAt = body.indexOf("conversationPendingMessage = message");
		const awaitAt = body.indexOf("await apiPost");
		assert.notEqual(
			pendingAt,
			-1,
			"l'invio non mette più il messaggio in coda alla conversazione",
		);
		assert.ok(
			pendingAt < awaitAt,
			"il messaggio compare in coda solo dopo la risposta del server: l'attesa resterebbe senza alcun segno",
		);
		assert.ok(
			body.indexOf('conversationDraft = ""') < awaitAt,
			"il campo si svuota solo dopo la risposta del server: il messaggio sarebbe in due posti insieme",
		);
	});

	it("una consegna rifiutata non lascia niente in coda e rimette il testo nel campo", () => {
		const body = sectionOf(js, "async function sendConversationMessage(");
		const catchAt = body.indexOf("} catch (err) {");
		assert.notEqual(catchAt, -1, "l'invio non gestisce più il rifiuto");
		const onFailure = body.slice(catchAt);
		assert.ok(
			onFailure.includes('conversationPendingMessage = ""'),
			"dopo un rifiuto la riga in consegna resta: mostrerebbe come consegnato un messaggio che non è partito",
		);
		assert.ok(
			onFailure.includes("conversationDraft = message"),
			"dopo un rifiuto il testo non torna nel campo: chi l'ha scritto dovrebbe riscriverlo per leggere la ragione",
		);
	});

	it("l'eco sparisce quando l'agente riporta indietro il messaggio", () => {
		const settle = sectionOf(js, "function settleConversationPending(");
		assert.ok(
			settle.includes('event.kind !== "user_message"'),
			"l'eco non si sistema più sull'evento che l'agente riporta",
		);
		assert.ok(
			settle.includes('conversationPendingMessage = ""'),
			"l'eco non viene più tolta: la stessa frase resterebbe scritta due volte",
		);
		const apply = sectionOf(js, "function applyConversationView(");
		assert.ok(
			apply.includes("settleConversationPending(event)"),
			"nessuno sistema più l'eco mentre la storia cresce",
		);
	});

	it("l'eco appartiene alla conversazione in cui è stata scritta", () => {
		const reset = sectionOf(js, "function resetConversationState(");
		assert.ok(
			reset.includes('conversationPendingMessage = ""'),
			"cambiando workspace l'eco sopravvive: comparirebbe sotto la conversazione di un altro progetto",
		);
		const switching = sectionOf(js, "async function switchToConversation(");
		assert.ok(
			switching.includes('conversationPendingMessage = ""'),
			"cambiando conversazione l'eco sopravvive: sarebbe una frase detta a qualcun altro",
		);
	});
});
