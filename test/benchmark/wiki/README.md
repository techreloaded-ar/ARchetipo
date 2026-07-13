# Wiki benchmark pilot

This benchmark compares an agent working directly from a repository (`raw`)
with the same agent starting from the current ARchetipo Wiki (`wiki`). The first
pilot is pinned to the fresh RideAtlas Wiki bootstrap at commit `a07564c`.

It is intentionally separate from `test/e2e/`: E2E scenarios verify workflow
correctness, while this harness measures context cost and codebase-comprehension
quality with a real model. Generated sandboxes, traces, and reports live below
`test/workspaces/wiki-benchmark/` and are ignored by Git.

## Run

```bash
npm run test:wiki-benchmark -- --dry-run
npm run test:wiki-benchmark:unit
npm run test:wiki-benchmark -- --cases publish-workflow --conditions raw,wiki
npm run test:wiki-benchmark -- --repetitions 3
npm run test:wiki-benchmark -- --reparse test/workspaces/wiki-benchmark/runs/<run-id>
```

Useful overrides:

```bash
npm run test:wiki-benchmark -- \
  --repository /path/to/RideAtlas \
  --revision a07564c \
  --model opencode-go/deepseek-v4-flash
```

The runner invokes `pi` in JSON, read-only tool mode and records:

- provider-reported input, output, cache, reasoning, and total processed tokens;
- model cost when supplied by the provider;
- elapsed time;
- tool calls and file paths found in the structured trace;
- deterministic oracle scores for expected files, findings, and forbidden claims.

## Conditions

- `raw`: checks out the pinned revision and removes `docs/wiki/`.
- `wiki`: keeps the fresh generated Wiki and instructs the agent to route through
  `docs/wiki/index.md`, then verify implementation claims in code.

Both conditions retain every other repository document. This reflects normal
development rather than creating an artificially documentation-free baseline.
Execution order is balanced deterministically across cases and repetitions so
that one condition is not always advantaged by running second.

The report keeps uncached input, cache reads, output, cost, and total processed
tokens separate. `total_tokens` is the provider total and includes cache reads;
it must not be presented as fresh prompt input.

`--reparse` rebuilds scores and reports from saved structured traces without
invoking the model. Use it after changing an oracle or the trace parser.

## Interpreting the pilot

One repetition is a harness smoke test, not evidence. Use at least three
repetitions before comparing mean scores and token usage. The current scorer is
deliberately conservative and deterministic; it does not replace blind human
review for plan or implementation quality.

Bootstrap cost is not included yet. It will be measured in a separate lane so
that one-time Wiki generation cost is not mixed with repeated downstream task
consumption.
