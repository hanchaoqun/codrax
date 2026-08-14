# Selected parallel eval sweep

- date: 2026-08-14T12:22:53Z
- sweep_start_ts: 20260814-052252
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | s8a | FAIL | no_regex_match:(normalizer\.Normalize|TermGraph) no_regex_match:(hdp\.Plan|hypothes|HypothesisSet|binder\.BindByRelevanc | 299s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | none | eval/results/s8a-20260814-052254 |
| 1 | qf_sequence_analyzer_gate | FAIL | no_text_regex_match:gate\.Run([^A-Za-z0-9_]|$).*RunWith.*gate\.go:[0-9]+ | 322s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260814-052253 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
