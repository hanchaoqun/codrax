# Selected parallel eval sweep

- date: 2026-08-08T22:05:56Z
- sweep_start_ts: 20260808-150555
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | PASS | - | 222s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-150556 |
| 1 | qf_diagram_pipeline | PASS | - | 412s | 1 | 1 | 0 | 2 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-150556 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
