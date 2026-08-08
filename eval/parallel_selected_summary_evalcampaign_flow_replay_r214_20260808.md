# Selected parallel eval sweep

- date: 2026-08-08T13:55:27Z
- sweep_start_ts: 20260808-065526
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 215s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-065528 |
| 2 | qf_diagram_pipeline | PASS | - | 366s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-065528 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
