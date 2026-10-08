# Selected parallel eval sweep

- date: 2026-10-08T03:43:39Z
- sweep_start_ts: 20261007-204338
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_cpu_json_20261007

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_existing_sqlite_hisys_row_identity | FAIL | no_primary_regex_match:9007199254740993 no_primary_regex_match:9223372036854775807 | 87s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_cpu_json_20261007/trace_existing_sqlite_hisys_row_identity-20261007-204339 |
| 1 | trace_cpu_state_frequency | PASS | - | 535s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_cpu_json_20261007/trace_cpu_state_frequency-20261007-204339 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
