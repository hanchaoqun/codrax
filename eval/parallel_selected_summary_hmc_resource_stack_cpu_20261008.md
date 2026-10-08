# Selected parallel eval sweep

- date: 2026-10-08T07:51:23Z
- sweep_start_ts: 20261008-005121
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_resource_stack_cpu_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_cpu_native_intervals | PASS | - | 169s | 2 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_resource_stack_cpu_20261008/trace_cpu_native_intervals-20261008-005123 |
| 1 | trace_native_resource_stack | PASS | - | 204s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_resource_stack_cpu_20261008/trace_native_resource_stack-20261008-005123 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
