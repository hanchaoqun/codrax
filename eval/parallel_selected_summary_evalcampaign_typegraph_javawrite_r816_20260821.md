# Selected parallel eval sweep

- date: 2026-08-21T16:57:24Z
- sweep_start_ts: 20260821-095723
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 55s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260821-095724 |
| 1 | qf_type_relation_loop_controller | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260821-095724 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
