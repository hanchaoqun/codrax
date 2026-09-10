# Selected parallel eval sweep

- date: 2026-09-10T12:04:43Z
- sweep_start_ts: 20260910-050443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 213s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260910-050443 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 345s | 1 | 1 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260910-050443 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
