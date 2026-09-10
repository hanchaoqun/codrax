# Selected parallel eval sweep

- date: 2026-09-10T01:56:16Z
- sweep_start_ts: 20260909-185614
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | FAIL | no_plan_regex:"kind":[[:space:]]*"patch" | 187s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260909-185616 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 288s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260909-185616 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
