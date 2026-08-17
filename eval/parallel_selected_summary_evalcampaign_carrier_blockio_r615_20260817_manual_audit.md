# r615 人工审计：read pipeline 载体关系 + IO 多口径有限事实

- 基线：`main@abcf0ab0562fd49ec1deb1539d63ac10ff4f834d`
- 执行：`qf_logic_view_read_pipeline` 与 `real_trace_h3_iofam_one_seat`，并发恰好 2，单次回放。
- Runner：QF PASS（564s）；H3 FAIL（178s）。
- 人工：QF major-partial；H3 fail。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| qf_logic_view_read_pipeline | PASS | major-partial | B971 获生产正证：四阶段 precedence 不再被单个同名源码符号拆散，completion 缺席集只剩 `Mutable/BusContext`。B972 的“call 只证明调用、不证明写入”软教学也生效，终稿没有再把 getter 写成写操作。但真实仓候选池把 carrier 导航稀释到大量 `Mutable/BusContext` 同名文件，未选中 `BuildAgentContext`/`applyStageOutput` 等交接点；36 reads、53 Explorer iterations、15 次 midloop、5 次 completion、4 次成文拒绝。终图只有四阶段顺序和零散局部 call，两个载体仍断开，正文却泛称其“贯穿全程”，因此 runner 为表面假绿。 |
| real_trace_h3_iofam_one_seat | FAIL | fail | 引擎与转换均有精确数据：窗口内至少四条目标 issuer 的 request pair，最大 `request_residence=1.347ms`，对应 `completion_closed_issuer_blocked=1.337ms`；块设备层 `paired=85,max=1.347ms,avg=0.343ms`，另有 190 条展示帽外请求、41.329 request·ms（请求可重叠，非墙钟总量）。Explorer 也读取并在 completion aggregate 中携带这些值；最终上下文却只保留 target state、wait occurrence 和 blocked_reason，答案完全遗漏 IO 延迟/层级/非墙钟复合口径。 |

## 系统 GAP 判定

1. `B973-BOUNDEDIOLATENCYFACTFAMILY1/P1`：`runtime_question_profile.fact_families` 没有 IO 延迟语义族；Analyzer 只能用 `count_or_duration/target_scheduler_state/recorded_reason` 近似。`window_stats` 的逐请求 IO 行只存在于可读文本，没有进入 typed observation ledger；有限目标投影因此合法但错误地删掉用户主问维度。这不是 parser、转换或 `block_rq_issue↔block_rq_complete` 配对失败。
2. 修向必须保持三把尺独立：请求 issue→complete residence 是单请求墙钟区间，但不是线程等待；严格 completion→issuer wake 闭合后，issuer blocked 是目标响应墙钟；跨请求 `request·ms`/综合压力可重叠，不是可直接相加的墙钟总量。不得把三者互换或相加。
3. 最优根修：增加 schema-valid `io_latency` fact family；`window_stats` 发布 exact pair typed rows及覆盖收据；有限投影只在该 family 下开放 `io_latency/storage_latency_by_layer/block_io_by_inode`；缺失时系统只补一个 `window_stats`，绝不扩大成 root-cause rank/critical-blocking/full causal projection。
4. `B974-CARRIERNAVIGATIONRANKDILUTION1/P1`：B972 两跳规则本身未被生产否证，但第一跳目标排序被同名 participant 的宽候选池与展示 cap 稀释。下一批应在已有 parser-owned owner/member binding 下优先选择跨 owner 的 qualified argument handoff，再生成有界显示计划；仍需模型读取和发射 operation，不可系统造边。

## 红线复核

- H3 是 `bounded_fact_set`，`trace_query_final_projection_blocks=0` 正确；该问题不要求完整 Trace 因果投影。修复有限事实通道不能借机恢复根因榜。
- 完整 `causal_diagnosis`、显式窗因果补齐、链上-only 主因、邻近/背景 support-only 均不改。
- 新权限只消费 analyzer schema enum 与 trace_query typed fields，不扫描用户/模型/答案原文，不改写答案或替模型下结论。
- QF 活跃生成持续 564s，没有因 4ms 或固定累计年龄降级；结束权仍只属于 cancel/deadline、无首字节、byte-stall、transport/decode failure。

