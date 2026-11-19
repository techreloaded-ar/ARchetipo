// test/web/viewer-unreachable.test.mjs
// Esecuzione: node --test test/web/viewer-unreachable.test.mjs
//
// Quando la macchina che ospita il viewer si blocca (per esempio la VM di
// Docker che esaurisce la memoria), ogni richiesta resta appesa. La pagina
// diceva che «questa conversazione» non si poteva più leggere e, dopo una
// ricarica, aprire la conversazione lasciava il pannello vuoto per sempre.
// Questi oracoli leggono app.js: il comportamento vive dentro l'IIFE e non è
// esportato, quindi si presidia dove è scritto.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = readFileSync(
	resolve(__dirname, "..", "..", "cli", "internal", "web", "assets", "app.js"),
	"utf8",
);

function functionBody(name) {
	const start = app.search(new RegExp(`(async\\s+)?function ${name}\\(`));
	assert.ok(start >= 0, `app.js non definisce più ${name}`);
	const open = app.indexOf("{", start);
	let depth = 0;
	for (let i = open; i < app.length; i += 1) {
		if (app[i] === "{") depth += 1;
		if (app[i] === "}") depth -= 1;
		if (depth === 0) return app.slice(open, i + 1);
	}
	throw new Error(`corpo di ${name} non chiuso`);
}

describe("viewer che non risponde", () => {
	it("ha una frase sua, distinta dalla conversazione non leggibile", () => {
		assert.match(app, /viewerUnreachable:\s*\n?\s*"Il viewer non risponde/);
		assert.match(app, /docker version/, "la frase non indica come controllare Docker");
	});

	it("chiede al viewer una risorsa statica, senza cache e con una scadenza", () => {
		const probe = functionBody("viewerAnswers");
		assert.match(probe, /fetch\("\/logo\.svg"/);
		assert.match(probe, /cache:\s*"no-store"/);
		assert.match(probe, /VIEWER_PROBE_TIMEOUT_MS/);
	});

	it("nomina il viewer solo se anche la risorsa statica non torna", () => {
		const note = functionBody("conversationReadFailureNote");
		assert.match(note, /isUnansweredRead\(err\)\s*&&\s*!\(await viewerAnswers\(\)\)/);
		assert.match(note, /TEXT\.viewerUnreachable/);
		assert.match(note, /TEXT\.conversationUnreadable/);
	});

	it("la prima lettura di una conversazione ha la stessa scadenza del polling", () => {
		const open = functionBody("switchToConversation");
		assert.match(open, /setTimeout\(\(\) => controller\.abort\(\), CONVERSATION_READ_TIMEOUT_MS\)/);
		assert.match(open, /signal: controller\.signal/);
		assert.match(open, /conversationReadFailureNote\(err\)/);
	});

	it("il polling che si arrende spiega se è fermo il viewer o la conversazione", () => {
		const poll = functionBody("startConversationPolling");
		assert.match(poll, /conversationReadFailureNote\(err\)/);
		assert.doesNotMatch(poll, /entro 15 secondi/, "la scadenza non va riscritta a mano nel polling");
	});
});
