// signin-panel.js
// Pure renderer for the GET /api/signin payload: one card per account, with
// its state and, during a sign-in, the link, the code and the paste field the
// account's own command asked for.
//
// It is pure: no DOM, no fetch, no document. It takes the payload and returns
// an HTML string. The data-attributes are the whole contract with the page:
// data-signin-start, data-signin-cancel and the data-signin-code form.
//
// Consumable in both browser (defines window.SigninPanel) and Node
// (exports renderSigninPanel / escapeHtml).
(function () {
	const TEXT = {
		title: "Accessi",
		subtitle:
			"Gli accessi restano nell’ambiente isolato: ARchetipo non vede le password e non legge nulla dal tuo computer.",
		unavailable:
			"Qui gli accessi si fanno dal tuo terminale, con il comando dell’agente o di GitHub.",
		signedIn: "collegato",
		signedOut: "non collegato",
		missingTool: "strumento non disponibile nell’ambiente",
		signIn: "Accedi",
		retry: "Riprova",
		cancel: "Annulla",
		starting: "Avvio dell’accesso…",
		openPage: "Apri la pagina di accesso",
		typeCode: "Sulla pagina inserisci questo codice:",
		pasteCode: "Codice mostrato dalla pagina dopo l’autorizzazione",
		sendCode: "Invia il codice",
		codeSent: "Codice inviato, attendo la conferma…",
		failed: "Accesso non riuscito",
		cancelled: "Accesso annullato",
		blocked: (domain) =>
			`Se la pagina non si apre, prova un altro browser: deve poter raggiungere ${domain}.`,
	};

	function escapeHtml(value) {
		return String(value == null ? "" : value)
			.replace(/&/g, "&amp;")
			.replace(/</g, "&lt;")
			.replace(/>/g, "&gt;")
			.replace(/"/g, "&quot;")
			.replace(/'/g, "&#39;");
	}

	function safeLink(url) {
		return typeof url === "string" && url.startsWith("https://") ? url : "";
	}

	function renderFlow(account) {
		const flow = account.flow;
		const kind = escapeHtml(account.kind);
		if (flow.state !== "waiting") return "";
		const link = safeLink(flow.url);
		let body = "";
		if (!link) {
			body += `<p class="signin-step">${escapeHtml(TEXT.starting)}</p>`;
		} else {
			body +=
				`<p class="signin-step"><a class="primary-btn" href="${escapeHtml(link)}" target="_blank" rel="noopener noreferrer">${escapeHtml(TEXT.openPage)}</a></p>`;
			if (!flow.needs_code && flow.code) {
				body +=
					`<p class="signin-step">${escapeHtml(TEXT.typeCode)} <code class="signin-code">${escapeHtml(flow.code)}</code></p>`;
			}
			if (flow.needs_code && flow.code_sent) {
				body += `<p class="signin-step">${escapeHtml(TEXT.codeSent)}</p>`;
			} else if (flow.needs_code) {
				body +=
					`<form class="signin-code-form" data-signin-code="${kind}">` +
					`<label class="field full"><span>${escapeHtml(TEXT.pasteCode)}</span>` +
					`<input type="text" name="code" autocomplete="off" spellcheck="false" required /></label>` +
					`<button type="submit" class="primary-btn">${escapeHtml(TEXT.sendCode)}</button>` +
					`</form>`;
			}
			body += `<p class="field-hint">${escapeHtml(TEXT.blocked(account.domain))}</p>`;
		}
		body += `<button type="button" class="ghost-btn" data-signin-cancel="${kind}">${escapeHtml(TEXT.cancel)}</button>`;
		return `<div class="signin-flow">${body}</div>`;
	}

	function renderCard(account) {
		const kind = escapeHtml(account.kind);
		const flow = account.flow || null;
		const waiting = Boolean(flow && flow.state === "waiting");
		let status;
		if (!account.available) status = TEXT.missingTool;
		else if (account.signed_in)
			status = account.detail
				? `${TEXT.signedIn} · ${account.detail}`
				: TEXT.signedIn;
		else status = TEXT.signedOut;

		let ended = "";
		if (flow && flow.state === "failed") {
			ended = `<p class="status-msg err">${escapeHtml(TEXT.failed)}${flow.error ? ": " + escapeHtml(flow.error) : ""}</p>`;
		} else if (flow && flow.state === "cancelled") {
			ended = `<p class="status-msg">${escapeHtml(TEXT.cancelled)}</p>`;
		}

		let action = "";
		if (account.available && !account.signed_in && !waiting) {
			const label = flow && flow.state === "failed" ? TEXT.retry : TEXT.signIn;
			action = `<button type="button" class="primary-btn" data-signin-start="${kind}">${escapeHtml(label)}</button>`;
		}

		return (
			`<article class="signin-card${account.signed_in ? " is-signed-in" : ""}" data-signin-kind="${kind}">` +
			`<header class="signin-card-head"><h3>${escapeHtml(account.label)}</h3>` +
			`<span class="signin-status">${escapeHtml(status)}</span></header>` +
			ended +
			(waiting ? renderFlow(account) : action) +
			`</article>`
		);
	}

	function renderSigninPanel(view) {
		const head =
			`<p class="signin-sub">${escapeHtml(TEXT.subtitle)}</p>`;
		if (!view || !view.available) {
			return `<p class="form-notice">${escapeHtml(TEXT.unavailable)}</p>`;
		}
		const accounts = Array.isArray(view.accounts) ? view.accounts : [];
		return head + `<div class="signin-cards">${accounts.map(renderCard).join("")}</div>`;
	}

	// True while any sign-in waits for the person, so the page knows to keep
	// reading the payload; the decision is the payload's, not a rule here.
	function anyWaiting(view) {
		return Boolean(
			view &&
				Array.isArray(view.accounts) &&
				view.accounts.some((a) => a.flow && a.flow.state === "waiting"),
		);
	}

	if (typeof module !== "undefined" && module.exports) {
		module.exports = { renderSigninPanel, anyWaiting, escapeHtml };
	} else {
		window.SigninPanel = { renderSigninPanel, anyWaiting };
	}
})();
