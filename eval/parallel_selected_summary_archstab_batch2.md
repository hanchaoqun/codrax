# Selected parallel eval sweep

- date: 2026-07-03T01:54:29Z
- sweep_start_ts: 20260703-095429
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | qf_architecture | FAIL | missing:analyze missing:explore missing:extract missing:finalize | 102s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260703-095429 |
| 2 | qf_diagram_pipeline | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260703-095429 |
| 3 | mr_implementers | PASS | - | 57s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_implementers-20260703-095611 |
| 5 | u9a | PASS | - | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/u9a-20260703-095708 |
| 4 | s1a | FAIL | missing:gate.Run | 184s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/s1a-20260703-095647 |
| 6 | trace_query_wakeup_causal_io_chain | PASS | - | 243s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260703-095905 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 2**
