# Selected parallel eval sweep

- date: 2026-09-10T15:16:38Z
- sweep_start_ts: 20260910-081636
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 273s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260910-081638 |
| 2 | read_combo_pipeline_sequence_table | TIMEOUT | exceeded 1200s wall-time | 1200s | 1 | 2 | 0 | 1 | 0 | 15 | 14 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260910-081638 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
