# Selected parallel eval sweep

- date: 2026-08-14T08:06:22Z
- sweep_start_ts: 20260814-010620
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 291s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260814-010622 |
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 506s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260814-010622 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
