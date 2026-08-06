# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T16:09:12Z
- sweep_start_ts: 20260806-090911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-090912 | typed_inventory_rowset,dimension_substring,answer_contains | none | 117s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S8 clean replay: exact 2 extend + 2 foreign func + 8 public class rows, paths/packages/citations aligned; no finalizer reject or JSON recovery. The generic coverage caveat is unnecessary model-authored hedging but does not change the inventory. |
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260806-090912 | typed_inventory_rowset,answer_contains | none | 228s | 23 | read=2,repo_map=1,list=1,trace=0,source_lens=1 | midloop=7,inv=1/0,fin_reject=11,unavail=1,prune=0 | fail | repo_map exposed 4 @Entry + 2 @Builder, but explorer grounded only Index and the two builders, then model-owned completion accepted the known-partial 1+2 aggregate. Finalizer also hit a deterministic ADD/REMOVE contradiction for the same admitted `Index (struct)` row. One JSON-encoded blocks string was losslessly recovered and was not causal. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
