# Selected parallel eval sweep

- date: 2026-08-09T08:12:16Z
- sweep_start_ts: 20260809-011215
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_background_demotion | PASS | - | 158s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260809-011216 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 262s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260809-011216 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
