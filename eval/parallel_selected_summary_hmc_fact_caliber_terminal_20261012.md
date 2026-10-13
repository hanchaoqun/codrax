# §232 固定双例：事实口径与恢复终态

- 冻结：`e93c5040619b633f3b3b1fad0b5a5f17c886bde2`；构建 `2026-10-13T01:33:39Z`，dirty仅文档。
- 本机2026-10-12 18:34两例并行启动，各一次，未追第三例。二进制使用selected CLI runner的一例，登记使用显式opt-in真实planner入口的一例；不是声称selected runner运行了两例。
- 原件根：`eval/results/hmc_fact_caliber_terminal_20261012/`；人工依据见[本批审计](parallel_selected_summary_hmc_fact_caliber_terminal_20261012_manual_audit.md)。

| 例 | 形式 | 机器终态 | 人工终态 | 耗时 | 结论 |
|---|---|---|---|---:|---|
| trace_gzip_sqlite_jank_inventory | 完整CLI、真实gzip/SQLite | PASS | FAIL | runner104s / metrics102s | 两条原生数据与全部数值正确；正文字段来源/换算说明、上游阈值与成员总体交接仍不准确 |
| native_registration_restored_terminal | 真实planner＋生产持久化/独立进程恢复，controller决策脚本化 | FAIL，正式exit1 | FAIL | 测试16.11s / Go包18.744s | 来源已verified，精确测试登记和行为证明成功；恢复后源码读取被算为新owner义务，工作流unverified |

合计机器1/2、完整人工0/2。不倒签历史FAIL，不将表格数值正确或proof ledger verified冒充整答/整轮通过。源码后续确定性补救与末版完整回归属于新冻结，不能改写上表真实评测版本。
