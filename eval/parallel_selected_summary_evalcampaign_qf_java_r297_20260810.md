# Selected parallel eval sweep

- date: 2026-08-11T06:21:32Z
- sweep_start_ts: 20260810-232130
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260810-232132 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 634s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260810-232132 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
