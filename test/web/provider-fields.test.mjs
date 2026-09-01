// test/web/provider-fields.test.mjs
// Tests for the pure provider-fields renderer used by the ARchetipo web viewer.
// Run: node --test test/web/provider-fields.test.mjs
//
// Same discipline as workspace-status.test.mjs: the oracles are on the
// *visible text* of the rendered HTML, not on the shape of the module. What
// the person configuring the execution actually reads in the model field is
// what the acceptance criteria are about. The only exception is which entry is
// selected — there the selection itself is the criterion, so the attribute is
// the oracle.
//
// Verifies:
//   - AC-1 every model the provider declares is visible in the field
//   - AC-2 exactly one entry reads as the provider default
//   - AC-4 with no catalog the reason is visible and the field stays typable
//   - AC-5 a value outside the catalog stays selected and is marked
//   - AC-6 with no configured model the empty entry is selected and submits ""

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
	"provider-fields.js",
);

// Same minimal virtual-machine loader as workspace-status.test.mjs: the UMD
// module detects `module.exports` first, so the Node branch is enough.
function loadProviderFields() {
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

const { renderProviderFields } = loadProviderFields();

// Strip every attribute from the markup, leaving only what a reader sees.
// An identifier that survives this is visible text; one that does not was
// only ever hidden in an attribute.
function visibleText(html) {
	return html.replace(/\s\w[\w-]*="[^"]*"/g, "");
}

// The entries of the rendered list, as {value, text, selected}. Used only for
// the criteria that are about the selection itself.
function options(html) {
	const out = [];
	const re = /<option\s*([^>]*)>([\s\S]*?)<\/option>/g;
	let m;
	while ((m = re.exec(html)) !== null) {
		const attrs = m[1];
		const value = /value="([^"]*)"/.exec(attrs);
		out.push({
			value: value ? value[1] : "",
			text: m[2],
			selected: /\bselected\b/.test(attrs),
		});
	}
	return out;
}

// What the form would submit for the field: the value of the selected entry.
function submittedValue(html) {
	const selected = options(html).filter((o) => o.selected);
	assert.equal(selected.length, 1, "esattamente una voce deve essere selezionata");
	return selected[0].value;
}

const CATALOG_VIEW = {
	id: "provider-inventato",
	model_field: "model",
	config_fields: [{ name: "model", label: "Modello" }],
	models: [
		{ id: "modello-alfa", label: "Modello Alfa" },
		{ id: "modello-beta", label: "Modello Beta", default: true },
		{ id: "modello-gamma" },
	],
};

describe("renderProviderFields — campo del catalogo", () => {
	it("mostra ogni modello dichiarato dal provider", () => {
		const text = visibleText(renderProviderFields(CATALOG_VIEW, {}));

		// Every declared identifier is the value the form would submit for its
		// entry, so no model of the catalog can be missing from the field.
		for (const id of ["modello-alfa", "modello-beta", "modello-gamma"]) {
			assert.ok(
				options(renderProviderFields(CATALOG_VIEW, {})).some((o) => o.value === id),
				`identificatore ${id} assente dal campo reso`,
			);
		}
		// The label wins when the provider declares one, the identifier is the
		// fallback: either way the reader sees each declared model.
		assert.ok(text.includes("Modello Alfa"), "l'etichetta del primo modello non è visibile");
		assert.ok(text.includes("Modello Beta"), "l'etichetta del secondo modello non è visibile");
		assert.ok(text.includes("modello-gamma"), "il modello senza etichetta non è visibile");
		assert.equal(
			options(renderProviderFields(CATALOG_VIEW, {})).length,
			4,
			"la voce vuota più i tre modelli dichiarati",
		);
	});

	it("indica un solo modello come predefinito del provider", () => {
		const html = renderProviderFields(CATALOG_VIEW, {});
		const text = visibleText(html);

		const marked = options(html).filter((o) => /default del provider/i.test(o.text));
		assert.equal(
			marked.length,
			1,
			"esattamente una voce deve essere indicata come predefinito del provider",
		);
		assert.ok(
			marked[0].text.includes("Modello Beta"),
			"la dicitura di predefinito è accanto al modello sbagliato",
		);
		assert.ok(text.includes("default del provider"), "la dicitura di predefinito non è testo visibile");
	});

	it("senza valore configurato seleziona la voce vuota", () => {
		const html = renderProviderFields(CATALOG_VIEW, {});

		assert.equal(submittedValue(html), "", "il form invierebbe un modello non vuoto");
		const selected = options(html).find((o) => o.selected);
		assert.ok(
			!["modello-alfa", "modello-beta", "modello-gamma"].includes(selected.value),
			"nessun identificatore di modello deve essere selezionato",
		);
	});

	it("un valore in catalogo è la voce selezionata", () => {
		const html = renderProviderFields(CATALOG_VIEW, { model: "modello-gamma" });

		assert.equal(submittedValue(html), "modello-gamma");
		assert.ok(
			!/non in elenco/i.test(visibleText(html)),
			"un modello del catalogo non deve essere marcato come non elencato",
		);
		assert.equal(options(html).length, 4, "nessuna voce aggiuntiva per un valore in catalogo");
	});

	it("un valore fuori catalogo resta selezionato e marcato come non elencato", () => {
		const html = renderProviderFields(CATALOG_VIEW, { model: "modello-fuori-catalogo" });
		const text = visibleText(html);

		assert.equal(
			submittedValue(html),
			"modello-fuori-catalogo",
			"il valore fuori catalogo non è più quello selezionato: il salvataggio lo cancellerebbe",
		);
		assert.ok(
			text.includes("modello-fuori-catalogo"),
			"il valore fuori catalogo non è testo visibile",
		);
		const unlisted = options(html).filter((o) => /non in elenco/i.test(o.text));
		assert.equal(unlisted.length, 1, "una sola voce deve essere marcata come non elencata");
		assert.equal(unlisted[0].value, "modello-fuori-catalogo");
		for (const id of ["modello-alfa", "modello-beta", "modello-gamma"]) {
			assert.ok(
				options(html).some((o) => o.value === id),
				`la voce di catalogo ${id} è sparita`,
			);
		}
	});

	it("senza catalogo dice perché e lascia scrivere il modello a mano", () => {
		const html = renderProviderFields(
			{
				...CATALOG_VIEW,
				models: [],
				models_unavailable_reason: "MOTIVO-CATALOGO-ASSENTE",
			},
			{ model: "modello-scritto-a-mano" },
		);
		const text = visibleText(html);

		assert.ok(text.includes("MOTIVO-CATALOGO-ASSENTE"), "il motivo non è testo visibile");
		assert.equal(options(html).length, 0, "senza catalogo non deve essere resa alcuna voce");
		assert.ok(
			/<input[^>]*type="text"[^>]*value="modello-scritto-a-mano"/.test(html),
			"il campo deve restare una casella di testo con il valore corrente invariato",
		);
	});
});

describe("renderProviderFields — resa invariata fuori dal catalogo", () => {
	it("un provider senza catalogo è reso come prima", () => {
		const html = renderProviderFields(
			{
				id: "provider-senza-catalogo",
				config_fields: [{ name: "model", label: "Modello" }],
			},
			{ model: "modello-corrente" },
		);

		assert.ok(
			/<input[^>]*name="provider_model"[^>]*value="modello-corrente"/.test(html),
			"il campo model deve restare una casella di testo",
		);
		assert.equal(options(html).length, 0, "nessuna voce di elenco senza catalogo dichiarato");
		const text = visibleText(html);
		assert.ok(!/default del provider/i.test(text), "nessuna dicitura di predefinito senza catalogo");
		assert.ok(!/non in elenco/i.test(text), "nessuna dicitura di voce non elencata senza catalogo");
	});

	it("gli altri campi non sono toccati", () => {
		const html = renderProviderFields(
			{
				id: "provider-inventato",
				model_field: "model",
				config_fields: [
					{ name: "command", label: "Comando", required: true, help: "AIUTO-COMANDO" },
					{ name: "model", label: "Modello", help: "AIUTO-MODELLO" },
					{ name: "permission_mode", label: "Modo permessi", help: "AIUTO-PERMESSI" },
					{
						name: "timeout_seconds",
						label: "Timeout",
						type: "integer",
						help: "AIUTO-TIMEOUT",
					},
				],
				models: [{ id: "modello-alfa", default: true }],
			},
			{ command: "comando-corrente", timeout_seconds: 42 },
		);
		const text = visibleText(html);

		assert.equal(
			(html.match(/data-provider-field="/g) || []).length,
			4,
			"devono essere resi quattro campi",
		);
		for (const label of ["Comando", "Modello", "Modo permessi", "Timeout"]) {
			assert.ok(text.includes(label), `l'etichetta ${label} non è testo visibile`);
		}
		for (const help of ["AIUTO-COMANDO", "AIUTO-MODELLO", "AIUTO-PERMESSI", "AIUTO-TIMEOUT"]) {
			assert.ok(text.includes(help), `il testo d'aiuto ${help} non è testo visibile`);
		}
		assert.ok(
			/<input[^>]*type="number"[^>]*name="provider_timeout_seconds"/.test(html),
			"il campo intero deve restare un input numerico",
		);
		assert.ok(
			/<input[^>]*name="provider_command"[^>]*value="comando-corrente"/.test(html),
			"il valore corrente di un campo non del catalogo deve restare invariato",
		);
		// Only the catalog field became a list.
		assert.equal((html.match(/<select/g) || []).length, 1);
	});
});

describe("renderProviderFields — robustezza", () => {
	it("neutralizza l'HTML che arriva dal payload", () => {
		const html = renderProviderFields(
			{
				id: "p",
				model_field: "model",
				config_fields: [{ name: "model", label: '<script>alert("l")</script>' }],
				models: [
					{ id: '<script>alert("id")</script>', label: '"><script>alert(1)</script>' },
				],
			},
			{ model: '"><script>alert("valore")</script>' },
		);

		assert.ok(!html.includes("<script"), "il payload ha prodotto marcatura eseguibile");
		assert.ok(html.includes("&lt;script"), "il markup del payload non è stato neutralizzato");
		assert.ok(
			visibleText(html).includes("alert("),
			"il testo del payload deve comunque comparire come testo",
		);

		const reason = renderProviderFields(
			{
				id: "p",
				model_field: "model",
				config_fields: [{ name: "model", label: "Modello" }],
				models: [],
				models_unavailable_reason: '<img src=x onerror="1">',
			},
			{},
		);
		assert.ok(!reason.includes("<img"), "il motivo ha prodotto un tag reale");
		assert.ok(reason.includes("&lt;img"), "il motivo non è stato neutralizzato");
	});

	it("non lancia su payload parziali", () => {
		const views = [
			null,
			undefined,
			{},
			{ config_fields: [] },
			{ config_fields: [{ name: "model" }], model_field: "model", models: null },
			{ config_fields: [{ name: "model" }], model_field: "model", models: "non-una-lista" },
			{ config_fields: [{ name: "model" }], model_field: "model", models: [null] },
		];
		for (const view of views) {
			for (const values of [undefined, null, {}, { model: null }]) {
				const html = renderProviderFields(view, values);
				assert.equal(typeof html, "string");
			}
		}
	});
});

// --- US-048: opzioni del modello selezionato --------------------------------
//
// Verifica:
//   - AC-1 solo le opzioni del modello scelto, con il predefinito indicato
//   - AC-2 un'opzione già salvata è riproposta selezionata
//   - AC-3 cambiando modello i controlli dell'opzione superata spariscono
//   - AC-5 un modello senza opzioni produce una frase esplicita, non una
//     sezione vuota

const OPTION_VIEW = {
	id: "provider-inventato",
	model_field: "model",
	config_fields: [{ name: "model", label: "Modello" }],
	models: [
		{
			id: "modello-alfa",
			label: "Modello Alfa",
			options: [
				{
					name: "sforzo",
					label: "Sforzo",
					help: "Lasciata vuota, decide il provider.",
					choices: [
						{ value: "basso", label: "basso" },
						{ value: "medio", label: "medio", default: true },
						{ value: "alto", label: "alto" },
					],
				},
			],
		},
		{ id: "modello-beta", label: "Modello Beta" },
	],
};

// The controls of one named option, as {value, text, selected}.
function optionControl(html, name) {
	const re = new RegExp(
		`<select name="provider_option_${name}"[^>]*>([\\s\\S]*?)</select>`,
	);
	const m = re.exec(html);
	return m ? options(m[1]) : null;
}

describe("renderProviderFields — opzioni del modello selezionato", () => {
	it("mostra soltanto le opzioni dichiarate dal modello scelto", () => {
		const html = renderProviderFields(OPTION_VIEW, { model: "modello-alfa" });
		const entries = optionControl(html, "sforzo");

		assert.ok(entries, "il controllo dell'opzione dichiarata non è stato disegnato");
		assert.deepEqual(
			entries.map((o) => o.value),
			["", "basso", "medio", "alto"],
			"la voce vuota più le tre scelte dichiarate, in quell'ordine",
		);
		const text = visibleText(html);
		assert.ok(text.includes("Sforzo"), "l'etichetta dell'opzione non è visibile");
		assert.ok(
			text.includes("Lasciata vuota, decide il provider."),
			"l'aiuto dell'opzione non è visibile",
		);
		// Exactly one entry reads as the provider default, and it is the one
		// the provider marked.
		const marked = entries.filter((o) => /default del provider/i.test(o.text));
		assert.equal(marked.length, 1, "una sola scelta deve leggersi come predefinita");
		assert.equal(marked[0].value, "medio", "il marcatore è finito sulla scelta sbagliata");
	});

	it("lascia la voce vuota selezionata quando l'opzione non è impostata", () => {
		const entries = optionControl(
			renderProviderFields(OPTION_VIEW, { model: "modello-alfa" }),
			"sforzo",
		);
		const selected = entries.filter((o) => o.selected);
		assert.equal(selected.length, 1, "esattamente una voce deve essere selezionata");
		assert.equal(selected[0].value, "", "un'opzione non impostata deve inviare la stringa vuota");
	});

	it("ripropone selezionata l'opzione già salvata", () => {
		const entries = optionControl(
			renderProviderFields(OPTION_VIEW, { model: "modello-alfa", sforzo: "alto" }),
			"sforzo",
		);
		const selected = entries.filter((o) => o.selected);
		assert.equal(selected.length, 1, "esattamente una voce deve essere selezionata");
		assert.equal(selected[0].value, "alto", "il valore salvato non è stato riproposto");
	});

	it("non disegna l'opzione di un modello che non è più quello scelto", () => {
		const html = renderProviderFields(OPTION_VIEW, {
			model: "modello-beta",
			sforzo: "alto",
		});
		assert.equal(
			optionControl(html, "sforzo"),
			null,
			"l'opzione del modello precedente è ancora disegnata",
		);
		assert.ok(
			!html.includes("provider_option_"),
			"il markup contiene ancora un controllo di opzione",
		);
	});

	it("mostra una frase esplicita per un modello che non dichiara opzioni", () => {
		const html = renderProviderFields(OPTION_VIEW, { model: "modello-beta" });
		const text = visibleText(html);
		assert.ok(
			text.includes("Questo modello non dichiara nessuna opzione."),
			"la frase esplicita non è visibile",
		);
		assert.ok(
			!html.includes("provider_option_"),
			"un contenitore di opzioni vuoto è stato disegnato",
		);
	});

	it("non dice nulla quando nessun modello è scelto o il catalogo non c'è", () => {
		const silent = [
			[OPTION_VIEW, {}],
			[OPTION_VIEW, { model: "modello-fuori-catalogo" }],
			[
				{ id: "p", config_fields: [{ name: "model", label: "Modello" }] },
				{ model: "qualunque" },
			],
			[
				{
					id: "p",
					model_field: "model",
					config_fields: [{ name: "model", label: "Modello" }],
					models: null,
				},
				{ model: "qualunque" },
			],
		];
		for (const [view, values] of silent) {
			const html = renderProviderFields(view, values);
			assert.ok(
				!html.includes("provider_option_"),
				"è stato disegnato un controllo di opzione senza modello di catalogo scelto",
			);
			assert.ok(
				!html.includes("Questo modello non dichiara nessuna opzione."),
				"è stata mostrata la frase del modello senza opzioni fuori dal suo caso",
			);
		}
	});

	it("marca ogni controllo di opzione con il proprio nome", () => {
		// Il nome non si legge dal prefisso del campo inviato: un campo di
		// configurazione chiamato `option_x` sarebbe indistinguibile
		// dall'opzione `x`.
		const html = renderProviderFields(OPTION_VIEW, { model: "modello-alfa" });
		assert.ok(
			html.includes('data-provider-option="sforzo"'),
			"il controllo dell'opzione non porta il proprio nome in un attributo dedicato",
		);
	});

	it("non lancia su opzioni malformate", () => {
		const broken = [
			{ options: null },
			{ options: "non-una-lista" },
			{ options: [null] },
			{ options: [{}] },
			{ options: [{ name: "x", choices: null }] },
			{ options: [{ name: "x", choices: [null] }] },
		];
		for (const extra of broken) {
			const view = {
				id: "p",
				model_field: "model",
				config_fields: [{ name: "model", label: "Modello" }],
				models: [{ id: "m", ...extra }],
			};
			const html = renderProviderFields(view, { model: "m" });
			assert.equal(typeof html, "string");
		}
	});

	it("neutralizza il testo dichiarato per un'opzione", () => {
		const view = {
			id: "p",
			model_field: "model",
			config_fields: [{ name: "model", label: "Modello" }],
			models: [
				{
					id: "m",
					options: [
						{
							name: "x",
							label: '<img src=x onerror="1">',
							choices: [{ value: '<script>alert(1)</script>' }],
						},
					],
				},
			],
		};
		const html = renderProviderFields(view, { model: "m" });
		assert.ok(!html.includes("<img"), "l'etichetta ha prodotto un tag reale");
		assert.ok(!html.includes("<script>"), "la scelta ha prodotto un tag reale");
		assert.ok(html.includes("&lt;img"), "l'etichetta non è stata neutralizzata");
	});
});

// ---------------------------------------------------------------------------
// renderModelChoice — la scelta di modello per la singola run.
//
// Stessa disciplina delle suite precedenti: gli oracoli stanno sul testo
// visibile e, dove il criterio è la selezione stessa, sull'attributo
// `selected`. Il modulo resta ignaro di ogni provider: i modelli qui sotto
// sono inventati apposta.
//
// Verifica:
//   - AC-1 il modello ereditato dal workspace è quello scritto sulla pastiglia,
//     e l'ereditarietà è dichiarata a parole
//   - AC-2 le opzioni mostrate sono quelle del modello selezionato
//   - AC-6 senza catalogo il motivo è visibile e nessuna pastiglia compare
// ---------------------------------------------------------------------------

const { renderModelChoice, renderConversationModelChoice } = loadProviderFields();

const CHOICE_VIEW = {
	available: true,
	model: "modello-uno",
	model_source: "workspace",
	options: {},
	models: [
		{
			id: "modello-uno",
			label: "Modello Uno",
			options: [
				{
					name: "sforzo",
					label: "Sforzo",
					choices: [
						{ value: "a", label: "Scelta A" },
						{ value: "b", label: "Scelta B" },
					],
				},
			],
		},
		{ id: "modello-due", label: "Modello Due", options: [] },
	],
};

describe("renderModelChoice — la riga agente della singola run", () => {
	it("mostra il modello ereditato dal workspace sulla pastiglia", () => {
		const html = renderModelChoice(CHOICE_VIEW, null);
		const row = runRow(html);

		assert.equal(row.length, 2, "una pastiglia per il modello e una per la sua opzione");
		assert.equal(row[0].key, "model", "la prima pastiglia non è quella del modello");
		assert.equal(
			row[0].value,
			"Modello Uno",
			"la pastiglia non porta scritto il modello ereditato",
		);
		assert.ok(
			!row[0].chosen,
			"il modello ereditato è segnato come una scelta staccata da lui",
		);
		assert.ok(
			/ereditato dal workspace/i.test(row[0].title + row[0].foot),
			"l'ereditarietà dal workspace non è dichiarata a parole",
		);
		assert.ok(
			/si fissa quando la run parte/.test(row[0].title + row[0].foot),
			"la riga della run parla della conversazione invece che della run",
		);
	});

	it("offre le opzioni del modello selezionato e non quelle degli altri", () => {
		const withOptions = runRow(
			renderModelChoice(CHOICE_VIEW, { model: "modello-uno" }),
		);
		const option = withOptions.find((cell) => cell.key === "option:sforzo");
		assert.ok(option, "l'opzione del modello scelto non è stata disegnata");
		assert.deepEqual(
			option.segments.map((seg) => seg.value),
			["", "a", "b"],
			"il segmento vuoto più le due scelte dichiarate, in quell'ordine",
		);
		assert.ok(
			option.segments.every((seg) => seg.option === "sforzo"),
			"i segmenti non portano il nome della propria opzione",
		);

		const other = renderModelChoice(CHOICE_VIEW, { model: "modello-due" });
		assert.equal(
			runRow(other).length,
			1,
			"un modello senza opzioni disegna comunque una pastiglia di opzione",
		);
		assert.ok(
			!other.includes('data-run-option="'),
			"un modello senza opzioni disegna comunque un controllo di opzione",
		);
		assert.ok(
			!visibleText(other).includes("Questo modello non dichiara nessuna opzione."),
			"la riga spiega un vuoto invece di non disegnarlo",
		);
	});

	it("riporta il valore di opzione già scelto, sulla pastiglia e nel segmento", () => {
		const row = runRow(
			renderModelChoice(CHOICE_VIEW, {
				model: "modello-uno",
				options: { sforzo: "b" },
			}),
		);
		const option = row.find((cell) => cell.key === "option:sforzo");
		assert.equal(option.value, "Scelta B", "la pastiglia non porta il valore scelto");
		const pressed = option.segments.filter((seg) => seg.pressed);
		assert.equal(pressed.length, 1, "esattamente un segmento deve essere premuto");
		assert.equal(pressed[0].value, "b", "la scelta già fatta non è il segmento premuto");
	});

	it("disegna la riga di separazione solo nell'elenco dei modelli", () => {
		const row = runRow(
			renderModelChoice(CHOICE_VIEW, { model: "modello-uno" }),
		);
		assert.ok(
			row[0].rule,
			"l'elenco dei modelli non è separato dalla sua nota",
		);
		assert.ok(
			!row.find((cell) => cell.key === "option:sforzo").rule,
			"i segmenti di un'opzione sono separati dal loro stesso aiuto",
		);
	});

	it("apre il popover che `open` nomina, e uno solo", () => {
		const row = runRow(
			renderModelChoice(CHOICE_VIEW, { model: "modello-uno" }, "option:sforzo"),
		);
		assert.deepEqual(
			row.filter((cell) => cell.open).map((cell) => cell.key),
			["option:sforzo"],
			"l'apertura non segue la chiave passata dal chiamante",
		);
	});

	it("dichiara la scelta non disponibile con il motivo, senza pastiglie", () => {
		const html = renderModelChoice(
			{
				available: false,
				model: "modello-uno",
				model_source: "workspace",
				unavailable_reason: "il catalogo non è ottenibile",
			},
			null,
		);

		assert.ok(
			html.includes("il catalogo non è ottenibile"),
			"il motivo dell'indisponibilità non è raggiungibile",
		);
		assert.ok(
			visibleText(html).includes("modello-uno"),
			"il modello effettivo non è dichiarato",
		);
		assert.ok(!html.includes("<select"), "la scelta non disponibile disegna un selettore");
		assert.equal(
			runRow(html).length,
			0,
			"la scelta non disponibile offre pastiglie da premere invano",
		);
	});

	it("neutralizza l'HTML che arriva dal payload", () => {
		const unavailable = renderModelChoice(
			{
				available: false,
				model: '<script>alert(1)</script>',
				unavailable_reason: '<img src=x onerror="1">',
			},
			null,
		);
		assert.ok(!unavailable.includes("<script"), "il modello ha prodotto un tag reale");
		assert.ok(!unavailable.includes("<img"), "il motivo ha prodotto un tag reale");
		assert.ok(unavailable.includes("&lt;img"), "il motivo non è stato neutralizzato");

		const available = renderModelChoice(
			{
				available: true,
				model: "m",
				models: [
					{
						id: "m",
						label: '<script>alert(1)</script>',
						options: [
							{
								name: "x",
								label: '<img src=x onerror="1">',
								choices: [{ value: '<script>alert(2)</script>' }],
							},
						],
					},
				],
			},
			null,
		);
		assert.ok(!available.includes("<script"), "un'etichetta ha prodotto un tag reale");
		assert.ok(!available.includes("<img"), "l'etichetta dell'opzione ha prodotto un tag reale");
	});

	it("non lancia su payload parziali", () => {
		const partials = [
			null,
			undefined,
			{},
			{ available: true },
			{ available: true, model: "sconosciuto" },
			{ available: true, models: "non-una-lista" },
			{ available: true, models: [null] },
			{ available: true, model: "m", models: [{ id: "m", options: null }] },
			{ available: false },
		];
		for (const view of partials) {
			const html = renderModelChoice(view, null, "model");
			assert.equal(typeof html, "string", "il renderer non ha restituito una stringa");
		}
	});

	// Una voce vuota offerta sopra un modello ereditato sarebbe una scelta che
	// il server non può distinguere da «nessuna scelta»: il pannello mostrerebbe
	// «il provider sceglie» e la run partirebbe con il modello configurato.
	// L'unico caso in cui la voce vuota dice il vero è quando è già lei quella
	// in vigore.
	it("non offre la voce vuota sopra un modello ereditato", () => {
		const row = runRow(renderModelChoice(CHOICE_VIEW, null));

		assert.ok(
			!row[0].entries.some((entry) => entry.value === ""),
			"la voce vuota è offerta anche se un modello è già in vigore",
		);
	});

	it("offre la voce vuota quando è il modello ereditato a essere vuoto", () => {
		const row = runRow(renderModelChoice({ ...CHOICE_VIEW, model: "" }, null));
		const empty = row[0].entries.filter((entry) => entry.value === "");

		assert.equal(empty.length, 1, "la voce vuota deve essere offerta una sola volta");
		assert.ok(empty[0].checked, "la voce vuota in vigore non è quella spuntata");
	});

	it("i due ambiti disegnano la stessa riga con marcatori distinti", () => {
		const selection = { model: "modello-uno", options: { sforzo: "b" } };
		const run = renderModelChoice(CHOICE_VIEW, selection);
		const conversation = renderConversationModelChoice(CHOICE_VIEW, selection);

		assert.deepEqual(
			runRow(run).map((cell) => [cell.key, cell.value]),
			agentRow(conversation).map((cell) => [cell.key, cell.value]),
			"le due righe non offrono le stesse pastiglie con gli stessi valori",
		);
		assert.ok(
			!run.includes("data-conversation-"),
			"la riga della run porta i marcatori della conversazione",
		);
		assert.ok(
			!conversation.includes("data-run-"),
			"la riga della conversazione porta i marcatori della run",
		);
	});
});

// ---------------------------------------------------------------------------
// La riga agente: la stessa scelta della run, resa come una riga di pastiglie.
//
// Gli oracoli restano sul testo visibile — che cosa il lettore legge sulla riga
// e dentro il popover — tranne dove il criterio è la selezione stessa
// (`aria-checked`, `aria-pressed`) o l'apertura (`hidden`): lì l'attributo è il
// criterio.
// ---------------------------------------------------------------------------

function firstAttribute(chunk, name) {
	const m = new RegExp(`${name}="([^"]*)"`).exec(chunk);
	return m ? m[1] : "";
}

function between(chunk, opening, closing) {
	const at = chunk.indexOf(opening);
	if (at === -1) return "";
	const from = at + opening.length;
	const to = chunk.indexOf(closing, from);
	return to === -1 ? "" : chunk.slice(from, to);
}

// Le voci dell'elenco dei modelli dentro un popover.
function modelEntries(chunk, scope = "conversation") {
	const out = [];
	const re = new RegExp(
		`<button [^>]*data-${scope}-model-choice="([^"]*)"[^>]*>([\\s\\S]*?)</button>`,
		"g",
	);
	let m;
	while ((m = re.exec(chunk)) !== null) {
		const open = m[0].slice(0, m[0].indexOf(">") + 1);
		out.push({
			value: m[1],
			text: between(m[2], '<span class="conv-pop-entry-text">', "</span>"),
			tag: between(m[2], '<span class="conv-pop-tag">', "</span>"),
			checked: /aria-checked="true"/.test(open),
		});
	}
	return out;
}

// I segmenti di un'opzione dentro un popover.
function optionSegments(chunk, scope = "conversation") {
	const out = [];
	const re = new RegExp(
		`<button [^>]*data-${scope}-option="([^"]*)" data-${scope}-option-choice="([^"]*)"[^>]*>([\\s\\S]*?)</button>`,
		"g",
	);
	let m;
	while ((m = re.exec(chunk)) !== null) {
		const open = m[0].slice(0, m[0].indexOf(">") + 1);
		out.push({
			option: m[1],
			value: m[2],
			text: m[3],
			pressed: /aria-pressed="true"/.test(open),
		});
	}
	return out;
}

// La riga come una lista di pastiglie, ognuna con il suo popover. Lo stesso
// helper legge le due righe che il modulo disegna — quella della conversazione e
// quella della run — perché è lo stesso markup sotto un altro prefisso.
function agentRow(html, scope = "conversation") {
	return html
		.split('<span class="conv-pill-shell">')
		.slice(1)
		.map((chunk) => ({
			key: firstAttribute(chunk, `data-${scope}-pill`),
			value: between(chunk, '<span class="conv-pill-value">', "</span>"),
			title: firstAttribute(chunk, "title"),
			chosen: /class="conv-pill is-chosen"/.test(chunk),
			open: !new RegExp(`data-${scope}-pop="[^"]*" hidden>`).test(chunk),
			foot: between(chunk, '<div class="conv-pop-foot">', "</div>"),
			rule: chunk.includes('<div class="conv-pop-rule"></div>'),
			entries: modelEntries(chunk, scope),
			segments: optionSegments(chunk, scope),
		}));
}

// La riga della run, letta dallo stesso helper con l'altro prefisso.
function runRow(html) {
	return agentRow(html, "run");
}

describe("renderConversationModelChoice — la riga agente", () => {
	it("disegna una pastiglia per il modello e una per la sua prima opzione", () => {
		const row = agentRow(
			renderConversationModelChoice(CHOICE_VIEW, {
				model: "modello-uno",
				options: { sforzo: "b" },
			}),
		);

		assert.deepEqual(
			row.map((pill) => pill.key),
			["model", "option:sforzo"],
			"la riga non è modello più prima opzione",
		);
		assert.equal(row[0].value, "Modello Uno", "la pastiglia non dice il modello");
		assert.equal(
			row[1].value,
			"Scelta B",
			"la pastiglia dell'opzione non dice il valore in vigore",
		);
	});

	it("non produce nessun controllo degli altri due ambiti", () => {
		const html = renderConversationModelChoice(CHOICE_VIEW, null);

		assert.ok(!html.includes("<select"), "la riga ha disegnato un select");
		assert.ok(!html.includes("data-run-model"), "la riga porta i marcatori della run");
	});

	it("mostra il modello del workspace finché non viene cambiato", () => {
		const row = agentRow(renderConversationModelChoice(CHOICE_VIEW, null));

		assert.equal(row[0].value, "Modello Uno");
		assert.ok(!row[0].chosen, "una scelta ereditata è segnata come scelta propria");
		assert.ok(
			/ereditato dal workspace/i.test(row[0].title),
			"l'ereditarietà dal workspace non è dichiarata nel title",
		);
		assert.ok(
			/ereditato dal workspace/i.test(row[0].foot),
			"l'ereditarietà dal workspace non è dichiarata nel piede del popover",
		);
	});

	// L'unico segno di colore della riga, e il criterio è la selezione stessa.
	it("segna la pastiglia quando la scelta si stacca da quella ereditata", () => {
		const row = agentRow(
			renderConversationModelChoice(CHOICE_VIEW, {
				model: "modello-due",
				options: {},
			}),
		);

		assert.ok(row[0].chosen, "una scelta diversa dall'ereditata non è segnata");
	});

	it("porta il catalogo nel popover, con la targhetta del default", () => {
		const row = agentRow(
			renderConversationModelChoice(
				{
					...CHOICE_VIEW,
					models: [
						{ id: "modello-uno", label: "Modello Uno", options: [] },
						{ id: "modello-due", label: "Modello Due", default: true, options: [] },
					],
				},
				null,
			),
		);
		const entries = row[0].entries;

		assert.deepEqual(
			entries.map((entry) => entry.value),
			["modello-uno", "modello-due"],
			"l'elenco non è il catalogo dichiarato",
		);
		assert.deepEqual(
			entries.map((entry) => entry.tag),
			["", "default"],
			"esattamente una voce deve portare la targhetta del default",
		);
		assert.equal(
			entries.filter((entry) => entry.checked).length,
			1,
			"esattamente una voce deve essere quella in vigore",
		);
		assert.ok(
			entries.every((entry) => !/default del provider/.test(entry.text)),
			"il nome del modello porta ancora il vecchio suffisso",
		);
	});

	it("tiene un valore fuori catalogo e lo marca", () => {
		const row = agentRow(
			renderConversationModelChoice(
				{ ...CHOICE_VIEW, model: "modello-mai-visto" },
				null,
			),
		);
		const unlisted = row[0].entries.filter(
			(entry) => entry.value === "modello-mai-visto",
		);

		assert.equal(unlisted.length, 1, "il valore fuori catalogo è stato perso");
		assert.equal(unlisted[0].tag, "non in elenco");
		assert.ok(unlisted[0].checked, "il valore in vigore non è quello segnato");
	});

	// Stessa regola della run, e per la stessa ragione: una voce vuota offerta
	// sopra un modello ereditato sarebbe una scelta che il server non distingue
	// da «nessuna scelta».
	it("offre la voce vuota solo quando il modello ereditato è vuoto", () => {
		const inherited = agentRow(renderConversationModelChoice(CHOICE_VIEW, null));
		assert.ok(
			!inherited[0].entries.some((entry) => entry.value === ""),
			"la voce vuota è offerta sopra un modello ereditato",
		);

		const row = agentRow(
			renderConversationModelChoice({ ...CHOICE_VIEW, model: "" }, null),
		);
		const entries = row[0].entries;
		assert.equal(
			entries[entries.length - 1].value,
			"",
			"la voce vuota non è in coda all'elenco",
		);
		assert.ok(entries[entries.length - 1].checked);
		assert.equal(row[0].value, "Sceglie il provider");
	});

	it("offre il segmento vuoto in testa alle scelte di un'opzione", () => {
		const row = agentRow(
			renderConversationModelChoice(CHOICE_VIEW, {
				model: "modello-uno",
				options: {},
			}),
		);
		const segments = row[1].segments;

		assert.deepEqual(
			segments.map((segment) => segment.value),
			["", "a", "b"],
			"i segmenti non sono la rinuncia più le scelte dichiarate",
		);
		assert.equal(segments[0].text, "auto");
		assert.ok(segments[0].pressed, "il valore vuoto in vigore non è quello premuto");
		assert.equal(row[1].value, "auto");
	});

	it("non disegna la seconda pastiglia se il modello non dichiara opzioni", () => {
		const row = agentRow(
			renderConversationModelChoice(CHOICE_VIEW, {
				model: "modello-due",
				options: {},
			}),
		);

		assert.deepEqual(row.map((pill) => pill.key), ["model"]);
	});

	it("raccoglie le opzioni oltre la prima in una pastiglia sola", () => {
		const view = {
			...CHOICE_VIEW,
			models: [
				{
					id: "modello-uno",
					label: "Modello Uno",
					options: [
						{ name: "sforzo", label: "Sforzo", choices: [{ value: "a" }] },
						{ name: "verbosita", label: "Verbosità", choices: [{ value: "x" }] },
						{ name: "lingua", label: "Lingua", choices: [{ value: "it" }] },
					],
				},
			],
		};
		const row = agentRow(renderConversationModelChoice(view, null));

		assert.deepEqual(
			row.map((pill) => pill.key),
			["model", "option:sforzo", "more"],
			"la riga cresce oltre tre voci",
		);
		assert.equal(row[2].value, "+2 opzioni");
		assert.deepEqual(
			[...new Set(row[2].segments.map((segment) => segment.option))],
			["verbosita", "lingua"],
			"la pastiglia di coda non raccoglie le opzioni oltre la prima",
		);
	});

	// Il pannello si ridisegna a ogni giro di poll: quale popover è aperto
	// arriva da fuori, e uno solo alla volta.
	it("apre il popover che le viene chiesto, e uno solo", () => {
		const row = agentRow(
			renderConversationModelChoice(CHOICE_VIEW, null, "option:sforzo"),
		);

		assert.deepEqual(
			row.map((pill) => pill.open),
			[false, true],
			"l'apertura chiesta non è quella disegnata",
		);
	});

	it("senza catalogo scrive il modello e non offre nessuna pastiglia", () => {
		const html = renderConversationModelChoice(
			{
				available: false,
				model: "modello-uno",
				unavailable_reason: "il provider non espone un catalogo",
			},
			null,
		);

		assert.equal(agentRow(html).length, 0, "una pastiglia è premibile invano");
		assert.ok(visibleText(html).includes("⚠ modello-uno"));
		assert.ok(
			html.includes('title="il provider non espone un catalogo"'),
			"la ragione del server non è nel title",
		);
	});

	it("non lancia su payload parziali", () => {
		const partials = [
			null,
			undefined,
			{},
			{ available: true },
			{ available: true, model: "sconosciuto" },
			{ available: true, models: "non-una-lista" },
			{ available: true, models: [null] },
			{ available: true, model: "m", models: [{ id: "m", options: null }] },
			{ available: false },
		];
		for (const view of partials) {
			assert.equal(
				typeof renderConversationModelChoice(view, null, "model"),
				"string",
				"il renderer non ha restituito una stringa",
			);
		}
	});
});
