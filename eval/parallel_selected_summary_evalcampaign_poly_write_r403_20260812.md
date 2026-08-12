# Selected parallel eval sweep

- date: 2026-08-12T20:00:35Z
- sweep_start_ts: 20260812-130033
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 57s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260812-130035 |
| 1 | mr_poly_binding_chain | PASS | - | 202s | 1 | 1 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260812-130035 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
