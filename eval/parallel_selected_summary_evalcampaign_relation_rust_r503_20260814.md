# Selected parallel eval sweep

- date: 2026-08-14T18:30:26Z
- sweep_start_ts: 20260814-113024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 226s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260814-113026 |
| 1 | qf_type_relation_loop_controller | PASS | - | 228s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260814-113026 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
