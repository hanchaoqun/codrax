# Selected parallel eval sweep

- date: 2026-08-11T06:39:26Z
- sweep_start_ts: 20260810-233925
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 186s | 2 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260810-233926 |
| 1 | read_combo_pipeline_sequence_table | TIMEOUT | exceeded 1200s wall-time | 1200s | 1 | 3 | 0 | 1 | 0 | 5 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260810-233926 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
