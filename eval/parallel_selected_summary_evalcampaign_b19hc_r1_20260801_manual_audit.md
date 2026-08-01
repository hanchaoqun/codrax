# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T10:47:39Z
- sweep_start_ts: 20260801-034737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-034739 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass_with_system_wording_gap | Typed canonical lead correctly says frame causation is unproven and keeps both occupancy/new-direction and eliminable/existing-rule axes. Explicit window, target, rank, wakeup chain, representative windows, projection, and supplements remain present. The same projection still used legacy proven-root wording in its dynamic legend, detail causal-position field, and pointed next step; recorded as the remaining CAUSAL1 system-surface gap and fixed after this replay. |
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260801-034739 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 225s | 39 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The finite state/time/count/kernel-recorded-reason request was expanded into the full causal report. The analyzer emitted runtime_question_profile=causal_diagnosis despite its typed rationale describing only finite observed facts; the same case emitted bounded_fact_set on the immediately previous build. This is declaration instability, not a causal-conclusion materializer regression. The requested three rows and 0.635ms were still correct, but the answer breadth and principal oracle were wrong. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
