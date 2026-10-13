# §233 请求身份修复后的固定双例

日期：2026-10-12（本机）。代码冻结：`9a997c96819b`；构建时间`2026-10-13T02:42:14Z`，dirty仅统一文档，Go与HEAD一致。两个真实评测并行、各一次，没有追跑第三例。输入12份SHA-256启动前全部一致。

| 评测 | 机器 | 完整人工 | 耗时 | 覆盖边界 |
| --- | --- | --- | --- | --- |
| `trace_gzip_sqlite_jank_inventory` | PASS | FAIL（最终数值/表格通过；声明与查询范围交接未通过） | 118s runner，114s pipeline | 实际CLI gzip→SQLite只读准备→原生事件查询；不是外部转换器stub，也不是实机采样验收 |
| `native_registration_restored_terminal` | PASS | PASS（限定终态路径） | test14.45s，package23.341s | 真实planner读取/登记、实际原生验证、独立进程恢复；controller为测试adapter，不冒充完整CLI/模型controller |

机器2/2，完整人工1/2。§232的两份人工FAIL不倒签；本次登记证明修后限定路径已通，不代表18.5永久完成。IO请求身份修复由实际TraceQuery→投影→Emit→ParseOutput公开回归验收，不把这两个轮换例说成IO同值碰撞live命中。

原件目录：`eval/results/hmc_request_identity_terminal_20261012/`。完整过程、上下文、最终正文/表格及旁路结论见[人工审计](parallel_selected_summary_hmc_request_identity_terminal_20261012_manual_audit.md)。全仓末版结果、提交推送与未覆盖范围见[统一账本§233](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
