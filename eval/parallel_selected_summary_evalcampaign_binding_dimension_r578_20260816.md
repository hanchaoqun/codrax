# Selected parallel eval sweep

- date: 2026-08-16T18:07:03Z
- sweep_start_ts: 20260816-110702
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-110703 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 483s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260816-110703 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
