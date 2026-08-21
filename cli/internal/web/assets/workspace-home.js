// workspace-home.js
// Pure renderer for the GET /api/workspaces payload.
//
// There is exactly one rendering of the known-workspaces list in the viewer,
// and it lives here: the modal reached from the topbar and the home shown when
// no workspace is open draw the same rows, from the same function, so a change
// to what an entry says can never apply to one of the two and not the other.
//
// It is pure: no DOM, no fetch, no document. It takes the payload and returns
// an HTML string; the time format is injected, so the module carries no clock
// either. Wiring the string into the page and acting on the buttons belong to
// the caller — the data-attributes are the whole contract between them.
//
// Consumable in both browser (defines window.WorkspaceHome) and Node
// (exports renderWorkspaceHome / renderWorkspaceRows / escapeHtml).
(function () {
	// The server owns both the order of the list and the vocabulary of the
	// statuses. The frontend only knows how to phrase a status it recognises and
	// falls back to the raw value for anything it does not: a new server status
	// must never disappear from the UI just because this map is older.
	const WORKSPACE_STATUS_LABELS = {
		missing: "not found",
		not_a_directory: "not a directory",
		not_readable: "not readable",
		not_a_workspace: "not an ARchetipo workspace",
	};

	/**
	 * Escape the five characters that would otherwise let payload text break
	 * out of the markup it is interpolated into. Names and paths come from the
	 * user's disk: not one of them is ever interpolated raw.
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

	/** The entries of the payload, or an empty list: a partial payload must not throw. */
	function entriesOf(view) {
		const value = view && typeof view === "object" ? view : {};
		return Array.isArray(value.workspaces) ? value.workspaces : [];
	}

	/** The injected time formatter, or a plain string fallback. */
	function timeFormatter(opts) {
		const fn = opts && typeof opts.formatTime === "function"
			? opts.formatTime
			: null;
		return (value) => {
			if (fn) return fn(value);
			return value === null || value === undefined ? "" : String(value);
		};
	}

	/** The phrase for an unreachable entry, in the server's own words when unknown. */
	function statusLabel(item) {
		return (
			WORKSPACE_STATUS_LABELS[item.status] || item.status || "unreachable"
		);
	}

	// An unreachable entry keeps its place in the list and says why: hiding it
	// would leave the user with a registry that silently disagrees with the disk.
	function statusBadge(item) {
		if (item.reachable) {
			return `<span class="workspace-badge">reachable</span>`;
		}
		return `<span class="workspace-badge warn">${escapeHtml(statusLabel(item))}</span>`;
	}

	// Open is offered but disabled where it would be a lie: on the workspace
	// already served, on one with no identity in the registry, and on one the
	// server has just probed as unreachable. The title carries the reason, so a
	// greyed button is never a mystery.
	function openRefusal(item) {
		if (!item.id) return "This entry has no identity in the registry";
		if (item.current) return "This is the workspace already open";
		if (!item.reachable) return `Cannot be opened: ${statusLabel(item)}`;
		return "";
	}

	function renderRow(item, formatTime) {
		const entry = item && typeof item === "object" ? item : {};
		const id = escapeHtml(entry.id || "");
		const label = escapeHtml(entry.name || entry.path || "");
		const refusal = openRefusal(entry);
		const openAttrs =
			`data-open="${id}" data-open-name="${label}"` +
			(refusal ? ` disabled title="${escapeHtml(refusal)}"` : "");

		const head =
			`<span class="workspace-name">${escapeHtml(entry.name || "")}</span>` +
			(entry.current ? `<span class="workspace-badge">current</span>` : "") +
			statusBadge(entry);

		return (
			`<div class="workspace-row">` +
			`<div class="workspace-row-main">` +
			`<div class="workspace-row-head">${head}</div>` +
			`<code class="workspace-path">${escapeHtml(entry.path || "")}</code>` +
			`<span class="workspace-meta">Last opened: ${escapeHtml(formatTime(entry.lastOpenedAt))}</span>` +
			`</div>` +
			`<div class="workspace-row-actions">` +
			`<button type="button" class="primary-btn" ${openAttrs}>Open</button>` +
			`<button type="button" class="ghost-btn" data-remove="${id}" data-remove-name="${label}">Remove</button>` +
			`</div>` +
			`</div>`
		);
	}

	// ---- public API ----

	/**
	 * Render the rows of the known-workspaces list to an HTML string.
	 * An empty or missing list renders as the empty string: what to say instead
	 * of nothing is the caller's decision, and the home and the modal say it
	 * differently.
	 *
	 * @param {object|null} view  The /api/workspaces payload.
	 * @param {{formatTime?: function}} [opts]  Injected time formatter.
	 * @returns {string} HTML string.
	 */
	function renderWorkspaceRows(view, opts) {
		const formatTime = timeFormatter(opts);
		return entriesOf(view)
			.map((item) => renderRow(item, formatTime))
			.join("");
	}

	/**
	 * Render the whole home: the heading that says no workspace is open, and
	 * the rows — or, with nothing recorded, the same sentence the modal uses.
	 *
	 * @param {object|null} view  The /api/workspaces payload.
	 * @param {{formatTime?: function, message?: string}} [opts]
	 * @returns {string} HTML string.
	 */
	function renderWorkspaceHome(view, opts) {
		const options = opts && typeof opts === "object" ? opts : {};
		const rows = renderWorkspaceRows(view, options);
		const body = rows
			? `<div class="workspace-home-list">${rows}</div>`
			: `<p class="form-notice">No workspace has been recorded yet. Create one, or add an existing one below.</p>`;
		// A message is how the caller reports that the list could not be read:
		// a page that does not know which workspace it serves must say so rather
		// than draw a board for one.
		const message = options.message
			? `<p class="status-msg err">${escapeHtml(options.message)}</p>`
			: "";
		return (
			`<header class="workspace-home-head">` +
			`<span class="workspace-home-eyebrow">workspaces</span>` +
			`<h1 class="workspace-home-title">Choose a workspace</h1>` +
			`<p class="workspace-home-sub">No workspace is open. Open one of the workspaces below, add an existing one, or create a new one.</p>` +
			`</header>` +
			message +
			body
		);
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderWorkspaceHome, renderWorkspaceRows, escapeHtml };
	} else {
		window.WorkspaceHome = { renderWorkspaceHome, renderWorkspaceRows };
	}
})();
