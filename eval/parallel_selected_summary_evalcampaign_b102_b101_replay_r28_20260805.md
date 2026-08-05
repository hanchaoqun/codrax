# Selected parallel eval sweep

- date: 2026-08-05T10:27:34Z
- sweep_start_ts: 20260805-032732
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:exported_functions:5 banned:iota no_regex_match:(函数.*(^|[^0-9])5([^0-9]|$)|(^|[^0-9]) | 227s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260805-032734 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_regex_match:buildAnalysisIR[^[:cntrl:]]*analyzerGraphForNormalize[^[:cntrl:]]*analyzer.go:1865[^[:cntrl:]]*analyzer.g | 272s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260805-032734 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
