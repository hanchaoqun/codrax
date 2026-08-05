# Selected parallel eval sweep

- date: 2026-08-05T05:02:51Z
- sweep_start_ts: 20260804-220248
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_multi_member_set_count_caveat | FAIL | dynamic_scalar_binding_missing:kind_constants:30 missing:KindExternalArtifactDecoded | 205s | 2 | 1 | 0 | 2 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260804-220251 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 613s | 3 | 4 | 0 | 7 | 0 | 7 | 6 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260804-220251 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
