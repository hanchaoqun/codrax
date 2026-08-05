# Selected parallel eval sweep

- date: 2026-08-05T09:40:20Z
- sweep_start_ts: 20260805-024018
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_sequence_analyzer_gate | FAIL | no_regex_match:buildAnalysisIR[^[:cntrl:]]*analyzerGraphForNormalize[^[:cntrl:]]*analyzer.go:1865[^[:cntrl:]]*analyzer.g | 228s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260805-024020 |
| 2 | qf_multi_member_set_count_caveat | FAIL | missing:KindExternalArtifactDecoded | 241s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260805-024020 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
