# Selected parallel eval sweep

- date: 2026-10-09T02:39:26Z
- sweep_start_ts: 20261008-193923
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_process_identity_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_process_measurements | PASS | - | 230s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_process_identity_20261008/trace_process_measurements-20261008-193926 |
| 2 | trace_rendering_candidates | PASS | - | 293s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_process_identity_20261008/trace_rendering_candidates-20261008-193926 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
