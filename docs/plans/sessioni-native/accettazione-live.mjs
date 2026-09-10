#!/usr/bin/env node
// Prova di accettazione end-to-end del task 12, con l'harness vero.
//
// Non è uno smoke della suite e non entra in `npm test`: gli smoke di
// `test/e2e/` provano l'adapter con un finto agente e restano credential-free,
// mentre questo script chiede al runtime reale ciò che soltanto il runtime può
// dimostrare — che un file venga scritto davvero, che il modello scelto per il
// prossimo turno arrivi davvero, che la memoria nativa sopravviva davvero al
// riavvio di View. Vive accanto al report che cita le sue osservazioni.
//
// Uso: node docs/plans/sessioni-native/accettazione-live.mjs [--keep]
// Richiede: Go, Node 22+, `claude` nel PATH e con credenziali valide.
//
// Ogni attesa interroga una rotta del viewer con un timeout che dice cosa
// aspettava e cosa ha trovato: nessuno sleep arbitrario. Il sandbox è una
// directory temporanea e viene rimossa alla fine, salvo `--keep`.

import { spawn } from "node:child_process";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const keep = process.argv.includes("--keep");
const proved = [];
const observations = {};

function ok(claim, detail) {
  proved.push(claim);
  console.log(`OK  ${claim}${detail ? ` — ${detail}` : ""}`);
}

function record(key, value) {
  observations[key] = value;
  console.log(`    ${key} = ${typeof value === "string" ? value : JSON.stringify(value)}`);
}

async function main() {
  const sandbox = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-accettazione-"));
  const cliPath = path.join(sandbox, "archetipo");
  let view = null;
  try {
    await run("build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], { cwd: path.join(repoRoot, "cli") });
    const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, ARCHETIPO_STATE_DIR: path.join(sandbox, "state") };
    const project = path.join(sandbox, "workspace");
    await fs.mkdir(project, { recursive: true });
    await run("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: project, env });
    await seedBacklog(project, cliPath, env);

    view = await startView(project, cliPath, env);
    record("viewer_pid", view.child.pid);
    await api(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      body: JSON.stringify({ id: "claude", config: { command: "claude", model: "haiku", permission_mode: "bypassPermissions" } }),
    });

    // --- 1. conversazione libera che agisce sul filesystem ------------------
    const opened = await api(`${view.url}/api/workspace/conversations`, { method: "POST", body: "{}" }, 201);
    const id = opened.conversation?.id;
    if (!id) throw new Error(`la conversazione non ha un id: ${JSON.stringify(opened)}`);
    record("conversation_id", id);
    const nativeFirst = await nativeSessionOf(view.url, id);
    record("native_session_id", nativeFirst);
    if (!nativeFirst) throw new Error("la conversazione non ha un riferimento nativo");

    await api(`${view.url}/api/workspace/conversations/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ message: "Crea nella directory corrente il file PROVA.txt contenente esattamente la riga NATIVE_VIEW_OK e non scrivere altri file. Poi rispondi solo 'fatto'." }),
    }, 202);
    await waitIdle(view.url, id, "il primo turno");
    const written = await fs.readFile(path.join(project, "PROVA.txt"), "utf8");
    if (!written.includes("NATIVE_VIEW_OK")) {
      throw new Error(`il file scritto dall'agente non contiene il marcatore: ${JSON.stringify(written)}`);
    }
    ok("AC-1 conversazione libera con tool reali", `PROVA.txt scritto nel workspace, ${JSON.stringify(written.trim())}`);

    // --- 2. catalogo di modelli e skill del runtime della sessione ---------
    const choice = await api(`${view.url}/api/workspace/conversations/${id}/model-choice`);
    const skillNames = (choice.skills || []).map((s) => s.name);
    record("skills_known", choice.skills_known);
    record("skills", skillNames);
    record("capabilities", choice.capabilities);
    if (!skillNames.some((n) => n.includes("archetipo-plan"))) {
      throw new Error(`il catalogo della sessione non dichiara le skill installate: ${JSON.stringify(skillNames)}`);
    }
    ok("AC-2 skill dell'harness scoperte dalla sessione", `${skillNames.length} skill, fra cui archetipo-plan`);

    // --- 3. il modello scelto vale dal turno successivo --------------------
    const appliedFirst = await appliedModelOf(view.url, id);
    record("applied_model_turn_1", appliedFirst);
    await api(`${view.url}/api/workspace/conversations/${id}/next-turn`, {
      method: "PUT",
      body: JSON.stringify({ model: "sonnet", model_options: { effort: "low" } }),
    });
    await api(`${view.url}/api/workspace/conversations/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ message: "Rispondi soltanto con la parola SECONDO." }),
    }, 202);
    await waitIdle(view.url, id, "il turno con il modello cambiato");
    const appliedSecond = await appliedModelOf(view.url, id);
    record("applied_model_turn_2", appliedSecond);
    // Applied porta il model *osservato* dal runtime, non la stringa richiesta:
    // `haiku` diventa `claude-haiku-4-5-…`. È esattamente ciò che rende questa
    // riga una prova — la conferma arriva dal processo e non dalla richiesta.
    if (!appliedFirst.includes("haiku") || !appliedSecond.includes("sonnet")) {
      throw new Error(`la scelta per turno non ha raggiunto il runtime: ${appliedFirst} -> ${appliedSecond}`);
    }
    const configAfter = await fs.readFile(path.join(project, ".archetipo", "config.yaml"), "utf8");
    if (!/model:\s*haiku/.test(configAfter)) {
      throw new Error("la scelta di un turno ha spostato il default del workspace");
    }
    ok("AC-3 modello per turno", `${appliedFirst} -> ${appliedSecond}, default del workspace ancora haiku`);

    // --- 4. un'azione ARchetipo dentro lo stesso thread --------------------
    const plan = await api(`${view.url}/api/spec/US-001/execution`, {
      method: "POST",
      body: JSON.stringify({ action: "plan", conversation_id: id }),
    }, 201);
    record("execution_id", plan.id);
    const settled = await waitFor(
      async () => {
        const rec = await api(`${view.url}/api/execution/${plan.id}`);
        return rec.status !== "RUNNING" ? rec : null;
      },
      "l'azione plan chiusa",
      15 * 60 * 1000,
    );
    record("plan_status", settled.status);
    const nativeAfterAction = await nativeSessionOf(view.url, id);
    if (nativeAfterAction !== nativeFirst) {
      throw new Error(`l'azione ha spostato la conversazione da ${nativeFirst} a ${nativeAfterAction}`);
    }
    // L'oracolo del piano è quello che il viewer rilegge dal workspace, non un
    // percorso indovinato qui: dove il piano finisca lo decide `paths.planning`
    // della configurazione, e un test che lo riscrive assumerebbe il default.
    const spec = await api(`${view.url}/api/spec/US-001`);
    record("spec_status", spec.spec?.status);
    record("plan_tasks", (spec.tasks || []).length);
    record("plan_body_chars", (spec.plan_body || "").length);
    const planning = await fs.readdir(path.join(project, ".archetipo", "plans")).catch(() => []);
    record("planning_dir", planning);
    if (settled.status === "SUCCEEDED" && spec.spec?.status !== "PLANNED") {
      throw new Error(`l'azione è SUCCEEDED ma la spec è ${spec.spec?.status}`);
    }
    const conversationAfter = await api(`${view.url}/api/workspace/conversations/${id}`);
    record("conversation_state_after_action", conversationAfter.conversation?.state);
    ok(
      "AC-4 azione di processo nella stessa sessione nativa",
      `execution ${plan.id} ${settled.status}, stesso native id, conversazione ancora aperta`,
    );

    // --- 5. spegnimento di View e ripresa della stessa sessione ------------
    await stop(view.child);
    view = await startView(project, cliPath, env);
    record("viewer_pid_after_restart", view.child.pid);
    const restored = await api(`${view.url}/api/workspace/conversations/${id}`);
    const nativeAfterRestart = restored.session?.session?.native?.id || "";
    if (nativeAfterRestart !== nativeFirst) {
      throw new Error(`dopo il restart il riferimento nativo è ${nativeAfterRestart}, non ${nativeFirst}`);
    }
    await api(`${view.url}/api/workspace/conversations/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ message: "Senza rileggere alcun file: qual è esattamente il contenuto che hai scritto in PROVA.txt all'inizio di questa conversazione? Rispondi con quella sola riga." }),
    }, 202);
    await waitIdle(view.url, id, "il turno dopo il restart");
    const history = await api(`${view.url}/api/workspace/conversations/${id}?after_id=0`);
    const texts = (history.events || []).filter((e) => e.kind === "text").map((e) => e.text || "").join("\n");
    const remembered = texts.includes("NATIVE_VIEW_OK");
    record("native_memory_after_restart", remembered);
    if (!remembered) {
      throw new Error("la sessione ripresa non ricorda ciò che aveva scritto prima del restart");
    }
    ok("AC-5 ripresa dopo il riavvio di View", `stessa conversazione, stesso ${nativeFirst}, memoria nativa conservata`);

    // --- 6. rilascio esplicito, il record resta ---------------------------
    const closed = await api(`${view.url}/api/workspace/conversations/${id}`, { method: "DELETE" });
    record("state_after_release", `${closed.conversation?.state} / ${closed.session?.connection}`);
    const recordFile = path.join(project, ".archetipo", "conversations", `${id}.json`);
    const persisted = JSON.parse(await fs.readFile(recordFile, "utf8"));
    if (persisted.session?.native?.id !== nativeFirst) {
      throw new Error("il rilascio ha perso il riferimento nativo");
    }
    ok("AC-6 rilascio senza distruzione del contesto", `record ancora su disco con native id ${nativeFirst}`);

    console.log(`\nPASS: accettazione live completata (${proved.length} affermazioni provate).`);
    console.log(JSON.stringify(observations, null, 2));
  } finally {
    if (view) await stop(view.child).catch(() => {});
    if (keep) console.log(`sandbox conservato: ${sandbox}`);
    else await fs.rm(sandbox, { recursive: true, force: true });
  }
}

// --- helper -----------------------------------------------------------------

async function seedBacklog(project, cliPath, env) {
  const file = path.join(project, ".archetipo", "specs-input.json");
  const specs = [{
    code: "US-001",
    title: "Saluto iniziale",
    epic: { code: "EP-001", title: "Prove di accettazione" },
    priority: "HIGH",
    points: 2,
    status: "TODO",
    body: [
      "**User Story**",
      "Come persona, voglio un saluto scritto in un file, per verificare la sessione nativa.",
      "",
      "**Criteri di accettazione**",
      "- [ ] AC-1 — Esiste un file SALUTO.txt con dentro una riga di saluto.",
      "",
    ].join("\n"),
  }];
  await fs.writeFile(file, `${JSON.stringify({ specs }, null, 2)}\n`);
  await run("spec add", cliPath, ["spec", "add", "--file", file], { cwd: project, env });
  await fs.rm(file, { force: true });
}

async function startView(project, cliPath, env) {
  const child = spawn(cliPath, ["view", "--no-open"], { cwd: project, env, stdio: ["ignore", "pipe", "pipe"] });
  let out = "";
  const url = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`il viewer non ha annunciato una porta:\n${out}`)), 60_000);
    const scan = (chunk) => {
      out += chunk.toString();
      const match = out.match(/http:\/\/127\.0\.0\.1:(\d+)/);
      if (match) {
        clearTimeout(timer);
        resolve(match[0]);
      }
    };
    child.stdout.on("data", scan);
    child.stderr.on("data", scan);
    child.once("error", reject);
  });
  await waitFor(async () => (await fetch(`${url}/api/board`).then((r) => r.ok, () => false)) || null, "il viewer pronto", 30_000);
  return { child, url };
}

async function stop(child) {
  if (!child || child.exitCode !== null) return;
  const ended = new Promise((resolve) => child.once("exit", resolve));
  child.kill("SIGTERM");
  const killer = setTimeout(() => child.kill("SIGKILL"), 5_000);
  await ended;
  clearTimeout(killer);
}

async function api(url, init = {}, expected = 200) {
  const response = await fetch(url, {
    ...init,
    headers: { "content-type": "application/json", ...(init.headers || {}) },
  });
  const text = await response.text();
  if (response.status !== expected) {
    throw new Error(`${init.method || "GET"} ${url} ha risposto ${response.status}, atteso ${expected}: ${text}`);
  }
  return text ? JSON.parse(text) : {};
}

async function nativeSessionOf(viewURL, id) {
  const view = await api(`${viewURL}/api/workspace/conversations/${id}`);
  return view.session?.session?.native?.id || "";
}

async function appliedModelOf(viewURL, id) {
  const view = await api(`${viewURL}/api/workspace/conversations/${id}`);
  return view.session?.current_turn?.applied?.model || "";
}

async function waitIdle(viewURL, id, what) {
  await waitFor(async () => {
    const view = await api(`${viewURL}/api/workspace/conversations/${id}`);
    const work = view.session?.work;
    return work && work === "IDLE" ? view : null;
  }, what, 15 * 60 * 1000);
}

async function waitFor(probe, what, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = "niente";
  for (;;) {
    try {
      const value = await probe();
      if (value) return value;
    } catch (err) {
      last = err.message;
    }
    if (Date.now() > deadline) throw new Error(`timeout aspettando ${what}; ultima osservazione: ${last}`);
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
}

function run(label, command, args, options) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { ...options, stdio: ["ignore", "pipe", "pipe"] });
    let out = "";
    child.stdout.on("data", (c) => (out += c));
    child.stderr.on("data", (c) => (out += c));
    child.once("error", reject);
    child.once("exit", (code) => (code === 0 ? resolve(out) : reject(new Error(`${label} è uscito con ${code}:\n${out}`))));
  });
}

main().catch((err) => {
  console.error(`FAIL: ${err.message}`);
  process.exitCode = 1;
});
