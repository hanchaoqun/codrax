# Selected parallel eval sweep

- date: 2026-08-08T12:37:38Z
- sweep_start_ts: 20260808-053737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 66s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260808-053738 |
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]] | 443s | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260808-053738 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
