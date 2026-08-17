# Selected parallel eval sweep

- date: 2026-08-17T00:08:47Z
- sweep_start_ts: 20260816-170846
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | inventory_count_mismatch:entry_page:got8:want4 | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260816-170847 |
| 2 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:49.623 missing:0.033 missing:按全域最大核最高频 missing:enumeration_status=incomplete missing:未计价 | 232s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-170847 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
