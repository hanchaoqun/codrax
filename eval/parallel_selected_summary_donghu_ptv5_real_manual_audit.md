# Selected Eval Manual Audit Scaffold

- date: 2026-07-05T17:32:00Z
- sweep_start_ts: 20260706-013200
- total cases: 5
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260706-013200 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 145s | 37 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | trace_query_donghu_mixed_platform | PASS | eval/results/trace_query_donghu_mixed_platform-20260706-013426 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 31 | read=4,repo_map=0,list=0,trace=2,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | trace_query_path_question_relative_donghu_short | PASS | eval/results/trace_query_path_question_relative_donghu_short-20260706-013714 | log_regex,answer_regex,answer_contains | none | 94s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | trace_query_path_question_absolute_donghu_short | PASS | eval/results/trace_query_path_question_absolute_donghu_short-20260706-013848 | log_regex,answer_regex,answer_contains | none | 118s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | trace_query_donghu_real_short_runnable | FAIL | eval/results/trace_query_donghu_real_short_runnable-20260706-013200 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 730s | 39 | read=9,repo_map=0,list=0,trace=54,source_lens=0 | midloop=5,inv=21/3,fin_reject=2,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
