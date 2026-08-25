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
		missing: "non trovato",
		not_a_directory: "non è una directory",
		not_readable: "non leggibile",
		not_a_workspace: "non è un workspace ARchetipo",
	};

	// Le parole che questo modulo possiede, tutte insieme e in una lingua sola.
	// Nessuna nomina un passo del metodo: sono le parole di un elenco — che cosa
	// è una riga, che cosa se ne può fare, e che cosa dice l'elenco vuoto.
	const TEXT = {
		reachable: "raggiungibile",
		current: "aperto",
		noIdentity: "Questa voce non ha un'identità nel registro",
		alreadyOpen: "È il workspace già aperto",
		cannotOpen: "Non si può aprire",
		lastOpened: "Ultima apertura",
		open: "Apri",
		remove: "Rimuovi",
		eyebrow: "workspace",
		title: "Scegli un workspace",
		subtitle:
			"Non c'è nessun workspace aperto. Aprine uno fra quelli qui sotto, aggiungine uno esistente, oppure creane uno nuovo.",
		empty:
			"Non è ancora stato registrato nessun workspace. Creane uno, oppure aggiungine uno esistente qui sotto.",
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
			return `<span class="workspace-badge">${escapeHtml(TEXT.reachable)}</span>`;
		}
		return `<span class="workspace-badge warn">${escapeHtml(statusLabel(item))}</span>`;
	}

	// Open is offered but disabled where it would be a lie: on the workspace
	// already served, on one with no identity in the registry, and on one the
	// server has just probed as unreachable. The title carries the reason, so a
	// greyed button is never a mystery.
	function openRefusal(item) {
		if (!item.id) return TEXT.noIdentity;
		if (item.current) return TEXT.alreadyOpen;
		if (!item.reachable) return `${TEXT.cannotOpen}: ${statusLabel(item)}`;
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
			(entry.current
				? `<span class="workspace-badge">${escapeHtml(TEXT.current)}</span>`
				: "") +
			statusBadge(entry);

		return (
			`<div class="workspace-row">` +
			`<div class="workspace-row-main">` +
			`<div class="workspace-row-head">${head}</div>` +
			`<code class="workspace-path">${escapeHtml(entry.path || "")}</code>` +
			`<span class="workspace-meta">${escapeHtml(TEXT.lastOpened)}: ${escapeHtml(formatTime(entry.lastOpenedAt))}</span>` +
			`</div>` +
			`<div class="workspace-row-actions">` +
			`<button type="button" class="primary-btn" ${openAttrs}>${escapeHtml(TEXT.open)}</button>` +
			`<button type="button" class="ghost-btn" data-remove="${id}" data-remove-name="${label}">${escapeHtml(TEXT.remove)}</button>` +
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
			: `<p class="form-notice">${escapeHtml(TEXT.empty)}</p>`;
		// A message is how the caller reports that the list could not be read:
		// a page that does not know which workspace it serves must say so rather
		// than draw a board for one.
		const message = options.message
			? `<p class="status-msg err">${escapeHtml(options.message)}</p>`
			: "";
		return (
			`<header class="workspace-home-head">` +
			`<span class="workspace-home-eyebrow">${escapeHtml(TEXT.eyebrow)}</span>` +
			`<h1 class="workspace-home-title">${escapeHtml(TEXT.title)}</h1>` +
			`<p class="workspace-home-sub">${escapeHtml(TEXT.subtitle)}</p>` +
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
