// go.mjs
//
// Go toolchain preflight shared by the dev scripts that build the CLI.
//
// Without it a missing toolchain surfaces as a bare "spawnSync go ENOENT" in
// the middle of the build: it says neither what to install nor why a `go` that
// works in the terminal is invisible here (npm run inherits the PATH of the
// shell it was started from).

import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const isWindows = process.platform === "win32";

// Where Go commonly lands but does not always reach the PATH of a non-login
// shell — the one an IDE terminal hands to npm run.
function commonGoBinaries() {
	if (isWindows) {
		const localAppData = process.env.LOCALAPPDATA ?? "";
		return [
			"C:\\Program Files\\Go\\bin\\go.exe",
			localAppData ? path.join(localAppData, "Programs", "Go", "bin", "go.exe") : "",
		].filter(Boolean);
	}
	return [
		"/usr/local/go/bin/go",   // official tarball / pkg
		"/opt/homebrew/bin/go",   // Homebrew on Apple Silicon
		"/usr/local/bin/go",      // Homebrew on Intel
		"/opt/go/bin/go",
		"/snap/bin/go",
		"/usr/lib/go/bin/go",
	];
}

function installHint() {
	if (isWindows) {
		return ["  winget install GoLang.Go", "  (or the .msi installer from https://go.dev/dl/)"];
	}
	if (process.platform === "darwin") {
		return ["  brew install go", "  (or the .pkg installer from https://go.dev/dl/)"];
	}
	return ["  your distribution package, or the tarball from https://go.dev/dl/"];
}

/** Go version required by cli/go.mod, or null when it cannot be read. */
export function goModVersion(repoRoot) {
	try {
		const goMod = fs.readFileSync(path.join(repoRoot, "cli", "go.mod"), "utf8");
		return goMod.match(/^go\s+(\S+)/m)?.[1] ?? null;
	} catch {
		return null;
	}
}

/**
 * Check that `go` runs before any build starts. On failure prints an
 * actionable diagnosis and exits 1. Returns the `go version` output.
 */
export function requireGo(repoRoot) {
	const probe = spawnSync("go", ["version"], { encoding: "utf8" });
	if (!probe.error && probe.status === 0) return probe.stdout.trim();

	const required = goModVersion(repoRoot);
	const lines = [""];

	if (probe.error && probe.error.code === "ENOENT") {
		lines.push("Go toolchain not found: `go` is not on this process's PATH.");
	} else if (probe.error) {
		lines.push(`Could not run \`go\`: ${probe.error.message}`);
	} else {
		lines.push(`\`go version\` exited with status ${probe.status} — the Go install looks broken.`);
		if (probe.stderr) lines.push(probe.stderr.trim());
	}

	lines.push(
		"",
		`The ARchetipo CLI is built from source, so Go${required ? ` ${required} or newer` : ""} is required.`,
		"",
	);

	const found = commonGoBinaries().find((p) => fs.existsSync(p));
	if (found) {
		const dir = path.dirname(found);
		lines.push(
			`Go is installed at ${found}, but ${dir} is not on your PATH. Add it:`,
			"",
			isWindows ? `  $env:PATH = "${dir};$env:PATH"` : `  export PATH="${dir}:$PATH"`,
			"",
			isWindows
				? "To make it permanent, add the line to your PowerShell profile ($PROFILE)."
				: "To make it permanent, add the line to ~/.zshrc (macOS default) or ~/.bashrc.",
		);
	} else {
		lines.push("Install it:", "", ...installHint());
	}

	lines.push(
		"",
		"Then verify with:  go version",
		"",
		"Note: `npm run` inherits the PATH of the shell it was started from. If `go version`",
		"works in your terminal but this script disagrees, npm is running from a different",
		"environment (an IDE terminal, another shell profile, or sudo).",
		"",
	);

	console.error(lines.join("\n"));
	process.exit(1);
}
