// conversation.js
// Pure renderer for the GET /api/workspace/conversation payload.
//
// The module contains no process rules: not one capability name, not one
// provider name, not one action identifier. Every word it draws about what is
// offered — and about what is refused — comes from the payload, so a reason it
// has never seen is rendered as it is, unchanged. Changing what a workspace can
// hold therefore changes what this module draws without a line changing here.
//
// It is pure: no DOM, no fetch, no document, no timers. It takes the view of
// the route plus the text being typed and returns an HTML string. Wiring that
// string into the page, reading the route and sending commands belong to the
// caller.
//
// Consumable in both browser (defines window.Conversation) and Node
// (exports renderConversation / escapeHtml).
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

	// The lifecycle words the wire carries. They are the vocabulary of a live
	// process, not of the process a workspace follows: nothing here names a step
	// of anybody's method.
	const STATE_LABELS = {
		ACTIVE: "talking",
		CLOSED: "ended",
		CRASHED: "ended badly",
	};

	const STATE_VARIANTS = {
		ACTIVE: "conv-live",
		CLOSED: "conv-ended",
		CRASHED: "conv-failed",
	};

	// The kinds the timeline knows how to label, each with the modifier class
	// the stylesheet differentiates it by. The vocabulary belongs to the agent
	// and may grow, so an unknown kind degrades to a readable generic row
	// instead of disappearing — and it takes the `cev-unknown` variant, drawn as
	// explicitly uninterpreted rather than disguised as agent text.
	const EVENT_KINDS = {
		text: { label: "agent", variant: "cev-agent" },
		thinking: { label: "thinking", variant: "cev-thinking" },
		user_message: { label: "you", variant: "cev-you" },
		tool_start: { label: "tool · start", variant: "cev-tool-start" },
		tool_end: { label: "tool · done", variant: "cev-tool-end" },
		tool_error: { label: "tool · error", variant: "cev-tool-error" },
		turn_end: { label: "turn end", variant: "cev-turn-end" },
	};

	function known(table, key) {
		return typeof key === "string" &&
			Object.prototype.hasOwnProperty.call(table, key);
	}

	/**
	 * One additive row above the timeline. Every caller passes a tone from the
	 * stylesheet's closed set — info, refused, ok — so payload text can never
	 * choose how it is presented.
	 */
	function renderNotice(tone, mark, body) {
		return `<div class="conv-notice ${tone}"${tone === "refused" ? ' role="alert"' : ""}>
			<span class="conv-notice-mark">${escapeHtml(mark)}</span>
			<span class="conv-notice-body">${escapeHtml(body)}</span>
		</div>`;
	}

	function formatTime(value) {
		if (!value) return "";
		const at = new Date(value);
		if (Number.isNaN(at.getTime())) return "";
		return at.toLocaleTimeString();
	}

	// One row of the timeline: the rail carries the monotonic id, the body the
	// head and the text. The id is shown because it is what makes the cursor
	// legible — a reader can see that the history went 5, 6, 7 and repeated
	// nothing.
	function renderEvent(event) {
		if (!event || typeof event !== "object") return "";
		const kind = typeof event.kind === "string" ? event.kind : "";
		const entry = known(EVENT_KINDS, kind) ? EVENT_KINDS[kind] : null;
		const variant = entry ? entry.variant : "cev-unknown";
		const label = entry ? entry.label : kind || "event";
		const head = [`<span class="conv-event-kind">${escapeHtml(label)}</span>`];
		const stamp = formatTime(event.at);
		if (stamp) {
			head.push(`<span class="conv-event-time">${escapeHtml(stamp)}</span>`);
		}
		const text = String(event.text === null || event.text === undefined ? "" : event.text);
		const lines = [];
		if (event.tool) {
			lines.push(
				`<p class="conv-event-text"><code class="conv-event-tool">${escapeHtml(event.tool)}</code></p>`,
			);
			if (text) {
				lines.push(`<p class="conv-event-detail">${escapeHtml(text)}</p>`);
			}
		} else if (text) {
			lines.push(`<p class="conv-event-text">${escapeHtml(text)}</p>`);
		}
		const id = event.id === null || event.id === undefined ? "" : String(event.id);
		return `<li class="conv-event ${variant}">
			<div class="conv-event-rail"><span class="conv-event-glyph" aria-hidden="true"></span>#${escapeHtml(id)}</div>
			<div class="conv-event-body">
				<div class="conv-event-head">${head.join("")}</div>
				${lines.join("")}
			</div>
		</li>`;
	}

	// The timeline, with the partial-history declaration at its head when the
	// payload says the history is partial. The sentence is the payload's own:
	// how much was dropped and where the kept history begins is a server fact,
	// and this module never writes a second wording for it.
	function renderTimeline(view, active) {
		const events = Array.isArray(view.events) ? view.events : [];
		const rows = [];
		if (view.truncated) {
			const declared =
				textAt(view, "notice") ||
				"the oldest part of this conversation is no longer kept";
			rows.push(
				`<li class="conv-history-partial" role="note"><span class="conv-history-mark">partial history</span><span class="conv-history-body">${escapeHtml(declared)}</span></li>`,
			);
		}
		if (!events.length) {
			rows.push(
				'<li class="conv-timeline-empty">Nothing has been said in this conversation yet.</li>',
			);
		} else {
			for (const event of events) rows.push(renderEvent(event));
		}
		const frozen = active ? "" : " is-frozen";
		return `<ol class="conv-timeline${frozen}">${rows.join("")}</ol>`;
	}

	// The close control exists only while the conversation is live: one that has
	// ended cannot be closed again, so offering it would be a lie.
	//
	// The confirmation is inline rather than a window.confirm: a modal dialog
	// would block the loop that keeps the panel current.
	function renderCloseControl(ui) {
		const disabled = ui.busy ? " disabled" : "";
		if (!ui.closeArmed) {
			return `<span class="conv-close">
				<button type="button" class="ghost-btn danger-ghost-btn" data-conversation-close-open${disabled}>Close conversation</button>
			</span>`;
		}
		return `<span class="conv-close">
			<span class="conv-close-confirm">
				<span class="conv-close-question">Close it and let the agent go?</span>
				<button type="button" class="approval-btn deny" data-conversation-close-confirm${disabled}>Yes, close</button>
				<button type="button" class="approval-btn" data-conversation-close-abort${disabled}>No</button>
			</span>
		</span>`;
	}

	// `offered` is whether a conversation can be opened in this workspace at all.
	// It gates the writing half and nothing else: a live conversation whose
	// provider is no longer the offered one takes no new message, but it is still
	// holding an agent process, so its close control stays exactly where it was.
	function renderComposer(active, draft, ui, offered) {
		const writable = active && offered !== false;
		const disabled = ui.busy || !writable ? " disabled" : "";
		const placeholder = writable
			? "Write to the agent…"
			: active
				? "This conversation takes no more messages: close it to let the agent go"
				: "This conversation is over and takes no more messages";
		const close = active ? renderCloseControl(ui) : "";
		const hint = writable ? "⌘ + enter to send" : "read only";
		return `<form class="conv-composer">
			<textarea class="conv-composer-input" rows="2" placeholder="${escapeHtml(placeholder)}"${disabled}>${escapeHtml(draft)}</textarea>
			<div class="conv-composer-row">
				${close}
				<span class="conv-composer-spacer"></span>
				<span class="conv-composer-hint">${escapeHtml(hint)}</span>
				<button type="submit" class="primary-btn"${disabled}>Send</button>
			</div>
		</form>`;
	}

	function renderHead(view, badge, variant) {
		const dir = objectAt(view, "conversation")
			? textAt(objectAt(view, "conversation"), "working_dir")
			: "";
		const provider = textAt(view, "provider_id");
		const dirHtml = dir
			? `<code class="conv-dir" title="${escapeHtml(dir)}">${escapeHtml(dir)}</code>`
			: "";
		const providerHtml = provider
			? `<span class="conv-provider">${escapeHtml(provider)}</span>`
			: "";
		return `<div class="conv-head">
			<span class="conv-badge ${variant}">${escapeHtml(badge)}</span>
			${dirHtml}
			<span class="conv-head-spacer"></span>
			${providerHtml}
		</div>`;
	}

	function renderOpenButton(ui, label) {
		const disabled = ui.busy ? " disabled" : "";
		return `<button type="button" class="primary-btn conv-open" data-conversation-open${disabled}>${escapeHtml(label)}</button>`;
	}

	// ---- public API ----

	/**
	 * Render the conversation view to an HTML string.
	 *
	 * Never throws on a partial payload: a null view, a missing conversation, a
	 * missing events list are all answers, and each has a rendering.
	 *
	 * @param {object|null} view  The /api/workspace/conversation payload.
	 * @param {string} draft      The text being typed, preserved across renders.
	 * @param {object} [ui]       Local, non-payload state of the panel:
	 *                            {busy, closeArmed, refusal, link}.
	 * @returns {string} HTML string.
	 */
	function renderConversation(view, draft, ui) {
		const value = view && typeof view === "object" ? view : {};
		const local = ui && typeof ui === "object" ? ui : {};
		const typed = typeof draft === "string" ? draft : "";
		const blocks = [];

		const conversation = objectAt(value, "conversation");
		// Not offered says a conversation cannot be *opened* here; it says nothing
		// about one that is already open. The two facts are independent in the
		// payload — a workspace can hold a live conversation whose provider is no
		// longer the offered one — so only their conjunction leaves nothing to
		// draw.
		const offered = value.available !== false;
		const refusal =
			textAt(value, "unavailable_reason") ||
			"a conversation cannot be opened in this workspace";

		// Refused here and now, with nothing open: the reason is the server's
		// sentence, shown verbatim, and nothing is offered next to it — neither a
		// composer nor a way to open one.
		if (!offered && !conversation) {
			return `<section class="conv-panel conv-unavailable" aria-label="Conversation">
				${renderHead(value, "not offered", "conv-off")}
				${renderNotice("refused", "not offered", refusal)}
			</section>`;
		}

		if (!conversation) {
			// Nothing open. The invitation is the whole answer, and the button is
			// the only control.
			return `<section class="conv-panel conv-empty-panel" aria-label="Conversation">
				${renderHead(value, "none open", "conv-off")}
				<div class="conv-empty">
					<p class="conv-empty-text">No conversation is open for this workspace.</p>
					${renderOpenButton(local, "Open a conversation")}
				</div>
			</section>`;
		}

		const state = textAt(conversation, "state");
		const active = state === "ACTIVE";
		const variant = known(STATE_VARIANTS, state)
			? STATE_VARIANTS[state]
			: "conv-ended";
		const badge = known(STATE_LABELS, state) ? STATE_LABELS[state] : state || "conversation";

		// The refusal rides at the head of a conversation that is open anyway: it
		// explains why nothing more can be said here, while the history stays
		// readable and the close control stays offered — the conversation is still
		// holding an agent, and letting it go must remain possible.
		if (!offered) blocks.push(renderNotice("refused", "not offered", refusal));

		const failure = textAt(conversation, "error");
		if (failure) blocks.push(renderNotice("refused", "error", failure));
		if (local.refusal) {
			blocks.push(renderNotice("refused", "refused", String(local.refusal)));
		}
		// A note the server sent that is not the partial-history declaration: the
		// declaration lives at the head of the timeline and nowhere else, so one
		// fact is never stated twice.
		if (!value.truncated && textAt(value, "notice")) {
			blocks.push(renderNotice("info", "note", textAt(value, "notice")));
		}
		if (local.link) {
			blocks.push(renderNotice("info", "channel", String(local.link)));
		}
		if (!active) {
			blocks.push(
				renderNotice(
					"info",
					"over",
					"This conversation is over: the agent behind it is no longer engaged, and what was said stays readable.",
				),
			);
		}
		blocks.push(renderTimeline(value, active));
		blocks.push(renderComposer(active, typed, local, offered));
		if (!active && offered) {
			blocks.push(
				`<div class="conv-again">${renderOpenButton(local, "Open a new conversation")}</div>`,
			);
		}

		return `<section class="conv-panel ${variant}" aria-label="Conversation">
			${renderHead(value, badge, variant)}
			${blocks.join("")}
		</section>`;
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderConversation, escapeHtml };
	} else {
		window.Conversation = { renderConversation };
	}
})();
