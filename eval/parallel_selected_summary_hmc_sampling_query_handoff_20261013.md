# Selected parallel eval sweep

- date: 2026-10-13T09:15:22Z
- sweep_start_ts: 20261013-021521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_query_handoff_20261013

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_gzip_sqlite_jank_multiapp_inventory | PASS | - | 88s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_query_handoff_20261013/trace_gzip_sqlite_jank_multiapp_inventory-20261013-021522 |
| 2 | trace_lossless_sampling_tree | PASS | - | 160s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_query_handoff_20261013/trace_lossless_sampling_tree-20261013-021522 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

人工复核：**0/2 PASS，2/2 FAIL**。以上机器结果原样保留，只表示既有弱oracle通过，不代表完整答案正确。A完整取数后误抄7帧记录appid，B未修复不可用树查询而混合采样分母并拆错原始帧；详见同批manual_audit，不据此销账或倒签历史失败。
