# Selected parallel eval sweep

- date: 2026-08-08T07:13:47Z
- sweep_start_ts: 20260808-001345
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260808-001347 |
| 1 | trace_query_wakeup_background_demotion | FAIL | no_regex_match:(唤醒链|唤醒依赖链|wakeup chain|on_wakeup_chain|链路|依赖链|级联唤醒).*(threadpool-400|n | 201s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260808-001347 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
