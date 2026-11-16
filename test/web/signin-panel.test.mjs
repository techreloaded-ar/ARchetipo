// test/web/signin-panel.test.mjs
// Tests for the pure renderer of the accounts panel shown in the isolated
// runtime.
// Run: node --test test/web/signin-panel.test.mjs
//
// The oracles are on the visible text, except for the action contract (the
// data-attributes the page delegates on) and the link and escaping tests.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

const __dirname = dirname(fileURLToPath(import.meta.url));
const modulePath = resolve(__dirname, "..", "..", "cli", "internal", "web", "assets", "signin-panel.js");

function load() {
	const mod = { exports: {} };
	runInContext(readFileSync(modulePath, "utf8"), createContext({ module: mod, window: undefined }));
	return mod.exports;
}

const { renderSigninPanel, anyWaiting } = load();

function visibleText(html) {
	return html.replace(/<[^>]*>/g, " ").replace(/\s+/g, " ");
}

const base = { kind: "claude", label: "Claude", domain: "claude.ai", available: true };

describe("signin-panel", () => {
	it("says where to sign in when the viewer offers no sign-in", () => {
		const html = renderSigninPanel({ available: false, accounts: [] });
		assert.match(visibleText(html), /dal tuo terminale/);
		assert.doesNotMatch(html, /data-signin-start/);
	});

	it("offers a sign-in only to an available account that is signed out", () => {
		const html = renderSigninPanel({
			available: true,
			accounts: [
				{ ...base, signed_in: true, detail: "abbonamento max" },
				{ ...base, kind: "codex", label: "Codex (ChatGPT)", signed_in: false },
				{ ...base, kind: "github", label: "GitHub", available: false },
			],
		});
		const text = visibleText(html);
		assert.match(text, /collegato · abbonamento max/);
		assert.match(text, /non collegato/);
		assert.match(text, /strumento non disponibile/);
		assert.deepEqual([...html.matchAll(/data-signin-start="([^"]+)"/g)].map((m) => m[1]), ["codex"]);
	});

	it("shows the link and the code to type for a device-code sign-in", () => {
		const html = renderSigninPanel({
			available: true,
			accounts: [{ ...base, kind: "codex", domain: "auth.openai.com", signed_in: false,
				flow: { state: "waiting", url: "https://auth.openai.com/codex/device", code: "LD2K-UTC6P", needs_code: false } }],
		});
		assert.match(html, /href="https:\/\/auth\.openai\.com\/codex\/device"/);
		assert.match(html, /rel="noopener noreferrer"/);
		assert.match(visibleText(html), /inserisci questo codice: LD2K-UTC6P/);
		assert.match(visibleText(html), /deve poter raggiungere auth\.openai\.com/);
		assert.doesNotMatch(html, /data-signin-code=/);
		assert.match(html, /data-signin-cancel="codex"/);
		assert.doesNotMatch(html, /data-signin-start/);
	});

	it("asks for the pasted code, then says it was sent", () => {
		const waiting = { state: "waiting", url: "https://claude.com/cai/oauth/authorize?x=1", needs_code: true };
		let html = renderSigninPanel({ available: true, accounts: [{ ...base, signed_in: false, flow: waiting }] });
		assert.match(html, /<form class="signin-code-form" data-signin-code="claude">/);
		html = renderSigninPanel({ available: true, accounts: [{ ...base, signed_in: false, flow: { ...waiting, code_sent: true } }] });
		assert.doesNotMatch(html, /data-signin-code=/);
		assert.match(visibleText(html), /Codice inviato/);
	});

	it("never renders a link that is not https, and escapes everything", () => {
		const html = renderSigninPanel({
			available: true,
			accounts: [{ ...base, label: "<img src=x>", signed_in: false,
				flow: { state: "waiting", url: "javascript:alert(1)", needs_code: true } }],
		});
		assert.doesNotMatch(html, /javascript:/);
		assert.doesNotMatch(html, /<img/);
		assert.match(visibleText(html), /Avvio dell’accesso/);
	});

	it("reports a failed sign-in with its reason and offers a retry", () => {
		const html = renderSigninPanel({
			available: true,
			accounts: [{ ...base, signed_in: false, flow: { state: "failed", error: "denied" } }],
		});
		assert.match(visibleText(html), /Accesso non riuscito: denied/);
		assert.match(html, /data-signin-start="claude">Riprova/);
	});

	it("tells the page whether any sign-in is still waiting", () => {
		assert.equal(anyWaiting({ accounts: [{ flow: { state: "waiting" } }] }), true);
		assert.equal(anyWaiting({ accounts: [{ flow: { state: "succeeded" } }, {}] }), false);
		assert.equal(anyWaiting(null), false);
	});
});
