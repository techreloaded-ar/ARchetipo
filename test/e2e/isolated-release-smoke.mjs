import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";

const version = process.argv[2];
assert.match(version ?? "", /^\d+\.\d+\.\d+(?:-[a-z][a-z0-9.-]*)?$/, "Pass the expected npm release version");
assert.ok(process.env.npm_execpath, "Run via npm run test:view-isolated-release-smoke");
const channel = version.split("-")[1]?.split(".")[0] ?? "latest";
const pkg = `@techreloaded/archetipo@${channel}`;
const image = `ghcr.io/techreloaded-ar/archetipo-isolated:${version}`;
const windows = process.platform === "win32";
const environment = `release-smoke-${randomBytes(6).toString("hex")}`;

const root = mkdtempSync(join(tmpdir(), "archetipo-release-"));
const prefix = join(root, "npm");
const bin = windows ? prefix : join(prefix, "bin");
const state = join(root, "state");
const env = {
  ...process.env,
  PATH: `${bin}${delimiter}${process.env.PATH}`,
  ARCHETIPO_STATE_DIR: state,
};
delete env.ARCHETIPO_ISOLATED_IMAGE;
delete env.ARCHETIPO_ISOLATED_FIRST_PORT;
delete env.ARCHETIPO_CONTAINER_ENGINE;

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: root, env, encoding: "utf8", ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} exited with ${result.status}\n${result.stderr ?? ""}${result.stdout ?? ""}`);
  }
  return result.stdout?.trim();
}

function launcher(args, options) {
  if (windows) {
    return run("cmd.exe", ["/d", "/c", "archetipo", ...args], options);
  }
  return run(join(bin, "archetipo"), args, options);
}

try {
  console.log(`Host ${process.platform}/${process.arch}, Docker ${run("docker", ["info", "--format", "{{.OSType}}/{{.Architecture}} {{.ServerVersion}}"])}`);
  const images = run("docker", ["image", "ls", "--format", "{{.Repository}}:{{.Tag}}"]);
  assert.doesNotMatch(images, /^(?:ghcr\.io\/techreloaded-ar\/)?archetipo-isolated:/m, "Use a Docker engine without ARchetipo images");
  const label = "label=archetipo.environment";
  assert.equal(run("docker", ["container", "ls", "--all", "--quiet", "--filter", label]), "", "Use a Docker engine without ARchetipo containers");
  assert.equal(run("docker", ["volume", "ls", "--quiet", "--filter", label]), "", "Use a Docker engine without ARchetipo volumes");

  run(process.execPath, [process.env.npm_execpath, "install", "--global", "--prefix", prefix, "--prefer-online", "--registry", "https://registry.npmjs.org", pkg], { stdio: "inherit" });
  assert.ok(existsSync(join(bin, windows ? "archetipo.cmd" : "archetipo")), "npm should install the launcher");
  assert.equal(launcher(["--version"]), `archetipo ${version}`);
  launcher(["view", "--isolated", "--environment", environment, "--no-open"], { stdio: "inherit" });

  const imageID = run("docker", ["image", "inspect", image, "--format", "{{.Id}}"]);
  const workspace = `archetipo-${environment}-workspace`;
  assert.equal(run("docker", ["container", "inspect", workspace, "--format", "{{.Image}}"]), imageID);
  const response = await fetch("http://127.0.0.1:47800/api/workspaces", { signal: AbortSignal.timeout(5000) });
  assert.equal(response.status, 200);
  assert.equal((await response.json()).cloneAvailable, true);
  console.log(`${pkg} ${version} automatically pulled ${image} and the viewer answered HTTP ${response.status}.`);
} finally {
  try {
    if (existsSync(join(state, "environments.json"))) {
      launcher(["view", "remove", "--isolated", "--environment", environment, "--yes"], { stdio: "inherit" });
      if (run("docker", ["image", "ls", "--quiet", image])) {
        run("docker", ["image", "rm", image], { stdio: "inherit" });
      }
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}
