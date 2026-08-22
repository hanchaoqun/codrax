# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T12:32:43Z
- sweep_start_ts: 20260822-053242
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-053243 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 210s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000000..2.020000s window, complete threadpool-400 -> network-300 -> cookie-200 -> app-100 typed wakeup chain, 11.000ms on-chain iowait first seat, three independent 1.000ms scheduling/priority candidates, actual-time/removable double account, business drilldown, background separation, and complete Trace causal projection all survive with zero finalizer rejects. Model prose slightly overstates the fscache point plus wakeup order as directly delaying the entire chain; retain as a soft wording observation, not a prose hard gate. |
| 1 | sr_py_registry_dispatch | FAIL | eval/results/sr_py_registry_dispatch-20260822-053243 | answer_regex,answer_contains | none | 375s | 39 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=11,unavail=0,prune=0 | fail | Evidence and prose identify JsonPlugin, REGISTRY, cls(), executor callback, and decorator registration, but the accepted output is a recovered stale draft after 11 atomic-repair rejects. The diagram keeps unsupported Factory/Registry/Plugin/Thread relations. One rejected transaction contained the useful edge replacements/removals, but a bad participant cleanup rolled the entire call back without an explicit full-replay instruction. Later patches assumed those edge edits had committed. Registry also becomes isolated only after the selected removals and therefore was absent from the precomputed optional-orphan roster. This is deterministic protocol churn, not model-only variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
