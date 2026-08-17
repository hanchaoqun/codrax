# r597 ArkTS / Trace production replay manual audit

- date: 2026-08-17T00:25:47Z
- sweep_start_ts: 20260816-172546
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | runner | human | result_dir | sec | ctx% | tools/churn | audit conclusion |
|--:|------|--------|-------|------------|----:|-----:|-------------|------------------|
| 1 | `arkts_repomap` | PASS | pass / B947 not exercised | `eval/results/arkts_repomap-20260816-172547` | 129s | 26 | read=5, repo_map=1, list=1; finalizer rejects=0 | Exact four `@Entry` and two `@Builder` members, paths, line anchors, summaries, and third-party boundary; each row appears once in one bucket section. One JSON-encoded `blocks` string was recovered losslessly. The analyzer emitted `has_per_member_table=false`, so the correct single-section carrier proves B946/no-duplicate behavior but does not exercise B947's typed per-member-table branch. |
| 2 | `real_trace_h7_self_seat_full_spectrum` | PASS | partial | `eval/results/real_trace_h7_self_seat_full_spectrum-20260816-172547` | 216s | 43 | trace=5; analyzer structural retry=1; finalizer rejects=0 | The analyzer selected required `causal_contributor_set`, `causal_diagnosis`, root-cause intent/scenario, named target, and exact window. Its first emit incorrectly retained finite `fact_families`; the existing precise typed validator rejected only that contradiction, and the second emit removed the families without demoting causal breadth. The final answer and deterministic projection preserve on-chain-only ranking, 65.912/36.757ms, 49.623/0.033 split, incomplete compaction disclosure, business spans, actual-occupancy versus rule-eliminable axes, and background isolation. Partial because model prose first says most sleep is upstream synchronization, then correctly says the state partition cannot classify sleep cause; typed evidence does not support the stronger “most” claim. |

## Decisions

- B947 remains `pending-production-replay`: this PASS did not activate its typed branch. The next heterogeneous replay must use a request that deterministically carries `HasPerMemberTable=true` rather than relying on analyzer variance in this ArkTS wording.
- The Trace r596 failure is confirmed as analyzer semantic variance, not a missing system rule. r597 consumed the existing exact causal teaching and the precise cross-field validator repaired only a real schema contradiction. No raw-request keyword gate, automatic scope widening, or system-authored conclusion is warranted.
- The contradictory sleep interpretation is model wording variance under otherwise precise context. Keep it as a soft observation; do not scan answer prose or rewrite the model conclusion.
- Both calls remained active beyond 4ms and completed normally. No fixed-age degradation, empty answer, finalizer recovery fallback, or projection suppression occurred.
