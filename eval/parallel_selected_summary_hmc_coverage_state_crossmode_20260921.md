# Selected parallel eval sweep

- date: 2026-09-21T08:22:00Z
- sweep_start_ts: 20260921-012200
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_coverage_state_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 208s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_coverage_state_crossmode_20260921/github_issue_nlohmann_long_double_symptom-20260921-012200 |
| 1 | trace_query_wakeup_background_demotion | PASS | - | 217s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_coverage_state_crossmode_20260921/trace_query_wakeup_background_demotion-20260921-012200 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
