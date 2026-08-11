# Selected parallel eval sweep

- date: 2026-08-11T05:58:11Z
- sweep_start_ts: 20260810-225809
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260810-225811 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 366s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260810-225811 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
