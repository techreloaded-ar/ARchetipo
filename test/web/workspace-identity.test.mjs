// test/web/workspace-identity.test.mjs
// Tests for the pure workspace-identity module and for the wiring that puts
// what it returns on the page.
// Run: node --test test/web/workspace-identity.test.mjs
//
// The module is the only thing that decides what is read in the topbar and in
// the browser tab: it has no DOM, so what it returns *is* what is written.
// Asserting on its return value is therefore asserting on what is on screen,
// with no test double standing between the two.
//
// The second suite reads index.html, app.css and app.js as text, because the
// remaining link — where the indicator sits and when the identity is
// reapplied — is visible to neither a module test nor an HTTP test.
//
// Verifies:
//   - AC-1 the open workspace is named, and its full path is available from
//     the indicator, which lives in the topbar and not in a window
//   - AC-2 two different workspaces produce two different tab titles
//   - AC-3 the identity is a function of the payload, and the wiring reapplies
//     it on the SSE tick and at the answer of the open
//   - AC-4 the indicator is a button whose click reaches the known workspaces
//   - AC-5 with no workspace open the indicator declares the absence, and is
//     not hidden by the no-workspace rule
//   - out of scope: the open behaviour (the single location.reload) is intact

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

const __dirname = dirname(fileURLToPath(import.meta.url));
const assetsDir = resolve(
	__dirname,
	"..",
	"..",
	"cli",
	"internal",
	"web",
	"assets",
);
const helperPath = resolve(assetsDir, "workspace-identity.js");
const htmlPath = resolve(assetsDir, "index.html");
const cssPath = resolve(assetsDir, "app.css");
const appPath = resolve(assetsDir, "app.js");

// Same minimal virtual-machine loader as workspace-layout.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadWorkspaceIdentity() {
	const src = readFileSync(helperPath, "utf8");
	const mod = { exports: {} };
	const ctx = createContext({
		module: mod,
		// No `window` — the UMD will detect `module` and use that path.
		window: undefined,
	});
	runInContext(src, ctx);
	return mod.exports;
}

const { resolveWorkspaceIdentity, EMPTY_LABEL, EMPTY_TITLE } =
	loadWorkspaceIdentity();

describe("resolveWorkspaceIdentity", () => {
	it("nomina il workspace aperto e ne espone il percorso completo (AC-1)", () => {
		const path = "/tmp/targets/alfa";
		const id = resolveWorkspaceIdentity({
			open: true,
			currentName: "alfa",
			currentPath: path,
		});
		assert.equal(id.open, true);
		assert.equal(id.label, "alfa");
		// The full path, not a prefix of it and not something recomputed from it.
		assert.equal(id.tooltip, path);
		assert.equal(id.actionable, true);
	});

	it("il titolo della scheda distingue due workspace diversi (AC-2)", () => {
		const alfa = resolveWorkspaceIdentity({
			open: true,
			currentName: "alfa",
			currentPath: "/tmp/targets/alfa",
		});
		const beta = resolveWorkspaceIdentity({
			open: true,
			currentName: "beta",
			currentPath: "/tmp/targets/beta",
		});
		assert.notEqual(alfa.documentTitle, beta.documentTitle);
		// The property the spec asks for is that each title names its own
		// workspace, not that the suffix is a particular string.
		assert.ok(alfa.documentTitle.includes("alfa"));
		assert.ok(beta.documentTitle.includes("beta"));
	});

	it("applicato al payload del nuovo workspace produce la nuova identità (AC-3)", () => {
		// This is what makes the transition a function of the state and not an
		// effect of a page load: the same call, on the payload of the workspace
		// just opened, already says everything the page has to show.
		const before = resolveWorkspaceIdentity({
			open: true,
			currentName: "alfa",
			currentPath: "/tmp/targets/alfa",
		});
		const after = resolveWorkspaceIdentity({
			open: true,
			currentName: "beta",
			currentPath: "/tmp/targets/beta",
		});
		assert.notEqual(before.label, after.label);
		assert.notEqual(before.tooltip, after.tooltip);
		assert.notEqual(before.documentTitle, after.documentTitle);
		assert.equal(after.label, "beta");
		assert.equal(after.tooltip, "/tmp/targets/beta");
	});

	it("senza workspace aperto lo dichiara invece di mostrare un nome (AC-5)", () => {
		const id = resolveWorkspaceIdentity({ open: false, workspaces: [] });
		assert.equal(id.open, false);
		assert.equal(id.label, EMPTY_LABEL);
		assert.equal(id.tooltip, "");
		assert.equal(id.documentTitle, EMPTY_TITLE);
		assert.equal(id.actionable, false);
	});

	it("non lancia su payload parziali e non mostra mai un'etichetta vuota (AC-5)", () => {
		const partials = [
			null,
			undefined,
			{},
			{ open: true },
			{ open: true, currentName: "   " },
		];
		for (const payload of partials) {
			const id = resolveWorkspaceIdentity(payload);
			assert.equal(
				id.open,
				false,
				`payload ${JSON.stringify(payload)} should fall back to the closed state`,
			);
			assert.equal(id.actionable, false);
			assert.equal(id.label, EMPTY_LABEL);
			assert.notEqual(id.label, "");
			assert.equal(id.documentTitle, EMPTY_TITLE);
		}
	});
});

// The body of a top-level function of app.js: from its declaration to the next
// declaration at the same indentation level. Searching the whole file for a
// call would pass even if the call sat somewhere else entirely.
function functionBody(src, name) {
	const lines = src.split("\n");
	const start = lines.findIndex((line) =>
		new RegExp(`^\\t(?:async )?function ${name}\\b`).test(line),
	);
	assert.ok(start >= 0, `app.js does not declare ${name}`);
	let end = lines.length;
	for (let i = start + 1; i < lines.length; i += 1) {
		if (/^\t(?:async )?function \w+/.test(lines[i])) {
			end = i;
			break;
		}
	}
	return lines.slice(start, end).join("\n");
}

describe("l'indicatore nella pagina", () => {
	const html = readFileSync(htmlPath, "utf8");
	const css = readFileSync(cssPath, "utf8");
	const app = readFileSync(appPath, "utf8");

	it("vive nella topbar, dentro la cella del brand (AC-1)", () => {
		const indicator = html.indexOf('id="workspace-indicator"');
		const brand = html.indexOf('class="brand"');
		const headerEnd = html.indexOf("</header>");
		assert.ok(indicator >= 0, "index.html has no #workspace-indicator");
		assert.ok(brand >= 0 && brand < indicator);
		assert.ok(
			indicator < headerEnd,
			"the indicator is outside the topbar header",
		);
	});

	it("non è workspace-scoped, quindi resta visibile a workspace chiuso (AC-5)", () => {
		const open = html.indexOf('<button id="workspace-indicator"');
		assert.ok(open >= 0);
		const tag = html.slice(open, html.indexOf(">", open) + 1);
		assert.ok(
			!tag.includes("data-workspace-scoped"),
			"the indicator carries data-workspace-scoped and would be hidden with no workspace open",
		);
		// The rule that absence is there to avoid still exists: without it the
		// assertion above would be about nothing.
		assert.match(css, /body\.no-workspace[\s\S]{0,200}\[data-workspace-scoped\]/);
	});

	it("è un bottone e il suo click porta ai workspace conosciuti (AC-4)", () => {
		const open = html.indexOf('<button id="workspace-indicator"');
		assert.ok(open >= 0, "the indicator is not a <button>");
		assert.match(
			app,
			/workspaceIndicator\.addEventListener\(\s*"click",\s*openWorkspaces\s*\)/,
		);
	});

	it("scrive il titolo della scheda a partire dall'identità risolta (AC-2)", () => {
		// Before this spec the string `document.title` did not appear in app.js
		// at all, so this assertion has a real oracle and is not tautological.
		assert.match(app, /document\.title\s*=/);
		assert.match(
			functionBody(app, "applyWorkspaceIdentity"),
			/document\.title\s*=\s*id\.documentTitle/,
		);
		assert.match(
			functionBody(app, "applyWorkspaceIdentity"),
			/WorkspaceIdentity\.resolveWorkspaceIdentity/,
		);
	});

	it("riapplica l'identità sul tick SSE e alla risposta dell'apertura (AC-3)", () => {
		assert.match(
			functionBody(app, "scheduleBoardReload"),
			/refreshWorkspaceIdentity\(\)/,
		);
		assert.match(
			functionBody(app, "openWorkspace"),
			/applyWorkspaceIdentity\(/,
		);
	});

	it("non tocca il comportamento di apertura, dichiarato fuori ambito", () => {
		const occurrences = app.match(/location\.reload/g) || [];
		assert.equal(
			occurrences.length,
			1,
			"opening a workspace still reloads exactly once: the open behaviour is out of scope for this spec",
		);
	});

	it("carica workspace-identity.js prima di app.js", () => {
		const helper = html.indexOf("/workspace-identity.js");
		const main = html.indexOf('src="/app.js"');
		assert.ok(helper >= 0, "index.html does not load workspace-identity.js");
		assert.ok(helper < main, "workspace-identity.js is loaded after app.js");
	});
});
