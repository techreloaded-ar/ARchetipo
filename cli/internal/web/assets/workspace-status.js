// workspace-status.js
// Pure decisions on the recommended step of the GET /api/workspace/status
// payload.
//
// The module draws nothing: the recommended step is drawn at the tail of the
// thread, by conversation.js. What is left here is the one thing that must be
// decided rather than rendered — whether pressing that step starts anything,
// and on what — kept in a module this project can test without a DOM.
//
// It contains no process rules: not one stage id, not one action id, not one
// ordering of the steps. The action it names is the payload's own string,
// whatever it is, so an action identifier it has never seen travels through
// untouched.
//
// It is pure: no DOM, no fetch, no document. It takes a view object and
// decides whether and what the recommended step starts — never starting it.
// Performing the start it names belongs to the caller.
//
// Consumable in both browser (defines window.WorkspaceStatus) and Node
// (exports nextStepTarget / nextStepDispatch).
(function () {
	// ---- public API ----

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
			nextStepTarget,
			nextStepDispatch,
		};
	} else {
		window.WorkspaceStatus = {
			nextStepTarget,
			nextStepDispatch,
		};
	}
})();
