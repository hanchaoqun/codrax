# Selected parallel eval sweep

- date: 2026-09-21T13:29:01Z
- sweep_start_ts: 20260921-062859
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_timeline_wait_delivery_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 86s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_timeline_wait_delivery_20260921/patch_go_typo-20260921-062901 |
| 1 | real_trace_c2_dstate_iowait | PASS | - | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_timeline_wait_delivery_20260921/real_trace_c2_dstate_iowait-20260921-062901 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
