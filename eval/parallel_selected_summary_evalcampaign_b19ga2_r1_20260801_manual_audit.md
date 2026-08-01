# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T08:54:26Z
- sweep_start_ts: 20260801-015425
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260801-015426 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 141s | 36 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=1,prune=0 | fail | Analyzer emitted return_value/condition but omitted runtime_targets; model reported 2 waits and 0.285/0.351ms instead of the authoritative 3 occurrences totaling 0.635ms. Supplement then promoted a model cursor and injected two full causal-projection blocks over a model-selected 19ms window. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-015426 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 216s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Explicit-window capabilities remained present: dual-axis occupancy, eliminable overview, root rank, wakeup and projection all materialized. Human fail because the opening prose calls priority inversion/IO a causal chain and “direct cause”, while the typed boundary later says frame_causality=unproven/frame_evidence_status=absent; candidate facts were promoted beyond their authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
