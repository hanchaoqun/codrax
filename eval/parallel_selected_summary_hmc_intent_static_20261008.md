# Selected parallel eval sweep

- date: 2026-10-08T12:11:41Z
- sweep_start_ts: 20261008-051140
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_intent_static_20261008

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_catalog_objects | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/hmc_intent_static_20261008/trace_catalog_objects-20261008-051141 |
| 2 | trace_static_initialize | PASS | - | 230s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_intent_static_20261008/trace_static_initialize-20261008-051141 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
