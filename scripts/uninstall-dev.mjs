#!/usr/bin/env node
// uninstall-dev.mjs
//
// Removes the global dev install created by scripts/install-dev.mjs.
// Both packages must go: they were installed as explicit top-level packages,
// so removing only the main one would leave the native sub-package behind.

import { spawnSync } from "node:child_process";

const isWindows = process.platform === "win32";
const subPkg = `@techreloaded/archetipo-${process.platform}-${process.arch}`;

const result = spawnSync(
	"npm",
	["uninstall", "-g", "@techreloaded/archetipo", subPkg],
	{ stdio: "inherit", shell: isWindows },
);

if (result.error) {
	console.error(`npm failed to start: ${result.error.message}`);
	process.exit(1);
}
if (result.status !== 0) {
	console.error("\nnpm uninstall -g failed.");
	process.exit(result.status ?? 1);
}

console.log("\nDev CLI removed from the global prefix.");
console.log("To install the stable release: npm install -g @techreloaded/archetipo");
