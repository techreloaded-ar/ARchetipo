// test/web/workspace-layout.test.mjs
// Tests for the pure workspace-layout decision module used by the ARchetipo
// web viewer.
// Run: node --test test/web/workspace-layout.test.mjs
//
// The oracles here are *invariants over every state*, not examples: the module
// takes three booleans, so the whole input space is eight states and the tests
// enumerate it rather than sampling it. What the person sees on screen is
// decided entirely by this function, so a rule that holds on a chosen example
// but breaks on a forgotten combination is a bug nobody would notice.
//
// Verifies:
//   - AC-1 in a wide window the conversation stays beside the work: the rail
//     and the primary column are visible together, spec open or not
//   - AC-2 the rail does not depend on the state of the primary column
//   - AC-6 in a narrow window exactly one pane is on screen, nothing that
//     exists is unreachable, and no state is empty
//   - the breakpoint is declared once: app.css and the module share one number

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
const helperPath = resolve(assetsDir, "workspace-layout.js");
const cssPath = resolve(assetsDir, "app.css");

// Same minimal virtual-machine loader as workspace-status.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadWorkspaceLayout() {
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

const { resolveLayout, NARROW_MAX_WIDTH, PANES, RAIL_FOCUS_VALUES } =
	loadWorkspaceLayout();

// The whole input space: two booleans plus every railFocus value the module
// declares as admissible. Nothing is chosen, everything is enumerated.
const STATES = [];
for (const specOpen of [false, true]) {
	for (const narrow of [false, true]) {
		for (const railFocus of RAIL_FOCUS_VALUES) {
			STATES.push({ specOpen, narrow, railFocus });
		}
	}
}

const WIDE_STATES = STATES.filter((s) => s.narrow === false);
const NARROW_STATES = STATES.filter((s) => s.narrow === true);

function describeState(state) {
	return `specOpen=${state.specOpen} narrow=${state.narrow} railFocus=${state.railFocus}`;
}

// The panes the layout reports as visible, whichever way the module chooses to
// say it: the `visible` list and the per-pane flags must agree, so both are
// read and cross-checked.
// Arrays coming back from the module are built inside the node:vm realm, so
// they are normalised to plain host arrays before any deep comparison.
function visiblePanes(layout) {
	const fromList = Array.from(layout.visible).sort();
	const fromFlags = Array.from(PANES)
		.filter((pane) => layout.panes[pane].visible)
		.sort();
	assert.deepEqual(
		fromList,
		fromFlags,
		"la lista dei riquadri visibili e i flag per riquadro devono dire la stessa cosa",
	);
	return fromFlags;
}

describe("resolveLayout — enumerazione di tutti gli stati", () => {
	it("copre lo spazio degli stati per intero", () => {
		// Eight states: the tests below are exhaustive only if this is.
		assert.equal(STATES.length, 2 * 2 * RAIL_FOCUS_VALUES.length);
		assert.ok(WIDE_STATES.length > 0);
		assert.ok(NARROW_STATES.length > 0);
	});

	it("nessuno stato lascia la finestra vuota", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			assert.ok(
				visiblePanes(layout).length >= 1,
				`nessun riquadro visibile con ${describeState(state)}`,
			);
		}
	});

	it("dichiara ogni riquadro visibile fra quelli che esistono in quello stato", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			for (const pane of visiblePanes(layout)) {
				assert.ok(
					layout.present.includes(pane),
					`il riquadro ${pane} è visibile ma non risulta presente con ${describeState(state)}`,
				);
			}
		}
	});
});

describe("finestra larga — la conversazione resta accanto al lavoro", () => {
	it("mostra sempre la rail insieme alla colonna primaria", () => {
		for (const state of WIDE_STATES) {
			const layout = resolveLayout(state);
			const visible = visiblePanes(layout);
			assert.ok(
				visible.includes("rail"),
				`la rail non è visibile con ${describeState(state)}`,
			);
			// AC-1: aprire una spec non nasconde la conversazione.
			// AC-2: la rail non dipende dallo stato della colonna primaria.
			const primary = state.specOpen ? "spec" : "board";
			assert.ok(
				visible.includes(primary),
				`la colonna primaria (${primary}) non è visibile con ${describeState(state)}`,
			);
			assert.deepEqual(
				visible,
				[primary, "rail"].sort(),
				`in finestra larga devono essere visibili esattamente la rail e ${primary} — ${describeState(state)}`,
			);
		}
	});

	it("tiene la rail visibile qualunque sia il valore di railFocus", () => {
		// La rail non è un riquadro che si conquista il posto: in finestra
		// larga c'è, e railFocus non ha voce in capitolo.
		for (const specOpen of [false, true]) {
			const withFocus = visiblePanes(
				resolveLayout({ specOpen, narrow: false, railFocus: true }),
			);
			const withoutFocus = visiblePanes(
				resolveLayout({ specOpen, narrow: false, railFocus: false }),
			);
			assert.deepEqual(withFocus, withoutFocus);
		}
	});
});

describe("finestra stretta — un contenuto alla volta, e nulla di irraggiungibile", () => {
	it("mostra esattamente un riquadro", () => {
		for (const state of NARROW_STATES) {
			const layout = resolveLayout(state);
			assert.equal(
				visiblePanes(layout).length,
				1,
				`in finestra stretta deve esserci un solo riquadro visibile — ${describeState(state)}`,
			);
		}
	});

	it("raggiunge con i commutatori ogni riquadro esistente e non visibile", () => {
		for (const state of NARROW_STATES) {
			const layout = resolveLayout(state);
			const visible = visiblePanes(layout);
			const reachable = new Set(visible);
			for (const switcher of layout.switchers) {
				assert.ok(
					PANES.includes(switcher.target),
					`commutatore verso un riquadro sconosciuto (${switcher.target}) con ${describeState(state)}`,
				);
				reachable.add(switcher.target);
			}
			for (const pane of layout.present) {
				assert.ok(
					reachable.has(pane),
					`il riquadro ${pane} esiste ma non è né visibile né raggiungibile con ${describeState(state)}`,
				);
			}
		}
	});

	it("non offre commutatori verso il riquadro già in vista", () => {
		for (const state of NARROW_STATES) {
			const layout = resolveLayout(state);
			const visible = visiblePanes(layout);
			for (const switcher of layout.switchers) {
				assert.ok(
					!visible.includes(switcher.target),
					`commutatore ridondante verso ${switcher.target} con ${describeState(state)}`,
				);
			}
		}
	});
});

describe("ritorno al lavoro", () => {
	it("offre il controllo di ritorno alla board quando una spec è aperta", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			if (state.specOpen) {
				assert.ok(
					layout.back,
					`manca il controllo di ritorno con ${describeState(state)}`,
				);
				assert.equal(layout.back.target, "board");
				assert.equal(layout.back.specOpen, false);
			} else {
				assert.equal(
					layout.back,
					null,
					`controllo di ritorno offerto senza spec aperta — ${describeState(state)}`,
				);
			}
		}
	});
});

describe("punto di rottura", () => {
	it("è dichiarato una volta sola: app.css usa il numero esportato dal modulo", () => {
		const css = readFileSync(cssPath, "utf8");
		// La media query della shell è quella che ridefinisce .workspace-shell.
		const shellQuery = css.match(
			/@media\s*\(max-width:\s*(\d+)px\)\s*\{[^@]*?\.workspace-shell\b/,
		);
		assert.ok(
			shellQuery,
			"app.css non contiene una media query che ridefinisce .workspace-shell",
		);
		assert.equal(
			Number(shellQuery[1]),
			NARROW_MAX_WIDTH,
			"il punto di rottura di app.css e quello di workspace-layout.js devono essere lo stesso numero",
		);
	});
});
