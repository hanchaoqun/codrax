# Selected parallel eval sweep

- date: 2026-08-31T19:36:34Z
- sweep_start_ts: 20260831-123632
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 99s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260831-123634 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 564s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260831-123634 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
