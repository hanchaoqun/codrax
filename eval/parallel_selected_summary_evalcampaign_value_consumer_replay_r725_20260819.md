# Selected parallel eval sweep

- date: 2026-08-19T09:11:24Z
- sweep_start_ts: 20260819-021123
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260819-021124 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 303s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-021124 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
