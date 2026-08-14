# Selected parallel eval sweep

- date: 2026-08-14T10:05:11Z
- sweep_start_ts: 20260814-030510
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260814-030512 |
| 1 | data_multifile_reference_projection | PASS | - | 201s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260814-030512 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
