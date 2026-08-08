# Selected parallel eval sweep

- date: 2026-08-08T13:29:04Z
- sweep_start_ts: 20260808-062903
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 138s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-062904 |
| 2 | qf_sequence_analyzer_gate | FAIL | no_regex_match:(normalizer\.Normalize|compiler\.Compile|hdp\.Plan|binder\.BindByRelevance|RecomputeBudget) | 205s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260808-062904 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

## Human audit

| case | runner | human | conclusion |
|------|--------|-------|------------|
| qf_logic_view_read_pipeline | PASS | FAIL | Analyzer no longer misroutes the component roster as a scalar role lookup, but the answer still uses an auto-repair helper as the Finalizer identity and draws unproved/reversed transfer edges. The run carried `flow_findings=0`; the current typed `predicate_axis=define` plus `IntentExplain -> lookup -> SkipFindings=true` path cannot produce the relation carrier that the new prompt asks the model to use. |
| qf_sequence_analyzer_gate | FAIL | FAIL | The answer correctly reports that `buildAnalysisIR` does not reach `gate.Run` and preserves the two real edges converging on `RunWith`, but it omits the requested ordered middle operations inside `buildAnalysisIR`. Exploration treated an intraprocedural sequence request as endpoint reachability, spending five completion attempts before the correct `no_directed_path` boundary. One finalizer retry was a model payload-shape error (`block.kind` empty), not a contradictory system contract. |

Batch result: **human 0/2**. `S37cc` is production-positive for role/classification authority but incomplete for relation/transfer authority. Filed `EVAL-B354-ARCHCOMPONENTFLOWCLOSURE1=P1` and `EVAL-B355-INTRAPROCSEQUENCEAUTH1=P1` for generalized typed fixes; no answer-text or request-text hard scan is proposed.
