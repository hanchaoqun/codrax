# Selected parallel eval sweep

- date: 2026-09-22T02:36:15Z
- sweep_start_ts: 20260921-193612
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_empty_projection_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_g1_english_dstate | PASS | - | 85s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_empty_projection_20260921/real_trace_g1_english_dstate-20260921-193615 |
| 2 | hilog_mixed_arkts_cangjie | FAIL | missing:of missing:bounds | 207s | 2 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hmc_empty_projection_20260921/hilog_mixed_arkts_cangjie-20260921-193615 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
