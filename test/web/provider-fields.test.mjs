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

		const marked = options(html).filter((o) => /provider default/i.test(o.text));
		assert.equal(
			marked.length,
			1,
			"esattamente una voce deve essere indicata come predefinito del provider",
		);
		assert.ok(
			marked[0].text.includes("Modello Beta"),
			"la dicitura di predefinito è accanto al modello sbagliato",
		);
		assert.ok(text.includes("provider default"), "la dicitura di predefinito non è testo visibile");
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
			!/not listed/i.test(visibleText(html)),
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
		const unlisted = options(html).filter((o) => /not listed/i.test(o.text));
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
		assert.ok(!/provider default/i.test(text), "nessuna dicitura di predefinito senza catalogo");
		assert.ok(!/not listed/i.test(text), "nessuna dicitura di voce non elencata senza catalogo");
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
