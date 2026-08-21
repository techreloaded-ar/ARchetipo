#!/usr/bin/env node

// End-to-end smoke for "see the spec and the conversation together, and know
// what the workspace is running" (US-055).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector, the `claude` provider with its
// stream-json client and its local session, the `arcipelago` provider with its
// SSE consumer and the server-side run follower, the route this story adds
// (`GET /api/workspace/runs`) and the assets the browser is really served
// (`GET /index.html`, `GET /app.css`). Two things are replaced, and only two:
// the ARcipelago hub, by a local Node server bound to 127.0.0.1, and the agent
// binary, by `support/fake-claude.mjs`, which speaks the same protocol on
// stdio. So nothing here needs a credential or leaves the loopback interface.
//
// Two providers appear because the story needs both and neither can stand in
// for the other: a run of workspace scope exists only under a provider that
// declares the workspace capabilities (`claude`), and a run waiting for a human
// decision exists only under a provider that has approvals at all (`arcipelago`
// — a local session runs with approvals disabled and never asks anything). The
// workspace default is moved from one to the other through the very route the
// Execution panel uses, exactly as a person would.
//
// The fact AC-4 rests on is asserted by counting, not by claiming: every viewer
// request this file makes goes through one recorder, and the assertion is that
// the list contains **zero** calls to `GET /api/execution/{id}/run` for the
// execution whose wait is observed. Learning that a run is blocked must not
// require having opened it.
//
// Nothing progresses on its own and nothing sleeps for an outcome: every frame
// is emitted by this test, the hub opens and closes the approval by hand, and
// each wait polls a viewer route with an explicit timeout that names what was
// expected and what arrived instead.

import fs from "node:fs/promises";
import { readFileSync } from "node:fs";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const assetsDir = path.join(repoRoot, "cli", "internal", "web", "assets");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "workspace-runs-view-smoke");

// The credential is a sentinel: the arcipelago provider must send it to the hub
// (which checks it, on the event stream too) and the viewer must never echo it
// back to the browser.
const TOKEN_SENTINEL = "workspace-runs-smoke-secret-token";
const WORKSPACE_ID = "ws-smoke";

// The spec whose run stays active and is the one waiting for a decision.
const SPEC_ACTIVE = "US-901";
// The spec whose run is closed before anything is listed: it is what makes
// "the list holds what is in flight" a statement with a counter-example.
const SPEC_CLOSED = "US-902";

const RUN_ID = "run-1";
const APPROVAL_ID = "appr-1";
const APPROVAL_OPTION = "allow-once";
const APPROVAL_TITLE = "Eseguire la suite di test";

// The identifiers of every operation the spec detail offers. They are listed
// here, in full, because the story moves that detail from an overlaid window
// into a pane of the shell: the move is only harmless if every one of them
// survives it, and a list written out is the only way to notice the one that
// did not.
const SPEC_OPERATION_IDS = [
	"story-form",
	"plan-form",
	"story-edit-btn",
	"story-delete-btn",
	"story-actions",
	"story-execution",
	"story-run",
	"review-tab",
	"review-approve-btn",
	"review-request-btn",
	"review-integrate-btn",
];
const SPEC_TABS = ["story", "plan", "review"];

// Every viewer request this file makes, in order. It is what turns "the panel
// of that run was never opened" from a comment into an assertion.
const viewerRequests = [];
// Every viewer response body, so the final check can prove the credential never
// travelled to the browser.
const viewerBodies = [];

// One entry per proved statement, for the report.
const checks = [];

function ok(criterion, statement) {
	checks.push({ criterion, statement });
	console.log(`-> ${criterion} ok: ${statement}`);
}

async function main() {
	const options = parseArgs(process.argv.slice(2));
	if (process.platform === "win32") {
		console.log("SKIP: the fake agent binary relies on a POSIX shebang");
		return;
	}
	const runDir = await createRunDir(options.workspaceRoot);
	console.log(`-> run directory: ${runDir}`);
	await fs.mkdir(binDir, { recursive: true });
	await buildCLI();

	const startedAt = Date.now();
	let failure = null;
	try {
		await scenario(runDir);
	} catch (error) {
		failure = error;
	}

	await writeReport(runDir, { startedAt, durationMs: Date.now() - startedAt, failure });

	if (failure) {
		if (options.cleanup) await fs.rm(runDir, { recursive: true, force: true });
		throw failure;
	}
	if (options.cleanup) {
		await fs.rm(runDir, { recursive: true, force: true });
		console.log(`-> cleaned run directory: ${runDir}`);
	}
	console.log(`\nPASS: workspace-runs view smoke test completed (${checks.length} statements proved).`);
	for (const check of checks) {
		console.log(`  ✓ ${check.criterion} — ${check.statement}`);
	}
	console.log(`Run directory: ${runDir}`);
}

// The whole story on one workspace and one viewer process, because it is one
// story: what is in flight, what is waiting for me, what the conversation still
// holds and what the browser is actually served are four views of the same
// running workspace.
async function scenario(runDir) {
	const sandboxDir = path.join(runDir, "sandbox");
	const stateDir = path.join(runDir, "state");
	await fs.mkdir(sandboxDir, { recursive: true });

	let control;
	let hub;
	let view;
	try {
		// Every process started from here writes the registry of known workspaces
		// inside the run directory, never in the real state of the machine.
		const env = {
			...process.env,
			ARCHETIPO_DATA_DIR: repoRoot,
			ARCHETIPO_STATE_DIR: stateDir,
			ARCIPELAGO_TOKEN: TOKEN_SENTINEL,
		};

		await runCommand("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], {
			cwd: sandboxDir,
			env,
		});
		await addSpecs(runDir, sandboxDir, env);

		control = await startControlServer();
		console.log(`-> control server for the fake claude: ${control.url}`);
		hub = await startFakeHub();
		console.log(`-> fake ARcipelago hub: ${hub.url}`);

		view = await startViewServer(sandboxDir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
		const pid = view.child.pid;
		console.log(`-> view ready: ${view.url} (pid ${pid})`);

		// --- Phase 1: the local provider ------------------------------------
		// The default is saved through the very route the Execution panel uses,
		// so every run below starts from the state the UI produces.
		await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
			id: "claude",
			config: { command: fakeClaudePath, timeout_seconds: 600 },
		}));

		// The run that ends. It is started, driven to a terminal record and its
		// process is watched out of existence before anything else is started,
		// so the list assertion later has a real counter-example behind it.
		const closedStart = await apiJSON(`${view.url}/api/spec/${SPEC_CLOSED}/execution`, postJSON({ action: "plan" }), 201);
		const closedID = closedStart.id;
		const closedProcess = await control.waitFor("argv", 1);
		control.push(emit({ type: "system", subtype: "init", session_id: "session-closed" }));
		control.push(emit({
			type: "result",
			subtype: "error_during_execution",
			is_error: true,
			result: "il finto non pianifica nulla",
		}));
		const closedRecord = await waitForExecution(
			view.url,
			closedID,
			(record) => record.status !== "RUNNING",
			`execution ${closedID} of ${SPEC_CLOSED} to reach a terminal status`,
		);
		// Waited on the operating system, not on a route: while that process is
		// alive it would consume the commands meant for the conversation.
		await waitForProcessGone(closedProcess.pid, `the agent process of ${SPEC_CLOSED} (pid ${closedProcess.pid}) to be released`);
		console.log(`-> ${SPEC_CLOSED}: execution ${closedID} is ${closedRecord.status}`);

		// The conversation of the workspace, opened and given a history. It is
		// the subject of AC-2 and it must survive everything that follows.
		control.push(emit({ type: "system", subtype: "init", session_id: "conversation-1" }));
		const opened = await apiJSON(`${view.url}/api/workspace/conversation`, postJSON({}), 201);
		const conversationID = opened.conversation?.id;
		if (!conversationID) {
			throw new Error(`the conversation did not open: ${JSON.stringify(opened)}`);
		}
		await control.waitFor("argv", 2);
		control.push(emit(assistantText("Guardo il workspace mentre apri le spec.")));
		const conversationBefore = await waitForConversation(
			view.url,
			(data) => (data.events || []).length === 1,
			"the assistant frame to become the first event of the conversation",
		);

		// The run of workspace scope. Nothing is ever emitted to its process
		// again: it stays in flight for the rest of the smoke, which is exactly
		// what the rail has to be able to show.
		const workspaceStart = await apiJSON(`${view.url}/api/workspace/execution`, postJSON({ action: "spec-draft" }), 201);
		const workspaceID = workspaceStart.id;
		if (workspaceStart.status !== "RUNNING" || workspaceStart.spec_code !== "") {
			throw new Error(`the workspace-scoped execution is not the expected one: ${JSON.stringify(workspaceStart)}`);
		}
		await control.waitFor("argv", 3);
		console.log(`-> workspace scope: execution ${workspaceID} (spec-draft) is running`);

		// --- Phase 2: the remote provider -------------------------------------
		// Same route, same panel: the workspace default becomes the arcipelago
		// provider pointed at the local hub. The run below is the one that will
		// be asked for a decision.
		await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
			id: "arcipelago",
			config: {
				base_url: hub.url,
				workspace_id: WORKSPACE_ID,
				poll_interval_seconds: 1,
				timeout_seconds: 600,
			},
		}));

		const specStart = await apiJSON(`${view.url}/api/spec/${SPEC_ACTIVE}/execution`, postJSON({ action: "plan" }), 201);
		const specID = specStart.id;
		if (specStart.status !== "RUNNING" || !specID) {
			throw new Error(`unexpected execution record on start: ${JSON.stringify(specStart)}`);
		}
		const remote = await hub.waitForTask(SPEC_ACTIVE);
		hub.assignRun(remote.id);
		console.log(`-> spec scope: execution ${specID} bound to remote task ${remote.id}, run ${RUN_ID} active`);

		// --- AC-3 -------------------------------------------------------------
		const listed = await waitForWorkspaceRuns(
			view.url,
			(data) => byID(data, specID) && byID(data, workspaceID),
			`both the spec-scoped run ${specID} and the workspace-scoped run ${workspaceID} to be listed`,
		);
		const ids = (listed.runs || []).map((entry) => entry.id).sort();
		if (JSON.stringify(ids) !== JSON.stringify([specID, workspaceID].sort())) {
			throw new Error(`AC-3: the list must hold exactly the two runs in flight; got ${JSON.stringify(listed.runs)}`);
		}
		if (ids.includes(closedID)) {
			throw new Error(`AC-3: the terminal execution ${closedID} must not be listed; got ${JSON.stringify(listed.runs)}`);
		}
		const specEntry = byID(listed, specID);
		assertEqual(specEntry.scope, "spec", `AC-3: the scope of the run of ${SPEC_ACTIVE}`);
		assertEqual(specEntry.spec_code, SPEC_ACTIVE, "AC-3: the spec the spec-scoped run is about");
		assertEqual(specEntry.action, "plan", "AC-3: the action of the spec-scoped run");
		assertEqual(specEntry.status, "RUNNING", "AC-3: the status of the spec-scoped run");
		const workspaceEntry = byID(listed, workspaceID);
		assertEqual(workspaceEntry.scope, "workspace", "AC-3: the scope of the workspace-scoped run");
		assertEqual(workspaceEntry.spec_code, "", "AC-3: the spec of a run whose object is the workspace");
		assertEqual(workspaceEntry.action, "spec-draft", "AC-3: the action of the workspace-scoped run");
		assertEqual(workspaceEntry.status, "RUNNING", "AC-3: the status of the workspace-scoped run");
		for (const entry of listed.runs) {
			if (typeof entry.id !== "string" || entry.id.trim() === "") {
				throw new Error(`AC-3: every entry must carry the execution id it is reached by; got ${JSON.stringify(entry)}`);
			}
			if (typeof entry.created_at !== "string" || entry.created_at.trim() === "") {
				throw new Error(`AC-3: every entry must say when it started; got ${JSON.stringify(entry)}`);
			}
		}
		ok(
			"AC-3",
			`GET /api/workspace/runs lists the two runs in flight — ${specID} (scope spec, ${SPEC_ACTIVE}, plan, RUNNING) and ${workspaceID} (scope workspace, spec-draft, RUNNING) — each carrying the execution id it is reached by, and leaves the terminal execution ${closedID} (${closedRecord.status}) out`,
		);

		// --- AC-4 -------------------------------------------------------------
		// The panel of the waiting run is never opened. The oracle is the
		// recorded list of requests this file made, not a promise in a comment.
		assertNoRunPanelCalls(specID, "AC-4: before the approval was even opened");
		hub.setApprovals([pendingApproval()]);
		const awaiting = await waitForWorkspaceRuns(
			view.url,
			(data) => byID(data, specID)?.awaiting_response === true,
			`the run ${specID} to report awaiting_response after the hub opened ${APPROVAL_ID}`,
		);
		// The assertion that gives the story its meaning, made at the moment the
		// wait becomes visible and not a line later.
		assertNoRunPanelCalls(specID, "AC-4: at the moment the wait became visible");
		const awaitingEntry = byID(awaiting, specID);
		assertEqual(awaitingEntry.pending?.id, APPROVAL_ID, "AC-4: the decision the waiting run names");
		assertEqual(awaitingEntry.pending?.title, APPROVAL_TITLE, "AC-4: the title of the decision the waiting run names");
		const otherEntry = byID(awaiting, workspaceID);
		if (otherEntry.awaiting_response !== false || otherEntry.pending) {
			throw new Error(`AC-4: only the run that was asked may be marked as waiting; got ${JSON.stringify(otherEntry)}`);
		}
		const listCalls = viewerRequests.filter((entry) => entry.path === "/api/workspace/runs").length;

		// Answered through the existing route, which is the first time this file
		// touches the run panel of that execution at all.
		await apiJSON(
			`${view.url}/api/execution/${specID}/run/approvals/${APPROVAL_ID}`,
			postJSON({ option_id: APPROVAL_OPTION }),
			202,
		);
		const responses = hub.approvalResponses();
		if (responses.length !== 1 || responses[0].approvalId !== APPROVAL_ID || responses[0].optionId !== APPROVAL_OPTION) {
			throw new Error(`AC-4: the hub must have recorded ("${APPROVAL_ID}","${APPROVAL_OPTION}"); got ${JSON.stringify(responses)}`);
		}
		const answered = await waitForWorkspaceRuns(
			view.url,
			(data) => byID(data, specID)?.awaiting_response === false,
			`the run ${specID} to stop reporting awaiting_response once the decision was answered`,
		);
		if (byID(answered, specID).pending) {
			throw new Error(`AC-4: an answered run must name no pending decision; got ${JSON.stringify(byID(answered, specID))}`);
		}
		ok(
			"AC-4",
			`the wait of ${specID} on ${APPROVAL_ID} (${JSON.stringify(APPROVAL_TITLE)}) was observed through ${listCalls} call(s) to GET /api/workspace/runs and exactly 0 to GET /api/execution/${specID}/run — counted over the ${viewerRequests.length} requests this smoke had made — while ${workspaceID} stayed unmarked; answering the decision cleared the mark`,
		);

		// --- AC-2 -------------------------------------------------------------
		// The routes of the spec detail are exercised between the two readings:
		// on the server, opening and closing that pane is exactly these reads.
		await apiJSON(`${view.url}/api/spec/${SPEC_ACTIVE}`);
		await probe(`${view.url}/api/spec/${SPEC_ACTIVE}/diff`);
		await probe(`${view.url}/api/spec/${SPEC_ACTIVE}/review`);
		await apiJSON(`${view.url}/api/board`);
		await apiJSON(`${view.url}/api/spec/${SPEC_CLOSED}`);

		const conversationAfter = await readConversation(view.url);
		assertEqual(conversationAfter.conversation?.id, conversationID, "AC-2: the id of the conversation after the spec detail was exercised");
		if (JSON.stringify(conversationAfter.events) !== JSON.stringify(conversationBefore.events)) {
			throw new Error(
				`AC-2: the history changed across the spec detail\n  before: ${JSON.stringify(conversationBefore.events)}\n  after:  ${JSON.stringify(conversationAfter.events)}`,
			);
		}
		assertEqual(conversationAfter.last_id, conversationBefore.last_id, "AC-2: the cursor of the conversation");
		assertEqual(view.child.pid, pid, "AC-2: the viewer PID across the whole story");
		assertAlive(view.child, "AC-2: the viewer process");
		ok(
			"AC-2",
			`after five reads of the spec detail routes the conversation ${conversationID} answers with the same id, the same ${conversationAfter.events.length}-event history and the same cursor ${conversationAfter.last_id}, on the same viewer process (pid ${pid}), never restarted`,
		);

		// --- AC-1 and AC-5 ----------------------------------------------------
		// Asserted on the document the browser is really served, not on the file
		// in the repository: what ships is what the viewer embeds.
		const html = await rawGet(`${view.url}/index.html`);
		assertShellStructure(html);
		ok(
			"AC-1, AC-5",
			`the served index.html nests #workspace-conversation and #workspace-runs inside <aside id="workspace-rail">, keeps #modal-root out of the rail and inside the primary column of #workspace-shell, and still carries all ${SPEC_OPERATION_IDS.length} identifiers of the spec operations plus the ${SPEC_TABS.length} tabs ${SPEC_TABS.join("/")}`,
		);

		// --- AC-6 -------------------------------------------------------------
		const css = await rawGet(`${view.url}/app.css`);
		const narrowClasses = assertNarrowMode(css);
		ok(
			"AC-6",
			`the served app.css carries the media query at the breakpoint ${narrowClasses.breakpoint}px declared by workspace-layout.js, and styles every class that module returns for a narrow state: ${narrowClasses.classes.join(", ")}`,
		);

		// --- the credential ---------------------------------------------------
		if (!hub.authorizedCalls()) {
			throw new Error("the provider never authenticated against the hub, so the credential path was not exercised");
		}
		if (hub.unauthorizedCalls()) {
			throw new Error(`the hub rejected ${hub.unauthorizedCalls()} call(s): the credential did not reach the provider`);
		}
		const leaked = viewerBodies.filter((body) => body.includes(TOKEN_SENTINEL));
		if (leaked.length) {
			throw new Error(`the viewer echoed the credential in ${leaked.length} response(s)`);
		}
		ok(
			"secrets",
			`the token was used on ${hub.authorizedCalls()} hub calls, was refused on none, and is absent from all ${viewerBodies.length} viewer responses`,
		);
	} finally {
		if (view) await stopProcess(view.child);
		if (hub) await hub.close();
		if (control) await control.close();
	}
}

// --- oracles -----------------------------------------------------------------

function byID(payload, id) {
	return (payload?.runs || []).find((entry) => entry.id === id);
}

// assertNoRunPanelCalls is the whole point of AC-4 expressed as a count: the
// panel of a run is what calls GET /api/execution/{id}/run, and learning that
// the run is waiting must not require having opened it.
function assertNoRunPanelCalls(executionID, label) {
	const prefix = `/api/execution/${executionID}/run`;
	const calls = viewerRequests.filter((entry) => entry.path === prefix || entry.path.startsWith(`${prefix}?`));
	if (calls.length !== 0) {
		throw new Error(`${label}: the run panel of ${executionID} was opened ${calls.length} time(s): ${JSON.stringify(calls)}`);
	}
}

// assertShellStructure states who is inside whom in the document the viewer
// really serves, and that nothing of the spec detail was lost when it stopped
// being an overlaid window.
function assertShellStructure(html) {
	const at = (needle, label) => {
		const index = html.indexOf(needle);
		if (index === -1) {
			throw new Error(`AC-1: the served index.html does not contain ${label} (${JSON.stringify(needle)})`);
		}
		return index;
	};

	const shellAt = at('id="workspace-shell"', "the shell");
	const primaryAt = at('id="workspace-primary"', "the primary column");
	const modalAt = at('id="modal-root"', "the spec detail");
	const railAt = at('<aside id="workspace-rail"', "the lateral rail");

	if (!(shellAt < primaryAt && primaryAt < modalAt && modalAt < railAt)) {
		throw new Error(
			`AC-1: the spec detail must sit inside the primary column of the shell; got offsets shell=${shellAt}, primary=${primaryAt}, modal-root=${modalAt}, rail=${railAt}`,
		);
	}

	const railEnd = html.indexOf("</aside>", railAt);
	if (railEnd === -1) {
		throw new Error("AC-1: the lateral rail is never closed in the served index.html");
	}
	const rail = html.slice(railAt, railEnd);
	for (const id of ["workspace-conversation", "workspace-runs"]) {
		if (!rail.includes(`id="${id}"`)) {
			throw new Error(`AC-1: #${id} must live inside the lateral rail; the rail holds ${JSON.stringify(truncate(rail, 400))}`);
		}
	}
	if (rail.includes("modal-root")) {
		throw new Error("AC-1: the spec detail must not be nested in the rail: opening a spec would then move the conversation");
	}

	// AC-5 — nothing of what could be done on a spec was lost by the move.
	const missingIDs = SPEC_OPERATION_IDS.filter((id) => !html.includes(`id="${id}"`));
	if (missingIDs.length) {
		throw new Error(`AC-5: the served index.html lost the spec operations ${JSON.stringify(missingIDs)}`);
	}
	const missingTabs = SPEC_TABS.filter((tab) => !html.includes(`data-tab="${tab}"`));
	if (missingTabs.length) {
		throw new Error(`AC-5: the served index.html lost the spec tabs ${JSON.stringify(missingTabs)}`);
	}
}

// assertNarrowMode checks the served stylesheet against the module that decides
// the layout, so the breakpoint is never a number this test invented: it is
// read from workspace-layout.js, exactly as the caller in the browser reads it.
function assertNarrowMode(css) {
	const { resolveLayout, NARROW_MAX_WIDTH } = loadWorkspaceLayout();
	const query = `@media (max-width: ${NARROW_MAX_WIDTH}px)`;
	if (!css.includes(query)) {
		throw new Error(`AC-6: the served app.css carries no media query at the declared breakpoint ${JSON.stringify(query)}`);
	}

	// The classes the module really returns for a narrow shell, plus the shell
	// class of the wide one: whatever it returns has to mean something in the
	// stylesheet the browser is served, or the decision would apply to nothing.
	const narrow = resolveLayout({ specOpen: true, narrow: true, railFocus: false });
	const wide = resolveLayout({ specOpen: true, narrow: false });
	const required = new Set([wide.shellClass]);
	for (const pane of Object.values(narrow.panes)) {
		required.add(pane.className);
		required.add(pane.stateClass);
	}
	const classes = [...required].sort();
	const missing = classes.filter((name) => !css.includes(`.${name}`));
	if (missing.length) {
		throw new Error(`AC-6: the served app.css styles none of ${JSON.stringify(missing)}, which the layout module returns`);
	}
	return { breakpoint: NARROW_MAX_WIDTH, classes };
}

// loadWorkspaceLayout runs the UMD module in a bare context: it detects
// `module.exports` first, so the Node branch is enough. Same loader as
// test/web/workspace-layout.test.mjs, for the same reason — the module is the
// declaration of the breakpoint and of the class names, and reading it is what
// keeps this smoke from repeating them.
function loadWorkspaceLayout() {
	const src = readFileSync(path.join(assetsDir, "workspace-layout.js"), "utf8");
	const mod = { exports: {} };
	const ctx = createContext({ module: mod, window: undefined });
	runInContext(src, ctx);
	return mod.exports;
}

function assertEqual(actual, expected, label) {
	if (actual !== expected) {
		throw new Error(`Unexpected ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
	}
}

function assertAlive(child, label) {
	if (child.exitCode !== null || child.signalCode !== null) {
		throw new Error(`${label}: the process is gone (exit ${child.exitCode}, signal ${child.signalCode})`);
	}
}

// --- waiting, always on a route ----------------------------------------------

// waitFor polls a condition until it holds. Every failure names what was
// expected and what the last reading was: a mute timeout would say nothing
// about the state the workspace got stuck in.
async function waitFor(read, predicate, expectation, describe, timeoutMs = 30000) {
	const started = Date.now();
	let last = null;
	let lastError = null;
	while (Date.now() - started < timeoutMs) {
		try {
			last = await read();
			lastError = null;
			if (predicate(last)) return last;
		} catch (error) {
			lastError = error;
		}
		await delay(150);
	}
	const detail = lastError ? `last error: ${lastError.message}` : describe(last);
	throw new Error(`Timed out after ${timeoutMs}ms waiting for ${expectation}\n  ${detail}`);
}

async function waitForWorkspaceRuns(viewURL, predicate, expectation, timeoutMs = 30000) {
	return waitFor(
		() => apiJSON(`${viewURL}/api/workspace/runs`),
		predicate,
		expectation,
		(last) => `last list: ${JSON.stringify(last)}`,
		timeoutMs,
	);
}

async function waitForExecution(viewURL, executionID, predicate, expectation, timeoutMs = 30000) {
	return waitFor(
		() => apiJSON(`${viewURL}/api/execution/${executionID}`),
		predicate,
		expectation,
		(last) => `last record: ${JSON.stringify(last)}`,
		timeoutMs,
	);
}

async function readConversation(viewURL) {
	return apiJSON(`${viewURL}/api/workspace/conversation?after_id=0`);
}

async function waitForConversation(viewURL, predicate, expectation, timeoutMs = 30000) {
	return waitFor(
		() => readConversation(viewURL),
		predicate,
		expectation,
		(last) => `last conversation: ${JSON.stringify(last?.conversation)}, ${JSON.stringify((last?.events || []).length)} event(s)`,
		timeoutMs,
	);
}

// waitForProcessGone polls the operating system until the agent process is no
// longer there. `kill(pid, 0)` sends nothing: it asks whether the process
// exists, and it is the only oracle that answers "the provider really released
// it" instead of "the route said so".
async function waitForProcessGone(pid, what, timeoutMs = 20000) {
	if (!Number.isInteger(pid)) {
		throw new Error(`the fake did not report the pid of its own process; got ${JSON.stringify(pid)}`);
	}
	const started = Date.now();
	while (Date.now() - started < timeoutMs) {
		try {
			process.kill(pid, 0);
		} catch (error) {
			if (error.code === "ESRCH") return;
			throw new Error(`could not ask the operating system about pid ${pid}: ${error.message}`);
		}
		await delay(50);
	}
	throw new Error(`Timed out after ${timeoutMs}ms waiting for ${what}; the process is still alive`);
}

// --- the fake ARcipelago hub --------------------------------------------------
//
// It serves the task routes the arcipelago client uses plus the run namespace
// the follower needs. Nothing progresses on its own: this test creates the run,
// opens the approval and would close both by hand.

async function startFakeHub() {
	const tasks = new Map(); // task id -> record
	const bySpec = new Map(); // spec code -> task id
	let created = 0;
	let authorized = 0;
	let unauthorized = 0;

	const run = { id: "", taskId: "", state: "active", createdAt: Date.now(), closedAt: 0, error: "" };
	const history = [];
	const approvalResponses = [];
	let approvals = [];
	let stream = null; // the currently attached SSE response, at most one

	const server = http.createServer((req, res) => {
		const url = new URL(req.url, "http://127.0.0.1");
		if (req.headers.authorization !== `Bearer ${TOKEN_SENTINEL}`) {
			unauthorized += 1;
			return sendJSON(res, 401, { error: "unauthorized" });
		}
		authorized += 1;

		if (req.method === "POST" && url.pathname === "/api/external/tasks") {
			return readBody(req).then((raw) => {
				let request;
				try {
					request = JSON.parse(raw);
				} catch {
					return sendJSON(res, 400, { error: "invalid_json" });
				}
				if (request.workspaceId !== WORKSPACE_ID) {
					return sendJSON(res, 404, { error: "workspace_not_found" });
				}
				const specCode = (request.metadata || {}).spec_code;
				if (!specCode || !request.prompt || !request.title) {
					return sendJSON(res, 400, { error: "invalid_task" });
				}
				created += 1;
				const id = `task-${created}`;
				const record = { id, status: "running", resultSummary: "", runId: "", specCode, request };
				tasks.set(id, record);
				bySpec.set(specCode, id);
				return sendJSON(res, 201, { task: publicTask(record) });
			});
		}

		if (req.method === "GET" && url.pathname === "/api/external/tasks/by-reference") {
			const externalID = url.searchParams.get("externalId");
			const match = [...tasks.values()].find((t) => t.request.externalId === externalID);
			if (!match) return sendJSON(res, 404, { error: "task_not_found" });
			return sendJSON(res, 200, { task: publicTask(match) });
		}

		if (req.method === "GET" && url.pathname.startsWith("/api/external/tasks/")) {
			const id = decodeURIComponent(url.pathname.slice("/api/external/tasks/".length));
			const record = tasks.get(id);
			if (!record) return sendJSON(res, 404, { error: "task_not_found" });
			return sendJSON(res, 200, { task: publicTask(record) });
		}

		const runRoute = /^\/api\/external\/runs\/([^/]+)(\/.*)?$/.exec(url.pathname);
		if (runRoute) {
			const runID = decodeURIComponent(runRoute[1]);
			const rest = runRoute[2] || "";
			if (!run.id || runID !== run.id) {
				return sendJSON(res, 404, { error: "run_not_found" });
			}
			if (req.method === "GET" && rest === "") {
				return sendJSON(res, 200, {
					run: {
						id: run.id,
						runnerId: "runner-1",
						taskId: run.taskId,
						state: run.state,
						createdAt: run.createdAt,
						closedAt: run.closedAt,
						error: run.error,
					},
				});
			}
			if (req.method === "GET" && rest === "/approvals") {
				return sendJSON(res, 200, { approvals });
			}
			if (req.method === "GET" && rest === "/events") {
				return openStream(req, res, url);
			}
			const respondRoute = /^\/approvals\/([^/]+)\/respond$/.exec(rest);
			if (req.method === "POST" && respondRoute) {
				const approvalID = decodeURIComponent(respondRoute[1]);
				return readBody(req).then((raw) => {
					let body = {};
					try {
						body = JSON.parse(raw || "{}");
					} catch {
						return sendJSON(res, 400, { error: "invalid_json" });
					}
					approvalResponses.push({ runId: runID, approvalId: approvalID, optionId: body.optionId });
					approvals = approvals.filter((approval) => approval.id !== approvalID);
					return sendJSON(res, 202, { ok: true });
				});
			}
			return sendJSON(res, 404, { error: "not_found" });
		}

		return sendJSON(res, 404, { error: "not_found" });
	});

	// openStream serves the SSE history from the cursor the subscriber asked
	// for, and holds the socket open while the run is active.
	function openStream(req, res, url) {
		const afterParam = url.searchParams.get("afterId");
		const headerCursor = req.headers["last-event-id"];
		const afterId = Number.parseInt(afterParam ?? headerCursor ?? "0", 10) || 0;

		res.writeHead(200, {
			"Content-Type": "text/event-stream; charset=utf-8",
			"Cache-Control": "no-cache",
			Connection: "keep-alive",
		});
		for (const frame of history) {
			if (frame.id > afterId) res.write(frameText(frame));
		}
		if (run.state !== "active") {
			res.write(endFrameText("closed"));
			res.end();
			return undefined;
		}
		stream = res;
		req.on("close", () => {
			if (stream === res) stream = null;
		});
		return undefined;
	}

	await new Promise((resolve, reject) => {
		server.once("error", reject);
		server.listen(0, "127.0.0.1", resolve);
	});
	const url = `http://127.0.0.1:${server.address().port}`;

	async function waitForTask(specCode, timeoutMs = 30000) {
		const started = Date.now();
		while (Date.now() - started < timeoutMs) {
			const id = bySpec.get(specCode);
			if (id) return tasks.get(id);
			await delay(50);
		}
		throw new Error(`the provider never created a remote task for ${specCode}`);
	}

	// assignRun is what turns an observable task into a followable run: until
	// the task carries a runId, ResolveRun has nothing to resolve.
	function assignRun(taskID) {
		const record = tasks.get(taskID);
		if (!record) throw new Error(`no remote task ${taskID} to bind a run to`);
		record.runId = RUN_ID;
		run.id = RUN_ID;
		run.taskId = taskID;
		run.state = "active";
		run.createdAt = Date.now();
		return run;
	}

	return {
		url,
		waitForTask,
		assignRun,
		setApprovals: (list) => {
			approvals = list;
		},
		approvalResponses: () => approvalResponses.map((entry) => ({ ...entry })),
		authorizedCalls: () => authorized,
		unauthorizedCalls: () => unauthorized,
		close: () =>
			new Promise((resolve) => {
				if (stream) {
					const res = stream;
					stream = null;
					res.destroy();
				}
				server.closeAllConnections?.();
				server.close(() => resolve());
			}),
	};
}

function publicTask(record) {
	return { id: record.id, status: record.status, resultSummary: record.resultSummary, runId: record.runId };
}

function pendingApproval() {
	return {
		id: APPROVAL_ID,
		runId: RUN_ID,
		runnerId: "runner-1",
		createdAt: Date.now(),
		request: {
			toolName: "Bash",
			title: APPROVAL_TITLE,
			args: { command: "npm test" },
			options: [
				{ optionId: APPROVAL_OPTION, name: "Consenti una volta", kind: "allow" },
				{ optionId: "reject", name: "Rifiuta", kind: "reject" },
			],
		},
	};
}

function frameText(frame) {
	return `id: ${frame.id}\nevent: run_event\ndata: ${JSON.stringify(frame)}\n\n`;
}

function endFrameText(reason) {
	return `event: end\ndata: ${JSON.stringify({ reason })}\n\n`;
}

// --- the control server of the fake agent -------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received. Several processes share it, so a command
// is only ever pushed while exactly one of them is alive — which is why the
// process of the closed run is watched out of existence before the conversation
// is opened, and why nothing is pushed once the workspace-scoped run has
// started.

async function startControlServer() {
	const commands = [];
	const received = [];

	const server = http.createServer(async (req, res) => {
		if (req.method === "GET" && req.url.startsWith("/next")) {
			sendJSON(res, 200, commands.shift() || { kind: "none" });
			return;
		}
		if (req.method === "POST" && req.url.startsWith("/received")) {
			received.push(JSON.parse(await readBody(req)));
			sendJSON(res, 200, { ok: true });
			return;
		}
		sendJSON(res, 404, { error: "not found" });
	});

	await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
	const url = `http://127.0.0.1:${server.address().port}`;

	return {
		url,
		push(command) {
			commands.push(command);
		},
		reports() {
			return received;
		},
		async waitFor(kind, count = 1, timeoutMs = 30000) {
			const started = Date.now();
			while (Date.now() - started < timeoutMs) {
				const matching = received.filter((entry) => entry.kind === kind);
				if (matching.length >= count) return matching[count - 1];
				await delay(50);
			}
			throw new Error(
				`Timed out after ${timeoutMs}ms waiting for the fake to report ${kind} ${count} time(s); it reported ${JSON.stringify(received.map((entry) => entry.kind))}`,
			);
		},
		close() {
			return new Promise((resolve) => server.close(resolve));
		},
	};
}

function emit(frame) {
	return { kind: "emit", frame };
}

function assistantText(text) {
	return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text }] } };
}

// --- fixtures ------------------------------------------------------------------

// Two specs, because the story needs two runs that are not the same run: one
// stays in flight and is asked for a decision, the other one ends before
// anything is listed.
function seedSpec(code, title) {
	return {
		code,
		title,
		epic: { code: "EP-999", title: "Smoke tests" },
		priority: "LOW",
		points: 1,
		status: "TODO",
		body: [
			"**User Story**",
			`Come persona di smoke voglio ${title.toLowerCase()}, per osservare le run del workspace.`,
			"",
			"**Criteri di accettazione**",
			"- [ ] AC-1 — la spec appare nella colonna TODO.",
			"",
		].join("\n"),
	};
}

async function addSpecs(runDir, sandboxDir, env) {
	const file = path.join(runDir, "specs.json");
	const specs = [
		seedSpec(SPEC_ACTIVE, "Seguire una run che attende una decisione"),
		seedSpec(SPEC_CLOSED, "Chiudere una run prima dell'elenco"),
	];
	await fs.writeFile(file, `${JSON.stringify({ specs }, null, 2)}\n`);
	await runCommand("spec-add", cliPath, ["spec", "add", "--file", file], { cwd: sandboxDir, env });
}

// --- the report ----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
	const summary = {
		smoke: "workspace-runs-from-view",
		spec: "US-055",
		passed: !failure,
		started_at: new Date(startedAt).toISOString(),
		duration_ms: durationMs,
		run_dir: runDir,
		checks,
		error: failure ? failure.message : null,
	};
	await fs.writeFile(path.join(runDir, "summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
	await fs.writeFile(path.join(runDir, "report.html"), renderReport(summary));
	console.log(`-> report: ${path.join(runDir, "report.html")}`);
}

function renderReport(summary) {
	const rows = summary.checks
		.map((check) => `<tr><td class="code">${escapeHTML(check.criterion)}</td><td>${escapeHTML(check.statement)}</td></tr>`)
		.join("\n        ");
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ARchetipo Smoke — Workspace runs and the anchored spec pane (US-055)</title>
  <style>
    :root { color-scheme: light; --bg: #f6f7f9; --panel: #fff; --ink: #172026; --muted: #61707d; --line: #d8dee6; --ok: #18794e; --fail: #c93a2f; }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--bg); color: var(--ink); font: 14px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { max-width: 1000px; margin: 0 auto; padding: 28px; }
    header { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; margin-bottom: 24px; }
    h1 { margin: 0 0 12px; font-size: 22px; }
    h2 { margin: 24px 0 12px; font-size: 18px; border-bottom: 1px solid var(--line); padding-bottom: 8px; }
    .meta { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; }
    .meta div { border: 1px solid var(--line); border-radius: 6px; padding: 8px 10px; background: #fbfcfd; }
    .label { display: block; color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .5px; }
    .value { overflow-wrap: anywhere; }
    .pass { color: var(--ok); font-weight: 700; }
    .fail { color: var(--fail); font-weight: 700; }
    table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; background: var(--panel); }
    th { text-align: left; padding: 8px 10px; border-bottom: 2px solid var(--line); color: var(--muted); background: #fbfcfd; }
    td { padding: 8px 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
    .code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 650; white-space: nowrap; }
    pre { background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 12px; overflow-x: auto; white-space: pre-wrap; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>ARchetipo Smoke — Workspace runs and the anchored spec pane (US-055)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>One real workspace served by the real <code>archetipo view</code>, with two runs in flight — one of
    spec scope on the <code>arcipelago</code> provider pointed at a fake local hub, one of workspace scope on
    the <code>claude</code> provider pointed at a fake agent binary — and a third run driven to a terminal
    record before anything is listed. The hub opens one approval by hand, and the wait is observed through
    <code>GET /api/workspace/runs</code> alone: this smoke records every request it makes and asserts that it
    never called <code>GET /api/execution/{id}/run</code> for that execution. The shell served to the browser
    and the stylesheet it loads are asserted on the bytes the viewer really answers with.</p>

    <h2>Proved statements</h2>
    <table>
      <thead><tr><th>Criterion</th><th>Statement</th></tr></thead>
      <tbody>
        ${rows || '<tr><td colspan="2">none</td></tr>'}
      </tbody>
    </table>
${summary.error ? `\n    <h2>Failure</h2>\n    <pre>${escapeHTML(summary.error)}</pre>\n` : ""}  </main>
</body>
</html>
`;
}

function escapeHTML(value) {
	return String(value ?? "")
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;");
}

function truncate(value, max = 200) {
	const text = String(value ?? "");
	return text.length <= max ? text : `${text.slice(0, max)}…`;
}

// --- harness --------------------------------------------------------------------

function parseArgs(argv) {
	const options = { workspaceRoot: defaultWorkspaceRoot, cleanup: false };
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		switch (arg) {
			case "--workspace-root":
				options.workspaceRoot = path.resolve(argv[++i]);
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
	console.log(`Smoke test for the workspace runs rail and the anchored spec pane (US-055)

Usage:
  node ./test/e2e/workspace-runs-view-smoke.mjs
  npm run test:view-workspace-runs-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated run directory
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
	const runsRoot = path.join(root, "runs");
	await fs.mkdir(runsRoot, { recursive: true });
	const stamp = new Date().toISOString().replace(/[:.]/g, "-");
	const runDir = path.join(runsRoot, stamp);
	await fs.mkdir(runDir, { recursive: true });
	return runDir;
}

async function buildCLI() {
	console.log(`-> building CLI: ${cliPath}`);
	await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
		cwd: path.join(repoRoot, "cli"),
		env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot },
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
	while (Date.now() - started < 15000) {
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

function postJSON(payload) {
	return { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

function putJSON(payload) {
	return { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

// record is the single door every viewer request goes through, which is what
// makes the AC-4 count possible at all.
function record(url, init) {
	const parsed = new URL(url);
	viewerRequests.push({ method: (init.method || "GET").toUpperCase(), path: `${parsed.pathname}${parsed.search}` });
}

async function rawGet(url) {
	record(url, {});
	const response = await fetch(url);
	const text = await response.text();
	viewerBodies.push(text);
	if (!response.ok) {
		throw new Error(`HTTP ${response.status} for ${url}: ${truncate(text, 400)}`);
	}
	return text;
}

async function apiJSON(url, init = {}, expected = null) {
	record(url, init);
	const response = await fetch(url, {
		...init,
		headers: { Accept: "application/json", ...(init.headers || {}) },
	});
	const text = await response.text();
	viewerBodies.push(text);
	let data = null;
	try {
		data = text ? JSON.parse(text) : null;
	} catch {
		data = text;
	}
	if (!response.ok) {
		throw new Error(`HTTP ${response.status} for ${url}: ${typeof data === "string" ? truncate(data, 400) : JSON.stringify(data)}`);
	}
	if (expected !== null && response.status !== expected) {
		throw new Error(`Expected HTTP ${expected} for ${url}, got ${response.status}: ${truncate(text, 400)}`);
	}
	return data;
}

// probe exercises a route without judging its answer. It is used for the reads
// of the spec detail that legitimately refuse on a spec in this state: what AC-2
// is about is that the conversation survives them, whatever they answered.
async function probe(url) {
	record(url, {});
	const response = await fetch(url, { headers: { Accept: "application/json" } });
	const text = await response.text();
	viewerBodies.push(text);
	return response.status;
}

function sendJSON(res, status, payload) {
	const body = JSON.stringify(payload);
	res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Content-Length": Buffer.byteLength(body) });
	res.end(body);
}

function readBody(req) {
	return new Promise((resolve, reject) => {
		const chunks = [];
		req.on("data", (chunk) => chunks.push(chunk));
		req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
		req.on("error", reject);
	});
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
		child.on("close", (code) => resolve({
			code,
			stdout: Buffer.concat(stdout).toString("utf8"),
			stderr: Buffer.concat(stderr).toString("utf8"),
		}));
		child.on("error", (error) => resolve({ code: 1, stdout: "", stderr: error.message }));
	});

	if (result.code !== 0 && !options.allowFailure) {
		throw new Error(`${label} failed with exit ${result.code}\nSTDOUT:\n${result.stdout}\nSTDERR:\n${result.stderr}`);
	}
	return result;
}

async function stopProcess(child) {
	if (!child || child.killed) return;
	child.kill("SIGTERM");
	await Promise.race([
		new Promise((resolve) => child.once("exit", resolve)),
		delay(3000),
	]);
	if (!child.killed) {
		child.kill("SIGKILL");
	}
}

main().catch((error) => {
	console.error(`\nFAIL: ${error.message}`);
	process.exit(1);
});
