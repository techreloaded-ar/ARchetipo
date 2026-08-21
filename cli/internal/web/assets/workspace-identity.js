// workspace-identity.js
// Pure resolver for one question only: which workspace is this page serving,
// and what is it called?
//
// It answers from the GET /api/workspaces payload and returns everything the
// page needs to say it — the label in the topbar, the full path to consult,
// the browser tab title — so those three can never contradict each other.
//
// It is pure: no DOM, no document, no fetch. And it returns plain strings,
// never HTML: the caller writes them with textContent and with the title
// attribute, so there is no escaping to get wrong on a name that comes from
// the user's own disk. That is a deliberate difference from workspace-home.js,
// which produces markup and therefore carries its own escapeHtml.
//
// Consumable in both browser (defines window.WorkspaceIdentity) and Node
// (exports resolveWorkspaceIdentity and the text constants).
(function () {
	// ---- text, declared once ----

	const TITLE_SUFFIX = " · ARchetipo";
	const EMPTY_LABEL = "No workspace open";
	const EMPTY_TITLE = "No workspace" + TITLE_SUFFIX;

	// ---- public API ----

	/**
	 * Resolve the identity of the open workspace from the workspace list payload.
	 *
	 * Never throws on a partial payload: null, {}, and a payload that claims to
	 * be open without naming anything are all answers, and each resolves to the
	 * state that declares the absence.
	 *
	 * @param {object|null} view  The /api/workspaces payload.
	 * @returns {{open: boolean, name: string, path: string, label: string,
	 *           tooltip: string, documentTitle: string, actionable: boolean}}
	 */
	function resolveWorkspaceIdentity(view) {
		const value = view && typeof view === "object" ? view : {};
		const name = typeof value.currentName === "string" ? value.currentName.trim() : "";
		const path = typeof value.currentPath === "string" ? value.currentPath.trim() : "";
		// A payload that declares itself open but cannot say which workspace it
		// serves is treated as closed: a page that does not know what it is
		// looking at must say so, not display an empty name as if it were one.
		const open = value.open === true && name !== "";

		if (open) {
			return {
				open: true,
				name: name,
				path: path,
				label: name,
				tooltip: path,
				documentTitle: name + TITLE_SUFFIX,
				actionable: true,
			};
		}
		return {
			open: false,
			name: "",
			path: "",
			label: EMPTY_LABEL,
			tooltip: "",
			documentTitle: EMPTY_TITLE,
			actionable: false,
		};
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { resolveWorkspaceIdentity, EMPTY_LABEL, EMPTY_TITLE, TITLE_SUFFIX };
	} else {
		window.WorkspaceIdentity = { resolveWorkspaceIdentity, EMPTY_LABEL, EMPTY_TITLE, TITLE_SUFFIX };
	}
})();
