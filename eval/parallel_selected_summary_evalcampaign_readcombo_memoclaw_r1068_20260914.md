# Selected parallel eval sweep

- date: 2026-09-14T07:23:24Z
- sweep_start_ts: 20260914-002324
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | - | 156s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260914-002324 |
| 1 | read_combo_answer_document_tools | PASS | - | 423s | 1 | 2 | 0 | 2 | 1 | 7 | 9 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260914-002324 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
