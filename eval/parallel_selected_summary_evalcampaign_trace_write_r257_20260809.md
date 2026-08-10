# Selected parallel eval sweep

- date: 2026-08-10T01:28:16Z
- sweep_start_ts: 20260809-182814
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 68s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260809-182816 |
| 1 | trace_query_core_topology_supply | PASS | - | 139s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_core_topology_supply-20260809-182816 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
