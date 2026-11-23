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
	// action of anybody's method: they are the furniture of a list. A caller
	// that wants other wording passes it in.
	const DEFAULT_TEXT = {
		title: "Esecuzioni in corso",
		awaiting: "aspetta te",
		stop: "Ferma",
	};

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
			${entry.can_stop === false ? "" : `<button type="button" class="ghost-btn" data-run-stop="${escapeHtml(id)}">${escapeHtml(textOption(opts, "stop"))}</button>`}
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
	 * How many entries the payload lists, waiting or not: the very entries the
	 * list draws, so the indicator that opens the list and the list itself can
	 * never disagree on whether there is anything to show.
	 *
	 * @param {object|null} view  The /api/workspace/runs payload.
	 * @returns {number}
	 */
	function runCount(view) {
		return arrayAt(view, "runs").filter(
			(entry) => entry && typeof entry === "object",
		).length;
	}

	/**
	 * Render the workspace runs view to an HTML string.
	 *
	 * Never throws on a partial payload: a null view, a missing runs array and
	 * an entry without action or status are all answers, and each has a
	 * rendering. With nothing in flight it renders nothing at all: the list is
	 * shown on request, from the indicator of the topbar, and an indicator with
	 * nothing to count is not drawn either.
	 *
	 * The panel is the whole list under one heading — what the list is and how
	 * many entries are waiting. It takes no room of its own on the page:
	 * whether it is on screen is the caller's choice, because it is a gesture a
	 * person made and the list is redrawn from scratch on every poll.
	 *
	 * @param {object|null} view  The /api/workspace/runs payload.
	 * @param {{title?: string, awaiting?: string, stop?: string}} [opts]
	 *        Wording overrides for the furniture of the list.
	 * @returns {string} HTML string.
	 */
	function renderWorkspaceRuns(view, opts) {
		const entries = arrayAt(view, "runs").filter(
			(entry) => entry && typeof entry === "object",
		);
		if (!entries.length) return "";
		const waiting = entries.filter(isAwaiting).length;
		const badge = waiting
			? `<span class="ws-runs-badge is-awaiting">${escapeHtml(String(waiting))}</span>`
			: "";
		const head = `<div class="ws-runs-head">
			<span class="ws-runs-title">${escapeHtml(textOption(opts, "title"))}</span>
			<span class="ws-runs-head-spacer"></span>
			${badge}
		</div>`;
		const rows = entries.map((entry) => renderRow(entry, opts)).join("");
		const classes = `ws-runs-panel${waiting ? " is-awaiting" : ""}`;
		return `<div class="${classes}">
			${head}
			<ul class="ws-runs-list">${rows}</ul>
		</div>`;
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderWorkspaceRuns, awaitingCount, runCount, escapeHtml };
	} else {
		window.WorkspaceRuns = { renderWorkspaceRuns, awaitingCount, runCount };
	}
})();
