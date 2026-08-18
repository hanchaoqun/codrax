# Selected parallel eval sweep

- date: 2026-08-18T02:12:47Z
- sweep_start_ts: 20260817-191245
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 204s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-191247 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 403s | 1 | 2 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260817-191247 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
