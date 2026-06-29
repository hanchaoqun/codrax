# Selected parallel eval sweep

- date: 2026-06-29T14:12:57Z
- sweep_start_ts: 20260629-221256
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | read_combo_trace_current_source_explanation | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260629-221257 |
| 1 | cangjie_repomap | PASS | - | 165s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-221257 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
