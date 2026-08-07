# Selected parallel eval sweep

- date: 2026-08-07T20:44:03Z
- sweep_start_ts: 20260807-134402
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_cpp_virtual_chain | PASS | - | 170s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260807-134404 |
| 2 | sr_ts_workspace_chain | PASS | - | 208s | 2 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260807-134404 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
