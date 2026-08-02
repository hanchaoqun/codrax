# Selected parallel eval sweep

- date: 2026-08-02T23:16:29Z
- sweep_start_ts: 20260802-161628
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 44s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260802-161629 |
| 1 | read_combo_log_current_code_dimensions | PASS | - | 177s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_dimensions-20260802-161629 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
