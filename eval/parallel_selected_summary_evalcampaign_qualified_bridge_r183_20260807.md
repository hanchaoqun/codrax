# Selected parallel eval sweep

- date: 2026-08-07T19:21:41Z
- sweep_start_ts: 20260807-122140
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_cross_module_chain | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-122141 |
| 2 | sr_ts_workspace_chain | PASS | - | 512s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260807-122141 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
