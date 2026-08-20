// biome-ignore lint/suspicious/noInnerHtml: safe — all data goes through escapeHtml() or marked.parse()
(() => {
	// DOM element ids are kept with their original "story-" naming for HTML
	// and CSS stability. The data model exposed by the API is "spec", which
	// is reflected in variable names, payloads and envelope keys.
	const boardEl = document.getElementById("board");
	const refreshBtn = document.getElementById("refresh-btn");
	const modal = document.getElementById("modal-root");
	const modalClose = document.getElementById("modal-close");
	const modalTitle = document.getElementById("story-editor-title");
	const tabs = modal.querySelectorAll(".tab");
	const panels = modal.querySelectorAll(".tab-panel");
	const specForm = document.getElementById("story-form");
	const planForm = document.getElementById("plan-form");
	const specStatus = document.getElementById("story-status");
	const planStatus = document.getElementById("plan-status");
	const specView = document.getElementById("story-view");
	const specViewMeta = document.getElementById("story-view-meta");
	const specViewTitle = document.getElementById("story-view-title");
	const specBodyView = document.getElementById("story-body-view");
	const specDeleteBtn = document.getElementById("story-delete-btn");
	const specEditBtn = document.getElementById("story-edit-btn");
	const specCancelBtn = document.getElementById("story-cancel-btn");
	const planView = document.getElementById("plan-view");
	const planBodyView = document.getElementById("plan-body-view");
	const planTasksView = document.getElementById("plan-tasks-view");
	const planEditBtn = document.getElementById("plan-edit-btn");
	const planCancelBtn = document.getElementById("plan-cancel-btn");
	const tasksTbody = document.getElementById("tasks-tbody");
	const addTaskBtn = document.getElementById("add-task-btn");
	const toast = document.getElementById("toast");
	const prdBtn = document.getElementById("prd-btn");
	const prdModal = document.getElementById("prd-modal");
	const prdModalClose = document.getElementById("prd-modal-close");
	const prdView = document.getElementById("prd-view");
	const prdInception = document.getElementById("prd-inception");
	const inceptionActions = document.getElementById("inception-actions");
	const inceptionExecution = document.getElementById("inception-execution");
	const inceptionRun = document.getElementById("inception-run");
	const prdBodyView = document.getElementById("prd-body-view");
	const prdEditBtn = document.getElementById("prd-edit-btn");
	const prdCancelBtn = document.getElementById("prd-cancel-btn");
	const prdForm = document.getElementById("prd-form");
	const prdStatus = document.getElementById("prd-status");
	const metricsBtn = document.getElementById("metrics-btn");
	const metricsModal = document.getElementById("metrics-modal");
	const metricsModalClose = document.getElementById("metrics-modal-close");
	const metricsBody = document.getElementById("metrics-body");
	const metricsStatus = document.getElementById("metrics-status");
	const newSpecBtn = document.getElementById("new-spec-btn");
	const newSpecModal = document.getElementById("new-spec-modal");
	const newSpecModalClose = document.getElementById("new-spec-modal-close");
	const newSpecForm = document.getElementById("new-spec-form");
	const newSpecStatus = document.getElementById("new-spec-status");
	const newSpecSubmit = document.getElementById("new-spec-submit");
	const newSpecCancel = document.getElementById("new-spec-cancel");
	const newSpecNoEpics = document.getElementById("new-spec-no-epics");
	const newWorkspaceBtn = document.getElementById("new-workspace-btn");
	const newWorkspaceModal = document.getElementById("new-workspace-modal");
	const newWorkspaceModalClose = document.getElementById(
		"new-workspace-modal-close",
	);
	const newWorkspaceForm = document.getElementById("new-workspace-form");
	const newWorkspaceStatus = document.getElementById("new-workspace-status");
	const newWorkspaceSubmit = document.getElementById("new-workspace-submit");
	const newWorkspaceCancel = document.getElementById("new-workspace-cancel");
	const newWorkspaceTools = document.getElementById("new-workspace-tools");
	const newWorkspaceTemplate = document.getElementById(
		"new-workspace-template",
	);
	const newWorkspaceUnavailable = document.getElementById(
		"new-workspace-unavailable",
	);
	const newWorkspaceUnavailableText = document.getElementById(
		"new-workspace-unavailable-text",
	);
	const newWorkspaceWorktreeEnabled = document.getElementById(
		"new-workspace-worktree-enabled",
	);
	const configBtn = document.getElementById("config-btn");
	const configModal = document.getElementById("config-modal");
	const configModalClose = document.getElementById("config-modal-close");
	const configTabs = configModal.querySelectorAll("[data-config-tab]");
	const configPanels = configModal.querySelectorAll("[data-config-panel]");
	const configPath = document.getElementById("config-path");
	const configRestartNotice = document.getElementById("config-restart-notice");
	const configGuidedForm = document.getElementById("config-guided-form");
	const configRaw = document.getElementById("config-raw");
	const configStatus = document.getElementById("config-status");
	const configValidateBtn = document.getElementById("config-validate-btn");
	const configSaveBtn = document.getElementById("config-save-btn");
	const configCancelBtn = document.getElementById("config-cancel-btn");
	const configSummaryConnector = document.getElementById("config-summary-connector");
	const configSummaryExists = document.getElementById("config-summary-exists");
	const configValidation = document.getElementById("config-validation");
	const configConnectorGrid = document.getElementById("config-connector-grid");
	const configSummaryProvider = document.getElementById("config-summary-provider");
	const executionProviderGrid = document.getElementById("execution-provider-grid");
	const executionFields = document.getElementById("execution-fields");
	const executionSaveBtn = document.getElementById("execution-save-btn");
	const executionStatus = document.getElementById("execution-status");
	const storyActions = document.getElementById("story-actions");
	const storyExecution = document.getElementById("story-execution");
	const storyRun = document.getElementById("story-run");
	const mockupsBtn = document.getElementById("mockups-btn");
	const mockupsMenu = document.getElementById("mockups-menu");
	const mockupsDropdown = document.getElementById("mockups-dropdown");
	const themeToggle = document.getElementById("theme-toggle");
	const statTotal = document.getElementById("stat-total");
	const statProgress = document.getElementById("stat-progress");
	const statDone = document.getElementById("stat-done");
	const reviewTab = document.getElementById("review-tab");
	const reviewBranch = document.getElementById("review-branch");
	const reviewDiff = document.getElementById("review-diff");
	const reviewStatus = document.getElementById("review-status");
	const reviewRequestBtn = document.getElementById("review-request-btn");
	const reviewIntegrateBtn = document.getElementById("review-integrate-btn");

	const THEME_KEY = "archetipo.theme";

	function setTheme(theme, persist) {
		const next = theme === "light" ? "light" : "dark";
		document.documentElement.dataset.theme = next;
		themeToggle.setAttribute(
			"aria-label",
			next === "dark" ? "Switch to light theme" : "Switch to dark theme",
		);
		if (persist) {
			try {
				localStorage.setItem(THEME_KEY, next);
			} catch (_) {
				/* ignore */
			}
		}
	}

	function toggleTheme() {
		const current =
			document.documentElement.dataset.theme === "light" ? "light" : "dark";
		setTheme(current === "dark" ? "light" : "dark", true);
	}

	setTheme(document.documentElement.dataset.theme, false);
	themeToggle.addEventListener("click", toggleTheme);

	const editorToolbar = [
		"bold",
		"italic",
		"heading",
		"|",
		"unordered-list",
		"ordered-list",
		"quote",
		"code",
		"|",
		"link",
		"image",
		"|",
		"preview",
		"side-by-side",
		"fullscreen",
		"|",
		"guide",
	];
	const specEditor = new EasyMDE({
		element: specForm.body,
		spellChecker: false,
		status: false,
		autoDownloadFontAwesome: true,
		previewRender: (plainText) => marked.parse(plainText),
		toolbar: editorToolbar,
		minHeight: "320px",
	});
	const planEditor = new EasyMDE({
		element: planForm.plan_body,
		spellChecker: false,
		status: false,
		autoDownloadFontAwesome: true,
		previewRender: (plainText) => marked.parse(plainText),
		toolbar: editorToolbar,
		minHeight: "240px",
	});
	const prdEditor = new EasyMDE({
		element: prdForm.prd_body,
		spellChecker: false,
		status: false,
		autoDownloadFontAwesome: true,
		previewRender: (plainText) => marked.parse(plainText),
		toolbar: editorToolbar,
		minHeight: "360px",
	});
	const newSpecEditor = new EasyMDE({
		element: newSpecForm.body,
		spellChecker: false,
		status: false,
		autoDownloadFontAwesome: true,
		previewRender: (plainText) => marked.parse(plainText),
		toolbar: editorToolbar,
		minHeight: "300px",
	});

	let currentSpecCode = null;
	let reviewComments = []; // inline comments for the spec currently under review
	let reviewLoaded = false; // whether the review tab has been loaded for this spec
	let currentSpecSnapshot = null; // last loaded spec (for cancel + re-render after save)
	let currentPlanSnapshot = null; // last loaded plan (for cancel + re-render after save)
	let boardSnapshot = null; // last loaded board (for undo on failed drag)
	let currentPrdSnapshot = ""; // last loaded PRD body
	// Client-side guard against a double submit of the creation form. It keeps
	// the user from firing twice; the server, not this flag, is what guarantees
	// that a repeated confirmation never creates a second spec.
	let newSpecBusy = false;
	let newWorkspaceBusy = false;
	let currentConfigSnapshot = null; // last loaded effective config
	let currentConfigRaw = ""; // last loaded/saved config YAML
	let currentConfigExists = false; // whether config.yaml existed on open
	let activeConfigTab = "guided";
	let mockupsCache = []; // cached list of mockups (refreshed lazily)
	let executionProviders = []; // providers offered by the server, in its order
	let executionDefault = null; // persisted workspace default, or null
	// The action/execution/run trio is one panel that can be mounted on more than
	// one place: the spec detail, or the PRD modal for a workspace-scoped action.
	// What it is following is a single context token — `spec:US-XXX` or
	// `workspace` — and every asynchronous continuation checks that token
	// instead of the open spec code, so leaving one panel stops the timers of
	// that panel and of no other. The workspace token names the workspace panel
	// as a whole rather than one of its actions: only one workspace execution
	// can be open at a time, so a single token is enough for all of them.
	const WORKSPACE_CONTEXT = "workspace";
	let panelContext = null; // context the panel is mounted on, or null
	let panelActions = null; // container the action chips are drawn in
	let panelExecution = null; // container the execution panel is drawn in
	let panelRun = null; // container the run panel is drawn in
	let panelStartURL = ""; // route that starts an action in this context
	let panelSettle = null; // what to do when the execution reaches a terminal state
	let executionPollTimer = null; // interval following the open spec's execution
	let lastExecutionRecord = null; // execution shown in the panel, for a failed poll
	let runPollTimer = null; // interval following the run behind the open execution
	let runExecutionID = null; // execution whose run the panel is following
	let runAfterID = 0; // highest event id already rendered — the only cursor
	let runEvents = []; // timeline, appended to and never rebuilt
	let runSnapshot = null; // run identity and state, exactly as the server reports it
	let runApprovals = []; // approvals the run is waiting on, verbatim from the provider
	let runPendingMessage = ""; // text delivered to the run and not yet confirmed by it
	let runNotice = ""; // server-side note about the projection (window, transport)
	let runConnected = true; // whether the server is currently attached to the run
	let runTruncated = false; // whether older history fell outside the retained window
	let runRefusal = ""; // last refused command, shown inline until the next one
	let runOutcome = ""; // outcome of the last accepted command
	let runDraft = ""; // composer text, preserved across re-renders
	let runBusy = false; // a command is in flight: the controls stay disabled
	let runCancelArmed = false; // the inline cancel confirmation is showing
	let runPollAbandoned = false; // the client gave up reading: it is not reconnecting
	let runPollBusy = false; // a poll is in flight: ticks never overlap
	let runPollFailures = 0; // consecutive failed reads, for the give-up threshold
	let runAnswered = null; // approval answered from here, kept as its resolved card
	let runCancelSent = false; // a cancel was delivered — a fact about the command, not the run
	let runSeams = new Set(); // event ids the timeline resumed at after a dropped channel
	let runSeamPending = false; // the channel dropped: the next appended event opens a seam

	refreshBtn.addEventListener("click", loadBoard);
	modalClose.addEventListener("click", closeModal);
	modal.addEventListener("click", (e) => {
		if (e.target === modal) closeModal();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !modal.classList.contains("hidden")) closeModal();
	});
	tabs.forEach((t) =>
		t.addEventListener("click", () => activateTab(t.dataset.tab)),
	);
	specForm.addEventListener("submit", onSaveSpec);
	planForm.addEventListener("submit", onSavePlan);
	specEditBtn.addEventListener("click", () => enterSpecEditMode());
	specDeleteBtn.addEventListener("click", () => {
		if (currentSpecCode)
			confirmAndDeleteSpec(
				currentSpecCode,
				currentSpecSnapshot && currentSpecSnapshot.title,
			);
	});
	specCancelBtn.addEventListener("click", () => exitSpecEditMode());
	planEditBtn.addEventListener("click", () => enterPlanEditMode());
	planCancelBtn.addEventListener("click", () => exitPlanEditMode());
	addTaskBtn.addEventListener("click", () => addTaskRow());
	reviewRequestBtn.addEventListener("click", onRequestChanges);
	reviewIntegrateBtn.addEventListener("click", onIntegrate);
	// The action chips are re-rendered on every open, so the handler lives on
	// their container instead of on buttons that no longer exist. Every
	// container the panel can be mounted on is bound once, here: which of them
	// is live at any moment is decided by the mounted context, not by the
	// listeners.
	bindActionsPanel(storyActions);
	bindActionsPanel(inceptionActions);
	bindRunPanel(storyRun);
	bindRunPanel(inceptionRun);

	function bindActionsPanel(container) {
		container.addEventListener("click", (e) => {
			const btn = e.target.closest(".action-chip-run");
			if (!btn) return;
			startPanelAction(btn.dataset.actionId, btn);
		});
	}

	// The run panel is redrawn on every poll, so its controls cannot own their
	// handlers: the container does, and each control declares what it is through
	// its class and data attributes.
	function bindRunPanel(container) {
		container.addEventListener("click", (e) => {
			const option = e.target.closest("[data-option-id]");
			if (option) {
				respondRunApproval(option.dataset.approvalId, option.dataset.optionId);
				return;
			}
			if (e.target.closest("[data-cancel-open]")) {
				runCancelArmed = true;
				renderRun();
				return;
			}
			if (e.target.closest("[data-cancel-abort]")) {
				runCancelArmed = false;
				renderRun();
				return;
			}
			if (e.target.closest("[data-cancel-confirm]")) {
				cancelRun();
				return;
			}
			const dismiss = e.target.closest("[data-notice-dismiss]");
			if (dismiss) dismissRunNotice(dismiss.dataset.noticeDismiss);
		});
		container.addEventListener("submit", (e) => {
			const form = e.target.closest(".run-composer");
			if (!form) return;
			e.preventDefault();
			sendRunMessage();
		});
		container.addEventListener("input", (e) => {
			const input = e.target.closest(".run-composer-input");
			if (input) runDraft = input.value;
		});
		container.addEventListener("keydown", (e) => {
			const input = e.target.closest(".run-composer-input");
			if (!input) return;
			if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
				e.preventDefault();
				runDraft = input.value;
				sendRunMessage();
			}
		});
	}

	prdBtn.addEventListener("click", openPRD);
	prdModalClose.addEventListener("click", closePRD);
	prdModal.addEventListener("click", (e) => {
		if (e.target === prdModal) closePRD();
	});
	prdEditBtn.addEventListener("click", enterPrdEditMode);
	prdCancelBtn.addEventListener("click", exitPrdEditMode);
	prdForm.addEventListener("submit", onSavePRD);
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !prdModal.classList.contains("hidden"))
			closePRD();
	});

	metricsBtn.addEventListener("click", openMetrics);
	metricsModalClose.addEventListener("click", closeMetrics);
	metricsModal.addEventListener("click", (e) => {
		if (e.target === metricsModal) closeMetrics();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !metricsModal.classList.contains("hidden"))
			closeMetrics();
	});

	newSpecBtn.addEventListener("click", openNewSpec);
	newSpecModalClose.addEventListener("click", closeNewSpec);
	newSpecCancel.addEventListener("click", closeNewSpec);
	newSpecModal.addEventListener("click", (e) => {
		if (e.target === newSpecModal) closeNewSpec();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !newSpecModal.classList.contains("hidden"))
			closeNewSpec();
	});
	newSpecForm.addEventListener("submit", onCreateSpec);

	newWorkspaceBtn.addEventListener("click", openNewWorkspace);
	newWorkspaceModalClose.addEventListener("click", closeNewWorkspace);
	newWorkspaceCancel.addEventListener("click", closeNewWorkspace);
	newWorkspaceModal.addEventListener("click", (e) => {
		if (e.target === newWorkspaceModal) closeNewWorkspace();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !newWorkspaceModal.classList.contains("hidden"))
			closeNewWorkspace();
	});
	newWorkspaceWorktreeEnabled.addEventListener("change", syncWorktreeFields);
	newWorkspaceForm.addEventListener("submit", onCreateWorkspace);

	configBtn.addEventListener("click", openConfig);
	configModalClose.addEventListener("click", closeConfig);
	configCancelBtn.addEventListener("click", closeConfig);
	configValidateBtn.addEventListener("click", validateConfig);
	configSaveBtn.addEventListener("click", saveConfig);
	configModal.addEventListener("click", (e) => {
		if (e.target === configModal) closeConfig();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !configModal.classList.contains("hidden"))
			closeConfig();
	});
	configTabs.forEach((tab) =>
		tab.addEventListener("click", () => activateConfigTab(tab.dataset.configTab)),
	);
	configConnectorGrid.addEventListener("change", updateConnectorSections);
	executionProviderGrid.addEventListener("change", onExecutionProviderSelected);
	executionSaveBtn.addEventListener("click", saveExecutionProvider);

	mockupsBtn.addEventListener("click", toggleMockupsMenu);
	document.addEventListener("click", (e) => {
		if (!mockupsDropdown.contains(e.target))
			mockupsMenu.classList.add("hidden");
	});

	// Global single-key shortcuts (ignored while typing in inputs / editors).
	document.addEventListener("keydown", (e) => {
		if (e.metaKey || e.ctrlKey || e.altKey) return;
		const tag = (e.target && e.target.tagName) || "";
		if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
		if (e.target && e.target.isContentEditable) return;
		const k = e.key.toLowerCase();
		if (k === "t") {
			e.preventDefault();
			toggleTheme();
		} else if (k === "r") {
			e.preventDefault();
			loadBoard();
		}
	});

	loadBoard();
	loadMockups();
	connectBoardStream();

	let boardReloadTimer = null;
	function scheduleBoardReload() {
		clearTimeout(boardReloadTimer);
		boardReloadTimer = setTimeout(() => {
			// Skip while a modal is open: reloading would discard the user's
			// in-progress edits. The next event after the modal closes will
			// bring the board back in sync.
			if (!modal.classList.contains("hidden")) return;
			if (!prdModal.classList.contains("hidden")) return;
			if (!configModal.classList.contains("hidden")) return;
			if (!newSpecModal.classList.contains("hidden")) return;
			if (!newWorkspaceModal.classList.contains("hidden")) return;
			loadBoard();
		}, 150);
	}

	function connectBoardStream() {
		if (typeof EventSource === "undefined") return;
		const es = new EventSource("/api/board/stream");
		es.addEventListener("board_changed", scheduleBoardReload);
		// EventSource reconnects automatically on transient errors; no log to avoid noise.
	}

	async function loadBoard() {
		boardEl.innerHTML = '<div class="empty-board">Loading…</div>';
		try {
			const view = await apiGet("/api/board");
			renderBoard(view);
			updateStats(view);
			boardSnapshot = view;
			// A workspace without epics has no admissible value for a new spec:
			// the action is switched off at the source instead of opening a
			// modal that can only explain why it cannot be used.
			const hasEpics = (view.epics || []).length > 0;
			newSpecBtn.disabled = !hasEpics;
			newSpecBtn.title = hasEpics
				? ""
				: "Define at least one epic in the backlog before creating a spec";
		} catch (err) {
			// A board that could not be read cannot vouch for its epics either.
			newSpecBtn.disabled = true;
			newSpecBtn.title = "The backlog could not be read";
			boardEl.innerHTML = `<div class="empty-board">Error: ${escapeHtml(err.message || err)}</div>`;
		}
	}

	function updateStats(view) {
		const cols = view.columns || [];
		let total = 0,
			progress = 0,
			done = 0;
		cols.forEach((c) => {
			const n = (c.specs || []).length;
			total += n;
			if (c.id === "in_progress" || c.id === "review") progress += n;
			if (c.id === "done") done += n;
		});
		if (statTotal) statTotal.textContent = total;
		if (statProgress) statProgress.textContent = progress;
		if (statDone) statDone.textContent = done;
	}

	function renderBoard(view) {
		boardEl.innerHTML = "";
		if (!view.columns || view.columns.length === 0) {
			boardEl.innerHTML =
				'<div class="empty-board">No backlog yet — run <code>archetipo init</code> to begin.</div>';
			return;
		}
		view.columns.forEach((col) => {
			const columnEl = document.createElement("section");
			columnEl.className = "column";
			columnEl.dataset.id = col.id;
			columnEl.dataset.status = col.status;

			const header = document.createElement("header");
			header.className = "column-header";
			const count = (col.specs || []).length;
			header.innerHTML = `
                <span class="column-title"><span class="column-dot"></span>${escapeHtml(col.title || col.id)}</span>
                <span class="column-count">${count}</span>
            `;
			columnEl.appendChild(header);

			const body = document.createElement("div");
			body.className = "column-body";
			body.dataset.columnId = col.id;
			(col.specs || []).forEach((s) => body.appendChild(renderCard(s)));
			if (!col.specs || col.specs.length === 0) {
				body.appendChild(emptyHint(col.id));
			}
			columnEl.appendChild(body);
			boardEl.appendChild(columnEl);

			createBoardSortable(body, col.id);
		});
	}

	function createBoardSortable(body, columnId) {
		if (columnId === "review") {
			Sortable.create(body, {
				group: { name: "review-approval", pull: true, put: false },
				sort: false,
				animation: 140,
				ghostClass: "sortable-ghost",
				dragClass: "sortable-drag",
			});
			return;
		}
		if (columnId === "done") {
			Sortable.create(body, {
				group: {
					name: "review-approval",
					pull: false,
					put: ["review-approval"],
				},
				draggable: ".done-drop-target-disabled",
				sort: false,
				animation: 140,
				ghostClass: "sortable-ghost",
				dragClass: "sortable-drag",
				onAdd: onDragMove,
			});
		}
	}

	function renderCard(spec) {
		const el = document.createElement("article");
		el.className = "card";
		if (spec.priority) el.classList.add("prio-" + spec.priority);
		el.dataset.code = spec.code;
		const epicCode = spec.epic && spec.epic.code ? spec.epic.code : "";
		const epicTooltip =
			spec.epic && spec.epic.title
				? `${epicCode} — ${spec.epic.title}`
				: epicCode;
		el.innerHTML = `
            <button type="button" class="card-delete-btn" title="Delete ${escapeHtml(spec.code)}" aria-label="Delete ${escapeHtml(spec.code)}">
                <svg width="13" height="13" viewBox="0 0 16 16" aria-hidden="true"><path d="M3.5 4.5h9" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M6 2.5h4" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M5 4.5v8h6v-8" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M6.75 6.5v4.25M9.25 6.5v4.25" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>
            </button>
            <div class="card-top">
                <span class="card-code">${escapeHtml(spec.code)}</span>
                ${spec.rework ? `<span class="rework-badge" title="In rework: review feedback waiting to be re-planned">⟲ rework</span>` : ""}
                ${spec.priority ? `<span class="priority-badge priority-${escapeHtml(spec.priority)}">${escapeHtml(spec.priority)}</span>` : ""}
            </div>
            <div class="card-title">${escapeHtml(spec.title || "(untitled)")}</div>
            <div class="card-meta">
                <span class="card-epic" title="${escapeHtml(epicTooltip)}">${escapeHtml(epicCode)}</span>
                <span class="card-points">${Number.isFinite(spec.points) ? spec.points + " pt" : ""}</span>
            </div>
            ${spec.branch ? `<div class="card-branch" title="git branch">⎇ ${escapeHtml(spec.branch)}</div>` : ""}
        `;
		const deleteBtn = el.querySelector(".card-delete-btn");
		const stopDeleteEvent = (event) => event.stopPropagation();
		["pointerdown", "mousedown", "touchstart"].forEach((type) =>
			deleteBtn.addEventListener(type, stopDeleteEvent),
		);
		deleteBtn.addEventListener("click", async (event) => {
			event.stopPropagation();
			await confirmAndDeleteSpec(spec.code, spec.title);
		});
		el.addEventListener("click", () => openEditor(spec.code));
		return el;
	}

	function emptyHint(columnId) {
		const e = document.createElement("div");
		e.className = "empty-column";
		e.textContent =
			columnId === "done" ? "drop a Review card here to approve" : "no specs";
		return e;
	}

	async function onDragMove(evt) {
		const sourceColumn =
			evt.from && evt.from.dataset ? evt.from.dataset.columnId : "";
		const targetColumn =
			evt.to && evt.to.dataset ? evt.to.dataset.columnId : "";
		if (sourceColumn !== "review" || targetColumn !== "done") {
			showToast("Only Review → Done drag-and-drop is allowed", "err");
			// revert any accidental DOM change by restoring the last known good board.
			if (boardSnapshot) {
				renderBoard(boardSnapshot);
				updateStats(boardSnapshot);
			} else {
				await loadBoard();
			}
			return;
		}

		const code = evt.item.dataset.code;
		// Determine anchor based on the card now next to the dragged item.
		let anchor = {};
		const cards = Array.from(evt.to.querySelectorAll(".card"));
		const idx = cards.findIndex((c) => c === evt.item);
		if (idx === -1) {
			anchor = {};
		} else if (idx < cards.length - 1) {
			anchor = { before: cards[idx + 1].dataset.code };
		} else if (idx > 0) {
			anchor = { after: cards[idx - 1].dataset.code };
		}
		try {
			await apiPost("/api/board/move", { code, to: targetColumn, ...anchor });
			showToast(`${code} approved and moved to ${targetColumn}`, "ok");
			await loadBoard();
		} catch (err) {
			showToast(`Move failed: ${err.message || err}`, "err");
			// revert the optimistic DOM change by reloading the last known good board.
			if (boardSnapshot) {
				renderBoard(boardSnapshot);
				updateStats(boardSnapshot);
			} else {
				await loadBoard();
			}
		}
	}

	async function openEditor(code) {
		// A timer from the previously open spec must not survive this call: it
		// would keep polling an execution nobody is looking at, and would reload
		// the detail of a spec the user has already left.
		currentSpecCode = code;
		mountExecutionPanels({
			context: specContext(code),
			startURL: `/api/spec/${encodeURIComponent(code)}/execution`,
			actions: storyActions,
			execution: storyExecution,
			run: storyRun,
			settle: settleSpecExecution,
		});
		modalTitle.textContent = `Spec ${code}`;
		modal.classList.remove("hidden");
		activateTab("story");
		specStatus.textContent = "Loading...";
		planStatus.textContent = "";
		showSpecView();
		showPlanView();
		reviewLoaded = false;
		reviewComments = [];
		try {
			const detail = await apiGet(`/api/spec/${encodeURIComponent(code)}`);
			currentSpecSnapshot = detail.spec || {};
			currentPlanSnapshot = {
				plan_body: detail.plan_body || "",
				tasks: detail.tasks || [],
			};
			fillSpecView(currentSpecSnapshot);
			renderSpecActions(detail.actions);
			// The server hands back the last execution of the spec on every read,
			// so a reload finds the run it left behind — and resumes following it
			// without ever starting a second one.
			resumeExecution(detail.execution, specContext(code));
			fillSpecForm(currentSpecSnapshot);
			fillPlanView(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			fillPlanForm(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			updateReviewTabVisibility(currentSpecSnapshot);
			specStatus.textContent = "";
		} catch (err) {
			specStatus.textContent = `Load failed: ${err.message || err}`;
			specStatus.className = "status-msg err";
		}
	}

	function fillSpecView(s) {
		specViewTitle.textContent = s.title || "(untitled)";
		const metaParts = [];
		if (s.priority)
			metaParts.push(
				`<span class="priority-badge priority-${escapeHtml(s.priority)}">${escapeHtml(s.priority)}</span>`,
			);
		if (Number.isFinite(s.points) && s.points > 0)
			metaParts.push(`<span class="meta-chip">${s.points} pt</span>`);
		if (s.epic && s.epic.code) {
			const epicText = s.epic.title
				? `${s.epic.code} — ${s.epic.title}`
				: s.epic.code;
			metaParts.push(`<span class="meta-chip">${escapeHtml(epicText)}</span>`);
		}
		if (s.scope)
			metaParts.push(`<span class="meta-chip">${escapeHtml(s.scope)}</span>`);
		if (s.blocked_by && s.blocked_by.length)
			metaParts.push(
				`<span class="meta-chip blocked">blocked by ${escapeHtml(s.blocked_by.join(", "))}</span>`,
			);
		const mockup = findMockupForSpec(s.code);
		if (mockup)
			metaParts.push(
				`<a class="meta-chip mockup-link" href="${escapeHtml(mockup.url)}" target="_blank" rel="noopener">↗ mockup</a>`,
			);
		specViewMeta.innerHTML = metaParts.join("");
		specDeleteBtn.title = s.code ? `Delete ${s.code}` : "Delete story";
		specDeleteBtn.setAttribute(
			"aria-label",
			s.code ? `Delete ${s.code}` : "Delete story",
		);
		specBodyView.innerHTML = marked.parse(s.body || "*(no description)*");
	}

	function findMockupForSpec(code) {
		if (!code) return null;
		return mockupsCache.find((m) => m.spec_code === code) || null;
	}

	function fillSpecForm(s) {
		specForm.title.value = s.title || "";
		specForm.priority.value = s.priority || "MEDIUM";
		specForm.story_points.value = s.points || 0;
		specForm.scope.value = s.scope || "";
		specForm.blocked_by.value = (s.blocked_by || []).join(", ");
		specEditor.value(s.body || "");
	}

	function fillPlanForm(body, tasks) {
		planEditor.value(body || "");
		tasksTbody.innerHTML = "";
		(tasks || []).forEach((t) => addTaskRow(t));
	}

	function showSpecView() {
		specView.classList.remove("hidden");
		specForm.classList.add("hidden");
	}

	function enterSpecEditMode() {
		specView.classList.add("hidden");
		specForm.classList.remove("hidden");
		specStatus.textContent = "";
		specStatus.className = "status-msg";
		// CodeMirror needs a refresh after being unhidden, otherwise it measures 0 height.
		setTimeout(() => specEditor.codemirror.refresh(), 0);
	}

	function exitSpecEditMode() {
		if (currentSpecSnapshot) fillSpecForm(currentSpecSnapshot);
		showSpecView();
	}

	function getTaskMarkdown(task) {
		const body = typeof task?.body === "string" ? task.body.trim() : "";
		if (body) return body;
		return typeof task?.description === "string" ? task.description.trim() : "";
	}

	function renderTaskTitleCell(task) {
		const title = escapeHtml(task.title || "");
		const taskMarkdown = getTaskMarkdown(task);
		if (!taskMarkdown) {
			return `<div class="task-title-text">${title}</div>`;
		}
		const rendered = TaskMarkdown.renderTaskMarkdown(taskMarkdown);
		return `
                <details class="task-desc-details">
                    <summary><span class="task-title-text">${title}</span></summary>
                    <div class="markdown-rendered task-desc-markdown">${rendered}</div>
                </details>
            `;
	}

	function fillPlanView(body, tasks) {
		planBodyView.innerHTML = marked.parse(body || "*(no plan)*");
		planTasksView.innerHTML = "";
		(tasks || []).forEach((t) => {
			const tr = document.createElement("tr");
			tr.innerHTML = `
                <td>${escapeHtml(t.id || "")}</td>
                <td>${renderTaskTitleCell(t)}</td>
                <td>${escapeHtml(t.type || "")}</td>
                <td>${escapeHtml(t.status || "")}</td>
                <td>${escapeHtml((t.dependencies || []).join(", "))}</td>
            `;
			planTasksView.appendChild(tr);
		});
		if (!tasks || tasks.length === 0) {
			const tr = document.createElement("tr");
			tr.innerHTML = '<td colspan="5" class="empty-cell">No tasks</td>';
			planTasksView.appendChild(tr);
		}
	}

	function showPlanView() {
		planView.classList.remove("hidden");
		planForm.classList.add("hidden");
	}

	function enterPlanEditMode() {
		planView.classList.add("hidden");
		planForm.classList.remove("hidden");
		planStatus.textContent = "";
		planStatus.className = "status-msg";
		setTimeout(() => planEditor.codemirror.refresh(), 0);
	}

	function exitPlanEditMode() {
		if (currentPlanSnapshot)
			fillPlanForm(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
		showPlanView();
	}

	function addTaskRow(task) {
		const t = task || {
			id: nextTaskID(),
			title: "",
			description: "",
			body: "",
			type: "Impl",
			status: "TODO",
			dependencies: [],
		};
		const tr = document.createElement("tr");
		tr.innerHTML = `
            <td><input type="text" class="task-id" value="${escapeHtml(t.id || "")}" /></td>
            <td>
                <input type="text" class="task-title" value="${escapeHtml(t.title || "")}" />
                <textarea class="task-desc" rows="2" placeholder="Task markdown body…">${escapeHtml(getTaskMarkdown(t))}</textarea>
            </td>
            <td>
                <select class="task-type">
                    <option value="Impl">Impl</option>
                    <option value="Test">Test</option>
                </select>
            </td>
            <td>
                <select class="task-status">
                    <option>TODO</option>
                    <option>PLANNED</option>
                    <option>IN PROGRESS</option>
                    <option>REVIEW</option>
                    <option>DONE</option>
                </select>
            </td>
            <td><input type="text" class="task-deps" value="${escapeHtml((t.dependencies || []).join(", "))}" placeholder="TASK-01" /></td>
            <td><button type="button" class="remove-task" aria-label="Remove">&times;</button></td>
        `;
		tr.querySelector(".task-type").value = t.type || "Impl";
		tr.querySelector(".task-status").value = t.status || "TODO";
		tr.querySelector(".remove-task").addEventListener("click", () =>
			tr.remove(),
		);
		tasksTbody.appendChild(tr);
	}

	function nextTaskID() {
		const ids = Array.from(tasksTbody.querySelectorAll(".task-id"))
			.map((i) => parseInt((i.value.match(/(\d+)$/) || [0, 0])[1], 10))
			.filter((n) => Number.isFinite(n));
		const next = (ids.length ? Math.max(...ids) : 0) + 1;
		return "TASK-" + String(next).padStart(2, "0");
	}

	async function onSaveSpec(e) {
		e.preventDefault();
		if (!currentSpecCode) return;
		const blocked = specForm.blocked_by.value
			.split(",")
			.map((s) => s.trim())
			.filter(Boolean);
		const patch = {
			title: specForm.title.value,
			priority: specForm.priority.value,
			points: parseInt(specForm.story_points.value, 10) || 0,
			scope: specForm.scope.value,
			blocked_by: blocked,
			body: specEditor.value(),
		};
		specStatus.textContent = "Saving...";
		specStatus.className = "status-msg";
		try {
			await apiPut(`/api/spec/${encodeURIComponent(currentSpecCode)}`, patch);
			specStatus.textContent = "Saved";
			specStatus.className = "status-msg ok";
			showToast(`${currentSpecCode} updated`, "ok");
			currentSpecSnapshot = { ...(currentSpecSnapshot || {}), ...patch };
			fillSpecView(currentSpecSnapshot);
			showSpecView();
			await loadBoard();
		} catch (err) {
			specStatus.textContent = `Save failed: ${err.message || err}`;
			specStatus.className = "status-msg err";
		}
	}

	async function onSavePlan(e) {
		e.preventDefault();
		if (!currentSpecCode) return;
		const rows = Array.from(tasksTbody.querySelectorAll("tr"));
		const tasks = rows
			.map((tr) => {
				const deps = tr
					.querySelector(".task-deps")
					.value.split(",")
					.map((s) => s.trim())
					.filter(Boolean);
				const descEl = tr.querySelector(".task-desc");
				const taskMarkdownBody = descEl ? descEl.value.trim() : "";
				return {
					id: tr.querySelector(".task-id").value.trim(),
					title: tr.querySelector(".task-title").value.trim(),
					body: taskMarkdownBody,
					description: taskMarkdownBody,
					type: tr.querySelector(".task-type").value,
					status: tr.querySelector(".task-status").value,
					dependencies: deps,
				};
			})
			.filter((t) => t.id !== "");
		const payload = {
			plan_body: planEditor.value(),
			tasks,
		};
		planStatus.textContent = "Saving...";
		planStatus.className = "status-msg";
		try {
			await apiPut(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/plan`,
				payload,
			);
			planStatus.textContent = "Saved";
			planStatus.className = "status-msg ok";
			showToast(`${currentSpecCode} plan updated`, "ok");
			currentPlanSnapshot = {
				plan_body: payload.plan_body,
				tasks: payload.tasks,
			};
			fillPlanView(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			showPlanView();
		} catch (err) {
			planStatus.textContent = `Save failed: ${err.message || err}`;
			planStatus.className = "status-msg err";
		}
	}

	function activateTab(name) {
		tabs.forEach((t) => {
			const active = t.dataset.tab === name;
			t.classList.toggle("active", active);
			t.setAttribute("aria-selected", active ? "true" : "false");
		});
		panels.forEach((p) => {
			p.classList.toggle("active", p.dataset.panel === name);
		});
		// CodeMirror instances mounted inside hidden panels need a refresh once visible.
		if (name === "plan" && !planForm.classList.contains("hidden")) {
			setTimeout(() => planEditor.codemirror.refresh(), 0);
		}
		if (name === "story" && !specForm.classList.contains("hidden")) {
			setTimeout(() => specEditor.codemirror.refresh(), 0);
		}
		if (name === "review" && !reviewLoaded) {
			loadReview();
		}
	}

	function closeModal() {
		modal.classList.add("hidden");
		unmountExecutionPanels(specContext(currentSpecCode));
		currentSpecCode = null;
		currentSpecSnapshot = null;
		currentPlanSnapshot = null;
		reviewComments = [];
		reviewLoaded = false;
		reviewTab.classList.add("hidden");
	}

	function showToast(msg, kind) {
		toast.textContent = msg;
		toast.classList.remove("hidden", "ok", "err");
		if (kind) toast.classList.add(kind);
		clearTimeout(showToast._t);
		showToast._t = setTimeout(() => toast.classList.add("hidden"), 2200);
	}

	// ---- Review (diff + inline comments) -----------------------------------

	function updateReviewTabVisibility(spec) {
		const inReview = spec && spec.status === "REVIEW";
		reviewTab.classList.toggle("hidden", !inReview);
		if (!inReview && reviewTab.classList.contains("active")) {
			activateTab("story");
		}
	}

	async function loadReview() {
		reviewLoaded = true;
		reviewStatus.textContent = "";
		reviewStatus.className = "status-msg";
		reviewDiff.innerHTML = '<div class="review-empty">Loading diff…</div>';
		try {
			const [diff, review] = await Promise.all([
				apiGet(`/api/spec/${encodeURIComponent(currentSpecCode)}/diff`),
				apiGet(`/api/spec/${encodeURIComponent(currentSpecCode)}/review`),
			]);
			reviewComments = (review && review.comments) || [];
			renderReviewBranch(diff);
			renderDiff(diff);
		} catch (err) {
			reviewLoaded = false;
			reviewDiff.innerHTML = `<div class="review-empty">Error: ${escapeHtml(err.message || err)}</div>`;
		}
	}

	function renderReviewBranch(diff) {
		const parts = [];
		if (diff.branch)
			parts.push(
				`<span class="review-chip">⎇ ${escapeHtml(diff.branch)}</span>`,
			);
		parts.push(
			`<span class="review-chip">base ${escapeHtml(diff.base || "")}</span>`,
		);
		if (diff.branch)
			parts.push(
				`<span class="review-chip">+${diff.ahead || 0} / −${diff.behind || 0}</span>`,
			);
		reviewBranch.innerHTML = parts.join("");
	}

	function renderDiff(diff) {
		reviewDiff.innerHTML = "";
		const files = diff.files || [];
		if (files.length === 0) {
			reviewDiff.innerHTML =
				'<div class="review-empty">No changes in this diff.</div>';
			return;
		}
		files.forEach((file) => reviewDiff.appendChild(renderFileDiff(file)));
		renderAllComments();
	}

	function renderFileDiff(file) {
		const path = file.new_path || file.old_path || "(unknown)";
		const wrap = document.createElement("div");
		wrap.className = "diff-file";
		const header = document.createElement("div");
		header.className = "diff-file-header";
		header.innerHTML = `<span class="diff-file-status diff-${escapeHtml(file.status || "modified")}">${escapeHtml(file.status || "modified")}</span><span class="diff-file-path">${escapeHtml(path)}</span>`;
		wrap.appendChild(header);
		(file.hunks || []).forEach((hunk) => {
			const hh = document.createElement("div");
			hh.className = "diff-hunk-header";
			hh.textContent = hunk.header;
			wrap.appendChild(hh);
			(hunk.lines || []).forEach((line) =>
				wrap.appendChild(renderDiffLine(path, file, line)),
			);
		});
		return wrap;
	}

	function renderDiffLine(path, file, line) {
		const row = document.createElement("div");
		row.className = `diff-line diff-${line.kind}`;
		const side = line.new_line > 0 ? "new" : "old";
		const lineNo = side === "new" ? line.new_line : line.old_line;
		const anchorFile = side === "old" ? file.old_path || path : path;
		row.dataset.file = anchorFile;
		row.dataset.side = side;
		row.dataset.line = String(lineNo);
		const sign = line.kind === "add" ? "+" : line.kind === "del" ? "−" : " ";
		row.innerHTML = `
            <span class="diff-gutter old">${line.old_line > 0 ? line.old_line : ""}</span>
            <span class="diff-gutter new">${line.new_line > 0 ? line.new_line : ""}</span>
            <span class="diff-comment-add" title="Add comment">+</span>
            <span class="diff-sign">${sign}</span>
            <span class="diff-code">${escapeHtml(line.text)}</span>
        `;
		row.querySelector(".diff-comment-add").addEventListener("click", (e) => {
			e.stopPropagation();
			openComposer(row, anchorFile, side, lineNo);
		});
		return row;
	}

	function renderAllComments() {
		// Remove existing comment blocks then re-attach from state.
		reviewDiff
			.querySelectorAll(".diff-comment-block")
			.forEach((n) => n.remove());
		reviewComments.forEach((c) => {
			const row = findLineRow(c.file, c.side, c.line);
			if (row) insertCommentBlock(row, c);
		});
	}

	function findLineRow(file, side, line) {
		return reviewDiff.querySelector(
			`.diff-line[data-file="${cssEscape(file)}"][data-side="${side}"][data-line="${line}"]`,
		);
	}

	function insertCommentBlock(row, comment) {
		const block = document.createElement("div");
		block.className = "diff-comment-block";
		block.innerHTML = `
            <div class="diff-comment-body">${escapeHtml(comment.body)}</div>
            <button type="button" class="diff-comment-del" aria-label="Delete comment">&times;</button>
        `;
		block
			.querySelector(".diff-comment-del")
			.addEventListener("click", () => deleteComment(comment));
		insertAfterTrailing(row, block);
	}

	function openComposer(row, file, side, line) {
		// Avoid duplicate composers on the same row.
		const existing = nextComposer(row);
		if (existing) {
			existing.querySelector("textarea").focus();
			return;
		}
		const box = document.createElement("div");
		box.className = "diff-comment-block diff-composer";
		box.innerHTML = `
            <textarea class="diff-comment-input" rows="3" placeholder="Leave a comment…"></textarea>
            <div class="diff-composer-actions">
                <button type="button" class="primary-btn diff-comment-save">Comment</button>
                <button type="button" class="ghost-btn diff-comment-cancel">Cancel</button>
            </div>
        `;
		box
			.querySelector(".diff-comment-cancel")
			.addEventListener("click", () => box.remove());
		box
			.querySelector(".diff-comment-save")
			.addEventListener("click", async () => {
				const body = box.querySelector("textarea").value.trim();
				if (!body) return;
				box.remove();
				await addComment({
					file,
					side,
					line,
					body,
					created_at: new Date().toISOString(),
				});
			});
		insertAfterTrailing(row, box);
		box.querySelector("textarea").focus();
	}

	// insertAfterTrailing inserts node after row and any comment blocks already
	// attached to it, so comments and composer stack in order under the line.
	function insertAfterTrailing(row, node) {
		let ref = row;
		while (
			ref.nextSibling &&
			ref.nextSibling.classList &&
			ref.nextSibling.classList.contains("diff-comment-block")
		) {
			ref = ref.nextSibling;
		}
		ref.parentNode.insertBefore(node, ref.nextSibling);
	}

	function nextComposer(row) {
		let ref = row.nextSibling;
		while (
			ref &&
			ref.classList &&
			ref.classList.contains("diff-comment-block")
		) {
			if (ref.classList.contains("diff-composer")) return ref;
			ref = ref.nextSibling;
		}
		return null;
	}

	async function addComment(comment) {
		reviewComments.push(comment);
		await persistReview();
		renderAllComments();
	}

	async function deleteComment(comment) {
		reviewComments = reviewComments.filter((c) => c !== comment);
		await persistReview();
		renderAllComments();
	}

	async function persistReview() {
		try {
			await apiPut(`/api/spec/${encodeURIComponent(currentSpecCode)}/review`, {
				comments: reviewComments,
			});
		} catch (err) {
			showToast(`Save failed: ${err.message || err}`, "err");
		}
	}

	async function confirmAndDeleteSpec(code, title) {
		if (!code) return false;
		const label = title ? `${code} — ${title}` : code;
		const confirmed = window.confirm(
			`Delete ${label}? This removes the story from the local backlog and deletes its local plan/review artifacts if present. This cannot be undone from the viewer.`,
		);
		if (!confirmed) return false;
		try {
			await apiDelete(`/api/spec/${encodeURIComponent(code)}`);
			showToast(`${code} deleted`, "ok");
			if (currentSpecCode === code) {
				closeModal();
			}
			await loadBoard();
			return true;
		} catch (err) {
			showToast(`Delete failed: ${err.message || err}`, "err");
			return false;
		}
	}

	async function onRequestChanges() {
		if (!currentSpecCode) return;
		if (reviewComments.length === 0) {
			showToast("Add at least one comment first", "err");
			return;
		}
		if (
			!window.confirm(
				`Convert ${reviewComments.length} comment(s) into Fix tasks and send ${currentSpecCode} back to IN PROGRESS?`,
			)
		)
			return;
		reviewStatus.textContent = "Requesting changes…";
		reviewStatus.className = "status-msg";
		try {
			const res = await apiPost(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/request-changes`,
				{},
			);
			showToast(
				`${currentSpecCode}: ${res.tasks_added} fix task(s) added`,
				"ok",
			);
			closeModal();
			await loadBoard();
		} catch (err) {
			reviewStatus.textContent = `Failed: ${err.message || err}`;
			reviewStatus.className = "status-msg err";
		}
	}

	async function onIntegrate() {
		if (!currentSpecCode) return;
		if (
			!window.confirm(
				`Merge ${currentSpecCode}'s branch into base, remove its worktree and mark it DONE?`,
			)
		)
			return;
		reviewStatus.textContent = "Integrating…";
		reviewStatus.className = "status-msg";
		try {
			await apiPost(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/integrate`,
				{},
			);
			showToast(`${currentSpecCode} integrated`, "ok");
			closeModal();
			await loadBoard();
		} catch (err) {
			reviewStatus.textContent = `Failed: ${err.message || err}`;
			reviewStatus.className = "status-msg err";
		}
	}

	// cssEscape escapes a string for use in a CSS attribute selector. Falls back
	// to a manual escape when CSS.escape is unavailable.
	function cssEscape(s) {
		if (window.CSS && CSS.escape) return CSS.escape(s);
		return String(s).replace(/["\\]/g, "\\$&");
	}

	// ---- Config ------------------------------------------------------------

	function configField(name) {
		return configModal.querySelector(`[name="${name}"]`);
	}

	function selectedConnector() {
		const checked = configModal.querySelector('input[name="connector"]:checked');
		return (checked && checked.value) || "file";
	}

	function setConnectorSelection(value) {
		configModal.querySelectorAll('input[name="connector"]').forEach((input) => {
			input.checked = input.value === value;
		});
		updateConnectorSections();
	}

	function updateConnectorSections() {
		const connector = selectedConnector();
		configModal
			.querySelectorAll(".config-connector-section")
			.forEach((section) => {
				section.classList.toggle(
					"hidden",
					section.dataset.connectorSection !== connector,
				);
			});
		configConnectorGrid.querySelectorAll(".config-connector-card").forEach((card) => {
			const input = card.querySelector('input[name="connector"]');
			card.classList.toggle("active", !!input && input.checked);
		});
	}

	function setConfigStatus(message, kind) {
		configStatus.textContent = message || "";
		configStatus.className = "status-msg";
		if (kind) configStatus.classList.add(kind);
	}

	function setConfigValidation(message, kind) {
		configValidation.textContent = message || "Not tested in this session.";
		configValidation.className = "config-validation";
		if (kind) configValidation.classList.add(kind);
	}

	function formatKVMap(obj) {
		if (!obj) return "";
		return Object.entries(obj)
			.map(([k, v]) => `${k}: ${v}`)
			.join("\n");
	}

	function parseKVMap(text) {
		const out = {};
		String(text || "")
			.split(/\r?\n/)
			.map((line) => line.trim())
			.filter(Boolean)
			.forEach((line) => {
				const idx = line.indexOf(":");
				if (idx === -1) {
					out[line] = "";
					return;
				}
				const key = line.slice(0, idx).trim();
				if (!key) return;
				out[key] = line.slice(idx + 1).trim();
			});
		return out;
	}

	function fillConfigForm(cfg) {
		const githubFields = (cfg.github && cfg.github.fields) || {};
		configField("paths_prd").value = (cfg.paths && cfg.paths.prd) || "";
		configField("paths_mockups").value = (cfg.paths && cfg.paths.mockups) || "";
		configField("paths_test_results").value =
			(cfg.paths && cfg.paths.test_results) || "";
		configField("file_backlog").value = (cfg.file && cfg.file.backlog) || "";
		configField("file_planning").value = (cfg.file && cfg.file.planning) || "";
		configField("status_todo").value =
			(cfg.workflow && cfg.workflow.statuses && cfg.workflow.statuses.todo) || "";
		configField("status_planned").value =
			(cfg.workflow && cfg.workflow.statuses && cfg.workflow.statuses.planned) || "";
		configField("status_in_progress").value =
			(cfg.workflow && cfg.workflow.statuses && cfg.workflow.statuses.in_progress) ||
			"";
		configField("status_review").value =
			(cfg.workflow && cfg.workflow.statuses && cfg.workflow.statuses.review) ||
			"";
		configField("status_done").value =
			(cfg.workflow && cfg.workflow.statuses && cfg.workflow.statuses.done) || "";
		// The Wiki gate defaults to off, so an absent section or key means
		// disabled: only an explicit true checks the box.
		configField("wiki_enabled").checked = !!(
			cfg.wiki && cfg.wiki.enabled
		);
		configField("worktree_enabled").checked = !!(
			cfg.worktree && cfg.worktree.enabled
		);
		configField("worktree_base").value =
			(cfg.worktree && cfg.worktree.base) || "";
		configField("worktree_dir").value = (cfg.worktree && cfg.worktree.dir) || "";
		configField("worktree_branch_prefix").value =
			(cfg.worktree && cfg.worktree.branch_prefix) || "";
		configField("e2e_record_demo_video").checked = !!(
			cfg.e2e && cfg.e2e.record_demo_video
		);
		configField("git_auto_commit").checked = !!(cfg.git && cfg.git.auto_commit);
		setConnectorSelection(cfg.connector || "file");
		configField("github_owner").value = (cfg.github && cfg.github.owner) || "";
		configField("github_project_number").value =
			(cfg.github && cfg.github.project_number) || "";
		configField("github_project_node_id").value =
			(cfg.github && cfg.github.project_node_id) || "";
		configField("github_project_url").value =
			(cfg.github && cfg.github.project_url) || "";
		configField("github_status_field_id").value =
			githubFields.status_field_id || "";
		configField("github_priority_field_id").value =
			githubFields.priority_field_id || "";
		configField("github_points_field_id").value =
			githubFields.points_field_id || "";
		configField("github_epic_field_id").value =
			githubFields.epic_field_id || "";
		configField("github_status_options").value = formatKVMap(
			githubFields.status_options,
		);
		configField("github_priority_options").value = formatKVMap(
			githubFields.priority_options,
		);
		configField("github_epic_options").value = formatKVMap(
			githubFields.epic_options,
		);
		configField("jira_base_url").value = (cfg.jira && cfg.jira.base_url) || "";
		configField("jira_project_key").value =
			(cfg.jira && cfg.jira.project_key) || "";
		configField("jira_email").value = (cfg.jira && cfg.jira.email) || "";
		configField("jira_story_type").value =
			(cfg.jira && cfg.jira.story_type) || "";
		configField("jira_subtask_type").value =
			(cfg.jira && cfg.jira.subtask_type) || "";
		configField("jira_points_field").value =
			(cfg.jira && cfg.jira.points_field) || "";
		configField("jira_status_map").value = formatKVMap(
			cfg.jira && cfg.jira.status_map,
		);
		configField("jira_priority_map").value = formatKVMap(
			cfg.jira && cfg.jira.priority_map,
		);
	}

	function buildGuidedConfig() {
		const projectNumber = parseInt(configField("github_project_number").value, 10);
		return {
			connector: selectedConnector(),
			paths: {
				prd: configField("paths_prd").value.trim(),
				mockups: configField("paths_mockups").value.trim(),
				test_results: configField("paths_test_results").value.trim(),
			},
			workflow: {
				statuses: {
					todo: configField("status_todo").value.trim(),
					planned: configField("status_planned").value.trim(),
					in_progress: configField("status_in_progress").value.trim(),
					review: configField("status_review").value.trim(),
					done: configField("status_done").value.trim(),
				},
			},
			file: {
				backlog: configField("file_backlog").value.trim(),
				planning: configField("file_planning").value.trim(),
			},
			github: {
				owner: configField("github_owner").value.trim(),
				project_number: Number.isFinite(projectNumber) ? projectNumber : 0,
				project_node_id: configField("github_project_node_id").value.trim(),
				project_url: configField("github_project_url").value.trim(),
				fields: {
					status_field_id: configField("github_status_field_id").value.trim(),
					status_options: parseKVMap(
						configField("github_status_options").value,
					),
					priority_field_id: configField("github_priority_field_id").value.trim(),
					priority_options: parseKVMap(
						configField("github_priority_options").value,
					),
					points_field_id: configField("github_points_field_id").value.trim(),
					epic_field_id: configField("github_epic_field_id").value.trim(),
					epic_options: parseKVMap(configField("github_epic_options").value),
				},
			},
			jira: {
				base_url: configField("jira_base_url").value.trim(),
				project_key: configField("jira_project_key").value.trim(),
				email: configField("jira_email").value.trim(),
				story_type: configField("jira_story_type").value.trim(),
				subtask_type: configField("jira_subtask_type").value.trim(),
				points_field: configField("jira_points_field").value.trim(),
				status_map: parseKVMap(configField("jira_status_map").value),
				priority_map: parseKVMap(configField("jira_priority_map").value),
			},
			wiki: {
				enabled: !!configField("wiki_enabled").checked,
			},
			worktree: {
				enabled: !!configField("worktree_enabled").checked,
				base: configField("worktree_base").value.trim(),
				dir: configField("worktree_dir").value.trim(),
				branch_prefix: configField("worktree_branch_prefix").value.trim(),
			},
			e2e: {
				record_demo_video: !!configField("e2e_record_demo_video").checked,
			},
			git: {
				auto_commit: !!configField("git_auto_commit").checked,
			},
		};
	}

	function activateConfigTab(name) {
		activeConfigTab = name || "guided";
		configTabs.forEach((tab) => {
			const active = tab.dataset.configTab === activeConfigTab;
			tab.classList.toggle("active", active);
			tab.setAttribute("aria-selected", active ? "true" : "false");
		});
		configPanels.forEach((panel) => {
			panel.classList.toggle(
				"active",
				panel.dataset.configPanel === activeConfigTab,
			);
		});
	}

	async function openConfig() {
		configModal.classList.remove("hidden");
		activateConfigTab(activeConfigTab);
		setConfigStatus("Loading...", null);
		setConfigValidation("Not tested in this session.", null);
		configRestartNotice.classList.add("hidden");
		setExecutionStatus("", null);
		await Promise.all([loadConfig(), loadExecutionProviders()]);
	}

	// ---- Execution provider --------------------------------------------------
	//
	// The browser knows nothing about any provider: the list, the labels and the
	// fields to fill all come from the server, which asks the provider itself.

	function setExecutionStatus(message, kind) {
		executionStatus.textContent = message || "";
		executionStatus.className = "status-msg";
		if (kind) executionStatus.classList.add(kind);
	}

	function selectedProviderID() {
		const checked = executionProviderGrid.querySelector(
			'input[name="execution_provider"]:checked',
		);
		return (checked && checked.value) || "";
	}

	function findProvider(id) {
		return executionProviders.find((p) => p.id === id) || null;
	}

	async function loadExecutionProviders() {
		try {
			const data = await apiGet("/api/execution/providers");
			executionProviders = (data && data.providers) || [];
			executionDefault = (data && data.default) || null;
			renderProviderGrid();
			// The initial pick never lands on a provider the server declared
			// unusable: a default already saved stays shown, otherwise the first
			// available one is selected, and when none is usable nothing is.
			const firstAvailable = executionProviders.find(
				(p) => p.available !== false,
			);
			const selected = executionDefault
				? executionDefault.id
				: firstAvailable
					? firstAvailable.id
					: "";
			selectProvider(selected);
			updateProviderSummary();
			if (!selected && executionProviders.length) {
				setExecutionStatus(
					"No execution provider is usable on this machine.",
					"err",
				);
			} else {
				setExecutionStatus("", null);
			}
		} catch (err) {
			executionProviders = [];
			executionDefault = null;
			renderProviderGrid();
			setExecutionStatus(`Load failed: ${err.message || err}`, "err");
		}
	}

	function renderProviderGrid() {
		if (!executionProviders.length) {
			executionProviderGrid.innerHTML =
				'<p class="config-copy">No execution provider is registered in this build.</p>';
			executionFields.innerHTML = "";
			return;
		}
		executionProviderGrid.innerHTML = executionProviders
			.map((p) => {
				const caps = (p.capabilities || []).join(", ") || "no capability declared";
				// `available` is a server verdict: the panel only renders it. An
				// unusable provider stays visible — with its reason — but cannot be
				// picked, so nobody saves a default that could never run.
				const unavailable = p.available === false;
				const reason = p.unavailable_reason || "runtime not usable";
				const cls = unavailable
					? "config-connector-card unavailable"
					: "config-connector-card";
				const title = unavailable ? ` title="${escapeHtml(reason)}"` : "";
				const note = unavailable
					? `<small class="connector-unavailable">${escapeHtml(reason)}</small>`
					: "";
				const disabled = unavailable ? " disabled" : "";
				return `<label class="${cls}"${title}><input type="radio" name="execution_provider" value="${escapeHtml(p.id)}"${disabled} /><strong>${escapeHtml(p.label || p.id)}</strong><small>${escapeHtml(caps)}</small>${note}</label>`;
			})
			.join("");
	}

	function selectProvider(id) {
		executionProviderGrid
			.querySelectorAll('input[name="execution_provider"]')
			.forEach((input) => {
				input.checked = input.value === id;
				input.closest(".config-connector-card").classList.toggle(
					"active",
					input.checked,
				);
			});
		const values =
			executionDefault && executionDefault.id === id
				? executionDefault.config || {}
				: {};
		renderProviderFields(findProvider(id), values);
	}

	function onExecutionProviderSelected() {
		setExecutionStatus("", null);
		selectProvider(selectedProviderID());
	}

	function renderProviderFields(provider, values) {
		if (!provider) {
			executionFields.innerHTML = "";
			return;
		}
		const fields = provider.config_fields || [];
		if (!fields.length) {
			executionFields.innerHTML =
				'<p class="config-copy">This provider declares no configurable setting.</p>';
			return;
		}
		executionFields.innerHTML = fields
			.map((f) => {
				const value = values[f.name];
				const inputType = f.type === "integer" ? "number" : "text";
				const required = f.required
					? ' <span class="field-required">required</span>'
					: "";
				const help = f.help
					? `<small class="field-help">${escapeHtml(f.help)}</small>`
					: "";
				return `<label class="field full" data-provider-field="${escapeHtml(f.name)}"><span>${escapeHtml(f.label || f.name)}${required}</span><input type="${inputType}" name="provider_${escapeHtml(f.name)}" placeholder="${escapeHtml(f.placeholder || "")}" value="${value === undefined || value === null ? "" : escapeHtml(String(value))}" />${help}</label>`;
			})
			.join("");
	}

	function collectProviderConfig(provider) {
		const config = {};
		(provider.config_fields || []).forEach((f) => {
			const input = executionFields.querySelector(
				`[name="provider_${cssEscape(f.name)}"]`,
			);
			if (!input) return;
			const raw = input.value.trim();
			// An empty optional field means "use the provider default": sending it
			// as an empty string would be a value, and the provider would rightly
			// reject it.
			if (raw === "") return;
			config[f.name] = f.type === "integer" ? Number(raw) : raw;
		});
		return config;
	}

	function markProviderFieldError(name) {
		executionFields
			.querySelectorAll("[data-provider-field]")
			.forEach((label) => label.classList.remove("field-error"));
		if (!name) return;
		const label = executionFields.querySelector(
			`[data-provider-field="${cssEscape(name)}"]`,
		);
		if (!label) return;
		label.classList.add("field-error");
		const input = label.querySelector("input");
		if (input) input.focus();
	}

	function updateProviderSummary() {
		configSummaryProvider.textContent = executionDefault
			? executionDefault.id
			: "not configured";
	}

	async function saveExecutionProvider() {
		const id = selectedProviderID();
		if (!id) {
			setExecutionStatus("Select a provider first.", "err");
			return;
		}
		const provider = findProvider(id);
		if (!provider) return;
		// The server remains the authority on this; refusing here only spares the
		// user a round trip that could not succeed.
		if (provider.available === false) {
			setExecutionStatus(
				`Rejected: ${provider.unavailable_reason || "runtime not usable"}`,
				"err",
			);
			return;
		}
		markProviderFieldError("");
		setExecutionStatus("Saving…", null);
		try {
			await apiPut("/api/execution/provider/default", {
				id,
				config: collectProviderConfig(provider),
			});
			// Reload from the server instead of trusting the local form: what the
			// panel shows is then exactly what was persisted.
			await loadExecutionProviders();
			setExecutionStatus(`${id} saved as workspace default.`, "ok");
			showToast(`Execution provider set to ${id}`, "ok");
		} catch (err) {
			markProviderFieldError(err.field || "");
			setExecutionStatus(`Rejected: ${err.message || err}`, "err");
		}
	}

	// ---- Panel mounting ------------------------------------------------------

	function specContext(code) {
		return code ? `spec:${code}` : null;
	}

	// mountExecutionPanels points the action/execution/run trio at one set of
	// containers and declares the context it is following. Mounting always stops
	// the timers of whatever was mounted before: a timer from the previous
	// context would keep polling something nobody is looking at.
	function mountExecutionPanels(mount) {
		stopExecutionPolling();
		panelContext = mount.context;
		panelActions = mount.actions;
		panelExecution = mount.execution;
		panelRun = mount.run;
		panelStartURL = mount.startURL;
		panelSettle = mount.settle;
		if (panelActions) panelActions.innerHTML = "";
		renderExecution(null);
		resetRunState();
	}

	// unmountExecutionPanels releases the panel, but only when the caller is the
	// one that still holds it: closing a modal whose context was already taken
	// over by another panel must not stop that other panel's timers.
	function unmountExecutionPanels(context) {
		if (!context || panelContext !== context) return;
		stopExecutionPolling();
		panelContext = null;
		panelSettle = null;
		panelStartURL = "";
	}

	// ---- Spec actions --------------------------------------------------------

	// renderSpecActions shows the actions the workspace process Template admits
	// for the spec in its current status. The list is computed server-side and
	// recomputed on every open, so a status change is reflected without any
	// process rule living here.
	//
	// Whether an action can actually be started is a server verdict too: this
	// code reads `runnable` and `unavailable_reason` and knows nothing about
	// statuses, providers or capabilities.
	function renderSpecActions(actions) {
		const list = actions || [];
		if (!panelActions) return;
		if (!list.length) {
			panelActions.innerHTML =
				'<span class="story-actions-empty">No action is available in this status</span>';
			return;
		}
		panelActions.innerHTML = list.map(renderSpecActionChip).join("");
	}

	function renderSpecActionChip(action) {
		const label = escapeHtml(action.label || action.id);
		const id = escapeHtml(action.id);
		const body = `<span class="action-chip-label">${label}</span><code class="action-chip-id">${id}</code>`;
		if (action.runnable) {
			return `<button type="button" class="action-chip action-chip-run" data-action-id="${id}" title="Run ${label}">${body}</button>`;
		}
		const reason = action.unavailable_reason
			? ` title="${escapeHtml(action.unavailable_reason)}"`
			: "";
		return `<span class="action-chip"${reason}>${body}</span>`;
	}

	// ---- Spec execution ------------------------------------------------------

	const EXECUTION_POLL_MS = 2000;

	function isExecutionTerminal(record) {
		return (
			!!record && (record.status === "SUCCEEDED" || record.status === "FAILED")
		);
	}

	// startPanelAction turns one press into exactly one execution: the button is
	// disabled before the request leaves and is only given back when the server
	// refuses, so a double click cannot ask for a second run. Which route is
	// asked is a property of the mounted panel, not of this code.
	async function startPanelAction(actionID, button) {
		if (!actionID || !panelContext || !panelStartURL) return;
		const ctx = panelContext;
		const url = panelStartURL;
		if (button) button.disabled = true;
		try {
			const record = await apiPost(url, { action: actionID });
			if (panelContext !== ctx) return;
			renderExecution(record);
			await followExecution(record, ctx);
		} catch (err) {
			showToast(err.message || String(err), "err");
			if (button) button.disabled = false;
		}
	}

	// resumeExecution renders the execution that came with the detail and picks
	// its polling back up when it is still open. It never starts anything: a
	// page load must show the run, not launch one.
	function resumeExecution(record, ctx) {
		renderExecution(record);
		if (record && !isExecutionTerminal(record)) {
			startExecutionPolling(record.id, ctx);
		}
		resumeRun(record, ctx);
	}

	// followExecution either keeps watching a still open execution, or settles
	// one that is already over.
	async function followExecution(record, ctx) {
		if (!record) return;
		if (!isExecutionTerminal(record)) {
			startExecutionPolling(record.id, ctx);
			resumeRun(record, ctx);
			return;
		}
		await settleExecution(record, ctx);
	}

	// settleExecution hands a terminal execution to whatever the mounted panel
	// declared as its settlement: what a finished run means is a property of the
	// context, not of the execution machinery.
	async function settleExecution(record, ctx) {
		if (panelContext !== ctx) return;
		const settle = panelSettle;
		if (!settle) return;
		await settle(record, ctx);
	}

	// settleSpecExecution reloads the detail once the run is over: the plan, the
	// spec status, the actions and the execution panel are all redrawn from the
	// server rather than guessed here. The board follows only on success, because
	// that is the only outcome that can have moved the card.
	async function settleSpecExecution(record) {
		if (!currentSpecCode) return;
		const succeeded = record.status === "SUCCEEDED";
		await openEditor(currentSpecCode);
		if (succeeded) await loadBoard();
	}

	function stopExecutionPolling() {
		// The run of an execution nobody is watching must not keep being read
		// either: leaving a spec stops both timers, always together.
		stopRunPolling();
		if (executionPollTimer === null) return;
		clearInterval(executionPollTimer);
		executionPollTimer = null;
	}

	// startExecutionPolling follows one execution of one context. Every tick
	// checks that the context it was started for is still the mounted one: the
	// modal stays closable and the board navigable while the provider works, so
	// the timer must be able to notice it has been left behind.
	function startExecutionPolling(executionID, ctx) {
		stopExecutionPolling();
		if (!executionID || !ctx) return;
		executionPollTimer = setInterval(async () => {
			if (panelContext !== ctx) {
				stopExecutionPolling();
				return;
			}
			let record;
			try {
				record = await apiGet(
					`/api/execution/${encodeURIComponent(executionID)}`,
				);
			} catch (err) {
				stopExecutionPolling();
				renderExecution(
					lastExecutionRecord,
					`Status unavailable: ${err.message || err}. Reopen to check again.`,
				);
				return;
			}
			if (panelContext !== ctx) {
				stopExecutionPolling();
				return;
			}
			renderExecution(record);
			if (!isExecutionTerminal(record)) return;
			stopExecutionPolling();
			await settleExecution(record, ctx);
		}, EXECUTION_POLL_MS);
	}

	// renderExecution draws the execution panel from the record alone. No record
	// means no panel: a spec that was never run shows nothing at all.
	function renderExecution(record, note) {
		lastExecutionRecord = record || null;
		if (!panelExecution) return;
		if (!record) {
			panelExecution.innerHTML = "";
			return;
		}
		const state =
			record.status === "SUCCEEDED"
				? "ok"
				: record.status === "FAILED"
					? "err"
					: "running";
		const action = escapeHtml(record.action || "");
		const headline =
			state === "ok"
				? `${action} succeeded`
				: state === "err"
					? `${action} failed`
					: `${action} is running`;
		const lines = [];
		if (record.provider_id) {
			lines.push(`provider ${escapeHtml(record.provider_id)}`);
		}
		const stamp = formatExecutionTime(record.completed_at || record.created_at);
		if (stamp) {
			lines.push(
				`${isExecutionTerminal(record) ? "completed" : "started"} ${escapeHtml(stamp)}`,
			);
		}
		const blocks = [];
		if (lines.length) {
			blocks.push(`<div class="execution-meta">${lines.join(" · ")}</div>`);
		}
		if (record.error) {
			const code = record.error.code
				? `<code class="execution-code">${escapeHtml(record.error.code)}</code>`
				: "";
			blocks.push(
				`<div class="execution-message">${code}<span>${escapeHtml(record.error.message || "the provider gave no reason")}</span></div>`,
			);
			if (record.error.external_id) {
				blocks.push(
					`<div class="execution-meta">external id ${escapeHtml(record.error.external_id)}</div>`,
				);
			}
		}
		if (record.result) {
			if (record.result.external_id) {
				blocks.push(
					`<div class="execution-meta">external id ${escapeHtml(record.result.external_id)}</div>`,
				);
			}
			const payload = formatExecutionPayload(record.result.payload);
			if (payload) {
				blocks.push(
					`<details class="execution-payload"><summary>Provider result</summary><pre>${escapeHtml(payload)}</pre></details>`,
				);
			}
		}
		if (note) {
			blocks.push(`<div class="execution-message">${escapeHtml(note)}</div>`);
		}
		panelExecution.innerHTML = `<div class="execution-panel execution-${state}">
			<div class="execution-head">
				<span class="execution-dot" aria-hidden="true"></span>
				<span class="execution-headline">${headline}</span>
				<code class="execution-id">${escapeHtml(record.id || "")}</code>
			</div>
			${blocks.join("")}
		</div>`;
	}

	function formatExecutionTime(value) {
		if (!value) return "";
		const at = new Date(value);
		if (Number.isNaN(at.getTime())) return String(value);
		return at.toLocaleString();
	}

	function formatExecutionPayload(payload) {
		if (payload === null || payload === undefined) return "";
		if (typeof payload === "string") return payload.trim();
		try {
			return JSON.stringify(payload, null, 2);
		} catch (_) {
			return "";
		}
	}

	// ---- Remote run ----------------------------------------------------------
	//
	// The run panel is the collaborative half of an execution: the history the
	// provider publishes, the messages sent into it, the approvals it waits on,
	// and the cancel request. It shares the cadence of the execution panel but
	// keeps its own cursor, because the two read different resources and stop
	// for different reasons.
	//
	// Everything drawn here is remote text. It is escaped on the way into the
	// markup, without exception: a run is written by an agent and by the tools
	// it calls, so its content is data and never markup.

	// A read may fail transiently — the run is not lost, only momentarily
	// unreachable — so the cursor survives and the loop keeps trying. It gives
	// up only after this many consecutive failures, to avoid polling forever
	// against a viewer that is gone.
	const RUN_POLL_FAILURE_LIMIT = 3;

	// The kinds the timeline knows how to label, each with the modifier class
	// the stylesheet differentiates it by. The vocabulary belongs to the
	// provider and may grow, so an unknown kind degrades to a readable generic
	// row instead of disappearing — and it takes the `ev-unknown` variant, which
	// is drawn as explicitly uninterpreted rather than disguised as agent text.
	const RUN_EVENT_KINDS = {
		text: { label: "agent", variant: "ev-assistant" },
		thinking: { label: "thinking", variant: "ev-thinking" },
		user_message: { label: "you", variant: "ev-user" },
		tool_start: { label: "tool · start", variant: "ev-tool-start" },
		tool_end: { label: "tool · done", variant: "ev-tool-end" },
		tool_error: { label: "tool · error", variant: "ev-tool-error" },
		turn_end: { label: "turn end", variant: "ev-turn-end" },
	};

	// The wire carries three states today. CANCELLED is listed because the
	// approved design fixed the vocabulary, so a provider that grows one gets
	// its own panel instead of the neutral fallback — nothing local ever picks
	// a row here.
	const RUN_STATE_LABELS = {
		ACTIVE: "active",
		CLOSED: "closed",
		CRASHED: "ended badly",
		CANCELLED: "cancelled",
	};

	// The panel variant drives the whole colour scheme of the card, so it is
	// picked from a closed table rather than interpolated from the remote state.
	const RUN_STATE_VARIANTS = {
		ACTIVE: "run-live",
		CLOSED: "run-closed",
		CRASHED: "run-failed",
		CANCELLED: "run-cancelled",
	};

	// How an approval option is presented. The keys are the kinds a provider
	// declares; anything else falls through to the neutral button, because
	// guessing the meaning of an unknown kind is how a "deny" ends up looking
	// like the safe choice.
	const RUN_OPTION_TONES = {
		allow: " allow",
		approve: " allow",
		accept: " allow",
		deny: " deny",
		reject: " deny",
	};

	function stopRunPolling() {
		if (runPollTimer === null) return;
		clearInterval(runPollTimer);
		runPollTimer = null;
	}

	// resetRunState forgets the run of the context being left. It is called
	// before a detail is loaded, exactly like renderExecution(null), so nothing
	// of the previous run can survive into the next one.
	function resetRunState() {
		stopRunPolling();
		runExecutionID = null;
		runAfterID = 0;
		runEvents = [];
		runSnapshot = null;
		runApprovals = [];
		runPendingMessage = "";
		runNotice = "";
		runConnected = true;
		runTruncated = false;
		runRefusal = "";
		runOutcome = "";
		runDraft = "";
		runBusy = false;
		runCancelArmed = false;
		runCancelSent = false;
		runAnswered = null;
		runSeams = new Set();
		runSeamPending = false;
		runPollBusy = false;
		runPollFailures = 0;
		runPollAbandoned = false;
		if (panelRun) panelRun.innerHTML = "";
	}

	// resumeRun asks once whether the execution has an interactive run, and
	// starts following it when it has.
	//
	// A 409 is an answer, not a failure: this provider exposes no run, or the
	// workspace has no provider that could. The panel simply does not appear.
	async function resumeRun(record, ctx) {
		if (!record || !record.id || !ctx) return;
		const executionID = record.id;
		let view;
		try {
			view = await apiGet(
				`/api/execution/${encodeURIComponent(executionID)}/run?after_id=0`,
			);
		} catch (err) {
			if (panelContext !== ctx) return;
			if (err.status === 409 || err.code === "E_CONFLICT") return;
			runNotice = `The run of this execution cannot be read: ${err.message || err}`;
			renderRun();
			return;
		}
		if (panelContext !== ctx) return;
		runExecutionID = executionID;
		applyRunView(view);
		renderRun();
		if (!view || !view.run) {
			// The remote work exists but has not been handed to a run yet. That is
			// worth waiting for while the execution can still get one, and worth
			// nothing once the execution is over.
			if (!isExecutionTerminal(record)) startRunPolling(executionID, ctx);
			return;
		}
		startRunPolling(executionID, ctx);
	}

	// startRunPolling follows one run of one context, with the discipline of
	// startExecutionPolling: every tick checks that the context it was started
	// for is still the mounted one, and stops itself when it is not.
	//
	// The loop keeps going after the run has left ACTIVE, because a closed run
	// can still have a final turn to deliver. It stops when the state is no
	// longer ACTIVE *and* a read brought nothing new, so the last turn is never
	// cut off.
	function startRunPolling(executionID, ctx) {
		stopRunPolling();
		if (!executionID || !ctx) return;
		runPollTimer = setInterval(async () => {
			if (panelContext !== ctx) {
				stopRunPolling();
				return;
			}
			if (runPollBusy) return;
			runPollBusy = true;
			let view;
			try {
				view = await apiGet(
					`/api/execution/${encodeURIComponent(executionID)}/run?after_id=${runAfterID}`,
				);
			} catch (err) {
				runPollBusy = false;
				if (panelContext !== ctx) {
					stopRunPolling();
					return;
				}
				runPollFailures += 1;
				runConnected = false;
				runSeamPending = true;
				if (runPollFailures >= RUN_POLL_FAILURE_LIMIT) {
					stopRunPolling();
					// Nothing is reconnecting any more, so the panel must stop
					// saying that it is.
					runPollAbandoned = true;
					runNotice = `Run unavailable: ${err.message || err}. Reopen to follow it again.`;
				}
				renderRun();
				return;
			}
			runPollBusy = false;
			if (panelContext !== ctx) {
				stopRunPolling();
				return;
			}
			runPollFailures = 0;
			const appended = applyRunView(view);
			renderRun();
			if (runSnapshot && runSnapshot.state !== "ACTIVE" && appended === 0) {
				stopRunPolling();
			}
		}, EXECUTION_POLL_MS);
	}

	// applyRunView folds one server view into the local projection and returns
	// how many events it appended.
	//
	// Events are only ever appended, and only when their id is beyond the
	// cursor: the list is never rebuilt. That is the client half of the
	// de-duplication — the server drops what it has already published, and this
	// drops what has already been drawn, so a re-read after a reconnection
	// cannot show the same line twice.
	function applyRunView(view) {
		if (!view) return 0;
		const events = Array.isArray(view.events) ? view.events : [];
		let appended = 0;
		for (const event of events) {
			if (!event || typeof event.id !== "number") continue;
			if (event.id <= runAfterID) continue;
			// The seam is a mark drawn before the first row that arrived after the
			// channel came back. It is decoration over the same list: no event is
			// inserted, removed or renumbered by it, and the cursor is untouched.
			if (runSeamPending) {
				runSeams.add(event.id);
				runSeamPending = false;
			}
			runEvents.push(event);
			runAfterID = event.id;
			appended += 1;
		}
		if (typeof view.last_id === "number" && view.last_id > runAfterID) {
			runAfterID = view.last_id;
		}
		runSnapshot = view.run || null;
		runApprovals = Array.isArray(view.approvals) ? view.approvals : [];
		runConnected = view.connected !== false;
		// The seam follows the indicator, and the indicator is the server's
		// statement: the UI marks a gap only where the server reported one.
		if (!runConnected) runSeamPending = true;
		runTruncated = !!view.truncated;
		runNotice = view.notice || "";
		// A message is confirmed by the run republishing it, never by the 202
		// that accepted it.
		if (runPendingMessage && isMessageConfirmed(events, runPendingMessage)) {
			runPendingMessage = "";
		}
		return appended;
	}

	function isMessageConfirmed(events, message) {
		const wanted = message.trim();
		return events.some(
			(event) =>
				event &&
				event.kind === "user_message" &&
				String(event.text || "").trim() === wanted,
		);
	}

	// showRunRefusal reports a command the run would not take. It writes to the
	// toast and to one inline row, and to nothing else: the timeline, the cursor
	// and the run state are what the server said they were, and a refused
	// command did not change any of them.
	function showRunRefusal(err) {
		const message = (err && err.message) || String(err);
		const hint = err && err.hint ? ` — ${err.hint}` : "";
		runRefusal = `${message}${hint}`;
		showToast(message, "err");
	}

	async function sendRunMessage() {
		if (runBusy || !runExecutionID) return;
		const message = runDraft.trim();
		if (!message) return;
		const ctx = panelContext;
		runBusy = true;
		renderRun();
		try {
			const view = await apiPost(
				`/api/execution/${encodeURIComponent(runExecutionID)}/run/messages?after_id=${runAfterID}`,
				{ message },
			);
			if (panelContext !== ctx) return;
			// Accepted means delivered to the runner, not published: the text
			// stays out of the timeline until a user_message event carries it
			// back. Until then it is visible as pending, and only as pending.
			runPendingMessage = message;
			runDraft = "";
			runRefusal = "";
			runOutcome = "";
			applyRunView(view);
		} catch (err) {
			if (panelContext !== ctx) return;
			showRunRefusal(err);
		} finally {
			runBusy = false;
			if (panelContext === ctx) renderRun();
		}
	}

	async function respondRunApproval(approvalID, optionID) {
		if (runBusy || !runExecutionID || !approvalID || !optionID) return;
		const ctx = panelContext;
		const answering = findRunApproval(approvalID);
		const option = findRunApprovalOption(answering, optionID);
		const label = (option && (option.label || option.id)) || optionID;
		runBusy = true;
		renderRun();
		try {
			const view = await apiPost(
				`/api/execution/${encodeURIComponent(runExecutionID)}/run/approvals/${encodeURIComponent(approvalID)}?after_id=${runAfterID}`,
				{ option_id: optionID },
			);
			if (panelContext !== ctx) return;
			runRefusal = "";
			applyRunView(view);
			// The outcome is read back from the projection the server returned,
			// not asserted from the answer that was sent.
			const stillPending = runApprovals.some(
				(item) => item && item.id === approvalID,
			);
			runOutcome = stillPending
				? `Answered “${label}” — the run is still waiting on this approval.`
				: `Answered “${label}” — the run took the decision.`;
			// The resolved card keeps the decision readable once the provider
			// stops listing it as pending. It shows the provider's own options,
			// verbatim and disabled, with the answered one marked.
			if (answering) {
				runAnswered = {
					id: approvalID,
					approval: answering,
					optionID,
					denied: approvalOptionTone(option) === " deny",
					outcome: runOutcome,
				};
			}
		} catch (err) {
			if (panelContext !== ctx) return;
			showRunRefusal(err);
		} finally {
			runBusy = false;
			if (panelContext === ctx) renderRun();
		}
	}

	function findRunApproval(approvalID) {
		for (const approval of runApprovals) {
			if (approval && approval.id === approvalID) return approval;
		}
		if (runAnswered && runAnswered.id === approvalID) return runAnswered.approval;
		return null;
	}

	function findRunApprovalOption(approval, optionID) {
		if (!approval) return null;
		for (const option of approval.options || []) {
			if (option && option.id === optionID) return option;
		}
		return null;
	}

	// The affirmative/negative tone comes from the option's own kind when it
	// declares one, never from parsing its label.
	function approvalOptionTone(option) {
		const kind = option && typeof option.kind === "string" ? option.kind : "";
		// hasOwnProperty, not a bare lookup: the kind is remote text, and a kind
		// spelled "constructor" or "toString" would otherwise resolve to an
		// inherited member and be interpolated into the class attribute.
		return Object.prototype.hasOwnProperty.call(RUN_OPTION_TONES, kind)
			? RUN_OPTION_TONES[kind]
			: "";
	}

	async function cancelRun() {
		if (runBusy || !runExecutionID) return;
		const ctx = panelContext;
		runCancelArmed = false;
		runBusy = true;
		renderRun();
		try {
			const view = await apiPost(
				`/api/execution/${encodeURIComponent(runExecutionID)}/run/cancel?after_id=${runAfterID}`,
				{},
			);
			if (panelContext !== ctx) return;
			runRefusal = "";
			// runCancelSent records that the command was delivered — a fact about
			// the command, never about the run. No terminal state is written
			// here: whether the run is over stays the server's statement, carried
			// by this view and by the reads that follow.
			runCancelSent = true;
			applyRunView(view);
			startRunPolling(runExecutionID, ctx);
		} catch (err) {
			if (panelContext !== ctx) return;
			showRunRefusal(err);
		} finally {
			runBusy = false;
			if (panelContext === ctx) renderRun();
		}
	}

	// renderRun draws the panel from the local projection alone. It never
	// fetches, never derives a state the server did not report, and preserves
	// the two things a re-render would otherwise steal: the text being typed
	// and the reading position in the timeline.
	function renderRun() {
		if (!panelRun) return;
		if (!runSnapshot && !runNotice) {
			panelRun.innerHTML = "";
			return;
		}
		const timeline = panelRun.querySelector(".run-timeline");
		const previousTop = timeline ? timeline.scrollTop : 0;
		const wasAtBottom = timeline
			? timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 24
			: true;
		const focused = document.activeElement;
		const composerHadFocus = !!(
			focused &&
			focused.classList &&
			focused.classList.contains("run-composer-input")
		);
		const caret = composerHadFocus ? focused.selectionStart : 0;

		if (!runSnapshot) {
			panelRun.innerHTML = `<section class="run-panel">
				${runNotice ? renderRunNotice("info", "waiting", runNotice) : ""}
			</section>`;
			return;
		}

		const variant = RUN_STATE_VARIANTS[runSnapshot.state] || "run-closed";
		// An approval answered from here keeps its card only once the provider
		// has stopped listing it as pending. While it is still pending the
		// provider's own card stands, and the answer is reported as a notice.
		const answeredShown = !!(
			runAnswered &&
			!runApprovals.some((item) => item && item.id === runAnswered.id)
		);
		const blocks = [];
		if (runSnapshot.error) {
			blocks.push(renderRunNotice("refused", "error", runSnapshot.error));
		}
		if (runRefusal) {
			blocks.push(renderRunNotice("refused", "refused", runRefusal, "refusal"));
		}
		if (runOutcome && !answeredShown) {
			blocks.push(renderRunNotice("ok", "confirmed", runOutcome, "outcome"));
		}
		if (runNotice) {
			blocks.push(renderRunNotice("info", "channel", runNotice));
		}
		const closedAt = formatExecutionTime(runSnapshot.closed_at);
		if (closedAt) {
			blocks.push(
				renderRunNotice("info", "ended", `The provider closed this run at ${closedAt}.`),
			);
		}
		if (runTruncated) {
			blocks.push(
				renderRunNotice(
					"info",
					"window",
					"Older history is beyond the window this viewer keeps; the provider still holds it.",
				),
			);
		}
		blocks.push(renderRunTimeline());
		// The tail says the run is still speaking. It is drawn from the state the
		// server reported and from its connection flag, never from a local guess.
		if (runSnapshot.state === "ACTIVE" && runConnected && runEvents.length) {
			blocks.push(
				'<div class="run-tail"><span class="run-tail-dots" aria-hidden="true"><span></span><span></span><span></span></span>the run is still working</div>',
			);
		}
		blocks.push(
			runApprovals.map((item) => renderRunApproval(item, null)).join(""),
		);
		if (answeredShown) {
			blocks.push(renderRunApproval(runAnswered.approval, runAnswered));
		}
		blocks.push(renderRunComposer());

		panelRun.innerHTML = `<section class="run-panel ${variant}" aria-label="Remote run">
			<div class="run-head">
				<span class="run-badge">${escapeHtml(RUN_STATE_LABELS[runSnapshot.state] || "run")}</span>
				<code class="run-id">${escapeHtml(runSnapshot.run_id || "")}</code>
				<span class="run-head-spacer"></span>
				${renderRunLink()}
			</div>
			${blocks.join("")}
		</section>`;

		const nextTimeline = panelRun.querySelector(".run-timeline");
		if (nextTimeline) {
			nextTimeline.scrollTop = wasAtBottom
				? nextTimeline.scrollHeight
				: previousTop;
		}
		const input = panelRun.querySelector(".run-composer-input");
		if (input) {
			// The draft is restored as a value and never as markup, so no amount
			// of typing can reach the parser.
			input.value = runDraft;
			if (composerHadFocus && !input.disabled) {
				input.focus();
				const at = Math.min(caret, input.value.length);
				input.setSelectionRange(at, at);
			}
		}
	}

	// renderRunNotice draws one additive row above the timeline. Every caller
	// passes a tone from the stylesheet's closed set — info, refused, ok — so a
	// provider string can never choose how it is presented.
	function renderRunNotice(tone, mark, body, dismiss) {
		const close = dismiss
			? `<button type="button" class="run-notice-dismiss" data-notice-dismiss="${escapeHtml(dismiss)}" aria-label="Dismiss this notice">✕</button>`
			: "";
		return `<div class="run-notice ${tone}"${tone === "refused" ? ' role="alert"' : ""}>
			<span class="run-notice-mark">${escapeHtml(mark)}</span>
			<span class="run-notice-body">${escapeHtml(body)}</span>
			${close}
		</div>`;
	}

	// dismissRunNotice closes one inline notice and only that: the projection it
	// was reporting on — timeline, cursor, run state — is untouched.
	function dismissRunNotice(which) {
		if (which === "refusal") runRefusal = "";
		else if (which === "outcome") runOutcome = "";
		else return;
		renderRun();
	}

	// renderRunLink shows the transport, and says so only when it is degraded:
	// a run being followed needs no announcement, a run being reconnected does.
	//
	// The run state is read first, because a run that has ended has no stream to
	// reconnect to: reporting "reconnecting…" next to a "Run closed" badge would
	// be two contradictory statements about the same run.
	function renderRunLink() {
		const mark = '<span class="run-link-mark" aria-hidden="true"></span>';
		if (runSnapshot && runSnapshot.state !== "ACTIVE") {
			return `<span class="run-link offline">${mark}channel closed</span>`;
		}
		if (runPollAbandoned) {
			return `<span class="run-link offline">${mark}not following</span>`;
		}
		if (runConnected) {
			return `<span class="run-link listening">${mark}following</span>`;
		}
		return `<span class="run-link reconnecting">${mark}reconnecting…</span>`;
	}

	// The cancel control exists only while the run is active. A run that has
	// ended cannot be reopened, so offering to close it again would be a lie.
	//
	// The confirmation is inline rather than a window.confirm: a modal dialog
	// would block the polling loop that keeps the panel current, and the run
	// would freeze behind it.
	function renderRunCancel() {
		if (!runSnapshot) return "";
		if (runSnapshot.state !== "ACTIVE") {
			// When a cancel was asked for from here, what confirms it is the
			// terminal state the server ended up reporting — never the request.
			if (!runCancelSent) return "";
			// The same instant as the "ended" notice, so it is formatted the same
			// way: one panel must not show one moment in two spellings.
			const stamp = formatExecutionTime(runSnapshot.closed_at);
			const settled = stamp ? `cancel confirmed · ${stamp}` : "cancel confirmed";
			return `<span class="run-cancel"><span class="run-cancel-state confirmed">${escapeHtml(settled)}</span></span>`;
		}
		if (runCancelSent) {
			return '<span class="run-cancel"><span class="run-cancel-state">cancel delivered · waiting for the run to report its state</span></span>';
		}
		const disabled = runBusy ? " disabled" : "";
		if (!runCancelArmed) {
			return `<span class="run-cancel">
				<button type="button" class="ghost-btn danger-ghost-btn" data-cancel-open${disabled}>Cancel run</button>
			</span>`;
		}
		return `<span class="run-cancel">
			<span class="run-cancel-confirm">
				<span class="run-cancel-question">Stop the agent where it is?</span>
				<button type="button" class="approval-btn deny" data-cancel-confirm${disabled}>Yes, cancel</button>
				<button type="button" class="approval-btn" data-cancel-abort${disabled}>No</button>
			</span>
		</span>`;
	}

	function renderRunTimeline() {
		// A terminal run's history is frozen: it stays readable, and the styling
		// says at a glance that nothing more will be added to it.
		const frozen =
			runSnapshot && runSnapshot.state !== "ACTIVE" ? " is-frozen" : "";
		if (!runEvents.length) {
			return `<ol class="run-timeline${frozen}"><li class="run-timeline-empty">Nothing has been published on this run yet.</li></ol>`;
		}
		const rows = runEvents
			.map((event) => renderRunSeam(event) + renderRunEvent(event))
			.join("");
		return `<ol class="run-timeline${frozen}">${rows}</ol>`;
	}

	// The seam is where the timeline picked up again after the channel dropped.
	// It shows both halves of AC-2 at a glance: the resume point, and that
	// nothing was lost or repeated around it.
	function renderRunSeam(event) {
		if (!runSeams.has(event.id)) return "";
		return `<li class="run-seam" role="separator">resumed at #${escapeHtml(String(event.id))}</li>`;
	}

	// renderRunEvent draws one row: the rail carries the glyph and the event id,
	// the body carries the head and the text. The id is shown because it is what
	// makes the de-duplication legible — an operator can see that the timeline
	// went 5, 6, 7 across a reconnection and repeated nothing.
	function renderRunEvent(event) {
		const kind = event && typeof event.kind === "string" ? event.kind : "";
		// The variant is picked from the known table, never built from the remote
		// string: an unknown kind gets the generic row rather than a class name
		// written by the provider.
		const known = Object.prototype.hasOwnProperty.call(RUN_EVENT_KINDS, kind);
		const variant = known ? RUN_EVENT_KINDS[kind].variant : "ev-unknown";
		const label = known ? RUN_EVENT_KINDS[kind].label : kind || "event";
		const head = [`<span class="run-event-kind">${escapeHtml(label)}</span>`];
		const stamp = formatRunTime(event.at);
		if (stamp) {
			head.push(`<span class="run-event-time">${escapeHtml(stamp)}</span>`);
		}
		// A tool event names its tool where the text would be; an event that has
		// both keeps the tool as the lead-in to the text.
		const text = String(event.text || "");
		const lines = [];
		if (event.tool) {
			lines.push(
				`<p class="run-event-text"><code class="run-event-tool">${escapeHtml(event.tool)}</code></p>`,
			);
			if (text) {
				lines.push(`<p class="run-event-detail">${escapeHtml(text)}</p>`);
			}
		} else if (text) {
			lines.push(`<p class="run-event-text">${escapeHtml(text)}</p>`);
		}
		const body = lines.join("");
		return `<li class="run-event ${variant}">
			<div class="run-event-rail"><span class="run-event-glyph" aria-hidden="true"></span>#${escapeHtml(String(event.id))}</div>
			<div class="run-event-body">
				<div class="run-event-head">${head.join("")}</div>
				${body}
			</div>
		</li>`;
	}

	// renderRunApproval shows the decision verbatim: which answers exist is the
	// provider's statement, so the buttons are its options and nothing else.
	// A resolved card carries the same options, disabled, with the answered one
	// marked: its label stays the provider's, unchanged.
	function renderRunApproval(approval, answered) {
		if (!approval || !approval.id) return "";
		const id = escapeHtml(approval.id);
		const head = [
			`<span class="run-approval-eyebrow">${answered ? "approval resolved" : "approval requested"}</span>`,
		];
		if (approval.tool_name) {
			head.push(
				`<code class="run-approval-tool">${escapeHtml(approval.tool_name)}</code>`,
			);
		}
		const stamp = formatRunTime(approval.created_at);
		if (stamp) {
			head.push(`<span class="run-event-time">${escapeHtml(stamp)}</span>`);
		}
		const args = answered ? "" : formatExecutionPayload(approval.args);
		const argsBlock = args
			? `<pre class="run-approval-args">${escapeHtml(args)}</pre>`
			: "";
		const options = (approval.options || [])
			.filter((option) => option && option.id)
			.map((option) => {
				// The affirmative/negative styling comes from the option's own kind
				// when it declares one, never from parsing its label. The kind is
				// the provider's word, so both spellings of a refusal are honoured
				// and an unknown one simply gets the neutral button.
				const tone = approvalOptionTone(option);
				const chosen =
					answered && answered.optionID === option.id ? " is-chosen" : "";
				const off = answered || runBusy ? " disabled" : "";
				return `<button type="button" class="approval-btn${tone}${chosen}" data-approval-id="${id}" data-option-id="${escapeHtml(option.id)}"${off}>${escapeHtml(option.label || option.id)}</button>`;
			})
			.join("");
		const card = ["run-approval"];
		if (answered) card.push("is-answered");
		if (answered && answered.denied) card.push("is-denied");
		// The outcome is the sentence built from the projection the server
		// returned, never from the answer that was sent.
		const outcome = answered
			? `<div class="run-approval-outcome${answered.denied ? " denied" : ""}">${escapeHtml(answered.outcome)}</div>`
			: "";
		return `<div class="${card.join(" ")}" role="group" aria-label="${answered ? "Resolved approval" : "Pending approval"}">
			<div class="run-approval-head">${head.join("")}</div>
			<p class="run-approval-title">${escapeHtml(approval.title || "The run is waiting for a decision")}</p>
			${argsBlock}
			<div class="run-approval-options">${options}</div>
			${outcome}
		</div>`;
	}

	function renderRunComposer() {
		const closed = runSnapshot && runSnapshot.state !== "ACTIVE";
		const disabled = runBusy || closed ? " disabled" : "";
		const placeholder = closed
			? "This run has ended and takes no more messages"
			: "Write a message to the agent…";
		// The pending pill sits in the composer row, not in the timeline: a
		// message that has been accepted but not yet republished is a state of
		// the composer, and putting it among the events would be exactly the
		// optimistic write AC-3 forbids.
		const pending = runPendingMessage
			? `<span class="run-pending" role="status">
					<span class="run-pending-mark" aria-hidden="true"></span>
					awaiting confirmation
					<span class="run-pending-text">«${escapeHtml(runPendingMessage)}»</span>
				</span>`
			: "";
		return `<form class="run-composer">
			<textarea class="run-composer-input" rows="2" placeholder="${escapeHtml(placeholder)}"${disabled}></textarea>
			<div class="run-composer-row">
				${renderRunCancel()}
				${pending}
				<span class="run-composer-spacer"></span>
				<span class="run-composer-hint">${closed ? "the run is terminal" : "⌘ + enter to send"}</span>
				<button type="submit" class="primary-btn"${disabled}>Send</button>
			</div>
		</form>`;
	}

	function formatRunTime(value) {
		if (!value) return "";
		const at = new Date(value);
		if (Number.isNaN(at.getTime())) return "";
		return at.toLocaleTimeString();
	}

	async function loadConfig() {
		try {
			const data = await apiGet("/api/config");
			currentConfigSnapshot = (data && data.config) || {};
			currentConfigRaw = (data && data.raw) || "";
			currentConfigExists = !!(data && data.exists);
			fillConfigForm(currentConfigSnapshot);
			configRaw.value = currentConfigRaw;
			configPath.textContent = `${data.path || ".archetipo/config.yaml"} · ${currentConfigExists ? "present" : "will be created on save"}`;
			configSummaryConnector.textContent =
				(currentConfigSnapshot && currentConfigSnapshot.connector) || "file";
			configSummaryExists.textContent = currentConfigExists ? "present" : "missing";
			setConfigStatus("", null);
		} catch (err) {
			setConfigStatus(`Load failed: ${err.message || err}`, "err");
		}
	}

	function closeConfig() {
		configModal.classList.add("hidden");
		setConfigStatus("", null);
		setConfigValidation("Not tested in this session.", null);
	}

	function configPayload() {
		if (activeConfigTab === "advanced") {
			return { raw: configRaw.value };
		}
		return { config: buildGuidedConfig() };
	}

	async function validateConfig() {
		setConfigStatus("Validating...", null);
		try {
			const result = await apiPost("/api/config/test", configPayload());
			const warnings = (result && result.warnings) || [];
			if (warnings.length > 0) {
				setConfigValidation(warnings.join(" "), "warn");
			} else if (result && result.info && result.info.connector) {
				setConfigValidation(
					`Validation ok · ${result.info.connector} connector is ready.`,
					"ok",
				);
			} else {
				setConfigValidation("Validation ok.", "ok");
			}
			setConfigStatus("Validation complete", "ok");
		} catch (err) {
			setConfigValidation(err.message || String(err), "err");
			setConfigStatus(`Validation failed: ${err.message || err}`, "err");
		}
	}

	async function saveConfig() {
		setConfigStatus("Saving...", null);
		try {
			const data = await apiPut("/api/config", configPayload());
			currentConfigSnapshot = (data && data.config) || currentConfigSnapshot;
			currentConfigRaw = (data && data.raw) || currentConfigRaw;
			currentConfigExists = true;
			fillConfigForm(currentConfigSnapshot);
			configRaw.value = currentConfigRaw;
			configSummaryConnector.textContent =
				(currentConfigSnapshot && currentConfigSnapshot.connector) || "file";
			configSummaryExists.textContent = "present";
			configRestartNotice.classList.toggle(
				"hidden",
				!(data && data.restart_required),
			);
			const bits = ["Config saved"];
			if (data && data.backup_path) bits.push(`backup: ${data.backup_path}`);
			if (data && data.restart_required) bits.push("restart required");
			setConfigStatus(bits.join(" · "), "ok");
			showToast("Config saved", "ok");
		} catch (err) {
			setConfigStatus(`Save failed: ${err.message || err}`, "err");
		}
	}

	// ---- API helpers --------------------------------------------------------

	async function apiGet(url) {
		const r = await fetch(url, { headers: { Accept: "application/json" } });
		return parseResponse(r);
	}
	async function apiPost(url, body) {
		const r = await fetch(url, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
		});
		return parseResponse(r);
	}
	async function apiPut(url, body) {
		const r = await fetch(url, {
			method: "PUT",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
		});
		return parseResponse(r);
	}
	async function apiDelete(url) {
		const r = await fetch(url, { method: "DELETE" });
		return parseResponse(r);
	}
	async function parseResponse(r) {
		const text = await r.text();
		let data = null;
		try {
			data = text ? JSON.parse(text) : null;
		} catch (_) {
			data = null;
		}
		if (!r.ok) {
			const msg = data && data.error ? data.error : `HTTP ${r.status}`;
			const err = new Error(msg);
			// Structured error details travel with the error: without `field` the
			// provider form could report a rejection but not point at the input
			// that caused it.
			if (data && data.code) err.code = data.code;
			if (data && data.field) err.field = data.field;
			if (data && Array.isArray(data.fields)) err.fields = data.fields;
			// The hint explains a refusal, and the status tells a refusal from a
			// fault: the run panel treats a 409 as "no interactive run here"
			// rather than as an error to paint red.
			if (data && data.hint) err.hint = data.hint;
			err.status = r.status;
			throw err;
		}
		return data;
	}

	// ---- New spec ----------------------------------------------------------

	// The skeleton a new story starts from: it makes the spec born already in
	// the shape the validator expects. It does not replace validation — the
	// server still rejects a body that stays empty.
	function specBodyTemplate() {
		return [
			"**User Story**",
			"Come **<ruolo>**, voglio **<capacità>**, così da **<beneficio>**.",
			"",
			"**Dimostrazione**",
			"<Cosa fa il revisore, cosa osserva, con quale esito.>",
			"",
			"**Criteri di accettazione**",
			"- [ ] AC-1 — ",
			"",
		].join("\n");
	}

	function openNewSpec() {
		newSpecForm.reset();
		clearNewSpecErrors();
		newSpecBusy = false;
		newSpecStatus.textContent = "";
		newSpecStatus.className = "status-msg";
		newSpecForm.priority.value = "MEDIUM";
		newSpecForm.story_points.value = "3";

		// The epic list is the workspace's own: the form never offers a value
		// the backlog does not already know.
		const epics = (boardSnapshot && boardSnapshot.epics) || [];
		const select = newSpecForm.epic_code;
		select.textContent = "";
		// No epic is preselected: the choice must be explicit, so a story is
		// never filed under an epic the author never looked at.
		const placeholder = document.createElement("option");
		placeholder.value = "";
		placeholder.disabled = true;
		placeholder.selected = true;
		placeholder.textContent = "Choose an epic…";
		select.appendChild(placeholder);
		epics.forEach((epic) => {
			const opt = document.createElement("option");
			opt.value = epic.code;
			opt.textContent = `${epic.code} — ${epic.title}`;
			select.appendChild(opt);
		});

		if (epics.length === 0) {
			// Without an epic there is no admissible value to submit: the modal
			// opens, explains itself, and refuses the confirmation.
			newSpecNoEpics.classList.remove("hidden");
			newSpecSubmit.disabled = true;
			newSpecModal.classList.remove("hidden");
			return;
		}

		newSpecNoEpics.classList.add("hidden");
		newSpecSubmit.disabled = false;
		newSpecEditor.value(specBodyTemplate());
		newSpecModal.classList.remove("hidden");
		setTimeout(() => newSpecEditor.codemirror.refresh(), 0);
	}

	function closeNewSpec() {
		// While a confirmation is in flight the modal cannot be dismissed:
		// closing it would let a late response reopen the editor on top of a
		// draft the user has already started rewriting.
		if (newSpecBusy) return;
		newSpecModal.classList.add("hidden");
		clearNewSpecErrors();
		newSpecBusy = false;
		newSpecSubmit.disabled = false;
	}

	function clearNewSpecErrors() {
		newSpecForm.querySelectorAll(".field-error").forEach((el) => {
			el.textContent = "";
		});
		newSpecForm.querySelectorAll(".field.has-error").forEach((el) => {
			el.classList.remove("has-error");
		});
	}

	// The server owns validation: every message is rendered where it belongs,
	// as text, and a finding without a field falls back to the global status.
	// Shared by every form that submits to a route answering with `fields[]`.
	function renderFieldErrors(form, statusEl, fields) {
		const orphans = [];
		// The slots are indexed once instead of being looked up with a selector
		// built from a server value: an unexpected field name becomes an orphan
		// message, never a selector syntax error.
		const slots = new Map();
		form.querySelectorAll(".field-error").forEach((el) => {
			slots.set(el.dataset.errorFor, el);
		});
		fields.forEach((f) => {
			const name = f && f.field;
			const slot = name ? slots.get(name) : null;
			if (!slot) {
				orphans.push((f && f.message) || "invalid value");
				return;
			}
			slot.textContent = f.message || "invalid value";
			const field = slot.closest(".field");
			if (field) field.classList.add("has-error");
		});
		const counted = fields.length - orphans.length;
		const parts = [];
		if (counted > 0)
			parts.push(
				`${counted} ${counted === 1 ? "field" : "fields"} to fix · nothing was written`,
			);
		parts.push(...orphans);
		statusEl.textContent = parts.join(" · ");
		statusEl.className = "status-msg err";
	}

	function showNewSpecErrors(fields) {
		renderFieldErrors(newSpecForm, newSpecStatus, fields);
	}

	async function onCreateSpec(e) {
		e.preventDefault();
		if (newSpecBusy) return;
		clearNewSpecErrors();
		newSpecBusy = true;
		newSpecSubmit.disabled = true;
		newSpecStatus.textContent = "Creating…";
		newSpecStatus.className = "status-msg";
		const blocked = newSpecForm.blocked_by.value
			.split(",")
			.map((s) => s.trim())
			.filter(Boolean);
		// The code is deliberately absent: it is assigned by the server from the
		// persisted backlog and never computed here.
		const payload = {
			title: newSpecForm.title.value,
			epic_code: newSpecForm.epic_code.value,
			priority: newSpecForm.priority.value,
			points: parseInt(newSpecForm.story_points.value, 10) || 0,
			scope: newSpecForm.scope.value,
			blocked_by: blocked,
			body: newSpecEditor.value(),
		};
		try {
			const res = await apiPost("/api/spec", payload);
			const code = res && res.spec && res.spec.code;
			// The request is over, so the modal may close again.
			newSpecBusy = false;
			closeNewSpec();
			await loadBoard();
			if (res && res.created === false) {
				showToast(`${code} already existed — nothing created`, "ok");
			} else {
				showToast(`${code} created`, "ok");
			}
			if (code) await openEditor(code);
		} catch (err) {
			if (Array.isArray(err.fields) && err.fields.length > 0) {
				showNewSpecErrors(err.fields);
			} else {
				newSpecStatus.textContent = `Create failed: ${err.message || err}`;
				newSpecStatus.className = "status-msg err";
			}
		} finally {
			newSpecBusy = false;
			newSpecSubmit.disabled = false;
		}
	}

	// ---- New workspace ------------------------------------------------------

	// The choices are fetched on every opening rather than cached at load time:
	// this form must offer what the server accepts now, and there is no list of
	// connectors, tools or defaults written down anywhere in the frontend.
	async function openNewWorkspace() {
		newWorkspaceForm.reset();
		clearWorkspaceErrors();
		newWorkspaceBusy = false;
		newWorkspaceStatus.textContent = "";
		newWorkspaceStatus.className = "status-msg";
		newWorkspaceUnavailable.classList.add("hidden");
		newWorkspaceSubmit.disabled = true;
		newWorkspaceModal.classList.remove("hidden");

		let options;
		try {
			options = await apiGet("/api/workspace/options");
		} catch (err) {
			// Without the contract there is nothing legitimate to offer, so the
			// form says why instead of inventing a plausible list.
			newWorkspaceUnavailableText.textContent = `The initialization options could not be read: ${err.message || err}`;
			newWorkspaceUnavailable.classList.remove("hidden");
			return;
		}

		const connectorSelect = newWorkspaceForm.connector;
		connectorSelect.textContent = "";
		(options.connectors || []).forEach((c, index) => {
			const opt = document.createElement("option");
			opt.value = c.id;
			opt.textContent = c.label || c.id;
			if (index === 0) opt.selected = true;
			connectorSelect.appendChild(opt);
		});

		newWorkspaceTools.textContent = "";
		(options.tools || []).forEach((tool) => {
			const label = document.createElement("label");
			const input = document.createElement("input");
			input.type = "checkbox";
			input.name = "tool";
			input.value = tool.id;
			const text = document.createElement("span");
			text.textContent = tool.label || tool.id;
			label.appendChild(input);
			label.appendChild(text);
			newWorkspaceTools.appendChild(label);
		});

		const paths = options.paths || {};
		newWorkspaceForm["paths.prd"].value = paths.prd || "";
		newWorkspaceForm["paths.wiki"].value = paths.wiki || "";
		newWorkspaceForm["paths.mockups"].value = paths.mockups || "";
		newWorkspaceForm["paths.test_results"].value = paths.test_results || "";

		const worktree = options.worktree || {};
		newWorkspaceWorktreeEnabled.checked = Boolean(worktree.enabled);
		newWorkspaceForm["worktree.base"].value = worktree.base || "";
		newWorkspaceForm["worktree.dir"].value = worktree.dir || "";
		newWorkspaceForm["worktree.branch_prefix"].value =
			worktree.branch_prefix || "";
		syncWorktreeFields();

		// The Archetype is reported, not chosen: there is exactly one installed
		// process, and a choice with a single element is not a choice.
		const tpl = options.template || {};
		newWorkspaceTemplate.childNodes[0].nodeValue = tpl.id
			? `${tpl.id} ${tpl.version || ""}`.trim()
			: "—";

		newWorkspaceSubmit.disabled = false;
		newWorkspaceForm.dir.focus();
	}

	function closeNewWorkspace() {
		// While a creation is in flight the modal cannot be dismissed: closing it
		// would hide an operation that is still writing to disk.
		if (newWorkspaceBusy) return;
		newWorkspaceModal.classList.add("hidden");
		clearWorkspaceErrors();
		newWorkspaceSubmit.disabled = false;
	}

	function clearWorkspaceErrors() {
		newWorkspaceForm.querySelectorAll(".field-error").forEach((el) => {
			el.textContent = "";
		});
		newWorkspaceForm.querySelectorAll(".field.has-error").forEach((el) => {
			el.classList.remove("has-error");
		});
	}

	// The three worktree settings are only read when the workflow is on, so they
	// are editable only then: an enabled input nobody reads invites a value that
	// silently does nothing.
	function syncWorktreeFields() {
		const on = newWorkspaceWorktreeEnabled.checked;
		["worktree.base", "worktree.dir", "worktree.branch_prefix"].forEach(
			(name) => {
				newWorkspaceForm[name].disabled = !on;
			},
		);
	}

	function selectedWorkspaceTools() {
		return Array.from(
			newWorkspaceTools.querySelectorAll("input[type=checkbox]"),
		)
			.filter((input) => input.checked)
			.map((input) => input.value);
	}

	async function onCreateWorkspace(e) {
		e.preventDefault();
		if (newWorkspaceBusy) return;
		clearWorkspaceErrors();
		newWorkspaceBusy = true;
		newWorkspaceSubmit.disabled = true;
		newWorkspaceStatus.textContent = "Creating…";
		newWorkspaceStatus.className = "status-msg";

		const worktreeOn = newWorkspaceWorktreeEnabled.checked;
		const payload = {
			dir: newWorkspaceForm.dir.value.trim(),
			connector: newWorkspaceForm.connector.value,
			tools: selectedWorkspaceTools(),
			paths: {
				prd: newWorkspaceForm["paths.prd"].value.trim(),
				wiki: newWorkspaceForm["paths.wiki"].value.trim(),
				mockups: newWorkspaceForm["paths.mockups"].value.trim(),
				test_results: newWorkspaceForm["paths.test_results"].value.trim(),
			},
			worktree: {
				enabled: worktreeOn,
				base: newWorkspaceForm["worktree.base"].value.trim(),
				dir: newWorkspaceForm["worktree.dir"].value.trim(),
				branch_prefix:
					newWorkspaceForm["worktree.branch_prefix"].value.trim(),
			},
		};

		try {
			const res = await apiPost("/api/workspace", payload);
			newWorkspaceBusy = false;
			newWorkspaceStatus.textContent =
				res.hint || `Workspace created in ${res.dir}`;
			newWorkspaceStatus.className = "status-msg ok";
			showToast(`Workspace created in ${res.dir}`, "ok");
		} catch (err) {
			if (Array.isArray(err.fields) && err.fields.length > 0) {
				renderFieldErrors(newWorkspaceForm, newWorkspaceStatus, err.fields);
			} else {
				newWorkspaceStatus.textContent = `Create failed: ${err.message || err}`;
				newWorkspaceStatus.className = "status-msg err";
			}
		} finally {
			newWorkspaceBusy = false;
			newWorkspaceSubmit.disabled = false;
		}
	}

	// ---- Metrics -----------------------------------------------------------

	async function openMetrics() {
		metricsModal.classList.remove("hidden");
		metricsBody.innerHTML = "";
		metricsStatus.textContent = "Loading...";
		metricsStatus.className = "status-msg";
		try {
			const data = await apiGet("/api/metrics");
			renderMetrics(data || {});
			metricsStatus.textContent = "";
		} catch (err) {
			metricsStatus.textContent = `Load failed: ${err.message || err}`;
			metricsStatus.className = "status-msg err";
		}
	}

	function closeMetrics() {
		metricsModal.classList.add("hidden");
	}

	function renderMetrics(data) {
		const totals = data.totals || {};
		const pct = totals.completion_pct || 0;
		const statusClass = {
			TODO: "todo",
			PLANNED: "planned",
			"IN PROGRESS": "progress",
			REVIEW: "review",
			DONE: "done",
		};
		let html = `
            <div class="metrics-hero">
                <div class="metrics-pct">${pct}<span>%</span></div>
                <div class="metrics-hero-detail">
                    <div class="metrics-bar"><div class="metrics-bar-fill" style="width:${Math.min(pct, 100)}%"></div></div>
                    <div class="metrics-hero-caption">
                        ${totals.done_points || 0}/${totals.points || 0} points ·
                        ${totals.done_specs || 0}/${totals.specs || 0} specs done ·
                        ${totals.wip_specs || 0} in flight
                    </div>
                </div>
            </div>
            <div class="metrics-statuses">`;
		(data.by_status || []).forEach((b) => {
			const cls = statusClass[b.status] || "todo";
			html += `<div class="metrics-status st-${cls}"><span class="metrics-status-num">${b.specs}</span><span class="metrics-status-label">${escapeHtml(b.status)}</span></div>`;
		});
		html += "</div>";

		const epics = data.by_epic || [];
		if (epics.length > 0) {
			html +=
				'<h3 class="metrics-section-title">Epics</h3><div class="metrics-epics">';
			epics.forEach((e) => {
				const epct = e.completion_pct || 0;
				html += `
                    <div class="metrics-epic">
                        <div class="metrics-epic-head">
                            <span class="metrics-epic-code">${escapeHtml(e.code)}</span>
                            <span class="metrics-epic-title">${escapeHtml(e.title || "")}</span>
                            <span class="metrics-epic-pct">${epct}%</span>
                        </div>
                        <div class="metrics-bar slim"><div class="metrics-bar-fill" style="width:${Math.min(epct, 100)}%"></div></div>
                        <div class="metrics-epic-caption">${e.done_points || 0}/${e.points || 0} points · ${e.done_specs || 0}/${e.specs || 0} specs</div>
                    </div>`;
			});
			html += "</div>";
		}

		if (data.flow) {
			html += `
                <h3 class="metrics-section-title">Flow</h3>
                <div class="metrics-flow">
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${fmtDuration(data.flow.avg_cycle_seconds)}</span><span class="metrics-flow-label">avg cycle time</span></div>
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${fmtDuration(data.flow.avg_lead_seconds)}</span><span class="metrics-flow-label">avg lead time</span></div>
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${data.flow.measured_specs}</span><span class="metrics-flow-label">specs measured</span></div>
                </div>`;
		}

		const rework = data.rework || [];
		const blocked = data.blocked || [];
		if (rework.length > 0 || blocked.length > 0) {
			html +=
				'<h3 class="metrics-section-title">Attention</h3><ul class="metrics-attention">';
			rework.forEach((code) => {
				html += `<li><span class="metrics-flag rework">rework</span> ${escapeHtml(code)} came back from review with feedback</li>`;
			});
			blocked.forEach((b) => {
				html += `<li><span class="metrics-flag blocked">blocked</span> ${escapeHtml(b.code)} waits on ${escapeHtml((b.blocked_by || []).join(", "))}</li>`;
			});
			html += "</ul>";
		}

		if ((totals.specs || 0) === 0) {
			html = '<div class="empty-board">No specs in the backlog yet.</div>';
		}
		metricsBody.innerHTML = html;
	}

	function fmtDuration(seconds) {
		const s = Number(seconds) || 0;
		if (s < 60) return `${s}s`;
		const mins = Math.floor(s / 60);
		if (mins < 60) return `${mins}m`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ${mins % 60}m`;
		const days = Math.floor(hours / 24);
		return `${days}d ${hours % 24}h`;
	}

	// ---- PRD & Mockups -----------------------------------------------------

	async function openPRD() {
		prdModal.classList.remove("hidden");
		showPrdView();
		prdStatus.textContent = "Loading...";
		prdStatus.className = "status-msg";
		try {
			await reloadPrdBody();
			prdStatus.textContent = "";
		} catch (err) {
			prdStatus.textContent = `Load failed: ${err.message || err}`;
			prdStatus.className = "status-msg err";
		}
		await loadWorkspaceActions();
	}

	async function reloadPrdBody() {
		const data = await apiGet("/api/prd");
		currentPrdSnapshot = (data && data.body) || "";
		fillPrdView(currentPrdSnapshot);
		prdEditor.value(currentPrdSnapshot);
	}

	function closePRD() {
		prdModal.classList.add("hidden");
		showPrdView();
		hideWorkspaceActions();
	}

	// ---- Workspace actions ---------------------------------------------------
	//
	// A workspace-scoped action is offered by the same panel the spec detail
	// uses, mounted on the PRD modal instead. Which actions exist at all is a
	// server verdict — `offered` is the workspace-state precondition and
	// `runnable` adds provider and concurrency — so nothing here decides when an
	// action is admissible.

	async function loadWorkspaceActions() {
		let view;
		try {
			view = await apiGet("/api/workspace/actions");
		} catch (_) {
			// The workspace actions are an addition to the PRD modal, not a
			// precondition of it: a viewer that cannot answer simply offers none.
			hideWorkspaceActions();
			return;
		}
		if (prdModal.classList.contains("hidden")) return;
		const offered = ((view && view.actions) || []).filter((a) => a.offered);
		// Nothing to offer and nothing left running is the only case where the
		// panel has no reason to exist: an execution to resume keeps it up even
		// when the action that started it is no longer offered.
		if (!offered.length && !(view && view.execution)) {
			hideWorkspaceActions();
			return;
		}
		prdInception.classList.remove("hidden");
		mountExecutionPanels({
			context: WORKSPACE_CONTEXT,
			startURL: "/api/workspace/execution",
			actions: inceptionActions,
			execution: inceptionExecution,
			run: inceptionRun,
			settle: settleWorkspaceAction,
		});
		// Only the offered actions are drawn: an offered action that is not
		// runnable stays a disabled chip carrying its `unavailable_reason`.
		renderSpecActions(offered);
		// The server hands back the workspace's last execution on every read, so
		// reopening the modal finds the conversation it left behind and resumes
		// following it without ever starting a second one.
		resumeExecution(view.execution, WORKSPACE_CONTEXT);
	}

	function hideWorkspaceActions() {
		unmountExecutionPanels(WORKSPACE_CONTEXT);
		prdInception.classList.add("hidden");
		inceptionActions.innerHTML = "";
		inceptionExecution.innerHTML = "";
		inceptionRun.innerHTML = "";
	}

	// settleWorkspaceAction reads the outcome back from the server rather than
	// asserting it: the PRD body is re-fetched, and which actions are still
	// offered is decided by the same route that offered them. A run that failed
	// leaves the workspace as it was, so the panel comes back carrying the
	// reason instead of disappearing.
	async function settleWorkspaceAction(record) {
		try {
			await reloadPrdBody();
		} catch (_) {
			// The PRD is unreadable for now; the reload below still reports the
			// state of the execution.
		}
		// The board follows only on success, because that is the only outcome
		// that can have created the specs it draws.
		if (record && record.status === "SUCCEEDED") await loadBoard();
		await loadWorkspaceActions();
	}

	function fillPrdView(body) {
		prdBodyView.innerHTML = marked.parse(body || "*(no PRD yet)*");
	}

	function showPrdView() {
		prdView.classList.remove("hidden");
		prdForm.classList.add("hidden");
	}

	function enterPrdEditMode() {
		prdView.classList.add("hidden");
		prdForm.classList.remove("hidden");
		prdStatus.textContent = "";
		prdStatus.className = "status-msg";
		setTimeout(() => prdEditor.codemirror.refresh(), 0);
	}

	function exitPrdEditMode() {
		prdEditor.value(currentPrdSnapshot || "");
		showPrdView();
	}

	async function onSavePRD(e) {
		e.preventDefault();
		const body = prdEditor.value();
		prdStatus.textContent = "Saving...";
		prdStatus.className = "status-msg";
		try {
			await apiPut("/api/prd", { body });
			currentPrdSnapshot = body;
			fillPrdView(body);
			showPrdView();
			prdStatus.textContent = "Saved";
			prdStatus.className = "status-msg ok";
			showToast("PRD updated", "ok");
			// A PRD written by hand is a PRD: the inception must stop being
			// offered and the backlog generation must start being offered, and
			// that verdict is re-read rather than assumed.
			await loadWorkspaceActions();
		} catch (err) {
			prdStatus.textContent = `Save failed: ${err.message || err}`;
			prdStatus.className = "status-msg err";
		}
	}

	async function loadMockups() {
		try {
			const data = await apiGet("/api/mockups");
			mockupsCache = (data && data.mockups) || [];
			renderMockupsMenu();
		} catch (_) {
			mockupsCache = [];
			renderMockupsMenu();
		}
	}

	function renderMockupsMenu() {
		const appMockups = mockupsCache.filter((m) => !m.spec_code);
		const specMockups = mockupsCache.filter((m) => !!m.spec_code);
		mockupsMenu.innerHTML = "";

		const appSection = document.createElement("div");
		appSection.className = "mockups-section";
		if (appMockups.length === 0) {
			const empty = document.createElement("div");
			empty.className = "dropdown-empty";
			empty.textContent = "No app mockups";
			appSection.appendChild(empty);
		} else {
			appMockups.forEach((m) => appSection.appendChild(createMockupItem(m)));
		}
		mockupsMenu.appendChild(appSection);

		if (specMockups.length > 0) {
			mockupsMenu.appendChild(createSpecsSection(specMockups));
		}
	}

	function createMockupItem(m) {
		const a = document.createElement("a");
		a.href = m.url;
		a.target = "_blank";
		a.rel = "noopener";
		a.className = "dropdown-item";
		a.textContent = m.name;
		return a;
	}

	function createSpecsSection(items) {
		const section = document.createElement("div");
		section.className = "mockups-section mockups-section-stories collapsed";

		const header = document.createElement("div");
		header.className = "mockups-section-header clickable";
		header.setAttribute("role", "button");
		header.setAttribute("tabindex", "0");
		header.innerHTML =
			`<span>Specs (${items.length})</span>` +
			'<svg class="mockups-section-caret" width="9" height="9" viewBox="0 0 9 9" aria-hidden="true">' +
			'<path d="M1.5 3l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.2"/></svg>';

		const body = document.createElement("div");
		body.className = "mockups-section-body hidden";
		items.forEach((m) => body.appendChild(createMockupItem(m)));

		const toggle = (e) => {
			e.stopPropagation();
			const collapsed = section.classList.toggle("collapsed");
			body.classList.toggle("hidden", collapsed);
		};
		header.addEventListener("click", toggle);
		header.addEventListener("keydown", (e) => {
			if (e.key === "Enter" || e.key === " ") {
				e.preventDefault();
				toggle(e);
			}
		});

		section.appendChild(header);
		section.appendChild(body);
		return section;
	}

	function collapseSpecsSection() {
		const section = mockupsMenu.querySelector(".mockups-section-stories");
		if (!section) return;
		section.classList.add("collapsed");
		const body = section.querySelector(".mockups-section-body");
		if (body) body.classList.add("hidden");
	}

	function toggleMockupsMenu(e) {
		e.stopPropagation();
		const wasHidden = mockupsMenu.classList.contains("hidden");
		mockupsMenu.classList.toggle("hidden");
		if (wasHidden) collapseSpecsSection();
	}

	function escapeHtml(s) {
		if (s === null || s === undefined) return "";
		return String(s)
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;")
			.replace(/'/g, "&#39;");
	}
})();
