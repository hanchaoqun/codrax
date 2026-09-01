# Selected parallel eval sweep

- date: 2026-09-01T09:37:47Z
- sweep_start_ts: 20260901-023745
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 143s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260901-023747 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 391s | 1 | 1 | 0 | 1 | 0 | 9 | 9 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260901-023747 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
