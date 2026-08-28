# Selected parallel eval sweep

- date: 2026-08-28T17:39:51Z
- sweep_start_ts: 20260828-103951
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | log_path_question_multi_runtime_files | FAIL | no_regex_match:(panic|nil pointer|空指针).*(\(\*Store\)\.Get|Store\.Get|store\.go:88)|(\(\*Store\)\.Get|Store\.Get|st | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/log_path_question_multi_runtime_files-20260828-103951 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 191s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260828-103951 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
