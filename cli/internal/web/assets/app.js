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

	let currentSpecCode = null;
	let reviewComments = []; // inline comments for the spec currently under review
	let reviewLoaded = false; // whether the review tab has been loaded for this spec
	let currentSpecSnapshot = null; // last loaded spec (for cancel + re-render after save)
	let currentPlanSnapshot = null; // last loaded plan (for cancel + re-render after save)
	let boardSnapshot = null; // last loaded board (for undo on failed drag)
	let currentPrdSnapshot = ""; // last loaded PRD body
	let currentConfigSnapshot = null; // last loaded effective config
	let currentConfigRaw = ""; // last loaded/saved config YAML
	let currentConfigExists = false; // whether config.yaml existed on open
	let activeConfigTab = "guided";
	let mockupsCache = []; // cached list of mockups (refreshed lazily)

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
		} catch (err) {
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
		currentSpecCode = code;
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
			worktree: {
				enabled: !!configField("worktree_enabled").checked,
				base: configField("worktree_base").value.trim(),
				dir: configField("worktree_dir").value.trim(),
				branch_prefix: configField("worktree_branch_prefix").value.trim(),
			},
			e2e: {
				record_demo_video: !!configField("e2e_record_demo_video").checked,
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
		await loadConfig();
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
			throw new Error(msg);
		}
		return data;
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
			const data = await apiGet("/api/prd");
			currentPrdSnapshot = (data && data.body) || "";
			fillPrdView(currentPrdSnapshot);
			prdEditor.value(currentPrdSnapshot);
			prdStatus.textContent = "";
		} catch (err) {
			prdStatus.textContent = `Load failed: ${err.message || err}`;
			prdStatus.className = "status-msg err";
		}
	}

	function closePRD() {
		prdModal.classList.add("hidden");
		showPrdView();
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
