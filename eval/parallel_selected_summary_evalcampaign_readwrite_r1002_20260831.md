# Selected parallel eval sweep

- date: 2026-09-01T06:50:23Z
- sweep_start_ts: 20260831-235022
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | write_final_verdict:unverified:proof_weak | 225s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260831-235023 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 787s | 1 | 2 | 0 | 1 | 0 | 10 | 10 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260831-235023 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
