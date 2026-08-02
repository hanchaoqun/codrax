# Selected parallel eval sweep

- date: 2026-08-02T00:24:27Z
- sweep_start_ts: 20260801-172426
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_pipeline_sequence_table | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260801-172427 |
| 1 | trace_query_state_churn_root_cause_rank | PASS | - | 162s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260801-172427 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
