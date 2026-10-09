# Selected parallel eval sweep

- date: 2026-10-09T03:58:06Z
- sweep_start_ts: 20261008-205805
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_frame_rate_scope_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_preferred_frame_rate | PASS | - | 191s | 1 | 1 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_frame_rate_scope_20261008/trace_preferred_frame_rate-20261008-205806 |
| 2 | trace_rendering_candidates | PASS | - | 307s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_frame_rate_scope_20261008/trace_rendering_candidates-20261008-205806 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
