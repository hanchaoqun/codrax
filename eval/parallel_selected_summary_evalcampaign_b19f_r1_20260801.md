# Selected parallel eval sweep

- date: 2026-08-01T07:58:10Z
- sweep_start_ts: 20260801-005808
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 155s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-005810 |
| 2 | real_trace_c2_dstate_iowait | FAIL | no_principal_regex_match:34579\.451701.*0\.138 no_principal_regex_match:34579\.452934.*0\.147 no_principal_regex_match:3 | 167s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-005810 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
