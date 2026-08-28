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

    view = await startViewServer(sandboxDir, cliEnv);
    console.log(`-> view ready: ${view.url}`);

    browser = await launchChrome(chromePath, path.join(runDir, "chrome-profile"));
    console.log(`-> devtools: ${browser.wsEndpoint}`);

    await exercisePage(browser, view.url);
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

async function findChrome(explicit) {
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

async function launchChrome(chromePath, userDataDir) {
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
  await fs.mkdir(root, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(root, stamp);
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

async function buildCLI(env) {
  console.log(`-> building CLI: ${cliPath}`);
  await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
    cwd: path.join(repoRoot, "cli"),
    env,
  });
}

async function startViewServer(cwd, env) {
  const child = spawn(cliPath, ["view", "--host", "127.0.0.1", "--port", "0", "--no-open"], {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });

  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk) => {
    stdout += chunk.toString("utf8");
  });

  const ready = new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`view server did not become ready in time\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`));
    }, 15000);

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString("utf8");
      const match = stderr.match(/ARchetipo view ready at (http:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    });

    child.on("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`view server exited early with code ${code}\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`));
    });

    child.on("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });

  const url = await ready;
  await waitForHTTP(`${url}/api/board`);
  return { child, url };
}

async function waitForHTTP(url) {
  const started = Date.now();
  while (Date.now() - started < 10000) {
    try {
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      if (response.ok) return;
    } catch {
      // keep polling
    }
    await delay(200);
  }
  throw new Error(`Timed out waiting for ${url}`);
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

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
