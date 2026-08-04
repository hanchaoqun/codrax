# Selected parallel eval sweep

- date: 2026-08-04T04:59:48Z
- sweep_start_ts: 20260803-215945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 369s | 1 | 4 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-215948 |
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 378s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260803-215948 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
