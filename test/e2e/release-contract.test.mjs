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

  const verify = steps.find((step) => step.name === "Verify anonymous access to the published manifest");
  assert.match(verify.run, /docker logout ghcr\.io/);
  assert.match(verify.run, /imagetools inspect/);
});

test("the isolated image embeds the release version in its CLI", async () => {
  const dockerfile = await readFile(new URL("docker/isolated/Dockerfile", root), "utf8");
  assert.match(dockerfile, /ARG VERSION=dev/);
  assert.match(dockerfile, /internal\/version\.Version=\$\{VERSION\}/);
  assert.match(dockerfile, /org\.opencontainers\.image\.source=.*techreloaded-ar\/ARchetipo/);
});
