# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T11:14:30Z
- sweep_start_ts: 20260801-041428
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-041430 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Typed unproven authority owns the complete published report. The model draft still overclaimed lock ownership and causal transfer in logs, but none of those model blocks appear after the final-answer delimiter. Canonical lead, occupancy/new-direction axis, eliminable/existing-rule axis, rank, wakeup relationships, representative windows, full projection, evidence, target identity, and supplements all remain. No legacy proven-root wording or “system authority” front block appears. CAUSAL1 can close. |
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260801-041430 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 193s | 33 | read=1,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass_with_open_gaps | Analyzer again emitted bounded_fact_set and the answer stayed finite with the exact three rows, 0.635ms total, and kernel caller. No full causal projection appeared. SCHEDPROSE1 and NARROWCAVEAT1 remain: the model summary narrates the typed non-IO-D versus io_wait partition as a scheduler-semantics distinction, and generic drill/wakeup caveats remain irrelevant to this finite request. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
