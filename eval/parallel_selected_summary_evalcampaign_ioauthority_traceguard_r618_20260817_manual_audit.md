# r618 人工审计：IO 精确事实进入终稿；全窗覆盖被误归目标

- 基线：`main@7516b2a2e` 加 B975/B976 首版。
- 执行：`real_trace_h3_iofam_one_seat` 与 `trace_query_wakeup_causal_runnable`，并发恰好 2，单次回放。
- Runner：H3 FAIL；Trace 因果对照 PASS。
- 人工：H3 major-partial；Trace pass。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| real_trace_h3_iofam_one_seat | FAIL | major-partial | 事实通道已生产生效：终稿正确区分 target scheduler D/io_wait=0、S sleep=70.338ms、单请求 residence 0.865–1.347ms、4 条 completion-closed issuer wait 并集 4.384ms，以及 198/8/190、41.329 request·ms 的截断边界。旧 runner 仍要求系统投影固定词 `完成端到端·IO延迟/块设备IO(inode)/综合评分`，属于过硬旧 oracle；有限事实答案不应被迫恢复系统代写席位。真实残余有两点：模型把全窗 emitted=8 说成目标自身 8 条请求；41.329 虽披露并说明请求并发，却没有显式写“非墙钟/不可相加”。 |
| trace_query_wakeup_causal_runnable | PASS | pass | 完整 causal diagnosis 仍形成链上根因，有限事实 authority 没有改动投影、补齐、根因选举或模型结论。 |

## 后续处置

1. `B977-IOLATENCYCOVERAGEOWNERSCOPE1/P1`：同一个 family 内还要区分 target-owned leaf 与 selected-window/global coverage；明确 global 8/198/190 不得成为目标请求数，目标可见 leaf 只是一组有界 witness，截断时不能宣称总数。
2. H3 oracle 改为物理量与 scope 语义：1.347 单请求墙钟、1.337 目标阻塞墙钟、41.329 request·ms 非墙钟/不可相加、scheduler D/io-wait=0；退役系统投影专有固定词，不降低 trace_query 调用与精确数值要求。
3. 不增加答案关键词硬门，不系统改写模型答案。若 r619 仍遗漏 41.329 的量纲分类，按模型消费波动留档，不能恢复旧 system-authored projection。
