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

## Batch F 详细任务清单(探索完成 + re-scope 裁定,2026-07-02)

### F1 — 实证结论:部分同步(absence 三路同步 ✓,waiver 未达 finalize)

waiver 只经 RuntimeGroundingDisposition 投影间接到达 contract_check,投影谓词(bundle ATTACHED ∧ ¬CurrentSourceEvidenceOrigin ∧ 观测支持>0)严格窄于接纳谓词;criterion evaluator 完全不见 waiver。分歧窗口:waived 0-citation 答案每轮 finalize 产生 ViolCitation+ViolSuccessCriterion(默认配置=错误 caveat+浪费重试;strict 提升=不可豁免重试环)。修复(4 子任务):

- T1 runtimeArtifactCitationFloorWaived(contract_check.go:626)加 typed arm `StableEvidenceFloorWaiver().IsActive()`;**不加进 ForBus 变体**(它还驱动 citation support 注水/pre-emit 污染提示/locus 容量);**不动 RuntimeGroundingDispositionFromWaiver**(20c0f3fc 的 artifact 门是深思的,动=reversal;evidence_floor_waiver.go:197-199 注释明确许可在 floor 侧放松)。
- T2 SC merge(orchestrator.go:5889-5919)仿 absence arm 加 waiver arm(CritCitationCountGE+CritContractSatisfied),复用 T1 helper 单一真源;不走 criterion.Env 插管(3+ 面且 explore/extract 停止条件不得放松)。
- T3 审计 ViolCitation→BackToExplore lane 是否计入 per-kind 预算(types/config.go:299 预算=1);有洞则补计数,**不建第二断路器**;指纹只用 RootClusterKey("CitationReq"),严禁 Violation.Detail(嵌计数必自搅动)。
- T4 pin:4 种 reason 各一(per-class)waiver retained + 0-citation → 零 ViolCitation;负控(无 waiver 门不弱化);stale-waiver 清除控;先在 HEAD 上重现分歧窗口再修。
- 注意:落地后 waived run 不再带 citation caveat——检查 eval fixture 是否 pin 了该 caveat(移除假 caveat 是正确性修复非降 bar)。

### F2 — RE-SCOPE:决策表已存在(B0 v3 274f0f76 ViolKindRegistry,69/69 kind)

原计划"建表"若照做=重建 B0 已消灭的 6-spot sync,直接 ping-pong。真活=完成 B0 明确排期但从未落地的迁移收尾:

- F2-1 StrictSeverity 字段入 registry(严格提升语义现 100% 走 legacy switch 4 条 bump 规则);删除前先 69-kind 字节等价 parity pin。
- F2-2 第 5 张手写表 retryRepairPhaseForViolation(answer_document_evaluator.go:2163)折入 registry RepairPhase 字段(**verbatim 转录 switch 值**——registry-Layer 推导对 ViolFamilyMismatch 等 kind 与 switch 分歧)。
- F2-3 AutoRepair 描述性枚举字段(none/toolparam/pre_emit/render)+ repaired-vs-retried telemetry;纯观测不 gate。
- F2-4 删 4 处 legacy 字面量+merge shim;TestRegistryDerivesAllLegacyTables 换成 golden snapshot pin(69 kind 全字段);修 repair_cooccurrence.go:232 陈旧注释。
- 已知不动:ViolSelfContradiction FinalizerOnly(Phase 1-D 裁定,BackToExplore 放大漂移);Implies vs CooccurrenceRule 双系统本批只 flag 不合并(需单独裁定)。

### F3 — RE-SCOPE:七条中三条已交付,真价值集中在 emit-time 机械修

已交付勿重做:line_range swap(emit_evidence per-item skip 语义,回批量拒=ping-pong)、citation_ref 越界修复/detach+双语 caveat 链(G6)、enum 大小写(toolparam 有但 opt-in)。2026-05-18 商用门大软化后 violation-kind 重试已稀少,真重试烧在 emit-time 工具拒绝。

- F3-1 enum 大小写归一 default-on(normalizeStringEnumAlias 去 per-property flag 门;修复条件=canonical 值 ∈ schema enum 精确信号;audit 模式 no-op 断言)。
- F3-2 数值 range clamp:**只 opt-in**(新 x-codrax-clamp-range 扩展,仅机械控制旋钮;clamp 语义字段=编造事实,红线;明确不标 declared_count/line 字段)。
- F3-3 answer document citations line/line_end swap 入 normalizeAnswerDocumentForPreEmit(纯转置断言);跨字段规则挂 normalizeKnownLocalModelToolParams(agent.go:3877)。
- F3-4 pre-emit ~20 个修复 pass 补结构化 repair 记录(镜像 toolparam.Repair)+ no-op pin + 只动 item ref 不动 Citations pool pin(保护不可软化 pool 门)。
- F3-5 AuthorityOverreach:raw-prose fallback 路径也走 authority-caveat 合成(结构化路径已兜);producer 留作纯 telemetry。
- F3-6 DeclaredCountDrift:**无生产者(dormant,B8-T4 已退役 V1 oracle)**;"自动 rescale"若改模型散文=系统代写红线违例,合规形态已存在(ViolCardinalityShort→CaveatFamilyTier2Cardinality);行动=修 2 处陈旧注释+registry 标注 dormant,关闭为 delivered-by-equivalent。
- F3-7 section_path 前缀补齐:**defer**(section grounder 是 Stage-2 占位,无真实重试源;补齐=猜测未发射段=编造风险)。

### F4 — 中央宽度 governor(6 子任务)

- T1 叶子包 internal/tool/width(tool 包 import repomap,表放 tool 包内 repomap 够不着——叶子包是 load-bearing)+ package tool façade;30+ 常量 verbatim 入表(grep 80/120/16K/48/80…,list_files 显式别名 grep 值,PathDiscovery 256 去重,read_file 100/200,trace_query 面 40/20/64/32/5,repo_map 96/4096/50/100);blob/observation_view_budgets/sourceinventory kernel/explore cap 明确 OUT(组合不吞并);头注释写四层组合(tool width→blob 传输→observation 渲染→TurnA 窗口)。
- T2 tool_width_* yaml prefix group(9 个主维度 knob,非全部;SetWidthGovernor 一次性注入仿 SetBlobLimits;非正=默认;production cap ≤ entry threshold 交叉校验)。
- T3 ToolRefinementHint.ParamNarrowingSuggestions []{Param,Priority,Suggested,ReasonCode}(ReasonCode=typed code 非本地化散文);**R2' 6 处同步强制**(type def/Normalize/Empty/merge/key/render 单投影);promoteSourceToolRefinementToRepoMap 必须翻译或丢弃跨工具建议(grep pattern 挂在 repo_map 下=误导);渲染带条目 cap 防 prompt 膨胀;硬门永不读该字段(类型注释 pin)。
- T4 producer 填充:grep path(1)→include(2)→pattern(3)→line window(4);list_files/read_file/exec/trace_query 各自 typed 事实驱动;coverage matrix 扩展断言截断必有非空建议。
- T5 repo_map roles/scopes 建议:**必须过 repoMapRuntimeCurrentSourceAvoidsSourceInventoryRefinement 权威门**(绕过=重开 2184de6c/5d176481 导航 ping-pong);scope 候选从 typed 观测计数导出。
- T6 E4 组合裁定(本 session 定):**E4 表留在 internal/tracequery**(stdlib-only,engine 内部 clamp 需要),width governor 引用 tracequery 导出常量,不搬表;原"第二个落地者搬进 width"改为"width 引用 tracequery,不复制值"。

### F5 — RE-SCOPE:registry 值全对齐(16 kind 全 SoftByDefault=true 且深思),真错位是 4 个结构洞

- F5-T1 声明式 subgate 路由表(25 call site)+ 对齐扫描测试(kind 全注册/hard lane 恰两条/go-parser 扫 body 集合相等/ForceHard 单产地);**扫描测试必须把当前 advisory 路由 pin 为期望态**——ViolCitation 载体 D1-F7w→D1-G95 已打过一轮往返,再硬化=第三轮。
- F5-T2 step-0 四个载体检查 early-return 改 append-and-continue(现状:advisory 载体提示静默跳过同 emit 的两条硬 lane);行为变化——落地前跑 cangjie member-set/diagram fixture。
- F5-T3 删死枚举 label 硬门分支(与 F8-T3 同一动作,**F8 ratchet 测试先行**防 F5 把死分支"修"成真硬拒)。
- F5-T4 未注册 kind fallback 改 return false+WARN(意外未注册必须降 advisory 而非升硬,噪音默认=红线);ForceHard 信号从 hint typed shape 导出统一查 policy rows。

### F6 — 5 项措辞修正(全部定位到行,行为零变化)

(1) defaults.go:88 REQUIRED 措辞→"显式完成信号+预算耗尽=弱交接"框架(不泄 ICPolicy/soft/strict);(2) SourceInventoryLensRoleNames() 单源 13 值 enum→repomap schema/tool 拒绝串/skill 列表+抽象示例(analysis_contract.go:426 8 值手写表同源;emit_analysis 全量 enum 面**保持宽**——请求形态载体职责不同);(3) negative_query=Go regex 系统重跑/section/struct_fields 实为全文件扫(与 scope_dispatch.go:421 TODO 交叉注释 R7)/crossfile >5=整项拒非 clamp;(4) waiver rationale 描述改"激活门控+verbatim 渲染到成文 prompt"(去"Audit-only"假话;span_waiver 先验证是否真有下游渲染再声称);(5) StageAnalyze 时 grep schema 描述投影追加 files_only=true 要求(复用运行时拒绝句词汇 SST;静态 schema 不动防误导 explore)。每项过 ATOMIC checklist + 3 个 lint 测试 + TestAnalysisSkill_PromptDocumentsFilesOnlyGuard。

### F7 — 空 blocks 保底(4 子任务)

- T1 validateMergedV2Doc 补 ≥1-block 不变量(patch 路径 remove_block_ids 全删可持久化空文档,绕过只在 full-emit 路径的不可软化门)。
- T2 拒绝时广告快照(ToolRepair.Metadata prior_draft_block_count/source);修 answerDocFullRejectPatchHint 对空 blocks 场景的错误措辞(keyed on verbatim repair code)。
- T3 同因断路器:fingerprint=repair code+快照存在位(**不含 hint 计数/递增文本**——处理器自变更值,af0a7cc5 教训);streak>3→停发 reject hint 强制 loop stop,走**既有** recoverRetryStateAnswerDocumentV2 恢复链(禁第二恢复路径);门本体永不接受空 payload。
- T4 测试:4×空 emit→breaker→快照+双语恢复注;interleave 重置;无快照→降级 lane;metadata pin;门无条件 pin。顺手:删 V1 payload_regression 死消费端(answer_document_evaluator.go:9731)。

### F8 — oracle 假阳性防护(5 子任务;**T3 ratchet 必须先/同 F5 落地**)

该面 12+ 次修正史全部朝软化方向,唯一反向=2e3da86c denial stamp(deliberate correctness promise)——本批**裁定部分回退它**:oracle 缺席是噪音信号(index 覆盖/tier/flat-form 启发),不得驱动 L1 grep 硬拒;证据侧双精确信号 AND 的 stampUngroundedEvidenceDenials 保留。

- T1 删 3 个 stamp 位(枚举 label :3760/diagram 端点 :3957(violation 永久 SOFT 却打硬 denial=公然不一致)/inline 标识符 :4106);violation warning 保留;更新 stamp-sites pin 测试并写明回退理由引用 2e3da86c 防再加。
- T2 typed 载体(json:"-" doc 字段仿 SystemGeneratedKind,避 R2')+"系统补充:枚举标签核对"双语 supplement(仿 renderReadLocalizationAuthoritySupplement);渲染时抑制同两 kind 的泛型 CaveatFamilyEnumerationDepth(噪音源头消除)。
- T3 删 vestigial preEmitEnumerationLabelGroundingHardGate(78f1eb6c 后两分支都 advisory)+ ratchet 测试 pin 恰两条硬 lane 且枚举 label 三 kind 永不可 hard。
- T4 member_set 精确子集不动(ForceHard+policy row+vouching lanes);审计 emit-loop 侧同因 breaker,缺则补(fingerprint=blockID+missing-member hash,不被修复路径自搅动)。
- T5 registry ratchet 测试(3 kind SoftByDefault pin,Promotable=true 保留 strict 逃生);修 contract_check_block.go:3221 "STRICT" 陈旧注释;SchemaDescriptionFragment "are rejected" 措辞对齐(过 prompt checklist)。
- 最终 eval 必含枚举编造 case 验证 supplement 足够醒目。

## 执行规程

每批:实现(先查历史)→ 测试看护 → go test ./... 全绿 → 对抗复核 workflow → commit/push → 本文档进展刷新。难度升级时拆细任务记录回本文档重排,不降标准。全部批次完成后:红线扫描(非测试代码零散文关键字匹配)→ 代表性 eval(分类优先级,2 并行×6/批)→ 人工读输出做架构级分析。

## 进展

- **Batch F Wave 2-4 交付(2026-07-03,3e69023d F6 / fd963e9c F7 / 84ebaec1 F2 / bf7bdf3e F3)——Batch E+F 全部完成**。F6:5 项措辞修正全落地(COMPLETION 真实后果框架/SourceInventoryLensRoleNames() 13 值单源横跨 repomap schema enum+拒绝串+skill+analyzer 契约(order pin)/negative_query 重跑与全文件扫描现实+scope_dispatch R7 交叉注释/waiver rationale 激活门控+verbatim 渲染事实(span waiver 核实无下游渲染,只写激活+日志)/StageAnalyze grep schema 投影与运行时拒绝句共享单一常量);3 个 lint 测试+guard pin 全绿。F7:validateMergedV2Doc ≥1-block 不变量(patch 全删路径曾可绕过 full-emit 门)+拒绝广告快照(typed metadata+patch 恢复 hint,repair code 提为 types 常量)+同因断路器(fingerprint=code+快照位,>3 连击强停走既有恢复链;门本体无条件 pin)+删 V1 payload_regression 死消费端。F2(re-scope 为 B0 迁移收尾):StrictSeverity(17 显式覆盖)+RepairPhase(27 显式覆盖)双 parity 字节等价证明后入 registry;4 张 legacy 表+merge shim 全删;TestRegistryDerivesAllLegacyTables 换 69-kind golden snapshot;AutoRepair 描述性 lane+policy report 列;DeclaredCountDrift 标 DORMANT+2 处陈旧注释修正。F3:enum 大小写别名 default-on(chokepoint 单源,精确 schema-member 证明;emit_analysis summary delta 测试随之更新——机械修复移到 chokepoint 是设计效果)/range clamp 严格 opt-in 机制落地(现有旋钮不标注:0=default 语义使内联标注有行为改变风险,新旋钮按需)/citations line/line_end 倒置 swap(纯转置,pool 成员 pin 不动)/normalize 链 29 个 pass 结构化 per-pass 修复计数单行 telemetry+全链 no-op pin;F3-5 实证=三条 fallback 路径已全过 authority 合成(delivered-by-existing);F3-7 defer 依探索裁定。
- **Batch G 裁定(2026-07-03)**:G1(§7.25 长唤醒链 bounded path view)与 G2(§7.30 相邻展示债务)经核实**已在 7.25 Batch1-6 与 7.30 B/C/D 三轮全部落地**(batch plan v2 撰写时 C/D 轮尚未完成,系陈旧标记);无剩余代码项。
- **红线扫描(2026-07-03)通过**:本计划全部 commit(5f37452b..bf7bdf3e)非测试代码新增字符串匹配仅 4 处,均为系统自产 verbatim 标记(trace_query 自有 caveat 格式 3 处 typed-first 的 fallback + source_inventory 记录 ID 前缀),零模型散文/用户措辞匹配;TestNoInternalTermsInPrompts / TestReviewerPrompts_LLMFacingNoInternalJargon / TestRemapStrictDecodeError_Sanitize 全绿。
- **代表性 eval 完成(2026-07-03,两批 12 case,2 并行,LLM=MiniMax-M2.7-highspeed,N=1/case)**:Batch1 5/6(trace state_churn/trace×代码边界/logtri_go/member-set 硬门/cangjie repomap PASS;donghu_real_frame_multicausal=LAUNCH_FAIL,环境性缺 ../customlogs 真机工件,已用 fixture 型 wakeup_causal_io_chain 替补 PASS);Batch2 4/6(diagram/mr_implementers/u9a/wakeup_io PASS;qf_architecture、s1a FAIL)。
- **人工审读——新机制全部在真实 run 中正向工作**:F4 收窄建议触发且用复数 scopes 参数(复核修正生效);E2 截断披露上屏("showing 24 of 128");F3-4 telemetry 单行显示一次 emit 机械修复 46 个 citation_ref+4 行枚举+3 视图兼容**零模型重试**;全部 12 case 零成文 reject/零断路器触发/零 transient retry;trace 案单次 trace_query 即答;logtri 行漂移对齐+诚实 not_enough_evidence;§7.11 typed 降级边界如实披露(微措辞观察:"trace_query 执行失败"vs"无承重行"可再精分,UX 级)。
- **两个 FAIL 架构级归因(同一类,均非本 session 回归——零重试零违规,新面未涉)**:**类=用户命名/analyzer 已确认锚点未被强制消费**。(a) qf_architecture:analyzer pre-scan task map 已点名 `internal/types/stage_binding.go`(ReadModePipelineStage),但模型把实体写歪成 `ReadModePipelineStageBin`(token 保真损失),真锚点只落在 keywords;explorer 首轮宽 source_inventory 命中 `internal/dataworkflow/stage.go` 同名 Stage* 诱饵家族后一路锚定到底,从未读 stage_binding.go/topology.go,MECHANISM WIRING 软指引(确认候选被问路径消费)未被任何 checkpoint 复述。(b) s1a:答案实质全对(9 检查顺序+不短路+白名单)但通篇用 `RunWith` 表述,用户点名的 `gate.Run`(:126 公共入口)未上答案面——用户 surface term 保真义务未绑定(analyzer 未设 exact_target,finalize 无 surface-term 复核拦截)。**per-class 方向(下一批候选,不 case-by-case 补)**:analyzer 已验证锚点(pre-scan 命中的 task-map 文件/用户点名符号)编译成 typed anchor obligation——探索 checkpoint 软复述"尚未消费的已知锚点"+完成门前 advisory 提示(精确信号:anchor ∈ readSet/evidence anchors,布尔);实体 token 保真走既有 prescan 别名纠正 lane(mangled entity 与 keywords 中的真符号前缀重合时 typed 纠正)。两案均在弱模型 N=1 下;primary model 复跑列入下批 eval 前置。
- **A1 anchor obligation lane 交付(2026-07-03,ecc3605b)**:深挖发现 `EntityProvenance`(2026-06 Batch 2 投产,注释明说"telemetry-only 等后续批次接消费端")正是本 lane 的预留生产端——零新信号,纯接消费。`CompileAnchorObligations`(ExactTargets/Resolved 实体/RequiredFiles,≤8,结构形状过滤)+精确消费布尔(path∈readSet alias-aware;symbol verbatim 上 evidence AnchorSymbol/Subject/Object,点分拼写接受尾段);消费端全软:explore prune-checkpoint 一行 advisory("if relevant…if unrelated, ignore")+接受完成 AppendCompletionGateNote(新 helper,拼接不覆盖断路器 note)。s1a 类:R3b amplifier 规则把 Resolved 用户命名符号 pin 入 MustInclude(仅非枚举单主体、非 runtime-artifact run、≤3;R3 只管枚举),走既有软 ViolMustInclude caveat lane。测试:编译/消费/checkpoint 出现与消费后消失/R3b pin+dedupe+未解析静默 四面 pin;全量绿。**遗留记录**:qualified name("gate.Run")的 oracle 解析(SymbolExistsFlat 扁平索引对 pkg.Symbol 形态可能 miss→Resolved=false→R3b 不 fire)是本 lane 覆盖面的已知边界,提升项单独跟踪;mangled-entity 别名纠正(ReadModePipelineStageBin 类)未在本批(需 prescan 侧对比 keywords 验证符号,单独小批)。
- **A1 实效复跑+A1b(2026-07-03)**:复跑两失败案 **s1a 转 PASS**(R3b MustInclude pin 生效,gate.Run 上答案面);qf_architecture 仍 FAIL——本轮 analyzer 实体质量已好(ReadModePipelineStage/StageBinding 均在)但首轮宽扫仍锚定 dataworkflow 诱饵,且短 run 从不 prune → checkpoint 提醒从未到达模型(架构级归因:提醒挂错了时机)。**A1b**:紧凑"Verified anchors"段经 BuildPromptContext 直投每个 explore dispatch 第 1 轮(plan 保留全量 prescan 段,其他 stage 维持抑制;NonPlanStages 抑制 pin 更新为新契约)。
- **Batch F Wave 1 交付(2026-07-03,5d496145/ebe836a4/26b9c3ae,对抗复核 4 lens 后修正)**。F1:实证=部分同步确认(HEAD 上重现 4 reason 全 FAIL 后修);typed waiver arm 入 runtimeArtifactCitationFloorWaived 单一 chokepoint + SC merge 同 helper;T3 审计=per-kind 预算已计 BackToExplore lane,无洞不建二断路器;**复核修正:retention 改按次声明制**——pending 槽 declared-until-retracted 跨被拒尝试存续,陈旧声明曾会被后续无 waiver 完成提升入 stable 槽并对 repo-grounded 答案解除成文 citation floor;现 RetainEvidenceFloorWaiver(declaredThisAttempt) 只在接受尝试自带声明时提升,无 waiver 完成=真收回。F4:internal/tool/width 叶子包 22 cap 单源(数值 pin)+ 9 个 tool_width_* yaml knob + ParamNarrowingSuggestions(R2' 六处同步+单渲染投影+promote 翻译 pin+权威门路由);**复核修正:多目录建议走复数 scopes 参数**(单数 scope 是单路径前缀匹配,逗号串匹配零文件)。F5:25 行声明式路由表+扫描测试(pin 现 advisory 态防 D1-F7w→G95 第三轮)+step-0 append-and-continue(行为变化:载体 advisory 不再跳硬 lane,eval 阶段验证)+未注册 kind 降 advisory。F8:3 个 oracle-miss stamp 位撤硬 denial(部分回退 2e3da86c,理由记入 pin 测试)改隔离 advisory 类;双语"系统补充:枚举标签核对"在 accept chokepoint 渲染并抑制泛型 caveat(spec 的 evaluator 链渲染点与现实不符——ParseOutput 先于 contract check,已改);ratchet pin 恰两条硬 lane;F8-T4 断路器审计=既有预算足,未加。
- v1 落盘;Batch E 开工(E1-E3 实现中)。
- **E4/E5/E6 交付(2026-07-03,724dfbe8/d203da02/29e170a6,对抗复核 4 lens 后修正)**。E4:`internal/tracequery/view_capacity.go` 单源容量表(20 view,数值字节等价 pin)+ `Result.Compactions` typed 截断记录(11 个截断点,prose caveat 原样)+ refinement 具体建议(limit=min(Total,MaxLimit) 或 LastEmittedTs 首段拆窗+next_segment;heavy view 给 fallback_view,event_search 永不给);**复核修正**:composite bundle 的 widen-vs-split 判定改读截断子 view 行(composite 行 MaxLimit=0 曾导致 limit=Total 永久 echo 环)。E5:`AnswerIntentContract.SourceInventoryAttributeDemand` 编译谓词(HasPerMemberTable OR 非身份 RequestedFields);demand→attr rank 中性化 + ≤6 挤占 cap 旁路;3fab4c14 advisory 场景字节不变(canary 未改通过)。E6:read-coverage 四写权威+门位 drain 的 path 自清(单向,fork merge 重扫)、grounded anchor symbol 自清、C2 门同因断路器(指纹=hits/uf/reads,处理器不自变更;trip→typed Advisory 降级+gate note);**复核裁定:删除轮次降龄 lane**——两个 producer 都在 analyze 期打标(MarkedRound 恒 0),轮次时钟=第 3 探索轮无条件过期反幻觉门;降级由断路器独占,MarkedRound 字段一并移除。
- v2:Batch G 挖掘完成补录(trace 文档零 open 正确性项;IR 24 条多为 eval-pending,并入最终 eval 阶段场景选择)。
- E1/E2/E3 交付(2026-07-02)。E1:`BoundTurnAToolResultsWithTruncation`(internal/types/turn_a_handoff_bounds.go)把窗口捕获/合并丢弃从 oldest-first 改为价值序(带确定性 runtime observation 的结果保底,grep 噪音先丢;双向合成序列 pin),截断记入 `TurnAArtifacts.ToolResultTruncation` 并在 transient/fact-retry checkpoint 提示显示 "tool results truncated by window budget: N dropped (tool×count)"。E2:统一常量源 `internal/types/observation_view_budgets.go`(checkpoint ledger 24→32 / extract 128 全站点单源 / 成文 render 18→32 / aggregate 16·48 同源 / trace top-5),checkpoint origins 摘要改全量 ledger 统计,观测/aggregate/checkpoint 截断一律 "(showing N of M; dropped: 类别×计数)"。E3:成文 trace coverage top-5 截断加 "(top view truncated: N more ...)" 指示。E4-E6 待做。
