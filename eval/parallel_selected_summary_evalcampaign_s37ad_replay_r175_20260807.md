# Selected parallel eval sweep

- date: 2026-08-07T16:37:26Z
- sweep_start_ts: 20260807-093724
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | PASS | - | 88s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260807-093726 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 474s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-093726 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
