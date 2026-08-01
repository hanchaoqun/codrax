# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T11:05:19Z
- sweep_start_ts: 20260801-040517
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-040519 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 133s | 35 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass_with_open_gaps | Analyzer returned runtime_question_profile=bounded_fact_set and the answer stayed finite: exact three rows, 0.635ms total, and caller are visible; no causal projection was injected. This confirms prior r1 failure was declaration variance. Remaining gaps: generic drill/wakeup caveats are irrelevant to the bounded request, and model scheduler wording tries to contrast io_wait with D state even though the typed surface intentionally partitions non-IO D-state from io_wait; keep SCHEDPROSE1 and NARROWCAVEAT1 open. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-040519 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | System lead, projection headline, dynamic legend, detail causal position, and pointed next step all obey typed unproven authority; explicit window, both analysis axes, wakeup chain, representative windows, evidence, and supplementation remain. However a model-owned supporting timeline item still states that the real bottleneck is CookieMonsterCl, contradicting the typed ceiling. This proves principal-only removal is insufficient; fixed after replay by structurally suppressing every model-owned block when unproven plus a real system projection is present. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
