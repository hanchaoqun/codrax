# Selected parallel eval sweep

- date: 2026-10-09T09:32:43Z
- sweep_start_ts: 20261009-023242
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_measurements_obligations_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_measurement_records | PASS | - | 183s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_measurements_obligations_20261009/trace_measurement_records-20261009-023244 |
| 2 | trace_transaction_handoffs | PASS | - | 442s | 1 | 1 | 0 | 1 | 0 | 9 | 10 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_measurements_obligations_20261009/trace_transaction_handoffs-20261009-023244 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

以上仅机器执行判定。完整人工审计 **0/2**：量测原值/身份/协议边界错误；事务主要数字/逐键事实正确，但必需交接图零边且误称未证。见[人工审计](parallel_selected_summary_hmc_measurements_obligations_20261009_manual_audit.md)。旧失败保留，不以机器PASS销账。
