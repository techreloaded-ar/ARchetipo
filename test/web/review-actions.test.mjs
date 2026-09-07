// test/web/review-actions.test.mjs
// Oracoli strutturali sulle azioni della scheda Review.
// Esecuzione: node --test test/web/review-actions.test.mjs
//
// Due fatti che nessun test unitario può presidiare, perché app.js è una IIFE
// senza DOM sotto e quello che conta è *dove* le cose sono scritte:
//
//   - «Integra e chiudi» compare solo quando c'è un ramo da integrare. Senza
//     ramo il server rifiuta (worktree spento, o spec mai partita in un
//     worktree) e il bottone prometteva una chiusura che non poteva dare;
//   - un'azione di revisione che fallisce lo dice anche in un toast. La riga
//     di stato del pannello sta in fondo, sotto il diff: su un incremento
//     lungo cade fuori dallo schermo, e il rifiuto sembrava un click andato
//     nel vuoto.

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
const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");

describe("le azioni della scheda Review", () => {
	it("mostra «Integra e chiudi» solo quando la spec ha un ramo", () => {
		assert.match(
			js,
			/reviewIntegrateBtn\.hidden = !diff\.branch;/,
			"renderReviewBranch non lega più la visibilità del bottone al ramo: senza ramo l'integrazione è un vicolo cieco",
		);
		assert.match(
			js,
			/reviewIntegrateBtn\.hidden = true;/,
			"il pannello di revisione è unico e riusato: senza il ripristino il bottone resta visibile sulla spec successiva",
		);
	});

	it("lascia premibile «Approva» anche sopra un dossier bloccato", () => {
		// È l'unica strada per chiudere una spec respinta quando i worktree sono
		// disabilitati e «Integra e chiudi» non c'è: spegnere il bottone lasciava
		// come sola azione la richiesta di modifiche, cioè nessuna uscita.
		assert.doesNotMatch(
			js,
			/reviewApproveBtn\.disabled = blockers\.length > 0;/,
			"«Approva» torna spento sopra i blocker: una spec respinta non si potrebbe più chiudere senza worktree",
		);
		assert.match(
			js,
			/approveConfirmBlocked/,
			"la conferma non nomina più i rilievi che si stanno scavalcando",
		);
	});

	it("nasconde davvero ciò che il JS marca come hidden", () => {
		// `.ghost-btn` dà `display: inline-flex` ai bottoni, e una dichiarazione
		// d'autore batte il `[hidden] { display: none }` del browser: senza questa
		// regola `reviewIntegrateBtn.hidden = true` è nascosto solo nel codice, e a
		// schermo il bottone resta lì, cliccabile.
		assert.match(
			css,
			/\[hidden\] \{\s*display: none !important;\s*\}/,
			"app.css non nasconde più l'attributo hidden: ogni elemento con un display d'autore resterebbe visibile",
		);
	});

	it("annuncia in un toast ogni fallimento delle tre azioni", () => {
		assert.match(
			js,
			/function reviewFailed\(err\)[\s\S]{0,300}?showToast\(msg, "err"\)/,
			"reviewFailed non emette più il toast: il fallimento resterebbe scritto solo sotto il diff",
		);
		assert.equal(
			(js.match(/\breviewFailed\(err\);/g) || []).length,
			3,
			"le tre azioni della revisione — chiedi modifiche, approva, integra — devono passare tutte da reviewFailed",
		);
	});
});
