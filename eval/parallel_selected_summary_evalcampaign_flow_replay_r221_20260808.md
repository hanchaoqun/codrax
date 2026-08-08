# Selected parallel eval sweep

- date: 2026-08-08T18:08:20Z
- sweep_start_ts: 20260808-110818
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | FAIL | mermaid_edges:0<1 | 129s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-110820 |
| 1 | qf_logic_view_read_pipeline | FAIL | mermaid_edges:0<1 | 403s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-110820 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
