# Selected parallel eval sweep

- date: 2026-08-08T18:59:33Z
- sweep_start_ts: 20260808-115930
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | PASS | - | 299s | 1 | 2 | 0 | 2 | 1 | 0 | 1 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-115933 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 435s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-115933 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
