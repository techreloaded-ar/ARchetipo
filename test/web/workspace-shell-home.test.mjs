// test/web/workspace-shell-home.test.mjs
// Structural oracles for the home of the workspace (US-057).
// Run: node --test test/web/workspace-shell-home.test.mjs
//
// What decides which view is on screen lives in a pure module and is tested
// there (workspace-layout.test.mjs). What is checked here is everything that
// module cannot possibly know, because it is a property of *where* things are
// written rather than of what a function returns:
//
//   - the conversation is mounted inside the primary column and is born
//     visible, and the lateral rail is gone (AC-1);
//   - the view switcher is one, never hidden, and wired on data-shell-view, so
//     the board is one command away from the home screen (AC-2);
//   - no path that changes view ever empties or rebuilds the conversation, so
//     its history and the text being typed survive (AC-3);
//   - the spec detail is a region of the primary column, not a window (AC-4);
//   - nothing hides the counters — which US-061 AC-6 moved from the topbar to
//     the board — and boot() reads the board even when the board is not the
//     visible view (AC-5);
//   - the narrow state is styled, overlay included (AC-6).
//
// These are facts a future refactor would break in silence: no unit test would
// go red, and the screen would only misbehave for a person. Hence the reading
// of the sources themselves. Nothing here asserts formatting, indentation or
// line numbers — only facts whose loss would break an acceptance criterion, and
// every failure message names the broken fact rather than a string diff.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

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

const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");
const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");

// Extracts the balanced `{...}` block that opens after `marker`. Used to read a
// single function body, so an assertion about "what this function does" cannot
// be satisfied by a coincidence somewhere else in a 5000-line file.
function sectionOf(source, marker) {
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `la sorgente non contiene più \`${marker}\``);
	const open = source.indexOf("{", at);
	assert.notEqual(open, -1, `nessun blocco dopo \`${marker}\``);
	let depth = 0;
	for (let i = open; i < source.length; i++) {
		if (source[i] === "{") depth++;
		else if (source[i] === "}") {
			depth--;
			if (depth === 0) return source.slice(open + 1, i);
		}
	}
	assert.fail(`blocco non bilanciato dopo \`${marker}\``);
}

// The opening tag of the element carrying `id="<id>"`, so an assertion about an
// attribute reads that element's own attributes and nobody else's.
function openingTagOf(source, id) {
	const at = source.indexOf(`id="${id}"`);
	assert.notEqual(at, -1, `index.html non contiene più #${id}`);
	const start = source.lastIndexOf("<", at);
	const end = source.indexOf(">", at);
	assert.ok(start !== -1 && end !== -1, `tag malformato attorno a #${id}`);
	return source.slice(start, end + 1);
}

// Every CSS rule as {selector, body}. Comments are dropped first so a failure
// names the offending selector and not the paragraph written above it.
function cssRules(text) {
	const stripped = text.replace(/\/\*[\s\S]*?\*\//g, "");
	const rules = [];
	const re = /([^{}]+)\{([^{}]*)\}/g;
	let m;
	while ((m = re.exec(stripped)) !== null) {
		rules.push({ selector: m[1].trim().replace(/\s+/g, " "), body: m[2] });
	}
	return rules;
}

describe("AC-1 — la conversazione è la casa del workspace", () => {
	it("è montata dentro la colonna principale, prima della board", () => {
		const primary = html.indexOf('id="workspace-primary"');
		const conversation = html.indexOf('id="workspace-conversation"');
		const board = html.indexOf('id="board"');
		assert.notEqual(
			primary,
			-1,
			"index.html non contiene più #workspace-primary",
		);
		assert.notEqual(
			conversation,
			-1,
			"index.html non contiene più #workspace-conversation",
		);
		assert.ok(
			conversation > primary,
			"la conversazione è dichiarata fuori da #workspace-primary: non sarebbe la vista della colonna principale ma un riquadro a parte, e AC-1 cadrebbe",
		);
		assert.ok(
			conversation < board,
			"la board è dichiarata prima della conversazione dentro la colonna: la casa del workspace deve essere il primo figlio della colonna principale",
		);
	});

	it("nasce visibile e non porta la classe hidden", () => {
		const tag = openingTagOf(html, "workspace-conversation");
		assert.ok(
			tag.includes("is-visible"),
			`la conversazione deve nascere visibile: la schermata iniziale sarebbe vuota. Tag trovato: ${tag}`,
		);
		assert.ok(
			!/\bclass="[^"]*\bhidden\b/.test(tag),
			`la conversazione non deve nascere con la classe hidden: la sua visibilità appartiene al layout. Tag trovato: ${tag}`,
		);
		assert.ok(
			tag.includes("shell-pane-conversation"),
			`la conversazione deve portare la classe di riquadro che il modulo restituisce. Tag trovato: ${tag}`,
		);
	});

	it("la board nasce nascosta, nella stessa colonna", () => {
		const tag = openingTagOf(html, "board");
		assert.ok(
			tag.includes("is-hidden"),
			`all'apertura la board non è la vista mostrata e deve nascere nascosta. Tag trovato: ${tag}`,
		);
		assert.ok(
			tag.includes("shell-pane-board"),
			`la board deve restare un riquadro della colonna principale. Tag trovato: ${tag}`,
		);
	});

	it("la rail laterale non esiste più", () => {
		assert.ok(
			!html.includes('id="workspace-rail"'),
			"index.html contiene ancora <aside id=\"workspace-rail\">: la rail è stata rimossa da US-057 e ritrovarla significa che la conversazione è di nuovo stretta in una colonna laterale",
		);
		assert.ok(
			!css.includes(".workspace-rail") && !css.includes(".shell-pane-rail"),
			"app.css contiene ancora regole della rail rimossa: sono selettori orfani che nessuno può più far corrispondere",
		);
	});

	it("la striscia delle run sta fuori dalla colonna principale", () => {
		const runs = html.indexOf('id="workspace-runs"');
		const primary = html.indexOf('id="workspace-primary"');
		const shell = html.indexOf('id="workspace-shell"');
		assert.notEqual(runs, -1, "index.html non contiene più #workspace-runs");
		assert.ok(
			runs > shell && runs < primary,
			"la striscia delle run deve stare dentro la shell e fuori dalla colonna principale: dentro la colonna toglierebbe spazio alla conversazione e sparirebbe cambiando vista",
		);
	});
});

describe("AC-2 — la board è a un solo comando", () => {
	it("il commutatore di vista è dichiarato una volta sola e non è nascosto", () => {
		const occurrences = html.split('id="workspace-views"').length - 1;
		assert.equal(
			occurrences,
			1,
			`#workspace-views deve essere dichiarato una volta sola nel documento; trovate ${occurrences} dichiarazioni`,
		);
		const tag = openingTagOf(html, "workspace-views");
		assert.ok(
			!/\bclass="[^"]*\bhidden\b/.test(tag),
			`il commutatore è permanente e non deve nascere nascosto. Tag trovato: ${tag}`,
		);
	});

	it("è cablato su data-shell-view, e il vecchio data-shell-target è sparito", () => {
		assert.ok(
			js.includes("data-shell-view"),
			"app.js non legge più data-shell-view: nessun bottone del commutatore cambierebbe vista",
		);
		assert.ok(
			!js.includes("data-shell-target"),
			"app.js contiene ancora data-shell-target, l'attributo dei vecchi commutatori della rail: due cablaggi per lo stesso gesto",
		);
		const listener = sectionOf(js, 'shellViewsEl.addEventListener("click"');
		assert.ok(
			listener.includes("data-shell-view"),
			"il listener del commutatore non riconosce più i bottoni di vista",
		);
		assert.ok(
			listener.includes("setShellView"),
			"il listener del commutatore non delega più a setShellView",
		);
	});
});

describe("AC-3 — la bozza e la storia non si perdono cambiando vista", () => {
	it("applyPaneState scrive solo classi", () => {
		const body = sectionOf(js, "function applyPaneState(");
		assert.ok(
			!body.includes("innerHTML"),
			"applyPaneState scrive innerHTML: cambiare vista ricostruirebbe un riquadro invece di nasconderlo, e la conversazione perderebbe storia e bozza",
		);
		assert.ok(
			body.includes("classList"),
			"applyPaneState non scrive più classi: non applicherebbe alcuna decisione del layout",
		);
	});

	it("nessun percorso di cambio vista tocca lo stato della conversazione", () => {
		for (const marker of [
			"function setShellView(",
			"function applyShellLayout(",
			"function renderShellViews(",
		]) {
			const body = sectionOf(js, marker);
			for (const forbidden of [
				"resetConversationState",
				"conversationEl.innerHTML",
				"conversationDraft =",
			]) {
				assert.ok(
					!body.includes(forbidden),
					`\`${marker}\` contiene \`${forbidden}\`: un cambio di vista azzererebbe o ricostruirebbe la conversazione, e tornando dalla board la storia o il testo non inviato sarebbero persi`,
				);
			}
		}
	});

	it("conversationDraft è assegnata solo nei tre punti attesi", () => {
		// I contesti ammessi: l'evento `input` del compositore (e la sua scorciatoia
		// da tastiera), l'invio che la azzera dopo aver spedito — nella
		// conversazione aperta come nella ripresa di una passata, che è un invio
		// anch'essa (US-058 AC-4) — e il reset di workspace. Nessuno è un cambio
		// di vista.
		const assignments = js
			.split("\n")
			.map((line, i) => ({ line: line.trim(), n: i + 1 }))
			.filter(({ line }) => /(^|[^.\w])conversationDraft\s*=[^=]/.test(line))
			// La dichiarazione non è un percorso di scrittura: la bozza nasce vuota.
			.filter(({ line }) => !line.startsWith("let conversationDraft"))
			.map((entry) => ({
				...entry,
				line: entry.line.replace(/^if \(input\) /, ""),
			}));
		const expected = [
			"conversationDraft = input.value;", // input del compositore
			"conversationDraft = input.value;", // scorciatoia cmd/ctrl+invio
			'conversationDraft = "";', // ripresa riuscita di una conversazione passata
			'conversationDraft = "";', // reset di workspace
			'conversationDraft = "";', // invio riuscito
		];
		assert.deepEqual(
			assignments.map(({ line }) => line),
			expected,
			`le assegnazioni a conversationDraft non sono più quelle attese. Ammesse: l'evento input del compositore, la scorciatoia da tastiera, l'azzeramento dopo l'invio riuscito e il reset di workspace. Trovate: ${assignments
				.map(({ n, line }) => `${n}: ${line}`)
				.join(" | ")}`,
		);
	});

	it("l'unico azzeramento della bozza fuori dall'invio è il reset di workspace", () => {
		const body = sectionOf(js, "function resetConversationState(");
		assert.ok(
			body.includes('conversationDraft = ""'),
			"resetConversationState non azzera più la bozza: la conversazione del workspace lasciato sopravviverebbe a quello nuovo",
		);
	});
});

describe("AC-4 — il dettaglio spec non è una finestra", () => {
	it("è una regione della colonna principale", () => {
		const primary = html.indexOf('id="workspace-primary"');
		const modalRoot = html.indexOf('id="modal-root"');
		assert.ok(
			modalRoot > primary,
			"#modal-root è uscito dalla colonna principale: tornerebbe a essere una finestra sovrapposta",
		);
		const tag = openingTagOf(html, "modal-root");
		assert.ok(
			tag.includes('role="region"'),
			`#modal-root deve restare una regione. Tag trovato: ${tag}`,
		);
		assert.ok(
			!tag.includes("aria-modal"),
			`#modal-root porta aria-modal: sarebbe di nuovo una finestra che rende inerte il resto della pagina. Tag trovato: ${tag}`,
		);
		assert.ok(
			!/\bclass="[^"]*\bmodal-root\b/.test(tag),
			`#modal-root porta la classe modal-root, che è quella dei veri dialoghi con fondale. Tag trovato: ${tag}`,
		);
	});

	it("aprire il dettaglio spec non rende inerte niente", () => {
		// `inert` esiste in app.js, ma appartiene alle vere modali con fondale:
		// enterModal lo mette sullo sfondo e leaveModal lo toglie, perché
		// aria-modal è una promessa che va mantenuta. Il dettaglio spec non è
		// una modale, quindi nessuna delle funzioni che lo aprono, lo chiudono o
		// lo dispongono deve passare di lì — altrimenti tornerebbe a bloccare il
		// resto della pagina, conversazione compresa.
		const strip = (source) =>
			source.replace(/\/\/[^\n]*/g, "").replace(/\/\*[\s\S]*?\*\//g, "");
		for (const marker of [
			"async function openEditor(",
			"function closeModal(",
			"function applyShellLayout(",
		]) {
			const body = strip(sectionOf(js, marker));
			assert.ok(
				!/\binert\b/.test(body) && !/enterModal\(/.test(body),
				`\`${marker}\` rende inerte lo sfondo: il dettaglio spec tornerebbe a essere una finestra`,
			);
		}
		const code = strip(js);
		assert.ok(
			!/enterModal\(\s*modal[,)]/.test(code),
			"il dettaglio spec (#modal-root) viene passato al trap del fuoco delle modali: non è una modale",
		);
	});

	it("le tre schede del dettaglio sono tutte ancora presenti", () => {
		for (const tab of ["story", "plan", "review"]) {
			assert.ok(
				html.includes(`data-tab="${tab}"`),
				`la scheda data-tab="${tab}" è sparita dal dettaglio spec: il contenuto del dettaglio non doveva cambiare`,
			);
		}
	});
});

// AC-5 di US-057 chiedeva che i contatori restassero leggibili dalla schermata
// iniziale, e li teneva nella barra superiore. AC-6 di US-061 la supera: i
// contatori restano leggibili, ma dalla board. Le due garanzie del blocco
// originale sono riportate qui sulla nuova posizione — che i tre numeri siano
// nel documento, e che nessuna regola li nasconda — mentre la terza, che boot()
// legga la board anche quando non è la vista visibile, resta invariata: è
// ancora ciò che garantisce che i numeri siano pronti quando la board viene
// aperta.
describe("AC-5 (US-057, superata da US-061 AC-6) — i contatori restano leggibili, dalla board", () => {
	it("sono emessi dalla board, non più dalla barra superiore", () => {
		const body = sectionOf(js, "function boardStatsHeader(");
		for (const id of ["stat-total", "stat-progress", "stat-done"]) {
			assert.ok(
				body.includes(id),
				`la board non emette più #${id}: quel contatore del backlog non sarebbe più leggibile da nessuna parte`,
			);
			assert.ok(
				!html.includes(`id="${id}"`),
				`index.html contiene ancora #${id}: i contatori sono tornati nella barra superiore, che è ciò che US-061 AC-6 toglie`,
			);
		}
	});

	it("nessuna regola di app.css li nasconde", () => {
		const hiding = cssRules(css).filter(
			(rule) =>
				/(^|[\s,>])\.board-stats\s*(,|$)/.test(rule.selector) &&
				/display\s*:\s*none/.test(rule.body),
		);
		assert.deepEqual(
			hiding.map((r) => r.selector),
			[],
			"una regola di app.css applica display:none a .board-stats: i contatori sparirebbero a una certa larghezza, e restare leggibili è ciò che il criterio chiede",
		);
	});

	it("boot() legge la board anche quando la board non è la vista visibile", () => {
		const body = sectionOf(js, "async function boot(");
		const open = body.indexOf("view.open");
		const load = body.indexOf("loadBoard()");
		assert.notEqual(
			load,
			-1,
			"boot() non legge più la board: i contatori resterebbero vuoti finché non si apre la board",
		);
		assert.ok(
			open !== -1 && load > open,
			"loadBoard() non è più sul ramo del workspace aperto: i contatori sarebbero alimentati solo aprendo la board",
		);
	});
});

describe("AC-6 — lo stato stretto è stilato", () => {
	it("la media query a 900px definisce la sovrapposizione", () => {
		const query = css.match(/@media\s*\(max-width:\s*900px\)\s*\{[\s\S]*?\n\}/g);
		assert.ok(
			query && query.length > 0,
			"app.css non contiene più una media query a 900px",
		);
		const narrow = query.join("\n");
		assert.ok(
			narrow.includes("is-overlay"),
			"la classe is-overlay non è stilata nella media query a 900px: in finestra stretta la vista chiamata non sarebbe disegnata sopra la conversazione",
		);
		assert.ok(
			narrow.includes("workspace-shell--narrow"),
			"la media query a 900px non stila workspace-shell--narrow, che è la classe che il modulo restituisce per lo stato stretto",
		);
	});

	it("la conversazione ha la sua classe di riquadro nel foglio di stile", () => {
		assert.ok(
			css.includes("shell-pane-conversation"),
			"app.css non stila shell-pane-conversation: la casa del workspace non avrebbe forma nella colonna principale",
		);
	});
});
