# Selected parallel eval sweep

- date: 2026-08-07T17:13:36Z
- sweep_start_ts: 20260807-101335
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap_fixture | PASS | - | 90s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260807-101336 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 266s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 1 | none | eval/results/qf_sequence_analyzer_gate-20260807-101336 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
