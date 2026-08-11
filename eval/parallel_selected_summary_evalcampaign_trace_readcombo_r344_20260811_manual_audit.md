# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T23:10:17Z
- sweep_start_ts: 20260811-161015
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-161017 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B578 remains positive: the one physical target sleep segment is emitted once as `自身·sleep 5.000ms [E1+E2]`; the explicit 5.000..5.007s window, four-state account, VerifyClass 4.600ms on-chain candidate, target runnable 0.800ms, actual-cost/removable-impact axes, and bounded causal projection remain intact. B577 repeats for a third production run: perf_triage calls the 5.005800 switch-in a wakeup, reports 5.8ms instead of the 5.000ms sleep plus 0.800ms runnable phases, and this navigation hypothesis propagates into analyzer/final prose despite later deterministic authority. The final answer also says VerifyClass blocked the wakeup while its authority block says no direct blocker is proved. This is a context-authority contamination, not a missing prompt sentence or isolated model fluctuation. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-161017 | answer_regex,answer_contains | none | 276s | 34 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | The final table, four-stage ordering, one typed call edge, one typed data-flow edge, and three precedence edges are source-grounded. B579's two-operation same-carrier branch was not exercised in this replay, so production closure remains pending. The final graph copied validator-private labels `invocation_segment`, `value_flow_segment`, and `unproven cross-component bridge` into reader-visible Notes; record B580 as a soft context/lexicon leak. The active model stream completed normally after 276s; no four-minute answer degradation, system answer, or stale-draft substitution occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
