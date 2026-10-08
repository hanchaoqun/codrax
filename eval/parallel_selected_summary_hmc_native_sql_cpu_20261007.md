# Selected parallel eval sweep

批次补充：此runner只统计CPU CLI一例；同批并行的真实模型只读登记使用独立Go入口，原始stdout在 `results/hmc_native_sql_cpu_20261007/native_registration_restored/go-test.stdout.log`。两入口合计恰好2并行×1、机器2/2；人工审计为CPU完整答案FAIL、只读登记限定范围PASS，详见[人工报告](parallel_selected_summary_hmc_native_sql_cpu_20261007_manual_audit.md)。下面保留runner原始口径，不把自动PASS当人工通过。

- date: 2026-10-08T06:39:18Z
- sweep_start_ts: 20261007-233917
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results/hmc_native_sql_cpu_20261007

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_cpu_native_intervals | PASS | - | 180s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_sql_cpu_20261007/trace_cpu_native_intervals-20261007-233918 |

**Pass: 1 / 1 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
