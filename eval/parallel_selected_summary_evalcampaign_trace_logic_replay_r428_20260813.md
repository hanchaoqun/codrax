# Selected parallel eval sweep

- date: 2026-08-13T09:14:11Z
- sweep_start_ts: 20260813-021409
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | FAIL | no_regex_match:(```mermaid|flowchart|graph[[:space:]]+(TD|LR)) mermaid_edges:0<1 | 229s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260813-021411 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:49.623 no_regex_match:供给折算缺口 65\.912ms no_regex_match:同源二分:全窗49\.656ms=锚定0\.033ms\(� | 248s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-021411 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
