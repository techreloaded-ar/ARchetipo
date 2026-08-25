// workspace-runs.js
// Pure renderer for the GET /api/workspace/runs payload.
//
// The module contains no process rules: not one action id, not one status
// name, not one ordering of the states. Every word it draws about what is in
// flight comes from the payload, so an action or a status it has never seen is
// rendered as it is, unchanged. Changing what a workspace can run therefore
// changes what this module draws without a line changing here.
//
// It does not navigate and it does not decide. Every row carries the execution
// id, the scope and the spec code as data attributes, and where a press should
// lead is the caller's judgement, taken from those attributes.
//
// It is pure: no DOM, no fetch, no document, no timers. It takes a view object
// and returns an HTML string. Wiring that string into the page belongs to the
// caller.
//
// Consumable in both browser (defines window.WorkspaceRuns) and Node
// (exports renderWorkspaceRuns / awaitingCount / escapeHtml).
(function () {
	// ---- internal helpers ----

	/**
	 * Escape the five characters that would otherwise let payload text break
	 * out of the markup it is interpolated into. Behaviourally identical to
	 * the escapeHtml of app.js and of the sibling modules.
	 */
	function escapeHtml(s) {
		if (s === null || s === undefined) return "";
		return String(s)
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;")
			.replace(/'/g, "&#39;");
	}

	/** Return the array at key, or an empty array: partial payloads must not throw. */
	function arrayAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return Array.isArray(found) ? found : [];
	}

	/** Return the object at key, or null: partial payloads must not throw. */
	function objectAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return found && typeof found === "object" ? found : null;
	}

	/** Return the string at key, or the empty string. Never a placeholder. */
	function stringAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		if (typeof found === "string") return found;
		if (typeof found === "number") return String(found);
		return "";
	}

	// The only words this module owns. None of them names a step, a state or an
	// action of anybody's method: they are the furniture of a list — what the
	// list is, and what it says when it is empty. A caller that wants other
	// wording passes it in.
	const DEFAULT_TEXT = {
		title: "In corso",
		empty: "In questo workspace non sta girando niente.",
		awaiting: "aspetta te",
	};

	/** How many targets the closed summary names before it counts the rest. */
	const DIGEST_MAX = 3;

	function textOption(opts, key) {
		const given = opts && typeof opts === "object" ? opts[key] : null;
		return typeof given === "string" && given.trim() ? given : DEFAULT_TEXT[key];
	}

	/**
	 * True when the entry declares it is waiting for a human answer. The flag is
	 * read, never derived: whether a run is blocked on a decision is a server
	 * fact this module is in no position to recompute.
	 */
	function isAwaiting(entry) {
		return !!(entry && typeof entry === "object" && entry.awaiting_response);
	}

	/**
	 * The readable target of a row: the spec code when the payload carries one,
	 * otherwise the scope word the payload itself declares. No word is written
	 * here for either case.
	 */
	function renderTarget(entry) {
		const code = stringAt(entry, "spec_code");
		if (code) {
			return `<code class="ws-run-code">${escapeHtml(code)}</code>`;
		}
		const scope = stringAt(entry, "scope");
		if (scope) {
			return `<span class="ws-run-scope">${escapeHtml(scope)}</span>`;
		}
		return "";
	}

	/**
	 * The waiting mark, present exactly for the entries the payload marks. It
	 * names the pending decision in the payload's own words — its title, or its
	 * id when there is no title — because a person must be able to tell one
	 * waiting run from another without opening either.
	 */
	function renderAwaiting(entry, opts) {
		if (!isAwaiting(entry)) return "";
		const pending = objectAt(entry, "pending");
		const named = pending
			? stringAt(pending, "title") || stringAt(pending, "id")
			: "";
		const body = named
			? `<span class="ws-run-await-body">${escapeHtml(named)}</span>`
			: "";
		// role="status" e non role="note": questa non è una postilla, è la cosa
		// che aspetta una persona. Perché l'annuncio non si ripeta a ogni
		// passata di polling, chi monta il pannello ridisegna solo quando il
		// markup cambia davvero (renderWorkspaceRunsPanel in app.js).
		return `<div class="ws-run-await" role="status">
			<span class="ws-run-await-mark">${escapeHtml(textOption(opts, "awaiting"))}</span>
			${body}
		</div>`;
	}

	/**
	 * The server's own explanation of why an entry could not be resolved. It is
	 * shown as it is: an entry whose waiting is unknown must not read as an
	 * entry that is confidently not waiting.
	 */
	function renderNotice(entry) {
		const notice = stringAt(entry, "notice");
		if (!notice) return "";
		return `<div class="ws-run-notice">${escapeHtml(notice)}</div>`;
	}

	/**
	 * The one-line digest read on the closed summary: the very targets the rows
	 * carry, in the very order they carry them. Nothing is named here that the
	 * open list does not repeat, and past DIGEST_MAX the remainder is counted
	 * rather than truncated in silence.
	 *
	 * With nothing in flight the digest is the sentence that says so, because a
	 * closed summary is the only thing on screen and the absence must still be
	 * declared.
	 */
	function renderDigest(entries, opts) {
		if (!entries.length) {
			return `<span class="ws-runs-digest">${escapeHtml(textOption(opts, "empty"))}</span>`;
		}
		const names = entries
			.map((entry) => stringAt(entry, "spec_code") || stringAt(entry, "scope"))
			.filter(Boolean);
		if (!names.length) return "";
		const shown = names.slice(0, DIGEST_MAX);
		const rest = names.length - shown.length;
		const text = shown.join(" \u00b7 ") + (rest > 0 ? ` +${rest}` : "");
		return `<span class="ws-runs-digest">${escapeHtml(text)}</span>`;
	}

	function renderRow(entry, opts) {
		if (!entry || typeof entry !== "object") return "";
		const id = stringAt(entry, "id");
		const scope = stringAt(entry, "scope");
		const code = stringAt(entry, "spec_code");
		const attrs =
			` data-run-id="${escapeHtml(id)}"` +
			` data-run-scope="${escapeHtml(scope)}"` +
			` data-run-spec="${escapeHtml(code)}"`;
		const waiting = isAwaiting(entry) ? " is-awaiting" : "";
		const target = renderTarget(entry);
		const action = stringAt(entry, "action");
		const status = stringAt(entry, "status");
		const head = [target];
		if (action) {
			head.push(`<span class="ws-run-action">${escapeHtml(action)}</span>`);
		}
		if (status) {
			head.push(`<span class="ws-run-status">${escapeHtml(status)}</span>`);
		}
		return `<li class="ws-run${waiting}"${attrs}>
			<div class="ws-run-head">${head.filter(Boolean).join("")}</div>
			${renderAwaiting(entry, opts)}
			${renderNotice(entry)}
		</li>`;
	}

	// ---- public API ----

	/**
	 * How many listed entries are waiting for a human answer.
	 *
	 * It lives here, and not in the caller, because it is a property of the
	 * payload and not of the page: the indicator that must stay visible whatever
	 * is on screen reads this number, and it is testable without a DOM.
	 *
	 * @param {object|null} view  The /api/workspace/runs payload.
	 * @returns {number}
	 */
	function awaitingCount(view) {
		return arrayAt(view, "runs").filter(isAwaiting).length;
	}

	/**
	 * Render the workspace runs view to an HTML string.
	 *
	 * Never throws on a partial payload: a null view, a missing runs array and
	 * an entry without action or status are all answers, and each has a
	 * rendering. An empty list is a rendered state, not an empty string — the
	 * rail says that nothing is in flight rather than silently disappearing.
	 *
	 * The panel is a <details>: closed it is one line — what the list is, what
	 * it is working on, and how many entries are waiting — and open it is the
	 * full list. Whether it is open is not this module's memory: the caller
	 * passes it in, because it is a choice a person made and the strip is
	 * redrawn from scratch on every poll.
	 *
	 * @param {object|null} view  The /api/workspace/runs payload.
	 * @param {{title?: string, empty?: string, awaiting?: string, expanded?: boolean}} [opts]
	 *        Wording overrides for the furniture of the list, and whether the
	 *        panel is drawn open.
	 * @returns {string} HTML string.
	 */
	function renderWorkspaceRuns(view, opts) {
		const entries = arrayAt(view, "runs").filter(
			(entry) => entry && typeof entry === "object",
		);
		const waiting = entries.filter(isAwaiting).length;
		const badge = waiting
			? `<span class="ws-runs-badge is-awaiting">${escapeHtml(String(waiting))}</span>`
			: "";
		const expanded = !!(opts && typeof opts === "object" && opts.expanded);
		const head = `<summary class="ws-runs-head">
			<span class="ws-runs-title">${escapeHtml(textOption(opts, "title"))}</span>
			${renderDigest(entries, opts)}
			<span class="ws-runs-head-spacer"></span>
			${badge}
		</summary>`;
		const rows = entries.length
			? entries.map((entry) => renderRow(entry, opts)).join("")
			: `<li class="ws-runs-empty">${escapeHtml(textOption(opts, "empty"))}</li>`;
		const classes = `ws-runs-panel${waiting ? " is-awaiting" : ""}`;
		return `<details class="${classes}"${expanded ? " open" : ""}>
			${head}
			<ul class="ws-runs-list">${rows}</ul>
		</details>`;
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderWorkspaceRuns, awaitingCount, escapeHtml };
	} else {
		window.WorkspaceRuns = { renderWorkspaceRuns, awaitingCount };
	}
})();
