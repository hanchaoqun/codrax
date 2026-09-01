# Selected parallel eval sweep

- date: 2026-09-01T10:19:39Z
- sweep_start_ts: 20260901-031938
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | write_final_verdict:unverified:proof_weak | 135s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260901-031939 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 685s | 1 | 2 | 0 | 1 | 0 | 14 | 14 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260901-031939 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
