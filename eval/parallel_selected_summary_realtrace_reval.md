# Selected parallel eval sweep

- date: 2026-07-05T13:03:59Z
- sweep_start_ts: 20260705-210359
- total cases: 3
- parallel: 3
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 3 | real_trace_f1_exclude_no_code | PASS | - | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260705-210359 |
| 2 | real_trace_e2_cross_trace_asymmetry | PASS | - | 180s | 1 | 1 | 0 | 3 | 2 | 0 | 1 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260705-210359 |
| 1 | real_trace_e1_dual_window_normalized | FAIL | missing_section:34579.472865 missing_section:34579.475857 missing_section:34579.505857 | 248s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260705-210359 |

**Pass: 2 / 3 — Fail/Timeout/LaunchFail: 1**
