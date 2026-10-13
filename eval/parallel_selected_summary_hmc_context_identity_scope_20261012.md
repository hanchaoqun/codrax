# §234 固定双例机器收据

2026-10-12 23:37（本地）并行启动，恰好2例、各1次；代码`a76d7fb12fcb`，dirty仅文档。这是CLI读路径加真实planner/独立恢复测试路径，不声称两例都是完整CLI。结果根：`eval/results/hmc_context_identity_scope_20261012/`。

| 场景 | 机器结论 | 收据 |
| --- | --- | --- |
| `trace_gzip_sqlite_jank_multiapp_inventory` | FAIL，87秒；缺appid=41002及其两行纳秒端点 | `trace_gzip_sqlite_jank_multiapp_inventory-20261012-233733/{run-1.verdict,summary.md,run-1.logs/}` |
| `native_registration_restored_terminal` | PASS，测试20.90秒/package21.680秒；正式exit0 | `native_registration_restored_terminal/{receipt.json,restart-context-pack-audit.json,restart-child-audit.json,logs/}` |

机器1/2。读例进程exit0不等于答案PASS，原始`run-1.verdict`为FAIL；trace_query调用次数1也不等于成功执行，其唯一调用被请求出处校验拒绝。独立oracle为阈值3、4条匹配、2个appid，不能把两条预览当全量。

源case/SQL/SQLite/gzip、登记测试及runner共9份启动前后SHA-256一致。完整过程与答案人工结论见[人工审计](parallel_selected_summary_hmc_context_identity_scope_20261012_manual_audit.md)。本轮后续确定性补救不改这两份原始verdict，也不再追加live追绿。
