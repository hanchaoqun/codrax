# Selected parallel eval sweep

- date: 2026-08-02T05:57:48Z
- sweep_start_ts: 20260801-225746
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 81s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260801-225748 |
| 1 | sr_java_config_precedence | PASS | - | 94s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_config_precedence-20260801-225748 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
