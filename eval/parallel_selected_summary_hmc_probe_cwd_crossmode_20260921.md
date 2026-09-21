# Selected parallel eval sweep

- date: 2026-09-21T10:19:13Z
- sweep_start_ts: 20260921-031913
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_probe_cwd_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_jank_field_inventory | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_probe_cwd_crossmode_20260921/trace_query_jank_field_inventory-20260921-031913 |
| 1 | nested_python_increment | FAIL | write_final_verdict:unverified:proof_weak | 184s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_probe_cwd_crossmode_20260921/nested_python_increment-20260921-031913 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
