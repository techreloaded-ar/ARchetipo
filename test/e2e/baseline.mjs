import path from "node:path";

function commandFailure(phase, args, result) {
  const detail = result?.stderr || result?.stdout || `exit ${result?.code ?? "unknown"}`;
  return new Error(`Sandbox baseline phase ${phase} failed (git ${args.join(" ")}): ${detail}`);
}

async function runGitPhase(runGit, phase, args) {
  const result = await runGit(phase, args);
  if (!result?.ok) throw commandFailure(phase, args, result);
  return result;
}

export function literalGitPathspec(file) {
  return `:(literal)${file}`;
}

export async function createGitAndWikiBaselines({
  baselinePaths = [],
  seedReviewedPages = [],
  wikiRoot = "docs/wiki",
  runGit,
  approvePages,
  verifySeededPages,
}) {
  await runGitPhase(runGit, "git-repository-init", ["init", "-b", "main"]);
  await runGitPhase(runGit, "git-identity-email", ["config", "user.email", "archetipo-e2e@example.com"]);
  await runGitPhase(runGit, "git-identity-name", ["config", "user.name", "ARchetipo E2E"]);

  if (baselinePaths.length > 0) {
    await runGitPhase(runGit, "generated-baseline-stage", ["add", "--", ...baselinePaths.map(literalGitPathspec)]);
  }
  await runGitPhase(runGit, "generated-baseline-commit", ["commit", "--allow-empty", "-m", "chore: e2e generated fixture baseline"]);
  const generated = await runGitPhase(runGit, "generated-baseline-resolve", ["rev-parse", "HEAD"]);
  const generatedBaselineCommit = generated.stdout.trim();
  if (!generatedBaselineCommit) throw new Error("Sandbox baseline phase generated-baseline-resolve returned an empty commit hash");

  let seededReviewBaselineCommit = generatedBaselineCommit;
  if (seedReviewedPages.length > 0) {
    const approval = await approvePages(seedReviewedPages);
    if (!approval?.ok) {
      const detail = approval?.stderr || approval?.stdout || `exit ${approval?.code ?? "unknown"}`;
      throw new Error(`Sandbox baseline phase seed-reviewed-wiki failed: ${detail}`);
    }

    const approved = JSON.parse(approval.stdout)?.data?.approved;
    if (approved !== seedReviewedPages.length) {
      throw new Error(`Sandbox baseline phase seed-reviewed-wiki expected ${seedReviewedPages.length} approvals, got ${approved}`);
    }

    const reviewedPaths = [
      ...seedReviewedPages.map((id) => path.posix.join(wikiRoot, `${id}.md`)),
      path.posix.join(wikiRoot, "index.md"),
      path.posix.join(wikiRoot, "log.md"),
    ];
    await runGitPhase(runGit, "seeded-review-baseline-stage", ["add", "--", ...reviewedPaths.map(literalGitPathspec)]);
    await runGitPhase(runGit, "seeded-review-baseline-commit", ["commit", "-m", "docs: seed reviewed Wiki baseline"]);
    const reviewed = await runGitPhase(runGit, "seeded-review-baseline-resolve", ["rev-parse", "HEAD"]);
    seededReviewBaselineCommit = reviewed.stdout.trim();
    if (!seededReviewBaselineCommit) throw new Error("Sandbox baseline phase seeded-review-baseline-resolve returned an empty commit hash");

    await verifySeededPages(seedReviewedPages);
  }

  return { generatedBaselineCommit, seededReviewBaselineCommit };
}
