# Selected parallel eval sweep

- date: 2026-08-07T18:13:21Z
- sweep_start_ts: 20260807-111320
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap_fixture | PASS | - | 57s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260807-111321 |
| 2 | data_json_strict_ids | PASS | - | 61s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260807-111321 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
