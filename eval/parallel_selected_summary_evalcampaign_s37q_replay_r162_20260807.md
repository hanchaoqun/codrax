# Selected parallel eval sweep

- date: 2026-08-07T12:00:55Z
- sweep_start_ts: 20260807-050054
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_rust_cross_module_chain | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260807-050055 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 501s | 1 | 1 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260807-050055 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
