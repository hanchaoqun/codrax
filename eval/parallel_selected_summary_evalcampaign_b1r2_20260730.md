# Selected parallel eval sweep

- date: 2026-07-31T04:24:19Z
- sweep_start_ts: 20260730-212419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | no_regex_match:(^|[^0-9])(3|三) ?[次条].*(iowait|io_?wait|IO|blocked_reason|D ?状态|D-state)|(iowait|io_?wait|IO|bl | 115s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260730-212419 |
| 2 | github_issue_zod_prefault_symptom | PASS | - | 288s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260730-212419 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
