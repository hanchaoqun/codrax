# Selected parallel eval sweep

- date: 2026-08-11T04:21:10Z
- sweep_start_ts: 20260810-212106
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 186s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260810-212110 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 | 659s | 1 | 2 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260810-212110 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
