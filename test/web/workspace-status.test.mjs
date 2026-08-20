// test/web/workspace-status.test.mjs
// Tests for the pure workspace-status renderer used by the ARchetipo web viewer.
// Run: node --test test/web/workspace-status.test.mjs
//
// The oracles are on the *visible text* of the rendered HTML, not on the shape
// of the module: what the person reading the board actually sees is what the
// acceptance criteria are about.
//
// Verifies:
//   - AC-1 the stage is declared with the Archetipo's own words
//   - AC-2 the recommended step names its label, scope, action and target spec
//   - AC-3 the renderer carries no process rules and echoes identifiers it
//     has never seen
//   - AC-4 a refused action states the condition that unlocks it, as text

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
	"workspace-status.js",
);

// Same minimal virtual-machine loader as task-markdown.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadWorkspaceStatus() {
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

const { renderWorkspaceStatus, nextStepTarget } = loadWorkspaceStatus();

// Strip every attribute from the markup, leaving only what a reader sees.
// An identifier that survives this is visible text; one that does not was
// only ever hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

describe("renderWorkspaceStatus", () => {
	it("dichiara lo stadio con le parole dell'Archetipo", () => {
		const html = renderWorkspaceStatus({
			template: { id: "identificativo-archetipo", label: "ARCHETIPO-Y", version: "9.9.9" },
			stage: { id: "stadio-x", label: "ETICHETTA-X", summary: "SINTESI-X" },
		});

		const text = visibleText(html);
		assert.ok(text.includes("ETICHETTA-X"), "stage label mancante nel testo reso");
		assert.ok(text.includes("SINTESI-X"), "stage summary mancante nel testo reso");
		assert.ok(text.includes("ARCHETIPO-Y"), "identità dell'Archetipo mancante");
		assert.ok(text.includes("9.9.9"), "versione dell'Archetipo mancante");
	});

	it("ricade sull'id dell'Archetipo quando manca l'etichetta", () => {
		const html = renderWorkspaceStatus({
			template: { id: "IDENTIFICATIVO-T", version: "1.2.3" },
			stage: { id: "s", label: "L", summary: "S" },
		});
		assert.ok(visibleText(html).includes("IDENTIFICATIVO-T"));
	});

	it("nomina il passo raccomandato e l'azione che lo esegue", () => {
		const workspaceStep = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "s", label: "L", summary: "S" },
			next_step: {
				scope: "workspace",
				action: "azione-inventata",
				label: "ETICHETTA-AZIONE",
				runnable: true,
			},
		});

		assert.ok(
			visibleText(workspaceStep).includes("ETICHETTA-AZIONE"),
			"l'etichetta del passo non è testo visibile",
		);
		assert.ok(workspaceStep.includes('data-next-action="azione-inventata"'));
		assert.ok(workspaceStep.includes('data-next-scope="workspace"'));
		assert.ok(
			!/<button[^>]*\sdisabled/.test(workspaceStep),
			"un passo eseguibile non deve essere disabled",
		);

		const specStep = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "s", label: "L", summary: "S" },
			next_step: {
				scope: "spec",
				action: "azione-inventata",
				label: "ETICHETTA-AZIONE",
				runnable: true,
				spec: { code: "US-777", title: "Una spec bersaglio" },
			},
		});

		assert.ok(
			visibleText(specStep).includes("US-777"),
			"il codice della spec bersaglio non è testo visibile",
		);
		assert.ok(specStep.includes('data-next-spec="US-777"'));
		assert.ok(specStep.includes('data-next-scope="spec"'));
	});

	it("mostra la condizione che sblocca il passo non eseguibile", () => {
		const html = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "s", label: "L", summary: "S" },
			next_step: {
				scope: "workspace",
				action: "azione-inventata",
				label: "ETICHETTA-AZIONE",
				runnable: false,
				unlocked_by: "CONDIZIONE-PASSO",
			},
		});

		assert.ok(/<button[^>]*\sdisabled/.test(html), "il passo non eseguibile deve essere disabled");
		assert.ok(
			visibleText(html).includes("CONDIZIONE-PASSO"),
			"la condizione di sblocco del passo non è testo visibile",
		);
	});

	it("senza passo raccomandato mostra la sintesi dello stadio e nessun bottone", () => {
		const html = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "s", label: "L", summary: "SINTESI-FINALE" },
		});

		assert.ok(html.includes("ws-status-next-none"));
		assert.ok(visibleText(html).includes("SINTESI-FINALE"));
		assert.ok(!html.includes("<button"), "senza passo non deve esserci un bottone");
	});

	it("non contiene regole di processo", () => {
		const html = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "stadio-inventato", label: "ETICHETTA-INVENTATA", summary: "S" },
			next_step: {
				scope: "workspace",
				action: "azione-inventata",
				label: "ETICHETTA-AZIONE",
				runnable: true,
			},
		});

		// Identifiers the module has never seen are echoed as they are.
		assert.ok(html.includes('data-next-action="azione-inventata"'));
		const withUnknownStageLabel = renderWorkspaceStatus({
			stage: { id: "stadio-inventato" },
		});
		assert.ok(
			visibleText(withUnknownStageLabel).includes("stadio-inventato"),
			"uno stadio sconosciuto deve essere riportato tale e quale",
		);

		// And the source itself declares no real stage or action identifier.
		const source = readFileSync(helperPath, "utf8");
		const forbidden = [
			"senza-prd",
			"senza-backlog",
			"da-pianificare",
			"da-implementare",
			"da-rivedere",
			"completo",
			'"inception"',
			'"backlog"',
			'"plan"',
			'"implement"',
			'"review"',
		];
		for (const token of forbidden) {
			assert.ok(
				!source.includes(token),
				`il renderer non deve contenere l'identificativo di processo ${token}`,
			);
		}
	});

	it("mostra la condizione che sblocca ogni azione non eseguibile", () => {
		const html = renderWorkspaceStatus({
			template: { label: "T", version: "1" },
			stage: { id: "s", label: "L", summary: "S" },
			actions: [
				{ id: "a1", label: "A1", runnable: false, unlocked_by: "CONDIZIONE-Z" },
				{ id: "a2", label: "A2", runnable: true },
			],
		});

		assert.ok(html.includes("CONDIZIONE-Z"));
		// The condition must survive attribute stripping: a chip with only a
		// tooltip would fail here, a talking chip passes.
		assert.ok(
			visibleText(html).includes("CONDIZIONE-Z"),
			"la condizione di sblocco è presente solo in un attributo",
		);

		// Le chip della striscia sono informative in entrambi i casi: la striscia
		// dichiara, non esegue. Un <button> qui sarebbe focalizzabile e annunciato
		// come premibile senza che nessuno lo ascolti — un controllo inerte.
		assert.ok(
			/<span[^>]*data-action-id="a1"/.test(html),
			"la chip non eseguibile deve essere uno <span>",
		);
		assert.ok(
			/<span[^>]*data-action-id="a2"/.test(html),
			"anche la chip eseguibile della striscia deve essere uno <span>",
		);
		assert.ok(
			!/<button[^>]*data-action-id=/.test(html),
			"nessuna chip della striscia deve essere un bottone",
		);

		const onlyReason = renderWorkspaceStatus({
			stage: { id: "s", label: "L", summary: "S" },
			actions: [
				{
					id: "a3",
					label: "A3",
					runnable: false,
					unavailable_reason: "MOTIVO-VISIBILE",
				},
			],
		});
		assert.ok(
			visibleText(onlyReason).includes("MOTIVO-VISIBILE"),
			"con il solo unavailable_reason la frase deve restare testo visibile",
		);
	});

	it("non lancia su payload parziali", () => {
		for (const view of [null, undefined, {}, { actions: [] }, { next_step: null }]) {
			const html = renderWorkspaceStatus(view);
			assert.equal(typeof html, "string");
		}
	});

	it("neutralizza l'HTML che arriva dal payload", () => {
		const html = renderWorkspaceStatus({
			stage: { id: "s", label: '<img src=x onerror=1>', summary: "S" },
		});
		assert.ok(html.includes("&lt;img"), "il markup del payload non è stato neutralizzato");
		assert.ok(!html.includes("<img"), "il payload ha prodotto un tag reale");
	});
});

describe("nextStepTarget", () => {
	it("riporta il bersaglio del passo", () => {
		assert.equal(nextStepTarget(null), null);
		assert.equal(nextStepTarget({}), null);
		assert.equal(nextStepTarget({ next_step: null }), null);

		const workspaceTarget = nextStepTarget({
			next_step: { scope: "workspace", action: "azione-inventata", label: "A" },
		});
		assert.equal(workspaceTarget.scope, "workspace");
		assert.equal(workspaceTarget.action, "azione-inventata");
		// No target spec for a workspace-scoped step.
		assert.ok(!workspaceTarget.code, "un passo di workspace non ha una spec bersaglio");

		const specTarget = nextStepTarget({
			next_step: {
				scope: "spec",
				action: "azione-inventata",
				label: "A",
				spec: { code: "US-777" },
			},
		});
		// Field-by-field: the module runs in a vm realm, so its objects do not
		// share this realm's Object prototype and deepStrictEqual would fail on
		// identity rather than on content.
		assert.equal(specTarget.scope, "spec");
		assert.equal(specTarget.action, "azione-inventata");
		assert.equal(specTarget.code, "US-777");
	});
});
