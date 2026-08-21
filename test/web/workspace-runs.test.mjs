// test/web/workspace-runs.test.mjs
// Tests for the pure workspace-runs renderer used by the ARchetipo web viewer.
// Run: node --test test/web/workspace-runs.test.mjs
//
// The oracles are on the *visible text* of the rendered HTML, not on the shape
// of the module: what the person reading the rail actually sees is what the
// acceptance criteria are about. The only exception is the target of each row,
// where the markup itself is the object under test — the caller navigates from
// those attributes, so they are asserted before the text is stripped.
//
// Verifies:
//   - AC-3 every run is listed with its target, its action and its status
//   - AC-4 the run waiting for an answer is marked, and only that one
//   - the renderer carries no process rules and echoes identifiers it has
//     never seen
//   - partial payloads and payload HTML are handled without harm

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
	"workspace-runs.js",
);

// Same minimal virtual-machine loader as workspace-status.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadWorkspaceRuns() {
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

const { renderWorkspaceRuns, awaitingCount } = loadWorkspaceRuns();

// Strip every attribute from the markup, leaving only what a reader sees.
// An identifier that survives this is visible text; one that does not was
// only ever hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

describe("renderWorkspaceRuns", () => {
	it("elenca ogni run con il suo bersaglio, la sua azione e il suo stato", () => {
		const html = renderWorkspaceRuns({
			runs: [
				{
					id: "esecuzione-1",
					scope: "spec",
					spec_code: "US-XYZ",
					action: "AZIONE-UNO",
					status: "STATO-UNO",
				},
				{
					id: "esecuzione-2",
					scope: "AMBITO-DUE",
					action: "AZIONE-DUE",
					status: "STATO-DUE",
				},
			],
		});

		const text = visibleText(html);
		assert.ok(text.includes("US-XYZ"), "codice della spec mancante nel testo reso");
		assert.ok(text.includes("AMBITO-DUE"), "ambito della run di workspace mancante");
		assert.ok(text.includes("AZIONE-UNO"), "azione della prima run mancante");
		assert.ok(text.includes("AZIONE-DUE"), "azione della seconda run mancante");
		assert.ok(text.includes("STATO-UNO"), "stato della prima run mancante");
		assert.ok(text.includes("STATO-DUE"), "stato della seconda run mancante");
	});

	it("segnala la run che attende una risposta e solo quella", () => {
		const view = {
			runs: [
				{
					id: "esecuzione-tranquilla",
					scope: "spec",
					spec_code: "US-CALMA",
					action: "AZIONE-A",
					status: "STATO-A",
				},
				{
					id: "esecuzione-in-attesa",
					scope: "spec",
					spec_code: "US-ATTESA",
					action: "AZIONE-B",
					status: "STATO-B",
					awaiting_response: true,
					pending: { id: "decisione-1", title: "TITOLO-DECISIONE" },
				},
			],
		};

		const html = renderWorkspaceRuns(view);

		// One row carries the waiting mark, and it is the marked one: the rows
		// are split on the markup, then each half is read as text.
		const rows = html
			.split("<li ")
			.slice(1)
			.map((row) => "<li " + row);
		assert.equal(rows.length, 2, "attese due righe nell'elenco");

		const calma = rows.find((row) => row.includes('data-run-id="esecuzione-tranquilla"'));
		const attesa = rows.find((row) => row.includes('data-run-id="esecuzione-in-attesa"'));
		assert.ok(calma, "riga della run non in attesa mancante");
		assert.ok(attesa, "riga della run in attesa mancante");

		const attesaText = visibleText(attesa);
		assert.ok(
			attesaText.includes("TITOLO-DECISIONE"),
			"titolo della decisione mancante nella riga in attesa",
		);
		assert.ok(
			/attend|attesa|wait/i.test(attesaText),
			"contrassegno di attesa mancante nella riga in attesa",
		);

		const calmaText = visibleText(calma);
		assert.ok(
			!calmaText.includes("TITOLO-DECISIONE"),
			"la decisione non deve comparire sulla run che non attende",
		);
		assert.ok(
			!/attend|attesa|wait/i.test(calmaText),
			"la run che non attende non deve portare il contrassegno",
		);

		assert.equal(awaitingCount(view), 1, "awaitingCount deve contare la sola run in attesa");
	});

	it("porta a ogni run il suo bersaglio", () => {
		const html = renderWorkspaceRuns({
			runs: [
				{ id: "id-spec", scope: "spec", spec_code: "US-BERSAGLIO", action: "a", status: "s" },
				{ id: "id-workspace", scope: "AMBITO-W", action: "a", status: "s" },
			],
		});

		assert.ok(html.includes('data-run-id="id-spec"'), "id di esecuzione mancante negli attributi");
		assert.ok(
			html.includes('data-run-id="id-workspace"'),
			"id della seconda esecuzione mancante negli attributi",
		);
		assert.ok(html.includes('data-run-scope="spec"'), "ambito mancante negli attributi");
		assert.ok(html.includes('data-run-scope="AMBITO-W"'), "secondo ambito mancante negli attributi");
		assert.ok(
			html.includes('data-run-spec="US-BERSAGLIO"'),
			"codice della spec mancante negli attributi",
		);
	});

	it("non contiene regole di processo", () => {
		const html = renderWorkspaceRuns({
			runs: [
				{
					id: "e1",
					scope: "AMBITO-MAI-VISTO",
					action: "AZIONE-MAI-VISTA",
					status: "STATO-MAI-VISTO",
				},
			],
		});

		const text = visibleText(html);
		assert.ok(text.includes("AZIONE-MAI-VISTA"), "azione sconosciuta non resa tale e quale");
		assert.ok(text.includes("STATO-MAI-VISTO"), "stato sconosciuto non reso tale e quale");
		assert.ok(text.includes("AMBITO-MAI-VISTO"), "ambito sconosciuto non reso tale e quale");

		// And the module itself must not know the real actions by name.
		const src = readFileSync(helperPath, "utf8");
		for (const action of [
			"plan",
			"implement",
			"review",
			"inception",
			"backlog",
			"spec-draft",
		]) {
			const pattern = new RegExp(`\\b${action.replace("-", "\\-")}\\b`, "i");
			assert.ok(
				!pattern.test(src),
				`il modulo non deve nominare l'azione reale "${action}"`,
			);
		}
	});

	it("senza run in corso lo dichiara", () => {
		const view = { runs: [] };
		const text = visibleText(renderWorkspaceRuns(view)).replace(/<[^>]*>/g, " ").trim();
		assert.ok(text.length > 0, "l'elenco vuoto deve dichiarare l'assenza, non sparire");
		assert.equal(awaitingCount(view), 0, "nessuna run in attesa su elenco vuoto");
	});

	it("non lancia su payload parziali", () => {
		assert.doesNotThrow(() => renderWorkspaceRuns(null));
		assert.doesNotThrow(() => renderWorkspaceRuns(undefined));
		assert.doesNotThrow(() => renderWorkspaceRuns({}));
		assert.doesNotThrow(() => renderWorkspaceRuns({ runs: null }));
		assert.doesNotThrow(() => renderWorkspaceRuns({ runs: [{}, null, { id: "solo-id" }] }));
		assert.doesNotThrow(() =>
			renderWorkspaceRuns({ runs: [{ awaiting_response: true }] }),
		);

		assert.equal(awaitingCount(null), 0);
		assert.equal(awaitingCount({}), 0);
		assert.equal(awaitingCount({ runs: [{}, { awaiting_response: true }] }), 1);
	});

	it("neutralizza l'HTML che arriva dal payload", () => {
		const html = renderWorkspaceRuns({
			runs: [
				{
					id: "e1",
					scope: "spec",
					spec_code: "US-1",
					action: "a",
					status: "s",
					awaiting_response: true,
					pending: { id: "d1", title: "<script>alert('x')</script>" },
				},
			],
		});

		assert.ok(!html.includes("<script>"), "il markup del payload non deve produrre un tag");
		assert.ok(html.includes("&lt;script&gt;"), "il markup del payload deve essere neutralizzato");
	});
});
