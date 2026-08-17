# r622 人工审计：IO 关系桥到达但跨阶段旧口径反向污染

- 基线：`main@b2fcc988d`
- 严格并发：恰好 2 个案例
- Runner：`1 PASS / 1 FAIL`
- 人工：Trace `pass`；H3 `fail`

| case | runner | 人工 | 审计结论 |
|---|---|---|---|
| `trace_query_wakeup_causal_runnable` | PASS | pass | 显式窗、typed 查询、`worker-200 → app-100` 唤醒链、8.300ms 链上调度等待、因果投影和自动补齐均保留；邻近/背景未晋升主因。 |
| `real_trace_h3_iofam_one_seat` | FAIL | fail | 最终上下文确实带 B980 关系桥，并正确写出 4 个 completion-closed S 等待的 union≥4.384ms、scheduler D/io_wait=0 与 41.329 request·ms 非墙钟；但漏掉 1.337ms 单次 target-blocking witness，并仍把 1.347ms issue→complete elapsed 写成“否（非墙钟）”。同时复制 `request_residence`、`completion_closed_issuer_wait`、`coverage=lower_bound` 等 wire 词，读者语言仍不干净。 |

## 深层原因

B980 的 finalizer 关系桥已经准确到达（日志 1894–1900），所以这不是 ledger、显示预算或最终接线缺失。更早的 explorer 在聚合事实中把
`request_residence=1.347ms` 铸成 `unit=non_wall_clock_ms`，并把“请求可并发、不可跨请求求和”错误扩大为“单个请求的经过时间不是墙钟”。
该错误 aggregate fact 随后与 finalizer 的正确 typed authority 同时进入上下文；模型沿用了先到的错误事实。系统因此向同一模型提供了互相矛盾的
量纲合同。仅在 finalizer 继续叠加教学不能闭环。

## 最优根修与边界

新立 `B981-IOCLOCKSCOPECROSSSTAGE1/P1`：把物理量尺说明前移到 explorer 首次消费的 `trace_query` 文本行，并与
typed observation 使用同一 scope：

- `issue→complete`：单请求 elapsed wall clock；不是目标线程 blocking wall clock；
- `completion-closed issuer interval`：目标线程 blocking wall clock，S/D 均可；
- 多请求 `request·ms`：可重叠、非墙钟、不可相加。

该修复只发布 producer-owned typed measurement scope，不扫描问题或答案原文，不改写 explorer/finalizer 结论，不把背景行提升为主因，
也不改变完整 causal-diagnosis 的查询、投影和补齐。`b1eb11121` 已实现并以 tool/agent/types 包测试钉住；r623 负责生产回放。

内部 wire enum 泄漏作为 `B982-RUNTIMEFACTREADERFACE1/P1` 继续观察：优先方案是由 typed family 生成 reader-facing 卡、将 raw row
降为机器审计载荷，而不是对模型输出做关键词删除。是否施工要先看 r623 在统一源后是否仍发生，避免重复干预。

状态：

- `B980-IOMEASUREMENTRELATIONBRIDGE1=production-delivered-r622/insufficient-alone`
- `B981-IOCLOCKSCOPECROSSSTAGE1=implemented/producer-first+pinned/pending-r623`
- `B982-RUNTIMEFACTREADERFACE1=open/observe-r623`
- `Trace explicit-window causal projection/auto-supplement=pass-r622`
- Trace root=`typed-on-chain-only`; adjacent/background=`support-only`
- `system-answer/conclusion-authorship=none`
- `active-stream-4ms-degrade=forbidden/not-observed`
