# Read Mode Representative Batch U9a - 2026-06-20

## Scope

Six representative read-mode cases were run with max parallelism 2 after the
attached-trace selector normalization fix:

- `trace_query_wakeup_causal_io_chain`
- `arkts_repomap`
- `cangjie_repomap`
- `qf_relation_subagent_registry`
- `qf_architecture`
- `read_combo_log_current_code_dimensions`

## Results

| Case | Harness | Manual audit | Key observation |
| --- | --- | --- | --- |
| `trace_query_wakeup_causal_io_chain` | PASS | PASS | Attached trace stayed on `trace_query`; no source-file drift. Still used 8 trace queries and about 51k estimated context. |
| `arkts_repomap` | FAIL | FAIL | Answer claimed no real `.ets` source while a repo-owned ArkTS corpus source existed under `internal/thirdparty/tree-sitter-arkts/corpus/sources`. |
| `cangjie_repomap` | PASS | WEAK | Harness passed, but the answer relied on parser/helper Go metadata instead of a first-class Cangjie source-inventory authority. |
| `qf_relation_subagent_registry` | PASS | PASS | Correct closed-set answer. Still heavier than expected for a small registry lookup. |
| `qf_architecture` | PASS | PASS | Correct architecture answer, but final output still carried extra system supplement/noise. |
| `read_combo_log_current_code_dimensions` | PASS | PASS | Correctly separated runtime artifact from current source. Still showed duplicated proof work and repair retries. |

## Gap Mapping

- RNE-C52 is closed for default attached-trace selectors.
- RNE-C47/RNE-C48 remained important because over-wide source-inventory calls
  must be corrected both at the model/tool boundary and inside the algorithm.
- RNE-C53 remains P0: source-class inventory authority is not yet executable
  enough for repo-owned corpus/fixture source surfaces across supported
  languages.
- RNE-C43/RNE-C54/RNE-C55 remain P1: broad/simple answers are still expensive,
  sibling proof can duplicate work, and structured repair/handoff needs a
  shared carrier.

## Follow-Up

Batch U7i closes the immediate broad `repo_map(source_inventory)` runaway class:
root `roles=["file"]` calls are refused before index work, and broad lens scans
now have a separate scan budget so no-match scans return incomplete/truncated
typed observations rather than absence proof.
