# RideAtlas phase-0 pilot

Date: 2026-07-13  
Repository revision: `a07564c`  
Model: `opencode-go/deepseek-v4-flash`

## Question

Does a fresh `archetipo-wiki bootstrap` reduce the context needed for realistic
codebase-comprehension tasks without reducing answer quality?

The pilot compares two isolated checkouts of the same revision:

- `raw`: `docs/wiki/` is removed;
- `wiki`: the agent starts from `docs/wiki/index.md` and verifies implementation
  claims in code.

Both conditions are read-only and keep all other repository documentation. Four
cases cover diagnosis, change localization, planning, and runtime explanation.
The provider trace supplies token and cost telemetry; deterministic oracles
score expected files, required findings, forbidden claims, and output format.

## Exploratory result

| Metric | Raw | Wiki | Change |
|---|---:|---:|---:|
| Mean quality score | 91.67 | 91.67 | 0.00 |
| Total processed tokens | 2,284,119 | 1,068,423 | -53.22% |
| Uncached input tokens | 177,418 | 116,371 | -34.41% |
| Cache-read tokens | 2,072,960 | 930,816 | -55.10% |
| Output tokens | 33,741 | 21,236 | -37.06% |
| Provider cost | $0.04009 | $0.02484 | -38.03% |
| Files opened | 85 | 76 | -10.59% |
| Elapsed time | 410.0 s | 281.7 s | -31.28% |

Per case, total processed-token reduction ranged from 32.06% to 73.61%. All
required semantic findings were recovered after calibrating one overly narrow
oracle. The publish case lost ten format points because the Wiki response was
missing its final JSON brace; the Trip Builder case gained ten format points,
so the aggregate quality score remained equal.

The generated Wiki contains 15 Markdown files, 1,088 lines, 8,992 words, and
72,153 bytes. Generation cost and elapsed time are unknown because bootstrap
had already completed before instrumentation; they must be measured in a fresh
lane before calculating amortization.

## Interpretation

This is enough to validate the benchmark shape and justify a larger experiment.
It is not enough to claim a general 53% saving: each pair has one repetition,
all cases use one repository and one model, and the first pilot ran conditions
in fixed raw-then-wiki order. The runner now balances order for subsequent runs.

The useful success criterion for ARchetipo is not “fewest files read”. It is:

1. no material quality regression or increase in unsupported claims;
2. lower uncached input and total processed context;
3. a bootstrap/update cost that amortizes over a realistic number of tasks;
4. resilience when Wiki content is stale or conflicts with code.

## Next phase-0 runs

1. Repeat each pair at least three times with balanced condition order.
2. Add a fresh-bootstrap lane and compute break-even task count from measured
   generation cost versus downstream savings.
3. Add stale-Wiki and code/Wiki-conflict cases; code must remain authoritative.
4. Add one change task scored by tests, not only read-only comprehension.
5. Test a `constitution` as a separate intervention (`wiki` versus
   `wiki+constitution`) using invariant-sensitive change tasks. Do not fold it
   into bootstrap until it demonstrates fewer policy violations at acceptable
   context cost.
