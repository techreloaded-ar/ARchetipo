import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { parse } from "yaml";

const root = new URL("../../", import.meta.url);

test("a release publishes the matching multi-architecture isolated image", async () => {
  const source = await readFile(new URL(".github/workflows/release.yml", root), "utf8");
  const workflow = parse(source);
  assert.equal(workflow.permissions.packages, "write");

  const steps = workflow.jobs.release.steps;
  const publish = steps.find((step) => step.name === "Build and publish isolated runtime");
  assert.equal(publish.uses, "docker/build-push-action@v6");
  assert.equal(publish.with.push, true);
  assert.equal(publish.with.platforms, "linux/amd64,linux/arm64");
  assert.match(publish.with.tags, /isolated-image\.outputs\.image/);
  assert.match(publish.with["build-args"], /isolated-image\.outputs\.version/);

  const tag = steps.find((step) => step.name === "Resolve isolated image tag");
  assert.match(tag.run, /GITHUB_REF_NAME#v/);
  assert.match(tag.run, /ghcr\.io\/techreloaded-ar\/archetipo-isolated/);

  const verify = steps.find((step) => step.name === "Verify isolated startup with an anonymous pull");
  assert.ok(steps.indexOf(publish) < steps.indexOf(verify));
  assert.ok(steps.indexOf(verify) < steps.findIndex((step) => step.name === "Publish to npm"));
  assert.match(verify.env.RELEASE_IMAGE, /isolated-image\.outputs\.image/);
  assert.match(verify.env.RELEASE_VERSION, /isolated-image\.outputs\.version/);
  assert.match(verify.run, /docker logout ghcr\.io/);
  assert.match(verify.run, /export DOCKER_CONFIG="\$smoke_dir\/docker"/);
  assert.match(verify.run, /internal\/version\.Version=\$RELEASE_VERSION/);
  const requireMissingImage = 'if docker image inspect "$RELEASE_IMAGE"';
  const startLauncher = '"$launcher" view --isolated';
  assert.ok(verify.run.includes(requireMissingImage));
  assert.ok(verify.run.includes(startLauncher));
  assert.ok(verify.run.indexOf(requireMissingImage) < verify.run.indexOf(startLauncher));
  assert.match(verify.run, /curl --fail[^\n]*\/api\/workspaces/);

  const startup = workflow.jobs["npm-startup"];
  assert.equal(startup.needs, "release");
  assert.ok(startup.steps.some((step) => step.run?.includes("test:view-isolated-release-smoke")));
});

test("a semver preview is marked as a prerelease on GitHub", async () => {
  const goreleaser = parse(await readFile(new URL("cli/.goreleaser.yaml", root), "utf8"));
  assert.equal(goreleaser.release.prerelease, "auto");
});

test("the isolated image embeds the release version in its CLI", async () => {
  const dockerfile = await readFile(new URL("docker/isolated/Dockerfile", root), "utf8");
  assert.match(dockerfile, /ARG VERSION=dev/);
  assert.match(dockerfile, /internal\/version\.Version=\$\{VERSION\}/);
  assert.match(dockerfile, /org\.opencontainers\.image\.source=.*techreloaded-ar\/ARchetipo/);
});
