// conversation-index.js
// Pure renderer for the GET /api/workspace/conversations payload.
//
// It draws the thread rail: the list of the conversations this workspace has
// had, grouped by what they are about, plus the standing offer to open a new
// one. It knows no process rule — not one spec status, not one action name.
// Every word it draws about a thread comes from the payload, so a title or a
// code it has never seen is rendered as it is, unchanged.
//
// The only judgement it makes is presentational: which group a thread belongs
// to, and how long ago its last message was, said in words instead of an
// ISO timestamp. Both are functions of the payload plus an injected `now`, so
// a test owns the clock and never flakes.
//
// It is pure: no DOM, no network calls, no timers. It takes a view object and
// returns an HTML string. Wiring that string into the page, and deciding what a
// press on a thread should do, belong to the caller — every thread carries its
// conversation id as a data attribute for exactly that reason.
//
// The mockup docs/mockups/redesign-chat/stato-b-storico.html is the visual
// contract: the class names below are its class names. It also draws a search
// field, which is deliberately absent here — full-text search is out of scope
// for this spec, and an inert box would be a promise this page cannot keep.
//
// Consumable in both browser (defines ConversationIndex on the global object)
// and Node (exports renderConversationIndex / relativeTime /
// renderResumeBanner).
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

	/** Return the object at key, or null: partial payloads must not throw. */
	function objectAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return found && typeof found === "object" ? found : null;
	}

	function textAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return typeof found === "string" ? found : "";
	}

	/** Return the array at key, or an empty array: partial payloads must not throw. */
	function arrayAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return Array.isArray(found) ? found : [];
	}

	// The words this module owns. None of them names a step or a state of
	// anybody's method: they are the furniture of a list — what its groups are,
	// what the standing offer is, and what the list says when it is empty.
	const TEXT = {
		newThread: "Nuova conversazione",
		groupLive: "In corso",
		groupSpec: "Spec",
		groupFree: "Progetto",
		live: "in corso",
		empty: "Su questo workspace non c'è ancora nessuna conversazione.",
		freeCode: "···",
	};

	const MINUTE = 60 * 1000;
	const HOUR = 60 * MINUTE;
	const DAY = 24 * HOUR;
	const WEEK = 7 * DAY;

	function pad2(n) {
		return n < 10 ? `0${n}` : String(n);
	}

	function shortDate(date) {
		return `${pad2(date.getDate())}/${pad2(date.getMonth() + 1)}/${date.getFullYear()}`;
	}

	/**
	 * True when the entry says it is the conversation currently alive. The flag
	 * is read, never derived: whether a conversation is live is a server fact
	 * held by the process, and this module is in no position to recompute it.
	 */
	function isLive(entry) {
		return !!(entry && typeof entry === "object" && entry.live);
	}

	function threadClasses(entry, currentId) {
		const classes = ["thread"];
		if (!textAt(entry, "spec_code")) classes.push("is-free");
		const id = textAt(entry, "id");
		if (currentId && id && id === currentId) classes.push("is-current");
		return classes;
	}

	function renderMeta(entry, now) {
		const parts = [];
		if (isLive(entry)) {
			parts.push(
				`<span class="thread-flag is-live">${TEXT.live}</span>`,
			);
		}
		const when = relativeTime(textAt(entry, "last_message_at"), now);
		if (when) parts.push(escapeHtml(when));
		if (!parts.length) return "";
		return `<span class="thread-meta">${parts.join(" · ")}</span>`;
	}

	function renderThread(entry, currentId, now) {
		if (!entry || typeof entry !== "object") return "";
		const id = textAt(entry, "id");
		const code = textAt(entry, "spec_code");
		const classes = threadClasses(entry, currentId).join(" ");
		const current = classes.indexOf("is-current") >= 0 ? ` aria-current="true"` : "";
		return `<button type="button" class="${classes}" data-conversation-id="${escapeHtml(id)}"${current}>
			<span class="thread-code">${code ? escapeHtml(code) : TEXT.freeCode}</span>
			<span class="thread-title">${escapeHtml(textAt(entry, "title"))}</span>
			${renderMeta(entry, now)}
		</button>`;
	}

	/**
	 * A group is drawn only when it holds something. An empty group would leave
	 * an orphan heading standing over nothing, which reads as a list that failed
	 * to load rather than as a group that has no members.
	 */
	function renderGroup(label, entries, currentId, now) {
		if (!entries.length) return "";
		const threads = entries
			.map((entry) => renderThread(entry, currentId, now))
			.join("");
		return `<p class="rail-group">${label}</p>${threads}`;
	}

	// The mockup draws a `N` keyboard hint next to this command. It is not
	// rendered here for the same reason the search box is not: the viewer binds
	// no such shortcut, and a key legend that does nothing when pressed is a
	// promise this page cannot keep.
	function renderNewThread() {
		return `<button type="button" class="new-thread" data-conversation-new>
			<span>${TEXT.newThread}</span>
		</button>`;
	}

	// ---- public API ----

	/**
	 * How long ago, said in Italian, instead of an ISO timestamp nobody reads.
	 *
	 * `now` is injectable so the caller owns the clock: a test that fixes it
	 * asserts on a stable sentence, and the page passes the real one.
	 * An unparsable or absent value returns the empty string — an unknown moment
	 * is an answer, and it is silence, not a wrong guess.
	 *
	 * @param {string|number|Date|null} value  The moment being described.
	 * @param {string|number|Date|null} [now]  The moment to describe it from.
	 * @returns {string}
	 */
	function relativeTime(value, now) {
		if (value === null || value === undefined || value === "") return "";
		const then = value instanceof Date ? value : new Date(value);
		const at = then.getTime();
		if (!Number.isFinite(at)) return "";
		const reference = now === null || now === undefined ? new Date() : now;
		const base = reference instanceof Date ? reference : new Date(reference);
		const from = base.getTime();
		if (!Number.isFinite(from)) return "";

		const diff = from - at;
		if (diff < MINUTE) return "adesso";
		if (diff < HOUR) return `${Math.floor(diff / MINUTE)} min fa`;
		if (diff < DAY) {
			const hours = Math.floor(diff / HOUR);
			return hours === 1 ? "1 ora fa" : `${hours} ore fa`;
		}
		if (diff < 2 * DAY) return "ieri";
		if (diff <= WEEK) return `${Math.floor(diff / DAY)} giorni fa`;
		return shortDate(then);
	}

	/**
	 * Render the thread rail to an HTML string.
	 *
	 * What comes back is the *contents* of the rail — its top command and its
	 * list — and not the element that holds them: the caller owns that element,
	 * and owning it twice is how a page ends up with a rail inside a rail.
	 *
	 * Never throws on a partial payload: a null view, a missing conversations
	 * array and an entry without a title are all answers, and each has a
	 * rendering. An empty list is a rendered state, not an empty string — the
	 * rail says out loud that this workspace has had no conversation yet, and
	 * offers to open the first one, rather than showing a mute void.
	 *
	 * The order inside each group is the payload's order: the server already
	 * sorted by last message, and re-sorting here would let two truths about
	 * recency exist at once.
	 *
	 * @param {object|null} view  The /api/workspace/conversations payload.
	 * @param {{currentId?: string}} [ui]  Which conversation the page is showing.
	 * @returns {string} HTML string.
	 */
	function renderConversationIndex(view, ui) {
		const entries = arrayAt(view, "conversations").filter(
			(entry) => entry && typeof entry === "object",
		);
		const currentId = textAt(ui, "currentId");
		const now = ui && typeof ui === "object" ? ui.now : null;

		const top = `<div class="rail-top">${renderNewThread()}</div>`;

		if (!entries.length) {
			return `${top}
		<div class="rail-list">
			<p class="rail-empty">${TEXT.empty}</p>
		</div>`;
		}

		const live = entries.filter(isLive);
		const spec = entries.filter(
			(entry) => !isLive(entry) && !!textAt(entry, "spec_code"),
		);
		const free = entries.filter(
			(entry) => !isLive(entry) && !textAt(entry, "spec_code"),
		);

		const groups = [
			renderGroup(TEXT.groupLive, live, currentId, now),
			renderGroup(TEXT.groupSpec, spec, currentId, now),
			renderGroup(TEXT.groupFree, free, currentId, now),
		]
			.filter(Boolean)
			.join("");

		return `${top}
		<div class="rail-list">${groups}</div>`;
	}

	/**
	 * The declaration a person reads when the conversation on screen was opened
	 * to carry on an earlier one.
	 *
	 * It is deliberately explicit that this is a *new* conversation which was
	 * handed the earlier one as context, and not the earlier one resumed: the
	 * agent has no memory of that session, and a page that let the two look
	 * alike would be lying about what the agent knows.
	 *
	 * Returns the empty string when nothing was resumed — an absent banner is
	 * the honest rendering of an ordinary conversation.
	 *
	 * @param {object|null} view  A conversation payload, or {conversation: …}.
	 * @returns {string} HTML string.
	 */
	function renderResumeBanner(view) {
		const conversation = objectAt(view, "conversation") || view;
		const from = textAt(conversation, "resumed_from");
		if (!from) return "";
		return `<div class="resume-banner" role="note" data-resumed-from="${escapeHtml(from)}">
			<span class="resume-banner-mark">ripresa</span>
			<span class="resume-banner-body">Questa è una conversazione nuova che riprende <code>${escapeHtml(from)}</code>: ne ha ricevuto la storia come contesto.</span>
		</div>`;
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = {
			renderConversationIndex,
			relativeTime,
			renderResumeBanner,
			escapeHtml,
		};
	} else {
		window.ConversationIndex = {
			renderConversationIndex,
			relativeTime,
			renderResumeBanner,
		};
	}
})();
