import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { normalizeConfig, portableProjectRelativePath } from "./config.mjs";

const configPath = "/fixture/run.yaml";
const stringArrayFields = [
  "seed_reviewed_pages",
  "reviewed_pages",
  "exact_reviewed_pages",
  "review_commit_pages",
  "affected_only_reconfirmed_pages",
  "context_fresh_pages",
  "unchanged_review_metadata_pages",
  "changed_review_metadata_pages",
  "output_includes",
  "reconfirm_output_pages",
];

function manifest(reviewOverrides = {}) {
  return {
    agents: {
      test: { tool: "pi", command: "pi", args: ["--print", "{prompt}"] },
    },
    scenarios: {
      review: {
        agent: "test",
        prompts: [],
        verify_review_wiki: {
          spec_code: "US-001",
          branch: "archetipo/US-001",
          worktree: ".archetipo/worktrees/US-001",
          require_clean: true,
          ...reviewOverrides,
        },
      },
    },
  };
}

function normalize(reviewOverrides = {}) {
  return normalizeConfig(manifest(reviewOverrides), configPath)[0].verify_review_wiki;
}

test("normalizeConfig preserves unknown verify_review_wiki extension keys", () => {
  const extension = { nested: true };
  const review = normalize({ future_extension: extension });
  assert.deepEqual(review.future_extension, extension);
});

test("normalizeConfig validates all known review string-array fields", async (t) => {
  for (const field of stringArrayFields) {
    await t.test(field, () => {
      assert.deepEqual(normalize({ [field]: ["value"] })[field], ["value"]);
      assert.throws(
        () => normalize({ [field]: [""] }),
        new RegExp(`verify_review_wiki\\.${field} must be a list of non-empty strings`),
      );
      assert.throws(
        () => normalize({ [field]: "value" }),
        new RegExp(`verify_review_wiki\\.${field} must be a list of non-empty strings`),
      );
    });
  }
});

test("normalizeConfig requires spec_code, branch, worktree, and boolean require_clean", async (t) => {
  for (const field of ["spec_code", "branch", "worktree"]) {
    await t.test(field, () => {
      assert.throws(() => normalize({ [field]: "  " }), new RegExp(`verify_review_wiki\\.${field} must be`));
      assert.throws(() => normalize({ [field]: 1 }), new RegExp(`verify_review_wiki\\.${field} must be`));
    });
  }
  assert.throws(() => normalize({ require_clean: "true" }), /verify_review_wiki\.require_clean must be a boolean/);
  const withoutRequireClean = manifest();
  delete withoutRequireClean.scenarios.review.verify_review_wiki.require_clean;
  assert.throws(() => normalizeConfig(withoutRequireClean, configPath), /verify_review_wiki\.require_clean must be a boolean/);
});

function normalizeScenario(overrides) {
  return normalizeConfig({
    agents: { test: { tool: "pi", command: "pi", args: ["--print", "{prompt}"] } },
    scenarios: { autopilot: { agent: "test", prompts: [], ...overrides } },
  }, configPath)[0];
}

test("verify_spec_status defaults to empty and requires non-empty status strings", () => {
  assert.deepEqual(Object.entries(normalizeScenario({}).verify_spec_status), []);
  assert.deepEqual(
    Object.entries(normalizeScenario({ verify_spec_status: { "US-001": "DONE" } }).verify_spec_status),
    [["US-001", "DONE"]],
  );
  assert.throws(() => normalizeScenario({ verify_spec_status: ["US-001"] }), /verify_spec_status must be a spec-code-to-status object/);
  assert.throws(() => normalizeScenario({ verify_spec_status: { "US-001": "  " } }), /verify_spec_status\.US-001 must be a non-empty string/);
});

test("verify_worktree_cleanup requires an explicit spec, branch, and portable worktree per entry", () => {
  assert.deepEqual(normalizeScenario({}).verify_worktree_cleanup, []);
  assert.deepEqual(
    normalizeScenario({ verify_worktree_cleanup: [{ spec: "US-001", branch: "archetipo/US-001", worktree: ".archetipo\\worktrees\\US-001" }] }).verify_worktree_cleanup,
    [{ spec: "US-001", branch: "archetipo/US-001", worktree: ".archetipo/worktrees/US-001" }],
  );
  assert.throws(() => normalizeScenario({ verify_worktree_cleanup: { spec: "US-001" } }), /verify_worktree_cleanup must be a list of objects/);
  assert.throws(
    () => normalizeScenario({ verify_worktree_cleanup: [{ spec: "US-001", branch: "b", worktree: "../outside" }] }),
    /verify_worktree_cleanup\[0\]\.worktree must be a safe portable project-relative path/,
  );
  assert.throws(
    () => normalizeScenario({ verify_worktree_cleanup: [{ branch: "b", worktree: "w" }] }),
    /verify_worktree_cleanup\[0\]\.spec must be a non-empty string/,
  );
});

test("branch uses its non-empty Git-ref string contract", () => {
  assert.equal(normalize({ branch: "release/candidate" }).branch, "release/candidate");
});

test("portable path validator accepts canonical relative paths and normalizes separators", () => {
  assert.equal(portableProjectRelativePath("docs/wiki/page.md", "field", configPath), "docs/wiki/page.md");
  assert.equal(portableProjectRelativePath("docs\\wiki\\page.md", "field", configPath), "docs/wiki/page.md");
});

test("portable path validator rejects unsafe and non-portable forms", async (t) => {
  const invalid = [
    "",
    "   ",
    "/absolute/path",
    "C:/absolute/path",
    "C:drive-relative",
    "\\\\server\\share\\path",
    "\\\\?\\C:\\extended",
    "\\\\.\\NUL",
    "../outside",
    "safe/../../outside",
    "safe\\..\\outside",
    "file.txt:stream",
    "NUL",
    "con.txt",
    "trailing. ",
    "bad?.txt",
  ];
  for (const value of invalid) {
    await t.test(JSON.stringify(value), () => {
      assert.throws(
        () => portableProjectRelativePath(value, "review.path", configPath),
        /review\.path must be a safe portable project-relative path/,
      );
    });
  }
});

test("every known path and page field uses the same portable validator", async (t) => {
  const cases = [
    ["worktree", "../outside"],
    ["wiki_root", "/absolute/wiki"],
    ["affected_file", "C:\\outside.txt"],
    ["seed_baseline_paths", ["safe", "\\\\server\\share"]],
    ["implemented_file_contents", { "file.txt:stream": "content" }],
    ["seed_reviewed_pages", ["../outside"]],
    ["review_commit_pages", ["safe\\..\\outside"]],
  ];
  for (const [field, value] of cases) {
    await t.test(field, () => {
      assert.throws(() => normalize({ [field]: value }), /must be a safe portable project-relative path/);
    });
  }
});

test("output_includes remains arbitrary non-empty expected text rather than a path", () => {
  assert.deepEqual(normalize({ output_includes: ["warning: behavior / references/prd"] }).output_includes, ["warning: behavior / references/prd"]);
});

test("implemented_file_contents requires string values and retains exact contents", () => {
  const review = normalize({ implemented_file_contents: { "hello.txt": "", "nested/file.txt": "value" } });
  assert.deepEqual(Object.fromEntries(Object.entries(review.implemented_file_contents)), { "hello.txt": "", "nested/file.txt": "value" });
  assert.throws(
    () => normalize({ implemented_file_contents: { "hello.txt": 42 } }),
    /implemented_file_contents\.hello\.txt must be a string/,
  );
  assert.throws(
    () => normalize({ implemented_file_contents: [] }),
    /implemented_file_contents must be a path-to-content object/,
  );
  assert.throws(
    () => normalize({ implemented_file_contents: { "nested/file.txt": "one", "nested\\file.txt": "two" } }),
    /implemented_file_contents contains duplicate portable path nested\/file\.txt/,
  );
});

test("implemented_file_contents retains __proto__ for downstream content verification", async (t) => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-config-proto-"));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  await fs.writeFile(path.join(root, "__proto__"), "expected content");
  const configured = normalize({ implemented_file_contents: JSON.parse('{"__proto__":"expected content"}') }).implemented_file_contents;

  assert.equal(Object.getPrototypeOf(configured), null);
  assert.deepEqual(Object.entries(configured), [["__proto__", "expected content"]]);
  for (const [file, expected] of Object.entries(configured)) {
    assert.equal(await fs.readFile(path.join(root, file), "utf8"), expected);
  }
});

test("importing config validation has no runner side effects", () => {
  assert.equal(typeof normalizeConfig, "function");
});
