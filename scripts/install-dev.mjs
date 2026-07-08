#!/usr/bin/env node
// install-dev.mjs
//
// Builds the current repo as the @techreloaded/archetipo npm package for the
// current platform only and installs it globally, simulating what an end user
// gets from `npm install -g @techreloaded/archetipo` — without publishing and
// without touching the tracked files under npm/ (everything is staged in the
// gitignored .dev/npm-staging/ directory).
//
// The installed version is 0.0.0-dev.g<short-sha>[.dirty] so that
// `archetipo version` tells exactly which commit the global install came from.
//
// Output layout:
//   .dev/npm-staging/archetipo/                  → staged main package (shim + skills + runtime)
//   .dev/npm-staging/archetipo-<plat>-<arch>/    → staged native sub-package (Go binary)
//   .dev/npm-staging/*.tgz                       → npm pack output, installed with npm install -g

import fs from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");
const stagingDir = path.join(repoRoot, ".dev", "npm-staging");

const isWindows = process.platform === "win32";

// Same set as npm/archetipo/bin/archetipo.js — Node's platform/arch naming
// already matches the npm sub-package naming.
const supported = new Set([
	"darwin-arm64", "darwin-x64",
	"linux-arm64", "linux-x64",
	"win32-arm64", "win32-x64",
]);

function run(cmd, args, opts = {}) {
	const result = spawnSync(cmd, args, {
		cwd: repoRoot,
		encoding: "utf8",
		// npm is npm.cmd on Windows and needs a shell to resolve
		shell: isWindows && cmd === "npm",
		...opts,
	});
	if (result.error) {
		console.error(`${cmd} failed to start: ${result.error.message}`);
		process.exit(1);
	}
	return result;
}

function runOrDie(cmd, args, opts = {}, hint = "") {
	const result = run(cmd, args, opts);
	if (result.status !== 0) {
		if (result.stderr) process.stderr.write(result.stderr);
		console.error(`\n${cmd} ${args.join(" ")} exited with status ${result.status}.`);
		if (hint) console.error(hint);
		process.exit(1);
	}
	return result;
}

async function exists(p) {
	try { await fs.access(p); return true; } catch { return false; }
}

async function emptyDir(dir) {
	await fs.rm(dir, { recursive: true, force: true });
	await fs.mkdir(dir, { recursive: true });
}

async function copyDir(src, dst) {
	await fs.mkdir(dst, { recursive: true });
	const entries = await fs.readdir(src, { withFileTypes: true });
	for (const e of entries) {
		const s = path.join(src, e.name);
		const d = path.join(dst, e.name);
		if (e.isDirectory()) await copyDir(s, d);
		else if (e.isFile()) await fs.copyFile(s, d);
	}
}

function computeDevVersion() {
	const sha = runOrDie(
		"git", ["rev-parse", "--short", "HEAD"], {},
		"install:dev needs a git checkout to derive the dev version.",
	).stdout.trim();

	// Exit code 1 = tracked files (staged or unstaged) differ from HEAD.
	// Untracked files are ignored on purpose: scratch files must not mark
	// every build as dirty.
	const dirty = run("git", ["diff-index", "--quiet", "HEAD", "--"]).status !== 0;

	// The "g" prefix (git-describe convention) keeps the prerelease identifier
	// alphanumeric: an all-digit sha with a leading zero would be invalid semver.
	return `0.0.0-dev.g${sha}${dirty ? ".dirty" : ""}`;
}

function detectPlatform() {
	const key = `${process.platform}-${process.arch}`;
	if (!supported.has(key)) {
		console.error(`ARchetipo has no native package for ${key}.`);
		console.error(`Supported: ${[...supported].join(", ")}.`);
		process.exit(1);
	}
	return {
		key,
		pkgName: `@techreloaded/archetipo-${key}`,
		pkgDir: `archetipo-${key}`,
		binName: isWindows ? "archetipo.exe" : "archetipo",
	};
}

async function stagePackages(version, platform) {
	await emptyDir(stagingDir);

	// Main package: shim + package.json only. skills/ and runtime/ under
	// npm/archetipo/ may hold stale output of build-npm.mjs, so they are
	// re-synced from the sources of truth instead of copied.
	const mainSrc = path.join(repoRoot, "npm", "archetipo");
	const mainDst = path.join(stagingDir, "archetipo");
	await fs.mkdir(mainDst, { recursive: true });
	await copyDir(path.join(mainSrc, "bin"), path.join(mainDst, "bin"));
	for (const name of ["README.md"]) {
		const src = path.join(mainSrc, name);
		if (await exists(src)) await fs.copyFile(src, path.join(mainDst, name));
	}

	await copyDir(path.join(repoRoot, "skills"), path.join(mainDst, "skills"));
	await fs.mkdir(path.join(mainDst, "runtime"), { recursive: true });
	for (const name of ["config.yaml", "shared-runtime.md"]) {
		const src = path.join(repoRoot, ".archetipo", name);
		if (await exists(src)) {
			await fs.copyFile(src, path.join(mainDst, "runtime", name));
		}
	}

	const mainPkg = JSON.parse(
		await fs.readFile(path.join(mainSrc, "package.json"), "utf8"),
	);
	mainPkg.version = version;
	// Only the current platform, pinned to the exact dev version: the other
	// entries would point at versions that do not exist on the registry and
	// could make the install fail. npm satisfies this one with the sibling
	// .tgz passed on the install command line.
	mainPkg.optionalDependencies = { [platform.pkgName]: version };
	await fs.writeFile(
		path.join(mainDst, "package.json"),
		JSON.stringify(mainPkg, null, 2) + "\n",
	);

	// Native sub-package: package.json only; the binary is built next.
	const nativeSrc = path.join(repoRoot, "npm", platform.pkgDir);
	const nativeDst = path.join(stagingDir, platform.pkgDir);
	await fs.mkdir(nativeDst, { recursive: true });
	const nativePkg = JSON.parse(
		await fs.readFile(path.join(nativeSrc, "package.json"), "utf8"),
	);
	nativePkg.version = version;
	await fs.writeFile(
		path.join(nativeDst, "package.json"),
		JSON.stringify(nativePkg, null, 2) + "\n",
	);

	console.log(`✓ staged packages in ${path.relative(repoRoot, stagingDir)}/`);
	return { mainDst, nativeDst };
}

async function buildBinary(version, platform, nativeDst) {
	console.log(`Building ${platform.key} binary @ ${version}`);
	const binPath = path.join(nativeDst, "bin", platform.binName);
	await fs.mkdir(path.dirname(binPath), { recursive: true });
	const ldflags =
		`-s -w -X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=${version}`;

	runOrDie(
		"go",
		["build", "-o", binPath, "-ldflags", ldflags, "./cmd/archetipo"],
		{ cwd: path.join(repoRoot, "cli"), stdio: "inherit" },
		"go build failed — is Go installed and on PATH?",
	);

	if (!isWindows) await fs.chmod(binPath, 0o755);
	console.log(`✓ ${path.relative(repoRoot, binPath)}`);
}

function packTarball(pkgDir) {
	const result = runOrDie(
		"npm",
		["pack", "--json", "--pack-destination", stagingDir],
		{ cwd: pkgDir },
	);
	const [info] = JSON.parse(result.stdout);
	const tgz = path.join(stagingDir, info.filename);
	console.log(`✓ ${path.relative(repoRoot, tgz)}`);
	return tgz;
}

function installGlobal(tarballs) {
	console.log("\nInstalling globally (npm install -g)…");
	runOrDie(
		"npm",
		["install", "-g", ...tarballs],
		{ stdio: "inherit" },
		"Global install failed. If npm uses a system prefix you may lack " +
		"permissions: prefer a user-level prefix (nvm/volta) over sudo.",
	);
}

function verifyAndPrint(version, platform) {
	const prefix = runOrDie("npm", ["prefix", "-g"]).stdout.trim();
	const globalBin = isWindows
		? path.join(prefix, "archetipo.cmd")
		: path.join(prefix, "bin", "archetipo");

	const result = run(globalBin, ["version"], { shell: isWindows });
	const output = `${result.stdout ?? ""}${result.stderr ?? ""}`.trim();
	if (result.status !== 0 || !output.includes(version)) {
		console.error(`\nVerification failed: ${globalBin} version → "${output}"`);
		console.error(`Expected it to report ${version}.`);
		process.exit(1);
	}

	const hr = "─".repeat(60);
	console.log();
	console.log(hr);
	console.log(`Dev CLI installed globally: ${version}`);
	console.log();
	console.log(`  ${globalBin}`);
	console.log();
	console.log("Check which binary wins on your PATH (a .dev/bin or .local/bin");
	console.log("entry from the PATH-based dev flow takes precedence):");
	console.log("  which -a archetipo");
	console.log("  archetipo version");
	console.log();
	console.log("To remove:                npm run uninstall:dev");
	console.log("To restore the stable CLI: npm install -g @techreloaded/archetipo");
	console.log(hr);
}

async function main() {
	const version = computeDevVersion();
	const platform = detectPlatform();
	console.log(`Packaging ${platform.pkgName} + @techreloaded/archetipo @ ${version}\n`);

	const { mainDst, nativeDst } = await stagePackages(version, platform);
	await buildBinary(version, platform, nativeDst);

	const nativeTgz = packTarball(nativeDst);
	const mainTgz = packTarball(mainDst);

	installGlobal([nativeTgz, mainTgz]);
	verifyAndPrint(version, platform);
}

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
