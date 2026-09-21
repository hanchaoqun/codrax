# Selected parallel eval sweep

- date: 2026-09-21T09:17:43Z
- sweep_start_ts: 20260921-021743
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_context_ruler_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 121s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_context_ruler_crossmode_20260921/patch_go_typo-20260921-021743 |
| 1 | real_trace_e1_dual_window_normalized | PASS | - | 134s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_context_ruler_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-021743 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
