# Selected parallel eval sweep

- date: 2026-08-14T12:07:15Z
- sweep_start_ts: 20260814-050713
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | PASS | - | 316s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-050715 |
| 2 | s8a | PASS | - | 324s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/s8a-20260814-050715 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
