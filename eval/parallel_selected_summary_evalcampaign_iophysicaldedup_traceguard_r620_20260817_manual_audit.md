# r620 人工审计：IO 物理去重正证与 trace 附件路由漂移

- 基线：B975–B978；有限 IO authority 已加入物理请求 identity 去重。
- 执行：`real_trace_h3_iofam_one_seat` 与 `trace_query_wakeup_causal_runnable`，并发恰好 2，单次回放。
- Runner：0/2 PASS，flagged 2/2。
- 人工：H3 partial；Trace fail。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| real_trace_h3_iofam_one_seat | FAIL | partial | B978 获生产正证：authority 明示 `target_owned_request_rows_rendered=6`，终稿表格也只列 6 个不同物理请求，不再出现 r619 的 7/6 矛盾。1.347ms 请求墙钟、1.337ms completion-closed S-state 目标阻塞墙钟及 4.384ms 下界保留。模型仍无视同段 owner-scope 指令，把全窗 198/8/190 归到目标，并未明确 41.329 request·ms 非墙钟；又把 scheduler D/io_wait=0 扩成“目标没有 IO 阻塞”，与同文 4 段已证 S-state completion-closed wait 矛盾。r619 在相同 typed authority 下曾正确消费，故这是模型消费波动/软教学仍不足，不授权系统改写答案或新增 prose 硬门。 |
| trace_query_wakeup_causal_runnable | FAIL | fail | Dispatcher 明知已加载 runtime trace，却把模型的 `raw_route=operation` 归一成 `route=data`；data planner 随后把内部 `trace_query` 当作仓内待发现工具并寻找 trace 文件，完全绕开预处理、trace_query、因果投影和自动补齐。相同代码在 r617–r619 均走正确 Trace 车道，因此不是 B978 回归，但暴露附件能力与 route enum 之间缺少 typed 一致性约束。 |

## 新处置

`B978-RUNTIMEFACTPHYSICALDEDUP1` 可收为 production-positive。去重只使用 producer-owned exact identity，
没有合并近邻请求，也没有改变 ledger、因果排序或答案。

`B977-IOLATENCYCOVERAGEOWNERSCOPE1` 状态改为 production-mixed：精确信息已经交付，r619 正确消费、r620
消费失败。继续用异构回放判断是否需要更低心智的 typed scope ledger；禁止用答案关键词校验、系统结论注入或改写模型正文闭环。

`B979-RUNTIMEATTACHMENTINTERNALTOOLROUTECONSISTENCY1/P1`：路由应消费已加载附件的 typed kind/capability，
而不是把用户/模型提到的工具名当外部 operation。带 trace 附件且请求只需要内部 Trace 调查能力时，`operation/data`
必须 fail-loud 重试或回到 repo Trace pipeline；真实外部数据操作仍保持 data route。施工不得扫描用户原文关键词，
不得以 case 名或 `trace_query` 字符串硬门。

活跃流未观察到固定 4ms 降级；两案失败均发生在已完成流后的 route/语义阶段。
