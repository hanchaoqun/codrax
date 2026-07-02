# 架构稳定性批次计划 v1(2026-07-02)

来源:四路代码级审计(硬门矩阵 29+ 门/工具结果过宽/阶段间 handoff 保真/模型面矛盾+统一修复层+成文重试盘点)。v1 不含两份大文档(ir_execution_engine_stage_direction_plan_20260621 / trace 三文档)未完成项挖掘——挖掘重启后补录为 v2/Batch G。排序:trace 分析优先 → 架构稳定 → 其他。

## 原则与红线(全批适用)

- 硬门只依赖精确信号(单布尔/单整数/verbatim 子串/typed 枚举);噪音信号只作软引导;真硬门必配 typed escape + 重试超限降级保底(先例:citation floor 确定性 bypass 链 + 同因无进展 3 连击断路器,注意 low-delta 收敛器 blockerKey 自搅动的历史教训)。
- **不可软化门**(硬门审计红线):citation floor eligible 计数本体 / 成文 empty-blocks 拒绝 / citation pool 越界 / investigationCompletePolicy=override 屏障。
- per-class 不 per-shape;修复前查历史(memory + 文档"当前进展"段)防 ping-pong;严禁散文/用户措辞关键字匹配(测试除外);ZH/EN 对称;无需向后兼容。

## Batch E — trace 优先(handoff 保真 + trace_query 面)

- **E1 ToolResults 窗口合并按价值排序丢弃(CRITICAL)**。现状:合并按时间先入先出丢弃,确定性 trace_query 观测(高价值)会输给早期 grep 噪音,且静默。方案:复用 observationRecordRank 多轴评分在丢弃前重排(deterministic runtime observation 保底保留),并加"已截断 N 条(类别)"提示。历史核对:无既往修复,非 ping-pong。验收:合成序列(先 grep 后 trace_query 超窗)→ trace_query 观测存活 pin。
- **E2 三层观测 cap 对称化 + 优先级截断可见**。现状:checkpoint 24 / extract 128 / 成文 prompt 18,aggregate facts 无界;截断静默。方案:统一常量层(单一来源,建议 32/128/32 或按 token 预算),截断必显"(showing N of M, dropped: 类别×计数)";checkpoint 摘要基于全量 ledger 统计而非 24 条视图。验收:三处 cap 单源 + 截断指示 pin。
- **E3 trace 覆盖 Top-5 隐蔽截断加指示**。现状:100+ trace 观测只渲染 5 条无提示。方案:补"(另有 N 条维度未展示,完整见原始 trace_query 记录)";Top-N 常量入统一层。 
- **E4 trace_query view 容量表 + FallbackView**。现状:各 view 截断阈值隐含、refinement 分散。方案:`view_width_cap` 显式表(view→MaxEvents/MaxBytes/FallbackView),traceQueryRefinement 统一消费,超容建议带具体参数(limit 建议值/窗口拆分)。与 C3 已交付的触顶恢复参数衔接。
- **E5 SourceInventoryAttr 排序与 HasPerMemberTable 谓词对齐**。现状:per-member 表问题需要 attr 记录却被 +30 降权。方案:rank 消费典型 typed 谓词(HasPerMemberTable→attr 提权),精确信号驱动。 
- **E6 unverified findings 门自清路径**。现状:标记后无自动清除,永久阻塞该证据直至 grep 异位重发现。方案:验证成功(read_file 命中该 anchor)自动清除 + 超龄降级为 advisory;保底:同因 denial 断路器覆盖该门(fingerprint 含 unverified 集合尺寸)。历史核对:findings_validator 生命周期未修过。

## Batch F — 架构稳定(硬门/重试/修复层/引导)

- **F1 waiver×成文 ViolCitation 同步(ping-pong 高危,先历史核对)**。审计:evidence_floor_waiver/absence 豁免与 finalize 侧 contract.Check 的 ViolCitation 判定不同步→潜在不可豁免重试环(与调查阶段死锁同类)。历史:B 轮修复只覆盖调查门(emit_investigation_complete);finalize 侧 contract_check/answer evaluator 是否消费 StableEvidenceFloorWaiver 未证——**先实证再修**:若真不同步,contract.Check 读 waiver/absence typed 状态动态 relax CitationReq;加同因断路器兜底。验收:waiver 已接受 run 的成文阶段零 ViolCitation 重试 pin。
- **F2 成文重试路由统一决策表**。现状:40+ ViolationKind 的处置(BackToExplore/FinalizerOnly/reject)散落三文件手写。方案:types 层单一决策表(Kind→{Layer,Severity,AutoRepairable,处置}),三消费点改查表;为 F3 提供挂点。验收:路由行为回归 pin(现行为不变,仅收敛真源)。
- **F3 可自动修类重试消除(30-40% 非正确性重试)**。normalize 规则库扩展:enum 大小写归一/数值 range clamp+审计说明/section_path 前缀补齐/line_range swap/citation_ref 越界自动选界+说明;AuthorityOverreach 渲染层全兜(不再进 evaluator);DeclaredCountDrift 自动 rescale+系统补充说明。全部走既有 toolparam/Normalize chokepoint,修复带 telemetry。验收:每规则单测 + "修复不改答案语义"断言。
- **F4 中央宽度 governor + 参数缩小建议**。`tool/width_governor.go` 统一 cap 常数(grep/list_files/repo_map/read_file/exec/trace_query 引用单源,codrax.yaml 可覆盖);ToolRefinementHint 消费端加 ParamNarrowingSuggestions(维度优先级+建议幅度+理由),渲染进 prompt fields。repo_map 补 roles/scopes 收窄建议(现只有 cursor)。
- **F5 soft/hard 注册表对齐核查**。pre-emit 10 个 subgate 的路由与 types.ViolationKind.SoftByDefault 逐一核对,错位即修;加扫描测试锁一致性。
- **F6 模型面概念矛盾修正(审计 A 表 5 项)**:completion REQUIRED vs 弱 enforcement 表述统一;repo_map source_inventory roles 枚举+示例补进 skill;evidence scope 边界(负向 pattern 语义/crossfile 上限/scope 混用)补 schema 描述;waiver rationale 用途澄清;grep files_only 分阶段差异写明。全部过 prompt 红线 checklist。
- **F7 成文空 blocks 保底**。empty-blocks 硬拒不软化,但加草稿保全(上次有效 blocks 快照可恢复)+ 断路器覆盖。
- **F8 枚举 label oracle 假阳性防护**。enumeration label grounding 硬门(oracle 混合信号)按精确信号红线改造:oracle miss 不硬拒,降 typed 警告+系统补充;保留 member_set 结构必需场景的精确子集。

## Batch G — 文档挖掘补录(v2,挖掘完成)

**结论:代码欠账基本清零,欠的是 eval 验证。** trace 三文档:大 trace gaps 与 trace-only completion 全部已交付/裁定,方法论文档仅剩 2 条 UX 级(§7.25 长唤醒链十几跳 join 撑宽首屏→G1;§7.30 相邻展示债务→G2,均低优)。IR 计划 24 条核心条目:7 条硬门精度 + 4 条 handoff + 5 条工具过宽 + 2 条 prompt + 1 条保底,**绝大多数 code complete/focused tests ✓,状态=eval pending**(D2 商用代表批:add-proof next-action/AnalyzeRefine true path/source-inventory completion authority/interrupt-resume read-run);真 partial 仅三处:StageRunner seam(orchestrator dispatch 拆分,eval pending)、read 状态去重(partial)、loopkernel read action(advisory→soft gate 待验)。**处置:eval-pending 项并入本计划最终 eval 阶段的代表性 case 选择(覆盖上述四类场景);G1/G2 低优排 Batch F 后;三处 partial 待 eval 结果定级。**

## 执行规程

每批:实现(先查历史)→ 测试看护 → go test ./... 全绿 → 对抗复核 workflow → commit/push → 本文档进展刷新。难度升级时拆细任务记录回本文档重排,不降标准。全部批次完成后:红线扫描(非测试代码零散文关键字匹配)→ 代表性 eval(分类优先级,2 并行×6/批)→ 人工读输出做架构级分析。

## 进展

- v1 落盘;Batch E 开工(E1-E3 实现中)。
- v2:Batch G 挖掘完成补录(trace 文档零 open 正确性项;IR 24 条多为 eval-pending,并入最终 eval 阶段场景选择)。
- E1/E2/E3 交付(2026-07-02)。E1:`BoundTurnAToolResultsWithTruncation`(internal/types/turn_a_handoff_bounds.go)把窗口捕获/合并丢弃从 oldest-first 改为价值序(带确定性 runtime observation 的结果保底,grep 噪音先丢;双向合成序列 pin),截断记入 `TurnAArtifacts.ToolResultTruncation` 并在 transient/fact-retry checkpoint 提示显示 "tool results truncated by window budget: N dropped (tool×count)"。E2:统一常量源 `internal/types/observation_view_budgets.go`(checkpoint ledger 24→32 / extract 128 全站点单源 / 成文 render 18→32 / aggregate 16·48 同源 / trace top-5),checkpoint origins 摘要改全量 ledger 统计,观测/aggregate/checkpoint 截断一律 "(showing N of M; dropped: 类别×计数)"。E3:成文 trace coverage top-5 截断加 "(top view truncated: N more ...)" 指示。E4-E6 待做。
