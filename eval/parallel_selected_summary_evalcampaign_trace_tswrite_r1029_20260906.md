# Selected parallel eval sweep

- date: 2026-09-07T06:47:19Z
- sweep_start_ts: 20260906-234718
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | write_final_verdict:unverified:proof_weak | 134s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260906-234719 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:49.623 no_regex_match:同源二分:全窗49\.656ms=锚定0\.033ms\(锚定席未上本榜,见明细\)\+本行其� | 218s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260906-234719 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
