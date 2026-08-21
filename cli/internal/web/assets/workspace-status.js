// workspace-status.js
// Pure renderer for the GET /api/workspace/status payload.
//
// The module contains no process rules: not one stage id, not one action id,
// not one ordering of the steps. Every word it draws about the process comes
// from the payload, so a stage or an action identifier it has never seen is
// rendered as it is, unchanged. Changing the Archetipo therefore changes what
// this module draws without a line changing here.
//
// It is pure: no DOM, no fetch, no document. It takes a view object and
// returns an HTML string, and it decides whether and what the recommended step
// starts — never starting it. Wiring that string into the page, and performing
// the start it names, belong to the caller.
//
// Consumable in both browser (defines window.WorkspaceStatus) and Node
// (exports renderWorkspaceStatus / nextStepTarget / nextStepDispatch /
// escapeHtml).
(function () {
	// ---- internal helpers ----

	/**
	 * Escape the five characters that would otherwise let payload text break
	 * out of the markup it is interpolated into. Behaviourally identical to
	 * the escapeHtml of app.js.
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

	/** Return the object at key, or an empty object: partial payloads must not throw. */
	function objectAt(value, key) {
		const found = value && typeof value === "object" ? value[key] : null;
		return found && typeof found === "object" ? found : {};
	}

	/**
	 * The condition that unlocks a refused step, in the payload's own words:
	 * unlocked_by when the server states one, otherwise the refusal itself.
	 * Never a sentence written here.
	 */
	function unlockText(entry) {
		if (!entry || typeof entry !== "object") return "";
		const unlocked = entry.unlocked_by;
		if (typeof unlocked === "string" && unlocked.trim()) return unlocked;
		const reason = entry.unavailable_reason;
		return typeof reason === "string" ? reason : "";
	}

	function renderTemplateIdentity(template) {
		const name = template.label || template.id;
		const parts = [];
		if (name) parts.push(escapeHtml(name));
		if (template.version) parts.push(escapeHtml(template.version));
		if (!parts.length) return "";
		return `<div class="ws-status-template">${parts.join(" &middot; ")}</div>`;
	}

	function renderNextStep(step, stage) {
		if (!step || typeof step !== "object") {
			// Nothing pending: the stage summary is the whole answer, and there
			// is no button because there is no step to press.
			return `<div class="ws-status-next-none">${escapeHtml(stage.summary)}</div>`;
		}
		const spec = step.spec && typeof step.spec === "object" ? step.spec : null;
		const label = escapeHtml(step.label || step.action);
		const code = spec ? escapeHtml(spec.code) : "";
		const body = code
			? `<span class="ws-status-next-label">${label}</span> <code class="ws-status-next-spec">${code}</code>`
			: `<span class="ws-status-next-label">${label}</span>`;
		const attrs =
			` data-next-scope="${escapeHtml(step.scope)}"` +
			` data-next-action="${escapeHtml(step.action)}"` +
			` data-next-spec="${code}"`;
		const disabled = step.runnable ? "" : " disabled";
		let html = `<button type="button" class="ws-status-next"${attrs}${disabled}>${body}</button>`;
		if (!step.runnable) {
			const unlock = unlockText(step);
			if (unlock) {
				html += `<span class="ws-status-next-unlock">${escapeHtml(unlock)}</span>`;
			}
		}
		return html;
	}

	function renderActionChip(action) {
		if (!action || typeof action !== "object") return "";
		const id = escapeHtml(action.id);
		const label = escapeHtml(action.label || action.id);
		let body = `<span class="action-chip-label">${label}</span>`;
		if (!action.runnable) {
			const unlock = unlockText(action);
			if (unlock) {
				body += `<span class="action-chip-unlock">${escapeHtml(unlock)}</span>`;
			}
		}
		// Every strip chip is a span, runnable or not: the strip states what the
		// workspace allows, it does not dispatch. A button here would be focusable
		// and announced as pressable while nothing listens to it — the inert
		// control the spec forbids, only with the sign reversed. The one control
		// of the strip is the recommended step.
		return `<span class="action-chip ws-status-action" data-action-id="${id}">${body}</span>`;
	}

	// ---- public API ----

	/**
	 * Render the workspace status view to an HTML string.
	 *
	 * Never throws on a partial payload: a null view, a missing next_step and a
	 * missing actions list are all answers, and each has a rendering.
	 *
	 * @param {object|null} view  The /api/workspace/status payload.
	 * @returns {string} HTML string.
	 */
	function renderWorkspaceStatus(view) {
		const value = view && typeof view === "object" ? view : {};
		const stage = objectAt(value, "stage");
		const template = objectAt(value, "template");
		const actions = Array.isArray(value.actions) ? value.actions : [];

		const header =
			`<div class="ws-status-stage">${escapeHtml(stage.label || stage.id)}</div>` +
			`<div class="ws-status-summary">${escapeHtml(stage.summary)}</div>` +
			renderTemplateIdentity(template);

		const next = renderNextStep(value.next_step, stage);
		const chips = actions
			.map(renderActionChip)
			.filter(Boolean)
			.join("");
		const actionsHtml = chips
			? `<div class="ws-status-actions">${chips}</div>`
			: "";

		return (
			`<div class="ws-status-header">${header}</div>` +
			`<div class="ws-status-next-wrap">${next}</div>` +
			actionsHtml
		);
	}

	/**
	 * The target of the recommended step, or null when there is none.
	 * It is the whole decision a caller needs to act on the step — which scope,
	 * which action, on which spec — kept here so it can be tested without a DOM.
	 *
	 * @param {object|null} view  The /api/workspace/status payload.
	 * @returns {{scope: string, action: string, code: string, runnable: boolean}|null}
	 */
	function nextStepTarget(view) {
		const value = view && typeof view === "object" ? view : {};
		const step = value.next_step;
		if (!step || typeof step !== "object" || !step.action) return null;
		const spec = step.spec && typeof step.spec === "object" ? step.spec : null;
		return {
			scope: step.scope || "",
			action: step.action,
			code: spec && spec.code ? spec.code : "",
			runnable: step.runnable === true,
		};
	}

	/**
	 * What pressing the recommended step starts, or null when pressing it must
	 * start nothing.
	 *
	 * The decision lives here, and not in the caller, for the reason the rest of
	 * this module lives here: it is the one layer of the frontend this project
	 * can test without a DOM. And the refusal to start a blocked step has to be
	 * a decision, not only a disabled attribute on the markup: an attribute is
	 * a hint to the pointer, while this is the answer given to anyone who
	 * reaches the handler by any route.
	 *
	 * Still no process rule: the action it returns is the payload's own string,
	 * whatever it is.
	 *
	 * @param {object|null} view  The /api/workspace/status payload.
	 * @returns {{scope: string, action: string, code: string}|null}
	 */
	function nextStepDispatch(view) {
		const target = nextStepTarget(view);
		if (!target) return null;
		if (!target.runnable) return null;
		// A spec-scoped step with no spec names nothing to act on.
		if (target.scope === "spec" && !target.code) return null;
		return { scope: target.scope, action: target.action, code: target.code };
	}

	// ---- exports ----

	if (typeof module !== "undefined" && module.exports) {
		module.exports = {
			renderWorkspaceStatus,
			nextStepTarget,
			nextStepDispatch,
			escapeHtml,
		};
	} else {
		window.WorkspaceStatus = {
			renderWorkspaceStatus,
			nextStepTarget,
			nextStepDispatch,
		};
	}
})();
