# Selected parallel eval sweep

- date: 2026-07-11T06:40:25Z
- sweep_start_ts: 20260711-144025
- total cases: 3
- parallel: 1
- timeout: 600s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | real_trace_b3_process_level_rollup | FAIL | missing:NetworkService | 225s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b3_process_level_rollup-20260711-144025 |
| 2 | trace_query_converted_inode_io_pressure | PASS | - | 122s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_converted_inode_io_pressure-20260711-144410 |
| 3 | read_combo_trace_current_code_dimensions | FAIL | no_regex_match:internal/(analysis|tool|agent|orchestrator|types)/[^[:space:]]+\.go:[0-9]+ | 155s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_dimensions-20260711-144613 |

**Pass: 1 / 3 — Fail/Timeout/LaunchFail: 2**
