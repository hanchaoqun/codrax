# Selected parallel eval sweep

- date: 2026-09-07T02:51:01Z
- sweep_start_ts: 20260906-195100
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | - | 129s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260906-195101 |
| 1 | sr_py_registry_dispatch | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260906-195101 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
