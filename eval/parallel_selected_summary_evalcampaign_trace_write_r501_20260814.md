# Selected parallel eval sweep

- date: 2026-08-14T17:55:24Z
- sweep_start_ts: 20260814-105522
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 88s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260814-105524 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:49.623 missing:0.033 missing:按全域最大核最高频 missing:enumeration_status=incomplete missing:未计价 | 173s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260814-105524 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
