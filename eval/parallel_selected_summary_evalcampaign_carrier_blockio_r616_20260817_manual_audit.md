# r616 人工审计：载体导航复放 + IO family 首次生产复放

- 基线：`main@7516b2a2e` 加 B973 未提交工作树。
- 执行：`qf_logic_view_read_pipeline` 与 `real_trace_h3_iofam_one_seat`，并发恰好 2，单次回放。
- Runner：QF PASS；H3 FAIL。
- 人工：QF major-partial；H3 fail。

| case | runner | 人工 | 关键审计 |
|---|---|---|---|
| qf_logic_view_read_pipeline | PASS | major-partial | 四阶段 precedence 与真实局部 call 均保留，BusContext/Mutable 仍缺共享载体交接。26 reads、34 Explorer iterations、13 次 midloop、11 次 Finalizer、10 次成文拒绝，说明 B974 的候选稀释/导航收敛问题仍未解决；runner 只证明固定词面存在，不能签关系拓扑完整。 |
| real_trace_h3_iofam_one_seat | FAIL | fail | B973 的 producer 已发布 `io_latency` typed rows，但 Analyzer 仍只选状态/计数族，没有选择新 `io_latency` family；有限投影依法不把未请求 family 交给 Finalizer，答案因此继续漏掉精确 request residence 与 completion-closed issuer wait。 |

## 判定

1. 新增 enum/producer 不等于生产闭环；Analyzer 需要共享 ontology 教学，明确目标调度等待、请求延迟、复合压力是三张独立卡，generic `count_or_duration` 不得替代具体 family。
2. 该教学只能影响 schema enum 选择，不能扫描用户原文或答案，也不能把有限事实问题改成 causal diagnosis。
3. QF 残余继续归 `B974-CARRIERNAVIGATIONRANKDILUTION1`，不在 IO 批里放宽关系门。
