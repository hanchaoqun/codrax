# Selected parallel eval sweep

- date: 2026-08-08T21:28:41Z
- sweep_start_ts: 20260808-142840
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | PASS | - | 422s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-142841 |
| 1 | qf_diagram_pipeline | PASS | - | 739s | 1 | 1 | 0 | 2 | 1 | 11 | 12 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-142841 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
