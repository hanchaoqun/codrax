# Selected parallel eval sweep

- date: 2026-08-04T16:34:41Z
- sweep_start_ts: 20260804-093440
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | missing_inventory_section:entry_page:@Entry_ArkTS missing_inventory_section:builder_fragment:@Builder | 88s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260804-093441 |
| 2 | cangjie_repomap_fixture | PASS | - | 120s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260804-093441 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
