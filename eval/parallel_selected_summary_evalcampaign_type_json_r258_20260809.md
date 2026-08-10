# Selected parallel eval sweep

- date: 2026-08-10T01:59:33Z
- sweep_start_ts: 20260809-185932
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 171s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260809-185933 |
| 2 | data_json_strict_ids | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:"ids" no_regex_match:"u1" no_regex_match:"u3" | 283s | 0 | 0 | 0 | 0 | 5 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260809-185933 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
