# Selected parallel eval sweep

- date: 2026-08-18T09:22:47Z
- sweep_start_ts: 20260818-022246
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_c_platform_fork | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_c_platform_fork-20260818-022247 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 258s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260818-022247 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
