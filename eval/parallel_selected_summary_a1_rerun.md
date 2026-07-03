# Selected parallel eval sweep

- date: 2026-07-03T06:28:00Z
- sweep_start_ts: 20260703-142800
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | s1a | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/s1a-20260703-142800 |
| 1 | qf_architecture | FAIL | missing:analyze missing:explore missing:extract missing:finalize | 197s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260703-142800 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
