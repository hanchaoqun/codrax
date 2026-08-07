# Selected parallel eval sweep

- date: 2026-08-07T20:14:38Z
- sweep_start_ts: 20260807-131437
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_ts_workspace_chain | PASS | - | 158s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260807-131438 |
| 1 | sr_rust_cross_module_chain | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-131438 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
