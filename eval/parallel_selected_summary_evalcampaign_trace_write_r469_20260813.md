# Selected parallel eval sweep

- date: 2026-08-14T05:12:16Z
- sweep_start_ts: 20260813-221213
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_chrono_duration_min | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 352s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min-20260813-221216 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | banned:132.041 no_principal_text_regex_match:((CPU ?=? ?4|cpu ?=? ?4).{0,240}(2\.10 ?GHz|2\.1 ?GHz|2100 ?MHz|2100000 ?kH | 376s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260813-221216 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
