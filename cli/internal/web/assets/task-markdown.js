// task-markdown.js
// Pure helper for rendering task markdown in the read-only plan view.
// Converts GFM task list items (- [ ] / - [x]) to plain bullets so they
// render as a normal unordered list instead of an interactive checklist.
// All other markdown constructs (headings, code, tables, etc.) pass through unchanged.
//
// Consumable in both browser (defines window.TaskMarkdown) and Node
// (exports renderTaskMarkdown / normalizeTaskChecklist via module.exports).
(function () {
	// ---- internal helpers ----

	/**
	 * Replace GFM task list markers at the beginning of list items with
	 * plain bullets. Handles both `- [ ]` and `* [ ]` styles (checked or
	 * unchecked). Preserves indentation and the trailing space.
	 *
	 *   "- [ ] Do thing"  →  "- Do thing"
	 *   "  * [x] Done"    →  "  * Done"
	 *
	 * Does NOT alter checkboxes inside inline text or inside code spans /
	 * fenced code blocks — the regex only matches at line start after
	 * optional leading whitespace.
	 */
	function normalizeTaskChecklist(markdownText) {
		if (typeof markdownText !== "string") return "";
		return markdownText.replace(/^(\s*)([-*]) \[[ x]\] /gm, "$1$2 ");
	}

	// ---- public API ----

	/**
	 * Render task markdown to HTML with checklist items converted to
	 * plain bullet lists.
	 *
	 * @param {string} markdownText  The raw markdown source of the task body.
	 * @param {Function} [markedParse]  Optional marked.parse function; if
	 *   omitted, the browser global `marked.parse` is used.
	 * @returns {string} HTML string.
	 */
	function renderTaskMarkdown(markdownText, markedParse) {
		const parse =
			typeof markedParse === "function"
				? markedParse
				: typeof marked !== "undefined" && typeof marked.parse === "function"
					? marked.parse.bind(marked)
					: null;

		if (!parse) {
			throw new Error(
				"renderTaskMarkdown: marked.parse is not available. " +
					"Pass it as the second argument or ensure the global `marked` library is loaded.",
			);
		}

		if (typeof markdownText !== "string" || !markdownText.trim()) {
			return "";
		}

		return parse(normalizeTaskChecklist(markdownText));
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderTaskMarkdown, normalizeTaskChecklist };
	} else {
		window.TaskMarkdown = { renderTaskMarkdown, normalizeTaskChecklist };
	}
})();
