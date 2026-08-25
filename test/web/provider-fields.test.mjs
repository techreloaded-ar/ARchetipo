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
//   - AC-1 il modello ereditato dal workspace è la voce selezionata, e
//     l'ereditarietà è dichiarata a parole
//   - AC-2 le opzioni mostrate sono quelle del modello selezionato
//   - AC-6 senza catalogo il motivo è visibile e nessun selettore compare
// ---------------------------------------------------------------------------

const { renderModelChoice } = loadProviderFields();

// Le voci del selettore del modello della run, isolate dal resto del markup.
function runModelControl(html) {
	const m = /<select name="run_model"[^>]*>([\s\S]*?)<\/select>/.exec(html);
	return m ? options(m[1]) : null;
}

// Le voci del controllo di una opzione della run, per nome.
function runOptionControl(html, name) {
	const re = new RegExp(
		`<select name="run_option_${name}"[^>]*>([\\s\\S]*?)</select>`,
	);
	const m = re.exec(html);
	return m ? options(m[1]) : null;
}

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

describe("renderModelChoice — scelta per la singola run", () => {
	it("mostra il modello ereditato dal workspace come voce selezionata", () => {
		const html = renderModelChoice(CHOICE_VIEW, null);
		const entries = runModelControl(html);

		assert.ok(entries, "il selettore del modello della run non è stato disegnato");
		const selected = entries.filter((o) => o.selected);
		assert.equal(selected.length, 1, "esattamente una voce deve essere selezionata");
		assert.equal(
			selected[0].value,
			"modello-uno",
			"la voce selezionata non è il modello ereditato dal workspace",
		);
		const text = visibleText(html);
		assert.ok(
			/ereditato dal workspace/i.test(text),
			"l'ereditarietà dal workspace non è dichiarata a parole",
		);
		assert.ok(text.includes("Modello Uno"), "il modello ereditato non è leggibile");
	});

	it("offre le opzioni del modello selezionato e non quelle degli altri", () => {
		const withOptions = renderModelChoice(CHOICE_VIEW, { model: "modello-uno" });
		const entries = runOptionControl(withOptions, "sforzo");
		assert.ok(entries, "l'opzione del modello scelto non è stata disegnata");
		assert.deepEqual(
			entries.map((o) => o.value),
			["", "a", "b"],
			"la voce vuota più le due scelte dichiarate, in quell'ordine",
		);
		assert.ok(
			withOptions.includes('data-run-option="sforzo"'),
			"il controllo dell'opzione non porta il proprio nome in un attributo dedicato",
		);

		const other = renderModelChoice(CHOICE_VIEW, { model: "modello-due" });
		assert.equal(
			runOptionControl(other, "sforzo"),
			null,
			"l'opzione di un altro modello resta disegnata dopo il cambio di modello",
		);
		assert.ok(
			!other.includes('data-run-option="'),
			"un modello senza opzioni disegna comunque un controllo di opzione",
		);
		assert.ok(
			visibleText(other).includes("Questo modello non dichiara nessuna opzione."),
			"il modello senza opzioni non lo dichiara a parole",
		);
	});

	it("riporta il valore di opzione già scelto", () => {
		const entries = runOptionControl(
			renderModelChoice(CHOICE_VIEW, {
				model: "modello-uno",
				options: { sforzo: "b" },
			}),
			"sforzo",
		);
		const selected = entries.filter((o) => o.selected);
		assert.equal(selected.length, 1, "esattamente una voce deve essere selezionata");
		assert.equal(selected[0].value, "b", "la scelta già fatta non è riproposta selezionata");
	});

	it("dichiara la scelta non disponibile con il motivo", () => {
		const html = renderModelChoice(
			{
				available: false,
				model: "modello-uno",
				model_source: "workspace",
				unavailable_reason: "il catalogo non è ottenibile",
			},
			null,
		);
		const text = visibleText(html);

		assert.ok(
			text.includes("il catalogo non è ottenibile"),
			"il motivo dell'indisponibilità non è visibile",
		);
		assert.ok(text.includes("modello-uno"), "il modello effettivo non è dichiarato");
		assert.ok(
			!html.includes("<select"),
			"la scelta non disponibile disegna comunque un selettore",
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
			const html = renderModelChoice(view, null);
			assert.equal(typeof html, "string", "il renderer non ha restituito una stringa");
		}
	});

	// Una voce vuota offerta sopra un modello ereditato sarebbe una scelta che
	// il server non può distinguere da «nessuna scelta»: il pannello mostrerebbe
	// «il provider sceglie» e la run partirebbe con il modello configurato.
	// L'unico caso in cui la voce vuota dice il vero è quando è già lei quella
	// in vigore.
	it("non offre la voce vuota sopra un modello ereditato", () => {
		const entries = runModelControl(renderModelChoice(CHOICE_VIEW, null));

		assert.ok(entries, "il selettore del modello della run non è stato disegnato");
		assert.ok(
			!entries.some((o) => o.value === ""),
			"la voce vuota è offerta anche se un modello è già in vigore",
		);
	});

	it("offre la voce vuota quando è il modello ereditato a essere vuoto", () => {
		const entries = runModelControl(
			renderModelChoice({ ...CHOICE_VIEW, model: "" }, null),
		);

		assert.ok(entries, "il selettore del modello della run non è stato disegnato");
		const empty = entries.filter((o) => o.value === "");
		assert.equal(empty.length, 1, "la voce vuota deve essere offerta una sola volta");
		assert.ok(
			empty[0].selected,
			"la voce vuota in vigore non è quella selezionata",
		);
	});
});
