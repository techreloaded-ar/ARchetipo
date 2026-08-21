// workspace-layout.js
// Pure decision module for the workspace shell layout.
//
// It answers one question: given the current state, which panes of the shell
// are visible, which control returns to the work, and which switchers reach
// the panes that are not visible.
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
// (exports resolveLayout, NARROW_MAX_WIDTH, PANES, RAIL_FOCUS_VALUES).

(function () {
	"use strict";

	// The shell switches to one-pane-at-a-time at this width. It matches the
	// existing 900px media query, where the topbar already collapses and the
	// board tightens: below it there is no room for two columns.
	const NARROW_MAX_WIDTH = 900;

	// Every pane the shell can own. Order is the reading order of the shell:
	// primary column first (board or spec detail), lateral rail last.
	const PANES = ["board", "spec", "rail"];

	// Every value railFocus may take. The test enumerates from here.
	const RAIL_FOCUS_VALUES = [false, true];

	const PANE_META = {
		board: { className: "shell-pane-board", label: "Board" },
		spec: { className: "shell-pane-spec", label: "Spec" },
		// The rail holds what is in flight *and* the conversation, so it is named
		// after neither: "Activity" is what a person reaches when they press it.
		rail: { className: "shell-pane-rail", label: "Activity" },
	};

	const SHELL_CLASS_WIDE = "workspace-shell--wide";
	const SHELL_CLASS_NARROW = "workspace-shell--narrow";
	const PANE_VISIBLE_CLASS = "is-visible";
	const PANE_HIDDEN_CLASS = "is-hidden";

	// A pane exists in a state only when it has something to show. The spec
	// pane exists only while a spec is open; the board is always there behind
	// it, and the rail is permanent.
	function presentPanes(specOpen) {
		return specOpen ? ["board", "spec", "rail"] : ["board", "rail"];
	}

	// Which single pane the narrow shell shows: the rail when it has focus,
	// otherwise the primary column — the open spec, or the board.
	function narrowVisiblePane(specOpen, railFocus) {
		if (railFocus) return "rail";
		return specOpen ? "spec" : "board";
	}

	// Which panes the wide shell shows: the rail is always there, next to the
	// primary column, whatever the primary column happens to be showing.
	function wideVisiblePanes(specOpen) {
		return [specOpen ? "spec" : "board", "rail"];
	}

	function switcherFor(pane, specOpen) {
		return {
			target: pane,
			className: PANE_META[pane].className,
			label: PANE_META[pane].label,
			// The state that reaching this pane implies, so the caller has
			// nothing left to infer: returning to the board closes the spec.
			specOpen: pane === "spec" ? true : pane === "board" ? false : specOpen,
			railFocus: pane === "rail",
		};
	}

	function resolveLayout(state) {
		const value = state && typeof state === "object" ? state : {};
		const specOpen = value.specOpen === true;
		const narrow = value.narrow === true;
		const railFocus = value.railFocus === true;

		const present = presentPanes(specOpen);
		const visible = narrow
			? [narrowVisiblePane(specOpen, railFocus)]
			: wideVisiblePanes(specOpen);

		// In narrow mode every pane that exists and is not on screen must be
		// one tap away, or it would be unreachable. In wide mode the rail and
		// the primary column are both on screen and nothing needs switching:
		// the board is reached by the return control instead.
		const switchers = narrow
			? present
					.filter((pane) => visible.indexOf(pane) === -1)
					.map((pane) => switcherFor(pane, specOpen))
			: [];

		// The return control leaves the spec detail and puts the board back in
		// the primary column. It is shown whenever a spec is open, in both
		// modes, because in both modes the board is the thing behind it.
		const back = specOpen ? switcherFor("board", specOpen) : null;

		const panes = {};
		PANES.forEach(function (pane) {
			const isVisible = visible.indexOf(pane) !== -1;
			panes[pane] = {
				present: present.indexOf(pane) !== -1,
				visible: isVisible,
				className: PANE_META[pane].className,
				stateClass: isVisible ? PANE_VISIBLE_CLASS : PANE_HIDDEN_CLASS,
			};
		});

		return {
			narrow: narrow,
			specOpen: specOpen,
			railFocus: narrow ? railFocus : false,
			shellClass: narrow ? SHELL_CLASS_NARROW : SHELL_CLASS_WIDE,
			present: present,
			visible: visible,
			panes: panes,
			back: back,
			switchers: switchers,
		};
	}

	const api = {
		NARROW_MAX_WIDTH,
		PANES,
		RAIL_FOCUS_VALUES,
		SHELL_CLASS_WIDE,
		SHELL_CLASS_NARROW,
		PANE_VISIBLE_CLASS,
		PANE_HIDDEN_CLASS,
		resolveLayout,
	};

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = api;
	} else {
		window.WorkspaceLayout = api;
	}
})();
