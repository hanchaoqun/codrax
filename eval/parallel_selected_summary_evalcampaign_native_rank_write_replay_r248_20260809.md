# Selected parallel eval sweep

- date: 2026-08-09T08:59:52Z
- sweep_start_ts: 20260809-015951
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 76s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260809-015952 |
| 1 | data_json_strict_ids | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:"ids" no_regex_match:"u1" no_regex_match:"u3" | 388s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260809-015952 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
