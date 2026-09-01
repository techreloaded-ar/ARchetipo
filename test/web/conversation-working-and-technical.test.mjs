// test/web/conversation-working-and-technical.test.mjs
// Oracoli per le tre modifiche al visore chieste su archetipo-view.
// Esecuzione: node --test test/web/conversation-working-and-technical.test.mjs
//
// Le tre cose:
//
//   1. la firma in fondo alla pagina porta da qualche parte — il nome del
//      prodotto al suo repository, la firma a chi lo fa;
//   2. fra l'invio di un messaggio e la risposta la conversazione dichiara che
//      l'agente sta lavorando, e la dichiarazione finisce quando finisce il
//      turno — non prima e, soprattutto, non da prima che il turno cominci:
//      ogni conversazione vera si apre con due righe di gancio, e leggere solo
//      l'ultima riga della storia faceva dichiarare un'attesa a conversazione
//      appena aperta;
//   3. il dettaglio tecnico — strumenti, ganci, ragionamento — sta ripiegato,
//      e chi lo cerca lo apre.
//
// Il renderer è presidiato in conversation.test.mjs, che è dove stanno le
// parole. Qui si prova il resto: la regola del turno, che è una funzione di
// app.js e si estrae per chiamarla davvero, e il collegamento fra pannello e
// renderer, che è una proprietà di *dove* le cose sono scritte e si legge
// nella sorgente — la stessa disciplina di p1-audit-fixes.test.mjs.

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

const js = readFileSync(resolve(assetsDir, "app.js"), "utf8");
const css = readFileSync(resolve(assetsDir, "app.css"), "utf8");
const html = readFileSync(resolve(assetsDir, "index.html"), "utf8");

/** Il testo intero della funzione `name`, dalla parola `function` alla sua graffa. */
function functionOf(source, name) {
	const marker = `function ${name}(`;
	const at = source.indexOf(marker);
	assert.notEqual(at, -1, `app.js non contiene più \`${name}\``);
	const open = source.indexOf("{", at);
	let depth = 0;
	for (let i = open; i < source.length; i++) {
		if (source[i] === "{") depth++;
		else if (source[i] === "}") {
			depth--;
			if (depth === 0) return source.slice(at, i + 1);
		}
	}
	assert.fail(`blocco non bilanciato in \`${name}\``);
}

const CONVERSATION_WORKING = functionOf(js, "conversationWorking");
// Le righe da cui si capisce a che cosa si sta lavorando viaggiano insieme alla
// regola che le legge: se una viene cambiata senza l'altra, questo test se ne
// accorge invece di provare una regola che nel visore non esiste più.
const ACTIVITY_KINDS = (js.match(
	/const CONVERSATION_ACTIVITY_KINDS = \{[\s\S]*?\};/,
) || [])[0];
assert.ok(ACTIVITY_KINDS, "app.js non dichiara più le righe di attività");

// La regola del turno si prova chiamandola, non leggendola: la funzione vive
// dentro la chiusura di app.js e legge lo stato del pannello come variabili
// libere, quindi qui lo stato glielo si mette intorno.
function working(state) {
	const ctx = createContext({
		conversationIsActive: () => state.active !== false,
		conversationEvents: state.events || [],
		conversationPendingMessage: state.pending || "",
		conversationWorkingSince: state.since || 0,
		Date,
		Math,
		out: null,
	});
	runInContext(
		`${ACTIVITY_KINDS}\n${CONVERSATION_WORKING}\nout = conversationWorking();`,
		ctx,
	);
	return ctx.out;
}

// Le due righe con cui ogni conversazione vera comincia: i ganci scattati
// all'avvio dell'agente, che nessun provider traduce nel vocabolario comune.
const GANCI_DI_APERTURA = [
	{ id: 1, kind: "system/hook_started" },
	{ id: 2, kind: "system/hook_response" },
];

describe("la firma in fondo alla pagina porta da qualche parte", () => {
	it("il nome del prodotto porta al suo repository", () => {
		assert.match(
			html,
			/<a class="hint-link" href="https:\/\/github\.com\/techreloaded-ar\/ARchetipo"[^>]*>ARchetipo<\/a>/,
			"«ARchetipo» in fondo alla pagina non porta al repository",
		);
	});

	it("la firma porta al sito di chi lo fa", () => {
		assert.match(
			html,
			/<a class="hint-link" href="https:\/\/techreloaded\.it"[^>]*>by Tech Reloaded<\/a>/,
			"«by Tech Reloaded» non porta al sito di Tech Reloaded",
		);
	});

	it("i due collegamenti si aprono altrove e senza dare la finestra a chi li riceve", () => {
		const links = html.match(/<a class="hint-link"[^>]*>/g) || [];
		assert.equal(links.length, 2, "i collegamenti della firma non sono due");
		for (const link of links) {
			assert.match(link, /target="_blank"/, `${link} non si apre in una scheda nuova`);
			assert.match(link, /rel="noopener noreferrer"/, `${link} non è protetto`);
		}
	});

	it("la barra non è più nascosta in blocco alla lettura assistita", () => {
		// Un collegamento raggiungibile col tasto di tabulazione dentro un
		// contenitore aria-hidden è un punto di fermata senza voce: la barra si
		// è scoperta e sono i promemoria dei tasti a essere decorativi.
		assert.ok(
			!/<footer class="hint-bar" aria-hidden="true">/.test(html),
			"la firma è tornata dentro un contenitore nascosto",
		);
		assert.match(html, /<span aria-hidden="true"><kbd>R<\/kbd>/);
	});

	it("i collegamenti hanno un contorno visibile da tastiera", () => {
		assert.match(css, /\.hint-link:focus-visible \{[^}]*outline:/);
	});
});

describe("la regola del turno", () => {
	it("una conversazione appena aperta non sta aspettando l'agente", () => {
		// Il caso che si vedeva davvero: appena aperta la conversazione non è
		// vuota — porta già i due ganci di apertura — e leggere solo l'ultima
		// riga faceva dire che l'agente stava lavorando prima ancora che
		// qualcuno gli avesse scritto. Lì non si aspetta l'agente: si aspetta
		// chi scrive.
		assert.equal(working({ events: [] }).active, false);
		assert.equal(
			working({ events: GANCI_DI_APERTURA }).active,
			false,
			"i ganci di apertura vengono contati come un turno in corso",
		);
	});

	it("un messaggio consegnato e non ancora tornato indietro è già un turno", () => {
		const stato = working({ events: GANCI_DI_APERTURA, pending: "DETTO-ADESSO" });
		assert.equal(stato.active, true);
		assert.equal(
			stato.kind,
			"",
			"l'attesa nomina un'attività che l'agente non ha ancora svolto",
		);
	});

	it("fra la domanda e la fine del turno l'agente sta lavorando", () => {
		const stato = working({
			events: [
				...GANCI_DI_APERTURA,
				{ id: 3, kind: "user_message", text: "DOMANDA" },
				{ id: 4, kind: "tool_start", tool: "STRUMENTO-UNO" },
			],
		});
		assert.equal(stato.active, true);
		assert.equal(stato.kind, "tool_start");
		assert.equal(
			stato.tool,
			"STRUMENTO-UNO",
			"l'attesa non dice a quale strumento sta lavorando",
		);
	});

	it("subito dopo l'invio il turno è aperto e nessuna attività è ancora nota", () => {
		// L'altro caso che si vedeva: l'agente ha ricevuto il messaggio e non ha
		// ancora prodotto niente. Il turno è aperto, e che cosa stia facendo non
		// si sa ancora — che è un fatto, non una frase finita.
		const stato = working({
			events: [...GANCI_DI_APERTURA, { id: 3, kind: "user_message", text: "DOMANDA" }],
		});
		assert.equal(stato.active, true);
		assert.equal(stato.kind, "");
	});

	it("una riga di protocollo non cancella l'attività in corso", () => {
		// I ganci scattano anche in mezzo a un turno. Dicono che qualcosa è
		// successo, non che cosa si sta facendo: fra un gancio e l'altro «sta
		// ragionando» resta vero.
		const stato = working({
			events: [
				...GANCI_DI_APERTURA,
				{ id: 3, kind: "user_message" },
				{ id: 4, kind: "thinking" },
				{ id: 5, kind: "system/hook_started" },
			],
		});
		assert.equal(stato.active, true);
		assert.equal(stato.kind, "thinking");
	});

	it("la fine del turno chiude l'attesa", () => {
		assert.equal(
			working({
				events: [
					...GANCI_DI_APERTURA,
					{ id: 3, kind: "user_message" },
					{ id: 4, kind: "text", text: "RISPOSTA" },
					{ id: 5, kind: "turn_end" },
				],
			}).active,
			false,
		);
	});

	it("chiuso un turno, il successivo riapre l'attesa", () => {
		const stato = working({
			events: [
				{ id: 1, kind: "user_message" },
				{ id: 2, kind: "text" },
				{ id: 3, kind: "turn_end" },
				{ id: 4, kind: "user_message" },
				{ id: 5, kind: "thinking" },
			],
		});
		assert.equal(stato.active, true);
		assert.equal(stato.kind, "thinking");
	});

	it("un errore chiude l'attesa come la chiude la fine del turno", () => {
		assert.equal(
			working({
				events: [
					{ id: 1, kind: "user_message" },
					{ id: 2, kind: "error", text: "GUASTO" },
				],
			}).active,
			false,
		);
	});

	it("fuori da una conversazione viva non si aspetta niente", () => {
		assert.equal(
			working({
				active: false,
				pending: "DETTO-ADESSO",
				events: [{ id: 1, kind: "user_message" }],
			}).active,
			false,
		);
	});

	it("l'attesa si misura da quando è cominciata, non dall'ultimo disegno", () => {
		const stato = working({
			events: [{ id: 1, kind: "user_message" }, { id: 2, kind: "thinking" }],
			since: Date.now() - 7_000,
		});
		assert.ok(
			stato.seconds >= 6 && stato.seconds <= 9,
			`l'attesa dovrebbe essere di circa 7 secondi, è ${stato.seconds}`,
		);
	});
});

describe("il pannello passa al renderer ciò che il renderer non può sapere", () => {
	it("l'attesa e le pieghe aperte viaggiano nello stato locale", () => {
		const chiamata = js.slice(
			js.indexOf("window.Conversation.renderConversation("),
		);
		const argomenti = chiamata.slice(0, chiamata.indexOf("\n\t\t);"));
		for (const campo of ["technicalOpen:", "technicalAll:", "working,"]) {
			assert.ok(
				argomenti.includes(campo),
				`il pannello non passa \`${campo}\` al renderer`,
			);
		}
	});

	it("i due comandi del dettaglio tecnico sono ascoltati dal contenitore", () => {
		const legame = js.slice(js.indexOf("function bindConversationPanel"));
		assert.ok(legame.includes("data-conversation-technical-toggle"));
		assert.ok(legame.includes("data-conversation-technical-all"));
	});

	it("la scelta di tenere aperto il dettaglio tecnico resta scritta", () => {
		assert.match(js, /CONVERSATION_TECHNICAL_KEY = "archetipo:conversation:technical"/);
		assert.match(js, /localStorage\.getItem\(CONVERSATION_TECHNICAL_KEY\)/);
		assert.match(js, /localStorage\.setItem\(\s*CONVERSATION_TECHNICAL_KEY/);
	});

	it("il comando della testata azzera anche le pieghe aperte a mano", () => {
		// «Ripiega il dettaglio tecnico» deve ripiegare tutto: senza l'azzeramento
		// una piega aperta una per una restava aperta, e il comando non
		// comandava cio' che dice.
		const commuta = functionOf(js, "toggleConversationTechnicalAll");
		assert.ok(
			commuta.includes("conversationTechnicalOpen = {}"),
			"commutare il dettaglio tecnico non dimentica le pieghe aperte a mano",
		);
	});

	it("le pieghe aperte si dimenticano cambiando conversazione", () => {
		// Una piega aperta appartiene alla conversazione in cui è stata aperta:
		// sotto un'altra sarebbe una scelta fatta su qualcos'altro.
		const cambio = js.slice(
			js.indexOf("async function switchToConversation"),
			js.indexOf("function startConversationPolling"),
		);
		assert.ok(cambio.includes("conversationTechnicalOpen = {}"));
		assert.ok(cambio.includes("conversationWorkingSince = 0"));
	});

	it("il contatore dei secondi non ridisegna il pannello", () => {
		// Ridisegnare tutto una volta al secondo costerebbe quanto un giro di
		// poll per far scorrere un numero: il tempo riscrive quel solo numero.
		const ticker = functionOf(js, "startConversationWorkingTicker");
		assert.ok(
			!ticker.includes("renderConversationPanel"),
			"il contatore dei secondi ridisegna tutto il pannello",
		);
		assert.ok(ticker.includes("data-conversation-working-elapsed"));
	});
});
