#!/usr/bin/env node
// build-runner-bundle.mjs
//
// Stages a self-contained ARchetipo CLI bundle for a foreign platform — the one
// an ARcipelago runner container runs on — without publishing anything and
// without touching the tracked files under npm/.
//
// The bundle is deliberately *not* an npm install: a container that only has to
// invoke `archetipo` does not need the Node shim nor the optional-dependency
// dance. It needs the native binary, the packaged skills and the runtime
// assets, plus ARCHETIPO_DATA_DIR pointing at the bundle root — which is
// exactly what the shim sets before spawning the binary
// (npm/archetipo/bin/archetipo.js).
//
// Usage:
//   node scripts/build-runner-bundle.mjs [--platform linux-arm64] [--out <dir>]
//
// Output layout (default .dev/runner-bundle/):
//   bin/archetipo        → Go binary cross-compiled for the target platform
//   skills/              → packaged ARchetipo skills
//   runtime/             → config.yaml + shared-runtime.md
//
// Mount it into the runner and export:
//   ARCHETIPO_DATA_DIR=<bundle>   PATH=<bundle>/bin:$PATH

import fs from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");

const supported = new Set([
	"darwin-arm64", "darwin-x64",
	"linux-arm64", "linux-x64",
	"win32-arm64", "win32-x64",
]);

function parseArgs(argv) {
	const args = { platform: "linux-arm64", out: null };
	for (let i = 0; i < argv.length; i++) {
		const flag = argv[i];
		if (flag === "--platform" || flag === "-p") args.platform = argv[++i];
		else if (flag === "--out" || flag === "-o") args.out = argv[++i];
		else {
			console.error(`unknown argument: ${flag}`);
			process.exit(1);
		}
	}
	if (!supported.has(args.platform)) {
		console.error(`unsupported platform ${args.platform}; expected one of ${[...supported].join(", ")}`);
		process.exit(1);
	}
	args.out ??= path.join(repoRoot, ".dev", "runner-bundle");
	return args;
}

function run(cmd, cmdArgs, opts = {}) {
	const result = spawnSync(cmd, cmdArgs, { cwd: repoRoot, encoding: "utf8", ...opts });
	if (result.error) {
		console.error(`${cmd} failed to start: ${result.error.message}`);
		process.exit(1);
	}
	if (result.status !== 0) {
		if (result.stderr) process.stderr.write(result.stderr);
		console.error(`${cmd} ${cmdArgs.join(" ")} exited with status ${result.status}.`);
		process.exit(1);
	}
	return result;
}

async function exists(p) {
	try { await fs.access(p); return true; } catch { return false; }
}

// The version string is the same one install:dev stamps, so a bundle and a
// global dev install built from the same commit report the same version and a
// mismatch between host and runner is visible in `archetipo version`.
function devVersion() {
	const sha = run("git", ["rev-parse", "--short", "HEAD"]).stdout.trim();
	const dirty = spawnSync("git", ["diff-index", "--quiet", "HEAD", "--"], { cwd: repoRoot }).status !== 0;
	return `0.0.0-dev.g${sha}${dirty ? ".dirty" : ""}`;
}

const args = parseArgs(process.argv.slice(2));
const [goos, goarch] = args.platform.split("-");
const version = devVersion();
const outDir = path.resolve(args.out);

await fs.rm(outDir, { recursive: true, force: true });
await fs.mkdir(path.join(outDir, "bin"), { recursive: true });

console.log(`Building ${args.platform} binary @ ${version}`);
run(
	"go",
	[
		"build",
		"-o", path.join(outDir, "bin", goos === "win32" ? "archetipo.exe" : "archetipo"),
		"-ldflags", `-s -w -X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=${version}`,
		"./cmd/archetipo",
	],
	{
		cwd: path.join(repoRoot, "cli"),
		stdio: "inherit",
		// win32 is npm's spelling, not Go's.
		env: { ...process.env, GOOS: goos === "win32" ? "windows" : goos, GOARCH: goarch, CGO_ENABLED: "0" },
	},
);

await fs.cp(path.join(repoRoot, "skills"), path.join(outDir, "skills"), { recursive: true });
await fs.mkdir(path.join(outDir, "runtime"), { recursive: true });
for (const name of ["config.yaml", "shared-runtime.md"]) {
	const src = path.join(repoRoot, ".archetipo", name);
	if (await exists(src)) await fs.copyFile(src, path.join(outDir, "runtime", name));
}

console.log(`✓ ${path.relative(repoRoot, outDir)}/ (${args.platform}, ${version})`);
console.log("  mount it and export ARCHETIPO_DATA_DIR=<mount> and PATH=<mount>/bin:$PATH");
