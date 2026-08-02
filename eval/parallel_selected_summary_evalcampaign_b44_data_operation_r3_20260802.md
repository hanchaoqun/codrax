# Selected parallel eval sweep

- date: 2026-08-02T17:18:47Z
- sweep_start_ts: 20260802-101846
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_web_manual_summary | FAIL | no_regex_match:(用户(使用)?手册|使用指南|manual|guide) | 112s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260802-101847 |
| 1 | data_json_strict_ids | FAIL | no_regex_match:"ids" no_regex_match:"u1" no_regex_match:"u3" | 210s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260802-101847 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
