// test/web/config-single-save.test.mjs
// Oracoli per la modale di configurazione con un solo salvataggio.
// Esecuzione: node --test test/web/config-single-save.test.mjs
//
// Il fatto da presidiare e' uno: nella modale c'e' un bottone che salva, e
// salva tutto. Il provider di esecuzione non ha piu' un bottone suo, viaggia
// nello stesso payload del form guidato, e il footer dice in quali schede ci
// sono modifiche non ancora salvate. Sono proprieta' di *dove* le cose sono
// scritte, e si leggono nelle sorgenti — la stessa disciplina di
// p1-audit-fixes.test.mjs.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const assetsDir = resolve(__dirname, "..", "..", "cli", "internal", "web", "assets");
const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");
const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");

function sectionOf(source, marker) {
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `la sorgente non contiene più \`${marker}\``);
	const open = source.indexOf("{", at);
	let depth = 0;
	for (let i = open; i < source.length; i++) {
		if (source[i] === "{") depth++;
		if (source[i] === "}") depth--;
		if (depth === 0) return source.slice(open, i + 1);
	}
	assert.fail(`blocco non bilanciato dopo \`${marker}\``);
}

describe("la configurazione si salva con un bottone solo", () => {
	it("il provider non ha piu' un bottone di salvataggio suo", () => {
		assert.ok(!html.includes('id="execution-save-btn"'), "la scheda Esecuzione ha ancora un bottone di salvataggio proprio");
		assert.ok(!js.includes("saveExecutionProvider"), "app.js salva ancora il provider per conto suo");
		assert.ok(!js.includes("/api/execution/provider/default"), "app.js scrive ancora il provider da una rotta separata");
	});

	it("il provider scelto viaggia nello stesso payload del form guidato", () => {
		const payload = sectionOf(js, "function configPayload()");
		assert.match(payload, /provider/, "il payload del salvataggio non porta il provider");
		assert.match(payload, /collectProviderConfig/, "il payload non porta i campi del provider");
	});

	it("chiudere con un provider cambiato chiede conferma come ogni altra modifica", () => {
		const states = sectionOf(js, "function configSectionStates()");
		assert.match(states, /selectedProviderID\(\)/, "lo stato della modale ignora il provider scelto");
		assert.match(states, /executionFields/, "lo stato della modale ignora i campi del provider");
		assert.match(states, /configConnectorGrid/, "lo stato della modale ignora il connettore scelto");
	});

	it("il rilascio per inattivita' si legge e si scrive dal form guidato", () => {
		assert.ok(html.includes('name="execution_idle_release_seconds"'), "la scheda Esecuzione non ha il campo del rilascio per inattivita'");
		const settings = sectionOf(js, "function executionSettings()");
		assert.match(settings, /idle_release_seconds/, "il form guidato non scrive execution.idle_release_seconds");
		const build = sectionOf(js, "function buildGuidedConfig()");
		assert.match(build, /executionSettings\(\)/, "buildGuidedConfig non porta le impostazioni di esecuzione");
	});

	it("il footer dice in quali schede ci sono modifiche non salvate", () => {
		assert.ok(html.includes('id="config-dirty"'), "il footer non ha piu' la riga delle modifiche non salvate");
		assert.match(js, /configUnsaved:/, "manca la frase per le modifiche non salvate");
		const refresh = sectionOf(js, "function refreshConfigDirty()");
		assert.match(refresh, /configSections\[/, "la riga non nomina le schede");
	});
});
