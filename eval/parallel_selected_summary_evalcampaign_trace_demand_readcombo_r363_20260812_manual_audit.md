# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T05:09:53Z
- sweep_start_ts: 20260811-220950
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-220953 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | Stable B605 control: demand-side conclusion remains model-authored, no cross-seat sum is used, 10.331ms compute-delivery deficit remains a positive secondary candidate, and explicit window / typed wakeup chain / on-chain rank / occupancy-vs-eliminable axes / semantic business clue / background demotion / causal projection remain intact. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-220953 | trace_attachment,answer_regex | perf_triage+trace_query | 255s | 34 | read=3,repo_map=1,list=0,trace=3,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | B607 production positive but not sufficient: defaults.go teaching text is gone and source exploration reads production parse/integrity files. The answer still misstates sync pairing as payload-PID plus adjacent E; actual findSpanWindowsCompacted uses physical source + ftrace row emitter PID LIFO stacks, lifecycle/malformed resets, and non-adjacent nesting. Confirm B609 current-source mechanism lifecycle coverage gap. B608 also repeats: pretriage/analyzer mint unsupported 60fps/16.7ms/5.16x aggregate facts despite no typed refresh/deadline carrier. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
