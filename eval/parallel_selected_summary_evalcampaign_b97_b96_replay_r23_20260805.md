# Selected parallel eval sweep

- date: 2026-08-05T07:28:12Z
- sweep_start_ts: 20260805-002810
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 236s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260805-002812 |
| 2 | qf_multi_member_set_count_caveat | PASS | - | 558s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260805-002812 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
