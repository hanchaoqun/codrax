# Selected parallel eval sweep

- date: 2026-08-16T06:24:18Z
- sweep_start_ts: 20260815-232417
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_loose_multi_question_units | FAIL | no_regex_match:(第一|(^|[^0-9])1([^0-9]|$)|运行时配置|配置加载).*(第二|(^|[^0-9])2([^0-9]|$)|Mermaid|图)|( | 198s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260815-232418 |
| 2 | read_combo_answer_document_tools | PASS | - | 679s | 1 | 3 | 0 | 2 | 1 | 5 | 7 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260815-232418 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
