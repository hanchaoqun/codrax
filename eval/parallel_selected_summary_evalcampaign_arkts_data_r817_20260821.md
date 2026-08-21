# Selected parallel eval sweep

- date: 2026-08-21T17:01:32Z
- sweep_start_ts: 20260821-100130
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 183s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260821-100132 |
| 1 | arkts_repomap | FAIL | degraded_answer_checks_skipped:1 inventory_count_mismatch:builder_fragment:got6:want2 | 514s | 1 | 1 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260821-100132 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
