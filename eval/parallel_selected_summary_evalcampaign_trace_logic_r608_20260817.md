# Selected parallel eval sweep

- date: 2026-08-17T10:18:17Z
- sweep_start_ts: 20260817-031816
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | - | 151s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-031817 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 318s | 1 | 1 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260817-031818 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
