# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T18:52:29Z
- sweep_start_ts: 20260805-115228
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-115229 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 200s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Contract-source repair is effective: runtime-only Required Blocks now permit section/list/table/bullets, no current-source hard facet remains, and the model emits valid JSON in one attempt. Human semantics still cross the compact typed boundary: `causal_conclusion=unproven`/`frame_evidence_status=absent` becomes a definite dropped-frame cause; wakeup/dependency edges become a blocking chain; 34/36 wakeups becomes effectively exclusive; D/IO and rank seats are recombined despite `cross_row_addition=not_authorized`; sleep is asserted to be design-internal and wakeup latency is claimed to trend upward without a complete typed series. Treat as `EVAL-B124-TRACECOGNITION1`, not a reason for system-written replacement prose. |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-115229 | answer_regex,answer_contains | none | 469s | 24 | read=7,repo_map=2,list=0,trace=0,source_lens=1 | midloop=13,inv=6/0,fin_reject=5,unavail=0,prune=0 | fail | Analyzer emitted `diagram_hint.kind=call_dag` for an ordinary prose request. The compiler treated any hint as an explicit user mandate, so Required Blocks demanded exactly one diagram. Explorer then spent 13 rounds trying to collapse static calls + registry binding + factory/value flow + inheritance/MRO into a directed call path; finalizer spent 5 patch rounds removing unsupported arrows. The accepted answer still cites unrelated `runner.py:21` for `JsonPlugin.handle`, displays only `run_pipeline -> resolve` while calling it the complete chain, and repeats citations. Runner PASS is a false positive. Confirms `EVAL-B125-DIAGRAMAUTH1` and C3 compound-dispatch completion gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
