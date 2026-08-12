# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T23:48:14Z
- sweep_start_ts: 20260811-164813
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-164814 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 28 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B581 production positive: the 5.000ms value is now identified as pre-wakeup sleep/blocking-start to sched_wakeup wait, not post-wakeup scheduling delay. The answer preserves the explicit 5.000..5.007s window, typed on-chain root ranking, actual-occupancy/eliminable-rule dual axes, frame-causality boundary, and causal projection. It also discloses that the VerifyClass span ends after sched_wakeup, so no completion dependency is invented. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-164815 | answer_regex,answer_contains | none | 499s | 44 | read=34,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=1,unavail=0,prune=7 | fail | B582 production positive: after one repair the Mermaid preserves all three typed request-spine precedence edges and supporting calls do not replace it. However the final structured table mixes two labeled-row width conventions: the finalize row has one fewer cell and shifts every visible value left while the runner still passes (B583). The completion lane also spends 47 explorer iterations trying to find citable source operations incident to generic request labels such as `codrax read mode` and `stage`; these are request-visible context labels, not source endpoint identities, so the typed role contract is unsatisfiable and causes 499s churn (B584). The active model stream completed normally after 499s; no elapsed-time degradation or system-authored answer occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
