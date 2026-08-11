# Selected parallel eval sweep

- date: 2026-08-11T09:04:02Z
- sweep_start_ts: 20260811-020400
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | FAIL | inventory_count_mismatch:public_class:got11:want8 | 192s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260811-020402 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 412s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-020402 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
