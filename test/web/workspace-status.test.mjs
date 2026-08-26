// test/web/workspace-status.test.mjs
// Tests for the pure workspace-status module used by the ARchetipo web viewer.
// Run: node --test test/web/workspace-status.test.mjs
//
// Since US-061 the module renders nothing: the recommended step is drawn at
// the tail of the thread by conversation.js, and its rendering is tested in
// conversation.test.mjs (label, target spec, position, disabled state, reason
// inside the block). What is left here is the decision — whether pressing the
// step starts anything, and on what — which is the one part of the frontend
// this project can test without a DOM, and the part that answers anyone
// reaching the handler by any route.
//
// Verifies:
//   - US-056 AC-2 the recommended step dispatches exactly the action the
//     payload declares, on exactly the target it names
//   - US-056 AC-4 a blocked step dispatches nothing, whatever the markup says
//   - US-056 AC-5 an absent workspace status suggests no step at all
//   - the module carries no process rules and echoes identifiers it has never
//     seen

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

const { nextStepTarget, nextStepDispatch } = loadWorkspaceStatus();

// The absence of process knowledge is a property of the module itself, not of
// anything it returns: it survived the removal of the rendering because it is
// what makes an action identifier the module has never seen travel through
// untouched.
describe("il modulo non contiene regole di processo", () => {
	it("non nomina alcuno stadio né alcuna azione reale", () => {
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
				`il modulo non deve contenere l'identificativo di processo ${token}`,
			);
		}
	});

	it("riporta tale e quale un'azione mai vista", () => {
		const target = nextStepTarget({
			next_step: {
				scope: "ambito-inventato",
				action: "azione-inventata",
				runnable: true,
			},
		});
		assert.equal(target.action, "azione-inventata");
		assert.equal(target.scope, "ambito-inventato");
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

describe("nextStepDispatch", () => {
	it("avvia sulla spec bersaglio l'azione dichiarata dal payload", () => {
		const dispatch = nextStepDispatch({
			next_step: {
				scope: "spec",
				action: "plan",
				label: "Pianifica",
				runnable: true,
				spec: { code: "US-010" },
			},
		});

		assert.equal(dispatch.scope, "spec");
		assert.equal(dispatch.action, "plan");
		assert.equal(dispatch.code, "US-010");
		// Il nome del passo viaggia con quello che lo avvia: è il nome che il
		// filo aperto per questo passo si porta nell'elenco delle conversazioni,
		// dove nessuno scriverà per primo e la data non direbbe quale.
		assert.equal(dispatch.label, "Pianifica");

		// Un identificativo mai visto prima deve uscire tale e quale: nessuna
		// regola di processo vive nel modulo.
		const invented = nextStepDispatch({
			next_step: {
				scope: "spec",
				action: "azione-mai-vista",
				runnable: true,
				spec: { code: "US-011" },
			},
		});
		assert.equal(invented.action, "azione-mai-vista");
		assert.equal(invented.code, "US-011");
		// Nessun nome nel payload, nessun nome inventato qui.
		assert.equal(invented.label, "");
	});

	it("avvia l'azione di workspace senza spec bersaglio", () => {
		const dispatch = nextStepDispatch({
			next_step: {
				scope: "workspace",
				action: "inception",
				label: "Inception",
				spec: null,
				runnable: true,
			},
		});

		assert.equal(dispatch.scope, "workspace");
		assert.equal(dispatch.action, "inception");
		assert.ok(!dispatch.code, "un passo di workspace non ha una spec bersaglio");
	});

	// The refusal that counts: an attribute on the markup is a hint to the
	// pointer, while this is the answer given to anyone who reaches the handler
	// by any route. That the button is drawn disabled, and that its condition is
	// readable inside the block, is proved in conversation.test.mjs.
	it("un passo bloccato non avvia nulla", () => {
		const step = {
			scope: "workspace",
			action: "inception",
			label: "Inception",
			runnable: false,
			unlocked_by: "installa un provider utilizzabile",
		};

		assert.equal(
			nextStepDispatch({ next_step: step }),
			null,
			"un passo bloccato produce comunque un avvio",
		);
	});

	it("un passo di spec senza spec bersaglio non è avviabile", () => {
		assert.equal(
			nextStepDispatch({
				next_step: { scope: "spec", action: "plan", runnable: true, spec: null },
			}),
			null,
		);
	});

	it("senza workspace aperto nessun passo è suggerito né avviabile", () => {
		for (const view of [null, {}, { next_step: null }]) {
			assert.equal(
				nextStepDispatch(view),
				null,
				`un payload ${JSON.stringify(view)} non deve suggerire nessun passo`,
			);
			assert.equal(
				nextStepTarget(view),
				null,
				`un payload ${JSON.stringify(view)} non deve nominare nessun bersaglio`,
			);
		}
	});

	it("avvio e navigazione leggono lo stesso payload senza divergere", () => {
		const view = {
			next_step: {
				scope: "spec",
				action: "azione-mai-vista",
				runnable: true,
				spec: { code: "US-012" },
			},
		};

		const dispatch = nextStepDispatch(view);
		const target = nextStepTarget(view);
		assert.ok(dispatch, "il passo eseguibile deve produrre un dispatch");
		assert.equal(dispatch.action, target.action);
		assert.equal(dispatch.scope, target.scope);
		assert.equal(dispatch.code, target.code);
		assert.equal(target.runnable, true);
	});
});
