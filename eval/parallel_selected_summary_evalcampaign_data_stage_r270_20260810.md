# Selected parallel eval sweep

- date: 2026-08-10T15:57:46Z
- sweep_start_ts: 20260810-085744
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 284s | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-085746 |
| 2 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 460s | 1 | 2 | 0 | 1 | 0 | 8 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-085746 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
