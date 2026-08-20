# Selected parallel eval sweep

- date: 2026-08-20T20:55:32Z
- sweep_start_ts: 20260820-135530
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | FAIL | no_regex_match:(低优先级|lower.priority|priority inversion|优先级反转|prio) | 203s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-135533 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 | 1666s | 1 | 1 | 0 | 2 | 0 | 34 | 31 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-135532 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
