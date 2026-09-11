#!/usr/bin/env node
// install-runners.mjs
//
// Installs the ARchetipo CLI and skills built from this working copy into the
// runner containers of a running ARcipelago fleet, and writes the capability
// that says they are there.
//
// It is `install:dev` for somebody else's machine. The runner image stays a
// generic ARcipelago image — nothing about ARchetipo is baked into it — and the
// tooling arrives afterwards, the way a package does, at the pace ARchetipo is
// rebuilt rather than the pace the fleet is redefined.
//
// Usage:
//   npm run install:runners
//   npm run install:runners -- --services runner-1 --project-name arcipelago-dev
//
// What lands in each container:
//   /opt/archetipo/                      the bundle: bin/, skills/, runtime/
//   /usr/local/bin/archetipo             wrapper setting ARCHETIPO_DATA_DIR
//   $HOME/.pi/agent/skills -> the bundle's skills, so Pi finds them for every
//                             session without the project having to install
//                             them for a particular tool
//   $HOME/.arcipelago/runner.json        capabilities: archetipo, archetipo:<version>
//
// Then each container is restarted, because a runner reads its capabilities
// once, at startup: without the restart the fleet would carry the tooling and
// still tell the hub it does not have it.
//
// This writes into the container filesystem, not into a volume, and that is the
// deliberate trade: everything installed here disappears together when the
// container is recreated — the tooling and the claim about the tooling — so the
// two can never disagree. After a `docker compose up` that recreates a runner,
// run this again. It takes seconds.

import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { devVersion } from "./lib/dev-version.mjs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");

const INSTALL_ROOT = "/opt/archetipo";
const WRAPPER = "/usr/local/bin/archetipo";

function parseArgs(argv) {
	const args = { projectName: "arcipelago-dev", services: null, restart: true };
	for (let i = 0; i < argv.length; i++) {
		const flag = argv[i];
		if (flag === "--project-name") args.projectName = argv[++i];
		else if (flag === "--services") args.services = argv[++i].split(",").map((s) => s.trim()).filter(Boolean);
		else if (flag === "--no-restart") args.restart = false;
		else {
			console.error(`unknown argument: ${flag}`);
			process.exit(1);
		}
	}
	return args;
}

function docker(dockerArgs, { capture = true } = {}) {
	return spawnSync("docker", dockerArgs, { encoding: "utf8", stdio: capture ? "pipe" : "inherit" });
}

function dockerOrDie(dockerArgs, what) {
	const result = docker(dockerArgs);
	if (result.error) {
		console.error(`docker failed to start: ${result.error.message}`);
		process.exit(1);
	}
	if (result.status !== 0) {
		if (result.stderr) process.stderr.write(result.stderr);
		console.error(`${what} failed (docker ${dockerArgs.join(" ")}).`);
		process.exit(1);
	}
	return result.stdout;
}

/** Runs a command inside the container as root, and dies naming what it was for. */
function exec(container, script, what) {
	return dockerOrDie(["exec", "-u", "root", container, "sh", "-c", script], `${what} in ${container}`);
}

// The fleet is discovered rather than configured: Compose labels every
// container it starts with its project and its service, so the runners of a
// running stack are a query, not a list somebody has to keep in sync with the
// compose file.
function discoverRunners({ projectName, services }) {
	const format = "{{.Names}}\t{{.Label \"com.docker.compose.service\"}}";
	const out = dockerOrDie(
		["ps", "--filter", `label=com.docker.compose.project=${projectName}`, "--format", format],
		"listing the fleet",
	);
	const runners = out
		.split("\n")
		.map((line) => line.trim())
		.filter(Boolean)
		.map((line) => {
			const [name, service] = line.split("\t");
			return { name, service };
		})
		.filter(({ service }) => (services ? services.includes(service) : service.startsWith("runner")));
	return runners.sort((a, b) => a.service.localeCompare(b.service));
}

// Asked of the machine rather than assumed from this one: a fleet is allowed to
// be a different architecture than the laptop driving it, and the binary that
// would then be copied in is one that cannot run — a failure that surfaces much
// later, inside an agent session, as a command that exits 126.
function platformOf(container) {
	const machine = dockerOrDie(["exec", container, "uname", "-m"], "reading the architecture").trim();
	const arch = { aarch64: "arm64", arm64: "arm64", x86_64: "x64" }[machine];
	if (!arch) {
		console.error(`${container} reports an architecture we have no build for: ${machine}`);
		process.exit(1);
	}
	return `linux-${arch}`;
}

/** Where the runner process would look for its config file. */
function homeOf(container) {
	const home = dockerOrDie(
		["exec", container, "node", "-p", "require('node:os').homedir()"],
		"reading the home directory",
	).trim();
	if (!home.startsWith("/")) {
		console.error(`${container} reports an unusable home directory: ${home}`);
		process.exit(1);
	}
	return home;
}

const args = parseArgs(process.argv.slice(2));
const version = devVersion(repoRoot);

if (docker(["version", "--format", "{{.Server.Version}}"]).status !== 0) {
	console.error("Docker is not answering. Start it and try again.");
	process.exit(1);
}

const runners = discoverRunners(args);
if (runners.length === 0) {
	console.error(`No running runner found in the compose project "${args.projectName}".`);
	console.error("Bring the fleet up first, then run this again.");
	process.exit(1);
}

// One build per architecture present, not one per container: a two-runner fleet
// on one machine is the normal case and cross-compiling twice for it would
// double the slowest step for nothing.
const platforms = new Map();
for (const runner of runners) {
	runner.platform = platformOf(runner.name);
	runner.home = homeOf(runner.name);
	if (!platforms.has(runner.platform)) {
		// The bundler's own default location for that platform, so a bundle built
		// by hand and one built from here are the same directory.
		platforms.set(runner.platform, path.join(repoRoot, ".dev", "runner-bundle", runner.platform));
	}
}

for (const [platform, outDir] of platforms) {
	const build = spawnSync(
		process.execPath,
		[path.join(__dirname, "build-runner-bundle.mjs"), "--platform", platform, "--out", outDir],
		{ cwd: repoRoot, stdio: "inherit" },
	);
	if (build.status !== 0) {
		console.error(`Building the ${platform} bundle failed.`);
		process.exit(1);
	}
}

// The wrapper is what puts `archetipo` on the PATH of every session without the
// fleet having to declare an environment variable for it. ARCHETIPO_DATA_DIR is
// set here, next to the binary it belongs to, so the bundle stays one movable
// thing: nothing outside /opt/archetipo has to know where it was mounted.
const wrapper = `#!/bin/sh
# Installed by ARchetipo's scripts/install-runners.mjs — do not edit.
ARCHETIPO_DATA_DIR=${INSTALL_ROOT}
export ARCHETIPO_DATA_DIR
exec ${INSTALL_ROOT}/bin/archetipo "$@"
`;
const stagingDir = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-runners-"));
const wrapperPath = path.join(stagingDir, "archetipo");
await fs.writeFile(wrapperPath, wrapper, { mode: 0o755 });

/**
 * Merges our tags into whatever the file already claims.
 *
 * Both tags are written, and they answer different questions. `archetipo` is
 * what a workspace requires: it has to stay the same string across every
 * rebuild or the requirement would have to be rewritten on every commit.
 * `archetipo:<version>` is what makes a fleet running yesterday's build visible
 * as such, instead of failing in a way that looks like a bug in the skill.
 *
 * Every previous `archetipo*` tag is dropped first, so a reinstall replaces the
 * version rather than leaving the fleet advertising two of them.
 */
function capabilitiesFor(container, home) {
	const read = docker(["exec", container, "cat", `${home}/.arcipelago/runner.json`]);
	let existing = {};
	if (read.status === 0) {
		try {
			existing = JSON.parse(read.stdout);
		} catch {
			console.error(`${container}: ${home}/.arcipelago/runner.json is not valid JSON; replacing it.`);
		}
	}
	const kept = (Array.isArray(existing.capabilities) ? existing.capabilities : []).filter(
		(tag) => typeof tag === "string" && tag !== "archetipo" && !tag.startsWith("archetipo:"),
	);
	return { ...existing, capabilities: [...kept, "archetipo", `archetipo:${version}`] };
}

for (const runner of runners) {
	const { name, home, platform } = runner;
	const bundleDir = platforms.get(platform);

	// Cleared, not overwritten: `docker cp` merges into what is there, so a
	// skill deleted or renamed in this working copy would otherwise survive on
	// the runner and keep being offered to the agent.
	exec(name, `rm -rf ${INSTALL_ROOT} && mkdir -p ${INSTALL_ROOT}`, "clearing the previous install");
	dockerOrDie(["cp", `${bundleDir}/.`, `${name}:${INSTALL_ROOT}`], `copying the bundle to ${name}`);
	dockerOrDie(["cp", wrapperPath, `${name}:${WRAPPER}`], `installing the wrapper in ${name}`);
	exec(
		name,
		`chmod -R a+rX ${INSTALL_ROOT} && chmod a+rx ${INSTALL_ROOT}/bin/archetipo ${WRAPPER}`,
		"making the install readable",
	);

	// A symlink and not a copy: the skills the agent loads and the skills the
	// CLI would install are then the same files, and one of them cannot go
	// stale while the other is refreshed.
	exec(
		name,
		`mkdir -p ${home}/.pi/agent && rm -rf ${home}/.pi/agent/skills && ` +
			`ln -s ${INSTALL_ROOT}/skills ${home}/.pi/agent/skills`,
		"linking the skills",
	);

	const configPath = path.join(stagingDir, `${name}.runner.json`);
	await fs.writeFile(configPath, `${JSON.stringify(capabilitiesFor(name, home), null, 2)}\n`, { mode: 0o644 });
	exec(name, `mkdir -p ${home}/.arcipelago`, "creating the config directory");
	dockerOrDie(["cp", configPath, `${name}:${home}/.arcipelago/runner.json`], `writing the capability in ${name}`);

	console.log(`✓ ${name} (${platform}) ← ${version}`);
}

await fs.rm(stagingDir, { recursive: true, force: true });

if (!args.restart) {
	console.log("\nNot restarting (--no-restart). The fleet carries the tooling but does not");
	console.log("advertise it yet: a runner reads its capabilities once, at startup.");
	process.exit(0);
}

console.log("\nRestarting, so the fleet advertises what it now carries:\n");
for (const runner of runners) {
	dockerOrDie(["restart", runner.name], `restarting ${runner.name}`);
	console.log(`  ${runner.name} restarted`);
}

// The last word is the machine's, not ours. Everything above reports what was
// asked of Docker; this reports what the runner can actually run, which is the
// only claim worth printing.
console.log("");
let ok = true;
for (const runner of runners) {
	const reported = docker(["exec", runner.name, "archetipo", "version"]);
	const line = `${reported.stdout ?? ""}${reported.stderr ?? ""}`.trim().split("\n")[0] ?? "";
	if (reported.status !== 0) {
		ok = false;
		console.error(`✗ ${runner.name}: \`archetipo version\` exited ${reported.status}: ${line}`);
		continue;
	}
	if (!line.includes(version)) {
		ok = false;
		console.error(`✗ ${runner.name} reports "${line}", expected ${version}`);
		continue;
	}
	console.log(`✓ ${runner.name}: ${line}`);
}
if (!ok) process.exit(1);

console.log("\nThe fleet now advertises `archetipo` and `archetipo:" + version + "`.");
console.log("A workspace reaches it with:\n");
console.log("  arcipelago workspaces update <workspace> --requires archetipo\n");
