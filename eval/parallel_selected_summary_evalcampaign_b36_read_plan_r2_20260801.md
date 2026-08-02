# Selected parallel eval sweep

- date: 2026-08-02T05:27:16Z
- sweep_start_ts: 20260801-222715
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 89s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260801-222717 |
| 1 | sr_java_config_precedence | PASS | - | 93s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_config_precedence-20260801-222716 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
