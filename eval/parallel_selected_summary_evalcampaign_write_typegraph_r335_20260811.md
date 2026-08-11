# Selected parallel eval sweep

- date: 2026-08-11T20:35:13Z
- sweep_start_ts: 20260811-133511
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_cpp_typo | PASS | - | 56s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260811-133513 |
| 2 | qf_type_relation_loop_controller | PASS | - | 407s | 1 | 4 | 0 | 2 | 1 | 5 | 6 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260811-133513 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
