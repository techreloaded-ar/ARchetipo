#!/usr/bin/env node
// build-cli-dev.mjs
//
// Builds a native Go binary for the current platform only (dev mode) and
// creates wrapper scripts under .dev/bin/ so a target project can use the dev
// CLI by prepending .dev/bin/ to PATH.
//
// Output layout:
//   .dev/native/archetipo[.exe]   → Go binary with dev-local version
//   .dev/bin/archetipo.cmd        → Windows wrapper (sets ARCHETIPO_DATA_DIR)
//   .dev/bin/archetipo-dev.cmd    → alias to archetipo.cmd
//   .dev/bin/archetipo.sh         → Unix wrapper
//   .dev/bin/archetipo-dev.sh     → alias to archetipo.sh

import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");

const nativeDir = path.join(repoRoot, ".dev", "native");
const binDir = path.join(repoRoot, ".dev", "bin");

const isWindows = process.platform === "win32";
const exeName = isWindows ? "archetipo.exe" : "archetipo";
const exePath = path.join(nativeDir, exeName);

async function main() {
	// 1. Ensure output directories
	await fs.mkdir(nativeDir, { recursive: true });
	await fs.mkdir(binDir, { recursive: true });

	// 2. Build Go binary
	console.log("Building dev CLI for", `${os.platform()}-${os.arch()}`);
	const ldflags =
		"-s -w -X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=dev-local";

	const result = spawnSync(
		"go",
		["build", "-o", exePath, "-ldflags", ldflags, "./cmd/archetipo"],
		{ cwd: path.join(repoRoot, "cli"), stdio: "inherit" },
	);

	if (result.status !== 0) {
		console.error("go build failed — is Go installed and on PATH?");
		process.exit(1);
	}

	// Make binary executable on Unix
	if (!isWindows) {
		await fs.chmod(exePath, 0o755);
	}

	console.log(`  ✓ ${path.relative(repoRoot, exePath)}`);

	// 3. Write wrapper scripts
	if (isWindows) {
		await writeWindowsWrappers();
	} else {
		await writeUnixWrappers();
	}

	// 4. Print instructions
	printInstructions();
}

// ---------------------------------------------------------------------------
// Windows wrappers (.cmd)
// ---------------------------------------------------------------------------

async function writeWindowsWrappers() {
	// %%~fI resolves to the full absolute path → clean env var
	const cmd = [
		`@echo off`,
		`for %%I in ("%~dp0..\\..") do set "ARCHETIPO_DATA_DIR=%%~fI"`,
		`"%~dp0..\\native\\archetipo.exe" %*`,
		"",
	].join("\r\n");

	const cmdPath = path.join(binDir, "archetipo.cmd");
	await fs.writeFile(cmdPath, cmd);
	await fs.writeFile(path.join(binDir, "archetipo-dev.cmd"), cmd);

	console.log(`  ✓ .dev/bin/archetipo.cmd`);
	console.log(`  ✓ .dev/bin/archetipo-dev.cmd`);
}

// ---------------------------------------------------------------------------
// Unix wrappers (.sh)
// ---------------------------------------------------------------------------

async function writeUnixWrappers() {
	const sh = [
		`#!/usr/bin/env bash`,
		`set -euo pipefail`,
		`SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"`,
		`export ARCHETIPO_DATA_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"`,
		`exec "$SCRIPT_DIR/../native/archetipo" "$@"`,
		"",
	].join("\n");

	const shPath = path.join(binDir, "archetipo.sh");
	await fs.writeFile(shPath, sh);
	await fs.chmod(shPath, 0o755);

	const devShPath = path.join(binDir, "archetipo-dev.sh");
	await fs.writeFile(devShPath, sh);
	await fs.chmod(devShPath, 0o755);

	console.log(`  ✓ .dev/bin/archetipo.sh`);
	console.log(`  ✓ .dev/bin/archetipo-dev.sh`);
}

// ---------------------------------------------------------------------------
// Usage instructions
// ---------------------------------------------------------------------------

function printInstructions() {
	const hr = "─".repeat(60);
	console.log();
	console.log(hr);
	console.log("Dev CLI ready.");
	console.log();
	console.log("Direct invocation:");
	if (isWindows) {
		console.log(`  set "ARCHETIPO_DATA_DIR=${repoRoot}"`);
		console.log(`  ${exePath} version`);
	} else {
		console.log(`  ARCHETIPO_DATA_DIR="${repoRoot}" ${exePath} version`);
	}
	console.log();
	console.log("Via PATH (recommended for skill testing):");
	if (isWindows) {
		console.log(`  $env:PATH = "${repoRoot}\\.dev\\bin;$env:PATH"`);
		console.log(`  archetipo version`);
		console.log(`  archetipo doctor`);
	} else {
		console.log(`  export PATH="${repoRoot}/.dev/bin:$PATH"`);
		console.log(`  archetipo version`);
		console.log(`  archetipo doctor`);
	}
	console.log();
	console.log("To use in a target project:");
	console.log("  1. Open a new shell and prepend .dev/bin to PATH");
	console.log("  2. cd to the target project");
	console.log("  3. Run archetipo commands normally");
	console.log("  4. Skills already installed will invoke the dev CLI");
	console.log();
	console.log("To go back to the stable global CLI, close the shell or remove");
	console.log(".dev/bin from PATH.");
	console.log(hr);
}

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
