# Selected parallel eval sweep

- date: 2026-08-09T05:52:22Z
- sweep_start_ts: 20260808-225221
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 128s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260808-225223 |
| 1 | read_combo_pipeline_sequence_table | FAIL | missing:Mutable | 262s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260808-225223 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
