# Selected parallel eval sweep

- date: 2026-09-23T10:03:37Z
- sweep_start_ts: 20260923-030337
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_resource_semantics_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_resource_coordinates | FAIL | no_regex_match:(5120|5[,.]120)\n(8[:,，]0)\n(8[:,，]1)\n(8[:,，]2) | 195s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_resource_semantics_20260923/trace_query_resource_coordinates-20260923-030338 |
| 2 | read_combo_config_two_knobs_precedence | PASS | - | 309s | 1 | 1 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | none | eval/results/hmc_resource_semantics_20260923/read_combo_config_two_knobs_precedence-20260923-030338 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
