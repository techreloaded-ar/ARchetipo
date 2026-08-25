// test/web/workspace-layout.test.mjs
// Tests for the pure workspace-layout decision module used by the ARchetipo
// web viewer.
// Run: node --test test/web/workspace-layout.test.mjs
//
// The oracles here are *invariants over every state*, not examples: the module
// takes a view plus two booleans, so the whole input space is twelve states and
// the tests enumerate it rather than sampling it. What the person sees on
// screen is decided entirely by this function, so a rule that holds on a chosen
// example but breaks on a forgotten combination is a bug nobody would notice.
//
// Verifies:
//   - AC-1 the conversation is the home: an empty state, and every view that is
//     not admissible, resolve to the conversation, in both widths
//   - AC-2 every view that exists and is not the current one is reachable with
//     exactly one switcher — one command, one button
//   - AC-4 in a wide window the primary column shows exactly one view and
//     nothing is laid over anything
//   - AC-6 in a narrow window the conversation is always on screen and the
//     called view is an overlay; and a choice made inside an overlay closes it
//   - the thread rail is the companion of the conversation: it is on screen
//     exactly when the conversation is the view, in both widths
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

const {
	resolveLayout,
	nextViewAfterSelection,
	NARROW_MAX_WIDTH,
	PANES,
	VIEWS,
	DEFAULT_VIEW,
} = loadWorkspaceLayout();

// The whole input space: every view the module declares, times the two
// booleans. Nothing is chosen, everything is enumerated.
const STATES = [];
for (const view of VIEWS) {
	for (const specOpen of [false, true]) {
		for (const narrow of [false, true]) {
			STATES.push({ view, specOpen, narrow });
		}
	}
}

const WIDE_STATES = STATES.filter((s) => s.narrow === false);
const NARROW_STATES = STATES.filter((s) => s.narrow === true);

function describeState(state) {
	return `view=${state.view} specOpen=${state.specOpen} narrow=${state.narrow}`;
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

function overlaidPanes(layout) {
	return Array.from(PANES).filter((pane) => layout.panes[pane].overlay === true);
}

describe("resolveLayout — enumerazione di tutti gli stati", () => {
	it("copre lo spazio degli stati per intero", () => {
		// Twelve states: the tests below are exhaustive only if this is.
		assert.equal(STATES.length, VIEWS.length * 2 * 2);
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

describe("la casa del workspace", () => {
	it("apre sulla conversazione quando nessuna vista è stata chiesta", () => {
		// AC-1: lo stato vuoto è quello della pagina appena caricata.
		for (const narrow of [false, true]) {
			for (const state of [
				{ narrow },
				{ view: undefined, narrow },
				{ view: "sconosciuta", narrow },
				{ view: "spec", specOpen: false, narrow },
			]) {
				const layout = resolveLayout(state);
				assert.equal(
					layout.view,
					DEFAULT_VIEW,
					`la vista di casa deve essere ${DEFAULT_VIEW} con ${JSON.stringify(state)}`,
				);
				assert.equal(DEFAULT_VIEW, "conversation");
				assert.ok(
					visiblePanes(layout).includes("conversation"),
					`la conversazione non è visibile con ${JSON.stringify(state)}`,
				);
			}
		}
	});

	it("risponde alla casa anche quando resolveLayout è chiamata senza stato", () => {
		const layout = resolveLayout();
		assert.equal(layout.view, DEFAULT_VIEW);
		assert.deepEqual(Array.from(layout.visible), ["conversation"]);
	});
});

describe("finestra larga — una vista alla volta, a piena larghezza", () => {
	it("mostra esattamente la vista corrente e nient'altro", () => {
		for (const state of WIDE_STATES) {
			const layout = resolveLayout(state);
			const visible = visiblePanes(layout);
			assert.equal(
				visible.length,
				1,
				`in finestra larga deve essere visibile un solo riquadro — ${describeState(state)}, trovati ${visible.join(", ")}`,
			);
			assert.deepEqual(
				visible,
				[layout.view],
				`la colonna primaria deve mostrare la vista corrente (${layout.view}) — ${describeState(state)}`,
			);
		}
	});

	it("non sovrappone nulla a nulla", () => {
		// AC-4: il dettaglio spec occupa la colonna, non ci si sovrappone.
		for (const state of WIDE_STATES) {
			const layout = resolveLayout(state);
			assert.deepEqual(
				overlaidPanes(layout),
				[],
				`in finestra larga nessun riquadro deve essere una sovrapposizione — ${describeState(state)}`,
			);
		}
	});
});

describe("finestra stretta — la conversazione resta, le altre viste si sovrappongono", () => {
	it("tiene la conversazione visibile in ogni stato stretto", () => {
		for (const state of NARROW_STATES) {
			const layout = resolveLayout(state);
			assert.ok(
				layout.panes.conversation.visible,
				`la conversazione non è visibile con ${describeState(state)}`,
			);
			assert.equal(
				layout.panes.conversation.overlay,
				false,
				`la conversazione non deve mai essere una sovrapposizione — ${describeState(state)}`,
			);
		}
	});

	it("disegna la vista chiamata come sovrapposizione, mai al posto della conversazione", () => {
		for (const state of NARROW_STATES) {
			const layout = resolveLayout(state);
			if (layout.view === "conversation") {
				assert.deepEqual(
					overlaidPanes(layout),
					[],
					`sulla casa non deve esserci alcuna sovrapposizione — ${describeState(state)}`,
				);
				continue;
			}
			assert.ok(
				layout.panes[layout.view].visible,
				`la vista chiamata (${layout.view}) non è visibile con ${describeState(state)}`,
			);
			assert.deepEqual(
				overlaidPanes(layout),
				[layout.view],
				`in finestra stretta la sola sovrapposizione deve essere la vista corrente — ${describeState(state)}`,
			);
		}
	});
});

describe("nulla di irraggiungibile", () => {
	it("raggiunge con un commutatore ogni vista esistente e non corrente", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			const reachable = new Set([layout.view]);
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
					`il riquadro ${pane} esiste ma non è né la vista corrente né raggiungibile con ${describeState(state)}`,
				);
			}
		}
	});

	it("non offre commutatori verso la vista già corrente", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			for (const switcher of layout.switchers) {
				assert.notEqual(
					switcher.target,
					layout.view,
					`commutatore ridondante verso la vista corrente ${layout.view} — ${describeState(state)}`,
				);
			}
		}
	});

	it("offre un solo bottone per bersaglio: un solo comando è anche un solo bottone", () => {
		// AC-2.
		for (const state of STATES) {
			const layout = resolveLayout(state);
			const targets = layout.switchers.map((s) => s.target);
			assert.equal(
				new Set(targets).size,
				targets.length,
				`bersagli duplicati fra i commutatori (${targets.join(", ")}) con ${describeState(state)}`,
			);
		}
	});

	it("raggiunge la board con esattamente un comando quando non è la vista corrente", () => {
		// AC-2, alla lettera.
		for (const state of STATES) {
			const layout = resolveLayout(state);
			if (layout.view === "board") continue;
			const toBoard = layout.switchers.filter((s) => s.target === "board");
			assert.equal(
				toBoard.length,
				1,
				`la board deve essere a un solo comando — ${describeState(state)}, trovati ${toBoard.length}`,
			);
			assert.equal(toBoard[0].view, "board");
		}
	});
});

describe("il rail dei thread accompagna la conversazione", () => {
	it("è sullo schermo solo quando la vista è la conversazione", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			assert.equal(
				layout.rail.visible,
				layout.view === "conversation",
				`il rail delle conversazioni non segue la vista — ${describeState(state)}`,
			);
		}
	});

	it("dichiara la classe di stato coerente con la sua visibilità", () => {
		for (const state of STATES) {
			const layout = resolveLayout(state);
			assert.equal(
				layout.rail.stateClass,
				layout.rail.visible ? "is-visible" : "is-hidden",
				`la classe del rail contraddice la sua visibilità — ${describeState(state)}`,
			);
		}
	});

	it("non toglie larghezza alla board in nessuna delle due misure", () => {
		// Il motivo per cui la regola esiste: la board è la vista che ha più
		// bisogno di colonne, e il rail accanto a lei indicizzava altro.
		for (const narrow of [false, true]) {
			const layout = resolveLayout({ view: "board", specOpen: false, narrow });
			assert.equal(layout.rail.visible, false);
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

describe("la scelta richiude la sovrapposizione", () => {
	it("scegliere una card dentro la board porta al dettaglio spec", () => {
		const from = { view: "board", specOpen: false, narrow: true };
		const next = nextViewAfterSelection(from, "spec");
		assert.equal(next.view, "spec");
		assert.equal(next.specOpen, true);
		const layout = resolveLayout(next);
		assert.equal(
			layout.panes.board.visible,
			false,
			"dopo la scelta la board non deve più essere sullo schermo",
		);
		assert.equal(layout.panes.spec.overlay, true);
	});

	it("lasciare il dettaglio riporta alla board e lo chiude", () => {
		const next = nextViewAfterSelection(
			{ view: "spec", specOpen: true, narrow: true },
			"board",
		);
		assert.equal(next.view, "board");
		assert.equal(next.specOpen, false);
		assert.equal(resolveLayout(next).panes.spec.present, false);
	});

	it("tornare alla casa non chiude una spec aperta", () => {
		for (const specOpen of [false, true]) {
			const next = nextViewAfterSelection(
				{ view: "board", specOpen, narrow: true },
				"conversation",
			);
			assert.equal(next.view, "conversation");
			assert.equal(
				next.specOpen,
				specOpen,
				"andare alla conversazione non deve cambiare lo stato di apertura della spec",
			);
		}
	});

	it("una selezione sconosciuta lascia lo stato com'era", () => {
		for (const state of STATES) {
			const next = nextViewAfterSelection(state, "qualcos-altro");
			assert.deepEqual(
				{
					view: resolveLayout(next).view,
					visible: Array.from(resolveLayout(next).visible).sort(),
				},
				{
					view: resolveLayout(state).view,
					visible: Array.from(resolveLayout(state).visible).sort(),
				},
				`una selezione sconosciuta ha cambiato il layout — ${describeState(state)}`,
			);
		}
	});

	it("nessuna selezione ammessa lascia due sovrapposizioni contemporanee", () => {
		// AC-6.
		for (const state of STATES) {
			for (const selection of ["conversation", "board", "spec"]) {
				const layout = resolveLayout(nextViewAfterSelection(state, selection));
				assert.ok(
					overlaidPanes(layout).length <= 1,
					`due sovrapposizioni dopo la selezione ${selection} da ${describeState(state)}`,
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
