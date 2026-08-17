# r621 人工审计：trace 附件路由生产闭环与 IO 量尺关系歧义

- 基线：`main@8b7214505`，B979 fresh-runtime-attachment typed route guard。
- 执行：`trace_query_wakeup_causal_runnable` 与 `real_trace_h3_iofam_one_seat`，并发恰好 2，单次回放。
- Runner：1/2 PASS，flagged 1/2。
- 人工：Trace pass；H3 fail。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| trace_query_wakeup_causal_runnable | PASS | pass | B979 生产闭环：route=repo，执行 3 次 trace_query；`worker-200 → app-100` 真链、8.300ms runnable 反转候选、链上-only 主因、背景 supply pressure 及完整 Trace 因果投影均保留。无 data workflow、成文拒绝、答案恢复或固定 4ms 降级。 |
| real_trace_h3_iofam_one_seat | FAIL | fail | 路由和 6-request 物理去重均正确，1.347/1.337/4.384 数值保留；但模型把单请求 issue→complete 的 elapsed wall clock 错写成“非墙钟”，把只覆盖 D/io_wait 与 blocked-reason-iowait S 的零 roster 扩大成“interruptible S 中也没有 IO 等待”，与 4 段 completion-closed S wait 自相矛盾；41.329 request·ms 未标非墙钟/不可相加，并直接泄漏 `status=complete`、`capacity_truncated`、`completion_closed_issuer_wait` 等内部枚举。 |

## 判定与后续

`B979-RUNTIMEATTACHMENTINTERNALTOOLROUTECONSISTENCY1` 收为 production-closed。其修复只收敛 route，未改变
Trace 查询、因果补齐、投影或答案。

新立 `B980-IOMEASUREMENTRELATIONBRIDGE1/P1`。现有精确事实分散在三块 authority 中，而且
`target_window_wait_occurrences` 的表面名称比实际集合更宽：producer 真义只含 D/io_wait，以及带
blocked_reason iowait=1 的 S；它不覆盖 completion-closed 但没有该 marker 的 S 等待。最优方案是由同一 typed ledger
生成一张紧凑的 IO ruler/scope bridge，显式并置四个互不替代的集合：单请求 elapsed wall clock、completion-closed
目标阻塞 wall clock、scheduler D/io_wait roster、全窗 request·ms 覆盖值，并给出零值不可否定另一个集合的关系。
这是模型上下文，不是系统答案；不得新增答案关键词门或正文改写。
