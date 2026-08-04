# Selected parallel eval sweep

- date: 2026-08-04T05:54:31Z
- sweep_start_ts: 20260803-225429
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 237s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260803-225431 |
| 2 | read_combo_command_current_source_explanation | PASS | - | 544s | 2 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_command_current_source_explanation-20260803-225431 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
