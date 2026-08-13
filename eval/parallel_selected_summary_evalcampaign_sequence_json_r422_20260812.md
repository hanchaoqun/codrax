# Selected parallel eval sweep

- date: 2026-08-13T05:19:41Z
- sweep_start_ts: 20260812-221939
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_pipeline_sequence_table | PASS | - | 333s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260812-221941 |
| 2 | data_json_strict_ids | FAIL | no_regex_match:"ids" | 437s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260812-221941 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
