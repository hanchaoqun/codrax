# Selected parallel eval sweep

- date: 2026-08-11T11:59:13Z
- sweep_start_ts: 20260811-045912
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 173s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260811-045913 |
| 1 | read_combo_pipeline_sequence_table | FAIL | missing:Mutable | 544s | 1 | 1 | 0 | 1 | 0 | 5 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-045913 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
