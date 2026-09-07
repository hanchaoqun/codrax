# Selected parallel eval sweep

- date: 2026-09-07T03:14:15Z
- sweep_start_ts: 20260906-201404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 104s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260906-201415 |
| 1 | cangjie_repomap_fixture | PASS | - | 117s | 1 | 3 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260906-201415 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
