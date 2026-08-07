# Selected parallel eval sweep

- date: 2026-08-07T15:06:03Z
- sweep_start_ts: 20260807-080602
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | no_regex_match:(normalizer\.Normalize|compiler\.Compile|hdp\.Plan|binder\.BindByRelevance|RecomputeBudget) | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-080603 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 182s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260807-080603 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
