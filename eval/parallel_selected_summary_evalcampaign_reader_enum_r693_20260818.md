# Selected parallel eval sweep

- date: 2026-08-18T16:10:01Z
- sweep_start_ts: 20260818-090959
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 215s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260818-091001 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 383s | 1 | 1 | 0 | 1 | 0 | 4 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260818-091001 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
