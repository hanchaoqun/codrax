# Selected parallel eval sweep

- date: 2026-09-21T12:16:56Z
- sweep_start_ts: 20260921-051654
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_user_target_authority_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((CPU ?=? ?4|cpu ?=? ?4).{0,240}(2\.10 ?GHz|2\.1 ?GHz|2100 ?MHz|2100000 ?kHz).{0,160}(上� | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_user_target_authority_20260921/real_trace_h4_supply_thermal_witness-20260921-051656 |
| 1 | trace_query_business_marker_io_chain | FAIL | no_primary_text_regex_match:((不可|不能|不应|不要|不再|避免).{0,60}(相加|叠加|重复)|(相加|叠加|重 | 261s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_user_target_authority_20260921/trace_query_business_marker_io_chain-20260921-051656 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
