# Selected parallel eval sweep

- date: 2026-09-10T11:23:57Z
- sweep_start_ts: 20260910-042356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h6_channel_mixed_display | FAIL | missing:根因排序#1 missing:❶ | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h6_channel_mixed_display-20260910-042357 |
| 2 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 692s | 1 | 1 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260910-042357 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
