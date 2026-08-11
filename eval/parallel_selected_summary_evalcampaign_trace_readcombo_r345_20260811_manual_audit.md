# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T23:25:26Z
- sweep_start_ts: 20260811-162524
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-162526 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B577b is active: all model-authored observation semantics are withheld and only candidate line/time locators reach downstream agents; deterministic validator rows remain full. The answer now separates sleep 5.000ms and runnable 0.800ms instead of calling switch-in a 5.8ms wakeup. A deeper deterministic contract bug remains: WakeupEdge.LatencyMs is calculated as sleep-segment start→sched_wakeup, yet the trace wait handoff renders it as `wakeup latency 5.000ms`; the final timeline copies that term and falsely says the class-verification span completed before wakeup even though the span ends at 5.005400 after the 5.005000 wakeup. Explicit window, on-chain rank, dual axes, frame-absent caveat, causal projection, and B578 one-seat sleep remain intact. Record B581. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-162526 | answer_regex,answer_contains | none | 316s | 39 | read=6,repo_map=4,list=0,trace=0,source_lens=1 | midloop=7,inv=4/2,fin_reject=4,unavail=0,prune=1 | fail | B580 is positive: no internal `invocation_segment`/`value_flow_segment`/bridge-status token leaks into the final graph. However four relation-gate rejections force repeated deletion; the final required sequence diagram omits Analyze→Explore and leaves two disconnected orchestrator calls beside Explore→Extract→Finalize. Prose/table are accurate, but the requested visual relation is incomplete. Record B582: when a typed requested-relation-spine exists, the repair context must foreground one compact exact spine recipe and demote disconnected supporting components to optional prose, without system-authored edges or output rewriting. The active stream completed normally at 316s with no fixed four-minute degradation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
