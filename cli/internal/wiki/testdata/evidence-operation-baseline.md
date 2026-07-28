# Wiki evidence operation Phase A baseline

Fixture: two reviewed pages share `evidence/shared.txt`; one cites the file and one cites the containing directory. The file is Git-tracked. Counts are deterministic Git command categories captured by `TestEvidenceOperationCommandCount`.

The pre-Phase-A setup counts are reproduced on every test run by `TestEvidenceOperationLegacySetupBaseline`. That harness uses the current fingerprint semantics but instantiates the historical independent snapshot call graph: one resolver, repository probe, and stage-index load per page evidence pass. This isolates the setup behavior Phase A removes without retaining a second production evaluator. `TestEvidenceOperationCommandCount` measures the real Phase A top-level operations on the same fixture.

Phase A intentionally does not cache per-page fingerprints, clean-filter hashes, directory membership, or embedded-repository membership. `TestEvidenceResolverDoesNotCacheDirectoryMembership` mutates a directory between two resolutions through the same resolver and proves the second resolution observes the new entry.

| Operation | Pre repo probes | Pre index loads | Phase A repo probes | Phase A index loads | Phase A clean hashes | Phase A untracked lists |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Validate | 2 | 2 | 1 | 1 | 2 | 1 |
| Status | 4 | 4 | 1 | 1 | 4 | 2 |
| Approve | 4 | 4 | 1 | 1 | 4 | 2 |
| Reconfirm | 6 | 6 | 1 | 1 | 6 | 3 |
| Catalog | 2 | 2 | 1 | 1 | 2 | 1 |
| Search(status) | 2 | 2 | 1 | 1 | 2 | 1 |

Reproduce command counts:

```sh
go test ./internal/wiki -run 'TestEvidence(OperationLegacySetupBaseline|OperationCommandCount|ResolverDoesNotCacheDirectoryMembership)' -count=1 -v
```

Repeatable wall-clock/allocation sampling (five independent samples; compare with `benchstat` when installed):

```sh
go test ./internal/wiki -bench 'BenchmarkWiki(Evidence|Validate|Status|Approve)' -benchmem -count=5
```

Reference Phase A sample on darwin/arm64, Apple M5 (2026-07-27; five samples, directory membership uncached): Evidence 19.5-20.6 ms/op, Validate 33.0-35.3 ms/op, Status 53.4-59.1 ms/op, Approve 60.5-64.7 ms/op. Wall-clock results are machine-dependent and are not asserted in tests. The release invariant is the deterministic setup-count reduction above; Phase B per-file/page memoization is explicitly out of scope.
