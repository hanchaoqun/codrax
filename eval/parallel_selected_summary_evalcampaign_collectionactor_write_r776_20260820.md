# Selected parallel eval sweep

- date: 2026-08-20T14:51:12Z
- sweep_start_ts: 20260820-075110
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 92s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260820-075112 |
| 1 | qf_type_relation_loop_controller | PASS | - | 206s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260820-075112 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
