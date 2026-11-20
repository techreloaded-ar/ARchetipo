#!/usr/bin/env node
// wait-npm-release.mjs <version> [--timeout-seconds <n>] [--interval-seconds <n>]
//
// Waits until the npm registry, as seen from this machine, serves the release
// that publish-npm.mjs just published: for every package under /npm/, the
// dist-tag derived from <version> must point at <version> and the version
// itself must be listed. Both the abbreviated document `npm install` reads and
// the full document `npm view` reads are checked, because the registry caches
// them separately.
//
// npm acknowledges a publish before the new dist-tag is visible to readers
// ("Your package is being processed and may take a few minutes to become
// available"), so a job that installs `@<dist-tag>` right after the publish
// can silently get the previous release. The post-release jobs of release.yml
// call this script first, so they verify the release they were started for.
//
// Exits 0 when every package is visible, 1 when the timeout elapses (naming
// what the registry still serves), 2 on invalid arguments.

import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const npmDir = path.join(path.resolve(__dirname, ".."), "npm");
const registry = "https://registry.npmjs.org";

function numberOption(name, fallback) {
  const index = process.argv.indexOf(name);
  if (index === -1) return fallback;
  const value = Number(process.argv[index + 1]);
  if (!Number.isFinite(value) || value <= 0) {
    console.error(`✗ ${name} needs a positive number`);
    process.exit(2);
  }
  return value;
}

const expectedVersion = String(process.argv[2] ?? "").replace(/^v/, "");
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(expectedVersion)) {
  console.error("usage: wait-npm-release.mjs <version> [--timeout-seconds <n>] [--interval-seconds <n>]");
  process.exit(2);
}
const timeoutSeconds = numberOption("--timeout-seconds", 600);
const intervalSeconds = numberOption("--interval-seconds", 10);

// Same derivation as publish-npm.mjs: prerelease identifier or "latest".
const prerelease = expectedVersion.split("-")[1];
const distTag = prerelease ? prerelease.split(".")[0] : "latest";

const packageNames = [];
for (const entry of await fs.readdir(npmDir, { withFileTypes: true })) {
  if (!entry.isDirectory() || !entry.name.startsWith("archetipo")) continue;
  const manifest = JSON.parse(await fs.readFile(path.join(npmDir, entry.name, "package.json"), "utf8"));
  packageNames.push(manifest.name);
}
if (packageNames.length === 0) {
  console.error(`✗ no npm packages found under ${npmDir}`);
  process.exit(2);
}

const documentKinds = [
  { label: "install", accept: "application/vnd.npm.install-v1+json" },
  { label: "full", accept: "application/json" },
];

// Returns null when the package is visible, otherwise what the registry serves.
async function pendingReason(packageName) {
  const url = `${registry}/${packageName.replace("/", "%2f")}`;
  for (const kind of documentKinds) {
    let response;
    try {
      response = await fetch(url, {
        headers: { accept: kind.accept, "cache-control": "no-cache" },
        signal: AbortSignal.timeout(15000),
      });
    } catch (error) {
      return `${kind.label} document unreachable (${error.message})`;
    }
    if (!response.ok) return `${kind.label} document answered HTTP ${response.status}`;
    const document = await response.json();
    const tagged = document["dist-tags"]?.[distTag];
    if (tagged !== expectedVersion) return `${kind.label} document has ${distTag}=${tagged ?? "(missing)"}`;
    if (!document.versions?.[expectedVersion]) return `${kind.label} document does not list ${expectedVersion}`;
  }
  return null;
}

console.log(`Waiting for ${packageNames.length} npm package(s) to serve ${distTag}=${expectedVersion} (timeout ${timeoutSeconds}s).`);
const deadline = Date.now() + timeoutSeconds * 1000;
let pending = new Map();
for (;;) {
  const reasons = await Promise.all(packageNames.map(pendingReason));
  pending = new Map(packageNames.map((name, index) => [name, reasons[index]]).filter(([, reason]) => reason));
  if (pending.size === 0) {
    console.log(`✓ every package serves ${distTag}=${expectedVersion}.`);
    process.exit(0);
  }
  if (Date.now() + intervalSeconds * 1000 > deadline) break;
  console.log(`… ${pending.size} package(s) not visible yet: ${[...pending.keys()].join(", ")}`);
  await new Promise((resolve) => setTimeout(resolve, intervalSeconds * 1000));
}

console.error(`✗ after ${timeoutSeconds}s the registry still does not serve ${distTag}=${expectedVersion}:`);
for (const [name, reason] of pending) console.error(`  ${name}: ${reason}`);
process.exit(1);
