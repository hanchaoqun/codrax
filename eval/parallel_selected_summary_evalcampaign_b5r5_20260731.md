# Selected parallel eval sweep

- date: 2026-07-31T14:43:18Z
- sweep_start_ts: 20260731-074318
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | - | 152s | 2 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-074318 |
| 2 | cangjie_repomap | PASS | - | 167s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260731-074318 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
