# Selected parallel eval sweep

- date: 2026-08-10T06:18:19Z
- sweep_start_ts: 20260809-231817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | FAIL | mermaid_edges:0<1 | 376s | 1 | 1 | 0 | 2 | 1 | 5 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260809-231819 |
| 1 | data_multifile_reference_projection | PASS | - | 477s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260809-231819 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
