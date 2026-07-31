# Selected parallel eval sweep

- date: 2026-07-31T12:43:31Z
- sweep_start_ts: 20260731-054330
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_e2_cross_trace_asymmetry | FAIL | no_regex_match:((^|[^0-9])14[45](\.[0-9]+)? *(ms|毫秒)|(^|[^0-9])144\.[56]) | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-054331 |
| 2 | cangjie_repomap | FAIL | missing_dimension:package:demo.greeter missing_inventory_row:public_class:Greeter_02_class_init_methods.cj_demo.greeter  | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260731-054331 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
