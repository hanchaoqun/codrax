# Selected parallel eval sweep

- date: 2026-09-11T03:41:37Z
- sweep_start_ts: 20260910-204137
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap_fixture | PASS | - | 68s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260910-204137 |
| 1 | real_trace_h9_conversion_single_basis | FAIL | missing:3.309 missing:1.023 no_regex_match:有效归因 3\.309ms = runnable\(全额\) 1\.023ms \+ running\(折算\) 2\.2 | 182s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h9_conversion_single_basis-20260910-204137 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
