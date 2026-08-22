# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T07:59:39Z
- sweep_start_ts: 20260822-005938
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-005939 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000..2.020s window, four-thread/three-edge wakeup chain, 11.000ms on-chain IO root, actual-time versus rule-removable axes, business/kernel drill-down, background isolation, system supplement and full Trace causal projection all survive. No 4ms/active-stream downgrade and no finalizer reject. Minor wording calls the second threadpool ranking seat “重复”, but the typed board keeps the distinct 11ms IO and 1ms runnable ledgers separate. |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-005939 | answer_regex,answer_contains | none | 511s | 40 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=8,unavail=0,prune=0 | uncertain | Final prose names JsonPlugin and describes decorator-time binding correctly, but the deterministic producer emitted only the indexed write at registry.py:17: the exact lookup and entry-call coordinates were already citable yet outside the later paginated read range. No complete typed dynamic candidate reached the finalizer. Eight relation retries followed and the final diagram retained only run_pipeline -> resolve -> cls, omitting assignment/lookup/argument/callback structure. Runner substring PASS is therefore not production closure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
