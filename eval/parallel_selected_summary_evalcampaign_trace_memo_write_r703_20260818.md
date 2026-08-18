# Selected parallel eval sweep

- date: 2026-08-18T20:57:38Z
- sweep_start_ts: 20260818-135737
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 149s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260818-135738 |
| 1 | trace_query_wakeup_causal_io_chain | FAIL | no_regex_match:(中间|传递|链路|path|intermediate|仅传递).*(network|cookie)|(network|cookie).*(中间|传递|链 | 257s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260818-135738 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
