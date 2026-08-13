# Selected parallel eval sweep

- date: 2026-08-13T01:00:30Z
- sweep_start_ts: 20260812-180029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 144s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260812-180030 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 181s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-180030 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
