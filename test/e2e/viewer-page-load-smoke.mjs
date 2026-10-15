#!/usr/bin/env node

// End-to-end smoke for "the viewer page actually runs".
//
// Every other smoke under test/e2e/ starts `archetipo view` and then talks to
// it with `fetch`: they prove the protocol, and never the page. test/web/ is the
// other half — pure renderers imported as modules, plus a few assertions on the
// *text* of app.js. Between the two there was a hole exactly the size of the
// regression this smoke exists for: a `TypeError` thrown at top level inside the
// IIFE of app.js interrupted the file, so everything below the throw was never
// executed. The rail of conversations was drawn but no click handler had been
// registered, and two `const` further down stayed in temporal dead zone, so
// "Nuova spec" stopped answering too. The board still worked, because it is
// bound above the breaking point. Nothing was red.
//
// So this smoke loads the real page in a real browser and asks the two questions
// no fetch can ask:
//
//   1. did anything throw while the page was loading?  (the direct oracle)
//   2. are the handlers that live *after* the first draw actually there?
//      (the oracle that matters, because a clean load does not prove a bind)
//
// The second one is asked by pressing things: a thread of the rail — bound at
// app.js:5452, below the point that used to break — must open that conversation,
// and "Nuova spec" — whose handler reads a `const` declared at app.js:6503, below
// the same point — must open its modal.
//
// No new dependency. Chrome is driven over the DevTools protocol through the
// `WebSocket` that Node 22+ exposes as a global, so there is no puppeteer, no
// playwright and no jsdom in package.json. The workspace is real, the CLI is
// built from source, the viewer is the real one, and the past conversation the
// rail must list is seeded as the record file the journal itself writes —
// `.archetipo/conversations/<id>.json` — so no agent, no credential and no
// network are involved.
//
// There is no arbitrary sleep: every wait polls the page itself with an explicit
// timeout that names what it was waiting for and what was on screen instead.
//
// ---- What happens where Chrome is not installed ----------------------------
//
// This is the one decision a browser test cannot avoid, and it is taken here
// rather than guessed at run time: **a missing browser is a failure, unless the
// caller has said in advance that it is acceptable.**
//
// Running with no browser and no flag exits 1 and explains how to point the
// smoke at a Chrome. Running with `--allow-missing-browser` (or
// ARCHETIPO_ALLOW_MISSING_BROWSER=1) exits 0, but prints on stdout *and* stderr
// a banner that names, one by one, the claims that were not proved.
//
// The point of the split is that the skip is never the test's own inference from
// its environment: it is a written, reviewable choice of whoever configured the
// pipeline. A CI without Chrome stays green only because that line is in the CI
// file where someone can see it, and a CI with Chrome gets the real check. That
// is the only shape that satisfies both halves of the requirement — not red for
// a missing browser, and never quietly green.

import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, waitForHTTP } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "viewer-page-load-smoke");

// The conversation the rail must list, and the words that prove the panel is
// showing *that* transcript and not an empty state that happens to look busy.
const CONVERSATION_ID = "smoke-page-load-conversation";
const CONVERSATION_TITLE = "Conversazione di prova del caricamento pagina";
const HUMAN_SENTINEL = "smoke-page-load-human-said-sentinel";
const AGENT_SENTINEL = "smoke-page-load-agent-said-sentinel";

// What this smoke claims to prove. Printed on success one by one, and printed
// again — as *unproved* — when a missing browser makes it skip, so a skip can
// never be read as a pass.
const CLAIMS = [
  "il caricamento della pagina non emette nessuna eccezione non catturata",
  "il rail elenca la conversazione presente sul disco del workspace",
  "il click su un thread del rail apre quella conversazione (bind sotto il primo disegno)",
  "il click su «Nuova spec» apre la modale (const dichiarata sotto il primo disegno)",
  "nessuna eccezione non catturata è emessa da nessuna delle due interazioni",
  "le card della board mostrano le azioni e i motivi restituiti dal server",
  "Conversa sulla card prepara una conversazione collegata alla stessa spec",
  "la conferma del form crea una storia leggibile dal server",
  "un workspace appena inizializzato permette di creare la prima epica e storia",
  "le attività raccolgono la proposta prima di avviare una skill e conservano il testo se l’avvio fallisce",
  "le esecuzioni si possono fermare anche dalla vista Conversazione",
  "i permessi dichiarati dal provider si scelgono e si salvano dal prossimo turno senza perdersi cambiando modello",
  "Documentazione legge la Wiki, cerca nel contenuto e naviga tra pagine con cronologia e link diretti",
  "Documentazione conserva la bozza di conversazione e rende il Markdown senza contenuti attivi",
  "Documentazione è leggibile su schermi stretti e distingue Wiki assente e pagina mancante",
];

// One entry per proved statement, for the report.
const checks = [];

function ok(claim, statement) {
  checks.push({ claim, statement });
  console.log(`-> ok: ${statement}`);
}

async function main() {
  const options = parseArgs(process.argv.slice(2));

  const chromePath = await findChrome(options.chromePath);
  if (!chromePath) {
    reportMissingBrowser(options.allowMissingBrowser, options.chromePath);
    if (options.allowMissingBrowser) return;
    process.exitCode = 1;
    return;
  }
  console.log(`-> browser: ${chromePath}`);

  const runDir = await createRunDir(options.workspaceRoot);
  console.log(`-> run directory: ${runDir}`);
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
  const cliEnv = {
    ...process.env,
    ARCHETIPO_DATA_DIR: repoRoot,
    ARCHETIPO_STATE_DIR: path.join(runDir, "state"),
  };

  let view = null;
  let browser = null;
  try {
    await buildCLI(cliEnv);
    await runCommand("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
      cwd: sandboxDir,
      env: cliEnv,
    });
    // A backlog with an epic in it, so pressing "Nuova spec" walks the long
    // branch of openNewSpec — the one that fills the epic select and loads the
    // body editor — instead of the short "no epic" one.
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], {
      cwd: sandboxDir,
      env: cliEnv,
    });
    await seedConversationRecord(sandboxDir);
    await seedDocumentation(sandboxDir);

    view = await startViewServer(sandboxDir, cliEnv);
    console.log(`-> view ready: ${view.url}`);

    browser = await launchChrome(chromePath, path.join(runDir, "chrome-profile"));
    console.log(`-> devtools: ${browser.wsEndpoint}`);

    await exercisePage(browser, view.url);
    await exerciseDocumentation(browser, view.url, runDir);
    await exerciseDocumentationRace(browser, view.url);
    await exerciseActivityComposer(browser, view.url);
    await exercisePermissionComposer(browser, view.url);
    const emptyDir = path.join(runDir, "empty-workspace");
    await fs.mkdir(emptyDir, { recursive: true });
    await runCommand("init-empty", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
      cwd: emptyDir, env: cliEnv,
    });
    await stopProcess(view.child);
    view = await startViewServerShared(cliPath, emptyDir, cliEnv, "/api/workspace/status");
    const emptyIndex = await fs.readFile(path.join(emptyDir, ".archetipo", "backlog.yaml"), "utf8");
    if (!emptyIndex.includes("order: []")) throw new Error("init non ha creato un backlog vuoto");
    await exerciseEmptyWorkspace(browser, view.url);
  } finally {
    if (browser) await browser.close();
    if (view) await stopProcess(view.child);
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned run directory: ${runDir}`);
    }
  }

  console.log(`\nPASS: viewer page-load smoke completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.statement}`);
  }
}

// --- The page itself ---------------------------------------------------------

async function exercisePage(browser, url) {
  const page = await browser.newPage();

  // Every uncaught exception the page reports, in order, with the phase it
  // arrived in. An unhandled promise rejection surfaces here too, so a failure
  // in the asynchronous half of the boot is caught by the same oracle.
  const exceptions = [];
  let phase = "caricamento";
  page.on("Runtime.exceptionThrown", (params) => {
    const details = params.exceptionDetails || {};
    exceptions.push({
      phase,
      text: details.text || "",
      description: (details.exception && details.exception.description) || "",
      url: details.url || "",
      line: typeof details.lineNumber === "number" ? details.lineNumber + 1 : null,
      column: typeof details.columnNumber === "number" ? details.columnNumber + 1 : null,
    });
  });

  // Console and network errors are not the oracle — a page can log an error and
  // still be entirely correct — but when something else fails they are the
  // context that explains it, so they are collected and printed on failure.
  const logEntries = [];
  page.on("Log.entryAdded", (params) => {
    const entry = params.entry || {};
    if (entry.level !== "error") return;
    logEntries.push(`${entry.source || "?"}: ${entry.text || ""}${entry.url ? ` (${entry.url})` : ""}`);
  });

  function assertNoExceptions(what) {
    const raised = exceptions.filter((e) => !e.reported);
    if (raised.length === 0) return;
    for (const e of raised) e.reported = true;
    const rendered = raised
      .map((e) => `  [${e.phase}] ${e.description || e.text} (${e.url || "?"}:${e.line ?? "?"}:${e.column ?? "?"})`)
      .join("\n");
    const context = logEntries.length ? `\nErrori in console:\n  ${logEntries.join("\n  ")}` : "";
    throw new Error(`${what}\n${rendered}${context}`);
  }

  await page.send("Runtime.enable");
  await page.send("Log.enable");
  await page.send("Page.enable");
  await page.send("Page.navigate", { url });
  await page.once("Page.loadEventFired", 30000, "l'evento load della pagina");

  // The moment the load is really over is not `load`: it is the first read of
  // the workspace coming back and the rail being drawn from it. Waiting for that
  // is what gives the exception check something to be true *about*, and it needs
  // no sleep. In the failure this smoke was written for the rail was drawn all
  // the same — it was only deaf — so this wait succeeds either way and the
  // exception assertion right below it is the one that speaks.
  try {
    await page.waitFor(
      `!!document.querySelector('#workspace-conversations [data-conversation-id="${CONVERSATION_ID}"]')`,
      20000,
      `il rail elenca la conversazione ${CONVERSATION_ID}`,
      () =>
        page.evaluate(
          `({ rail: document.getElementById("workspace-conversations")?.innerHTML.length ?? null,
              threads: document.querySelectorAll("#workspace-conversations [data-conversation-id]").length })`,
        ),
    );
  } catch (timedOut) {
    // Un'eccezione al caricamento è la causa, l'attesa scaduta è solo l'effetto:
    // se ce n'è una la si riporta per prima, con il suo file e la sua riga.
    // Altrimenti vale il timeout, che dice cosa c'era sullo schermo.
    assertNoExceptions(
      `${timedOut.message}\n\nLa causa sta a monte — il caricamento della pagina ha emesso eccezioni non catturate:`,
    );
    throw timedOut;
  }
  assertNoExceptions("Il caricamento della pagina ha emesso eccezioni non catturate:");
  ok(CLAIMS[0], "il caricamento della pagina non ha emesso nessuna eccezione non catturata");
  ok(CLAIMS[1], `il rail elenca la conversazione ${CONVERSATION_ID} letta dal disco del workspace`);

  // --- Il click su un thread apre la conversazione ---------------------------
  //
  // Questo è l'oracolo che il caricamento pulito da solo non dà: il gestore del
  // rail è registrato *sotto* il punto in cui l'IIFE si spezzava, quindi un
  // pulsante visibile che non fa niente è esattamente il sintomo che si sta
  // cercando. Il click parte dall'elemento vero, come quello di una persona.
  phase = "click sul thread";
  const clicked = await page.evaluate(
    `(() => {
      const thread = document.querySelector('#workspace-conversations [data-conversation-id="${CONVERSATION_ID}"]');
      if (!thread) return "nessun thread da premere";
      thread.click();
      return "premuto";
    })()`,
  );
  if (clicked !== "premuto") throw new Error(`Il thread del rail non era premibile: ${clicked}`);

  await page.waitFor(
    `(() => {
      const current = document.querySelector('#workspace-conversations .thread.is-current');
      const panel = document.getElementById("workspace-conversation");
      return !!current
        && current.getAttribute("data-conversation-id") === ${JSON.stringify(CONVERSATION_ID)}
        && !!panel && panel.textContent.indexOf(${JSON.stringify(AGENT_SENTINEL)}) >= 0;
    })()`,
    20000,
    "il click sul thread apre quella conversazione nel pannello",
    () =>
      page.evaluate(
        `({ current: document.querySelector('#workspace-conversations .thread.is-current')?.getAttribute("data-conversation-id") ?? null,
            pannello: (document.getElementById("workspace-conversation")?.textContent || "").slice(0, 400) })`,
      ),
  );
  assertNoExceptions("Il click sul thread del rail ha emesso eccezioni non catturate:");
  ok(CLAIMS[2], "il click su un thread del rail apre quella conversazione, sentinella dell'agente compresa");

  // --- Il click su «Nuova spec» apre la modale -------------------------------
  //
  // L'altro sintomo della stessa rottura, e per la stessa ragione: openNewSpec
  // legge `newSpecGuard`, dichiarata sotto il punto di rottura, e la leggeva in
  // temporal dead zone. Una modale che si apre è la prova che quella `const` è
  // stata inizializzata.
  phase = "click su Nuova spec";
  const pressed = await page.evaluate(
    `(() => {
      const btn = document.getElementById("new-spec-btn");
      if (!btn) return "nessun pulsante Nuova spec";
      btn.click();
      return "premuto";
    })()`,
  );
  if (pressed !== "premuto") throw new Error(`Il pulsante «Nuova spec» non era premibile: ${pressed}`);

  await page.waitFor(
    `!document.getElementById("new-spec-modal").classList.contains("hidden")`,
    10000,
    "il click su «Nuova spec» apre la modale",
    () =>
      page.evaluate(
        `({ classi: document.getElementById("new-spec-modal")?.className ?? null })`,
      ),
  );
  assertNoExceptions("Il click su «Nuova spec» ha emesso eccezioni non catturate:");
  ok(CLAIMS[3], "il click su «Nuova spec» apre la modale: la sentinella dichiarata più in basso è inizializzata");
  ok(CLAIMS[4], "nessuna delle due interazioni ha emesso eccezioni non catturate");

  const duplicateControls = await page.evaluate(`!!document.querySelector('#workspace-conversation .conv-nextstep, #workspace-conversation [data-conversation-actions-toggle]')`);
  if (duplicateControls) throw new Error('La conversazione mostra ancora il suggerimento o il menu Azioni');

  phase = "azioni dalla board";
  await page.evaluate(`document.getElementById('new-spec-modal-close').click(); document.querySelector('[data-shell-view="board"]').click();`);
  await page.waitFor(`!!document.querySelector('[data-card-actions="US-701"]')`, 20000, "il comando Azioni della card");
  await page.evaluate(`document.querySelector('[data-card-actions="US-701"]').click()`);
  await page.waitFor(`!!document.querySelector('.card[data-code="US-701"] .card-action-choices')`, 20000, "le azioni lette dal server sulla card");
  const serverActions = await (await fetch(`${url}/api/spec/US-701`)).json();
  const drawnActions = await page.evaluate(`Array.from(document.querySelectorAll('.card[data-code="US-701"] [data-card-action]')).map(button=>({id:button.dataset.cardAction,label:button.textContent,disabled:button.disabled}))`);
  const expectedActions = (serverActions.actions || []).map(action=>({id:action.id,label:action.label,disabled:action.runnable !== true}));
  if (JSON.stringify(drawnActions) !== JSON.stringify(expectedActions)) throw new Error('La card inventa le azioni o la loro disponibilità');
  const cardText = await page.evaluate(`document.querySelector('.card[data-code="US-701"] .card-action-choices').textContent`);
  for (const action of serverActions.actions || []) {
    const reason = action.unlocked_by || action.unavailable_reason;
    if (!action.runnable && reason && !cardText.includes(reason)) throw new Error('La card nasconde il motivo di indisponibilità');
  }
  ok(CLAIMS[5], "le azioni della card e le loro ragioni coincidono con il dettaglio del server");
  await page.evaluate(`document.querySelector('.card[data-code="US-701"] .card-actions button[aria-label="Conversa su US-701"]').click()`);
  await page.waitFor(`document.getElementById('workspace-conversation').textContent.includes('US-701')`, 20000, "il contesto della spec nella nuova conversazione");
  assertNoExceptions("I comandi della board hanno emesso eccezioni non catturate:");
  ok(CLAIMS[6], "Conversa sulla card prepara il contesto US-701 senza avviare un agente");


  phase = "creazione manuale di una storia";
  await page.evaluate(`document.getElementById('new-spec-btn').click()`);
  await page.waitFor(`!document.getElementById('new-spec-modal').classList.contains('hidden')`, 10000, "il form della nuova storia");
  const storyTitle = "Storia creata dal form nel browser";
  const body = "**User Story**\nCome revisore voglio creare una storia.\n\n**Dimostrazione**\nCompilo il form e osservo la storia nella board.\n\n**Criteri di accettazione**\n- [ ] AC-1 — La storia compare in TODO.\n";
  await page.evaluate(`(() => {
    const form = document.getElementById('new-spec-form');
    form.elements.title.value = ${JSON.stringify(storyTitle)};
    form.elements.epic_code.value = 'EP-700';
    document.querySelector('#new-spec-modal .CodeMirror').CodeMirror.setValue(${JSON.stringify(body)});
    document.getElementById('new-spec-submit').click();
  })()`);
  await page.waitFor(`document.getElementById('new-spec-modal').classList.contains('hidden')`, 10000,
    "la creazione della storia", () => page.evaluate(`document.getElementById('new-spec-status').textContent`));
  const created = await (await fetch(`${url}/api/spec/US-702`)).json();
  if (created.spec?.title !== storyTitle) throw new Error('Il form non ha persistito la storia attesa');
  assertNoExceptions("La creazione manuale ha emesso eccezioni:");
  ok(CLAIMS[7], "la conferma del form crea US-702 e il dettaglio del server ne restituisce il titolo");
  await page.close();
}

// The real page with deterministic provider responses: no agent or credential.
// Server tests separately prove the native cancellation and persisted effects.
async function exerciseActivityComposer(browser, url) {
  const page = await browser.newPage();
  const exceptions = [];
  page.on("Runtime.exceptionThrown", params => exceptions.push(params.exceptionDetails));
  await page.send("Runtime.enable");
  await page.send("Page.enable");
  await page.send("Page.addScriptToEvaluateOnNewDocument", { source: `
    const originalFetch = window.fetch.bind(window);
    window.smokeStarts = [];
    window.smokeStopped = false;
    window.fetch = async (input, options = {}) => {
      const path = new URL(typeof input === 'string' ? input : input.url, location.href).pathname;
      const method = options.method || 'GET';
      const json = (body, status = 200) => new Response(JSON.stringify(body), {status, headers:{'Content-Type':'application/json'}});
      if (path === '/api/workspace/conversations' && method === 'GET') return json({conversations:[],available:true,closed_limit:20});
      if (path === '/api/workspace/activities') return json({items:[
        {id:'story',label:'Nuova storia',description:'Descrivi la tua proposta',placeholder:'Descrivi la tua proposta…',result:'Storia per il backlog',kind:'action',scope:'workspace',action:'spec-draft',primary:true,runnable:true},
        {id:'product',label:'Definisci un prodotto',description:'Descrivi la tua idea',placeholder:'Descrivi la tua idea…',result:'Documento di prodotto',kind:'action',scope:'workspace',action:'inception',primary:true,runnable:true}
      ]});
      if (path === '/api/workspace/execution' && method === 'POST' || path === '/api/workspace/conversations' && method === 'POST') {
        window.smokeStarts.push({path,body:JSON.parse(options.body)});
        return json({error:'Avvio rifiutato per la prova'},409);
      }
      if (path === '/api/workspace/runs') return json({runs:window.smokeStopped ? [] : [{id:'smoke-active',scope:'workspace',action:'test-action',status:'RUNNING'}]});
      if (path === '/api/execution/smoke-active/run/cancel') { window.smokeStopped = true; return json({events:[],approvals:[],run:null},202); }
      return originalFetch(input, options);
    };
  ` });
  await page.send("Page.navigate", { url });
  await page.once("Page.loadEventFired", 30000, "la pagina con il provider di prova");
  await page.waitFor(`!!document.querySelector('[data-conversation-activity="story"]:not(:disabled)')`, 10000, "Nuova storia disponibile");
  await page.evaluate(`document.querySelector('[data-conversation-activity="story"]').click()`);
  await page.waitFor(`document.querySelector('.conv-composer-input')?.placeholder === 'Descrivi la tua proposta…'`, 10000, "il campo della proposta prima della skill");
  if (await page.evaluate(`window.smokeStarts.length`) !== 0) throw new Error("La scelta di Nuova storia ha avviato un agente prima della proposta");
  const proposal = "Voglio cercare le storie per titolo";
  await page.evaluate(`(() => { const input = document.querySelector('.conv-composer-input'); input.value = ${JSON.stringify(proposal)}; input.dispatchEvent(new Event('input',{bubbles:true})); document.querySelector('[data-shell-view="documentation"]').click(); })()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Panoramica Wiki'`,10000,'Wiki dalla bozza');
  await page.evaluate(`document.querySelector('[data-shell-view="conversation"]').click()`);
  if (await page.evaluate(`document.querySelector('.conv-composer-input').value`) !== proposal) throw new Error('Documentazione perde la bozza della conversazione');
  ok(CLAIMS[13], 'la documentazione preserva cronologia e bozza e il lettore esclude contenuti attivi');
  await page.evaluate(`document.querySelector('.conv-composer').requestSubmit()`);
  await page.waitFor(`window.smokeStarts.length === 1 && !document.querySelector('.conv-composer-input').disabled`, 10000, "l’invio della proposta e il rifiuto del provider");
  const start = await page.evaluate(`window.smokeStarts[0]`);
  if (start.path !== "/api/workspace/execution" || start.body.action !== "spec-draft" || start.body.user_input !== proposal) throw new Error(`La proposta non raggiunge l’azione: ${JSON.stringify(start)}`);
  if (await page.evaluate(`document.querySelector('.conv-composer-input').value`) !== proposal) throw new Error("Il rifiuto ha perso il testo della proposta");
  await page.evaluate(`document.querySelector('[data-conversation-activity-clear]').click(); document.querySelector('[data-conversation-activity="product"]').click()`);
  await page.waitFor(`document.querySelector('.conv-composer-input')?.placeholder === 'Descrivi la tua idea…'`, 10000, "il campo specifico dell’inception");
  if (await page.evaluate(`window.smokeStarts.length`) !== 1) throw new Error("La scelta dell’inception è partita prima dell’invio");
  ok(CLAIMS[9], "storia e inception chiedono la proposta; l’invio la consegna una volta e un rifiuto conserva il testo");
  await page.waitFor(`!!document.querySelector('[data-run-stop="smoke-active"]')`, 10000, "l’esecuzione da fermare");
  await page.evaluate(`document.querySelector('.ws-runs-panel').open = true`);
  if (await page.evaluate(`getComputedStyle(document.getElementById('workspace-runs')).display`) === "none") throw new Error("Le esecuzioni sono nascoste nella vista Conversazione");
  await page.evaluate(`document.querySelector('[data-run-stop="smoke-active"]').click()`);
  await page.waitFor(`window.smokeStopped && !document.querySelector('[data-run-stop="smoke-active"]')`, 10000, "l’arresto dall’elenco delle esecuzioni");
  if (exceptions.length) throw new Error(`Eccezioni nel flusso delle attività: ${JSON.stringify(exceptions)}`);
  ok(CLAIMS[10], "Ferma cancella l’esecuzione dall’elenco anche nella vista Conversazione");
  await page.close();
}

async function exercisePermissionComposer(browser, url) {
  const page = await browser.newPage();
  const exceptions = [];
  page.on("Runtime.exceptionThrown", params => exceptions.push(params.exceptionDetails));
  await page.send("Runtime.enable");
  await page.send("Page.enable");
  await page.send("Page.addScriptToEvaluateOnNewDocument", {source: `
    const originalFetch = window.fetch.bind(window);
    window.permissionSaves = [];
    window.permissionCatalogReads = 0;
    window.losePermissionReply = false;
    let next = {model:'fixture-one',options:{},permission_profile:'limited'};
    window.fetch = async (input, options = {}) => {
      const path = new URL(typeof input === 'string' ? input : input.url, location.href).pathname;
      const method = options.method || 'GET';
      const json = body => new Response(JSON.stringify(body), {headers:{'Content-Type':'application/json'}});
      if (path === '/api/workspace/conversations' && method === 'GET') return json({available:true,conversations:[{id:'permission-fixture',title:'Permessi fixture',live:true,message_count:0}],closed_limit:20});
      if (path === '/api/workspace/conversations/permission-fixture') return json({available:true,provider_id:'fixture',conversation:{id:'permission-fixture',state:'ACTIVE'},session:{work:'IDLE',archive:'OPEN',connection:'CONNECTED',recovery:'RESUMABLE',pending_inputs:[]},next_turn:next,events:[],approvals:[],runs:[]});
      if (path === '/api/workspace/conversations/permission-fixture/model-choice') { window.permissionCatalogReads++; return json({available:true,provider_id:'fixture',model:next.model,options:next.options,permission_profile:next.permission_profile,models:[{id:'fixture-one',label:'Modello uno'},{id:'fixture-two',label:'Modello due'}],permission_profiles:[{id:'limited',label:'Solo progetto',description:'Limiti del provider'},{id:'wide',label:'Ampia autonomia',description:'Accesso fuori cartella'}],capabilities:[]}); }
      if (path === '/api/workspace/conversations/permission-fixture/next-turn' && method === 'PUT') {
        const body = JSON.parse(options.body);
        window.permissionSaves.push(body);
        if (window.permissionSaves.length === 1) await new Promise(resolve => { window.releasePermissionSave = resolve; });
        next = {model:body.model,options:body.model_options,permission_profile:body.permission_profile};
        if (window.losePermissionReply) { window.losePermissionReply = false; throw new Error('Risposta persa dopo il salvataggio'); }
        return json({next_turn:next});
      }
      if (path === '/api/workspace/activities') return json({items:[]});
      if (path === '/api/workspace/runs') return json({runs:[]});
      return originalFetch(input, options);
    };
  `});
  await page.send("Page.navigate", {url});
  await page.once("Page.loadEventFired", 30000, "la pagina della selezione permessi");
  await page.waitFor(`!!document.querySelector('[data-conversation-pill="permissions"]')`, 10000, "i permessi dichiarati dal provider");
  await page.evaluate(`document.querySelector('[data-conversation-pill="permissions"]').click()`);
  await page.waitFor(`!document.querySelector('[data-conversation-pop="permissions"]').hidden`, 10000, "il menu dei permessi");
  const labels = await page.evaluate(`Array.from(document.querySelectorAll('[data-conversation-permission-choice]')).map(b=>b.textContent)`);
  if (labels.length !== 2 || !labels[1].includes("Accesso fuori cartella")) throw new Error(`Il menu non rispetta il catalogo: ${JSON.stringify(labels)}`);
  await page.evaluate(`document.querySelector('[data-conversation-permission-choice="wide"]').click()`);
  await page.waitFor(`window.permissionSaves.length === 1`, 10000, "il primo salvataggio dei permessi");
  await page.evaluate(`document.querySelector('[data-conversation-pill="model"]').click(); document.querySelector('[data-conversation-model-choice="fixture-two"]').click()`);
  if (await page.evaluate(`window.permissionSaves.length`) !== 1) throw new Error("I salvataggi del prossimo turno sono concorrenti");
  await page.evaluate(`window.releasePermissionSave()`);
  await page.waitFor(`window.permissionSaves.length === 2`, 10000, "il salvataggio successivo del modello");
  const saves = await page.evaluate(`window.permissionSaves`);
  if (saves[0].permission_profile !== "wide" || saves[1].permission_profile !== "wide" || saves[1].model !== "fixture-two") throw new Error(`Cambio modello ha perso i permessi: ${JSON.stringify(saves)}`);
  const readsBeforeFailure = await page.evaluate(`window.permissionCatalogReads`);
  await page.evaluate(`window.losePermissionReply = true; document.querySelector('[data-conversation-pill="permissions"]').click(); document.querySelector('[data-conversation-permission-choice="limited"]').click()`);
  await page.waitFor(`window.permissionSaves.length === 3 && window.permissionCatalogReads > ${readsBeforeFailure} && document.querySelector('[data-conversation-pill="permissions"]').textContent.includes('Solo progetto')`, 10000, "i permessi riletti dopo una risposta persa");
  if (exceptions.length) throw new Error(`Eccezioni nel selettore dei permessi: ${JSON.stringify(exceptions)}`);
  ok(CLAIMS[11], "Permessi mostra solo il catalogo del provider, salva la scelta e la conserva cambiando modello con richieste ordinate");
  await page.close();
}

async function seedDocumentation(sandboxDir) {
  const root = path.join(sandboxDir, 'docs', 'wiki');
  await fs.mkdir(path.join(root, 'engineering'), {recursive:true});
  const page = (title, body) => `---\ntype: overview\ntitle: ${title}\ndescription: Documentazione di prova\nstatus: generated\n---\n${body}\n`;
  await fs.writeFile(path.join(root,'overview.md'), page('Panoramica Wiki', '# Panoramica Wiki\n\n[Codice](engineering/code-map.md#dettagli)\n\n<script>window.documentationUnsafe = true</script>\n<img src=x onerror="window.documentationUnsafe=true">\n<a href="javascript:window.documentationUnsafe=true">Inerte</a>\n<div onclick="window.documentationUnsafe=true">Testo innocuo</div>'));
  await fs.writeFile(path.join(root,'engineering','code-map.md'), page('Mappa del codice', '# Mappa del codice\n\n## Dettagli\n\nUnicocontenuto della mappa.\n\n[Panoramica](/overview.md)\n\n[Mancante](missing.md)\n\n' + 'Paragrafo di prova.\n\n'.repeat(80)));
}

async function exerciseDocumentation(browser, url, runDir) {
  const page = await browser.newPage();
  const exceptions = [];
  page.on('Runtime.exceptionThrown', params => exceptions.push(params.exceptionDetails));
  await page.send('Runtime.enable');
  await page.send('Page.enable');
  await page.send('Page.navigate', {url});
  await page.once('Page.loadEventFired',30000,'pagina Documentazione');
  await page.waitFor(`document.querySelector('[data-shell-view="documentation"]') && document.querySelector('[data-conversation-id="${CONVERSATION_ID}"]')`,20000,'terza vista e conversazione');
  await page.evaluate(`document.querySelector('[data-conversation-id="${CONVERSATION_ID}"]').click()`);
  await page.waitFor(`document.getElementById('workspace-conversation').textContent.includes('${AGENT_SENTINEL}')`,10000,'cronologia della conversazione');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Panoramica Wiki'`,10000,'lettura della Wiki');
  const safe = await page.evaluate(`!window.documentationUnsafe && !document.querySelector('#docs-content script, #docs-content img, #docs-content [onclick], #docs-content a[href^="javascript:"]')`);
  if (!safe) throw new Error('Markdown attivo nel lettore');
  await page.evaluate(`document.querySelector('[data-shell-view="conversation"]').click()`);
  if (!await page.evaluate(`document.getElementById('workspace-conversation').textContent.includes('${AGENT_SENTINEL}')`)) throw new Error('Cambio vista perde la cronologia');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  const screenshot = await page.send('Page.captureScreenshot', {format:'png'});
  await fs.writeFile(path.join(runDir,'documentation-desktop.png'),Buffer.from(screenshot.data,'base64'));
  await page.evaluate(`(() => {const search=document.getElementById('docs-search'); search.value='UNICOCONTENUTO';search.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  await page.waitFor(`document.querySelectorAll('#docs-index [data-doc-page]').length === 1 && document.querySelector('#docs-index [data-doc-page]').dataset.docPage === 'engineering/code-map'`,10000,'ricerca nel corpo del documento');
  await page.evaluate(`(() => {const search=document.getElementById('docs-search');search.value='';search.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  await page.waitFor(`document.querySelectorAll('#docs-index [data-doc-page]').length === 2`,10000,'indice completo');
  await page.evaluate(`document.querySelector('#docs-content a[data-doc-page="engineering/code-map"]').click()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Mappa del codice' && location.hash === '#dettagli'`,10000,'link relativo con ancora');
  await page.evaluate(`history.back()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Panoramica Wiki'`,10000,'Indietro alla panoramica');
  await page.evaluate(`history.forward()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Mappa del codice'`,10000,'Avanti alla mappa');
  await page.evaluate(`document.querySelector('#docs-content a[data-doc-page="overview"]').click()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Panoramica Wiki'`,10000,'link dalla radice Wiki');
  await page.evaluate(`history.back()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Mappa del codice'`,10000,'ritorno alla mappa');
  await page.send('Page.reload', {ignoreCache:true});
  await page.once('Page.loadEventFired',30000,'ricaricamento del link diretto');
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Mappa del codice' && document.querySelector('[data-shell-view="documentation"]').getAttribute('aria-selected') === 'true'`,15000,'documento dal link diretto');
  ok(CLAIMS[12], 'ricerca nel contenuto, link relativi e assoluti, Indietro/Avanti e ricaricamento funzionano sulla Wiki reale');
  await page.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:false});
  await page.waitFor(`getComputedStyle(document.getElementById('workspace-documentation')).display === 'grid' && document.getElementById('docs-content').clientHeight > 200`,10000,'lettore in finestra stretta');
  if (await page.evaluate(`document.documentElement.scrollWidth > innerWidth`)) throw new Error('Overflow orizzontale della documentazione mobile');
  await page.evaluate(`document.getElementById('docs-index-toggle').click()`);
  await page.waitFor(`document.getElementById('docs-index-toggle').getAttribute('aria-expanded') === 'true' && getComputedStyle(document.getElementById('docs-index')).display !== 'none'`,10000,'indice espandibile su telefono');
  await page.evaluate(`document.getElementById('docs-index-toggle').click();document.getElementById('docs-content').scrollTop=0;`);
  const mobile = await page.send('Page.captureScreenshot', {format:'png'});
  await fs.writeFile(path.join(runDir,'documentation-mobile.png'),Buffer.from(mobile.data,'base64'));
  await page.evaluate(`document.querySelector('#docs-content a[data-doc-page="engineering/missing"]').click()`);
  await page.waitFor(`document.getElementById('docs-content').textContent.includes('Pagina non trovata')`,10000,'pagina mancante distinta da Wiki vuota');
  ok(CLAIMS[14], 'la lettura resta accessibile su 390px e i link mancanti mostrano un errore esplicito');
  if (exceptions.length) throw new Error(`Eccezioni in Documentazione: ${JSON.stringify(exceptions)}`);
  await page.close();
}

// Hold a real response to prove a late read cannot navigate over a newer choice.
async function exerciseDocumentationRace(browser, url) {
  const page = await browser.newPage();
  await page.send('Runtime.enable');
  await page.send('Page.enable');
  await page.send('Page.addScriptToEvaluateOnNewDocument', {source: `
    const originalFetch = window.fetch.bind(window);
    let hold = true;
    window.fetch = async (input, options) => {
      const response = await originalFetch(input, options);
      if (hold && new URL(typeof input === 'string' ? input : input.url, location.href).pathname === '/api/documentation') {
        hold = false;
        await new Promise(resolve => { window.releaseDocumentation = resolve; });
      }
      return response;
    };
  `});
  await page.send('Page.navigate', {url});
  await page.once('Page.loadEventFired',30000,'pagina per la risposta ritardata');
  await page.waitFor(`document.querySelector('[data-shell-view="documentation"]')`,10000,'terza vista');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click(); document.querySelector('[data-shell-view="conversation"]').click();`);
  await page.waitFor(`typeof window.releaseDocumentation === 'function'`,10000,'risposta Wiki in attesa');
  await page.evaluate(`window.releaseDocumentation()`);
  await page.waitFor(`document.querySelector('#docs-content h1')?.textContent === 'Panoramica Wiki'`,10000,'lettura completata dietro la conversazione');
  if (!await page.evaluate(`document.getElementById('workspace-shell').dataset.view === 'conversation' && !new URL(location.href).searchParams.has('page')`)) throw new Error('La lettura ritardata ha sovrascritto la navigazione');
  // A spec held behind Documentation is restored by Back, then Board leaves it.
  await page.evaluate(`document.querySelector('[data-shell-view="board"]').click()`);
  await page.waitFor(`document.querySelector('.card[data-code="US-701"]')`,10000,'card della board');
  await page.evaluate(`document.querySelector('.card[data-code="US-701"]').click()`);
  await page.waitFor(`new URL(location.href).searchParams.get('spec') === 'US-701'`,10000,'spec nel dettaglio');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click(); history.back();`);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view === 'spec'`,10000,'Indietro dalla Wiki alla stessa spec');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click(); document.querySelector('[data-shell-view="board"]').click();`);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view === 'board' && !new URL(location.href).searchParams.has('spec')`,10000,'board dalla documentazione con una spec nascosta');
  console.log('-> ok: le risposte ritardate preservano la navigazione e la documentazione convive con la cronologia del dettaglio spec');
  await page.close();
}

async function exerciseEmptyWorkspace(browser, url) {
  const page = await browser.newPage();
  await page.send("Runtime.enable");
  await page.send("Page.enable");
  await page.send("Page.navigate", { url });
  await page.once("Page.loadEventFired", 30000, "il caricamento del workspace vuoto");
  await page.waitFor(`document.getElementById('new-spec-btn').title.includes('epica') && !document.getElementById('new-spec-btn').disabled`,
    20000, "Nuova spec raggiungibile senza epiche");
  await page.evaluate(`document.getElementById('new-spec-btn').click()`);
  await page.waitFor(`!document.getElementById('new-spec-modal').classList.contains('hidden')
    && !document.getElementById('new-spec-no-epics').classList.contains('hidden')
    && !document.getElementById('new-spec-submit').disabled
    && !document.getElementById('spec-draft-mode-assisted').disabled`, 10000,
    "il form permette di definire la prima epica senza backlog generato");
  const body = "**User Story**\nCome autore voglio creare la prima storia.\n\n**Dimostrazione**\nLa storia compare in TODO.\n\n**Criteri di accettazione**\n- [ ] AC-1 — La prima storia è persistita.\n";
  await page.evaluate(`(() => {
    const form = document.getElementById('new-spec-form');
    form.elements.title.value = 'Prima storia senza backlog';
    form.elements.epic_title.value = 'Primo obiettivo';
    document.querySelector('#new-spec-modal .CodeMirror').CodeMirror.setValue(${JSON.stringify(body)});
    document.getElementById('new-spec-submit').click();
  })()`);
  await page.waitFor(`document.getElementById('new-spec-modal').classList.contains('hidden')`, 10000,
    "la prima storia creata senza epiche", () => page.evaluate(`document.getElementById('new-spec-status').textContent`));
  const created = await (await fetch(`${url}/api/spec/US-001`)).json();
  if (created.spec?.epic?.code !== 'EP-001' || created.spec?.epic?.title !== 'Primo obiettivo') {
    throw new Error('La prima storia non ha persistito la nuova epica');
  }
  ok(CLAIMS[8], "la conferma crea US-001 e la prima epica EP-001 in un workspace appena inizializzato");
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.getElementById('docs-content').textContent.includes('non è ancora stata generata')`, 10000, "Wiki assente distinta da un errore");
  await page.close();
}

// --- The workspace the page is shown ----------------------------------------

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: "US-701",
        title: "Smoke della pagina del viewer",
        epic: { code: "EP-700", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di appoggio: serve un epic perché «Nuova spec» percorra il ramo lungo.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

// The rail needs a conversation to list, and a conversation is a file: the
// journal writes one record per conversation under
// `.archetipo/conversations/<id>.json`, and the listing route reads that
// directory and nothing else. Writing the record directly is what lets this
// smoke stay credential-free and agent-free while still exercising the real
// read path — the same one a conversation held yesterday would go through.
async function seedConversationRecord(sandboxDir) {
  const dir = path.join(sandboxDir, ".archetipo", "conversations");
  await fs.mkdir(dir, { recursive: true, mode: 0o700 });
  const openedAt = new Date(Date.now() - 3600_000).toISOString();
  const record = {
    id: CONVERSATION_ID,
    spec_code: "",
    title: CONVERSATION_TITLE,
    working_dir: sandboxDir,
    provider_id: "claude",
    opened_at: openedAt,
    last_message_at: openedAt,
    message_count: 2,
    resumed_from: "",
    // Sealed: this is history, so the viewer reads it from the store instead of
    // looking for a process that is holding it.
    final_state: "closed",
    events: [
      { id: 1, seq: 1, at: openedAt, kind: "user_message", text: HUMAN_SENTINEL },
      { id: 2, seq: 2, at: openedAt, kind: "text", text: AGENT_SENTINEL },
    ],
  };
  await fs.writeFile(path.join(dir, `${CONVERSATION_ID}.json`), `${JSON.stringify(record, null, 2)}\n`, {
    mode: 0o600,
  });
}

// --- Chrome, over the DevTools protocol -------------------------------------

// Where a Chrome may be. An explicit path always wins, so a machine with the
// browser somewhere unusual needs no change here.
function chromeCandidates(explicit) {
  if (explicit) return [explicit];
  const fromEnv = process.env.ARCHETIPO_CHROME || process.env.CHROME_PATH;
  const candidates = fromEnv ? [fromEnv] : [];
  if (process.platform === "darwin") {
    candidates.push(
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
      "/Applications/Chromium.app/Contents/MacOS/Chromium",
      "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
      path.join(os.homedir(), "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
    );
  } else if (process.platform === "win32") {
    const programFiles = [process.env["PROGRAMFILES"], process.env["PROGRAMFILES(X86)"], process.env.LOCALAPPDATA];
    for (const base of programFiles) {
      if (base) candidates.push(path.join(base, "Google", "Chrome", "Application", "chrome.exe"));
    }
  } else {
    candidates.push(
      "/usr/bin/google-chrome",
      "/usr/bin/google-chrome-stable",
      "/usr/bin/chromium",
      "/usr/bin/chromium-browser",
      "/snap/bin/chromium",
    );
  }
  return candidates;
}

export async function findChrome(explicit) {
  for (const candidate of chromeCandidates(explicit)) {
    try {
      await fs.access(candidate, fs.constants.X_OK);
      return candidate;
    } catch {
      // keep looking
    }
  }
  return null;
}

export async function launchChrome(chromePath, userDataDir) {
  await fs.mkdir(userDataDir, { recursive: true });
  const child = spawn(
    chromePath,
    [
      "--headless=new",
      "--disable-gpu",
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-background-networking",
      "--disable-component-update",
      "--disable-extensions",
      "--disable-sync",
      // Wide enough to be above the shell's narrow breakpoint (900px), so the
      // page under test is the one with the rail beside the conversation.
      "--window-size=1400,900",
      // Port 0: the browser picks a free one and says which on stderr, so two
      // runs of this smoke never fight over a fixed debugging port.
      "--remote-debugging-port=0",
      `--user-data-dir=${userDataDir}`,
      "about:blank",
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );

  let stderr = "";
  const endpoint = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`Chrome non ha annunciato il suo endpoint DevTools entro 20s\nSTDERR:\n${stderr}`));
    }, 20000);
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString("utf8");
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timer);
        resolve(match[1]);
      }
    });
    child.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`Chrome è terminato con codice ${code} prima di aprire DevTools\nSTDERR:\n${stderr}`));
    });
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
  });

  const httpBase = `http://${new URL(endpoint).host}`;
  return {
    wsEndpoint: endpoint,
    async newPage() {
      // `/json/new` wants a PUT since Chrome 111.
      const response = await fetch(`${httpBase}/json/new?${encodeURIComponent("about:blank")}`, { method: "PUT" });
      if (!response.ok) {
        throw new Error(`Chrome non ha aperto una scheda: HTTP ${response.status} ${await response.text()}`);
      }
      const target = await response.json();
      return connectPage(target, httpBase);
    },
    async close() {
      await stopProcess(child);
    },
  };
}

// A minimal CDP client: the request/response correlation by id, the event
// listeners, and the two conveniences the assertions above are written in terms
// of — `evaluate` and `waitFor`.
async function connectPage(target, httpBase) {
  const socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    socket.addEventListener("open", resolve, { once: true });
    socket.addEventListener("error", () => reject(new Error("connessione DevTools rifiutata")), { once: true });
  });

  let nextId = 1;
  const pending = new Map();
  const listeners = new Map();

  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (message.id !== undefined) {
      const entry = pending.get(message.id);
      if (!entry) return;
      pending.delete(message.id);
      if (message.error) entry.reject(new Error(`${entry.method}: ${JSON.stringify(message.error)}`));
      else entry.resolve(message.result);
      return;
    }
    for (const listener of listeners.get(message.method) || []) {
      listener(message.params || {});
    }
  });

  function send(method, params) {
    const id = nextId++;
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject, method });
      socket.send(JSON.stringify({ id, method, params: params || {} }));
    });
  }

  function on(method, listener) {
    if (!listeners.has(method)) listeners.set(method, []);
    listeners.get(method).push(listener);
  }

  function once(method, timeoutMs, what) {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Timeout di ${timeoutMs}ms aspettando ${what}`)), timeoutMs);
      on(method, (params) => {
        clearTimeout(timer);
        resolve(params);
      });
    });
  }

  // An expression is evaluated in the page and its value comes back by value. An
  // exception *inside the evaluation* is raised here rather than swallowed: a
  // probe that silently returned undefined would turn a broken page into a
  // confusing timeout.
  async function evaluate(expression) {
    const result = await send("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    });
    if (result.exceptionDetails) {
      const details = result.exceptionDetails;
      const description = (details.exception && details.exception.description) || details.text;
      throw new Error(`La pagina ha rifiutato la sonda: ${description}\nEspressione: ${expression}`);
    }
    return result.result.value;
  }

  // Polls the page until the condition holds. On timeout it says what it was
  // waiting for and, through `diagnose`, what was on screen instead — which is
  // the difference between a useful failure and "timed out".
  async function waitFor(expression, timeoutMs, what, diagnose) {
    const deadline = Date.now() + timeoutMs;
    let last = null;
    while (Date.now() < deadline) {
      if (await evaluate(expression)) return;
      if (diagnose) last = await diagnose();
      await delay(100);
    }
    const seen = last === null ? "" : `\nSullo schermo: ${JSON.stringify(last, null, 2)}`;
    throw new Error(`Timeout di ${timeoutMs}ms aspettando che ${what}.${seen}`);
  }

  return {
    send,
    on,
    once,
    evaluate,
    waitFor,
    async close() {
      try {
        await fetch(`${httpBase}/json/close/${target.id}`);
      } catch {
        // the browser is going away anyway
      }
      socket.close();
    },
  };
}

// --- The missing-browser decision, stated out loud --------------------------

function reportMissingBrowser(allowed, explicit) {
  const looked = chromeCandidates(explicit).join("\n  ");
  const banner = [
    "",
    "==============================================================================",
    allowed
      ? "SALTATO: nessun Chrome trovato, e il chiamante ha dichiarato che va bene."
      : "FAIL: nessun Chrome trovato su questa macchina.",
    "",
    "Questo smoke NON ha provato niente di quanto segue:",
    ...CLAIMS.map((claim) => `  ✗ ${claim}`),
    "",
    "Cercato in:",
    `  ${looked}`,
    "",
    allowed
      ? "Per provarlo davvero: installa Chrome, oppure indica quello che hai con\n  --chrome <percorso> o ARCHETIPO_CHROME=<percorso>."
      : "Se questa macchina non deve avere un browser, dichiaralo esplicitamente con\n  --allow-missing-browser (o ARCHETIPO_ALLOW_MISSING_BROWSER=1),\nche fa uscire questo smoke con 0 stampando comunque questo elenco.\nAltrimenti indica il tuo Chrome con --chrome <percorso> o ARCHETIPO_CHROME=<percorso>.",
    "==============================================================================",
    "",
  ].join("\n");
  console.log(banner);
  // Anche su stderr: un log che tiene solo il canale degli errori deve vedere
  // che qui non è stato provato niente.
  console.error(banner);
}

// --- Harness ----------------------------------------------------------------

function parseArgs(argv) {
  const options = {
    workspaceRoot: defaultWorkspaceRoot,
    cleanup: false,
    chromePath: "",
    allowMissingBrowser: process.env.ARCHETIPO_ALLOW_MISSING_BROWSER === "1",
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--workspace-root":
        options.workspaceRoot = path.resolve(argv[++i]);
        break;
      case "--chrome":
        options.chromePath = path.resolve(argv[++i]);
        break;
      case "--allow-missing-browser":
        options.allowMissingBrowser = true;
        break;
      case "--cleanup":
        options.cleanup = true;
        break;
      case "--help":
      case "-h":
        printHelp();
        process.exit(0);
        break;
      default:
        throw new Error(`Unknown argument: ${arg}`);
    }
  }

  return options;
}

function printHelp() {
  console.log(`Smoke test: la pagina di archetipo view si carica e i suoi comandi rispondono

Usage:
  node ./test/e2e/viewer-page-load-smoke.mjs
  npm run test:view-page-load-smoke

Options:
  --workspace-root <dir>    Parent directory for the generated sandbox
  --chrome <path>           Chrome/Chromium executable to drive
  --allow-missing-browser   Exit 0 (instead of 1) when no Chrome is found,
                            printing the list of claims left unproved
  --cleanup                 Remove the run directory after the test passes/fails

Environment:
  ARCHETIPO_CHROME / CHROME_PATH        same as --chrome
  ARCHETIPO_ALLOW_MISSING_BROWSER=1     same as --allow-missing-browser
`);
}

async function createRunDir(root) {
  return createRunDirShared(root, false);
}

async function buildCLI() {
  return buildCLIShared(cliPath, repoRoot, runCommand);
}

async function startViewServer(cwd, env) {
  return startViewServerShared(cliPath, cwd, env, "/api/board");
}


async function runCommand(label, command, args, options = {}) {
  console.log(`-> ${label}: ${command} ${args.join(" ")}`);
  const result = await new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env || process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => stderr.push(chunk));
    child.on("close", (code) =>
      resolve({
        code,
        stdout: Buffer.concat(stdout).toString("utf8"),
        stderr: Buffer.concat(stderr).toString("utf8"),
      }),
    );
    child.on("error", (error) => resolve({ code: 1, stdout: "", stderr: error.message }));
  });

  if (result.code !== 0) {
    throw new Error(`${label} failed with exit ${result.code}\nSTDOUT:\n${result.stdout}\nSTDERR:\n${result.stderr}`);
  }
  return result;
}

async function stopProcess(child) {
  if (!child || child.killed) return;
  if (process.platform === "win32") {
    await runCommand("taskkill", "taskkill", ["/PID", String(child.pid), "/T", "/F"]);
    return;
  }
  child.kill("SIGTERM");
  await Promise.race([new Promise((resolve) => child.once("exit", resolve)), delay(3000)]);
  if (!child.killed) {
    child.kill("SIGKILL");
  }
}

if (path.resolve(process.argv[1] || "") === __filename) {
  main().catch((error) => {
    console.error(`\nFAIL: ${error.message}`);
    process.exit(1);
  });
}
