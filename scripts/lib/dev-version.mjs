// dev-version.mjs
//
// The one definition of the version a build made from a working copy carries:
// 0.0.0-dev.g<short-sha>[.dirty].
//
// It lives on its own because three things stamp it — the global install, the
// runner bundle, and the capability a runner advertises — and the whole point
// of the string is that a host and a runner built from the same commit report
// the same one. Two copies of the rule would eventually disagree, and the
// mismatch it exists to reveal would be the mismatch it invented.

import { spawnSync } from "node:child_process";

export function devVersion(repoRoot) {
	const sha = spawnSync("git", ["rev-parse", "--short", "HEAD"], { cwd: repoRoot, encoding: "utf8" });
	if (sha.status !== 0) {
		if (sha.stderr) process.stderr.write(sha.stderr);
		console.error("Deriving the dev version needs a git checkout.");
		process.exit(1);
	}

	// Exit code 1 = tracked files (staged or unstaged) differ from HEAD.
	// Untracked files are ignored on purpose: scratch files must not mark
	// every build as dirty.
	const dirty = spawnSync("git", ["diff-index", "--quiet", "HEAD", "--"], { cwd: repoRoot }).status !== 0;

	// The "g" prefix (git-describe convention) keeps the prerelease identifier
	// alphanumeric: an all-digit sha with a leading zero would be invalid semver.
	return `0.0.0-dev.g${sha.stdout.trim()}${dirty ? ".dirty" : ""}`;
}
