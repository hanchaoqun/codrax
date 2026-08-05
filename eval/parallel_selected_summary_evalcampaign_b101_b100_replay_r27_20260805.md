# Selected parallel eval sweep

- date: 2026-08-05T10:11:55Z
- sweep_start_ts: 20260805-031154
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | no_regex_match:buildAnalysisIR[^[:cntrl:]]*analyzerGraphForNormalize[^[:cntrl:]]*analyzer.go:1865[^[:cntrl:]]*analyzer.g | 227s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260805-031156 |
| 2 | qf_multi_member_set_count_caveat | PASS | - | 236s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260805-031156 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
