# Selected parallel eval sweep

- date: 2026-08-14T12:41:45Z
- sweep_start_ts: 20260814-054144
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | s8a | PASS | - | 329s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/s8a-20260814-054146 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 355s | 1 | 1 | 0 | 1 | 0 | 5 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-054145 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
