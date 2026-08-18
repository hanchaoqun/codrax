# Selected parallel eval sweep

- date: 2026-08-18T22:11:53Z
- sweep_start_ts: 20260818-151151
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:impact_targets_unverified | 182s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260818-151153 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 225s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260818-151153 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
