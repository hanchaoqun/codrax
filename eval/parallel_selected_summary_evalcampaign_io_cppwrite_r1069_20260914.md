# Selected parallel eval sweep

- date: 2026-09-14T08:17:33Z
- sweep_start_ts: 20260914-011733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | FAIL | write_final_verdict:unverified:proof_weak | 174s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260914-011733 |
| 1 | real_trace_h3_iofam_one_seat | FAIL | no_regex_match:(单次请求|设备端).*(墙钟|墙上|wall.clock) | 334s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260914-011733 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
