# Selected parallel eval sweep

- date: 2026-08-14T13:15:19Z
- sweep_start_ts: 20260814-061518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 223s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-061520 |
| 2 | trace_query_state_churn_root_cause_rank | PASS | - | 243s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260814-061520 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
