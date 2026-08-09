# Selected parallel eval sweep

- date: 2026-08-09T10:39:33Z
- sweep_start_ts: 20260809-033932
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 52s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260809-033933 |
| 1 | data_basic_sum_with_rules | PASS | - | 292s | 0 | 0 | 0 | 0 | 5 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260809-033934 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
