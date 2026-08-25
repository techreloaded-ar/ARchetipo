// test/web/workspace-home.test.mjs
// Tests for the pure known-workspaces renderer used by the ARchetipo web
// viewer, shared by the workspaces modal and by the home shown when no
// workspace is open.
// Run: node --test test/web/workspace-home.test.mjs
//
// The oracles are on the *visible text* of the rendered HTML, not on the class
// names: what the person choosing a workspace actually reads is what the
// acceptance criteria are about. The two exceptions are the action contract
// (the data-attributes the page delegates on) and the escaping test, and both
// are commented as such where they appear.
//
// Verifies:
//   - AC-1 every entry shows name, path, last opened and reachability
//   - AC-6 open and remove are offered for every entry, and an impossible
//     open is disabled with its reason
//   - a status this frontend has never seen is still shown, raw
//   - a hostile folder name never becomes markup

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

const __dirname = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(
	__dirname,
	"..",
	"..",
	"cli",
	"internal",
	"web",
	"assets",
	"workspace-home.js",
);

// Same minimal virtual-machine loader as workspace-status.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadWorkspaceHome() {
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

const { renderWorkspaceHome, renderWorkspaceRows } = loadWorkspaceHome();

// Strip every attribute from the markup, leaving only what a reader sees.
// A word that survives this is visible text; one that does not was only ever
// hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

// The time format belongs to the caller, so the tests inject a recognisable
// one: an assertion on a locale string would be an assertion on the machine.
// I delimitatori sono volutamente innocui: il renderer sfugge l'HTML, e
// un formato con parentesi angolari proverebbe l'escaping, non il formato.
const formatTime = (value) => (value ? `AT[${value}]` : "");
const opts = { formatTime };

const twoEntries = {
	open: false,
	currentPath: "",
	workspaces: [
		{
			id: "id-uno",
			name: "NOME-UNO",
			path: "/percorso/uno",
			lastOpenedAt: "2026-03-01T09:30:00Z",
			status: "ok",
			reachable: true,
			current: false,
		},
		{
			id: "id-due",
			name: "NOME-DUE",
			path: "/percorso/due",
			lastOpenedAt: "2026-02-01T08:00:00Z",
			status: "missing",
			reachable: false,
			current: false,
		},
	],
};

describe("renderWorkspaceRows", () => {
	it("mostra nome, percorso, ultimo accesso e raggiungibilità di ogni voce", () => {
		const text = visibleText(renderWorkspaceRows(twoEntries, opts));

		assert.ok(text.includes("NOME-UNO"), "il nome della prima voce non è visibile");
		assert.ok(text.includes("NOME-DUE"), "il nome della seconda voce non è visibile");
		assert.ok(text.includes("/percorso/uno"), "il percorso della prima voce non è visibile");
		assert.ok(text.includes("/percorso/due"), "il percorso della seconda voce non è visibile");
		assert.ok(
			text.includes("AT[2026-03-01T09:30:00Z]"),
			"l'ultimo accesso della prima voce non è reso con il formatTime iniettato",
		);
		assert.ok(
			text.includes("AT[2026-02-01T08:00:00Z]"),
			"l'ultimo accesso della seconda voce non è reso con il formatTime iniettato",
		);
		assert.ok(text.includes("raggiungibile"), "la voce raggiungibile non lo dichiara");
		assert.ok(
			text.includes("non trovato"),
			"la voce irraggiungibile non dice perché non lo è",
		);
	});

	it("offre apri e dimentica su ogni voce", () => {
		const html = renderWorkspaceRows(twoEntries, opts);

		// Le uniche asserzioni sugli attributi: sono il contratto fra la resa e
		// gli handler delegati della pagina, non un dettaglio di stile.
		for (const id of ["id-uno", "id-due"]) {
			assert.ok(html.includes(`data-open="${id}"`), `manca apri per ${id}`);
			assert.ok(html.includes(`data-remove="${id}"`), `manca dimentica per ${id}`);
		}
		assert.ok(
			visibleText(html).includes("Apri") && visibleText(html).includes("Rimuovi"),
			"le azioni non sono nominate nel testo visibile",
		);
	});

	it("disabilita l'apertura impossibile e ne dà la ragione", () => {
		const html = renderWorkspaceRows(twoEntries, opts);
		const rows = html.split('<div class="workspace-row">');
		const unreachable = rows.find((r) => r.includes('data-open="id-due"'));
		const reachable = rows.find((r) => r.includes('data-open="id-uno"'));

		assert.ok(
			/data-open="id-due"[^>]*\sdisabled/.test(unreachable),
			"una voce irraggiungibile resta apribile",
		);
		assert.match(
			unreachable,
			/title="Non si può aprire: non trovato"/,
			"l'apertura disabilitata non dice la sua ragione",
		);
		assert.ok(
			!/data-open="id-uno"[^>]*\sdisabled/.test(reachable),
			"una voce raggiungibile non deve essere disabilitata",
		);
	});

	it("marca la voce corrente e non ne offre l'apertura", () => {
		const html = renderWorkspaceRows(
			{
				open: true,
				workspaces: [
					{
						id: "id-corrente",
						name: "NOME-CORRENTE",
						path: "/percorso/corrente",
						lastOpenedAt: "2026-04-01T00:00:00Z",
						status: "ok",
						reachable: true,
						current: true,
					},
				],
			},
			opts,
		);

		assert.ok(
			visibleText(html).includes("aperto"),
			"la voce corrente non è dichiarata tale",
		);
		assert.ok(
			/data-open="id-corrente"[^>]*\sdisabled/.test(html),
			"la voce corrente resta apribile",
		);
		assert.match(
			html,
			/title="È il workspace già aperto"/,
			"l'apertura della voce corrente non dice perché è disabilitata",
		);
	});

	it("disabilita l'apertura di una voce senza identità nel registro", () => {
		const html = renderWorkspaceRows(
			{ workspaces: [{ name: "SENZA-ID", path: "/percorso/senza-id" }] },
			opts,
		);
		assert.ok(/data-open=""[^>]*\sdisabled/.test(html));
		assert.match(html, /title="Questa voce non ha un&#39;identità nel registro"/);
	});

	it("mostra grezzo uno stato che non conosce", () => {
		// Un badge non deve sparire perché il frontend è più vecchio del server.
		const html = renderWorkspaceRows(
			{
				workspaces: [
					{
						id: "id-x",
						name: "NOME-X",
						path: "/percorso/x",
						status: "something_new",
						reachable: false,
					},
				],
			},
			opts,
		);
		assert.ok(
			visibleText(html).includes("something_new"),
			"lo stato sconosciuto non compare nel testo",
		);
	});

	it("non lancia su payload parziali", () => {
		assert.equal(typeof renderWorkspaceRows(null, opts), "string");
		assert.equal(typeof renderWorkspaceRows({}, opts), "string");
		assert.equal(typeof renderWorkspaceRows({ workspaces: null }, opts), "string");
		const html = renderWorkspaceRows(
			{ workspaces: [{ name: "SOLO-NOME" }, {}] },
			opts,
		);
		assert.ok(visibleText(html).includes("SOLO-NOME"));
		// Senza lastOpenedAt la riga esiste comunque, con l'etichetta e nulla dopo.
		assert.ok(visibleText(html).includes("Ultima apertura:"));
	});
});

describe("renderWorkspaceHome", () => {
	it("con elenco vuoto dice che nessun workspace è stato registrato", () => {
		const html = renderWorkspaceHome({ workspaces: [], open: false }, opts);
		assert.ok(
			visibleText(html).includes("Non è ancora stato registrato nessun workspace"),
			"l'elenco vuoto non si spiega",
		);
		assert.ok(
			!html.includes('class="workspace-row"'),
			"un elenco vuoto non deve contenere righe di voce",
		);
	});

	it("elenca i workspace conosciuti con le stesse voci della modale", () => {
		const home = renderWorkspaceHome(twoEntries, opts);
		const rows = renderWorkspaceRows(twoEntries, opts);
		assert.ok(
			home.includes(rows),
			"la home non riusa la stessa resa delle voci",
		);
		const text = visibleText(home);
		assert.ok(text.includes("NOME-UNO") && text.includes("/percorso/due"));
	});

	it("riporta il messaggio di errore che il chiamante le passa", () => {
		const html = renderWorkspaceHome(null, {
			formatTime,
			message: "Load failed: registro illeggibile",
		});
		assert.ok(visibleText(html).includes("Load failed: registro illeggibile"));
		assert.ok(!html.includes('class="workspace-row"'));
	});

	it("non lancia senza payload né opzioni", () => {
		assert.equal(typeof renderWorkspaceHome(null), "string");
		assert.equal(typeof renderWorkspaceHome({ workspaces: [{}] }), "string");
	});

	it("non lascia diventare markup un nome ostile", () => {
		// Unica asserzione non sul testo visibile: qui l'oracolo è proprio la
		// forma del markup, perché è il markup a essere in gioco.
		const html = renderWorkspaceHome(
			{
				workspaces: [
					{
						id: "id-ostile",
						name: '<img src=x onerror=1>',
						path: '/tmp/"virgolette"&e-commerciale',
						status: "ok",
						reachable: true,
					},
				],
			},
			opts,
		);
		assert.ok(!html.includes("<img"), "un nome ostile è diventato markup");
		assert.ok(html.includes("&lt;img src=x onerror=1&gt;"));
		assert.ok(html.includes("&quot;virgolette&quot;&amp;e-commerciale"));
	});
});
