# Selected parallel eval sweep

- date: 2026-08-04T07:17:54Z
- sweep_start_ts: 20260804-001752
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_command_current_source_explanation | PASS | - | 86s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260804-001754 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260804-001754 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
