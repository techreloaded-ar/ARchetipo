// conversation.js
// Pure renderer for the GET /api/workspace/conversations/{id} payload.
//
// The module contains no process rules: not one capability name, not one
// provider name, not one action identifier. Every word it draws about what is
// offered — and about what is refused — comes from the payload, so a reason it
// has never seen is rendered as it is, unchanged. Changing what a workspace can
// hold therefore changes what this module draws without a line changing here.
//
// The same holds for a proposed step: its name, its target, the reason it
// cannot be taken and the remedy that would unlock it all arrive in the
// payload, already resolved by the server against the workspace. This module
// still does not know what a step of anybody's method is — it draws a label, a
// code and a sentence — and it never decides whether one can be taken.
//
// It also draws the runs the payload carries: a run started from this
// conversation is drawn where it was asked for — right after the event whose id
// the payload names as its anchor — with its log, and with the card of every
// decision it is waiting on. None of that is a judgement made here: whether a
// run is live, whether it is waiting, which answers a decision accepts and what
// a tool was asked to do are all facts of the payload, drawn as they arrive.
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

	/** Return the array at key, or an empty array: partial payloads must not throw. */
	function arrayAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return Array.isArray(found) ? found : [];
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

	// The affirmative/negative tone of an answer, keyed by the kind the provider
	// declares for it. It is the same closed table the run panel uses, and it is
	// the only thing this module reads from an option besides its id and its
	// label: an unknown kind simply gets the neutral button, and no label is
	// ever parsed to guess what pressing it would mean.
	const OPTION_TONES = {
		allow: " allow",
		approve: " allow",
		accept: " allow",
		deny: " deny",
		reject: " deny",
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

	// The run's own history, compressed to one line per event: the flow shows
	// that the run is speaking, not everything it said — the whole log stays one
	// press away, behind the control at the foot of the block.
	//
	// The kinds are not interpreted here any more than they are in the timeline:
	// the same table labels them, and an unknown kind is printed as it arrived.
	function renderRunLog(events) {
		const rows = (Array.isArray(events) ? events : []).filter(
			(event) => event && typeof event === "object",
		);
		if (!rows.length) {
			return '<pre class="conv-run-log conv-run-log-empty">This run has published nothing yet.</pre>';
		}
		const lines = rows.map((event) => {
			const kind = typeof event.kind === "string" ? event.kind : "";
			const entry = known(EVENT_KINDS, kind) ? EVENT_KINDS[kind] : null;
			const label = entry ? entry.label : kind || "event";
			const text = String(
				event.text === null || event.text === undefined ? "" : event.text,
			);
			const tool = typeof event.tool === "string" ? event.tool : "";
			const body = tool && text ? `${tool} · ${text}` : tool || text;
			const parts = [formatTime(event.at), label];
			if (body) parts.push(body);
			return escapeHtml(parts.filter(Boolean).join(" · "));
		});
		return `<pre class="conv-run-log">${lines.join("\n")}</pre>`;
	}

	/**
	 * The command a run is waiting a consent for, in the clear.
	 *
	 * The arguments are serialised, never summarised: a person answering about a
	 * command must read the command that will actually run, so the payload's own
	 * object is printed whole. The buttons are the options the payload declares
	 * and nothing else — no answer is invented here, and none is hidden.
	 *
	 * When the panel already knows how this decision was answered, the card is
	 * drawn resolved: the chosen option stays marked and readable, so a refusal
	 * remains on the page after the provider has stopped listing it as pending.
	 */
	function renderRunApprovalCard(approval, run, ui) {
		if (!approval || typeof approval !== "object") return "";
		const id = textAt(approval, "id");
		if (!id) return "";
		const local = ui && typeof ui === "object" ? ui : {};
		const answers = objectAt(local, "answeredApprovals") || {};
		const answered = Object.prototype.hasOwnProperty.call(answers, id)
			? answers[id]
			: null;
		const execution = textAt(run, "execution_id");

		const head = [
			`<span class="run-approval-eyebrow">${answered ? "approval resolved" : "approval requested"}</span>`,
		];
		const tool = textAt(approval, "tool_name");
		if (tool) {
			head.push(`<code class="run-approval-tool">${escapeHtml(tool)}</code>`);
		}
		const stamp = formatTime(approval.created_at);
		if (stamp) {
			head.push(`<span class="conv-event-time">${escapeHtml(stamp)}</span>`);
		}

		const args = formatArgs(approval.args);
		const argsBlock = args
			? `<pre class="run-approval-args">${escapeHtml(args)}</pre>`
			: "";

		const chosenID =
			answered && typeof answered === "object" ? textAt(answered, "optionID") : "";
		const options = (Array.isArray(approval.options) ? approval.options : [])
			.filter((option) => option && typeof option === "object" && option.id)
			.map((option) => {
				const optionID = String(option.id);
				const kind = typeof option.kind === "string" ? option.kind : "";
				// hasOwnProperty, not a bare lookup: the kind is remote text, and a
				// kind spelled "constructor" would otherwise resolve to an inherited
				// member and be interpolated into the class attribute.
				const tone = Object.prototype.hasOwnProperty.call(OPTION_TONES, kind)
					? OPTION_TONES[kind]
					: "";
				const chosen = answered && chosenID === optionID ? " is-chosen" : "";
				const off = answered || local.busy ? " disabled" : "";
				const label =
					typeof option.label === "string" && option.label
						? option.label
						: optionID;
				return `<button type="button" class="approval-btn${tone}${chosen}" data-run-approval-id="${escapeHtml(id)}" data-run-option-id="${escapeHtml(optionID)}" data-execution-id="${escapeHtml(execution)}"${off}>${escapeHtml(label)}</button>`;
			})
			.join("");

		const card = ["run-approval", "conv-run-approval"];
		if (answered) card.push("is-answered");
		if (answered && answered.denied) card.push("is-denied");
		const outcome = answered
			? `<div class="run-approval-outcome${answered.denied ? " denied" : ""}">${escapeHtml(
					textAt(answered, "label") || chosenID,
				)}</div>`
			: "";
		return `<div class="${card.join(" ")}" role="group" aria-label="${answered ? "Resolved approval" : "Pending approval"}">
			<div class="run-approval-head">${head.join("")}</div>
			<p class="run-approval-title">${escapeHtml(textAt(approval, "title") || "The run is waiting for a decision")}</p>
			${argsBlock}
			<div class="run-approval-options">${options}</div>
			${outcome}
		</div>`;
	}

	// Arguments as they are: an object is serialised whole, text is kept as it
	// was written. Nothing is shortened — the point of showing a command is that
	// it can be read before it is allowed.
	function formatArgs(payload) {
		if (payload === null || payload === undefined) return "";
		if (typeof payload === "string") return payload.trim();
		try {
			return JSON.stringify(payload, null, 2);
		} catch (_) {
			return "";
		}
	}

	// One run, drawn as a block of the flow. Everything in it is the payload's:
	// the action and the label it was started under, the target it names, the
	// state of the run and the state of the record, the note the server attached
	// when it could not read one of them.
	//
	// The modifier is read, never derived: waiting is a fact the server states,
	// and this module is in no position to recompute whether a run is blocked on
	// a person. The control at the foot leads to the whole log, which lives in
	// the run panel and is not duplicated here.
	function renderRunBlock(run, ui) {
		if (!run || typeof run !== "object") return "";
		const local = ui && typeof ui === "object" ? ui : {};
		const snapshot = objectAt(run, "run");
		const state = snapshot ? textAt(snapshot, "state") : "";
		const waiting = !!run.awaiting_response;
		const modifier = waiting
			? "is-waiting"
			: state === "ACTIVE"
				? "is-live"
				: "is-ended";
		const anchor =
			run.anchor_event_id === null || run.anchor_event_id === undefined
				? ""
				: String(run.anchor_event_id);
		const execution = textAt(run, "execution_id");
		const code = textAt(run, "spec_code");
		const scope = textAt(run, "scope");

		const head = ['<span class="conv-run-mark">run</span>'];
		const action = textAt(run, "action");
		const label = textAt(run, "label") || action;
		if (label) {
			head.push(`<b class="conv-run-label">${escapeHtml(label)}</b>`);
		}
		if (code) {
			head.push(`<code class="conv-run-code">${escapeHtml(code)}</code>`);
		}
		head.push('<span class="conv-run-head-spacer"></span>');
		if (state) {
			head.push(`<span class="conv-run-state">${escapeHtml(state)}</span>`);
		}
		const status = textAt(run, "status");
		if (status) {
			head.push(`<span class="conv-run-status">${escapeHtml(status)}</span>`);
		}

		const rows = [];
		const notice = textAt(run, "notice");
		if (notice) {
			rows.push(`<div class="conv-run-notice">${escapeHtml(notice)}</div>`);
		}
		rows.push(renderRunLog(run.events));
		if (run.truncated) {
			rows.push(
				'<div class="conv-run-partial" role="note">Older history of this run is beyond the window this viewer keeps; the provider still holds it.</div>',
			);
		}
		const pending = Array.isArray(run.approvals) ? run.approvals : [];
		for (const approval of pending) {
			rows.push(renderRunApprovalCard(approval, run, local));
		}

		// A decision answered from this panel keeps its card once the provider
		// has stopped listing the approval as pending: the refusal must stay
		// readable in the conversation that refused it. While the approval is
		// still pending the payload's own card stands and nothing is added here
		// — the same discipline the run panel applies to its answered approval.
		//
		// The card is drawn from the declaration the caller kept, so the command
		// and the options remain the provider's own words. An answer that names
		// no run, or an answer whose declaration is missing, draws nothing: this
		// module invents no approval it has not been given.
		const stillPending = {};
		for (const approval of pending) {
			const pendingID = approval ? textAt(approval, "id") : "";
			if (pendingID) stillPending[pendingID] = true;
		}
		const answers = objectAt(local, "answeredApprovals") || {};
		for (const approvalID of Object.keys(answers)) {
			if (Object.prototype.hasOwnProperty.call(stillPending, approvalID)) {
				continue;
			}
			const answered = answers[approvalID];
			const declared = objectAt(answered, "approval");
			if (!declared) continue;
			const owner = textAt(answered, "executionID");
			if (owner && execution && owner !== execution) continue;
			rows.push(
				renderRunApprovalCard(
					Object.assign({}, declared, { id: approvalID }),
					run,
					local,
				),
			);
		}

		const disabled = local.busy ? " disabled" : "";
		const reach = `<div class="conv-run-controls">
			<button type="button" class="ghost-btn conv-run-reach" data-conversation-reach-run data-scope="${escapeHtml(scope)}" data-code="${escapeHtml(code)}" data-execution-id="${escapeHtml(execution)}"${disabled}>Go to the whole log</button>
		</div>`;

		const highlight =
			anchor && String(local.highlightAnchor || "") === anchor
				? " is-highlight"
				: "";
		return `<div class="conv-run ${modifier}${highlight}" data-conversation-run-anchor="${escapeHtml(anchor)}" data-execution-id="${escapeHtml(execution)}">
			<div class="conv-run-head">${head.join("")}</div>
			${rows.filter(Boolean).join("")}
			${reach}
		</div>`;
	}

	// True when the payload says at least one of its runs is waiting for an
	// answer. Read, never derived, by the same rule as the block modifier.
	function anyRunAwaiting(view) {
		return arrayAt(view, "runs").some(
			(run) => run && typeof run === "object" && !!run.awaiting_response,
		);
	}

	// The timeline, with the partial-history declaration at its head when the
	// payload says the history is partial. The sentence is the payload's own:
	// how much was dropped and where the kept history begins is a server fact,
	// and this module never writes a second wording for it.
	//
	// The runs the payload carries are interleaved into it: each block is
	// emitted right after the event whose id it names as its anchor, because
	// that is where it was asked for. A run whose anchor has already left the
	// retained history is emitted at the tail — out of place, but present: a run
	// that is waiting for an answer must never become invisible because the line
	// that started it has aged out.
	function renderTimeline(view, active, ui) {
		const events = Array.isArray(view.events) ? view.events : [];
		const local = ui && typeof ui === "object" ? ui : {};
		const anchored = new Map();
		const pending = [];
		for (const run of arrayAt(view, "runs")) {
			if (!run || typeof run !== "object") continue;
			const block = renderRunBlock(run, local);
			if (!block) continue;
			const anchor =
				run.anchor_event_id === null || run.anchor_event_id === undefined
					? ""
					: String(run.anchor_event_id);
			const row = `<li class="conv-run-row">${block}</li>`;
			if (!anchor) {
				pending.push(row);
				continue;
			}
			if (!anchored.has(anchor)) anchored.set(anchor, []);
			anchored.get(anchor).push(row);
		}
		const emitted = new Set();
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
			for (const event of events) {
				rows.push(renderEvent(event));
				const id =
					event && event.id !== null && event.id !== undefined
						? String(event.id)
						: "";
				if (!id || emitted.has(id) || !anchored.has(id)) continue;
				emitted.add(id);
				for (const row of anchored.get(id)) rows.push(row);
			}
		}
		for (const [anchor, blocks] of anchored) {
			if (emitted.has(anchor)) continue;
			for (const row of blocks) rows.push(row);
		}
		for (const row of pending) rows.push(row);
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
	//
	// A run waiting for an answer adds a hint under the composer and nothing
	// else: it never takes the word away. Writing to the agent while its run is
	// stopped on a decision is exactly what the composer is for, so `writable`
	// keeps looking only at the state of the conversation.
	function renderComposer(active, draft, ui, offered, awaiting) {
		const writable = active && offered !== false;
		const disabled = ui.busy || !writable ? " disabled" : "";
		const placeholder = writable
			? "Write to the agent…"
			: active
				? "This conversation takes no more messages: close it to let the agent go"
				: "This conversation is over and takes no more messages";
		const close = active ? renderCloseControl(ui) : "";
		const hint = writable ? "⌘ + enter to send" : "read only";
		const await_ = awaiting
			? '<p class="conv-composer-await">the run resumes when you answer the wait above</p>'
			: "";
		return `<form class="conv-composer">
			<textarea class="conv-composer-input" rows="2" placeholder="${escapeHtml(placeholder)}"${disabled}>${escapeHtml(draft)}</textarea>
			<div class="conv-composer-row">
				${close}
				<span class="conv-composer-spacer"></span>
				<span class="conv-composer-hint">${escapeHtml(hint)}</span>
				<button type="submit" class="primary-btn"${disabled}>Send</button>
			</div>
			${await_}
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

	// The pending proposal: what the agent says it *would* run, never what it
	// has run. The card names the thing and its target, and — when the server
	// says it can be taken here and now — offers the two answers a person can
	// give. Both carry the id of the proposal, because the caller answers about
	// the exact line it is looking at and not about whatever is pending by the
	// time the answer lands.
	//
	// When it cannot be taken, the server's reason is shown as it is and no
	// confirmation exists at all: an inert button next to a refusal would invite
	// a press that the server has already said it would refuse.
	function renderProposal(proposal, ui) {
		if (!proposal || typeof proposal !== "object") return "";
		const label = textAt(proposal, "label") || textAt(proposal, "action");
		const code = textAt(proposal, "spec_code");
		const title = textAt(proposal, "spec_title");
		const target = [];
		if (code) {
			target.push(`<code class="conv-proposal-code">${escapeHtml(code)}</code>`);
		}
		if (title) {
			target.push(`<span class="conv-proposal-title">${escapeHtml(title)}</span>`);
		}
		const head = `<div class="conv-proposal-head">
			<span class="conv-proposal-mark">proposed</span>
			<span class="conv-proposal-label">${escapeHtml(label)}</span>
			${target.join("")}
		</div>`;

		if (!proposal.runnable) {
			const rows = [];
			const reason = textAt(proposal, "unavailable_reason");
			if (reason) rows.push(renderNotice("refused", "not possible", reason));
			const unlocked = textAt(proposal, "unlocked_by");
			if (unlocked) rows.push(renderNotice("info", "unlocked by", unlocked));
			return `<div class="conv-proposal is-refused">${head}${rows.join("")}</div>`;
		}

		const disabled = ui && ui.busy ? " disabled" : "";
		const id =
			proposal.event_id === null || proposal.event_id === undefined
				? ""
				: String(proposal.event_id);
		return `<div class="conv-proposal">
			${head}
			<p class="conv-proposal-promise">Nothing has started yet: this is only what the agent would do, and it happens if you confirm it.</p>
			<div class="conv-proposal-controls">
				<button type="button" class="approval-btn allow" data-conversation-proposal-accept data-proposal-id="${escapeHtml(id)}"${disabled}>Confirm</button>
				<button type="button" class="approval-btn deny" data-conversation-proposal-decline data-proposal-id="${escapeHtml(id)}"${disabled}>Refuse</button>
			</div>
		</div>`;
	}

	// What became of the last proposal: the decision in the server's own word,
	// and the thing it was about — carried by the payload because the line that
	// proposed it may well have left the retained history by now.
	//
	// The way to reach what was started exists exactly when the payload carries
	// one: a refused proposal started nothing, so there is nothing to reach, and
	// a control offering to go there would be pointing at a record that does not
	// exist.
	function renderOutcome(outcome, ui) {
		if (!outcome || typeof outcome !== "object") return "";
		const decision = textAt(outcome, "decision");
		const label = textAt(outcome, "label") || textAt(outcome, "action");
		const code = textAt(outcome, "spec_code");
		const parts = [];
		if (decision) {
			parts.push(
				`<span class="conv-outcome-decision">${escapeHtml(decision)}</span>`,
			);
		}
		if (label) {
			parts.push(`<span class="conv-outcome-label">${escapeHtml(label)}</span>`);
		}
		if (code) {
			parts.push(`<code class="conv-outcome-code">${escapeHtml(code)}</code>`);
		}
		let reach = "";
		if (textAt(outcome, "execution_id")) {
			const disabled = ui && ui.busy ? " disabled" : "";
			reach = `<button type="button" class="ghost-btn conv-outcome-reach" data-conversation-reach-run data-scope="${escapeHtml(textAt(outcome, "scope"))}" data-code="${escapeHtml(code)}"${disabled}>Go to the run</button>`;
		}
		return `<div class="conv-outcome">
			<div class="conv-outcome-body">${parts.join("")}</div>
			${reach}
		</div>`;
	}

	// ---- public API ----

	/**
	 * Render the conversation view to an HTML string.
	 *
	 * Never throws on a partial payload: a null view, a missing conversation, a
	 * missing events list are all answers, and each has a rendering.
	 *
	 * @param {object|null} view  The /api/workspace/conversations/{id} payload.
	 * @param {string} draft      The text being typed, preserved across renders.
	 * @param {object} [ui]       Local, non-payload state of the panel:
	 *                            {busy, closeArmed, refusal, link,
	 *                            answeredApprovals, highlightAnchor}.
	 *                            answeredApprovals maps an approval id to the
	 *                            answer already given for it
	 *                            ({optionID, label, denied, executionID,
	 *                            approval}), so a decision stays readable once
	 *                            the provider stops listing it as pending: the
	 *                            kept approval is what the resolved card is
	 *                            drawn from, and executionID names the run it
	 *                            belongs to.
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
		// The proposal and its outcome sit between the notices and the timeline:
		// both are about now, and now must be readable without scrolling a
		// history that grows under it. Each is drawn only when the payload
		// carries it — nothing pending and nothing decided are answers, and
		// neither has a card.
		const proposal = objectAt(value, "proposal");
		if (proposal) blocks.push(renderProposal(proposal, local));
		const outcome = objectAt(value, "outcome");
		if (outcome) blocks.push(renderOutcome(outcome, local));
		blocks.push(renderTimeline(value, active, local));
		blocks.push(
			renderComposer(active, typed, local, offered, anyRunAwaiting(value)),
		);
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
