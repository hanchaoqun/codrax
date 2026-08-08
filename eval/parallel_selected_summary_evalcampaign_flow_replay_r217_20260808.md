# Selected parallel eval sweep

- date: 2026-08-08T15:55:40Z
- sweep_start_ts: 20260808-085539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-085540 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 305s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-085540 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
