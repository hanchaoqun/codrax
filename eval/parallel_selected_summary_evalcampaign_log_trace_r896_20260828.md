# Selected parallel eval sweep

- date: 2026-08-28T18:03:15Z
- sweep_start_ts: 20260828-110313
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | log_path_question_multi_runtime_files | FAIL | no_regex_match:(context deadline exceeded|deadline|超时).*(\(\*Client\)\.Fetch|Client\.Fetch|client\.go:41)|(\(\*Clien | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/log_path_question_multi_runtime_files-20260828-110315 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260828-110315 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
