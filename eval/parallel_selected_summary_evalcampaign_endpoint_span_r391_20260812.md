# Selected parallel eval sweep

- date: 2026-08-12T14:39:07Z
- sweep_start_ts: 20260812-073906
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_java_call_chain | PASS | - | 132s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260812-073908 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_regex_match:(normalizer\.Normalize|compiler\.Compile|hdp\.Plan|binder\.BindByRelevance|RecomputeBudget) | 220s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260812-073908 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
