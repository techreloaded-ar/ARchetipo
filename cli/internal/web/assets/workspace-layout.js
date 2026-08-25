// workspace-layout.js
// Pure decision module for the workspace shell layout.
//
// The shell has one primary column, and that column holds one view at a time:
// the conversation, the board, or the open spec detail. The conversation is the
// home of the workspace — an empty state, the one the page has just booted
// with, answers `conversation` — and the other views are called into the very
// same space. In a narrow window the conversation stays on screen at full width
// and the called view is laid over it, so nothing that was there is unmounted.
//
// It answers one question: given the current state, which panes of the shell
// are visible, which of them is an overlay, whether the thread rail beside them
// is on screen, which control returns to the work, and which switchers reach
// the views that are not on screen.
//
// It is pure: no DOM, no document, no fetch, and it never applies a class. It
// returns the class names; writing them onto elements belongs to the caller.
// It knows nothing about what the panes contain — only that they exist.
//
// The breakpoint is declared here once, and only here: app.css uses the same
// number in its media query, and the test reads it from this module, so CSS
// and JS cannot drift apart on two numbers written in two places.
//
// Consumable in both browser (defines window.WorkspaceLayout) and Node
// (exports resolveLayout, nextViewAfterSelection, NARROW_MAX_WIDTH, PANES,
// VIEWS, DEFAULT_VIEW).

(function () {
	"use strict";

	// The shell lays the called view over the conversation at this width. It
	// matches the existing 900px media query, where the topbar already collapses
	// and the board tightens: below it there is no room for two things side by
	// side.
	const NARROW_MAX_WIDTH = 900;

	// Every pane the primary column can own. Order is the reading order of the
	// shell: the home first, then the views that are called into its place.
	const PANES = ["conversation", "board", "spec"];

	// Every view the primary column can be showing. Same set as the panes,
	// because a view *is* the pane that occupies the column.
	const VIEWS = ["conversation", "board", "spec"];

	// Le linguette della barra in alto, e sono queste due e basta. Il dettaglio
	// spec non è una vista fra pari: è il dettaglio di una card della board, si
	// apre scegliendo la card e si chiude col comando di ritorno. Dargli una
	// linguetta faceva comparire e sparire una terza voce a seconda di ciò che
	// era aperto, e una barra che cambia numero di voci non è più una barra:
	// è un elenco di cose che capitano.
	const TABS = ["conversation", "board"];

	// The home of the workspace. An empty state — the page that has just booted
	// and asked for nothing — resolves to this, so opening a workspace lands on
	// the conversation by construction and not by the order in which the caller
	// happens to call its own functions.
	const DEFAULT_VIEW = "conversation";

	const PANE_META = {
		conversation: {
			className: "shell-pane-conversation",
			label: "Conversazione",
		},
		board: { className: "shell-pane-board", label: "Board" },
		spec: { className: "shell-pane-spec", label: "Spec" },
	};

	const SHELL_CLASS_WIDE = "workspace-shell--wide";
	const SHELL_CLASS_NARROW = "workspace-shell--narrow";
	const PANE_VISIBLE_CLASS = "is-visible";
	const PANE_HIDDEN_CLASS = "is-hidden";
	// Narrow mode only: the pane is drawn *over* the conversation instead of in
	// its place, so what is underneath stays mounted and intact.
	const PANE_OVERLAY_CLASS = "is-overlay";

	// A pane exists in a state only when it has something to show. The
	// conversation and the board are always there; the spec pane exists only
	// while a spec is open.
	function presentPanes(specOpen) {
		return specOpen
			? ["conversation", "board", "spec"]
			: ["conversation", "board"];
	}

	// A view is admissible only if it is known and, for the spec, only while a
	// spec is actually open. Everything else falls back to the home.
	function normalizeView(view, specOpen) {
		if (typeof view !== "string") return DEFAULT_VIEW;
		if (VIEWS.indexOf(view) === -1) return DEFAULT_VIEW;
		if (view === "spec" && !specOpen) return DEFAULT_VIEW;
		return view;
	}

	// La linguetta accesa nella barra, o la stringa vuota quando non ce n'è
	// nessuna: mentre il dettaglio spec occupa la colonna nessuna delle due è la
	// vista corrente, e restano entrambe premibili — la board perché chiude il
	// dettaglio e torna in colonna, la conversazione perché è la casa. Accendere
	// una linguetta che non è ciò che si sta guardando direbbe una cosa falsa.
	function currentTabFor(view) {
		return TABS.indexOf(view) !== -1 ? view : "";
	}

	function switcherFor(pane, specOpen) {
		return {
			target: pane,
			// The view pressing it produces. Same value as `target`: a switcher
			// reaches a pane by making it the view of the primary column.
			view: pane,
			className: PANE_META[pane].className,
			label: PANE_META[pane].label,
			// The state that reaching this view implies, so the caller has
			// nothing left to infer: leaving the detail for the board closes it,
			// while going home does not close a spec that is open.
			specOpen: pane === "spec" ? true : pane === "board" ? false : specOpen,
		};
	}

	function resolveLayout(state) {
		const value = state && typeof state === "object" ? state : {};
		const specOpen = value.specOpen === true;
		const narrow = value.narrow === true;
		const view = normalizeView(value.view, specOpen);
		const currentTab = currentTabFor(view);

		const present = presentPanes(specOpen);

		// Wide: the primary column shows exactly one view, at full width, and the
		// others stay mounted behind it. Narrow: the conversation never leaves,
		// and the called view is laid over it.
		const overlaid = narrow && view !== "conversation" ? view : null;
		const visible = narrow
			? overlaid
				? ["conversation", overlaid]
				: ["conversation"]
			: [view];

		// Ogni linguetta che non è quella corrente è a un solo comando, in
		// entrambe le larghezze: la barra è permanente, non un ripiego per la
		// finestra stretta. Le linguette sono sempre le stesse due, così la
		// barra non cambia forma sotto le dita mentre si lavora.
		const switchers = TABS.filter(
			(pane) => present.indexOf(pane) !== -1 && pane !== view,
		).map((pane) => switcherFor(pane, specOpen));

		// The return control leaves the spec detail and puts the board back in
		// the primary column. It is offered whenever a spec is open.
		const back = specOpen ? switcherFor("board", specOpen) : null;

		// The rail of past conversations belongs to the conversation, not to the
		// shell that holds it: it is the index of what has been said in this
		// workspace, and it says nothing about a board or about a spec detail.
		// So it is on screen exactly when the conversation is the view being
		// read — in both widths — and the permanent switcher in the topbar stays
		// the way back to it. Beside the board it was a column of links to
		// somewhere else, taking width from the one thing the board needs.
		const railVisible = view === "conversation";

		const panes = {};
		PANES.forEach(function (pane) {
			const isVisible = visible.indexOf(pane) !== -1;
			panes[pane] = {
				present: present.indexOf(pane) !== -1,
				visible: isVisible,
				overlay: pane === overlaid,
				className: PANE_META[pane].className,
				// The word for this view, so the caller never invents one.
				label: PANE_META[pane].label,
				stateClass: isVisible ? PANE_VISIBLE_CLASS : PANE_HIDDEN_CLASS,
			};
		});

		return {
			view: view,
			// La linguetta accesa nella barra: la vista corrente quando è una
			// delle due, la stringa vuota mentre in colonna c'è il dettaglio
			// spec, che linguetta non ne ha.
			currentTab: currentTab,
			narrow: narrow,
			specOpen: specOpen,
			shellClass: narrow ? SHELL_CLASS_NARROW : SHELL_CLASS_WIDE,
			present: present,
			visible: visible,
			panes: panes,
			// The thread rail is not a pane of the primary column — it is the
			// companion of one view — so it gets its own answer rather than an
			// entry in `panes`, and the caller applies the same two state classes
			// to it that it applies to a pane.
			rail: {
				visible: railVisible,
				stateClass: railVisible ? PANE_VISIBLE_CLASS : PANE_HIDDEN_CLASS,
			},
			back: back,
			switchers: switchers,
		};
	}

	// nextViewAfterSelection is where — and only where — the rule of AC-6 lives:
	// an overlay does not survive a choice made inside it, because the choice
	// changes the view. Choosing a card inside the board leads to the spec;
	// leaving the detail leads back to the board; asking for the home leads to
	// the conversation without closing an open spec.
	function nextViewAfterSelection(state, selection) {
		const value = state && typeof state === "object" ? state : {};
		const specOpen = value.specOpen === true;
		const narrow = value.narrow === true;
		const view = normalizeView(value.view, specOpen);

		if (selection === "spec") {
			return { view: "spec", specOpen: true, narrow: narrow };
		}
		if (selection === "board") {
			return { view: "board", specOpen: false, narrow: narrow };
		}
		if (selection === "conversation") {
			return { view: "conversation", specOpen: specOpen, narrow: narrow };
		}
		// An unknown selection changes nothing.
		return { view: view, specOpen: specOpen, narrow: narrow };
	}

	const api = {
		NARROW_MAX_WIDTH,
		PANES,
		VIEWS,
		TABS,
		DEFAULT_VIEW,
		SHELL_CLASS_WIDE,
		SHELL_CLASS_NARROW,
		PANE_VISIBLE_CLASS,
		PANE_HIDDEN_CLASS,
		PANE_OVERLAY_CLASS,
		resolveLayout,
		nextViewAfterSelection,
	};

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = api;
	} else {
		window.WorkspaceLayout = api;
	}
})();
