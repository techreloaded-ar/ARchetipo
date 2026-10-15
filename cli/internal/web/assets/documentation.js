// Pure presentation and Wiki-link resolution; lifecycle labels come from Go.
(function () {
	"use strict";
	function escape(value) {
		return String(value || "").replace(/[&<>"']/g, c => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;"})[c]);
	}
	function pageURL(collection, id, anchor) {
		return `?view=documentation&collection=${encodeURIComponent(collection)}${id ? `&page=${encodeURIComponent(id)}` : ""}${anchor ? `#${encodeURIComponent(anchor)}` : ""}`;
	}
	function wikiLink(path, href) {
		if (!href || /^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//")) return null;
		let decoded;
		try { decoded = decodeURIComponent(href); } catch (_) { return null; }
		const [file, anchor = ""] = decoded.split("#");
		if (!file || file.includes("?") || !file.endsWith(".md") || file.includes("\\")) return null;
		const segments = file.startsWith("/") ? [] : path.split("/").slice(0, -1);
		for (const part of file.split("/")) {
			if (!part || part === ".") continue;
			if (part === "..") { if (!segments.length) return null; segments.pop(); }
			else segments.push(part);
		}
		const id = segments.join("/").slice(0, -3);
		return id ? { id, anchor } : null;
	}
	function renderIndex(collections, selectedCollection, selectedPage) {
		return (collections || []).map(collection => {
			const groups = new Map();
			for (const page of collection.items || []) {
				const group = page.id.includes("/") ? page.id.split("/")[0] : "Generale";
				if (!groups.has(group)) groups.set(group, []);
				groups.get(group).push(page);
			}
			return `<section class="docs-collection"><h2>${escape(collection.label)}</h2>${Array.from(groups, ([group, pages]) => `<h3>${escape(group)}</h3><ul>${pages.map(page => `<li><a class="docs-page-link${collection.id === selectedCollection && page.id === selectedPage ? " is-current" : ""}" ${collection.id === selectedCollection && page.id === selectedPage ? 'aria-current="page"' : ""} href="${pageURL(collection.id, page.id)}" data-doc-collection="${escape(collection.id)}" data-doc-page="${escape(page.id)}"><span>${escape(page.meta.title || page.id)}</span><small>${escape(page.state_label)}</small></a></li>`).join("")}</ul>`).join("")}${!collection.available ? '<p class="docs-note">La Wiki non è ancora stata generata.</p>' : !groups.size ? '<p class="docs-note">Nessuna pagina trovata.</p>' : ""}</section>`;
		}).join("");
	}
	function renderAction(action, pageID) {
		return `<button type="button" class="ghost-btn docs-action" data-doc-action="${escape(action.id)}"${pageID ? ` data-doc-action-page="${escape(pageID)}"` : ""}${action.runnable ? "" : " disabled"}>${escape(action.label)}</button>${action.unavailable_reason ? `<small class="docs-action-reason">${escape(action.unavailable_reason)}</small>` : ""}`;
	}
	function renderActions(actions) {
		const primary = (actions || []).filter(action => action.placement === "primary");
		const menu = (actions || []).filter(action => action.placement === "menu");
		return primary.map(action => renderAction(action)).join("") + (menu.length ? `<details class="docs-actions-menu"><summary>Altre azioni</summary>${menu.map(action => renderAction(action)).join("")}</details>` : "");
	}
	function renderActivities(activities) {
		const card = activity => `<section class="docs-activity"><strong>${escape(activity.label)}</strong><p>${escape(activity.status_label)}</p>${activity.conversation_id ? `<button class="ghost-btn" type="button" data-doc-conversation="${escape(activity.conversation_id)}">Apri conversazione</button>` : ""}${activity.changed_pages?.length ? `<details><summary>Pagine cambiate (${activity.changed_pages.length})</summary><ul>${activity.changed_pages.map(id => `<li><a href="${pageURL("wiki", id)}" data-doc-collection="wiki" data-doc-page="${escape(id)}">${escape(id)}</a></li>`).join("")}</ul></details>` : ""}</section>`;
		const active = (activities || []).filter(activity => activity.running);
		const recent = (activities || []).filter(activity => !activity.running);
		return active.map(card).join("") + (recent.length ? `<details class="docs-activity-history"><summary>Attività recenti (${recent.length})</summary>${recent.map(card).join("")}</details>` : "");
	}
	function renderPage(page, renderMarkdown, actions = []) {
		const meta = page.meta || {};
		return `<header class="docs-page-header"><p class="docs-eyebrow">Wiki</p><h1>${escape(meta.title || page.id)}</h1>${meta.description ? `<p>${escape(meta.description)}</p>` : ""}<span class="docs-state">${escape(page.state_label)}</span><div class="docs-page-actions">${actions.filter(action => action.placement === "page").map(action => renderAction(action, page.id)).join("")}</div></header><details class="docs-provenance"><summary>Fonti e revisione</summary><p>${escape(page.path)}</p>${meta.review ? `<p>Ultima revisione: ${escape(meta.review.reviewed_at)}</p>` : ""}<ul>${(meta.sources || []).map(source => `<li><code>${escape(source.path)}</code>${source.role ? ` · ${escape(source.role)}` : ""}</li>`).join("")}</ul>${(meta.issues || []).map(issue => `<p>${escape(issue.code)}: ${escape(issue.summary)}</p>`).join("")}</details><div class="docs-markdown markdown-rendered">${renderMarkdown(page.body || "")}</div>`;
	}
	const api = { pageURL, wikiLink, renderIndex, renderPage, renderActions, renderActivities };
	if (typeof module !== "undefined" && module.exports) module.exports = api;
	else window.Documentation = api;
})();
