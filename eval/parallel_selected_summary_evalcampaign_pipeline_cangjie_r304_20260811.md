# Selected parallel eval sweep

- date: 2026-08-11T10:11:48Z
- sweep_start_ts: 20260811-031146
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | PASS | - | 162s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260811-031148 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 233s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-031148 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
