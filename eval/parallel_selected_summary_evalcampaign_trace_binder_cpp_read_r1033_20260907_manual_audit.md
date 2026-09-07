# r1033 人工审计：机器 2/2 通过，不等于两份答案正确

- 基线：干净提交 `8cba22b92108`，构建于 2026-09-07 11:26:09Z。
- 严格并行 2：04:26:46 同时启动 H1 真实 Trace 与 C++ 多级继承读问答；无第三路。
- 本轮未改 oracle、预算或原始答案。机器汇总在同名前缀的非 manual_audit 文件。

| 用例 | 机器 | 人工 | 过程 |
|---|---|---|---|
| real_trace_h1_binder_true_false_attribution | PASS / 204s | 核心 IPC 结论不通过 | trace_query=5、read=0、repo_map=0、上下文54%；runner拒绝计数1实为已接受初稿后的维度软提醒，一次patch；精确数字命中不能代表归因正确 |
| sr_cpp_sink_impls | PASS / 96s | 用户核心问题通过，附加表述有错 | read=4、repo_map=2、source_lens=1、上下文28%；一次系统自铸双owner硬拒、两次patch |

## C++：关系与源码支持

结果目录 `eval/results/sr_cpp_sink_impls-20260907-042646`；日志 `run-1.logs/codrax-20260907-042647-000-55927.log`；最终 `.codrax/output/20260907-042820.599-55927.md`。

1. 四个源码全文实际入模（日志1094–1221）。最终列出ConsoleSink/FileSink/RotatingSink和各定义位置，明确 `RotatingSink → FileSink → Sink`，不把间接继承展平，也未把Sink当第四个实现。未画Mermaid，题目没有强制画图；文本箭头正确。
2. 纯虚write/name与默认flush空函数体在read中可见，但没有独立callable-body证据席；不作为B1592全链正控。只有一次完成调用，未出现后读再撤警示的路径；B1599本例未命中，不冒称生产正证。
3. **B1600 / P1**：首稿日志2363只填evidence_ids；系统2366–2367自动补引用与三条source_inventory_row_id，随后2371起以“两种精确owner不能共存”拒绝。模型并未首稿双填。修复自动补全尊重已选择的稳定证据身份，不自动删模型字段、不放松显式双owner校验。
4. 最终弱证据附注另有来源：ConsoleSink/FileSink定义在supporting_context，RotatingSink定义及继承在principal；answer_item_citation_role×2、principal_evidence.member_coverage×1（日志2217–2257、2544）存在支持域假弱化疑点。typed roster完整，不据此直接删除警示。另一次“实现类名称”维度提醒后模型原样重发，与B1600分开记P2复核。
5. 最终“仅复写write()”不准确：rotating_sink.hpp:24还覆写name()，源码注释自身也有only write说法。列模型未充分对照实现的表达问题，不扫正文强改结论、不以注释为新硬合同。

## H1：不能用命中两个数字代替归因正确性

结果目录 `eval/results/real_trace_h1_binder_true_false_attribution-20260907-042646`；日志 `run-1.logs/codrax-20260907-042647-000-55924.log`；最终 `.codrax/output/20260907-043008.616-55924.md`。

1. 用户窗口保持 `13762.791708..13763.024898`，233.190ms。Trace因果投影、目标线程状态、实际占用/规则可消双轴、链上业务span、邻近/背景支撑仍在。相应行不能并为IPC总等待；算力缺口不是实测执行时长。
2. 模型正文错误：S可中断睡眠不能排除同步Binder/IO阻塞；发送到接收的0.029–0.132ms不是请求到回复完成墙钟；容量截断时不能把1.409ms称为唯一、全部或完整总量。正文还把其余四次对端说成3+1+1。输入已有S与窄口径D/IO区分、传输与阻塞区分和容量下界指导（日志2366–2405），不能把全部错误归咎于缺提示，也不继续堆措辞硬门。
3. 15.758ms有明确帧间空闲资格，不能进入IPC总等待或主根因排名。14.302ms真实区间是 `13763.009537..13763.023839`，完整在用户窗内；它是普通sleep、非Binder waker，无pacing资格。模型将其也称“正常帧节奏空闲”是额外归因。不得误把结束时刻当开始再错误立案“跨窗”。
4. **B1601 / P2**：最终系统附注1083–1086将reduced root_evidence称为“根因证据”，包括15.758/15.565ms pacing支撑。生产者明确tier=context_only/effective=0，显示却只看旧前缀。修复只按既有typed tier/位置换中性或背景说明，不改数值、排名、资格、正文。
5. **B1602 / P1**：准确1.409ms目标自身等待行的tier=target_self_state保留，但因Rank=0不得占根因席，显示Predicate改成root_cause_context_only。下游Target Blocking Wall-Clock错误依赖旧显示Predicate而漏收，专用入模栏只有IO、缺Binder。critical twin的跨度从发送起、值从入睡起算，0.050ms口径差被正确拒绝，不能放松一致性门或猜造时长。最小修向让测量消费依赖语义tier/dimension及精确值/区间，保留无排名与容量下界。
6. `.root-causes.json`已生成139字节，schema_version=2、root_causes=[]、status=unavailable、reason_code=valid_model_root_cause_selection_unavailable。本轮没有有效模型选择，不是旁路漏写，也不能由系统从正文根因列表代填；空旁路不作为“有效根因接口结果”正证。
7. B1598等待附注新说明真实发布；本例无正值逐段D/IO roster，不冒称逐段本地化全正控。流持续生成时未因固定4ms/4m降级；未进入超长流边界，机制正控仍来自独立回归。

## 收账与后续

- B1598/B1599已分批推送，完整受影响包和types race通过，详见统一账本§123.1657。
- B1600自动owner自冲突已推送8d3583e67，真实提交/patch与原r1033归一化首稿针、race通过；B1601支撑误称根因已推送5cfa69267，实际解析入口中英22臂通过；B1602按语义tier补回精确1.409ms下界，真实Donghu生产链路与中英附注count3通过，已推送c28454879。最终生产四包全绿（tool280.616s/types34.461s/agent53.124s/orchestrator18.291s）；新代码的LLM回放待后续两路批次，不修改本轮人工未通过结论。
- C++ principal/supporting成员域对齐列P2复核；critical Binder twin起点/值不同口径保留独立精度审计，不因B1602恢复另一路便宣称已修。
- 后续轮换异构高价值用例；不固化正文关键字、不硬改模型答案、不把不完整观测升级为全量。
