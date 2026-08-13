# Selected parallel eval sweep

- date: 2026-08-13T15:11:01Z
- sweep_start_ts: 20260813-081100
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | missing:4次(3.774~16.064ms) no_regex_match:自身·D-state(\(对端未解析\))? 36\.757ms no_regex_match:等待对象[ | 206s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260813-081101 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:65.912 missing:49.623 missing:0.033 missing:未计价占用 no_regex_match:供给折算缺口 65\.912ms no_regex | 212s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-081101 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
