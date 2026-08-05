# Selected parallel eval sweep

- date: 2026-08-05T16:47:38Z
- sweep_start_ts: 20260805-094737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_py_registry_dispatch | PASS | - | 288s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260805-094738 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_report_failed write_final_run_status:in_progress write_final_verdict:missing:missing | 1155s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260805-094738 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
