# Selected parallel eval sweep

- date: 2026-08-28T13:50:57Z
- sweep_start_ts: 20260828-065055
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 208s | 1 | 2 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260828-065057 |
| 1 | qf_logic_view_read_pipeline | FAIL | mermaid_incident_nodes:4<6 | 526s | 1 | 2 | 0 | 2 | 1 | 7 | 8 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260828-065057 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
