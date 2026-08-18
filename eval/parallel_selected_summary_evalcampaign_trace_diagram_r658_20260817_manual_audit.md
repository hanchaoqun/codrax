# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T02:30:05Z
- sweep_start_ts: 20260817-193003
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-193005 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 261s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 10ms window, deterministic supplement, typed on-chain worker-200 root, 8.300ms effective impact, role-bound priority/class/CPU, cross-CPU caliber, actual-occupancy versus existing-rule-eliminable axes, and Trace causal projection all survived. No neighboring/background promotion and no sleep/nanosleep mechanism inference. Three byte-identical role capsules still repeat across query scopes (B1036). |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-193005 | answer_regex,answer_contains,mermaid_edge_count | none | 542s | 43 | read=21,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=4/0,fin_reject=3,unavail=0,prune=0 | fail | B1037 production-positive: no missing-boundary/connected-boundary oscillation, no stale-draft recovery, and final structured checks passed. But the requested all-component data-flow graph is still split into disconnected components: Analyzer→Explorer→Extractor→Finalizer, Mutable.Objective→objective, and BusContext→BuildAgentContext/Orchestrator. Per-participant incidence incorrectly passes without a full requested relation spine. Explorer also force-completed an unchanged invalid member-set support-ref repair after low-delta convergence. Record B1038/B1039; do not hard-connect or synthesize edges. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
