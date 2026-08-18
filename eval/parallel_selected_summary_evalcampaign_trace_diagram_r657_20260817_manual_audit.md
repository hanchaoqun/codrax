# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T02:12:47Z
- sweep_start_ts: 20260817-191245
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-191247 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 204s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B1034/B1035 production-positive: finalizer received exact role-bound `worker-200=20/ohos_cfs/cpu2 -> app-100=52/ohos_rt/cpu1` and the zero-wait-reason mechanism boundary; the model preserved both, crowned only the typed on-chain worker candidate (8.300ms effective), retained explicit-window projection/supplement and did not infer sleep/nanosleep. The same role capsule was repeated across three query scopes; record as B1036 compaction debt, not a correctness failure. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260817-191247 | answer_regex,answer_contains,mermaid_edge_count | none | 403s | 37 | read=15,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=6,unavail=0,prune=0 | fail | Deterministic P0 contract conflict, not model variance: the grounded local technical edge `bus.Mutable -> Mutable` made the validator reject the required unproven participant boundary as `unproven_boundary_has_visible_incident_edge`; removing that boundary was then rejected as `missing_unproven_boundary`. Six finalizer rejects exhausted repair and recovered a stale draft. Requested-relation coverage and independent local technical incidence need separate typed states. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
