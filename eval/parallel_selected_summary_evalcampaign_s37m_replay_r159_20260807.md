# Selected parallel eval sweep

- date: 2026-08-07T10:46:40Z
- sweep_start_ts: 20260807-034638
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_sequence_analyzer_gate | FAIL | no_regex_match:buildAnalysisIR[^[:cntrl:]]*analyzerGraphForNormalize[^[:cntrl:]]*analyzer.go:1865[^[:cntrl:]]*analyzer.g | 105s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-034640 |
| 1 | github_issue_napi_force_wasi_env_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 148s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_napi_force_wasi_env_symptom-20260807-034640 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
