// test/web/review-actions.test.mjs
// Oracoli strutturali sulle azioni della scheda Review.
// Esecuzione: node --test test/web/review-actions.test.mjs
//
// La scheda offre due strade e non una: riportare la storia a TODO, oppure
// chiuderla. La chiusura non si spegne mai — è la persona a decidere, non il
// dossier — ma cambia faccia a seconda di che cosa sta facendo: approvare un
// incremento promosso, o chiuderne uno che il dossier dichiara bloccato, con o
// senza un ramo che viaggia con la chiusura.
//
// Sono fatti che nessun test unitario può presidiare, perché app.js è una IIFE
// senza DOM sotto e quello che conta è *dove* le cose sono scritte. Da qui la
// lettura delle sorgenti, incluse quelle Go per il verdetto, che è un fatto di
// dominio e non di interfaccia.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoDir = resolve(__dirname, "..", "..");
const assetsDir = resolve(repoDir, "cli", "internal", "web", "assets");
const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");
const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");
const go = readFileSync(
	resolve(repoDir, "cli", "internal", "domain", "types.go"),
	"utf8",
);

/** Il corpo bilanciato che si apre dopo `marker`: una funzione sola, non il file. */
function sectionOf(source, marker) {
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `la sorgente non contiene più \`${marker}\``);
	const open = source.indexOf("{", at);
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

describe("le azioni della scheda Review", () => {
	it("offre sempre due strade: indietro a TODO, oppure la chiusura", () => {
		const toolbar = html.slice(
			html.indexOf('<div class="review-actions">'),
			html.indexOf("</div>", html.indexOf('<div class="review-actions">')),
		);
		const bottoni = toolbar.match(/<button/g) || [];
		assert.equal(
			bottoni.length,
			2,
			"la toolbar della revisione deve avere due bottoni esatti: uno riporta a TODO, l'altro chiude",
		);
		assert.match(
			toolbar,
			/id="review-request-btn"/,
			"manca la strada per riportare la storia a TODO",
		);
		assert.match(
			toolbar,
			/id="review-approve-btn"/,
			"manca la strada per chiudere la storia",
		);
		assert.doesNotMatch(
			toolbar,
			/review-integrate-btn/,
			"il bottone di integrazione è tornato: /approve integra già da sé quando c'è un ramo, e un terzo bottone rimette il vicolo cieco dove il worktree è spento",
		);
	});

	it("chiude sempre, e l'etichetta dice quale delle due cose sta facendo", () => {
		const fn = sectionOf(js, "function updateCloseButton()");
		for (const caption of [
			"closeApprove",
			"closeApproveIntegrate",
			"closeForce",
			"closeForceIntegrate",
		]) {
			assert.match(
				fn,
				new RegExp(`TEXT\\.${caption}\\b`),
				`updateCloseButton non usa più ${caption}: una delle quattro combinazioni di dossier e ramo resta senza etichetta propria`,
			);
		}
		assert.doesNotMatch(
			js,
			/reviewApproveBtn\.disabled = /,
			"il bottone di chiusura torna a spegnersi: una spec respinta non avrebbe più nessuna strada per essere chiusa",
		);
		assert.match(
			fn,
			/danger-ghost-btn/,
			"la chiusura forzata non si distingue più dall'approvazione: stesso vestito per due gesti diversi",
		);
	});

	it("non veste da forzatura una chiusura che ha solo segnalazioni minori", () => {
		const fn = sectionOf(js, "function updateCloseButton()");
		assert.doesNotMatch(
			fn,
			/minor_findings/,
			"la faccia del bottone guarda di nuovo le segnalazioni minori: un incremento che nessuno blocca si chiuderebbe come una forzatura",
		);
		assert.match(
			go,
			/MinorFindings \[\]string/,
			"il dossier non ha più un posto per le segnalazioni non bloccanti: tornerebbero tutte nei blocker",
		);
		assert.match(
			sectionOf(js, "function renderDossier(review)"),
			/dossier\.minor_findings/,
			"il dossier non mostra più le segnalazioni non bloccanti: sarebbero raccolte e mai lette",
		);
	});

	it("registra e mostra i tre verdetti, non due", () => {
		assert.match(
			js,
			/closed_over_blockers/,
			"il viewer non conosce più il verdetto di chiusura forzata: lo mostrerebbe come «Modifiche richieste»",
		);
		assert.match(
			go,
			/ReviewDecisionClosedOverBlockers = "closed_over_blockers"/,
			"il dominio non ha più il terzo verdetto: una chiusura sopra i rilievi verrebbe registrata come approvazione",
		);
	});

	it("nasconde davvero ciò che il JS marca come hidden", () => {
		// `.ghost-btn` dà `display: inline-flex` ai bottoni, e una dichiarazione
		// d'autore batte il `[hidden] { display: none }` del browser: senza questa
		// regola `el.hidden = true` nasconde solo nel codice, e a schermo
		// l'elemento resta lì, cliccabile.
		assert.match(
			css,
			/\[hidden\] \{\s*display: none !important;\s*\}/,
			"app.css non nasconde più l'attributo hidden: ogni elemento con un display d'autore resterebbe visibile",
		);
	});

	it("annuncia in un toast ogni fallimento delle due azioni", () => {
		assert.match(
			js,
			/function reviewFailed\(err\)[\s\S]{0,300}?showToast\(msg, "err"\)/,
			"reviewFailed non emette più il toast: il fallimento resterebbe scritto solo sotto il diff, fuori schermo",
		);
		assert.equal(
			(js.match(/\breviewFailed\(err\);/g) || []).length,
			2,
			"le due azioni della revisione — chiedi modifiche e chiudi — devono passare entrambe da reviewFailed",
		);
	});

	it("non manda a sbattere chi chiede modifiche senza avere nulla da dire", () => {
		// Il server rifiuta con 400 una richiesta di modifiche senza rilievi:
		// la domanda arriva prima, dove si può ancora scrivere qualcosa.
		assert.match(
			js,
			/countReworkItems\(freeText\) === 0/,
			"onRequestChanges non controlla più di avere qualcosa da rimandare indietro",
		);
	});
});
