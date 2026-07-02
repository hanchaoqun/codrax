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

## E4/E5/E6 详细任务清单(探索完成,2026-07-02)

### E4(trace_query 容量表)— 6 子任务

关键历史约束:C3(133520c1)已把 event_search 定为全预算失败的流式逃生舱,**over-cap 不得建议 event_search 换 view**(只给更窄窗口/limit);3bde10fa 教训 = cap 数值变化会把模型赶回 grep,**数值必须字节等价**(T5 value pin);O10 裁定 wakeup MaxBranches=8 只 caveat 不提升;v7 O1 trace-mark 双桶 cap(8/16)不得合并;refinement ReasonCode 字符串被 §7.11 投影覆盖面(answer_document_mutation_runtime.go:886-930)消费,只增不改。

- T1 `internal/tracequery/view_capacity.go`:ViewCapacity{View,DefaultLimit,MaxLimit,Dimension,HeavyView,RelationScoped,FallbackView,FallbackEventTypes} 表 + IsHeavyView/RelationScopedView(替换 trace_query.go:1250-1259/1187-1193 switch);替换 query.go:651/2822/5225/7577/7694/9352/9765/10511、ipc.go:111-118、stream_search.go:52-54 字面量;index 预算(250K/512MiB)不入表(C3/Gap3 阶梯所有)。
- T2 `Result.Compactions []ViewCompaction{View,Dimension,Total,Emitted,LastEmittedTs,LastEmittedLine}`:8 个截断点旁路发布 typed 记录(prose caveat 原样保留);traceQueryResultCompacted 先读 typed 再 fallback 子串——修复现状两族漏检(streamed event_search"event_search_stream_compacted=true"、tracebundle"%s_compacted total=")。
- T3 refinement 消费表:limit<MaxLimit→建议 limit=min(Total,effectiveMax)(非回显);已到 MaxLimit→给 time_start/time_end 子窗口拆分(取 LastEmittedTs,C3 copy-pastable 风格);heavy view→fallback_view 参数;建议必须严格收窄(anti-echo,防断路器指纹不变喂环)。
- T4 C3 面(traceQueryIndexLimitSummary/Refinement、heavy-guard、recipe-discovery)的 view 名/limit 40/StreamStateCluster 8 字面量改由表供给;pinned 句子不动。
- T5 测试:表值 pin(TestViewCapacityTablePinsCurrentBehavior)、coverage matrix 补三类(root_cause_rank 拆窗/scheduler_latency 建议 limit/streamed 触发 typed)、anti-echo 断言、既有 4 pin 全绿。
- T6 文档刷新。

### E5(attr 排序对齐)— 通用 typed-predicate→rank lane

历史:+30 降权来自 3fab4c14(2026-05-24,advisory-only 时代);HasPerMemberTable 2026-06-12 才落地,从未接入 rank。修复双点缺一不可:rank 中性化 + budgetSourceInventoryObservationRecords ≤6 挤占 cap 旁路,都 strictly keyed on 编译 typed 谓词(3fab4c14 场景字节不变,无 ping-pong)。

- AnswerIntentContract 加 `SourceInventoryAttributeDemand bool`(CompileAnswerIntentContract 从 HasPerMemberTable OR SourceInventoryProfile 非身份 RequestedFields 编译;不 key IntentEnumerate——纯成员枚举不要列)。非 R2' 信号(编译投影非新 schema 信号)。
- observation_ledger.go:729-733 attr 分支:demand→中性(非负 boost——等 rank 保持 member→attr 交错;负 boost 会造成孤儿列)。
- budgetSourceInventoryObservationRecords 传 intent,demand→跳过 ≤6 advisory cap(budgetObservationRecordsByOrigin 仍是总限);E2 截断披露路径保持。
- 顺手加固:observationRecordIsSourceInventoryAttribute 收紧为 `source_inventory:` 前缀+`:attr:`。
- 测试:demand=true 交错存活 pin / demand=false +30 保持 pin / budget 双向 pin / Compile pin;canary=TestProjectObservationPromptRecords_SourceInventoryDoesNotCrowdMixedOrigins 必须原样绿。

### E6(unverified findings 自清)— 5 子任务

历史:findings_validator 生命周期确认从未修过;9445f104 converger 把 unverified identity 纳入 blockerKey 且文档明说"真收缩=合法 key 变化";af0a7cc5 教训=拒绝处理器自搅动指纹。设计:清除=readSet 命中(精确布尔),降级=单调 Advisory flag(绝不移除,防 converger 弱化 ping-pong),断路器指纹只用 C2 处理器不自我变更的计数器。

- T1 UnverifiedFinding+{MarkedRound,Advisory};AppendUnverifiedFinding 盖 round 戳;dropVerifiedUnverifiedFindsLocked(path kind & readAliasLocked 命中→单向移除);DemoteOverageUnverifiedFinds(单整数比较,单调);mergeUnverifiedFindings OR Advisory/min Round;MergeFrom 尾部重扫防 fork 复活。
- T2 四个 read-coverage 写权威(SetReadSet/AddReadSet/AddReadRanges/SetReadRanges)尾部自清;emit_investigation_complete.go:1965/2755 DrainSatisfiedPendingReads 旁边补 Drain;清除不弱化 exact_resolved_defining_proof(独立门,注释+测试编码依赖)。
- T3 detectStallAndAct 内按 fingerprint 历史长度降龄(阈值 3,不得低于 converger 阈值);消费端过滤 Advisory:C2 unverifiedPaths / ExactResolutionPendingTargets / builder.go formatUnverifiedFindings 加"(advisory: aged out of blocking)"软标;ComputeDowngradeBlockerKey 哈希追加 adv 位(单调翻转=一次深思 key 变化,非 churn);**MarkedRound 不得渲染进 prompt**(内部信息红线)。
- T4 C2 门断路器(保底):fingerprint=`unverified_path_evidence hits=%d uf=%d reads=%d`(全部 C2 处理器不变更的计数器);streak>completionDenialBreakerMaxStreak→typed 降级该批 findings 至 Advisory + SetCompletionGateNote + 放行。
- T5 测试:clear-on-read(含 sub-repo alias)/单向不复活(SetReadSet 快照替换、fork MergeFrom)/降龄精确阈值+不回翻/blockerKey 对 C2 自身 RepairExpandSearch 重加字节稳定(af0a7cc5 回归 pin)/断路器 3 连击第 4 次放行+任一计数器变化重置。
- 已知边界(接受):Advisory 降级 = N 轮后 C2 反幻觉门软化成 prompt 警告+gate note(裁定:typed caveat 优于 livelock);RecordCompletionDenialStreak 单指纹跨门共享,交替拒绝最坏 (maxStreak×门数) 仍有界,不现在升级 per-lane。

## 执行规程

每批:实现(先查历史)→ 测试看护 → go test ./... 全绿 → 对抗复核 workflow → commit/push → 本文档进展刷新。难度升级时拆细任务记录回本文档重排,不降标准。全部批次完成后:红线扫描(非测试代码零散文关键字匹配)→ 代表性 eval(分类优先级,2 并行×6/批)→ 人工读输出做架构级分析。

## 进展

- v1 落盘;Batch E 开工(E1-E3 实现中)。
- v2:Batch G 挖掘完成补录(trace 文档零 open 正确性项;IR 24 条多为 eval-pending,并入最终 eval 阶段场景选择)。
- E1/E2/E3 交付(2026-07-02)。E1:`BoundTurnAToolResultsWithTruncation`(internal/types/turn_a_handoff_bounds.go)把窗口捕获/合并丢弃从 oldest-first 改为价值序(带确定性 runtime observation 的结果保底,grep 噪音先丢;双向合成序列 pin),截断记入 `TurnAArtifacts.ToolResultTruncation` 并在 transient/fact-retry checkpoint 提示显示 "tool results truncated by window budget: N dropped (tool×count)"。E2:统一常量源 `internal/types/observation_view_budgets.go`(checkpoint ledger 24→32 / extract 128 全站点单源 / 成文 render 18→32 / aggregate 16·48 同源 / trace top-5),checkpoint origins 摘要改全量 ledger 统计,观测/aggregate/checkpoint 截断一律 "(showing N of M; dropped: 类别×计数)"。E3:成文 trace coverage top-5 截断加 "(top view truncated: N more ...)" 指示。E4-E6 待做。
