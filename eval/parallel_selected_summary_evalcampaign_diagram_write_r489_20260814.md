# Selected parallel eval sweep

- date: 2026-08-14T13:33:02Z
- sweep_start_ts: 20260814-063300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | - | 168s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260814-063302 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 293s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260814-063302 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
