# Selected parallel eval sweep

- date: 2026-07-03T06:51:49Z
- sweep_start_ts: 20260703-145149
- total cases: 1
- parallel: 1
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | qf_architecture | FAIL | missing:analyze missing:explore missing:extract missing:finalize | 206s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260703-145149 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**
