# Selected parallel eval sweep

- date: 2026-08-06T02:08:38Z
- sweep_start_ts: 20260805-190837
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 144s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260805-190838 |
| 1 | data_multifile_reference_projection | FAIL | no_log_regex:\[cli/data\] data task result.*contributions=4.*reconcile=pass | 210s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260805-190838 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
