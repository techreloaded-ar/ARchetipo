// biome-ignore lint/suspicious/noInnerHtml: safe — all data goes through escapeHtml() or marked.parse()
(() => {
	// ---- Le parole dell'interfaccia -----------------------------------------
	//
	// Ogni stringa che una persona legge sta qui, in una lingua sola. Nel punto
	// d'uso non si scrive più nessun testo a mano: una parola si cambia in un
	// posto solo, e non può più succedere che due righe vicine parlino due
	// lingue diverse. Quello che invece arriva dal server — stati, azioni, nomi
	// di provider, ragioni di rifiuto — resta com'è: è il vocabolario del
	// dominio, e riscriverlo qui vorrebbe dire inventarne una seconda versione.
	//
	// Le voci che dipendono da un valore sono funzioni: la frase resta intera e
	// leggibile qui dentro invece di essere cucita a pezzi nel punto d'uso.
	const TEXT = {
		// Tema e densità
		themeToLight: "Passa al tema chiaro",
		themeToDark: "Passa al tema scuro",
		densityComfortable: "Densità comoda",
		densityCompact: "Densità compatta",

		// Board e spec
		backTo: (label) => `Torna a ${label}`,
		statStories: "storie",
		statInFlight: "in corso",
		statDone: "concluse",
		untitled: "(senza titolo)",
		noDescription: "*(nessuna descrizione)*",
		noPlan: "*(nessun piano)*",
		noPRD: "*(nessun PRD ancora)*",
		deleteSpec: (code) => `Elimina ${code}`,
		deleteStory: "Elimina la storia",
		specTitle: (code) => `Spec ${code}`,
		blockedBy: (list) => `bloccata da ${list}`,
		emptyDone: "trascina qui una card in Review per approvarla",
		emptyColumn: "nessuna spec",
		dragOnlyReviewToDone:
			"Si può trascinare soltanto da Review a Done",
		movedToDone: (code, column) => `${code} approvata e spostata in ${column}`,
		moveFailed: (reason) => `Spostamento fallito: ${reason}`,
		noEpicsYet:
			"Definisci almeno un'epica nel backlog prima di creare una spec",
		backlogUnreadable: "Il backlog non si è potuto leggere",
		boardError: (reason) => `Errore: ${reason}`,
		stepNotStartable: "Questo passo non si può avviare adesso",
		loadFailed: (reason) => `Lettura fallita: ${reason}`,
		saveFailed: (reason) => `Salvataggio fallito: ${reason}`,
		specUpdated: (code) => `${code} aggiornata`,
		planUpdated: (code) => `piano di ${code} aggiornato`,

		// Attenzione sulle run
		waitingCount: (n) => `${n} in attesa`,
		waitingHint: "Vai alla run che aspetta una risposta",

		// Piano: righe dei task
		taskBodyPlaceholder: "Corpo del task in markdown…",
		removeTask: "Rimuovi",

		// Revisione
		verdictApproved: "Approvata",
		verdictChangesRequested: "Modifiche richieste",
		verdictWhen: (when) => ` il ${when}`,
		verdictFrom: (id) => ` — evidenze preparate dall'esecuzione ${id}`,
		noDossier:
			"Nessun dossier di revisione — avvia <em>Rivedi</em> perché il provider prepari le evidenze",
		approveHint: "Accetta l'incremento e chiude la spec",
		approveBlocked: (n) =>
			`Il dossier riporta ${n} elemento/i bloccante/i: chiedi modifiche, oppure risolvili prima`,
		unknownPath: "(sconosciuto)",
		addComment: "Aggiungi un commento",
		deleteComment: "Elimina il commento",
		commentPlaceholder: "Lascia un commento…",
		commentSave: "Commenta",
		commentCancel: "Annulla",
		commentsFirst: "Aggiungi prima almeno un commento",
		requestChangesConfirm: (n, code) =>
			`Convertire ${n} commento/i in task di fix e riportare ${code} a IN PROGRESS?`,
		requestingChanges: "Richiesta delle modifiche in corso…",
		changesRequested: (code, n) =>
			`${code}: ${n} commento/i convertito/i in task di fix`,
		failed: (reason) => `Fallito: ${reason}`,
		approveConfirm: (code) =>
			TEXT.approveConfirm(code),
		approving: "Approvazione in corso…",
		approvedIntegrated: (code) => `${code} approvata e integrata`,
		approved: (code) => `${code} approvata`,
		integrateConfirm: (code) =>
			`Integrare il ramo di ${code} nel base, rimuovere il suo worktree e segnarla DONE?`,
		integrating: "Integrazione in corso…",
		integrated: (code) => `${code} integrata`,
		deleteSpecConfirm: (label) =>
			`Eliminare ${label}? La storia esce dal backlog locale e i suoi artefatti locali di piano e revisione vengono cancellati, se ci sono. Dal visore non si può annullare.`,
		specDeleted: (code) => `${code} eliminata`,
		deleteFailed: (reason) => `Eliminazione fallita: ${reason}`,

		// Configurazione
		configNotTested: "Non verificata in questa sessione.",
		configPath: (path, exists) =>
			`${path} · ${exists ? "presente" : "verrà creato al salvataggio"}`,
		configPresent: "presente",
		configMissing: "mancante",
		configLoading: "Lettura in corso…",
		configSaving: "Salvataggio in corso…",
		configValidating: "Verifica in corso…",
		configValidOk: (connector) =>
			`Verifica riuscita · il connettore ${connector} è pronto.`,
		configValidPlain: "Verifica riuscita.",
		configValidDone: "Verifica completata",
		configValidFailed: (reason) => `Verifica fallita: ${reason}`,
		configSaved: "Configurazione salvata",
		configBackup: (path) => `backup: ${path}`,
		configRestartRequired: "serve riavviare",

		// Provider di esecuzione
		providerNotConfigured: "non configurato",
		providerNoneUsable:
			"Su questa macchina non c'è nessun provider di esecuzione utilizzabile.",
		providerNoCapability: "nessuna capacità dichiarata",
		providerRuntimeUnusable: "runtime non utilizzabile",
		providerPickFirst: "Scegli prima un provider.",
		providerRejected: (reason) => `Rifiutato: ${reason}`,
		providerSaving: "Salvataggio in corso…",
		providerSavedDefault: (id) => `${id} salvato come default del workspace.`,
		providerSetToast: (id) => `Provider di esecuzione impostato su ${id}`,

		// Pannello esecuzione
		executionStatusUnavailable: (reason) =>
			`Stato non disponibile: ${reason}. Riapri per controllare di nuovo.`,
		executionSucceeded: (action) => `${action} riuscita`,
		executionFailed: (action) => `${action} fallita`,
		executionRunning: (action) => `${action} in corso`,
		executionGoToThread: "Vai alla conversazione",
		executionProvider: (id) => `provider ${id}`,
		executionDirectory: (dir) => `directory ${dir}`,
		executionCompleted: (stamp) => `conclusa ${stamp}`,
		executionStarted: (stamp) => `avviata ${stamp}`,
		executionModel: (model, options, inherited) =>
			`modello ${model}${options}${inherited ? " (dal workspace)" : ""}`,
		executionPayload: "Risultato del provider",

		// Pannello run
		runPanel: "Run remota",
		runUnreadable: (reason) => `La run di questa esecuzione non si può leggere: ${reason}`,
		runUnavailable: (reason) =>
			`Run non disponibile: ${reason}. Riapri per seguirla di nuovo.`,
		runClosedAt: (when) => `Il provider ha chiuso questa run alle ${when}.`,
		runWindow:
			"La parte più vecchia della storia è fuori dalla finestra che il visore conserva; il provider la tiene ancora.",
		runDismissNotice: "Chiudi questo avviso",
		runSeam: (id) => `ripresa da #${id}`,
		runEventFallback: "evento",
		runBadgeFallback: "run",
		runCancel: "Annulla la run",
		runCancelQuestion: "Fermare l'agente dov'è?",
		runCancelYes: "Sì, annulla",
		runCancelNo: "No",
		runCancelConfirmed: (stamp) => `annullamento confermato · ${stamp}`,
		runCancelConfirmedPlain: "annullamento confermato",
		runCancelDelivered:
			"annullamento inviato · si aspetta che la run dichiari il suo stato",
		runApprovalPending: "Decisione in attesa",
		runApprovalTitle: "La run aspetta una decisione",
		runAnsweredStillWaiting: (label) =>
			`Risposto “${label}” — la run aspetta ancora su questa decisione.`,
		runAnsweredTaken: (label) => `Risposto “${label}” — la run ha preso la decisione.`,
		runComposerClosed: "Questa run è finita e non accetta altri messaggi",
		runComposerPlaceholder: "Scrivi un messaggio all'agente…",
		runComposerTerminal: "la run è terminale",
		runComposerHint: "invio per inviare · maiusc+invio a capo",
		runSend: "Invia",
		runPending: "in attesa di conferma",

		// Conversazione
		conversationUnreadable: (reason) =>
			`Questa conversazione non si può più leggere: ${reason}. Ricarica per seguirla di nuovo.`,

		// Nuova spec
		chooseEpic: "Scegli un'epica…",
		assistedUnavailable: (reason) =>
			`La creazione assistita non è disponibile: ${reason}`,
		assistedNotOffered:
			"Questo workspace non offre la creazione assistita.",
		draftUnreadable:
			"La run è finita senza una proposta leggibile. Scrivi la spec tu, oppure riprova.",
		draftProposed:
			"Proposta dall'agente — leggila, cambia quello che vuoi, poi conferma. Non è ancora stato scritto niente.",
		draftEpicUnknown: (epic) =>
			`L'agente ha proposto l'epica ${epic}, che questo workspace non dichiara: scegline una.`,
		fieldsToFix: (n) =>
			`${n} ${n === 1 ? "campo" : "campi"} da correggere · non è stato scritto niente`,
		creating: "Creazione in corso…",
		specExisted: (code) => `${code} esisteva già — non è stato creato niente`,
		specCreated: (code) => `${code} creata`,
		createFailed: (reason) => `Creazione fallita: ${reason}`,

		// Workspace
		workspaceOptionsUnreadable: (reason) =>
			`Le opzioni di inizializzazione non si sono potute leggere: ${reason}`,
		workspaceCreated: (dir) => `Workspace creato in ${dir}`,
		workspaceOpening: (name) => `Apertura di ${name}…`,
		workspaceOpenFailed: (reason) => `Apertura fallita: ${reason}`,
		workspaceRemoveConfirm: (label) =>
			`Rimuovere ${label} dai workspace conosciuti? La directory del workspace e i suoi file su disco non vengono toccati.`,
		workspaceRemoved: "Rimosso dai workspace conosciuti",
		workspaceRemoveFailed: (reason) => `Rimozione fallita: ${reason}`,
		workspaceAdding: "Aggiunta in corso…",
		workspaceAdded: (name) => `${name} aggiunto`,
		workspaceAddFailed: (reason) => `Aggiunta fallita: ${reason}`,
		workspaceFallbackName: "Workspace",

		// PRD e mockup
		prdLoadFailed: (reason) => `Lettura fallita: ${reason}`,
		prdSaving: "Salvataggio in corso…",
		prdSaved: "Salvato",
		prdUpdated: "PRD aggiornato",
		noMockups: "Nessun mockup dell'app",
		mockupsSpecs: (n) => `Spec (${n})`,

		// Bozze non confermate
		discardDraft: "Ci sono modifiche non salvate. Scartare la bozza?",

		// Board: stati di caricamento e vuoti
		boardLoading: "Caricamento…",
		boardNoBacklog:
			"Nessun backlog ancora — esegui <code>archetipo init</code> per cominciare.",
		reworkBadge: "In rework: le osservazioni della revisione aspettano di essere ripianificate",
		branchTitle: "ramo git",

		// Diff e revisione
		diffLoading: "Caricamento del diff…",
		diffError: (reason) => `Errore: ${reason}`,
		diffEmpty: "Nessuna modifica in questo diff.",

		// Azioni della spec
		noActionAvailable: "In questo stato non è disponibile nessuna azione",
		runAction: (label) => `Avvia ${label}`,

		// Esecuzione: errori
		providerGaveNoReason: "il provider non ha dato nessuna ragione",
		executionExternalID: (id) => `id esterno ${id}`,

		// Run: canale e coda
		runTailWorking: "la run sta ancora lavorando",
		runTimelineEmpty: "Su questa run non è ancora stato pubblicato niente.",
		runLinkClosed: "canale chiuso",
		runLinkOff: "non in ascolto",
		runLinkOn: "in ascolto",
		runLinkReconnecting: "riconnessione…",
		runApprovalEyebrowRequested: "decisione richiesta",

		// Provider: elenco vuoto
		providerNoneRegistered:
			"In questa build non è registrato nessun provider di esecuzione.",

		// Metriche
		metricsPoints: "punti",
		metricsSpecsDone: "spec concluse",
		metricsInFlight: "in corso",
		metricsEpics: "Epiche",
		metricsFlow: "Flusso",
		metricsAvgCycle: "tempo di ciclo medio",
		metricsAvgLead: "tempo di attraversamento medio",
		metricsMeasured: "spec misurate",
		metricsAttention: "Attenzione",
		metricsReworkRow: (code) =>
			`${code} è tornata dalla revisione con delle osservazioni`,
		metricsBlockedRow: (code, on) => `${code} aspetta ${on}`,
		metricsEmpty: "Nel backlog non c'è ancora nessuna spec.",
	};

	// DOM element ids are kept with their original "story-" naming for HTML
	// and CSS stability. The data model exposed by the API is "spec", which
	// is reflected in variable names, payloads and envelope keys.
	const boardEl = document.getElementById("board");
	const refreshBtn = document.getElementById("refresh-btn");
	// The shell: one primary column holding one view at a time — the
	// conversation, the board or the spec detail — plus the permanent view
	// switcher that reaches whichever view is off screen. Which of them is on
	// screen is not decided here (see the workspace shell layout section below).
	const shellEl = document.getElementById("workspace-shell");
	const shellViewsEl = document.getElementById("workspace-views");
	const workspaceRunsEl = document.getElementById("workspace-runs");
	const runsAttentionEl = document.getElementById("runs-attention");
	// The conversation panel. It is declared here, with the other elements of
	// the shell, because it is now the home view of the primary column and the
	// layout section below writes its classes at boot — before the conversation
	// section further down would have had a chance to declare it. One element,
	// one constant: the panel's own section reads this very binding.
	const conversationEl = document.getElementById("workspace-conversation");
	// The thread rail (US-058): the index of the conversations this workspace
	// has had. It is a child of the shell and not of the primary column, so it
	// is never one of the views the layout module alternates.
	const conversationsRailEl = document.getElementById(
		"workspace-conversations",
	);
	// The spec detail. It is a pane of the primary column, not a window: the
	// name stays `modal` because every id, tab and panel under it is unchanged
	// and renaming it would touch the whole file for nothing.
	const modal = document.getElementById("modal-root");
	const modalClose = document.getElementById("modal-close");
	const modalCloseLabel = document.getElementById("modal-close-label");
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
	const toastRegion = document.getElementById("toast-region");
	const toast = document.getElementById("toast");
	const toastMessage = document.getElementById("toast-message");
	const toastDismiss = document.getElementById("toast-dismiss");
	const prdBtn = document.getElementById("prd-btn");
	const prdModal = document.getElementById("prd-modal");
	const prdModalClose = document.getElementById("prd-modal-close");
	const prdView = document.getElementById("prd-view");
	const prdInception = document.getElementById("prd-inception");
	const inceptionModelChoice = document.getElementById(
		"inception-model-choice",
	);
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
	const specDraftModeManual = document.getElementById("spec-draft-mode-manual");
	const specDraftModeAssisted = document.getElementById("spec-draft-mode-assisted");
	const specDraftPanel = document.getElementById("spec-draft-panel");
	const specDraftNotice = document.getElementById("spec-draft-notice");
	const specDraftModelChoice = document.getElementById(
		"spec-draft-model-choice",
	);
	const specDraftActions = document.getElementById("spec-draft-actions");
	const specDraftExecution = document.getElementById("spec-draft-execution");
	const specDraftRun = document.getElementById("spec-draft-run");
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
	const workspaceIndicator = document.getElementById("workspace-indicator");
	const workspaceIndicatorName = document.getElementById(
		"workspace-indicator-name",
	);
	const workspacesBtn = document.getElementById("workspaces-btn");
	const workspacesModal = document.getElementById("workspaces-modal");
	const workspacesModalClose = document.getElementById(
		"workspaces-modal-close",
	);
	const workspacesList = document.getElementById("workspaces-list");
	const workspacesEmpty = document.getElementById("workspaces-empty");
	const workspacesAddForm = document.getElementById("workspaces-add-form");
	const workspacesAddSubmit = document.getElementById("workspaces-add-submit");
	const workspacesStatus = document.getElementById("workspaces-status");
	const workspaceHomeEl = document.getElementById("workspace-home");
	const workspaceHomeList = document.getElementById("workspace-home-list");
	const workspaceHomeActions = document.getElementById(
		"workspace-home-actions",
	);
	const workspaceHomeCreate = document.getElementById("workspace-home-create");
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
	const storyModelChoice = document.getElementById("story-model-choice");
	const storyActions = document.getElementById("story-actions");
	const storyExecution = document.getElementById("story-execution");
	const storyRun = document.getElementById("story-run");
	const mockupsBtn = document.getElementById("mockups-btn");
	const mockupsMenu = document.getElementById("mockups-menu");
	const mockupsDropdown = document.getElementById("mockups-dropdown");
	// The collector menu of the topbar (US-061): everything that used to sit
	// one click away is inside it now, with the same ids, so nothing here binds
	// a behaviour — this is the menu itself, not what it holds.
	const topbarMoreBtn = document.getElementById("topbar-more-btn");
	const topbarMoreMenu = document.getElementById("topbar-more-menu");
	const topbarMoreDropdown = document.getElementById("topbar-more");
	const themeToggle = document.getElementById("theme-toggle");
	const reviewTab = document.getElementById("review-tab");
	const reviewBranch = document.getElementById("review-branch");
	const reviewDiff = document.getElementById("review-diff");
	const reviewStatus = document.getElementById("review-status");
	const reviewRequestBtn = document.getElementById("review-request-btn");
	const reviewIntegrateBtn = document.getElementById("review-integrate-btn");
	const reviewApproveBtn = document.getElementById("review-approve-btn");
	const reviewDossier = document.getElementById("review-dossier");
	const densityToggle = document.getElementById("density-toggle");

	const THEME_KEY = "archetipo.theme";
	const DENSITY_KEY = "archetipo.density";

	function setTheme(theme, persist) {
		const next = theme === "light" ? "light" : "dark";
		document.documentElement.dataset.theme = next;
		themeToggle.setAttribute(
			"aria-label",
			next === "dark" ? TEXT.themeToLight : TEXT.themeToDark,
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

	// Quanto respira la cornice dell'interfaccia. Vive accanto al tema perché è
	// la stessa specie di scelta — come si guarda, non cosa si guarda — e come
	// il tema viene applicata prima del disegno da index.html, così la barra non
	// nasce alta per poi stringersi sotto gli occhi.
	function setDensity(density, persist) {
		const next = density === "compatta" ? "compatta" : "comoda";
		document.documentElement.dataset.density = next;
		if (densityToggle) {
			densityToggle.querySelector("[data-density-label]").textContent =
				next === "compatta" ? TEXT.densityComfortable : TEXT.densityCompact;
		}
		if (persist) {
			try {
				localStorage.setItem(DENSITY_KEY, next);
			} catch (_) {
				/* ignore */
			}
		}
	}

	setDensity(document.documentElement.dataset.density, false);
	if (densityToggle) {
		densityToggle.addEventListener("click", () => {
			const current =
				document.documentElement.dataset.density === "compatta"
					? "compatta"
					: "comoda";
			setDensity(current === "compatta" ? "comoda" : "compatta", true);
		});
	}

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
	// The assisted creation is a workspace action like the other two, but it is
	// drawn where it is used — inside the New spec modal, beside the manual
	// form — so it gets its own mount and its own context token.
	const SPEC_DRAFT_CONTEXT = "spec-draft";
	// The single piece of process vocabulary this file holds, and it has exactly
	// two uses: the PRD panel excludes this id, the New spec modal draws only
	// this id. Everything else about the action — whether it is offered, whether
	// it can run, and why not — stays a server verdict read from `offered`,
	// `runnable` and `unavailable_reason`.
	const SPEC_DRAFT_ACTION = "spec-draft";
	let panelContext = null; // context the panel is mounted on, or null
	let panelActions = null; // container the action chips are drawn in
	let panelExecution = null; // container the execution panel is drawn in
	let panelRun = null; // container the run panel is drawn in
	// La conversazione in cui si legge la run del pannello montato, vuota per
	// una run che non si legge da nessun'altra parte. È la risposta del server,
	// mai una deduzione della pagina: quale delle due sia una run dipende dal
	// provider che c'è dietro.
	let executionThreadID = "";
	let panelModelChoice = null; // container the single-run model choice is drawn in
	let modelChoiceView = null; // last read of GET /api/execution/model-choice
	let modelChoiceSelection = null; // what the reader chose here, or null when nobody touched it
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
	// Gli avvisi che il lettore ha chiuso con la loro X. Il pannello si
	// ridisegna a ogni lettura, quindi la scelta non può vivere nel DOM: senza
	// questa tabella il riquadro tornerebbe al giro successivo. Vale per la run
	// montata e si dimentica con lei.
	let runDismissedNotices = {}; // chiave avviso → true
	let runDraft = ""; // composer text, preserved across re-renders
	let runBusy = false; // a command is in flight: the controls stay disabled
	let runCancelArmed = false; // the inline cancel confirmation is showing
	let runPollAbandoned = false; // the client gave up reading: it is not reconnecting
	let runPollBusy = false; // a poll is in flight: ticks never overlap
	let runPollFailures = 0; // consecutive failed reads, for the give-up threshold
	let runAnsweredIDs = new Set(); // approvals answered from here, hidden immediately
	let runCancelSent = false; // a cancel was delivered — a fact about the command, not the run
	let runSeams = new Set(); // event ids the timeline resumed at after a dropped channel
	let runSeamPending = false; // the channel dropped: the next appended event opens a seam

	refreshBtn.addEventListener("click", refreshBoardAndStatus);
	modalClose.addEventListener("click", leaveSpecDetail);
	// One command in the head of the spec detail: talk about *this* spec
	// (US-058). It opens a conversation carrying the code of the spec on screen,
	// so the thread is filed under that code in the rail instead of among the
	// free ones.
	const specConversationBtn = document.getElementById("spec-conversation-btn");
	if (specConversationBtn) {
		specConversationBtn.addEventListener("click", () => {
			if (currentSpecCode) openSpecConversation(currentSpecCode);
		});
	}
	// No backdrop click any more: the spec detail is a pane of the primary
	// column, so a click on its own padding is a click inside the work, not a
	// dismissal of a window laid over the page.
	document.addEventListener("keydown", (e) => {
		// Escape leaves the spec and puts the board back in the primary column —
		// but only while the detail is the view on screen. Since US-057 a spec
		// can stay open behind the conversation, and there Escape is an ordinary
		// keystroke in a text field: acting on it would throw the reader onto
		// the board and unmount the panels of a spec they never asked to leave.
		// The handlers below still close real modals, and are untouched.
		if (e.key === "Escape" && specOpen && shellView === "spec")
			leaveSpecDetail();
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
	reviewApproveBtn.addEventListener("click", onApprove);
	// The action chips are re-rendered on every open, so the handler lives on
	// their container instead of on buttons that no longer exist. Every
	// container the panel can be mounted on is bound once, here: which of them
	// is live at any moment is decided by the mounted context, not by the
	// listeners.
	bindActionsPanel(storyActions);
	bindActionsPanel(inceptionActions);
	bindActionsPanel(specDraftActions);
	bindExecutionPanel(storyExecution);
	bindExecutionPanel(inceptionExecution);
	bindExecutionPanel(specDraftExecution);
	bindModelChoicePanel(storyModelChoice);
	bindModelChoicePanel(inceptionModelChoice);
	bindModelChoicePanel(specDraftModelChoice);
	bindRunPanel(storyRun);
	bindRunPanel(inceptionRun);
	bindRunPanel(specDraftRun);

	function bindActionsPanel(container) {
		container.addEventListener("click", (e) => {
			const btn = e.target.closest(".action-chip-run");
			if (!btn) return;
			startPanelAction(btn.dataset.actionId, btn, conversationsCurrentId);
		});
	}

	// Il pannello dell'esecuzione si ridisegna a ogni poll, quindi il pulsante
	// che porta alla conversazione della run non può possedere il proprio
	// handler: lo possiede il container, legato una volta sola qui.
	function bindExecutionPanel(container) {
		if (!container) return;
		container.addEventListener("click", (e) => {
			const btn = e.target.closest("[data-reach-thread]");
			if (!btn) return;
			revealConversationRun(btn.dataset.reachThread).catch((err) => {
				showToast(String((err && err.message) || err), "err");
			});
		});
	}

	// The model choice is redrawn from scratch every time it changes — picking a
	// model replaces the option controls below it — so the controls cannot own
	// their handlers either. The container does, bound once here; which of the
	// three containers is live is decided by the mounted panel, not by the
	// listeners.
	function bindModelChoicePanel(container) {
		if (!container) return;
		container.addEventListener("change", (e) => {
			if (container !== panelModelChoice) return;
			const target = e.target;
			if (!target) return;
			if (target.hasAttribute("data-run-model")) {
				chooseRunModel(target.value);
				return;
			}
			const option = target.getAttribute("data-run-option");
			if (option !== null) chooseRunOption(option, target.value);
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
		// Stessa regola del compositore della conversazione — Invio manda,
		// Maiusc+Invio va a capo — perché è lo stesso gesto sullo stesso tipo di
		// campo: due scorciatoie diverse per scrivere a un agente sarebbero due
		// cose da ricordare al posto di una.
		container.addEventListener("keydown", (e) => {
			const input = e.target.closest(".run-composer-input");
			if (!input) return;
			if (e.key !== "Enter") return;
			if (e.isComposing || e.keyCode === 229) return;
			if (e.shiftKey || e.altKey) return;
			e.preventDefault();
			runDraft = input.value;
			sendRunMessage();
		});
	}

	prdBtn.addEventListener("click", openPRD);
	prdModalClose.addEventListener("click", closePRD);
	prdModal.addEventListener("click", (e) => {
		if (e.target === prdModal && isTopModal(prdModal)) closePRD();
	});
	prdEditBtn.addEventListener("click", enterPrdEditMode);
	prdCancelBtn.addEventListener("click", exitPrdEditMode);
	prdForm.addEventListener("submit", onSavePRD);
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(prdModal)) closePRD();
	});

	metricsBtn.addEventListener("click", openMetrics);
	metricsModalClose.addEventListener("click", closeMetrics);
	metricsModal.addEventListener("click", (e) => {
		if (e.target === metricsModal && isTopModal(metricsModal)) closeMetrics();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(metricsModal)) closeMetrics();
	});

	newSpecBtn.addEventListener("click", openNewSpec);
	newSpecModalClose.addEventListener("click", closeNewSpec);
	newSpecCancel.addEventListener("click", closeNewSpec);
	newSpecModal.addEventListener("click", (e) => {
		if (e.target === newSpecModal && isTopModal(newSpecModal)) closeNewSpec();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(newSpecModal)) closeNewSpec();
	});
	newSpecForm.addEventListener("submit", onCreateSpec);
	specDraftModeManual.addEventListener("click", () => showSpecDraftMode(false));
	specDraftModeAssisted.addEventListener("click", enterAssistedMode);

	newWorkspaceBtn.addEventListener("click", openNewWorkspace);
	newWorkspaceModalClose.addEventListener("click", closeNewWorkspace);
	newWorkspaceCancel.addEventListener("click", closeNewWorkspace);
	newWorkspaceModal.addEventListener("click", (e) => {
		if (e.target === newWorkspaceModal && isTopModal(newWorkspaceModal)) closeNewWorkspace();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(newWorkspaceModal)) closeNewWorkspace();
	});
	newWorkspaceWorktreeEnabled.addEventListener("change", syncWorktreeFields);
	newWorkspaceForm.addEventListener("submit", onCreateWorkspace);

	workspacesBtn.addEventListener("click", openWorkspaces);
	// The indicator is the shortcut that names; #workspaces-btn stays the
	// explicit menu entry. With no workspace open the button is disabled and the
	// click never fires, which is exactly right: in home mode the modal is
	// missing its own form, moved into the home by enterNoWorkspaceMode.
	workspaceIndicator.addEventListener("click", openWorkspaces);
	workspacesModalClose.addEventListener("click", closeWorkspaces);
	workspacesModal.addEventListener("click", (e) => {
		if (e.target === workspacesModal && isTopModal(workspacesModal)) closeWorkspaces();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(workspacesModal)) closeWorkspaces();
	});
	workspacesAddForm.addEventListener("submit", onAddWorkspace);
	// The modal and the home draw the same rows, so they are acted on by the
	// same handler: one list rendering, one set of actions.
	function onWorkspaceListClick(e) {
		const opener = e.target.closest("[data-open]");
		if (opener) {
			// An entry with no id cannot be opened: posting an empty segment would
			// answer with a 404 that says nothing about why.
			if (opener.dataset.open) {
				openWorkspace(opener.dataset.open, opener.dataset.openName || "");
			}
			return;
		}
		const btn = e.target.closest("[data-remove]");
		if (!btn) return;
		removeWorkspace(btn.dataset.remove, btn.dataset.removeName || "");
	}
	workspacesList.addEventListener("click", onWorkspaceListClick);
	workspaceHomeEl.addEventListener("click", onWorkspaceListClick);
	workspaceHomeCreate.addEventListener("click", openNewWorkspace);

	configBtn.addEventListener("click", openConfig);
	configModalClose.addEventListener("click", closeConfig);
	configCancelBtn.addEventListener("click", closeConfig);
	configValidateBtn.addEventListener("click", validateConfig);
	configSaveBtn.addEventListener("click", saveConfig);
	configModal.addEventListener("click", (e) => {
		if (e.target === configModal && isTopModal(configModal)) closeConfig();
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && isTopModal(configModal)) closeConfig();
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

	topbarMoreBtn.addEventListener("click", toggleTopbarMoreMenu);
	// A click outside the collector closes it. The mockups dropdown is *inside*
	// this menu, so the condition is containment in #topbar-more and not in any
	// one entry: opening the nested submenu keeps the menu that holds it open.
	document.addEventListener("click", (e) => {
		if (!topbarMoreDropdown.contains(e.target)) closeTopbarMoreMenu();
	});
	// And so does pressing one of its entries: a menu that stays open over the
	// modal its own entry just opened would be the behaviour these buttons did
	// not have when they sat in the bar. The button that opens the collector
	// and the one that opens the nested submenu are the two exceptions — the
	// first toggles it, the second opens something inside it.
	topbarMoreMenu.addEventListener("click", (e) => {
		if (e.target.closest("#mockups-btn")) return;
		if (e.target.closest("button, a")) closeTopbarMoreMenu();
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
			refreshBoardAndStatus();
		}
	});

	// The board and the status strip answer two different questions about the
	// same workspace, so every explicit refresh asks both.
	function refreshBoardAndStatus() {
		// Nothing to refresh while no workspace is open: both routes would be
		// refused, and the refusal is not news the home has to report.
		if (noWorkspaceMode()) return;
		loadBoard();
		loadWorkspaceStatus();
		// The conversation is re-read from the beginning, not resumed: after a
		// workspace change the one to show is the one of the workspace now open,
		// and the cursor of the previous one means nothing here (AC-5).
		loadConversation();
		// The runs strip is read once now instead of waiting for the next tick:
		// an explicit refresh asks every question the page answers.
		loadWorkspaceRuns();
		// Including which workspace is being looked at.
		refreshWorkspaceIdentity();
	}

	// Boot is a fork, not a sequence. The page asks which workspace it serves
	// before drawing anything, because a page that does not know must not draw a
	// board: the board it would draw would belong to no workspace.
	async function boot() {
		let view = null;
		try {
			view = await apiGet("/api/workspaces");
		} catch (err) {
			applyWorkspaceIdentity(null);
			enterNoWorkspaceMode();
			resetConversationState();
			renderWorkspaceHomeView(null, TEXT.loadFailed(err.message || err));
			return;
		}
		// Written once, before the fork, so the two branches cannot diverge on
		// what the page says it is looking at (AC-1, AC-2, AC-5).
		applyWorkspaceIdentity(view);
		if (view && view.open) {
			loadBoard();
			loadWorkspaceStatus();
			loadConversation();
			// The runs strip follows what the workspace is running. It is started
			// only here, where a workspace is known to be open: without one the
			// route would be refused, and the refusal is not news the home has to
			// report.
			loadWorkspaceRuns();
			loadMockups();
			connectBoardStream();
			// Per ultimo, perché è l'indirizzo a decidere: se porta una spec, il
			// dettaglio si riapre sopra la board appena chiesta, e la voce
			// d'ingresso resta la board per chi premerà Indietro.
			restoreSpecFromLocation();
			return;
		}
		enterNoWorkspaceMode();
		resetConversationState();
		renderWorkspaceHomeView(view, "");
	}

	// No workspace open: the board, the status strip and every control that
	// presupposes a workspace disappear, and the add form moves out of the now
	// unreachable modal into the home — so "add an existing workspace" stays a
	// single implementation instead of becoming two.
	function enterNoWorkspaceMode() {
		document.body.classList.add("no-workspace");
		// Nothing is running in a workspace that is not open: the runs strip
		// stops reading and forgets what it last knew, so no leftover run
		// survives the workspace it belonged to.
		resetWorkspaceRunsState();
		// And nothing is recommended for a workspace that is not open: the strip
		// forgets its last step too, so no step of the workspace just left can be
		// put back on screen by the next redraw (AC-5).
		resetWorkspaceStatusState();
		workspaceHomeActions.insertBefore(
			workspacesAddForm,
			workspaceHomeActions.firstChild,
		);
		workspaceHomeEl.classList.remove("hidden");
	}

	function noWorkspaceMode() {
		return document.body.classList.contains("no-workspace");
	}

	function renderWorkspaceHomeView(view, message) {
		workspaceHomeList.innerHTML = WorkspaceHome.renderWorkspaceHome(view, {
			formatTime: formatExecutionTime,
			message,
		});
	}

	// Who is looking at what. This is the one place that writes the identity
	// onto the page: label, full path and tab title all come out of the same
	// answer, so they cannot contradict each other. The text is written with
	// textContent and with the title attribute: the name comes from the user's
	// disk and never ends up inside markup.
	function applyWorkspaceIdentity(view) {
		const id = WorkspaceIdentity.resolveWorkspaceIdentity(view);
		workspaceIndicatorName.textContent = id.label;
		workspaceIndicator.title = id.tooltip;
		workspaceIndicator.disabled = !id.actionable;
		workspaceIndicator.classList.toggle("is-empty", !id.open);
		document.title = id.documentTitle;
	}

	// A failure does not change what is read: the identity on screen stays the
	// last known one, which is better than a label that empties itself.
	async function refreshWorkspaceIdentity() {
		try {
			applyWorkspaceIdentity(await apiGet("/api/workspaces"));
		} catch (_) {
			/* keep the last known identity */
		}
	}

	// ---- Workspace shell layout (US-057) -------------------------------------
	//
	// Which view is on screen is not decided here. workspace-layout.js resolves
	// it from three facts — which view was asked for, is a spec open, is the
	// viewport narrow — and this section does nothing but hold those three facts
	// and write onto the elements the classes the module hands back. Not one
	// visibility rule is written twice.
	//
	// This is what makes AC-1 true by construction: the view starts at the
	// module's own DEFAULT_VIEW, so a workspace opens on the conversation
	// whatever order the rest of this file happens to run in. And it is what
	// makes AC-3 free: changing view hides a pane, it never unmounts it, so the
	// conversation's history and the text being typed survive untouched.

	let shellView = WorkspaceLayout.DEFAULT_VIEW; // which view owns the primary column
	let specOpen = false; // a spec is open in the detail pane
	let shellNarrow = false; // the viewport is below the module's breakpoint

	// The breakpoint is read from the module and never written here: app.css
	// declares the same number in its media query, so the two cannot answer
	// differently about the same viewport.
	const shellNarrowQuery =
		typeof window.matchMedia === "function"
			? window.matchMedia(`(max-width: ${WorkspaceLayout.NARROW_MAX_WIDTH}px)`)
			: null;

	function onShellWidthChange(e) {
		shellNarrow = !!(e && e.matches);
		applyShellLayout();
	}

	if (shellNarrowQuery) {
		shellNarrow = shellNarrowQuery.matches;
		if (typeof shellNarrowQuery.addEventListener === "function") {
			shellNarrowQuery.addEventListener("change", onShellWidthChange);
		} else if (typeof shellNarrowQuery.addListener === "function") {
			// Safari before 14 exposes only the deprecated form.
			shellNarrowQuery.addListener(onShellWidthChange);
		}
	}

	// applyPaneState writes one pane's answer, and writes nothing but classes:
	// no pane is ever emptied or rebuilt by a change of view, which is exactly
	// why the conversation's history and draft survive one. `hidden` says the
	// pane has nothing to show at all — the class every existing guard in this
	// file already reads on the spec detail — is-visible / is-hidden say whether
	// a pane that does have something to show is the one on screen, and
	// is-overlay says it is drawn over the conversation rather than in its
	// place.
	function applyPaneState(el, pane) {
		if (!el || !pane) return;
		el.classList.toggle(WorkspaceLayout.PANE_VISIBLE_CLASS, pane.visible);
		el.classList.toggle(WorkspaceLayout.PANE_HIDDEN_CLASS, !pane.visible);
		el.classList.toggle(WorkspaceLayout.PANE_OVERLAY_CLASS, pane.overlay === true);
		el.classList.toggle("hidden", !pane.present);
	}

	// The thread rail is not a pane of the primary column: it is the companion
	// of one view. It gets the same two state classes a pane gets — the shell
	// already hides `is-hidden` children — and never the `hidden` class, which
	// the rail's own renderer owns by emptying itself when there is no
	// workspace to index.
	function applyRailState(el, rail) {
		if (!el || !rail) return;
		el.classList.toggle(WorkspaceLayout.PANE_VISIBLE_CLASS, rail.visible);
		el.classList.toggle(WorkspaceLayout.PANE_HIDDEN_CLASS, !rail.visible);
	}

	// La barra è permanente, in entrambe le larghezze, e ha due linguette e
	// basta: Conversazione · Board, con quella corrente accesa. Le linguette non
	// correnti vengono dalla lista di commutatori del modulo; quella corrente
	// dal modulo stesso, così le due non possono contraddirsi. Il dettaglio spec
	// non è una linguetta — si apre da una card della board e si chiude col
	// comando di ritorno — quindi mentre occupa la colonna `current` è nullo e
	// nessuna delle due risulta accesa: accenderne una direbbe che si sta
	// guardando qualcosa che non si sta guardando.
	function renderShellViews(switchers, current) {
		if (!shellViewsEl) return;
		const buttons = (switchers || []).map((s) => ({
			view: s.view,
			label: s.label,
			current: false,
		}));
		// Senza linguetta corrente — è il dettaglio spec a occupare la colonna —
		// la barra resta quella di sempre, con le sue due voci, e nessuna accesa.
		if (current) {
			buttons.push({
				view: current.view,
				label: current.label,
				current: true,
			});
		}
		// L'ordine di lettura delle linguette: Conversazione · Board. È
		// presentazione, non una decisione su che cosa sia su schermo — quella
		// resta al modulo, che dichiara le stesse due linguette in TABS.
		const order = ["conversation", "board"];
		buttons.sort((a, b) => order.indexOf(a.view) - order.indexOf(b.view));
		shellViewsEl.innerHTML = buttons
			.map(
				(b) =>
					`<button type="button" class="view-tab${b.current ? " is-current" : ""}" role="tab" aria-selected="${b.current}" data-shell-view="${escapeHtml(b.view)}">${escapeHtml(b.label)}</button>`,
			)
			.join("");
	}

	function applyShellLayout() {
		const layout = WorkspaceLayout.resolveLayout({
			view: shellView,
			specOpen,
			narrow: shellNarrow,
		});
		// The module normalises the view — an unknown one, or the spec detail
		// with no spec open, falls back to the home — and that normalisation is
		// the only authority on what the view is.
		shellView = layout.view;
		shellEl.classList.remove(
			WorkspaceLayout.SHELL_CLASS_WIDE,
			WorkspaceLayout.SHELL_CLASS_NARROW,
		);
		shellEl.classList.add(layout.shellClass);
		applyPaneState(conversationEl, layout.panes.conversation);
		applyPaneState(boardEl, layout.panes.board);
		applyPaneState(modal, layout.panes.spec);
		applyRailState(conversationsRailEl, layout.rail);
		const currentTab = layout.currentTab
			? {
					view: layout.currentTab,
					label: layout.panes[layout.currentTab].label || layout.currentTab,
				}
			: null;
		renderShellViews(layout.switchers, currentTab);
		// The return control is the close button the spec pane has always had:
		// it leaves the detail and puts the board back. It is named after what
		// it reaches, in the module's own words.
		if (layout.back) {
			const label = TEXT.backTo(layout.back.label);
			modalClose.setAttribute("aria-label", label);
			modalClose.setAttribute("title", label);
			// E la parola sul comando è quella del modulo, non una seconda
			// scritta a mano qui: dove il comando porta lo sa lui solo.
			if (modalCloseLabel) modalCloseLabel.textContent = layout.back.label;
		}
	}

	// Reaching a view implies a state, and the module already said which one:
	// the board is reached by leaving the spec, and leaving the spec is what
	// closeModal does — panels, snapshots and review included. Nothing else
	// happens here: no pane is unmounted and no panel is redrawn.
	function setShellView(view) {
		if (WorkspaceLayout.VIEWS.indexOf(view) === -1) return;
		shellView = view;
		if (view === "board" && specOpen) {
			// Il ritorno alla board passa dalla cronologia: leaveSpecDetail
			// torna indietro di una voce e sarà popstate a chiudere davvero,
			// così questo comando e il tasto Indietro del browser percorrono la
			// stessa strada. Il layout lo applica closeModal, dall'altra parte.
			leaveSpecDetail();
			return;
		}
		applyShellLayout();
	}

	// ---- Cronologia del browser ----------------------------------------------
	//
	// Aprire la card di una spec è una navigazione, non un dettaglio interno:
	// chi la apre si aspetta che il tasto Indietro riporti alla board, che
	// Avanti ci ritorni dentro e che l'indirizzo dica quale spec sta guardando —
	// così ricaricare la pagina, o mandare il link a qualcuno, arriva allo
	// stesso posto invece che a una board qualunque.
	//
	// La regola è una sola, e tutto il resto discende da lì: **aprire aggiunge
	// una voce, e a chiudere il dettaglio è sempre e solo popstate**. Il comando
	// di ritorno, Escape e la linguetta Board non chiudono niente per conto
	// proprio: tornano indietro di una voce, e la chiusura arriva dall'evento,
	// esattamente come quando a premere è il browser. Un cammino solo, quindi la
	// cronologia non può descrivere una schermata diversa da quella su video.
	//
	// Il percorso dell'indirizzo non cambia mai: la spec viaggia come parametro
	// di ricerca, che il server serve la stessa pagina comunque.

	/** Il nome del parametro che porta la spec aperta nell'indirizzo. */
	const SPEC_QUERY_PARAM = "spec";
	/**
	 * Vero mentre apriamo o chiudiamo il dettaglio *in risposta* a un popstate:
	 * lì la cronologia è già dove deve essere, e riscriverla aggiungerebbe voci
	 * che nessuno ha chiesto.
	 */
	let navigatingFromHistory = false;

	function historySupported() {
		return (
			typeof window !== "undefined" &&
			!!window.history &&
			typeof window.history.pushState === "function"
		);
	}

	/** Il codice della spec aperta secondo l'indirizzo, o la stringa vuota. */
	function specCodeInLocation() {
		try {
			return (
				new URL(window.location.href).searchParams.get(SPEC_QUERY_PARAM) || ""
			);
		} catch (_) {
			return "";
		}
	}

	/** L'indirizzo che descrive la schermata con quella spec aperta (o nessuna). */
	function locationWithSpec(code) {
		const url = new URL(window.location.href);
		if (code) url.searchParams.set(SPEC_QUERY_PARAM, code);
		else url.searchParams.delete(SPEC_QUERY_PARAM);
		return url.pathname + url.search + url.hash;
	}

	/** La voce di cronologia corrente descrive un dettaglio spec aperto. */
	function historyHoldsSpec() {
		return !!(
			historySupported() &&
			window.history.state &&
			window.history.state.archetipoSpec
		);
	}

	// Aprire una spec dalla board aggiunge una voce: è quella che il tasto
	// Indietro toglierà. Passare da una spec all'altra senza uscire dal
	// dettaglio riscrive invece la voce che c'è già, altrimenti tornare alla
	// board costerebbe tanti Indietro quante card sono state guardate.
	function rememberSpecInHistory(code) {
		if (!historySupported() || navigatingFromHistory || !code) return;
		const state = { archetipoSpec: code };
		const url = locationWithSpec(code);
		if (historyHoldsSpec()) window.history.replaceState(state, "", url);
		else window.history.pushState(state, "", url);
	}

	// Il dettaglio si è chiuso da sé — spec eliminata, approvata, integrata —
	// senza che nessuno abbia chiesto di navigare. La voce che lo descriveva non
	// può restare: si riscrive come board, così un Indietro successivo non
	// riapre una spec che non c'è più.
	function forgetSpecInHistory() {
		if (!historySupported() || navigatingFromHistory) return;
		if (!historyHoldsSpec()) return;
		window.history.replaceState({ archetipoSpec: "" }, "", locationWithSpec(""));
	}

	// L'unico modo in cui l'interfaccia lascia il dettaglio: si torna indietro
	// di una voce e sarà popstate a chiudere. Senza una voce nostra in
	// cronologia — un browser senza History API — si chiude e basta.
	function leaveSpecDetail() {
		if (historyHoldsSpec()) {
			window.history.back();
			return;
		}
		closeModal();
	}

	// Il tasto Indietro (e Avanti) del browser. È qui che il dettaglio si apre e
	// si chiude davvero: la voce dice quale spec descrive, e la schermata si
	// mette d'accordo con lei.
	if (historySupported()) {
		window.addEventListener("popstate", (e) => {
			const code =
				(e.state && e.state.archetipoSpec) || specCodeInLocation() || "";
			navigatingFromHistory = true;
			try {
				if (code) {
					if (!specOpen || currentSpecCode !== code) openEditor(code);
				} else if (specOpen) {
					closeModal();
				}
			} finally {
				navigatingFromHistory = false;
			}
		});
	}

	// Chi arriva da un link con una spec dentro deve avere dove tornare: la voce
	// d'ingresso viene riscritta come board, e openEditor aggiunge sopra quella
	// del dettaglio. Così il primo Indietro porta alla board di questo
	// workspace, non fuori dall'applicazione.
	function restoreSpecFromLocation() {
		const code = specCodeInLocation();
		if (!code) return;
		if (historySupported()) {
			window.history.replaceState(
				{ archetipoSpec: "" },
				"",
				locationWithSpec(""),
			);
		}
		openEditor(code);
	}

	// The switcher is redrawn on every layout change, so the handler lives on
	// its container and each button declares the view it produces.
	if (shellViewsEl) {
		shellViewsEl.addEventListener("click", (e) => {
			const btn = e.target.closest("[data-shell-view]");
			if (!btn) return;
			setShellView(btn.dataset.shellView);
		});
	}

	applyShellLayout();

	// ---- Workspace runs strip (US-055) ---------------------------------------
	//
	// What the workspace is running right now, as a full-width strip above the
	// primary column: it is on screen in every view, so knowing what is in
	// flight never costs a change of view, and it takes no width away from the
	// conversation.
	//
	// It is read on a loop with the same
	// discipline as the conversation panel further down: ticks never overlap, a
	// failed read leaves the last known list on screen instead of claiming that
	// nothing is running, and the loop gives up after the same number of
	// consecutive failures rather than polling forever.
	//
	// Nothing about a run is decided here. The rows, the words in them and the
	// waiting mark all come from /api/workspace/runs through the pure renderer
	// in workspace-runs.js; this section reads the payload only to answer where
	// a press should lead, which is the one thing the renderer refuses to do.
	//
	// And where a press leads is no longer a panel by default: since US-060 the
	// payload says which conversation a run was started from and at which line,
	// so the strip and its topbar indicator lead to that point in the
	// conversation whenever the entry names one, and to the panel that mounts
	// the run only when it does not.

	const WORKSPACE_RUNS_RAIL_OPEN_KEY = "archetipo.runsOpen";
	const WORKSPACE_RUNS_POLL_MS = 2000;
	const WORKSPACE_RUNS_POLL_FAILURE_LIMIT = 3;

	let workspaceRunsView = null; // last read of GET /api/workspace/runs
	let workspaceRunsTimer = null; // interval following the workspace's runs
	let workspaceRunsBusy = false; // a poll is in flight: ticks never overlap
	let workspaceRunsFailures = 0; // consecutive failed reads, for the give-up threshold
	// L'ultimo markup scritto nella striscia. Serve a non riscrivere il pannello
	// quando la passata di polling non ha cambiato nulla: dentro la striscia c'è
	// un role="status" (la decisione in attesa), e reinserirlo identico ogni
	// pochi secondi significherebbe riannunciarlo a ogni giro allo screen reader.
	let workspaceRunsHTML = null;

	function stopWorkspaceRunsPolling() {
		if (workspaceRunsTimer === null) return;
		clearInterval(workspaceRunsTimer);
		workspaceRunsTimer = null;
	}

	// The runs on screen belong to the workspace that was open: leaving it
	// forgets them rather than leaving them to be read as the new one's.
	function resetWorkspaceRunsState() {
		stopWorkspaceRunsPolling();
		workspaceRunsView = null;
		workspaceRunsBusy = false;
		workspaceRunsFailures = 0;
		if (workspaceRunsEl) workspaceRunsEl.innerHTML = "";
		workspaceRunsHTML = null;
		renderRunsAttention();
	}

	// The indicator lives in the topbar so it stays visible whatever view is on
	// screen, and it counts from the very payload the strip draws — one read,
	// one truth about what is waiting.
	function renderRunsAttention() {
		if (!runsAttentionEl) return;
		const waiting = WorkspaceRuns.awaitingCount(workspaceRunsView);
		runsAttentionEl.classList.toggle("hidden", waiting === 0);
		runsAttentionEl.textContent = waiting ? TEXT.waitingCount(waiting) : "";
		runsAttentionEl.setAttribute(
			"title",
			waiting ? TEXT.waitingHint : "",
		);
	}

	// Aperta o chiusa la striscia lo decide chi guarda, e la scelta sopravvive
	// al ridisegno: il pannello viene riscritto da zero a ogni passata, quindi
	// lo stato non può stare nel DOM. Chiusa di partenza — una riga — perché
	// quello che serve a colpo d'occhio è quante run ci sono e su cosa stanno.
	let workspaceRunsExpanded = false;
	try {
		workspaceRunsExpanded =
			localStorage.getItem(WORKSPACE_RUNS_RAIL_OPEN_KEY) === "1";
	} catch (_) {
		/* ignore */
	}

	function renderWorkspaceRunsPanel() {
		if (!workspaceRunsEl) return;
		const html = WorkspaceRuns.renderWorkspaceRuns(workspaceRunsView, {
			expanded: workspaceRunsExpanded,
		});
		if (html !== workspaceRunsHTML) {
			workspaceRunsHTML = html;
			workspaceRunsEl.innerHTML = html;
		}
		renderRunsAttention();
	}

	// loadWorkspaceRuns is the entry point of every fresh read — boot and every
	// explicit refresh — and it is what (re)starts the loop.
	async function loadWorkspaceRuns() {
		if (!workspaceRunsEl) return;
		if (noWorkspaceMode()) {
			resetWorkspaceRunsState();
			return;
		}
		let view;
		try {
			view = await apiGet("/api/workspace/runs");
		} catch (_) {
			// The strip is an addition to the work, not a precondition of it: a
			// viewer that cannot answer must not stop anyone from working, so
			// what was last known stays on screen and no toast is raised.
			return;
		}
		workspaceRunsView = view || null;
		workspaceRunsFailures = 0;
		renderWorkspaceRunsPanel();
		startWorkspaceRunsPolling();
	}

	function startWorkspaceRunsPolling() {
		stopWorkspaceRunsPolling();
		workspaceRunsTimer = setInterval(async () => {
			if (noWorkspaceMode()) {
				stopWorkspaceRunsPolling();
				return;
			}
			if (workspaceRunsBusy) return;
			workspaceRunsBusy = true;
			let view;
			try {
				view = await apiGet("/api/workspace/runs");
			} catch (_) {
				workspaceRunsBusy = false;
				workspaceRunsFailures += 1;
				// The list survives the failed read: a run that could not be
				// asked about is not a run that has ended.
				if (workspaceRunsFailures >= WORKSPACE_RUNS_POLL_FAILURE_LIMIT) {
					stopWorkspaceRunsPolling();
				}
				return;
			}
			workspaceRunsBusy = false;
			workspaceRunsFailures = 0;
			workspaceRunsView = view || null;
			renderWorkspaceRunsPanel();
		}, WORKSPACE_RUNS_POLL_MS);
	}

	function workspaceRunByID(id) {
		const runs = (workspaceRunsView && workspaceRunsView.runs) || [];
		return runs.find((run) => run && run.id === id) || null;
	}

	// Where a row leads is judged here, from what the row declares: the renderer
	// navigates nowhere. Only one execution panel is mounted at a time, so
	// reaching a decision means opening the panel that mounts that very run —
	// the spec detail for a spec run, and for a workspace run the panel it was
	// started from, which is the New spec modal for the assisted creation and
	// the PRD modal for every other workspace action.
	//
	// A run that was started from a conversation is reached in that
	// conversation, at the line that asked for it (US-060, AC-6): the panel that
	// mounts the whole log is still there behind the block's own control, but
	// the place where the wait is happening is the flow that produced it.
	function openRunTarget(entry) {
		const row = entry && typeof entry === "object" ? entry : {};
		const scope = row.scope || "";
		const code = row.spec_code || "";
		const id = row.id || "";
		if (row.conversation_id) {
			revealConversationRun(row.conversation_id, row.anchor_event_id);
			return;
		}
		if (scope === "spec" && code) {
			openEditor(code);
			return;
		}
		const run = workspaceRunByID(id);
		if (run && run.action === SPEC_DRAFT_ACTION) {
			openNewSpec();
			enterAssistedMode();
			return;
		}
		openPRD();
	}

	if (workspaceRunsEl) {
		// `toggle` non risale la gerarchia: la cattura è l'unico modo di sentirlo
		// su un <details> che il ridisegno sostituisce di continuo.
		workspaceRunsEl.addEventListener(
			"toggle",
			(e) => {
				const panel = e.target;
				if (!panel || !panel.classList.contains("ws-runs-panel")) return;
				workspaceRunsExpanded = panel.open;
				try {
					localStorage.setItem(
						WORKSPACE_RUNS_RAIL_OPEN_KEY,
						panel.open ? "1" : "0",
					);
				} catch (_) {
					/* ignore */
				}
			},
			true,
		);

		// The strip is redrawn on every poll, so the handler lives on the section
		// and each row carries its own identity in its data attributes.
		workspaceRunsEl.addEventListener("click", (e) => {
			const row = e.target.closest("[data-run-id]");
			if (!row) return;
			// The row declares its identity; the entry behind it declares where
			// it came from. Both are handed over, so a run started from a
			// conversation is reached there and one started from the board keeps
			// leading exactly where it always did.
			const id = row.dataset.runId || "";
			openRunTarget(
				Object.assign(
					{
						scope: row.dataset.runScope || "",
						spec_code: row.dataset.runSpec || "",
						id,
					},
					workspaceRunByID(id) || {},
				),
			);
		});
	}

	if (runsAttentionEl) {
		// The indicator leads to the first entry that is waiting: seeing that
		// something needs an answer and reaching the answer are one gesture. The
		// whole entry is handed over, so when the run was born in a conversation
		// the press lands on the very block that is waiting.
		runsAttentionEl.addEventListener("click", () => {
			const runs = (workspaceRunsView && workspaceRunsView.runs) || [];
			const waiting = runs.find((run) => run && run.awaiting_response);
			if (!waiting) return;
			openRunTarget(waiting);
		});
	}

	boot();

	let boardReloadTimer = null;
	function scheduleBoardReload() {
		clearTimeout(boardReloadTimer);
		boardReloadTimer = setTimeout(() => {
			// The tick also arrives from a workspace switch: SwitchWorkspace
			// publishes on the broker, which lives on the server and not on the
			// session, so the connection survives the change. Knowing which
			// workspace is being looked at destroys no edit in progress, which
			// is why it sits before the guards (AC-3).
			refreshWorkspaceIdentity();
			// The recommended step sits before the guards because behind them it
			// froze for as long as a window stayed open — exactly what starting
			// a workspace-scoped step produces, since that opens the PRD modal
			// (AC-3). Reading it redraws the thread that hosts it, and that
			// redraw destroys no edit in progress: renderConversationPanel
			// carries the draft, the caret and the scroll across every render,
			// which is what makes it safe to run here.
			loadWorkspaceStatus();
			// Skip while something is being edited: reloading would discard the
			// user's in-progress edits. The next event after the edit ends will
			// bring the board back in sync.
			//
			// What is guarded is the edit, not the reading. The spec detail is a
			// pane beside the board now, so an open spec is no longer a reason to
			// skip — guarding on it would freeze the board for as long as anyone
			// keeps a spec open. Its two edit forms are guarded instead.
			if (!specForm.classList.contains("hidden")) return;
			if (!planForm.classList.contains("hidden")) return;
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
		// Inside .board-columns, like every other message the board shows: .board
		// itself is only the stack of header and row, and carries no padding.
		boardEl.innerHTML =
			`<div class="board-columns"><div class="empty-board">${escapeHtml(TEXT.boardLoading)}</div></div>`;
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
				: TEXT.noEpicsYet;
		} catch (err) {
			// A board that could not be read cannot vouch for its epics either.
			newSpecBtn.disabled = true;
			newSpecBtn.title = TEXT.backlogUnreadable;
			boardEl.innerHTML = `<div class="board-columns"><div class="empty-board">${escapeHtml(TEXT.boardError(err.message || err))}</div></div>`;
		}
	}

	// ---- Recommended step ---------------------------------------------------
	//
	// Which step comes next is a server verdict: /api/workspace/status derives
	// it from the installed Archetipo and from the real state of the workspace.
	// Nothing here decides which step follows which, or what unlocks a refused
	// one — every word drawn comes from the payload, and it is drawn at the tail
	// of the thread, by conversation.js. This section reads the payload, keeps
	// it, and performs the start it names; whether the step can be started at
	// all is decided by the pure module in workspace-status.js.

	// The last payload, kept because the navigation must read the step from the
	// payload and not from the DOM it produced.
	let workspaceStatusSnapshot = null;
	// Lo stesso payload, ma scopato sulla spec della conversazione aperta: in
	// una conversazione su una spec la card in coda al thread raccomanda il
	// prossimo passo di *quella* spec, non quello del workspace, che punta alla
	// prima carta della board. Tiene anche il codice per cui è stato letto, così
	// una risposta arrivata tardi non veste i panni di un'altra conversazione.
	let conversationStatusSnapshot = null; // { specCode, view }

	async function loadWorkspaceStatus() {
		let view;
		try {
			view = await apiGet("/api/workspace/status");
		} catch (_) {
			// The recommended step is an addition to the workspace, not a
			// precondition of it: a viewer that cannot answer must not stop anyone
			// from working, so the block simply disappears and no toast is raised.
			resetWorkspaceStatusState();
			return;
		}
		workspaceStatusSnapshot = view;
		// Il passo scopato segue la stessa cadenza: ogni board_changed che
		// aggiorna il workspace aggiorna anche la lettura della spec aperta.
		loadConversationNextStep();
		// The step lives in the thread, so the read that updates it must redraw
		// the thread: otherwise the block would stay on the step of a moment ago
		// until the next poll of the conversation.
		renderConversationPanel();
	}

	// La spec di cui parla la conversazione aperta, vuota per una conversazione
	// libera. È il payload a dirlo, mai una deduzione della pagina.
	function currentConversationSpecCode() {
		return conversationView &&
			conversationView.conversation &&
			conversationView.conversation.spec_code
			? String(conversationView.conversation.spec_code)
			: "";
	}

	// loadConversationNextStep legge il prossimo passo della spec della
	// conversazione aperta, con la stessa disciplina di loadWorkspaceStatus: una
	// lettura che non riesce toglie la card, non ferma nessuno.
	async function loadConversationNextStep() {
		const specCode = currentConversationSpecCode();
		if (!specCode) {
			if (conversationStatusSnapshot) {
				conversationStatusSnapshot = null;
				renderConversationPanel();
			}
			return;
		}
		let view;
		try {
			view = await apiGet(
				`/api/workspace/status?spec=${encodeURIComponent(specCode)}`,
			);
		} catch (_) {
			conversationStatusSnapshot = null;
			renderConversationPanel();
			return;
		}
		// Un cambio di conversazione ha vinto mentre la lettura era in volo:
		// il passo appartiene alla conversazione di adesso, non a quella di un
		// momento fa.
		if (specCode !== currentConversationSpecCode()) return;
		conversationStatusSnapshot = { specCode, view };
		renderConversationPanel();
	}

	// nextStepStatusView è la sorgente unica del passo in coda al thread: la
	// lettura scopata sulla spec della conversazione quando c'è, quella del
	// workspace altrimenti. Il render e l'avvio leggono entrambi da qui, così
	// non possono divergere: quel che si vede è quel che parte.
	function nextStepStatusView() {
		const specCode = currentConversationSpecCode();
		if (
			specCode &&
			conversationStatusSnapshot &&
			conversationStatusSnapshot.specCode === specCode
		) {
			return conversationStatusSnapshot.view;
		}
		return workspaceStatusSnapshot;
	}

	// The recommended step belongs to the workspace that produced it: leaving
	// the last snapshot in memory would mean being able to offer, and to start,
	// the step of a workspace that is no longer open. Forgetting it is one
	// implementation, shared by the unreadable answer and by the closed
	// workspace, so the two can never drift apart.
	function resetWorkspaceStatusState() {
		workspaceStatusSnapshot = null;
		conversationStatusSnapshot = null;
		// Forgetting the step of a workspace that is no longer open must take it
		// off the screen too: the block lives in the thread, so the thread is
		// what has to be redrawn.
		renderConversationPanel();
	}

	// startNextStep runs the recommended step, and runs it through the single
	// dispatch path of this application.
	//
	// It takes the user to the target's panel first — the spec detail, or the
	// workspace actions of the PRD modal — and only then delegates to
	// startPanelAction, the very function the board presses. So "the same action
	// the board would start" is not a resemblance to be checked case by case: it
	// is the same line of code, the same route, the same "one press, one
	// execution" guarantee, and the same panel where the run is then watched.
	//
	// The gesture is therefore "go to the target and start", not "start where
	// you stand": the run must be started somewhere it can be followed.
	// "Una pressione, una run" qui è del bottone e non del server: il server non
	// rifiuta più una seconda azione sulla stessa spec — un'azione è una
	// conversazione, e una run ferma su una domanda avrebbe tenuto chiusa la
	// spec a ogni altro passo finché qualcuno non rispondeva. startPanelAction
	// disabilita il bottone che le viene passato, ed è lì che un debounce sta.
	async function startNextStep(target, button) {
		let expected;
		if (target.scope === "spec" && target.code) {
			expected = specContext(target.code);
			await openEditor(target.code);
		} else if (target.scope === "workspace") {
			expected = WORKSPACE_CONTEXT;
			await openPRD();
		} else {
			// A scope this viewer has never seen does not get an invented path.
			// nextStepDispatch already refuses a spec-scoped step with no spec,
			// so this is defence in depth for any other caller.
			return;
		}
		// The panel that answered must be the target's own. Checking only that
		// *some* panel is mounted is not enough: a workspace with no action to
		// offer unmounts nothing when a spec detail is the mounted context, and
		// the workspace action would then be posted to the spec's own route.
		if (panelContext !== expected || !panelStartURL) {
			showToast(TEXT.stepNotStartable, "err");
			return;
		}
		// The thread the step was pressed in travels with the start: a run asked
		// for inside a conversation has to be readable there, and not only in
		// the workspace strip. It is the id the panel is showing, whatever it
		// is — a conversation that has ended ties nothing, and the server is
		// what decides that.
		await startPanelAction(target.action, button, conversationsCurrentId);
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
		// The counters live in the board's own header now (US-061 AC-6), and
		// renderBoard emits that header on every draw: resolving the elements at
		// boot would keep three nodes that the next draw has already replaced.
		// Before the first draw there is nothing to write on, and that is an
		// answer too.
		const total_el = document.getElementById("stat-total");
		const progress_el = document.getElementById("stat-progress");
		const done_el = document.getElementById("stat-done");
		if (total_el) total_el.textContent = total;
		if (progress_el) progress_el.textContent = progress;
		if (done_el) done_el.textContent = done;
	}

	// The three counters of the backlog, read where the backlog is (US-061
	// AC-6). They are emitted on every draw and in both branches — an empty
	// backlog is a backlog with three zeros, not a board with no header —
	// because renderBoard clears #board before drawing anything.
	function boardStatsHeader() {
		return `<header class="board-stats" id="board-stats">
			<span class="stat"><span class="stat-num" id="stat-total">&mdash;</span><span class="stat-label">${TEXT.statStories}</span></span>
			<span class="stat-sep">/</span>
			<span class="stat"><span class="stat-num" id="stat-progress">&mdash;</span><span class="stat-label">${TEXT.statInFlight}</span></span>
			<span class="stat-sep">/</span>
			<span class="stat"><span class="stat-num" id="stat-done">&mdash;</span><span class="stat-label">${TEXT.statDone}</span></span>
		</header>`;
	}

	function renderBoard(view) {
		// Two children, always: the counters and the row that holds the columns.
		// The header is emitted in both branches — an empty backlog is a backlog
		// with three zeros, not a board with no header — and the row is what
		// scrolls sideways, so the counters stay put while the columns are
		// scanned.
		boardEl.innerHTML = `${boardStatsHeader()}<div class="board-columns"></div>`;
		const columnsEl = boardEl.querySelector(".board-columns");
		if (!view.columns || view.columns.length === 0) {
			// The message is *added* under the header, never in its place: an
			// empty backlog must still be able to say it is empty in numbers.
			columnsEl.insertAdjacentHTML(
				"beforeend",
				`<div class="empty-board">${TEXT.boardNoBacklog}</div>`,
			);
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
			columnsEl.appendChild(columnEl);

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
		// La card è l'oggetto interattivo primario del prodotto: apre il
		// dettaglio della spec. Finora lo faceva solo col puntatore. Il nome
		// accessibile è dichiarato, e non lasciato al contenuto, perché dentro
		// la card c'è anche il comando di cancellazione e la sua etichetta
		// finirebbe nel nome di quello che si sta per aprire.
		el.setAttribute("role", "button");
		el.setAttribute("tabindex", "0");
		el.setAttribute(
			"aria-label",
			`${spec.code}: ${spec.title || TEXT.untitled}`,
		);
		const epicCode = spec.epic && spec.epic.code ? spec.epic.code : "";
		const epicTooltip =
			spec.epic && spec.epic.title
				? `${epicCode} — ${spec.epic.title}`
				: epicCode;
		el.innerHTML = `
            <button type="button" class="card-delete-btn" title="${escapeHtml(TEXT.deleteSpec(spec.code))}" aria-label="${escapeHtml(TEXT.deleteSpec(spec.code))}">
                <svg width="13" height="13" viewBox="0 0 16 16" aria-hidden="true"><path d="M3.5 4.5h9" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M6 2.5h4" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><path d="M5 4.5v8h6v-8" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/><path d="M6.75 6.5v4.25M9.25 6.5v4.25" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>
            </button>
            <div class="card-top">
                <span class="card-code">${escapeHtml(spec.code)}</span>
                ${spec.rework ? `<span class="rework-badge" title="${escapeHtml(TEXT.reworkBadge)}">⟲ rework</span>` : ""}
                ${spec.priority ? `<span class="priority-badge priority-${escapeHtml(spec.priority)}">${escapeHtml(spec.priority)}</span>` : ""}
            </div>
            <div class="card-title">${escapeHtml(spec.title || TEXT.untitled)}</div>
            <div class="card-meta">
                <span class="card-epic" title="${escapeHtml(epicTooltip)}">${escapeHtml(epicCode)}</span>
                <span class="card-points">${Number.isFinite(spec.points) ? spec.points + " pt" : ""}</span>
            </div>
            ${spec.branch ? `<div class="card-branch" title="${escapeHtml(TEXT.branchTitle)}">⎇ ${escapeHtml(spec.branch)}</div>` : ""}
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
		el.addEventListener("keydown", (event) => {
			if (event.key !== "Enter" && event.key !== " ") return;
			// Un tasto premuto dentro il comando di cancellazione è suo: la card
			// non deve aprire il dettaglio della spec che si sta cancellando.
			if (event.target !== el) return;
			event.preventDefault();
			openEditor(spec.code);
		});
		return el;
	}

	function emptyHint(columnId) {
		const e = document.createElement("div");
		e.className = "empty-column";
		e.textContent =
			columnId === "done" ? TEXT.emptyDone : TEXT.emptyColumn;
		return e;
	}

	async function onDragMove(evt) {
		const sourceColumn =
			evt.from && evt.from.dataset ? evt.from.dataset.columnId : "";
		const targetColumn =
			evt.to && evt.to.dataset ? evt.to.dataset.columnId : "";
		if (sourceColumn !== "review" || targetColumn !== "done") {
			showToast(TEXT.dragOnlyReviewToDone, "err");
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

		// Un drop su Done è un'approvazione a tutti gli effetti: chiede la
		// stessa conferma del bottone Approva, perché un trascinamento
		// accidentale non deve poter chiudere una spec. Se la conferma non
		// arriva, la card torna dov'era.
		if (!confirmApproval(code)) {
			if (boardSnapshot) {
				renderBoard(boardSnapshot);
				updateStats(boardSnapshot);
			} else {
				await loadBoard();
			}
			return;
		}

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
			showToast(TEXT.movedToDone(code, targetColumn), "ok");
			await loadBoard();
			await loadWorkspaceStatus();
		} catch (err) {
			showToast(TEXT.moveFailed(err.message || err), "err");
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
		// La card che si apre è una voce di cronologia: da qui in poi il tasto
		// Indietro del browser riporta alla board, e l'indirizzo dice quale spec
		// è sotto gli occhi. Sta prima di ogni attesa perché il flag che
		// distingue "aperta da chi guarda" da "aperta dalla cronologia" vale
		// solo finché non si cede il turno.
		rememberSpecInHistory(code);
		mountExecutionPanels({
			context: specContext(code),
			startURL: `/api/spec/${encodeURIComponent(code)}/execution`,
			actions: storyActions,
			execution: storyExecution,
			run: storyRun,
			modelChoice: storyModelChoice,
			settle: settleSpecExecution,
		});
		modalTitle.textContent = TEXT.specTitle(code);
		// Choosing a spec is a selection made inside the view that is on screen,
		// so the module's reducer says what comes next: the detail becomes the
		// view of the primary column, and in a narrow window the overlay the
		// choice was made in closes because the view changed. Nothing is
		// unmounted, so the conversation behind it is exactly as it was.
		const nextSpec = WorkspaceLayout.nextViewAfterSelection(
			{ view: shellView, specOpen, narrow: shellNarrow },
			"spec",
		);
		shellView = nextSpec.view;
		specOpen = nextSpec.specOpen;
		applyShellLayout();
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
			loadModelChoice(specContext(code));
			fillSpecForm(currentSpecSnapshot);
			fillPlanView(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			fillPlanForm(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			updateReviewTabVisibility(currentSpecSnapshot);
			specStatus.textContent = "";
		} catch (err) {
			specStatus.textContent = TEXT.loadFailed(err.message || err);
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
				`<span class="meta-chip blocked">${escapeHtml(TEXT.blockedBy(s.blocked_by.join(", ")))}</span>`,
			);
		const mockup = findMockupForSpec(s.code);
		if (mockup)
			metaParts.push(
				`<a class="meta-chip mockup-link" href="${escapeHtml(mockup.url)}" target="_blank" rel="noopener">↗ mockup</a>`,
			);
		specViewMeta.innerHTML = metaParts.join("");
		specDeleteBtn.title = s.code ? TEXT.deleteSpec(s.code) : TEXT.deleteStory;
		specDeleteBtn.setAttribute(
			"aria-label",
			s.code ? TEXT.deleteSpec(s.code) : TEXT.deleteStory,
		);
		specBodyView.innerHTML = marked.parse(s.body || TEXT.noDescription);
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
		planBodyView.innerHTML = marked.parse(body || TEXT.noPlan);
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
                <textarea class="task-desc" rows="2" placeholder="${escapeHtml(TEXT.taskBodyPlaceholder)}">${escapeHtml(getTaskMarkdown(t))}</textarea>
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
            <td><button type="button" class="remove-task" aria-label="${escapeHtml(TEXT.removeTask)}">&times;</button></td>
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
			showToast(TEXT.specUpdated(currentSpecCode), "ok");
			currentSpecSnapshot = { ...(currentSpecSnapshot || {}), ...patch };
			fillSpecView(currentSpecSnapshot);
			showSpecView();
			await loadBoard();
		} catch (err) {
			specStatus.textContent = TEXT.saveFailed(err.message || err);
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
			showToast(TEXT.planUpdated(currentSpecCode), "ok");
			currentPlanSnapshot = {
				plan_body: payload.plan_body,
				tasks: payload.tasks,
			};
			fillPlanView(currentPlanSnapshot.plan_body, currentPlanSnapshot.tasks);
			showPlanView();
		} catch (err) {
			planStatus.textContent = TEXT.saveFailed(err.message || err);
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
		// Leaving the spec gives the primary column back to the board, and the
		// module's reducer is what says so. The name stays: it has several call
		// sites, and renaming it would widen the change for nothing.
		const nextBoard = WorkspaceLayout.nextViewAfterSelection(
			{ view: shellView, specOpen, narrow: shellNarrow },
			"board",
		);
		shellView = nextBoard.view;
		specOpen = nextBoard.specOpen;
		applyShellLayout();
		// La cronologia smette di descrivere un dettaglio che non c'è più.
		// Quando a chiudere è stato il tasto Indietro questo non fa nulla: la
		// voce l'ha già tolta il browser.
		forgetSpecInHistory();
		unmountExecutionPanels(specContext(currentSpecCode));
		currentSpecCode = null;
		currentSpecSnapshot = null;
		currentPlanSnapshot = null;
		reviewComments = [];
		reviewLoaded = false;
		reviewTab.classList.add("hidden");
	}

	// ---- Toast ---------------------------------------------------------------
	//
	// Il toast è l'unico canale di feedback generale dell'app e veicola anche
	// gli errori: deve essere annunciato dalle tecnologie assistive (role="status"
	// sulla regione contenitrice, che resta sempre nel DOM), deve durare
	// abbastanza da poter leggere il dettaglio del server, deve poter essere
	// chiuso, e un secondo messaggio non deve cancellare il primo — si mette in
	// coda e aspetta il suo turno.

	/** Quanto resta a schermo un messaggio prima di lasciare il posto al successivo. */
	const TOAST_DURATION = 4500;
	/** Oltre questa soglia la coda smette di crescere: i messaggi più vecchi non letti si perdono. */
	const TOAST_QUEUE_MAX = 4;

	const toastQueue = [];
	let toastTimer = null;
	let toastShowing = false;

	function showToast(msg, kind) {
		const text = typeof msg === "string" ? msg : String(msg ?? "");
		if (!text) return;
		// Un messaggio identico a quello in coda non viene ripetuto: durante un
		// run gli stessi errori arrivano ravvicinati e la coda si riempirebbe di
		// copie della stessa frase.
		const last = toastQueue[toastQueue.length - 1];
		if (last && last.text === text && last.kind === kind) return;
		if (toastQueue.length >= TOAST_QUEUE_MAX) toastQueue.shift();
		toastQueue.push({ text, kind });
		if (!toastShowing) drainToastQueue();
	}

	function drainToastQueue() {
		const next = toastQueue.shift();
		if (!next) {
			toastShowing = false;
			hideToast();
			return;
		}
		toastShowing = true;
		toastMessage.textContent = next.text;
		toast.classList.remove("hidden", "ok", "err", "warn");
		if (next.kind) toast.classList.add(next.kind);
		clearTimeout(toastTimer);
		toastTimer = setTimeout(drainToastQueue, TOAST_DURATION);
	}

	function hideToast() {
		clearTimeout(toastTimer);
		toastTimer = null;
		toast.classList.add("hidden");
		toastMessage.textContent = "";
	}

	/** Il dismiss esplicito chiude il messaggio corrente e passa subito al successivo. */
	function dismissToast() {
		clearTimeout(toastTimer);
		toastTimer = null;
		drainToastQueue();
	}

	toastDismiss.addEventListener("click", dismissToast);

	// ---- Bozze non ancora confermate ----------------------------------------
	//
	// Le modali che ospitano un form o un editor ospitano lavoro: una spec
	// scritta a mano o proposta dall'agente, una configurazione compilata campo
	// per campo, un PRD riscritto. Esc e il click sul fondale sono gesti facili
	// da fare per sbaglio e finora scartavano tutto senza chiedere nulla.
	// Da qui in poi ogni modale con contenuto dichiara come si legge il proprio
	// stato: se lo stato differisce da quello di partenza, la chiusura chiede
	// conferma. Il submit in corso resta protetto a parte — è una cosa diversa,
	// e non sostituisce questa.

	/**
	 * Crea la sentinella di una modale. `readState` restituisce una stringa che
	 * riassume il contenuto corrente: due stringhe uguali sono due contenuti
	 * indistinguibili per chi guarda, ed è l'unica cosa che serve sapere.
	 */
	function createDirtyGuard(readState) {
		let baseline = null;
		function current() {
			try {
				return readState();
			} catch (_) {
				// Uno stato illeggibile non deve poter bloccare una chiusura:
				// meglio una modale che si chiude che una che non si chiude più.
				return null;
			}
		}
		return {
			/** Fissa lo stato di partenza. Va chiamata quando la modale è pronta. */
			arm() {
				baseline = current();
			},
			/** Dimentica lo stato di partenza: dopo un salvataggio o una chiusura già decisa. */
			disarm() {
				baseline = null;
			},
			isDirty() {
				if (baseline === null) return false;
				const now = current();
				return now !== null && now !== baseline;
			},
			/**
			 * True quando la chiusura può procedere: o non c'è niente da perdere,
			 * o chi guarda ha detto di procedere lo stesso.
			 */
			allowsClose() {
				if (!this.isDirty()) return true;
				return window.confirm(TEXT.discardDraft);
			},
		};
	}

	// ---- Modali: fuoco trattenuto e restituito ------------------------------
	//
	// aria-modal="true" è una promessa: finché la modale è aperta il resto della
	// pagina non c'è. Finora era soltanto scritta — Tab usciva nella board
	// dietro, e nessuno riportava il fuoco al comando che aveva aperto. Questa è
	// l'unica implementazione della promessa e vale per tutte le modali: una
	// pila, perché una modale può aprirne un'altra (la casa dei workspace apre
	// la creazione), e in quel caso la sola che risponde è quella in cima.

	const FOCUSABLE_SELECTOR = [
		"a[href]",
		"button:not([disabled])",
		"input:not([disabled])",
		"select:not([disabled])",
		"textarea:not([disabled])",
		'[tabindex]:not([tabindex="-1"])',
	].join(",");

	/** Gli elementi che possono davvero ricevere il fuoco dentro `root`, in ordine di Tab. */
	function focusableWithin(root) {
		return Array.from(root.querySelectorAll(FOCUSABLE_SELECTOR)).filter(
			(el) =>
				!el.closest(".hidden") &&
				!el.hasAttribute("inert") &&
				(el.offsetWidth > 0 ||
					el.offsetHeight > 0 ||
					el.getClientRects().length > 0),
		);
	}

	/** Le modali aperte, dalla prima all'ultima. Solo l'ultima è viva. */
	const openModals = [];

	function topModal() {
		return openModals.length ? openModals[openModals.length - 1] : null;
	}

	/** True quando `root` è la modale che deve rispondere a Esc e al fondale. */
	function isTopModal(root) {
		const top = topModal();
		return !!top && top.root === root;
	}

	/**
	 * Dichiara aperta una modale: sospende lo sfondo, ricorda a chi restituire
	 * il fuoco e lo porta sul primo elemento utile. Va chiamata dopo aver tolto
	 * `hidden`, altrimenti non c'è ancora niente su cui posare il fuoco.
	 */
	function enterModal(root, preferred) {
		if (!root || openModals.some((entry) => entry.root === root)) return;
		const opener =
			document.activeElement instanceof HTMLElement
				? document.activeElement
				: null;
		// Lo sfondo diventa inerte: non è raggiungibile da Tab, dal puntatore né
		// dalle tecnologie assistive. La regione del toast resta fuori, perché
		// il feedback deve poter essere annunciato anche sopra una modale.
		const suspended = Array.from(document.body.children).filter(
			(el) => el !== root && el !== toastRegion && !el.hasAttribute("inert"),
		);
		suspended.forEach((el) => el.setAttribute("inert", ""));
		openModals.push({ root, opener, suspended });
		const target =
			(preferred && !preferred.disabled && preferred) ||
			focusableWithin(root)[0] ||
			null;
		if (target) target.focus();
	}

	/** Dichiara chiusa una modale: rende di nuovo vivo lo sfondo e restituisce il fuoco. */
	function leaveModal(root) {
		const index = openModals.findIndex((entry) => entry.root === root);
		if (index === -1) return;
		const [entry] = openModals.splice(index, 1);
		entry.suspended.forEach((el) => el.removeAttribute("inert"));
		// Il fuoco torna dov'era solo se quel comando esiste ancora ed è
		// raggiungibile: dopo un'azione che ha ridisegnato la pagina può non
		// esserci più, e insistere lo manderebbe nel vuoto.
		const opener = entry.opener;
		if (opener && opener.isConnected && !opener.closest("[inert]")) {
			opener.focus();
		}
	}

	// Tab e Maiusc+Tab girano dentro la modale in cima. In cattura, così nessun
	// gestore di campo può portarsi via il tasto prima.
	document.addEventListener(
		"keydown",
		(e) => {
			if (e.key !== "Tab") return;
			const top = topModal();
			if (!top) return;
			const items = focusableWithin(top.root);
			if (items.length === 0) {
				// Una modale senza niente da mettere a fuoco trattiene comunque:
				// uscire da qui sarebbe uscire dietro a uno schermo inerte.
				e.preventDefault();
				return;
			}
			const first = items[0];
			const last = items[items.length - 1];
			const active = document.activeElement;
			if (!top.root.contains(active)) {
				e.preventDefault();
				(e.shiftKey ? last : first).focus();
				return;
			}
			if (e.shiftKey && active === first) {
				e.preventDefault();
				last.focus();
			} else if (!e.shiftKey && active === last) {
				e.preventDefault();
				first.focus();
			}
		},
		true,
	);

	/** Lo stato di un form come stringa: nomi e valori nell'ordine dei campi. */
	function formState(form) {
		if (!form) return "";
		const parts = [];
		Array.from(form.elements).forEach((el) => {
			if (!el.name) return;
			const type = el.type || "";
			if (type === "submit" || type === "button" || type === "file") return;
			if (type === "checkbox" || type === "radio") {
				parts.push(`${el.name}:${el.checked ? el.value || "on" : ""}`);
				return;
			}
			parts.push(`${el.name}:${el.value}`);
		});
		return parts.join(" ");
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
		reviewDiff.innerHTML = `<div class="review-empty">${escapeHtml(TEXT.diffLoading)}</div>`;
		try {
			const [diff, review] = await Promise.all([
				apiGet(`/api/spec/${encodeURIComponent(currentSpecCode)}/diff`),
				apiGet(`/api/spec/${encodeURIComponent(currentSpecCode)}/review`),
			]);
			reviewComments = (review && review.comments) || [];
			renderReviewBranch(diff);
			renderDossier(review || {});
			renderDiff(diff);
		} catch (err) {
			reviewLoaded = false;
			reviewDiff.innerHTML = `<div class="review-empty">${escapeHtml(TEXT.diffError(err.message || err))}</div>`;
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

	// renderDossier shows the evidence a provider prepared for this spec, and the
	// verdict already taken on it. Its three states are rendered explicitly, and
	// the empty one says what to do about it: an absent panel would read as a
	// broken page rather than as work not yet done.
	function renderDossier(review) {
		const dossier = review && review.dossier;
		const verdict = review && review.verdict;
		const parts = [];
		if (verdict) {
			const decided =
				verdict.decision === "approved"
					? TEXT.verdictApproved
					: TEXT.verdictChangesRequested;
			const when = verdict.decided_at
				? escapeHtml(TEXT.verdictWhen(verdict.decided_at))
				: "";
			const from = verdict.execution_id
				? escapeHtml(TEXT.verdictFrom(verdict.execution_id))
				: "";
			parts.push(
				`<div class="review-verdict">${decided}${when}${from}</div>`,
			);
		}
		if (!dossier) {
			parts.push(
				`<div class="review-empty">${TEXT.noDossier}</div>`,
			);
			reviewDossier.innerHTML = parts.join("");
			reviewApproveBtn.disabled = false;
			reviewApproveBtn.title = TEXT.approveHint;
			return;
		}
		const head = [];
		if (dossier.execution_id)
			head.push(
				`<span class="review-chip">execution ${escapeHtml(dossier.execution_id)}</span>`,
			);
		if (dossier.prepared_at)
			head.push(
				`<span class="review-chip">${escapeHtml(dossier.prepared_at)}</span>`,
			);
		parts.push(`<div class="review-dossier-head">${head.join("")}</div>`);
		if (dossier.summary)
			parts.push(
				`<p class="review-dossier-summary">${escapeHtml(dossier.summary)}</p>`,
			);
		const criteria = dossier.criteria || [];
		if (criteria.length > 0) {
			const rows = criteria
				.map((c) => {
					const verdictClass = `criterion-${escapeHtml(c.verdict || "unclear")}`;
					const note = c.note
						? `<span class="criterion-note">${escapeHtml(c.note)}</span>`
						: "";
					return `<li><span class="criterion-badge ${verdictClass}">${escapeHtml(c.verdict || "")}</span><span class="criterion-id">${escapeHtml(c.id || "")}</span>${note}</li>`;
				})
				.join("");
			parts.push(`<ul class="review-criteria">${rows}</ul>`);
		}
		const blockers = dossier.blockers || [];
		if (blockers.length > 0) {
			const items = blockers
				.map((b) => `<li>${escapeHtml(b)}</li>`)
				.join("");
			parts.push(
				`<div class="review-blockers"><strong>Blockers</strong><ul>${items}</ul></div>`,
			);
		}
		reviewDossier.innerHTML = parts.join("");
		// The verdict stays the person's, so the button is never removed — but the
		// interface does not invite closing an increment the dossier itself
		// declares blocked. Requesting changes stays available in every case.
		reviewApproveBtn.disabled = blockers.length > 0;
		reviewApproveBtn.title =
			blockers.length > 0
				? TEXT.approveBlocked(blockers.length)
				: TEXT.approveHint;
	}

	function renderDiff(diff) {
		reviewDiff.innerHTML = "";
		const files = diff.files || [];
		if (files.length === 0) {
			reviewDiff.innerHTML =
				`<div class="review-empty">${escapeHtml(TEXT.diffEmpty)}</div>`;
			return;
		}
		files.forEach((file) => reviewDiff.appendChild(renderFileDiff(file)));
		renderAllComments();
	}

	function renderFileDiff(file) {
		const path = file.new_path || file.old_path || TEXT.unknownPath;
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
            <span class="diff-comment-add" title="${escapeHtml(TEXT.addComment)}">+</span>
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
            <button type="button" class="diff-comment-del" aria-label="${escapeHtml(TEXT.deleteComment)}">&times;</button>
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
            <textarea class="diff-comment-input" rows="3" placeholder="${escapeHtml(TEXT.commentPlaceholder)}"></textarea>
            <div class="diff-composer-actions">
                <button type="button" class="primary-btn diff-comment-save">${escapeHtml(TEXT.commentSave)}</button>
                <button type="button" class="ghost-btn diff-comment-cancel">${escapeHtml(TEXT.commentCancel)}</button>
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
			showToast(TEXT.saveFailed(err.message || err), "err");
		}
	}

	async function confirmAndDeleteSpec(code, title) {
		if (!code) return false;
		const label = title ? `${code} — ${title}` : code;
		const confirmed = window.confirm(
			TEXT.deleteSpecConfirm(label),
		);
		if (!confirmed) return false;
		try {
			await apiDelete(`/api/spec/${encodeURIComponent(code)}`);
			showToast(TEXT.specDeleted(code), "ok");
			if (currentSpecCode === code) {
				closeModal();
			}
			await loadBoard();
			await loadWorkspaceStatus();
			return true;
		} catch (err) {
			showToast(TEXT.deleteFailed(err.message || err), "err");
			return false;
		}
	}

	async function onRequestChanges() {
		if (!currentSpecCode) return;
		if (reviewComments.length === 0) {
			showToast(TEXT.commentsFirst, "err");
			return;
		}
		if (
			!window.confirm(
				TEXT.requestChangesConfirm(reviewComments.length, currentSpecCode),
			)
		)
			return;
		reviewStatus.textContent = TEXT.requestingChanges;
		reviewStatus.className = "status-msg";
		try {
			const res = await apiPost(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/request-changes`,
				{},
			);
			showToast(
				TEXT.changesRequested(currentSpecCode, res.comments_moved),
				"ok",
			);
			closeModal();
			await loadBoard();
		} catch (err) {
			reviewStatus.textContent = TEXT.failed(err.message || err);
			reviewStatus.className = "status-msg err";
		}
	}

	// L'approvazione si può chiedere da due strade — il bottone Approva e il
	// trascinamento di una card da Review a Done — ma è la stessa azione
	// irreversibile, quindi deve fare la stessa domanda. La domanda sta scritta
	// qui una volta sola: due formulazioni diverse sarebbero due promesse
	// diverse sullo stesso effetto.
	function confirmApproval(code) {
		return window.confirm(
			TEXT.approveConfirm(code),
		);
	}

	async function onApprove() {
		if (!currentSpecCode) return;
		if (!confirmApproval(currentSpecCode)) return;
		reviewStatus.textContent = TEXT.approving;
		reviewStatus.className = "status-msg";
		try {
			const res = await apiPost(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/approve`,
				{},
			);
			showToast(
				res.integrated
					? TEXT.approvedIntegrated(currentSpecCode)
					: TEXT.approved(currentSpecCode),
				"ok",
			);
			closeModal();
			await loadBoard();
		} catch (err) {
			reviewStatus.textContent = TEXT.failed(err.message || err);
			reviewStatus.className = "status-msg err";
		}
	}

	async function onIntegrate() {
		if (!currentSpecCode) return;
		if (
			!window.confirm(
				TEXT.integrateConfirm(currentSpecCode),
			)
		)
			return;
		reviewStatus.textContent = TEXT.integrating;
		reviewStatus.className = "status-msg";
		try {
			await apiPost(
				`/api/spec/${encodeURIComponent(currentSpecCode)}/integrate`,
				{},
			);
			showToast(TEXT.integrated(currentSpecCode), "ok");
			closeModal();
			await loadBoard();
		} catch (err) {
			reviewStatus.textContent = TEXT.failed(err.message || err);
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
		configValidation.textContent = message || TEXT.configNotTested;
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
		enterModal(configModal);
		activateConfigTab(activeConfigTab);
		setConfigStatus(TEXT.configLoading, null);
		setConfigValidation(TEXT.configNotTested, null);
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
					TEXT.providerNoneUsable,
					"err",
				);
			} else {
				setExecutionStatus("", null);
			}
		} catch (err) {
			executionProviders = [];
			executionDefault = null;
			renderProviderGrid();
			setExecutionStatus(TEXT.loadFailed(err.message || err), "err");
		}
	}

	function renderProviderGrid() {
		if (!executionProviders.length) {
			executionProviderGrid.innerHTML =
				`<p class="config-copy">${escapeHtml(TEXT.providerNoneRegistered)}</p>`;
			executionFields.innerHTML = "";
			return;
		}
		executionProviderGrid.innerHTML = executionProviders
			.map((p) => {
				const caps = (p.capabilities || []).join(", ") || TEXT.providerNoCapability;
				// `available` is a server verdict: the panel only renders it. An
				// unusable provider stays visible — with its reason — but cannot be
				// picked, so nobody saves a default that could never run.
				const unavailable = p.available === false;
				const reason = p.unavailable_reason || TEXT.providerRuntimeUnusable;
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

	// The fields themselves are drawn by the pure provider-fields module: what
	// belongs here is only putting the produced markup on the page.
	function renderProviderFields(provider, values) {
		if (!provider) {
			executionFields.innerHTML = "";
			return;
		}
		executionFields.innerHTML = ProviderFields.renderProviderFields(
			provider,
			values,
		);
		bindModelFieldRedraw(provider);
	}

	// Choosing another model changes which options that model declares, so the
	// section has to be drawn again. The values already typed into the other
	// fields are collected first and carried over, so switching model never
	// costs the reader what they had entered.
	function bindModelFieldRedraw(provider) {
		if (!provider || !provider.model_field) return;
		const control = executionFields.querySelector(
			`[name="provider_${cssEscape(provider.model_field)}"]`,
		);
		if (!control) return;
		control.addEventListener("change", () => {
			const values = collectProviderConfig(provider);
			values[provider.model_field] = control.value;
			renderProviderFields(provider, values);
		});
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
		// The options are read from the controls that are actually in the page,
		// never from the union of what the catalog declares. That is what makes
		// an option disappear from the saved configuration as soon as another
		// model is chosen: after the redraw its control no longer exists, so
		// there is nothing to send.
		executionFields
			.querySelectorAll("[data-provider-option]")
			.forEach((control) => {
				const name = control.getAttribute("data-provider-option");
				if (!name) return;
				const raw = control.value.trim();
				// Same rule as the fields: empty means "use the provider
				// default", and sending it as an empty string would be a value.
				if (raw === "") return;
				config[name] = raw;
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
		const input = label.querySelector("input, select");
		if (input) input.focus();
	}

	function updateProviderSummary() {
		configSummaryProvider.textContent = executionDefault
			? executionDefault.id
			: TEXT.providerNotConfigured;
	}

	async function saveExecutionProvider() {
		const id = selectedProviderID();
		if (!id) {
			setExecutionStatus(TEXT.providerPickFirst, "err");
			return;
		}
		const provider = findProvider(id);
		if (!provider) return;
		// The server remains the authority on this; refusing here only spares the
		// user a round trip that could not succeed.
		if (provider.available === false) {
			setExecutionStatus(
				TEXT.providerRejected(
					provider.unavailable_reason || TEXT.providerRuntimeUnusable,
				),
				"err",
			);
			return;
		}
		markProviderFieldError("");
		setExecutionStatus(TEXT.providerSaving, null);
		try {
			await apiPut("/api/execution/provider/default", {
				id,
				config: collectProviderConfig(provider),
			});
			// Reload from the server instead of trusting the local form: what the
			// panel shows is then exactly what was persisted.
			await loadExecutionProviders();
			setExecutionStatus(TEXT.providerSavedDefault(id), "ok");
			showToast(TEXT.providerSetToast(id), "ok");
		} catch (err) {
			markProviderFieldError(err.field || "");
			setExecutionStatus(TEXT.providerRejected(err.message || err), "err");
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
		panelModelChoice = mount.modelChoice || null;
		panelStartURL = mount.startURL;
		panelSettle = mount.settle;
		if (panelActions) panelActions.innerHTML = "";
		// A choice belongs to the panel that was open when it was made: mounting
		// another one starts again from what the workspace declares.
		modelChoiceView = null;
		modelChoiceSelection = null;
		if (panelModelChoice) panelModelChoice.innerHTML = "";
		// Prima di renderExecution: è la condizione che quella legge, e montare
		// un pannello nuovo non deve ereditare il verdetto del precedente.
		executionThreadID = "";
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

	// ---- Model choice for a single run ---------------------------------------

	// The panel says which model the next run would use and lets this one run
	// depart from it. Everything shown comes from the route: which model is
	// inherited, which ones can be chosen, which options each of them declares,
	// and — when the list cannot be had — why. Nothing about any provider is
	// decided here.
	async function loadModelChoice(ctx) {
		if (!ctx || !panelModelChoice) return;
		let view;
		try {
			view = await apiGet("/api/execution/model-choice");
		} catch {
			// Not being able to read the choice is not being unable to start:
			// the section stays empty and the action chips keep working.
			return;
		}
		if (panelContext !== ctx) return;
		modelChoiceView = view || null;
		renderModelChoicePanel();
	}

	function renderModelChoicePanel() {
		if (!panelModelChoice) return;
		const render =
			window.ProviderFields && window.ProviderFields.renderModelChoice;
		if (!render) return;
		panelModelChoice.innerHTML = render(modelChoiceView, modelChoiceSelection);
	}

	// The option names one catalog entry declares. Unknown entry, or an entry
	// declaring nothing, both read as "no option survives".
	function declaredOptionNames(modelID) {
		const models =
			modelChoiceView && Array.isArray(modelChoiceView.models)
				? modelChoiceView.models
				: [];
		const model = models.find(
			(m) => m && String(m.id || "") === String(modelID || ""),
		);
		const options =
			model && Array.isArray(model.options) ? model.options : [];
		return options.filter(Boolean).map((o) => String(o.name || ""));
	}

	// The values the controls are showing right now: the ones already chosen
	// here, or — nobody having chosen yet — the ones the route reports.
	function currentModelChoice() {
		if (modelChoiceSelection) {
			return {
				model: String(modelChoiceSelection.model || ""),
				options: Object.assign({}, modelChoiceSelection.options),
			};
		}
		const view = modelChoiceView || {};
		return {
			model: view.model === undefined || view.model === null ? "" : String(view.model),
			options: Object.assign({}, view.options || {}),
		};
	}

	// Choosing a model drops the options the new one does not declare: an option
	// of the model just left would otherwise travel to a model that never
	// offered it.
	function chooseRunModel(value) {
		const current = currentModelChoice();
		const model = value === undefined || value === null ? "" : String(value);
		const declared = declaredOptionNames(model);
		const options = {};
		declared.forEach((name) => {
			if (current.options[name] !== undefined && current.options[name] !== null)
				options[name] = String(current.options[name]);
		});
		modelChoiceSelection = { model, options };
		renderModelChoicePanel();
	}

	// Choosing an option touches that option alone: the controls around it keep
	// what they are showing, so no redraw is needed.
	function chooseRunOption(name, value) {
		const current = currentModelChoice();
		current.options[String(name)] =
			value === undefined || value === null ? "" : String(value);
		modelChoiceSelection = { model: current.model, options: current.options };
	}

	// What this run must carry beyond the action: the choice, and only when it
	// really departs from what the workspace already declares. A run nobody
	// touched sends nothing, so using the workspace values is a property of the
	// request and not of what this client happens to remember.
	function runModelOverride() {
		if (!modelChoiceSelection || !modelChoiceView) return null;
		const inheritedModel =
			modelChoiceView.model === undefined || modelChoiceView.model === null
				? ""
				: String(modelChoiceView.model);
		const chosen = {
			model: String(modelChoiceSelection.model || ""),
			options: normalizeOptionMap(modelChoiceSelection.options),
		};
		const inheritedOptions = normalizeOptionMap(modelChoiceView.options);
		if (
			chosen.model === inheritedModel &&
			sameOptionMap(chosen.options, inheritedOptions)
		)
			return null;
		return { model: chosen.model, model_options: chosen.options };
	}

	// An option left empty is an option not chosen: it must compare equal to an
	// option that was never there at all.
	function normalizeOptionMap(options) {
		const out = {};
		const bag = options && typeof options === "object" ? options : {};
		Object.keys(bag).forEach((key) => {
			const value = bag[key] === undefined || bag[key] === null ? "" : String(bag[key]);
			if (value !== "") out[key] = value;
		});
		return out;
	}

	function sameOptionMap(a, b) {
		const keys = Object.keys(a);
		if (keys.length !== Object.keys(b).length) return false;
		return keys.every((key) => b[key] === a[key]);
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
				`<span class="story-actions-empty">${escapeHtml(TEXT.noActionAvailable)}</span>`;
			return;
		}
		panelActions.innerHTML = list.map(renderSpecActionChip).join("");
	}

	// A chip that cannot run must say what unlocks it, as visible text and not
	// only as a tooltip: a chip that is off and mute is exactly the inert action
	// the spec forbids. The title stays, but it is no longer the only place the
	// sentence exists.
	function renderSpecActionChip(action) {
		const label = escapeHtml(action.label || action.id);
		const id = escapeHtml(action.id);
		const body = `<span class="action-chip-label">${label}</span><code class="action-chip-id">${id}</code>`;
		if (action.runnable) {
			return `<button type="button" class="action-chip action-chip-run" data-action-id="${id}" title="${escapeHtml(TEXT.runAction(action.label || action.id))}">${body}</button>`;
		}
		const unlock = action.unlocked_by || action.unavailable_reason || "";
		if (!unlock) {
			return `<span class="action-chip">${body}</span>`;
		}
		const escaped = escapeHtml(unlock);
		return `<span class="action-chip" title="${escaped}">${body}<span class="action-chip-unlock">${escaped}</span></span>`;
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
	//
	// Non apre più nessun thread prima di avviare, e non gli serve: la run *è*
	// una conversazione. La sessione che il provider apre per lei è già quella
	// che una conversazione legge e comanda, e il server la tiene come tale
	// sotto l'id dell'esecuzione. Prima ne apriva uno lui, e quel thread era un
	// secondo processo d'agente, inerte, aperto solo per avere un posto in cui
	// raccontare il lavoro di un altro.
	//
	// conversationID resta, e nomina il thread *da cui viene la pressione* — il
	// passo consigliato premuto in coda a una conversazione. Serve alla run per
	// essere ricordata lì dentro, nel punto del discorso che l'ha chiesta, e a
	// nient'altro: una pressione che non viene da nessuna conversazione non ne
	// inventa una, perché non c'è nessun discorso a cui attaccarla.
	async function startPanelAction(actionID, button, conversationID) {
		if (!actionID || !panelContext || !panelStartURL) return;
		const ctx = panelContext;
		const url = panelStartURL;
		if (button) button.disabled = true;
		try {
			const body = { action: actionID };
			const from = String(conversationID || "");
			if (from && liveConversationEntries().some((entry) => entry.id === from)) {
				body.conversation_id = from;
			}
			const override = runModelOverride();
			if (override) {
				body.model = override.model;
				body.model_options = override.model_options;
			}
			const record = await apiPost(url, body);
			if (panelContext !== ctx) return;
			// The departure was for this run only: the panel goes back to what
			// the workspace declares, re-read from the server rather than
			// assumed to have stayed what it was.
			modelChoiceSelection = null;
			renderModelChoicePanel();
			loadModelChoice(ctx);
			await followExecution(record, ctx);
			// Il thread della run è la run: si raggiunge con il suo stesso id.
			await revealThread(record && record.id ? record.id : "");
		} catch (err) {
			showToast(err.message || String(err), "err");
			if (button) button.disabled = false;
		}
	}

	// La spec di cui parla il pannello montato, o niente per il contesto di
	// workspace. Il contesto è una stringa sola — `spec:<codice>` — e questo è
	// l'unico posto che la rilegge.
	function specCodeOfContext(context) {
		const value = String(context || "");
		return value.startsWith("spec:") ? value.slice("spec:".length) : "";
	}

	// Porta sullo schermo il filo in cui la run è appena nata.
	//
	// L'elenco si rilegge subito perché altrimenti non si rileggerebbe affatto:
	// la rail si aggiorna a ogni giro del poll della conversazione, e quel poll
	// gira solo mentre se ne sta seguendo una. Chi ha premuto un passo dai
	// dettagli di una spec senza avere un filo aperto è esattamente chi non ha
	// quel poll acceso, ed è chi non vedeva niente.
	//
	// Il pannello passa al filo nuovo soltanto se non ne sta già seguendo uno
	// vivo: chi stava leggendo una conversazione non se la vede sostituire da
	// sotto, e chi non stava leggendo niente si trova davanti il posto in cui
	// l'agente sta lavorando.
	async function revealThread(id) {
		if (!id) return;
		await loadConversationsIndex();
		if (liveConversationEntries().some((entry) => entry.id === conversationsCurrentId)) {
			return;
		}
		await switchToConversation(id);
	}

	// resumeExecution renders the execution that came with the detail and picks
	// its polling back up when it is still open. It never starts anything: a
	// page load must show the run, not launch one.
	function resumeExecution(record, ctx) {
		// resumeRun per prima, e il disegno dopo: è lei che impara dal server se
		// questa run si legge in una conversazione, ed è quella risposta che
		// decide se il pannello dell'esecuzione esiste. Disegnare prima
		// mostrerebbe per un istante un pannello che sta per sparire.
		resumeRun(record, ctx).then(() => {
			if (panelContext !== ctx) return;
			renderExecution(record);
		});
		if (record && !isExecutionTerminal(record)) {
			startExecutionPolling(record.id, ctx);
		}
	}

	// followExecution either keeps watching a still open execution, or settles
	// one that is already over.
	async function followExecution(record, ctx) {
		if (!record) return;
		if (!isExecutionTerminal(record)) {
			// Nell'ordine di resumeExecution, e per la stessa ragione: prima si
			// sa dove la run si legge, poi la si disegna.
			await resumeRun(record, ctx);
			if (panelContext !== ctx) return;
			renderExecution(record);
			startExecutionPolling(record.id, ctx);
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
		// A finished run may have moved the spec, and with it the step the
		// workspace recommends next: the strip follows without a reload (AC-3).
		await loadWorkspaceStatus();
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
					TEXT.executionStatusUnavailable(err.message || err),
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
	//
	// E nemmeno una run che si legge in una conversazione: quella run *è* la
	// conversazione — una sessione sola, un processo d'agente solo — e un
	// pannello accanto al thread sarebbe la stessa cosa disegnata due volte,
	// interrogata due volte, con due compositori che scrivono in un turno solo.
	// Chi lo sa è il server: `thread_id` sulla proiezione della run. Un provider
	// che non conversa non ha nessun thread, e per lui il pannello resta quello
	// di sempre — è il ripiego che l'ADR gli lascia.
	function renderExecution(record, note) {
		lastExecutionRecord = record || null;
		if (!panelExecution) return;
		if (!record) {
			panelExecution.innerHTML = "";
			return;
		}
		if (executionThreadID) {
			// La run vive in una conversazione: il pannello non la duplica, ma
			// finché è aperta offre la strada per raggiungerla. Chiusa la run,
			// l'esito lo racconta il thread e il pannello si spegne come prima.
			if (isExecutionTerminal(record)) {
				panelExecution.innerHTML = "";
				return;
			}
			const action = escapeHtml(record.action || "");
			panelExecution.innerHTML = `<div class="execution-panel execution-running execution-in-thread">
				<div class="execution-head">
					<span class="execution-dot" aria-hidden="true"></span>
					<span class="execution-headline">${TEXT.executionRunning(action)}</span>
					<code class="execution-id">${escapeHtml(record.id || "")}</code>
					<button type="button" class="ghost-btn execution-reach-thread" data-reach-thread="${escapeHtml(executionThreadID)}">${TEXT.executionGoToThread}</button>
				</div>
			</div>`;
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
				? TEXT.executionSucceeded(action)
				: state === "err"
					? TEXT.executionFailed(action)
					: TEXT.executionRunning(action);
		const lines = [];
		if (record.provider_id) {
			lines.push(escapeHtml(TEXT.executionProvider(record.provider_id)));
		}
		// The directory this run is executing in, read from the record and from
		// nothing else: an old run has to keep naming the directory it really ran
		// in, not the workspace that happens to be open while it is being read.
		// A record written before the field existed leaves the line as it was.
		if (typeof record.working_dir === "string" && record.working_dir) {
			lines.push(escapeHtml(TEXT.executionDirectory(record.working_dir)));
		}
		const modelLine = formatExecutionModel(record.model_choice);
		if (modelLine) lines.push(modelLine);
		const stamp = formatExecutionTime(record.completed_at || record.created_at);
		if (stamp) {
			lines.push(
				escapeHtml(
					isExecutionTerminal(record)
						? TEXT.executionCompleted(stamp)
						: TEXT.executionStarted(stamp),
				),
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
				`<div class="execution-message">${code}<span>${escapeHtml(record.error.message || TEXT.providerGaveNoReason)}</span></div>`,
			);
			if (record.error.external_id) {
				blocks.push(
					`<div class="execution-meta">${escapeHtml(TEXT.executionExternalID(record.error.external_id))}</div>`,
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
					`<details class="execution-payload"><summary>${escapeHtml(TEXT.executionPayload)}</summary><pre>${escapeHtml(payload)}</pre></details>`,
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

	// The model — and the options of that model — the run actually used, exactly
	// as the record reports them. Where they came from is stated only when the
	// record says they were inherited, so a run started with a departure does
	// not read as one that was not.
	function formatExecutionModel(choice) {
		if (!choice || typeof choice !== "object") return "";
		const model = choice.model ? String(choice.model) : "";
		if (!model) return "";
		const options = choice.options && typeof choice.options === "object" ? choice.options : {};
		const parts = Object.keys(options)
			.sort()
			.map((key) => `${escapeHtml(key)}=${escapeHtml(String(options[key]))}`);
		const rendered = parts.length ? ` ${parts.join(", ")}` : "";
		return TEXT.executionModel(
			escapeHtml(model),
			rendered,
			choice.source === "workspace",
		);
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
		text: { label: "agente", variant: "ev-assistant" },
		thinking: { label: "ragionamento", variant: "ev-thinking" },
		user_message: { label: "tu", variant: "ev-user" },
		tool_start: { label: "strumento · avvio", variant: "ev-tool-start" },
		tool_end: { label: "strumento · fatto", variant: "ev-tool-end" },
		tool_error: { label: "strumento · errore", variant: "ev-tool-error" },
		turn_end: { label: "fine turno", variant: "ev-turn-end" },
	};

	// The wire carries three states today. CANCELLED is listed because the
	// approved design fixed the vocabulary, so a provider that grows one gets
	// its own panel instead of the neutral fallback — nothing local ever picks
	// a row here.
	const RUN_STATE_LABELS = {
		ACTIVE: "attiva",
		CLOSED: "chiusa",
		CRASHED: "conclusa male",
		CANCELLED: "annullata",
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
		runDismissedNotices = {};
		runDraft = "";
		runBusy = false;
		runCancelArmed = false;
		runCancelSent = false;
		runAnsweredIDs = new Set();
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
			runNotice = TEXT.runUnreadable(err.message || err);
			renderRun();
			return;
		}
		if (panelContext !== ctx) return;
		// Una run che si legge in una conversazione non apre nessun pannello e
		// non si interroga: il thread la mostra già, dalla stessa sessione.
		// Il pannello dell'esecuzione si spegne con lui — l'esito lo porta il
		// payload della conversazione.
		executionThreadID = view && view.thread_id ? String(view.thread_id) : "";
		if (executionThreadID) {
			resetRunState();
			renderExecution(lastExecutionRecord);
			return;
		}
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
					runNotice = TEXT.runUnavailable(err.message || err);
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
				? TEXT.runAnsweredStillWaiting(label)
				: TEXT.runAnsweredTaken(label);
			// Remember which approval was answered so its card disappears
			// immediately, even if the refreshed projection still lists it once.
			if (answering) runAnsweredIDs.add(approvalID);
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
				${runNotice ? renderRunNotice("info", "waiting", runNotice, "waiting") : ""}
			</section>`;
			return;
		}

		const variant = RUN_STATE_VARIANTS[runSnapshot.state] || "run-closed";
		const blocks = [];
		if (runSnapshot.error) {
			blocks.push(renderRunNotice("refused", "error", runSnapshot.error, "error"));
		}
		if (runRefusal) {
			blocks.push(renderRunNotice("refused", "refused", runRefusal, "refusal"));
		}
		if (runOutcome) {
			blocks.push(renderRunNotice("ok", "confirmed", runOutcome, "outcome"));
		}
		if (runNotice) {
			blocks.push(renderRunNotice("info", "channel", runNotice, "channel"));
		}
		const closedAt = formatExecutionTime(runSnapshot.closed_at);
		if (closedAt) {
			blocks.push(
				renderRunNotice(
					"info",
					"ended",
					TEXT.runClosedAt(closedAt),
					"ended",
				),
			);
		}
		if (runTruncated) {
			blocks.push(
				renderRunNotice(
					"info",
					"window",
					TEXT.runWindow,
					"window",
				),
			);
		}
		blocks.push(renderRunTimeline());
		// The tail says the run is still speaking. It is drawn from the state the
		// server reported and from its connection flag, never from a local guess.
		if (runSnapshot.state === "ACTIVE" && runConnected && runEvents.length) {
			blocks.push(
				'<div class="run-tail"><span class="run-tail-dots" aria-hidden="true"><span></span><span></span><span></span></span>${escapeHtml(TEXT.runTailWorking)}</div>',
			);
		}
		blocks.push(
			runApprovals
				.filter((item) => !item || !runAnsweredIDs.has(item.id))
				.map((item) => renderRunApproval(item))
				.join(""),
		);
		blocks.push(renderRunComposer());

		panelRun.innerHTML = `<section class="run-panel ${variant}" aria-label="${escapeHtml(TEXT.runPanel)}">
			<div class="run-head">
				<span class="run-badge">${escapeHtml(RUN_STATE_LABELS[runSnapshot.state] || TEXT.runBadgeFallback)}</span>
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
	//
	// `dismiss` è la chiave con cui l'avviso è ricordato come chiuso. Ce l'hanno
	// tutti, perché ogni avviso si deve poter chiudere: una volta letto, lo
	// spazio che occupa appartiene alla run.
	function renderRunNotice(tone, mark, body, dismiss) {
		if (dismiss && runDismissedNotices[dismiss] === true) return "";
		const close = dismiss
			? `<button type="button" class="run-notice-dismiss" data-notice-dismiss="${escapeHtml(dismiss)}" title="${escapeHtml(TEXT.runDismissNotice)}" aria-label="${escapeHtml(TEXT.runDismissNotice)}">✕</button>`
			: "";
		return `<div class="run-notice ${tone}"${tone === "refused" ? ' role="alert"' : ""}>
			<span class="run-notice-mark">${escapeHtml(mark)}</span>
			<span class="run-notice-body">${escapeHtml(body)}</span>
			${close}
		</div>`;
	}

	// dismissRunNotice closes one inline notice and only that: the projection it
	// was reporting on — timeline, cursor, run state — is untouched.
	//
	// Il rifiuto e l'esito si spengono alla fonte — sono testo locale, e il
	// prossimo deve poter tornare a farsi vedere — mentre gli altri, che
	// raccontano il payload e tornerebbero identici al prossimo disegno, sono
	// ricordati come chiusi.
	function dismissRunNotice(which) {
		if (!which) return;
		if (which === "refusal") runRefusal = "";
		else if (which === "outcome") runOutcome = "";
		else runDismissedNotices[which] = true;
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
			return `<span class="run-link offline">${mark}${escapeHtml(TEXT.runLinkClosed)}</span>`;
		}
		if (runPollAbandoned) {
			return `<span class="run-link offline">${mark}${escapeHtml(TEXT.runLinkOff)}</span>`;
		}
		if (runConnected) {
			return `<span class="run-link listening">${mark}${escapeHtml(TEXT.runLinkOn)}</span>`;
		}
		return `<span class="run-link reconnecting">${mark}${escapeHtml(TEXT.runLinkReconnecting)}</span>`;
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
			const settled = stamp
				? TEXT.runCancelConfirmed(stamp)
				: TEXT.runCancelConfirmedPlain;
			return `<span class="run-cancel"><span class="run-cancel-state confirmed">${escapeHtml(settled)}</span></span>`;
		}
		if (runCancelSent) {
			return `<span class="run-cancel"><span class="run-cancel-state">${escapeHtml(TEXT.runCancelDelivered)}</span></span>`;
		}
		const disabled = runBusy ? " disabled" : "";
		if (!runCancelArmed) {
			return `<span class="run-cancel">
				<button type="button" class="ghost-btn danger-ghost-btn" data-cancel-open${disabled}>${escapeHtml(TEXT.runCancel)}</button>
			</span>`;
		}
		return `<span class="run-cancel">
			<span class="run-cancel-confirm">
				<span class="run-cancel-question">${escapeHtml(TEXT.runCancelQuestion)}</span>
				<button type="button" class="approval-btn deny" data-cancel-confirm${disabled}>${escapeHtml(TEXT.runCancelYes)}</button>
				<button type="button" class="approval-btn" data-cancel-abort${disabled}>${escapeHtml(TEXT.runCancelNo)}</button>
			</span>
		</span>`;
	}

	function renderRunTimeline() {
		// A terminal run's history is frozen: it stays readable, and the styling
		// says at a glance that nothing more will be added to it.
		const frozen =
			runSnapshot && runSnapshot.state !== "ACTIVE" ? " is-frozen" : "";
		if (!runEvents.length) {
			return `<ol class="run-timeline${frozen}"><li class="run-timeline-empty">${escapeHtml(TEXT.runTimelineEmpty)}</li></ol>`;
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
		return `<li class="run-seam" role="separator">${escapeHtml(TEXT.runSeam(String(event.id)))}</li>`;
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
		const label = known ? RUN_EVENT_KINDS[kind].label : kind || TEXT.runEventFallback;
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

	// renderRunApproval shows a pending decision verbatim: which answers exist is
	// the provider's statement, so the buttons are its options and nothing else.
	function renderRunApproval(approval) {
		if (!approval || !approval.id) return "";
		const id = escapeHtml(approval.id);
		const head = [
			`<span class="run-approval-eyebrow">${escapeHtml(TEXT.runApprovalEyebrowRequested)}</span>`,
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
		const args = formatExecutionPayload(approval.args);
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
				const off = runBusy ? " disabled" : "";
				return `<button type="button" class="approval-btn${tone}" data-approval-id="${id}" data-option-id="${escapeHtml(option.id)}"${off}>${escapeHtml(option.label || option.id)}</button>`;
			})
			.join("");
		return `<div class="run-approval" role="group" aria-label="${TEXT.runApprovalPending}">
			<div class="run-approval-head">${head.join("")}</div>
			<p class="run-approval-title">${escapeHtml(approval.title || TEXT.runApprovalTitle)}</p>
			${argsBlock}
			<div class="run-approval-options">${options}</div>
		</div>`;
	}

	function renderRunComposer() {
		const closed = runSnapshot && runSnapshot.state !== "ACTIVE";
		const disabled = runBusy || closed ? " disabled" : "";
		const placeholder = closed
			? TEXT.runComposerClosed
			: TEXT.runComposerPlaceholder;
		// The pending pill sits in the composer row, not in the timeline: a
		// message that has been accepted but not yet republished is a state of
		// the composer, and putting it among the events would be exactly the
		// optimistic write AC-3 forbids.
		const pending = runPendingMessage
			? `<span class="run-pending" role="status">
					<span class="run-pending-mark" aria-hidden="true"></span>
					${escapeHtml(TEXT.runPending)}
					<span class="run-pending-text">«${escapeHtml(runPendingMessage)}»</span>
				</span>`
			: "";
		return `<form class="run-composer">
			<textarea class="run-composer-input" rows="2" placeholder="${escapeHtml(placeholder)}"${disabled}></textarea>
			<div class="run-composer-row">
				${renderRunCancel()}
				${pending}
				<span class="run-composer-spacer"></span>
				<span class="run-composer-hint">${escapeHtml(closed ? TEXT.runComposerTerminal : TEXT.runComposerHint)}</span>
				<button type="submit" class="primary-btn"${disabled}>${escapeHtml(TEXT.runSend)}</button>
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
			configPath.textContent = TEXT.configPath(
				data.path || ".archetipo/config.yaml",
				currentConfigExists,
			);
			configSummaryConnector.textContent =
				(currentConfigSnapshot && currentConfigSnapshot.connector) || "file";
			configSummaryExists.textContent = currentConfigExists
				? TEXT.configPresent
				: TEXT.configMissing;
			setConfigStatus("", null);
			// Il riferimento è la configurazione appena letta dal server: è
			// arrivata adesso e non è lavoro di nessuno.
			configGuard.arm();
		} catch (err) {
			setConfigStatus(TEXT.loadFailed(err.message || err), "err");
		}
	}

	// La configurazione si compila da due lati — il form guidato e il YAML
	// grezzo — e la modale può essere chiusa da qualunque dei due: lo stato è
	// la somma, altrimenti chi lavora sul grezzo chiude senza che nessuno gli
	// chieda niente.
	const configGuard = createDirtyGuard(
		() => `${formState(configGuidedForm)}\n${configRaw.value}`,
	);

	function closeConfig() {
		if (!configGuard.allowsClose()) return;
		configGuard.disarm();
		configModal.classList.add("hidden");
		leaveModal(configModal);
		setConfigStatus("", null);
		setConfigValidation(TEXT.configNotTested, null);
	}

	function configPayload() {
		if (activeConfigTab === "advanced") {
			return { raw: configRaw.value };
		}
		return { config: buildGuidedConfig() };
	}

	async function validateConfig() {
		setConfigStatus(TEXT.configValidating, null);
		try {
			const result = await apiPost("/api/config/test", configPayload());
			const warnings = (result && result.warnings) || [];
			if (warnings.length > 0) {
				setConfigValidation(warnings.join(" "), "warn");
			} else if (result && result.info && result.info.connector) {
				setConfigValidation(
					TEXT.configValidOk(result.info.connector),
					"ok",
				);
			} else {
				setConfigValidation(TEXT.configValidPlain, "ok");
			}
			setConfigStatus(TEXT.configValidDone, "ok");
		} catch (err) {
			setConfigValidation(err.message || String(err), "err");
			setConfigStatus(TEXT.configValidFailed(err.message || err), "err");
		}
	}

	async function saveConfig() {
		setConfigStatus(TEXT.configSaving, null);
		try {
			const data = await apiPut("/api/config", configPayload());
			currentConfigSnapshot = (data && data.config) || currentConfigSnapshot;
			currentConfigRaw = (data && data.raw) || currentConfigRaw;
			currentConfigExists = true;
			fillConfigForm(currentConfigSnapshot);
			configRaw.value = currentConfigRaw;
			configSummaryConnector.textContent =
				(currentConfigSnapshot && currentConfigSnapshot.connector) || "file";
			configSummaryExists.textContent = TEXT.configPresent;
			configRestartNotice.classList.toggle(
				"hidden",
				!(data && data.restart_required),
			);
			const bits = [TEXT.configSaved];
			if (data && data.backup_path) bits.push(TEXT.configBackup(data.backup_path));
			if (data && data.restart_required) bits.push(TEXT.configRestartRequired);
			setConfigStatus(bits.join(" · "), "ok");
			showToast(TEXT.configSaved, "ok");
			// Quello che è a schermo è ora quello che c'è su disco.
			configGuard.arm();
		} catch (err) {
			setConfigStatus(TEXT.saveFailed(err.message || err), "err");
		}
	}

	// ---- Workspace conversation (US-053) ------------------------------------
	//
	// A conversation is not an execution: it has no record, no action, no
	// receipt. So it keeps its own state, its own cursor and its own polling
	// loop, and shares nothing with the run panel — the `run*` variables above
	// belong to the run behind an execution and staying out of them is what
	// keeps the two panels from describing each other's agent.
	//
	// Whether one can be opened at all, and why not, is a server verdict read
	// from `available` and `unavailable_reason`. Nothing here decides it.

	// Same discipline as the run panel: the read may fail transiently, so the
	// cursor survives and the loop keeps trying, and it gives up only after this
	// many consecutive failures rather than polling forever.
	const CONVERSATION_POLL_MS = 2000;
	const CONVERSATION_POLL_FAILURE_LIMIT = 3;
	// Ogni quanto scorre il contatore dell'attesa, e dove resta scritto che il
	// dettaglio tecnico va tenuto aperto.
	const CONVERSATION_WORKING_TICK_MS = 1000;
	const CONVERSATION_TECHNICAL_KEY = "archetipo:conversation:technical";
	// Le righe da cui si capisce a che cosa l'agente sta lavorando. Sono le
	// stesse che il vocabolario del renderer conosce per nome: tutto il resto —
	// i ganci, le righe di protocollo che un provider non traduce — dice che
	// qualcosa e' successo, non che cosa si sta facendo.
	const CONVERSATION_ACTIVITY_KINDS = {
		thinking: true,
		text: true,
		tool_start: true,
		tool_end: true,
		tool_error: true,
	};

	let conversationView = null; // last read of GET /api/workspace/conversation
	let conversationAfterID = 0; // highest event id already rendered — the only cursor
	let conversationEvents = []; // timeline, appended to and never rebuilt
	let conversationDraft = ""; // composer text, preserved across re-renders
	// Il messaggio consegnato all'agente e non ancora tornato indietro come
	// evento. Non è una copia della storia: è ciò che sta *fuori* dalla storia
	// per la finestra di tempo in cui il server non ha ancora niente da dire —
	// consegnare non pubblica, e quella finestra è tutta l'attesa dell'agente.
	// La conversazione lo disegna in coda, dichiarato in consegna, finché
	// l'evento vero non prende il suo posto.
	let conversationPendingMessage = "";
	let conversationBusy = false; // a command is in flight: the controls stay disabled
	let conversationCloseArmed = false; // the inline close confirmation is showing
	let conversationRefusal = ""; // last refused command, shown inline until the next one
	let conversationLink = ""; // note about the reading loop itself
	let conversationPollTimer = null; // interval following the open conversation
	let conversationPollBusy = false; // a poll is in flight: ticks never overlap
	let conversationPollFailures = 0; // consecutive failed reads, for the give-up threshold
	// The model choice belongs to the conversation that is about to start. It is
	// deliberately separate from modelChoiceSelection above, which belongs to a
	// process action, so touching one surface can never change the other.
	let conversationModelChoiceView = null;
	let conversationModelChoiceSelection = null;
	let conversationModelChoiceLoadToken = 0;
	let conversationOpeningSpecCode = "";

	// ---- Thread index (US-058, US-059) ---
	// Three variables, and none of them a second copy of a conversation: the
	// index is what the workspace has had, the current id is which of those the
	// page is showing, and the refusal is the last reason an open was denied.
	//
	// There is no transcript variable any more (US-059): one route serves a
	// conversation by id whether it is still live or already ended, so the panel
	// draws both from the same projection and tells them apart by the state
	// inside the payload. A second holder for the past ones would be a second
	// place claiming to hold a conversation.
	let conversationsIndex = null; // last read of GET /api/workspace/conversations
	let conversationsCurrentId = ""; // which thread the panel is showing
	// The server's own sentence when an open was refused — the limit of live
	// conversations and which ones to close (AC-5). It is kept here, beside the
	// rail, because the rail is where the offer to open one stands: it is shown
	// verbatim and never composed, and it is cleared by the first open or close
	// that succeeds.
	let conversationsRefusal = "";

	// ---- Answers given inside the conversation (US-060) ---
	// Two pieces of local state, and neither of them a second copy of anything
	// the server holds. The first remembers which approvals were answered from
	// this panel, so they disappear immediately even if one poll still lists
	// them as pending. It is keyed by id because one conversation can carry more
	// than one approval. The
	// second names the run block the viewer was just sent to, so arriving at it
	// marks it instead of only scrolling to it.
	let conversationAnsweredApprovals = {}; // approvalID → true
	let conversationHighlightAnchor = ""; // anchor event id of the block just reached
	// Quali avvisi del pannello sono stati chiusi con la loro X. Il pannello si
	// ridisegna a ogni lettura, quindi la scelta non può vivere nel DOM: senza
	// questa tabella il riquadro tornerebbe da solo al giro di poll successivo,
	// e la X sarebbe un comando che non comanda niente. Vale per la
	// conversazione su schermo e si dimentica con lei.
	let conversationDismissedNotices = {}; // chiave avviso → true

	// ---- Il dettaglio tecnico, e l'attesa ---
	// Quali pieghe tecniche sono state aperte a mano. Vale per la conversazione
	// su schermo e si dimentica con lei, come gli avvisi chiusi: una piega
	// aperta qui non deve stare aperta in un'altra conversazione.
	let conversationTechnicalOpen = {}; // chiave piega → true
	// La scelta di tenerle *tutte* aperte, invece, e' di chi guarda e non della
	// conversazione: chi lavora al dettaglio tecnico lo vuole aperto sempre, e
	// non deve ridirlo a ogni conversazione ne' domani mattina.
	let conversationTechnicalAll =
		localStorage.getItem(CONVERSATION_TECHNICAL_KEY) === "1";
	// Da quando l'agente sta lavorando a questo turno, in millisecondi. Zero
	// significa che non sta lavorando: e' l'unico stato: il *se* si deriva
	// dalla storia a ogni disegno, il *da quando* no — quello va ricordato,
	// perche' la storia non dice a che ora e' cominciata l'attesa di chi guarda.
	let conversationWorkingSince = 0;
	// Il contatore dei secondi. Ridisegnare tutto il pannello una volta al
	// secondo costerebbe quanto un giro di poll per far scorrere un numero:
	// questo tempo riscrive quel solo numero e niente altro.
	let conversationWorkingTimer = null;

	// Quanto è alto il campo lo decide ciò che c'è scritto, non un numero fisso:
	// da vuoto è una riga sola, e cresce riga per riga mentre si scrive. Serve
	// perché il pannello vive in una colonna e ogni pixel che il compositore
	// tiene fermo da vuoto è un pixel di conversazione in meno. Il tetto sta nel
	// CSS (max-height): oltre quello subentra la barra di scorrimento.
	function adattaAltezzaCompositore(input) {
		if (!input) return;
		// Azzerare prima è ciò che permette al campo di *tornare indietro*
		// quando il testo viene cancellato: scrollHeight non scende mai sotto
		// l'altezza già imposta.
		input.style.height = "auto";
		const bordi = input.offsetHeight - input.clientHeight;
		input.style.height = `${input.scrollHeight + bordi}px`;
	}

	// dismissConversationNotice chiude un avviso e soltanto quello: ciò di cui
	// parlava — lo stato della conversazione, la sua storia, il suo cursore —
	// non viene toccato.
	//
	// Un rifiuto è l'unico che si spegne alla fonte invece di essere ricordato
	// come chiuso: è testo locale, e tenerlo in vita solo per non disegnarlo
	// significherebbe far riapparire il riquadro al prossimo rifiuto uguale.
	function dismissConversationNotice(key) {
		if (!key) return;
		if (key === "refusal") conversationRefusal = "";
		else conversationDismissedNotices[key] = true;
		renderConversationPanel();
	}

	// toggleConversationTechnicalGroup apre o richiude una piega e soltanto
	// quella. Il pannello si ridisegna a ogni lettura, quindi la scelta non puo'
	// vivere nel DOM: senza questa tabella la piega si richiuderebbe da sola al
	// giro di poll successivo, e il comando non comanderebbe niente.
	function toggleConversationTechnicalGroup(key) {
		if (!key) return;
		if (conversationTechnicalOpen[key] === true) {
			delete conversationTechnicalOpen[key];
		} else {
			conversationTechnicalOpen[key] = true;
		}
		renderConversationPanel();
	}

	// toggleConversationTechnicalAll decide come si legge la conversazione — con
	// o senza il dettaglio tecnico — e la scelta resta scritta: e' di chi
	// guarda, non della conversazione che ha davanti.
	//
	// Riaccendendola le pieghe aperte a mano non vengono dimenticate: spegnerla
	// riporta esattamente allo stato di prima invece di richiudere anche quello
	// che era stato aperto uno per uno.
	function toggleConversationTechnicalAll() {
		conversationTechnicalAll = !conversationTechnicalAll;
		try {
			localStorage.setItem(
				CONVERSATION_TECHNICAL_KEY,
				conversationTechnicalAll ? "1" : "0",
			);
		} catch (_) {
			// Una preferenza che non si riesce a scrivere resta valida per la
			// sessione: non e' un motivo per rifiutare il comando.
		}
		renderConversationPanel();
	}

	// conversationWorking dice se l'agente sta lavorando a questo turno, e a che
	// punto e'. Non c'e' un campo del server che lo dichiari, e non ne viene
	// inventato uno: si legge dalla storia, che e' l'unica cosa che il server
	// dice davvero.
	//
	// La regola e' quella del turno. Un turno comincia quando un messaggio parte
	// e finisce quando arriva la riga che lo chiude — `turn_end`, che ogni
	// provider traduce nel vocabolario comune. Fra i due l'agente sta lavorando,
	// e l'ultima riga che ha prodotto dice a che cosa: ragiona, apre uno
	// strumento, scrive. Un errore chiude l'attesa come la chiude la fine del
	// turno: dopo un errore non si sta piu' aspettando niente.
	//
	// Fuori da una conversazione viva non si aspetta nulla per definizione, e
	// una conversazione appena aperta — nessuna riga, nessun messaggio in
	// consegna — non sta aspettando: sta aspettando *chi scrive*.
	function conversationWorking() {
		if (!conversationIsActive()) return { active: false };
		// Un turno si apre con il messaggio di chi scrive e si chiude con la
		// riga che lo chiude. Non basta guardare l'ultima riga della storia:
		// una conversazione appena aperta ne ha gia' due — i ganci che scattano
		// all'avvio dell'agente — e nessuna delle due e' un turno. Li' non si
		// sta aspettando l'agente: si sta aspettando chi scrive.
		let open = false;
		let doing = null;
		for (const event of conversationEvents) {
			const kind = event && typeof event.kind === "string" ? event.kind : "";
			if (kind === "user_message") {
				open = true;
				doing = null;
				continue;
			}
			if (kind === "turn_end" || kind === "error") {
				open = false;
				doing = null;
				continue;
			}
			// Solo le righe da cui si capisce *a che cosa* sta lavorando
			// diventano l'attivita' in corso. Una riga di protocollo del
			// provider non lo dice, e non deve cancellare l'ultima che lo
			// diceva: fra un gancio e l'altro «sta ragionando» resta vero.
			if (open && CONVERSATION_ACTIVITY_KINDS[kind] === true) doing = event;
		}
		// Il messaggio consegnato e non ancora tornato indietro e' gia' un turno
		// cominciato: l'attesa parte da li', non dal primo segno di vita
		// dell'agente, perche' e' li' che chi ha premuto invio comincia ad
		// aspettare.
		if (conversationPendingMessage) {
			open = true;
			doing = null;
		}
		if (!open) return { active: false };
		return {
			active: true,
			// Il tipo dell'ultima attivita' e il nome dello strumento che porta
			// sono fatti: le parole con cui vengono detti le sceglie il
			// renderer. Nessuna attivita' ancora e' un fatto anche quello — il
			// turno e' cominciato e l'agente non ha ancora prodotto niente — e
			// il renderer ha la frase generica per dirlo.
			kind: doing && typeof doing.kind === "string" ? doing.kind : "",
			tool: doing && typeof doing.tool === "string" ? doing.tool : "",
			seconds: conversationWorkingSince
				? Math.floor((Date.now() - conversationWorkingSince) / 1000)
				: 0,
		};
	}

	// Il tempo che fa scorrere il contatore dell'attesa. Riscrive un numero e
	// niente altro: non ridisegna il pannello, non legge dal server, e si ferma
	// da solo appena la riga dell'attesa non e' piu' sulla pagina.
	function startConversationWorkingTicker() {
		if (conversationWorkingTimer !== null) return;
		conversationWorkingTimer = setInterval(() => {
			if (!conversationEl || !conversationWorkingSince) {
				stopConversationWorkingTicker();
				return;
			}
			const cell = conversationEl.querySelector(
				"[data-conversation-working-elapsed]",
			);
			if (!cell) {
				stopConversationWorkingTicker();
				return;
			}
			const seconds = Math.floor(
				(Date.now() - conversationWorkingSince) / 1000,
			);
			cell.textContent =
				seconds >= 1 ? window.Conversation.formatElapsed(seconds) : "";
		}, CONVERSATION_WORKING_TICK_MS);
	}

	function stopConversationWorkingTicker() {
		if (conversationWorkingTimer === null) return;
		clearInterval(conversationWorkingTimer);
		conversationWorkingTimer = null;
	}

	// The panel is redrawn on every poll, so its controls cannot own their
	// handlers: the container does, bound once, and each control declares what
	// it is through its data attributes.
	function bindConversationPanel(container) {
		if (!container) return;
		container.addEventListener("click", (e) => {
			const noticeDismiss = e.target.closest(
				"[data-conversation-notice-dismiss]",
			);
			if (noticeDismiss) {
				dismissConversationNotice(
					noticeDismiss.getAttribute("data-conversation-notice-dismiss") || "",
				);
				return;
			}
			const technicalToggle = e.target.closest(
				"[data-conversation-technical-toggle]",
			);
			if (technicalToggle) {
				toggleConversationTechnicalGroup(
					technicalToggle.getAttribute(
						"data-conversation-technical-toggle",
					) || "",
				);
				return;
			}
			if (e.target.closest("[data-conversation-technical-all]")) {
				toggleConversationTechnicalAll();
				return;
			}
			if (e.target.closest("[data-conversation-open]")) {
				openConversation();
				return;
			}
			if (e.target.closest("[data-conversation-close-open]")) {
				conversationCloseArmed = true;
				renderConversationPanel();
				return;
			}
			if (e.target.closest("[data-conversation-close-abort]")) {
				conversationCloseArmed = false;
				renderConversationPanel();
				return;
			}
			if (e.target.closest("[data-conversation-close-confirm]")) {
				closeConversation();
				return;
			}
			// A decision is the only thing these two controls do: they name the
			// proposal and say yes or no. Whether that yes starts anything, and
			// what, is the server's business — the panel never dispatches an
			// execution itself.
			const accept = e.target.closest("[data-conversation-proposal-accept]");
			if (accept) {
				decideProposal(accept.getAttribute("data-proposal-id"), "accept");
				return;
			}
			const decline = e.target.closest("[data-conversation-proposal-decline]");
			if (decline) {
				decideProposal(decline.getAttribute("data-proposal-id"), "decline");
				return;
			}
			// Answering a run's approval from the flow that asked for it (AC-4).
			// This branch comes *before* the reach one on purpose: the buttons of
			// a waiting card live inside a run block that also carries the "go to
			// the whole log" control, and a press must answer rather than
			// navigate away from the very decision it just took.
			const answer = e.target.closest("[data-run-approval-id]");
			if (answer) {
				respondConversationApproval(
					answer.getAttribute("data-execution-id") || "",
					answer.getAttribute("data-run-approval-id") || "",
					answer.getAttribute("data-run-option-id") || "",
				);
				return;
			}
			// The recommended step, pressed from the tail of the thread. This
			// branch comes *before* the reach one on purpose: a block inside the
			// thread must not be mistaken for a navigation.
			//
			// The refusal of a blocked step is a decision of the pure module, not
			// the disabled attribute of the markup, and it holds for anyone
			// reaching this handler by any route. What runs afterwards is
			// startNextStep, hence startPanelAction: the very line the board
			// presses, on the very same target.
			const next = e.target.closest(".conv-nextstep-run");
			if (next) {
				const target = window.WorkspaceStatus.nextStepDispatch(
					nextStepStatusView(),
				);
				if (!target) return;
				startNextStep(target, next).catch((err) => {
					showToast(err.message || String(err), "err");
				});
				return;
			}
			// Un passo che si legge in una conversazione sua si raggiunge
			// andandoci: non c'è nessun pannello da montare, perché quella
			// conversazione è il passo.
			const reachThread = e.target.closest("[data-conversation-reach-thread]");
			if (reachThread) {
				const target = reachThread.getAttribute("data-conversation-reach-thread");
				if (target) {
					switchToConversation(target).catch((err) => {
						showToast(err.message || String(err), "err");
					});
				}
				return;
			}
			// Same rule as the status strip: reaching a run only navigates to the
			// panel where the run already lives, so there is a single place that
			// mounts execution panels and resumes a record.
			const reach = e.target.closest("[data-conversation-reach-run]");
			if (reach) {
				const scope = reach.getAttribute("data-scope");
				const code = reach.getAttribute("data-code");
				if (scope === "spec" && code) {
					openEditor(code);
				} else if (scope === "workspace") {
					openPRD();
				}
			}
		});
		container.addEventListener("submit", (e) => {
			const form = e.target.closest(".conv-composer");
			if (!form) return;
			e.preventDefault();
			sendConversationMessage();
		});
		container.addEventListener("change", (e) => {
			const target = e.target;
			if (!target) return;
			if (target.hasAttribute("data-conversation-model")) {
				chooseConversationModel(target.value);
				return;
			}
			const option = target.getAttribute("data-conversation-option");
			if (option !== null) chooseConversationOption(option, target.value);
		});
		container.addEventListener("input", (e) => {
			const input = e.target.closest(".conv-composer-input");
			if (!input) return;
			conversationDraft = input.value;
			adattaAltezzaCompositore(input);
		});
		// Invio manda il messaggio, Maiusc+Invio va a capo: è il gesto che si ha
		// nelle dita in una conversazione, e scrivere è ciò che qui si fa più
		// spesso. ⌘/Ctrl+Invio resta valido perché chi lo aveva imparato non
		// deve disimpararlo.
		//
		// Una composizione IME in corso non è un invio: il primo Invio chiude la
		// scelta dei caratteri e non deve mandare a metà ciò che si sta ancora
		// scrivendo.
		container.addEventListener("keydown", (e) => {
			const input = e.target.closest(".conv-composer-input");
			if (!input) return;
			if (e.key !== "Enter") return;
			if (e.isComposing || e.keyCode === 229) return;
			if (e.shiftKey || e.altKey) return;
			e.preventDefault();
			conversationDraft = input.value;
			sendConversationMessage();
		});
	}

	bindConversationPanel(conversationEl);

	// Drawn once, before the first read: the home of the workspace is on screen
	// from the very first paint, and what it shows until the server answers is
	// the renderer's own empty state.
	renderConversationPanel();

	// ---- Thread rail (US-058) -----------------------------------------------
	//
	// Bound once, like the conversation panel, and for the same reason: the rail
	// is redrawn on every read of the index, so its controls cannot own their
	// handlers. Each one declares what it is through its data attribute.
	function bindConversationsRail(container) {
		if (!container) return;
		container.addEventListener("click", (e) => {
			if (e.target.closest("[data-rail-notice-dismiss]")) {
				// Il rifiuto si spegne alla fonte: è testo locale, e il prossimo
				// rifiuto deve poter tornare a farsi vedere.
				conversationsRefusal = "";
				renderConversationsRail();
				return;
			}
			if (e.target.closest("[data-conversation-new]")) {
				prepareConversationOpen("");
				return;
			}
			const thread = e.target.closest("[data-conversation-id]");
			if (thread) {
				openConversationThread(
					thread.getAttribute("data-conversation-id") || "",
				);
			}
		});
	}

	bindConversationsRail(conversationsRailEl);

	// renderConversationsRail draws the rail from the local projection alone. It
	// owns the holder; the renderer owns everything inside it.
	function renderConversationsRail() {
		if (!conversationsRailEl) return;
		if (noWorkspaceMode() || !window.ConversationIndex) {
			// No workspace, no rail: an index of nothing is not an empty list, it
			// is a question that cannot be asked. Emptied rather than hidden, so
			// the `:empty` rule collapses the column with it.
			conversationsRailEl.innerHTML = "";
			return;
		}
		// The refusal rides above the list, where the offer that was refused
		// stands. Its words are the server's, rendered by the module and never
		// assembled here: the limit and the conversations to close are facts of
		// the workspace, and a second sentence about them written in the browser
		// would be a second truth.
		const notice = window.ConversationIndex.renderLimitNotice
			? window.ConversationIndex.renderLimitNotice(conversationsRefusal)
			: "";
		conversationsRailEl.innerHTML =
			notice +
			window.ConversationIndex.renderConversationIndex(conversationsIndex, {
				currentId: conversationsCurrentId,
			});
	}

	// loadConversationsIndex re-reads the index of the workspace. It is called
	// at every moment the list could have changed — a conversation opened,
	// closed or resumed, and every successful turn of the conversation poll — so
	// `last_message_at` and the "in corso" mark stay true without a second timer
	// of their own.
	async function loadConversationsIndex() {
		if (!conversationsRailEl) return;
		if (noWorkspaceMode()) {
			conversationsIndex = null;
			renderConversationsRail();
			return;
		}
		let view;
		try {
			view = await apiGet("/api/workspace/conversations");
		} catch (_) {
			// A read that failed says nothing about what the workspace holds, so
			// the rail keeps drawing the last index it had rather than claiming
			// the history is gone. No toast: a viewer that cannot answer must not
			// stop anyone from working.
			return;
		}
		conversationsIndex = view;
		renderConversationsRail();
	}

	// The entries the index says are live — all of them, because a workspace
	// holds more than one (AC-1). The flag is the server's, never derived here,
	// and the order is the index's own: the most recently spoken to comes first.
	function liveConversationEntries() {
		const entries =
			conversationsIndex && Array.isArray(conversationsIndex.conversations)
				? conversationsIndex.conversations
				: [];
		return entries.filter((entry) => entry && entry.live);
	}

	// emptyConversationView is what the panel draws while it is showing no
	// conversation at all. Whether one could be opened, and why not, is the
	// index's answer — the same `available` / `unavailable_reason` pair the
	// conversation payload carries, so the invitation and its refusal are drawn
	// from a server verdict here too and never from a guess made in the page.
	function emptyConversationView() {
		const index =
			conversationsIndex && typeof conversationsIndex === "object"
				? conversationsIndex
				: null;
		if (!index) return null;
		return {
			available: index.available !== false,
			unavailable_reason: index.unavailable_reason || "",
			provider_id: index.provider_id || "",
			conversation: null,
			events: [],
		};
	}

	async function loadConversationModelChoice() {
		const token = ++conversationModelChoiceLoadToken;
		try {
			const view = await apiGet("/api/execution/model-choice");
			if (token !== conversationModelChoiceLoadToken) return;
			conversationModelChoiceView = view || null;
			renderConversationPanel();
		} catch (_) {
			// Choosing is optional. A failed catalog read must not turn opening a
			// conversation into an unavailable action.
		}
	}

	function currentConversationModelChoice() {
		if (conversationModelChoiceSelection) {
			return {
				model: String(conversationModelChoiceSelection.model || ""),
				options: Object.assign({}, conversationModelChoiceSelection.options),
			};
		}
		const view = conversationModelChoiceView || {};
		return {
			model: view.model === undefined || view.model === null ? "" : String(view.model),
			options: Object.assign({}, view.options || {}),
		};
	}

	function conversationDeclaredOptionNames(modelID) {
		const models =
			conversationModelChoiceView && Array.isArray(conversationModelChoiceView.models)
				? conversationModelChoiceView.models
				: [];
		const model = models.find(
			(entry) => entry && String(entry.id || "") === String(modelID || ""),
		);
		return model && Array.isArray(model.options)
			? model.options.filter(Boolean).map((option) => String(option.name || ""))
			: [];
	}

	function chooseConversationModel(value) {
		const current = currentConversationModelChoice();
		const model = value === undefined || value === null ? "" : String(value);
		const options = {};
		conversationDeclaredOptionNames(model).forEach((name) => {
			if (current.options[name] !== undefined && current.options[name] !== null) {
				options[name] = String(current.options[name]);
			}
		});
		conversationModelChoiceSelection = { model, options };
		renderConversationPanel();
	}

	function chooseConversationOption(name, value) {
		const current = currentConversationModelChoice();
		current.options[String(name)] =
			value === undefined || value === null ? "" : String(value);
		conversationModelChoiceSelection = current;
	}

	function conversationModelOverride() {
		if (!conversationModelChoiceSelection || !conversationModelChoiceView) return null;
		const inheritedModel =
			conversationModelChoiceView.model === undefined || conversationModelChoiceView.model === null
				? ""
				: String(conversationModelChoiceView.model);
		const model = String(conversationModelChoiceSelection.model || "");
		const options = normalizeOptionMap(conversationModelChoiceSelection.options);
		if (
			model === inheritedModel &&
			sameOptionMap(options, normalizeOptionMap(conversationModelChoiceView.options))
		) return null;
		return { model, model_options: options };
	}

	function conversationModelChoiceMarkup() {
		const render =
			window.ProviderFields && window.ProviderFields.renderConversationModelChoice;
		return render
			? render(conversationModelChoiceView, conversationModelChoiceSelection)
			: "";
	}

	function prepareConversationOpen(specCode) {
		if (conversationBusy) return;
		stopConversationPolling();
		conversationsCurrentId = "";
		conversationView = null;
		conversationEvents = [];
		conversationAfterID = 0;
		conversationPendingMessage = "";
		conversationRefusal = "";
		conversationCloseArmed = false;
		conversationOpeningSpecCode = String(specCode || "");
		conversationModelChoiceSelection = null;
		conversationModelChoiceView = null;
		renderConversationsRail();
		renderConversationPanel();
		loadConversationModelChoice();
		setShellView("conversation");
	}

	// switchToConversation is the only place that changes which conversation the
	// panel is showing, and it is what makes AC-2 true on the page: the local
	// projection is a projection *of one conversation*, so it is emptied with the
	// change of id — a timeline left standing would show, in the one, what was
	// said in the other. The draft is deliberately not touched: what a person was
	// typing belongs to them and survives until the workspace changes.
	//
	// `preloaded` is the view a command has just been answered with (an open, a
	// resume): applying it avoids a second read of a conversation the server has
	// already described, without letting a second place assign the current id.
	async function switchToConversation(id, preloaded) {
		if (!id) return;
		stopConversationPolling();
		conversationsCurrentId = id;
		conversationView = null;
		conversationEvents = [];
		conversationAfterID = 0;
		// L'eco locale appartiene alla conversazione che si sta lasciando: sotto
		// un'altra sarebbe una frase detta a qualcun altro.
		conversationPendingMessage = "";
		conversationAnsweredApprovals = {};
		conversationHighlightAnchor = "";
		conversationDismissedNotices = {};
		// Le pieghe aperte a mano e l'attesa in corso appartengono alla
		// conversazione che si sta lasciando: sotto un'altra sarebbero l'attesa
		// di qualcun altro. La scelta di tenere il dettaglio tecnico sempre
		// aperto invece resta: quella e' di chi guarda.
		conversationTechnicalOpen = {};
		conversationWorkingSince = 0;
		stopConversationWorkingTicker();
		conversationRefusal = "";
		conversationLink = "";
		conversationCloseArmed = false;
		conversationPollBusy = false;
		conversationPollFailures = 0;
		conversationModelChoiceLoadToken += 1;
		conversationModelChoiceView = null;
		conversationModelChoiceSelection = null;
		conversationOpeningSpecCode = "";
		renderConversationsRail();
		renderConversationPanel();
		let view = preloaded;
		if (!view) {
			try {
				view = await apiGet(
					`/api/workspace/conversations/${encodeURIComponent(id)}?after_id=0`,
				);
			} catch (err) {
				showConversationRefusal(err);
				renderConversationPanel();
				return;
			}
			// A later switch won while this read was in flight: applying it now
			// would draw one conversation under the name of another.
			if (conversationsCurrentId !== id) return;
		}
		applyConversationView(view);
		renderConversationPanel();
		renderConversationsRail();
		if (conversationIsActive()) startConversationPolling();
		else loadConversationModelChoice();
	}

	// openConversationThread shows one thread of the rail. Where one looks does
	// not change: the panel stays mounted and only its contents differ. Live or
	// ended is not asked here — one route answers for both, and the state inside
	// the payload is what tells them apart.
	async function openConversationThread(id) {
		// Pressing the thread already on screen re-reads it from the beginning,
		// which is what makes the rail a way out of a read that failed.
		if (!id || conversationBusy) return;
		await switchToConversation(id);
	}

	// resumeConversationThread is what writing in a past thread does (AC-4).
	// Nothing of the old session is reopened: the server opens a *new*
	// conversation and hands it the old one as context, and the banner says so
	// in as many words.
	async function resumeConversationThread() {
		const id = conversationsCurrentId;
		if (conversationBusy || !id) return;
		const message = conversationDraft.trim();
		if (!message) return;
		conversationBusy = true;
		renderConversationPanel();
		try {
			const body = { message };
			const override = conversationModelOverride();
			if (override) {
				body.model = override.model;
				body.model_options = override.model_options;
			}
			const view = await apiPost(
				`/api/workspace/conversations/${encodeURIComponent(id)}/resume`,
				body,
			);
			// A new conversation is a new history, and switching to it is what
			// empties the previous one's projection — the draft included here,
			// because this one has just been sent.
			conversationDraft = "";
			conversationsRefusal = "";
			conversationModelChoiceSelection = null;
			await switchToConversation(conversationIdOf(view), view);
			// La conversazione nuova nasce vuota e il messaggio che l'ha aperta è
			// già stato consegnato: senza l'eco, chi ha premuto invio si
			// ritroverebbe davanti una storia vuota al posto di ciò che ha
			// appena detto. L'eco si rimette *dopo* il cambio, che l'ha azzerata
			// con tutto il resto, e si sistema subito contro quello che la
			// conversazione nuova porta già con sé.
			conversationPendingMessage = message;
			conversationEvents.forEach(settleConversationPending);
		} catch (err) {
			// Rifiutata: qui la bozza non era stata svuotata — si svuota solo
			// quando la ripresa riesce — quindi il testo è già dov'era.
			showConversationRefusal(err);
			renderConversationPanel();
		} finally {
			conversationBusy = false;
			renderConversationPanel();
			loadConversationsIndex();
		}
	}

	// The id of the conversation a command answered with, or the empty string:
	// every open, resume and read gets its id from the payload and never from
	// anything remembered before the call.
	function conversationIdOf(view) {
		return (view && view.conversation && view.conversation.id) || "";
	}

	// openSpecConversation opens a conversation about the spec on screen: the
	// same open route, told which spec it is about, so the thread lands in the
	// rail under its code instead of among the free ones.
	async function openSpecConversation(code) {
		if (!code) return;
		prepareConversationOpen(code);
	}

	function stopConversationPolling() {
		if (conversationPollTimer === null) return;
		clearInterval(conversationPollTimer);
		conversationPollTimer = null;
	}

	// resetConversationState forgets the conversation of the workspace being
	// left. The panel that comes back must be the one of the workspace now open,
	// never a leftover of the previous one.
	function resetConversationState() {
		stopConversationPolling();
		conversationView = null;
		conversationAfterID = 0;
		conversationEvents = [];
		conversationDraft = "";
		conversationPendingMessage = "";
		conversationBusy = false;
		conversationCloseArmed = false;
		conversationRefusal = "";
		conversationLink = "";
		conversationPollBusy = false;
		conversationPollFailures = 0;
		conversationModelChoiceLoadToken += 1;
		conversationModelChoiceView = null;
		conversationModelChoiceSelection = null;
		conversationOpeningSpecCode = "";
		// The answers given here and the block last reached belong to the
		// conversation being left: neither may be read as the new one's. Lo
		// stesso vale per gli avvisi chiusi: chiuderne uno qui non deve zittire
		// lo stesso avviso in un'altra conversazione.
		conversationAnsweredApprovals = {};
		conversationHighlightAnchor = "";
		conversationDismissedNotices = {};
		conversationTechnicalOpen = {};
		conversationWorkingSince = 0;
		stopConversationWorkingTicker();
		// The rail is forgotten with the conversation (AC-6): the index of the
		// workspace being left must never be on screen beside the workspace now
		// open, not even for the instant it takes to read the new one. Emptied
		// first, then re-read.
		conversationsIndex = null;
		conversationsCurrentId = "";
		conversationsRefusal = "";
		renderConversationsRail();
		loadConversationsIndex();
		// The panel is the home of the workspace: it never goes blank and it
		// never hides itself — its visibility belongs to the layout alone. What
		// it draws once the state is cleared is the home of a workspace with no
		// conversation, which the renderer already has a branch for.
		//
		// This is the only place authorised to clear conversationDraft, and it is
		// reached only by a change or a close of the workspace — never by a
		// change of view.
		renderConversationPanel();
	}

	// applyConversationView folds one server view into the local projection and
	// returns how many events it appended. Events are only ever appended, and
	// only beyond the cursor: the list is never rebuilt, so a re-read after a
	// failed poll cannot draw the same line twice.
	function applyConversationView(view) {
		if (!view) return 0;
		const events = Array.isArray(view.events) ? view.events : [];
		let appended = 0;
		for (const event of events) {
			if (!event || typeof event.id !== "number") continue;
			if (event.id <= conversationAfterID) continue;
			conversationEvents.push(event);
			conversationAfterID = event.id;
			appended += 1;
			settleConversationPending(event);
		}
		if (
			typeof view.last_id === "number" &&
			view.last_id > conversationAfterID
		) {
			conversationAfterID = view.last_id;
		}
		conversationView = view;
		// La spec della conversazione appena applicata potrebbe non essere
		// quella per cui il passo scopato era stato letto: ogni strada che porta
		// qui — switch, boot, poll — riallinea la lettura senza aspettarla.
		if (
			currentConversationSpecCode() !==
			(conversationStatusSnapshot ? conversationStatusSnapshot.specCode : "")
		) {
			loadConversationNextStep();
		}
		return appended;
	}

	// L'eco locale smette di esistere quando l'agente riporta indietro il
	// messaggio: quell'evento *è* la riga in coda alla conversazione, e tenerne
	// due sarebbe far leggere due volte la stessa frase.
	//
	// Il confronto è sul testo perché non c'è altro da confrontare: la consegna
	// non restituisce un identificatore, e l'id dell'evento lo assegna il
	// processo dell'agente. Si confrontano le parole con gli spazi normalizzati,
	// così un provider che ricompone gli a capo a modo suo non lascia l'eco
	// appesa per sempre sotto la sua stessa riga.
	function settleConversationPending(event) {
		if (!conversationPendingMessage) return;
		if (!event || event.kind !== "user_message") return;
		const said =
			event.text === undefined || event.text === null
				? ""
				: String(event.text);
		if (collapseSpaces(said) !== collapseSpaces(conversationPendingMessage)) {
			return;
		}
		conversationPendingMessage = "";
	}

	/** Le stesse parole, con ogni sequenza di spazi ridotta a uno solo. */
	function collapseSpaces(text) {
		return String(text).replace(/\s+/g, " ").trim();
	}

	function conversationIsActive() {
		return !!(
			conversationView &&
			conversationView.conversation &&
			conversationView.conversation.state === "ACTIVE"
		);
	}

	// loadConversation reads the conversations of the open workspace from the
	// beginning. It is the entry point of every fresh read — boot, refresh, and
	// every workspace change — so the cursor is reset with the projection.
	//
	// There is no "the" conversation to ask for any more (US-059): the index is
	// what says which ones the workspace holds, and the page shows one of the
	// live ones — the most recently spoken to, which is the one the index puts
	// first. With none alive the panel is the invitation to open one, and whether
	// that offer can be taken is the index's verdict too.
	async function loadConversation() {
		if (!conversationEl) return;
		if (noWorkspaceMode()) {
			resetConversationState();
			return;
		}
		resetConversationState();
		await loadConversationsIndex();
		const live = liveConversationEntries();
		const first = live.length ? live[0].id || "" : "";
		if (!first) {
			// A viewer that cannot answer, or a workspace holding nothing alive:
			// either way the panel draws the home of a workspace with no
			// conversation open. No toast — it is the home of the workspace, and
			// it does not vanish.
			renderConversationPanel();
			loadConversationModelChoice();
			return;
		}
		await switchToConversation(first);
	}

	// startConversationPolling follows the open conversation with the discipline
	// of startRunPolling: ticks never overlap, the loop gives up after a number
	// of consecutive failures and says so, and it stops on its own once the
	// conversation is no longer live and a read brought nothing new — so the
	// last turn is never cut off.
	function startConversationPolling() {
		stopConversationPolling();
		// The loop follows one conversation by name, and the name is taken once,
		// here: everything it does afterwards is checked against it, so a tick
		// that comes back for a conversation the page has since left is dropped
		// instead of being drawn over the one now on screen (AC-2).
		const followed = conversationsCurrentId;
		if (!followed) return;
		conversationPollTimer = setInterval(async () => {
			if (noWorkspaceMode()) {
				stopConversationPolling();
				return;
			}
			if (conversationsCurrentId !== followed) {
				// The panel has moved on and this timer is not the one following
				// what it shows: it stops rather than reading in the background.
				stopConversationPolling();
				return;
			}
			if (conversationPollBusy) return;
			conversationPollBusy = true;
			let view;
			try {
				view = await apiGet(
					`/api/workspace/conversations/${encodeURIComponent(followed)}?after_id=${conversationAfterID}`,
				);
			} catch (err) {
				conversationPollBusy = false;
				if (conversationsCurrentId !== followed) return;
				conversationPollFailures += 1;
				if (conversationPollFailures >= CONVERSATION_POLL_FAILURE_LIMIT) {
					stopConversationPolling();
					// Nothing is reconnecting any more, so the panel must stop
					// implying that it is.
					conversationLink = TEXT.conversationUnreadable(err.message || err);
				}
				renderConversationPanel();
				return;
			}
			conversationPollBusy = false;
			// The read left for one conversation and came back for another: what
			// it carries belongs to the conversation that was on screen when it
			// started, and nothing of it may reach the one that is on screen now.
			if (conversationsCurrentId !== followed) return;
			conversationPollFailures = 0;
			conversationLink = "";
			const appended = applyConversationView(view);
			renderConversationPanel();
			// One loop, two things kept true: the conversation on screen and the
			// index beside it. The rail's "in corso" mark and its last moment are
			// facts about the very conversation this tick just read, so they
			// travel with it instead of with a second timer of their own.
			loadConversationsIndex();
			if (!conversationIsActive() && appended === 0) {
				stopConversationPolling();
			}
		}, CONVERSATION_POLL_MS);
	}

	function showConversationRefusal(err) {
		const message = (err && err.message) || String(err);
		const hint = err && err.hint ? ` — ${err.hint}` : "";
		conversationRefusal = `${message}${hint}`;
		showToast(message, "err");
	}

	// refuseConversationOpen is what a refused open leaves on the page: the
	// server's own sentence, kept beside the offer that was refused. Past the
	// limit that sentence declares the limit and names the conversations to
	// close (AC-5), and it is shown exactly as it arrived — the page neither
	// writes a second one nor knows the number.
	function refuseConversationOpen(err) {
		showConversationRefusal(err);
		conversationsRefusal = conversationRefusal;
		renderConversationsRail();
	}

	async function openConversation() {
		if (conversationBusy) return;
		conversationBusy = true;
		renderConversationPanel();
		try {
			// No conversation is released to make room for this one: whatever the
			// workspace already holds keeps its history and its work in progress
			// (AC-1). Past the limit the server refuses, and the refusal is its
			// own words.
			const body = {};
			if (conversationOpeningSpecCode) body.spec_code = conversationOpeningSpecCode;
			const override = conversationModelOverride();
			if (override) {
				body.model = override.model;
				body.model_options = override.model_options;
			}
			const view = await apiPost("/api/workspace/conversations", body);
			conversationsRefusal = "";
			conversationOpeningSpecCode = "";
			conversationModelChoiceSelection = null;
			await switchToConversation(conversationIdOf(view), view);
		} catch (err) {
			refuseConversationOpen(err);
		} finally {
			conversationBusy = false;
			renderConversationPanel();
			loadConversationsIndex();
		}
	}

	async function sendConversationMessage() {
		if (conversationBusy) return;
		const id = conversationsCurrentId;
		if (!id || !conversationView) return;
		// Writing in a conversation that has ended is not writing in a live one:
		// it takes that one up again in a new conversation. One composer, two
		// routes, and the one taken is decided by the state the payload declares
		// — the page no longer keeps a separate holder to tell the two apart.
		if (!conversationIsActive()) {
			resumeConversationThread();
			return;
		}
		const message = conversationDraft.trim();
		if (!message) return;
		conversationBusy = true;
		// Il campo si svuota e il messaggio compare in coda alla conversazione
		// *adesso*, prima ancora di partire: chi ha premuto invio deve vedere
		// subito quello che ha detto, e l'attesa dell'agente deve essere l'attesa
		// di una risposta a qualcosa di visibile, non il silenzio di una schermata
		// identica a un istante prima. Se la consegna viene rifiutata, sotto si
		// rimette esattamente ciò che era stato scritto.
		conversationPendingMessage = message;
		conversationDraft = "";
		renderConversationPanel();
		try {
			const view = await apiPost(
				`/api/workspace/conversations/${encodeURIComponent(id)}/messages?after_id=${conversationAfterID}`,
				{ message },
			);
			// Accepted means delivered, not published: the text stays out of the
			// timeline until the agent carries it back. Quello che si vede in
			// coda fino ad allora è l'eco locale qui sopra, e porta scritto che
			// è in consegna — non si spaccia per storia.
			conversationRefusal = "";
			applyConversationView(view);
			startConversationPolling();
		} catch (err) {
			// Rifiutato: niente è stato consegnato, quindi niente resta in coda
			// alla conversazione, e il testo torna nel campo da cui era uscito —
			// chi l'ha scritto non deve riscriverlo per leggere il rifiuto.
			conversationPendingMessage = "";
			conversationDraft = message;
			showConversationRefusal(err);
		} finally {
			conversationBusy = false;
			renderConversationPanel();
		}
	}

	// decideProposal answers the pending proposal: it says which one and what was
	// decided, and nothing else. The decision route is the only one this panel
	// ever calls about an action — a confirmation goes through the very same
	// start path the board uses, on the server side, and a refusal starts
	// nothing at all.
	//
	// A refusal from the server is shown with the server's own words, like every
	// other refused command of this panel.
	async function decideProposal(proposalID, decision) {
		if (conversationBusy) return;
		const id = conversationsCurrentId;
		if (!id) return;
		conversationBusy = true;
		renderConversationPanel();
		try {
			const view = await apiPost(
				`/api/workspace/conversations/${encodeURIComponent(id)}/proposal?after_id=${conversationAfterID}`,
				{ proposal_id: Number(proposalID), decision },
			);
			conversationRefusal = "";
			applyConversationView(view);
			// A confirmation may have just started something: the board and the
			// status strip describe the process, not the conversation, so they are
			// re-read here rather than waiting for the next event — the same
			// gesture a move on the board makes.
			if (decision === "accept") {
				await loadBoard();
				await loadWorkspaceStatus();
			}
		} catch (err) {
			showConversationRefusal(err);
		} finally {
			conversationBusy = false;
			renderConversationPanel();
			if (conversationIsActive()) startConversationPolling();
		}
	}

	// respondConversationApproval answers, from the conversation, an approval a
	// run of that conversation is waiting on (AC-4). It calls the very route the
	// run panel calls — one decision route for one decision — and it starts no
	// loop of its own: the conversation poll already carries the run and its
	// approvals on every turn, so the only extra read here is the immediate one
	// that makes the answer visible without waiting for the next tick.
	//
	// The composer is not touched by any of this: conversationBusy rises and
	// falls exactly as it does for every other command of this panel, so writing
	// to the agent while a run is stopped stays possible (AC-5).
	async function respondConversationApproval(executionID, approvalID, optionID) {
		if (conversationBusy || !approvalID || !optionID) return;
		// No execution id means the decision is the conversation's own: the agent
		// holding the thread stopped to ask, and there is no run to answer on.
		// The route is the conversation's, and the id it needs is the one on
		// screen — the same one every other command of this panel is sent to.
		const showingID = conversationsCurrentId;
		if (!executionID && !showingID) return;
		conversationBusy = true;
		renderConversationPanel();
		try {
			await apiPost(
				executionID
					? `/api/execution/${encodeURIComponent(executionID)}/run/approvals/${encodeURIComponent(approvalID)}`
					: `/api/workspace/conversations/${encodeURIComponent(showingID)}/approvals/${encodeURIComponent(approvalID)}`,
				{ option_id: optionID },
			);
			conversationRefusal = "";
			// Hide the answered card immediately. The next projection will normally
			// stop listing it too; this local id closes the short gap without keeping
			// the command payload in the conversation.
			conversationAnsweredApprovals[approvalID] = true;
			try {
				const showing = conversationsCurrentId;
				const view = await apiGet(
					`/api/workspace/conversations/${encodeURIComponent(showing)}?after_id=${conversationAfterID}`,
				);
				// Same rule as the poll: what came back describes the conversation
				// that was on screen when it was asked for, and nothing else.
				if (conversationsCurrentId === showing) applyConversationView(view);
			} catch (_) {
				// The answer was accepted; only the read that would have shown its
				// effect failed. The next poll says the same thing, so nothing is
				// refused here over a read.
			}
		} catch (err) {
			showConversationRefusal(err);
		} finally {
			conversationBusy = false;
			renderConversationPanel();
		}
	}

	// revealConversationRun brings the viewer to the exact block that is waiting
	// (AC-6): the conversation view, the live thread, and the run block at the
	// point where it was asked for.
	async function revealConversationRun(conversationID, anchorEventID) {
		// Reaching the conversation is a selection like any other, so the layout
		// module says what the view becomes — an open spec is left open, and in a
		// narrow window the overlay the press was made in closes with the change
		// of view.
		const next = WorkspaceLayout.nextViewAfterSelection(
			{ view: shellView, specOpen, narrow: shellNarrow },
			"conversation",
		);
		shellView = next.view;
		specOpen = next.specOpen;
		applyShellLayout();
		// The wait belongs to one conversation, the one the entry names: a panel
		// showing any other is brought to that one before the block is looked
		// for. Named by id, because more than one may be alive (AC-1).
		const wanted = conversationID ? String(conversationID) : "";
		if (wanted && wanted !== conversationsCurrentId) {
			await switchToConversation(wanted);
		}
		if (!conversationEl) return;
		const anchor =
			anchorEventID === null || anchorEventID === undefined
				? ""
				: String(anchorEventID);
		if (!anchor) return;
		// Marked, not merely framed: the anchor is raised first so the redraw
		// carries the mark, and the block that comes out of that redraw is the
		// one brought into view.
		conversationHighlightAnchor = anchor;
		renderConversationPanel();
		// Looked up by comparison rather than by an interpolated selector: the
		// anchor is remote text, and it never reaches the selector parser.
		const blocks = conversationEl.querySelectorAll(
			"[data-conversation-run-anchor]",
		);
		for (const block of blocks) {
			if (block.getAttribute("data-conversation-run-anchor") !== anchor) {
				continue;
			}
			if (typeof block.scrollIntoView === "function") {
				block.scrollIntoView({ block: "center" });
			}
			return;
		}
	}

	async function closeConversation() {
		if (conversationBusy) return;
		const id = conversationsCurrentId;
		if (!id) return;
		conversationCloseArmed = false;
		conversationBusy = true;
		renderConversationPanel();
		try {
			// Closing names the conversation being closed (AC-4): the others stay
			// exactly as they were, and none of their state is touched here.
			const view = await apiDelete(
				`/api/workspace/conversations/${encodeURIComponent(id)}?after_id=${conversationAfterID}`,
			);
			conversationRefusal = "";
			// A slot has just been freed, so the reason an open was refused is no
			// longer the truth of this workspace.
			conversationsRefusal = "";
			if (conversationsCurrentId === id) applyConversationView(view);
			stopConversationPolling();
		} catch (err) {
			showConversationRefusal(err);
		} finally {
			conversationBusy = false;
			renderConversationPanel();
			// A closed conversation is a past one: the rail says so from the next
			// read, not from a guess made here.
			loadConversationsIndex();
		}
	}

	// renderConversationPanel draws the panel from the local projection alone.
	// It never fetches and never derives a state the server did not report, and
	// it preserves the two things a re-render would otherwise steal: the text
	// being typed and the reading position in the timeline.
	function renderConversationPanel() {
		if (!conversationEl) return;
		const timeline = conversationEl.querySelector(".conv-timeline");
		const previousTop = timeline ? timeline.scrollTop : 0;
		const wasAtBottom = timeline
			? timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 24
			: true;
		const focused = document.activeElement;
		const composerHadFocus = !!(
			focused &&
			focused.classList &&
			focused.classList.contains("conv-composer-input")
		);
		const caret = composerHadFocus ? focused.selectionStart : 0;

		// The renderer reads the accumulated timeline, not the last page of it:
		// the cursor means the server only ever sends what is new. Before the
		// first read there is no view at all, and the renderer answers that with
		// its own empty state — the invitation to open a conversation — so the
		// home of the workspace is never a blank panel.
		//
		// A conversation that has ended travels in the very same payload as a live
		// one and is drawn by the same branch — one renderer for both, so the
		// history of a conversation looks the same whether it is happening or
		// already happened (AC-3). With nothing on screen the panel draws the
		// invitation, and whether it can be taken comes from the index.
		const view = conversationView
			? Object.assign({}, conversationView, { events: conversationEvents })
			: emptyConversationView();

		// Da quanto si aspetta lo decide qui il pannello, e in un posto solo: se
		// l'attesa comincia adesso si segna l'ora, se e' finita si dimentica.
		// Cosi' il contatore misura l'attesa vera — quella che comincia quando
		// il messaggio parte — e non riparte da zero a ogni ridisegno.
		const working = conversationWorking();
		if (working.active) {
			if (!conversationWorkingSince) conversationWorkingSince = Date.now();
		} else {
			conversationWorkingSince = 0;
		}

		conversationEl.innerHTML = window.Conversation.renderConversation(
			view,
			conversationDraft,
			{
				busy: conversationBusy,
				closeArmed: conversationCloseArmed,
				refusal: conversationRefusal,
				link: conversationLink,
				// Two facts the payload cannot carry, because neither is the
				// server's: how an approval was answered from this panel, and
				// which block the viewer was just sent to.
				answeredApprovals: conversationAnsweredApprovals,
				highlightAnchor: conversationHighlightAnchor,
				// Quali avvisi sono stati chiusi con la loro X: il renderer non
				// disegna quelli, e nessuno dei due lati decide da solo.
				dismissed: conversationDismissedNotices,
				// Come si legge il dettaglio tecnico: quali pieghe sono aperte,
				// e se vanno tenute aperte tutte. Sono scelte di chi guarda, e
				// il server non ne sa niente.
				technicalOpen: conversationTechnicalOpen,
				technicalAll: conversationTechnicalAll,
				// Che l'agente stia lavorando a questo turno non e' un campo del
				// payload: e' letto dalla storia qui sopra e passato come fatto.
				working,
				// Il messaggio consegnato e non ancora tornato indietro. È
				// l'unica cosa che il pannello disegna in coda alla storia senza
				// che il server l'abbia detta, ed è dichiarata come tale.
				pendingMessage: conversationPendingMessage,
				modelChoiceHtml:
					!view.conversation || !conversationIsActive()
						? conversationModelChoiceMarkup()
						: "",
				openingSpecCode: conversationOpeningSpecCode,
				// The recommended step comes from /api/workspace/status and from
				// no other source — scoped to the spec of this conversation when
				// it has one, workspace-wide otherwise. The thread hosts it at
				// its tail — it does not own it, and it does not decide whether
				// it can be taken.
				nextStep: (() => {
					const status = nextStepStatusView();
					return status && typeof status === "object"
						? status.next_step
						: null;
				})(),
				// Il parser Markdown della pagina, iniettato perché il renderer
				// resti puro e consumabile anche dove marked non esiste.
				markedParse:
					typeof marked !== "undefined" &&
					marked &&
					typeof marked.parse === "function"
						? marked.parse.bind(marked)
						: null,
			},
		);

		// Il contatore vive quanto la riga che aggiorna, non un istante di piu'.
		if (conversationWorkingSince) startConversationWorkingTicker();
		else stopConversationWorkingTicker();

		// The declaration of a resume, at the head of the conversation it is
		// about (AC-4). It is drawn from the payload alone — a conversation that
		// took up no other one carries no resumed_from, and the renderer answers
		// that with silence.
		const banner =
			window.ConversationIndex &&
			conversationDismissedNotices["resume"] !== true
				? window.ConversationIndex.renderResumeBanner(view)
				: "";
		if (banner) {
			const panel = conversationEl.querySelector(".conv-panel");
			const head = panel ? panel.querySelector(".conv-head") : null;
			if (head) head.insertAdjacentHTML("afterend", banner);
			else if (panel) panel.insertAdjacentHTML("afterbegin", banner);
		}

		// A conversation that has ended takes no more messages *in itself*, and
		// the renderer is right to say so — but it can be taken up, and the
		// composer is where that is done. So the controls the renderer froze are
		// handed back here, with words that name what pressing Send actually
		// does: a new conversation, given this one as context. This is the
		// panel's local state, which is exactly what the caller owns and the
		// renderer does not.
		if (view && view.conversation && !conversationIsActive()) {
			const resumeInput = conversationEl.querySelector(".conv-composer-input");
			const resumeSend = conversationEl.querySelector(
				".conv-composer .conv-composer-row button[type='submit']",
			);
			if (resumeInput) {
				resumeInput.disabled = conversationBusy;
				resumeInput.placeholder =
					"Scrivi per riprendere questa conversazione…";
			}
			if (resumeSend) resumeSend.disabled = conversationBusy;
			// Che la risposta arrivi in una conversazione nuova non è più un
			// suggerimento stretto accanto al campo: il renderer lo dice per
			// esteso su una riga propria sopra al compositore, dove si legge
			// senza togliere larghezza a ciò che si scrive.
		}

		const nextTimeline = conversationEl.querySelector(".conv-timeline");
		if (nextTimeline) {
			nextTimeline.scrollTop = wasAtBottom
				? nextTimeline.scrollHeight
				: previousTop;
		}
		const input = conversationEl.querySelector(".conv-composer-input");
		if (input) {
			// The draft is restored as a value and never as markup, so no amount
			// of typing can reach the parser.
			input.value = conversationDraft;
			adattaAltezzaCompositore(input);
			if (composerHadFocus && !input.disabled) {
				input.focus();
				const at = Math.min(caret, input.value.length);
				input.setSelectionRange(at, at);
			}
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

	// Lo stato della modale è il form più il corpo nell'editor: il corpo è la
	// parte che costa di più ed è l'unica che non è un campo del form.
	const newSpecGuard = createDirtyGuard(
		() => `${formState(newSpecForm)}\n${newSpecEditor.value()}`,
	);

	function openNewSpec() {
		newSpecForm.reset();
		clearNewSpecErrors();
		clearSpecDraftNotice();
		// Every opening starts on the manual form: the assisted mode is an
		// offer, never a default, and a modal that opened mid-conversation
		// would be showing a run nobody asked for.
		showSpecDraftMode(false);
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
		placeholder.textContent = TEXT.chooseEpic;
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
			newSpecGuard.arm();
			enterModal(newSpecModal);
			return;
		}

		newSpecNoEpics.classList.add("hidden");
		newSpecSubmit.disabled = false;
		newSpecEditor.value(specBodyTemplate());
		newSpecModal.classList.remove("hidden");
		// Il fuoco parte dal titolo: è il primo campo che si compila davvero.
		enterModal(newSpecModal, newSpecForm.title);
		// Il riferimento è il modulo appena preparato, template compreso: quello
		// che il form propone da solo non è lavoro di nessuno, e chiudere senza
		// averlo toccato non deve chiedere niente.
		newSpecGuard.arm();
		setTimeout(() => newSpecEditor.codemirror.refresh(), 0);
	}

	function closeNewSpec() {
		// While a confirmation is in flight the modal cannot be dismissed:
		// closing it would let a late response reopen the editor on top of a
		// draft the user has already started rewriting.
		if (newSpecBusy) return;
		// La bozza è la cosa più costosa che questa modale contiene — scritta a
		// mano o prodotta da un run dell'agente: non si scarta per sbaglio.
		if (!newSpecGuard.allowsClose()) return;
		newSpecGuard.disarm();
		// Closing the form is how a person cancels an assisted creation, so the
		// run is asked to stop. What guarantees that nothing was created is the
		// server, which refuses a proposal that wrote and takes back whatever it
		// wrote; this only spares the operator the wait for a timeout.
		cancelSpecDraftRun();
		teardownSpecDraft();
		newSpecModal.classList.add("hidden");
		leaveModal(newSpecModal);
		clearNewSpecErrors();
		newSpecBusy = false;
		newSpecSubmit.disabled = false;
	}

	// ---- Assisted spec creation ---------------------------------------------
	//
	// The agent proposes, a person confirms. The proposal is poured into the
	// manual form that is already there, which is what makes it reviewable and
	// editable without a second editor, and the only write is still the one the
	// form has always performed on confirmation.

	function showSpecDraftMode(assisted) {
		specDraftModeAssisted.classList.toggle("is-active", assisted);
		specDraftModeAssisted.setAttribute("aria-pressed", String(assisted));
		specDraftModeManual.classList.toggle("is-active", !assisted);
		specDraftModeManual.setAttribute("aria-pressed", String(!assisted));
		specDraftPanel.classList.toggle("hidden", !assisted);
		newSpecForm.classList.toggle("hidden", assisted);
		if (!assisted) setTimeout(() => newSpecEditor.codemirror.refresh(), 0);
	}

	function setSpecDraftNotice(text, tone) {
		specDraftNotice.textContent = text || "";
		specDraftNotice.className = text
			? `form-notice${tone ? ` ${tone}` : ""}`
			: "form-notice hidden";
	}

	function clearSpecDraftNotice() {
		setSpecDraftNotice("");
	}

	function teardownSpecDraft() {
		unmountExecutionPanels(SPEC_DRAFT_CONTEXT);
		specDraftActions.innerHTML = "";
		specDraftExecution.innerHTML = "";
		specDraftRun.innerHTML = "";
		specDraftModelChoice.innerHTML = "";
	}

	// The cancel is fire-and-forget on purpose: the modal must close now, and a
	// run the server refuses to cancel is a run the server will close on its own
	// terms anyway.
	function cancelSpecDraftRun() {
		if (panelContext !== SPEC_DRAFT_CONTEXT) return;
		const record = lastExecutionRecord;
		if (!record || isExecutionTerminal(record)) return;
		apiPost(
			`/api/execution/${encodeURIComponent(record.id)}/run/cancel`,
			{},
		).catch(() => {});
	}

	async function enterAssistedMode() {
		showSpecDraftMode(true);
		clearSpecDraftNotice();
		let view;
		try {
			view = await apiGet("/api/workspace/actions");
		} catch (err) {
			setSpecDraftNotice(
				TEXT.assistedUnavailable(err.message || err),
				"err",
			);
			return;
		}
		if (newSpecModal.classList.contains("hidden")) return;
		const action = ((view && view.actions) || []).find(
			(a) => a.id === SPEC_DRAFT_ACTION,
		);
		if (!action || !action.offered) {
			setSpecDraftNotice(
				(action && action.unavailable_reason) ||
					TEXT.assistedNotOffered,
				"err",
			);
			return;
		}
		mountExecutionPanels({
			context: SPEC_DRAFT_CONTEXT,
			startURL: "/api/workspace/execution",
			actions: specDraftActions,
			execution: specDraftExecution,
			run: specDraftRun,
			modelChoice: specDraftModelChoice,
			settle: settleSpecDraft,
		});
		// Only this action is drawn here. An offered action that is not runnable
		// stays a disabled chip carrying its own reason.
		renderSpecActions([action]);
		// The server hands back the workspace's last execution on every read, so
		// reopening the modal finds the conversation it left behind instead of
		// starting a second one.
		resumeExecution(view.execution, SPEC_DRAFT_CONTEXT);
		loadModelChoice(SPEC_DRAFT_CONTEXT);
	}

	// settleSpecDraft is where the proposal becomes a form. It writes nothing:
	// the spec is created only by the confirmation the person gives afterwards,
	// which is the ordinary submit of this very form.
	async function settleSpecDraft(record) {
		if (!record || record.status !== "SUCCEEDED") return;
		const draft =
			record.result && record.result.payload && record.result.payload.spec_draft;
		if (!draft || typeof draft !== "object") {
			setSpecDraftNotice(
				TEXT.draftUnreadable,
				"err",
			);
			return;
		}
		applySpecDraft(draft);
		showSpecDraftMode(false);
		setSpecDraftNotice(
			TEXT.draftProposed,
		);
	}

	function applySpecDraft(draft) {
		clearNewSpecErrors();
		if (typeof draft.title === "string") newSpecForm.title.value = draft.title;
		if (typeof draft.priority === "string" && draft.priority)
			newSpecForm.priority.value = draft.priority;
		if (typeof draft.points === "number" && draft.points > 0)
			newSpecForm.story_points.value = String(draft.points);
		if (typeof draft.scope === "string") newSpecForm.scope.value = draft.scope;
		newSpecForm.blocked_by.value = Array.isArray(draft.blocked_by)
			? draft.blocked_by.join(", ")
			: "";
		if (typeof draft.body === "string") newSpecEditor.value(draft.body);
		selectProposedEpic(draft.epic_code);
	}

	// The epic is the one field the agent can get wrong in a way the form must
	// not absorb: the select offers the epics the backlog declares, and a value
	// outside that list would either be silently dropped by the browser or,
	// worse, added as an option the workspace has never heard of. So an unknown
	// code leaves the placeholder selected and says so under the field.
	function selectProposedEpic(code) {
		const select = newSpecForm.epic_code;
		const wanted = String(code || "").trim();
		const match = Array.from(select.options).find(
			(opt) => opt.value && opt.value.toLowerCase() === wanted.toLowerCase(),
		);
		if (match) {
			select.value = match.value;
			return;
		}
		select.value = "";
		if (!wanted) return;
		showNewSpecErrors([
			{
				field: "epic_code",
				message: TEXT.draftEpicUnknown(wanted),
			},
		]);
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
				TEXT.fieldsToFix(counted),
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
		newSpecStatus.textContent = TEXT.creating;
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
			// La bozza è stata confermata: non c'è più niente da perdere, e la
			// chiusura non deve chiedere nulla.
			newSpecGuard.disarm();
			closeNewSpec();
			await loadBoard();
			await loadWorkspaceStatus();
			if (res && res.created === false) {
				showToast(TEXT.specExisted(code), "ok");
			} else {
				showToast(TEXT.specCreated(code), "ok");
			}
			if (code) await openEditor(code);
		} catch (err) {
			if (Array.isArray(err.fields) && err.fields.length > 0) {
				showNewSpecErrors(err.fields);
			} else {
				newSpecStatus.textContent = TEXT.createFailed(err.message || err);
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
	const newWorkspaceGuard = createDirtyGuard(() =>
		formState(newWorkspaceForm),
	);

	async function openNewWorkspace() {
		newWorkspaceForm.reset();
		clearWorkspaceErrors();
		newWorkspaceBusy = false;
		newWorkspaceStatus.textContent = "";
		newWorkspaceStatus.className = "status-msg";
		newWorkspaceUnavailable.classList.add("hidden");
		newWorkspaceSubmit.disabled = true;
		newWorkspaceModal.classList.remove("hidden");
		// Il fuoco viene trattenuto subito, non a lettura finita: fra l'apertura
		// e le opzioni c'è un'attesa, e in quell'attesa la modale è già a
		// schermo — Esc e il click sul fondale devono già essere i suoi.
		enterModal(newWorkspaceModal);

		let options;
		try {
			options = await apiGet("/api/workspace/options");
		} catch (err) {
			// Without the contract there is nothing legitimate to offer, so the
			// form says why instead of inventing a plausible list.
			newWorkspaceUnavailableText.textContent = TEXT.workspaceOptionsUnreadable(
				err.message || err,
			);
			newWorkspaceUnavailable.classList.remove("hidden");
			newWorkspaceGuard.arm();
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
		// I valori proposti dal server sono il punto di partenza: quello che
		// arriva già compilato non è lavoro di nessuno.
		newWorkspaceGuard.arm();
		// Il primo campo che si compila davvero è la destinazione.
		newWorkspaceForm.dir.focus();
	}

	function closeNewWorkspace() {
		// While a creation is in flight the modal cannot be dismissed: closing it
		// would hide an operation that is still writing to disk.
		if (newWorkspaceBusy) return;
		if (!newWorkspaceGuard.allowsClose()) return;
		newWorkspaceGuard.disarm();
		newWorkspaceModal.classList.add("hidden");
		leaveModal(newWorkspaceModal);
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
		newWorkspaceStatus.textContent = TEXT.creating;
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
			// The workspace is on disk either way, so a registry that could not
			// record it is reported beside the outcome rather than swallowed.
			const outcome = res.hint || TEXT.workspaceCreated(res.dir);
			newWorkspaceStatus.textContent = res.registryWarning
				? `${outcome} — ${res.registryWarning}`
				: outcome;
			newWorkspaceStatus.className = res.registryWarning
				? "status-msg warn"
				: "status-msg ok";
			showToast(TEXT.workspaceCreated(res.dir), "ok");
			// Il workspace è stato creato: quello che resta nel form è già
			// scritto su disco, non è più una bozza da difendere.
			newWorkspaceGuard.arm();
			// The server records the new workspace in the registry, so the list
			// the user is looking at is already stale. From the modal that was
			// invisible; from the home it is the very list the new workspace
			// should have joined.
			await loadWorkspaces();
		} catch (err) {
			if (Array.isArray(err.fields) && err.fields.length > 0) {
				renderFieldErrors(newWorkspaceForm, newWorkspaceStatus, err.fields);
			} else {
				newWorkspaceStatus.textContent = TEXT.createFailed(err.message || err);
				newWorkspaceStatus.className = "status-msg err";
			}
		} finally {
			newWorkspaceBusy = false;
			newWorkspaceSubmit.disabled = false;
		}
	}

	// ---- Known workspaces ---------------------------------------------------

	// The rows themselves are drawn by workspace-home.js, which also owns the
	// vocabulary of the statuses: the modal and the home must never disagree on
	// what an entry says.

	async function openWorkspaces() {
		workspacesAddForm.reset();
		clearWorkspacesErrors();
		workspacesStatus.textContent = "";
		workspacesStatus.className = "status-msg";
		workspacesModal.classList.remove("hidden");
		enterModal(workspacesModal);
		await loadWorkspaces();
	}

	function closeWorkspaces() {
		workspacesModal.classList.add("hidden");
		leaveModal(workspacesModal);
	}

	function clearWorkspacesErrors() {
		workspacesAddForm.querySelectorAll(".field-error").forEach((el) => {
			el.textContent = "";
		});
		workspacesAddForm.querySelectorAll(".field.has-error").forEach((el) => {
			el.classList.remove("has-error");
		});
	}

	// A failed load reports itself and leaves the list empty instead of throwing:
	// the add form below stays usable even when the registry cannot be read.
	async function loadWorkspaces() {
		try {
			const view = await apiGet("/api/workspaces");
			renderWorkspaces(view);
		} catch (err) {
			const message = TEXT.loadFailed(err.message || err);
			workspacesList.textContent = "";
			workspacesEmpty.classList.add("hidden");
			workspacesStatus.textContent = message;
			workspacesStatus.className = "status-msg err";
			if (noWorkspaceMode()) renderWorkspaceHomeView(null, message);
		}
	}

	// One rendering of the list for both places that show it. The rows come from
	// workspace-home.js, escaped there: a workspace name and path come from the
	// user's disk and are never interpolated raw. The delegated handlers keep
	// working because the data-attributes are the same ones they always read.
	function renderWorkspaces(view) {
		const items = (view && view.workspaces) || [];
		workspacesList.innerHTML = WorkspaceHome.renderWorkspaceRows(view, {
			formatTime: formatExecutionTime,
		});
		workspacesEmpty.classList.toggle("hidden", items.length > 0);
		// The modal payload is the same answer the indicator reads: no extra
		// question is asked where it is already in hand.
		applyWorkspaceIdentity(view);
		if (noWorkspaceMode()) renderWorkspaceHomeView(view, "");
	}

	// openWorkspace hands the switch to the server and then rebuilds the
	// document.
	//
	// Reloading is deliberate rather than lazy: this module keeps board, drawer,
	// execution panels, config and mockup caches at module level, and clearing
	// them one by one would be a list that ages badly — a cache forgotten there
	// is data from the previous workspace still on screen. Rebuilding the
	// document leaves nothing to forget. The viewer process, its HTTP server and
	// its execution store are untouched: only this page is rebuilt.
	async function openWorkspace(id, name) {
		const buttons = document.querySelectorAll(
			"#workspaces-list button, #workspace-home-list button",
		);
		buttons.forEach((b) => {
			b.disabled = true;
		});
		workspacesStatus.textContent = TEXT.workspaceOpening(name || id);
		workspacesStatus.className = "status-msg";
		try {
			const res = await apiPost(
				`/api/workspaces/${encodeURIComponent(id)}/open`,
				{},
			);
			// AC-3: name and title become those of the new workspace here, at
			// the answer of the open — not at a page load. The answer already
			// carries name and path, so no second question is needed.
			applyWorkspaceIdentity({
				open: true,
				currentName: res && res.name,
				currentPath: res && res.path,
			});
			if (res && res.registryWarning) {
				showToast(res.registryWarning, "warn");
				await new Promise((resolve) => setTimeout(resolve, 1200));
			}
			// La spec eventualmente nell'indirizzo era di *quell'altro*
			// workspace: la si toglie prima di ricostruire il documento,
			// altrimenti la pagina appena aperta andrebbe a cercare un codice
			// che qui non esiste. Si guarda l'indirizzo, non la voce di
			// cronologia: il parametro può esserci anche prima che qualcuno
			// abbia aperto qualcosa.
			if (historySupported() && specCodeInLocation()) {
				window.history.replaceState(
					{ archetipoSpec: "" },
					"",
					locationWithSpec(""),
				);
			}
			window.location.reload();
		} catch (err) {
			// The switch was refused, so nothing changed on the server: the board
			// on screen is still the one this page was built for, and it stays
			// usable. Only the reason is new.
			buttons.forEach((b) => {
				b.disabled = false;
			});
			await loadWorkspaces();
			workspacesStatus.textContent = TEXT.workspaceOpenFailed(err.message || err);
			workspacesStatus.className = "status-msg err";
		}
	}

	async function removeWorkspace(id, name) {
		const label = name || id;
		const ok = window.confirm(
			TEXT.workspaceRemoveConfirm(label),
		);
		if (!ok) return;
		try {
			await apiDelete(`/api/workspaces/${encodeURIComponent(id)}`);
			showToast(TEXT.workspaceRemoved, "ok");
			workspacesStatus.textContent = "";
			workspacesStatus.className = "status-msg";
			await loadWorkspaces();
		} catch (err) {
			workspacesStatus.textContent = TEXT.workspaceRemoveFailed(err.message || err);
			workspacesStatus.className = "status-msg err";
		}
	}

	async function onAddWorkspace(e) {
		e.preventDefault();
		clearWorkspacesErrors();
		workspacesAddSubmit.disabled = true;
		workspacesStatus.textContent = TEXT.workspaceAdding;
		workspacesStatus.className = "status-msg";
		try {
			const res = await apiPost("/api/workspaces", {
				path: workspacesAddForm.path.value.trim(),
			});
			workspacesAddForm.reset();
			workspacesStatus.textContent = "";
			workspacesStatus.className = "status-msg";
			showToast(TEXT.workspaceAdded((res && res.name) || TEXT.workspaceFallbackName), "ok");
			await loadWorkspaces();
		} catch (err) {
			if (Array.isArray(err.fields) && err.fields.length > 0) {
				renderFieldErrors(workspacesAddForm, workspacesStatus, err.fields);
			} else {
				workspacesStatus.textContent = TEXT.workspaceAddFailed(err.message || err);
				workspacesStatus.className = "status-msg err";
			}
		} finally {
			workspacesAddSubmit.disabled = false;
		}
	}

	// ---- Metrics -----------------------------------------------------------

	async function openMetrics() {
		metricsModal.classList.remove("hidden");
		enterModal(metricsModal);
		metricsBody.innerHTML = "";
		metricsStatus.textContent = "Loading...";
		metricsStatus.className = "status-msg";
		try {
			const data = await apiGet("/api/metrics");
			renderMetrics(data || {});
			metricsStatus.textContent = "";
		} catch (err) {
			metricsStatus.textContent = TEXT.loadFailed(err.message || err);
			metricsStatus.className = "status-msg err";
		}
	}

	function closeMetrics() {
		metricsModal.classList.add("hidden");
		leaveModal(metricsModal);
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
                        ${totals.done_points || 0}/${totals.points || 0} ${TEXT.metricsPoints} ·
                        ${totals.done_specs || 0}/${totals.specs || 0} ${TEXT.metricsSpecsDone} ·
                        ${totals.wip_specs || 0} ${TEXT.metricsInFlight}
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
				`<h3 class="metrics-section-title">${escapeHtml(TEXT.metricsEpics)}</h3><div class="metrics-epics">`;
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
                        <div class="metrics-epic-caption">${e.done_points || 0}/${e.points || 0} ${TEXT.metricsPoints} · ${e.done_specs || 0}/${e.specs || 0} spec</div>
                    </div>`;
			});
			html += "</div>";
		}

		if (data.flow) {
			html += `
                <h3 class="metrics-section-title">${TEXT.metricsFlow}</h3>
                <div class="metrics-flow">
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${fmtDuration(data.flow.avg_cycle_seconds)}</span><span class="metrics-flow-label">${TEXT.metricsAvgCycle}</span></div>
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${fmtDuration(data.flow.avg_lead_seconds)}</span><span class="metrics-flow-label">${TEXT.metricsAvgLead}</span></div>
                    <div class="metrics-flow-item"><span class="metrics-flow-num">${data.flow.measured_specs}</span><span class="metrics-flow-label">${TEXT.metricsMeasured}</span></div>
                </div>`;
		}

		const rework = data.rework || [];
		const blocked = data.blocked || [];
		if (rework.length > 0 || blocked.length > 0) {
			html +=
				`<h3 class="metrics-section-title">${escapeHtml(TEXT.metricsAttention)}</h3><ul class="metrics-attention">`;
			rework.forEach((code) => {
				html += `<li><span class="metrics-flag rework">rework</span> ${escapeHtml(TEXT.metricsReworkRow(code))}</li>`;
			});
			blocked.forEach((b) => {
				html += `<li><span class="metrics-flag blocked">blocked</span> ${escapeHtml(TEXT.metricsBlockedRow(b.code, (b.blocked_by || []).join(", ")))}</li>`;
			});
			html += "</ul>";
		}

		if ((totals.specs || 0) === 0) {
			html = `<div class="empty-board">${escapeHtml(TEXT.metricsEmpty)}</div>`;
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
		enterModal(prdModal);
		showPrdView();
		prdStatus.textContent = TEXT.configLoading;
		prdStatus.className = "status-msg";
		try {
			await reloadPrdBody();
			prdStatus.textContent = "";
		} catch (err) {
			prdStatus.textContent = TEXT.prdLoadFailed(err.message || err);
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

	// Il PRD è in bozza solo quando la modale è in modalità scrittura: la
	// sentinella si arma entrando in modifica e si disarma uscendone, così in
	// lettura non c'è nessuna domanda da fare — non c'è niente da perdere.
	const prdGuard = createDirtyGuard(() => prdEditor.value());

	function closePRD() {
		if (!prdGuard.allowsClose()) return;
		prdGuard.disarm();
		prdModal.classList.add("hidden");
		leaveModal(prdModal);
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
		// The assisted spec creation is offered beside the manual form, in the
		// New spec modal, so it is deliberately not drawn here: see
		// enterAssistedMode. Everything else the process declares is.
		const offered = ((view && view.actions) || []).filter(
			(a) => a.offered && a.id !== SPEC_DRAFT_ACTION,
		);
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
			modelChoice: inceptionModelChoice,
			settle: settleWorkspaceAction,
		});
		// Only the offered actions are drawn: an offered action that is not
		// runnable stays a disabled chip carrying its `unavailable_reason`.
		renderSpecActions(offered);
		// The server hands back the workspace's last execution on every read, so
		// reopening the modal finds the conversation it left behind and resumes
		// following it without ever starting a second one.
		resumeExecution(view.execution, WORKSPACE_CONTEXT);
		loadModelChoice(WORKSPACE_CONTEXT);
	}

	function hideWorkspaceActions() {
		unmountExecutionPanels(WORKSPACE_CONTEXT);
		prdInception.classList.add("hidden");
		inceptionModelChoice.innerHTML = "";
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
		await loadWorkspaceStatus();
	}

	function fillPrdView(body) {
		prdBodyView.innerHTML = marked.parse(body || TEXT.noPRD);
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
		prdGuard.arm();
		setTimeout(() => prdEditor.codemirror.refresh(), 0);
	}

	function exitPrdEditMode() {
		// Annullare è un modo di scartare come gli altri: se c'è testo scritto,
		// la domanda va fatta anche qui.
		if (!prdGuard.allowsClose()) return;
		prdGuard.disarm();
		prdEditor.value(currentPrdSnapshot || "");
		showPrdView();
	}

	async function onSavePRD(e) {
		e.preventDefault();
		const body = prdEditor.value();
		prdStatus.textContent = TEXT.prdSaving;
		prdStatus.className = "status-msg";
		try {
			await apiPut("/api/prd", { body });
			currentPrdSnapshot = body;
			prdGuard.disarm();
			fillPrdView(body);
			showPrdView();
			prdStatus.textContent = TEXT.prdSaved;
			prdStatus.className = "status-msg ok";
			showToast(TEXT.prdUpdated, "ok");
			// A PRD written by hand is a PRD: the inception must stop being
			// offered and the backlog generation must start being offered, and
			// that verdict is re-read rather than assumed.
			await loadWorkspaceActions();
		} catch (err) {
			prdStatus.textContent = TEXT.saveFailed(err.message || err);
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
			empty.textContent = TEXT.noMockups;
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
			`<span>${TEXT.mockupsSpecs(items.length)}</span>` +
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

	function toggleTopbarMoreMenu(e) {
		e.stopPropagation();
		const wasHidden = topbarMoreMenu.classList.contains("hidden");
		topbarMoreMenu.classList.toggle("hidden");
		topbarMoreBtn.setAttribute("aria-expanded", wasHidden ? "true" : "false");
	}

	function closeTopbarMoreMenu() {
		topbarMoreMenu.classList.add("hidden");
		topbarMoreBtn.setAttribute("aria-expanded", "false");
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
