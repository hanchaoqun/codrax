# Selected parallel eval sweep

- date: 2026-08-04T03:28:43Z
- sweep_start_ts: 20260803-202842
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap_fixture | PASS | - | 95s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260803-202843 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 240s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-202843 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
