# 6a1c 冻结双例完整人工审计

- date: 2026-09-23T01:24:57Z
- sweep_start_ts: 20260922-182445
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_identity_sequence_budget_20260922

同一构建 `6a1c4625d453-dirty`（dirty 仅文档），两个案例并行且各运行一次。机器 1/2，完整人工 0/2。已读完整 primary/principal/answer-transcript、工具日志、原始材料/代码和最终旁路；自动正则不代替语义验收，不修改旧 oracle 或回写旧 FAIL。随后 `d0d9c24e9` 仅迁移旧测试输入，不改变本次模型使用的生产代码。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | FAIL | eval/results/hmc_identity_sequence_budget_20260922/real_trace_e2_cross_trace_asymmetry-20260922-182457 | log_regex,answer_regex,answer_contains | none | 150s | 38 | read=6,repo_map=0,list=0,trace=14,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 时长正确；无据断言不同时间基准、存在信号被扩大为完整采样，内部枚举泄漏；ReadFile 额外空分段造成系统行数 +1 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/hmc_identity_sequence_budget_20260922/qf_sequence_analyzer_gate-20260922-182457 | answer_regex,answer_contains | none | 362s | 40 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 汇聚方向正确；缺身份被系统导向新增，导致已有调用重复；独立入口/条件阶段未区分，关键函数列表放在图前 |

## 1. 两份 Trace 的范围与同钟判断

实际首末事件分别为 34579.450627–34579.595184 秒和 2942.244845–2942.245401 秒，即 144.557/0.556ms；正文取约 144.6/0.56ms 正确。长材料有 CPU 频率和 VSync 相关记录、短材料同类查询为空的有限判断也有依据。窄 VSync patterns 为 59，加入 Choreographer 后为 60 条混合文本事件；不能把 60 条当脉冲数，也不能拿旧 PROFILE 的 59 反判原生查询错误。

人工失败理由与机器正则失败独立：缺共享时钟/设备锚点只能说明不能证明可直接对齐，不能据 31000 多秒数值差断言“绝对时间戳不属于同一基准”或所有跨文件比较均无物理依据。日志 `run-1.logs.all.log:2929–2934` 的最终边界明确提醒未知不能推同/异；正文仍越界。CPU/VSync 样本存在不等于“完整 CPU 调频采样/完整 VSync 帧渲染链路”。正文还暴露 `typed cross-artifact` 和 `shared_clock_origin=unproven` 等控制术语。该次完整上下文足以避免这些误述，不证明纯模型波动，不追加原文关键词硬门。

确定性系统行域缺口：原文件实际 15623/100 行，TraceQuery 一致；ReadFile 用末尾 LF 的额外 split 空元素产生 15624/101，并写进可见 banner/Enumeration Authority，模型沿用。应统一物理行与分页/引用坐标，保真实空行、无尾 LF 行及空文件，不能改两个数字糊过案例。归统一账本 §146，共享材料行域子任务仍未实施。

本次只保留两个真实采集的一对，身份正控通过；没有出现伪 scope/重绑 blob，所以不能签身份修复全部负控的 live 验收。进程 exit0、案例 148 秒（外层150），不是超时。最终 Markdown/HTML 存在，根因 JSON schema2 空数组与 `trace_root_cause_contract_not_active` 符合有限事实题，不要求因果图。系统附录正确不抵消正文错误。旧 PROFILE 的异时钟推断另记 HMC-18 维护债，原机器结果保持。

## 2. 源码时序、已有消息与修补权限

已核实际代码：`internal/agent/analyzer.go:2745` 是 buildAnalysisIR→gate.RunWith；`internal/analysis/gate/gate.go:135` 是 gate.Run→RunWith。正文及两条箭头方向正确，没有造 RunWith→Run 或 helper 互调。19 个关键函数和源码位置大体有据，不要求穷举 235 个调用；但题目明确要求图后列表，终稿列表在图前。两个独立入口排进同一 sequence，条件式 InferScenario/反事实扩展未标条件，读者会误认一次连续、必经执行。

完整初稿仅一条 `GR->>RW: 调用质量门`，随后系统 participant 校验要求补该已存在调用的精确身份，发布 `addition_ref` 而没有已有消息重绑定权限。日志 3631 的 joint-delta 提示明确选择 `action:add` 并保留未列出的原边；3649 模型照此选择 addition_ref 与新的 placement_ref，系统接受后变两条完全相同的 GR→RW。已有边识别只看精确 typed tuple，初稿 anchor 缺 FromIdentity/ToIdentity，所以识别失败。这是修补能力发布接缝，不是位置执行器擅改顺序或模型初稿重复，也不能以字符串去重修复（合法重复消息必须保留）。

新 placement_ref 在真实调用中命中，系统按模型所选 gap 插入并保留原稿；证明位置执行路径可用，不代表元数据补全协议或全图关系已闭环。深审优先方案是统一普通调用边门与 participant 门的只读严格源码证明：前者已有 qualified owner/定义/唯一性桥，后者却只比原始 tuple。参与者覆盖与候选已绘制判断复用同一 requestedRelationEvidence 内的精确来源/端点/消息绑定，可避免已证已有消息误入新增修补。确实需要模型补绑定才扩显式 attach；现有 attach 仅支持没有 prior anchor 的 body-only 分支，不能直接覆盖本例“不完整 prior anchor”。需歧义、合法重复消息、同actor异方法、跨块/旧引用负控，禁止 addition_ref 偷选已有消息；详见统一账本 §146，当前仅设计未实施。

本例上下文包含完整函数体和两个精确入口，约 40% 占用（最高80164/200000 tokens）；7次读文件、14轮explorer，存在重复整段读取与可选成文提示因构建预算未投递，分别保留效率/覆盖边界，不归为缺少两个调用证据。两个调查完成均无阻塞，未真实触发 stage-only 参数候选负控。第二次可选修补又请求未发布的 `field=facet_ids`，被合法拒绝并保留先前接受答案；不取消数组 schema 或任意放宽字段补丁。

案例360秒（外层362），真实请求日志为 first_byte_timeout=10m、stall_timeout=5m、timeout=10m，流式语义活跃时继续运行，没有按4ms/4分钟总耗时降级。没有真实等满600秒，不冒称十分钟端到端超时已回放；该边界由定向时钟/取消测试验证。

## 3. 结账边界与下一 ROI

本批 §142–145 四类实现分别验身份来源、参数义务、局部插入坐标和分类预算，公共红绿/race、全仓与提交收据另记主账本。两例完整答案仍 FAIL，66父开放项不减，旧人工FAIL不改签。后续先已有图消息的元数据补全协议和统一材料物理行域，再原生断言补绑定/执行代次、caller语义双轴、业务实例/旁路来源与容量及其它领域能力；不追本原题第三例求绿，不用模型输出扫描硬门。
