# Selected parallel eval sweep

- date: 2026-08-18T11:44:31Z
- sweep_start_ts: 20260818-044429
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_loose_multi_question_units | FAIL | no_regex_match:(第一|(^|[^0-9])1([^0-9]|$)|运行时配置|配置加载).*(第二|(^|[^0-9])2([^0-9]|$)|Mermaid|图)|( | 303s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260818-044431 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 389s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260818-044431 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
