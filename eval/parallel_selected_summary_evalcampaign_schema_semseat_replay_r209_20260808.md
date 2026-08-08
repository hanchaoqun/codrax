# Selected parallel eval sweep

- date: 2026-08-08T11:53:25Z
- sweep_start_ts: 20260808-045324
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260808-045325 |
| 2 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 237s | 0 | 0 | 0 | 0 | 5 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260808-045325 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
