# Selected parallel eval sweep

- date: 2026-08-06T06:43:16Z
- sweep_start_ts: 20260805-234315
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_diagram_pipeline | PASS | - | 124s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260805-234316 |
| 2 | data_multifile_reference_projection | PASS | - | 163s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260805-234316 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
