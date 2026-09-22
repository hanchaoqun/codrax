# Selected parallel eval sweep

- date: 2026-09-22T14:43:02Z
- sweep_start_ts: 20260922-074302
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_reader_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dateutil_relativedelta_float | PASS | - | 148s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_reader_20260922/github_issue_dateutil_relativedelta_float-20260922-074302 |
| 2 | trace_query_jank_field_inventory | FAIL | no_primary_text_regex_match:((匹配|符合|满足|共|总计|合计|总数).{0,80}(^|[^0-9])(3|三)([^0-9]|$)|(3|三)[[ | 181s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_reader_20260922/trace_query_jank_field_inventory-20260922-074302 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
