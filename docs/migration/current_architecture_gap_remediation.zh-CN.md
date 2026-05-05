# 当前架构缺口整改需求（中文版）

状态：待持续整改  
代码基线：`origin/main@4b83d1fcb361bfd8a2ee81b5ff9892550ef75570`  
工作区：`C:\Users\ssccv\codrax`

---

## 1. 文档目的

本文档只描述**当前代码仍存在的架构缺口**、根因、职责边界和可执行整改要求。

本文档不记录历史修复过程，也不区分“哪一轮修了什么”。  
它只回答四个问题：

1. 当前主链还有哪些真实缺口。
2. 这些缺口的根因是什么。
3. 每一层应该承担什么职责，不能再把问题甩给下游 LLM 自由发挥。
4. 下一步应如何按文件、按函数、按数据结构继续收口。

---

## 2. 当前总体判断

当前系统的主干已经正确：

- 分类轴：`QuestionFamily`
- 运行时语义轴：`AnswerSemanticView`
- 最终答案载体：`AnswerDocumentV2`
- 证据语义轴：`ClaimForm` / `FacetCoverageContract` / `RenderedClaimUse`
- 最终答案写入：`emit_answer_document` / `emit_answer_document_patch`
- 最终答案校验：`contract_check_block`

也就是说，系统已经**不是**“缺 block-only carrier”或“缺 semantic contract”的阶段。

当前剩余问题已经切换为四条更深的主线：

1. repair 已经具备 cluster closure，但 cluster 身份仍未完全摆脱 `Violation.Detail` 文本形状；
2. diagram relation 已 typed-first，但 relation 真值仍保留 label fallback；
3. richness / completeness 已经有 typed validator 和 reviewer，但还没有统一成一个单一质量闭环；
4. full emit / patch emit 已共享 mutation runtime，但外部协议、retry 心智和字段演进仍然是双入口维护面；
5. 模型可见提示的大头已经正确，但 helper 级提示和文档仍残留少量实现层术语，会继续误导模型和后续开发。

一句话总结：

> 当前系统的底座已经正确，剩下的核心问题不是“没有机制”，而是“机制之间还没有完全编排成一个稳定、低回归、且不压薄答案的整体”。

---

## 3. 职责边界（必须统一）

### 3.1 `QuestionFamily`

代码锚点：

- [C:/Users/ssccv/codrax/internal/types/facet_plan.go](C:/Users/ssccv/codrax/internal/types/facet_plan.go)

职责：

- 只负责问题族分类。
- 只决定进入哪套 family compile 模板。

禁止：

- 不允许把 `QuestionFamily` 扩成另一个大而全的运行时决策枚举。
- 不允许再回退到 shape 式“题型 = 载体形状”的心智模型。

### 3.2 `AnswerSemanticView`

代码锚点：

- [C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go](C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go)
- [C:/Users/ssccv/codrax/internal/types/answer_semantic_view_compile.go](C:/Users/ssccv/codrax/internal/types/answer_semantic_view_compile.go)
- family-specific compilers under `internal/types/answer_semantic_view_compile_*.go`

职责：

- 编译 `RequiredBlocks`、`FacetCoverage`、`DiagramPlan`、`RichnessCandidates`、`ExactResolution`、`UncertaintyRules`。
- 成为 read-mode runtime 的唯一语义真理源。

禁止：

- 不允许 read-mode 新逻辑绕开 `AnswerSemanticView` 再发明一套平行 contract。
- 不允许把 rich answer 期望散落到 prompt 文案、repair hint 和 reviewer 中各说各话。

### 3.3 `AnswerDocumentV2`

代码锚点：

- [C:/Users/ssccv/codrax/internal/types/answer_document_v2.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2.go)

职责：

- 作为最终答案唯一 carrier。
- 承载 `blocks[]`、`claim_uses[]`、`edge_anchors[]`、`citations[]`、`exact_resolution`。

禁止：

- 不允许重新引入任何 V1 顶层 payload。
- 不允许让模型可见契约和 V2 schema 出现双轨心智。

### 3.4 mutation runtime（full emit / patch emit）

代码锚点：

- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go)
- [C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go](C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go)
- [C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go)

职责：

- full emit 和 patch emit 只是不同 mutation 入口。
- merged-doc 校验、持久化、telemetry、retry state 必须共享同一套运行时。

禁止：

- 不允许两条入口各自维护独立字段解释、保留规则或 repair 语义。
- 不允许“主路径支持了新字段，patch/retry 路径静默漏掉”。

### 3.5 `contract_check_block` / reviewers

代码锚点：

- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go](C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go)

职责：

- `contract_check_block`：负责硬合同和 typed coverage。
- reviewer：负责“没胡说但偏薄”的第二层质量判断。

禁止：

- 不允许 reviewer 替代主合同。
- 不允许 richness / completeness 全部降级成 advisory。
- 不允许 diagram 只检查“有图”，不检查“图边的关系真值”。

### 3.6 repair orchestration

代码锚点：

- [C:/Users/ssccv/codrax/internal/orchestrator/repair_plan.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_plan.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go)

职责：

- 根据 typed violations 编译 root-cause clusters。
- 决定下一步修哪个 owner。
- 避免把 finalize-local 问题无谓拖回 extract/explore。

禁止：

- 不允许 cluster 身份长期依赖 `Detail` 文本片段稳定。
- 不允许 owner 推进只看“某些 violation 种类没了”，而不看 cluster 是否真正闭环。

---

## 4. 当前现存问题与根因

### 4.1 问题 A：repair cluster 已有 closure，但 cluster identity 仍未完全结构化

优先级：`P1`

代码锚点：

- [C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go)
- [C:/Users/ssccv/codrax/internal/types/violation.go](C:/Users/ssccv/codrax/internal/types/violation.go)

关键函数：

- `clusterFingerprintOf(...)`
- `computeClusterClosure(...)`
- `ClusterStateForCluster(...)`

当前现象：

- 系统已经有 `RepairClusterExecutionState`。
- 已经能区分 `PrimaryResolved / DerivedResolved / StableAttempts`。
- 已经不是旧式“单 owner + 全局 kind-set strict subset”。

但 cluster identity 仍部分依赖：

- `block id="<X>"` 这类 detail 片段
- `facet "<X>"` 这类 detail 片段
- `relation kind=<X>` 这类 detail 片段
- `Detail` 前缀 hash fallback

根因：

- cluster identity 仍然是“typed signal + detail extraction”的混合体。
- `SuspectedRoot.IRField`、`EvidenceRefs` 已经接入 fingerprint，但仍不是 producer-side 的显式 cluster id。
- violation producer 没有统一产出“我属于哪个 repair cluster”的结构化字段。

风险：

- 只要某类 violation 的 `Detail` wording 漂移，closure/rebuild 行为仍可能抖动。
- 系统虽然不再是“单 owner 粗暴回退”，但还没达到“完全由 typed cluster id 驱动”的终态。

整改要求：

1. 为 `types.Violation` 增加显式 `ClusterKey` 或等价 typed 字段。
2. violation producer 在生成 violation 时就写明 cluster identity，不允许 closure 再从 `Detail` 反解析。
3. `clusterFingerprintOf(...)` 最终应退化为 migration compatibility path，而不是长期 authoritative path。
4. `repair_execution_plan.go` 的 owner 推进逻辑必须以 cluster-state 真实闭环为准，而不是以“fresh violations 缩小了”作为代理。

---

### 4.2 问题 B：diagram relation 已 typed-first，但 relation 真值仍保留 label fallback

优先级：`P1`

代码锚点：

- [C:/Users/ssccv/codrax/internal/types/diagram_relation.go](C:/Users/ssccv/codrax/internal/types/diagram_relation.go)
- [C:/Users/ssccv/codrax/internal/types/answer_document_v2.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go)
- [C:/Users/ssccv/codrax/internal/skill/diagram_relation_doc.go](C:/Users/ssccv/codrax/internal/skill/diagram_relation_doc.go)

关键函数 / 字段：

- `InferRelationFromLabel(...)`
- `AnswerBlock.EdgeAnchors`
- `DiagramEdgeAnchor.RelationKind`

当前现象：

- `edge_anchors[]` 已支持 `relation_kind`。
- validator 已优先消费 typed relation。
- schema 和说明都已经明确：`relation_kind` 是首选 authoritative surface。

但只要 `relation_kind` 缺失：

- 系统仍会回退到 `InferRelationFromLabel(...)`
- relation authority 仍会部分绑在 Mermaid label 词汇上

根因：

- relation 真值还不是完全 typed；label vocabulary 仍承担兜底识别职责。
- diagram edge 的“存在”与“这条边到底代表什么关系”仍然没有被完全拆开。

风险：

- 图的端点 grounded 了，边的 `claim_form` 也合法，但 relation 真值仍可能受 wording 影响。
- 用户看到的是“图没错”，实际系统接受的是“词汇看起来像对”。

整改要求：

1. 凡是 load-bearing edge，最终都必须显式携带 `relation_kind`。
2. `InferRelationFromLabel(...)` 只允许保留在兼容窗口内，不能作为长期 authoritative path。
3. `contract_check_block` 需要把“relation 只靠 label 成立”的情况从 advisory 逐步升级到 migration warning，再到 default reject。
4. `diagram_relation_doc.go` 和 V2 tool schema 必须把 typed relation 声明成默认路径，而不是“更推荐的写法”。

---

### 4.3 问题 C：richness / completeness 仍是“多层拼接”，不是单一质量闭环

优先级：`P1`

代码锚点：

- [C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go](C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go)
- [C:/Users/ssccv/codrax/internal/types/facet_plan.go](C:/Users/ssccv/codrax/internal/types/facet_plan.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go](C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go)

关键函数：

- `validateRichnessRegression(...)`
- `validateRichnessGlaringGap(...)`
- `SemanticQualityReviewer.Review(...)`

当前现象：

- typed validators 已经存在：
  - `ViolRichnessRegression`
  - `ViolRichnessGlaringGap`
  - `ViolPrincipalProseUnderfilled`
- reviewer 也已接线，能发现“答案没有胡说，但偏薄”。

但系统仍把 richness 拆散在多层：

- hard facet coverage
- principal prose underfill
- optional richness gap
- reviewer 的二级判断

根因：

- richness 还是“多路补丁式质量层”，不是统一 `RichnessContract`。
- `AnswerSemanticView` 有 `RichnessCandidates`，但没有完整表达：
  - 哪些 rich facet 只是可选
  - 哪些在证据充分时应强烈要求
  - 哪些缺失后必须 retry，而不能只记 telemetry

风险：

- 系统不太会完全没答，但仍可能在“没胡说”和“答案真正有用”之间留下灰区。
- 会继续出现“轮次少了，但答案被压薄”的现象。

整改要求：

1. 为 `AnswerSemanticView` 增加统一 `RichnessContract`。
2. 显式区分：
   - `must_render`
   - `should_render_when_evidence_sufficient`
   - `telemetry_only`
3. `contract_check_block` 负责 richness 的主判断；reviewer 只补主合同看不到的“薄而不假”问题。
4. 不允许 richness 永久停留在“软提醒 + reviewer prose”模式。

---

### 4.4 问题 D：full emit / patch emit 仍然是双入口协议

优先级：`P2`

代码锚点：

- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go)
- [C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go](C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go)
- [C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go)
- [C:/Users/ssccv/codrax/internal/types/retry_state.go](C:/Users/ssccv/codrax/internal/types/retry_state.go)

关键函数：

- `ApplyAndPersistMutation(...)`
- `ApplyAnswerDocumentV2Patch(...)`
- `convertEmitBlocksToTyped(...)`

当前现象：

- 运行时 merge/apply 已共享。
- block normalize 和 merged-doc invariants 也已经共享。

但对模型和 retry 系统可见的外部协议仍有两种：

- full emit
- patch emit

而且仍要同时维护：

- first emit 心智
- patch preserve 规则
- retained draft / retry summary

根因：

- 语义运行时统一了，但模型可见写入协议还没有统一成一个 mutation worldview。
- tool surface 仍让人感觉“这是两套不同的提交方式”，而不是“一种文档写入协议的两种 mutation 形态”。

风险：

- 未来 block 字段演进时，最容易出现“主路径已支持、patch/retry path 静默丢字段”的回归。

整改要求：

1. 继续向“统一 mutation worldview”收敛。
2. full emit 与 patch emit 必须共享同一份字段说明、同一份 preserve 语义、同一份 repair 语言。
3. retry hint 里对“整份 payload 原样贴回”的依赖要继续削弱，优先鼓励 block-delta 修复。

---

### 4.5 问题 E：helper prompt 与文档仍残留少量实现层术语

优先级：`P3`

代码锚点：

- [C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go](C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go)
- [C:/Users/ssccv/codrax/internal/skill/defaults.go](C:/Users/ssccv/codrax/internal/skill/defaults.go)
- [C:/Users/ssccv/codrax/docs/architecture.md](C:/Users/ssccv/codrax/docs/architecture.md)

当前现象：

- 主 V2 skill 已经大体 clean。
- 但 helper 级提示和主文档中仍残留少量实现侧说法，例如：
  - `backbone`
  - `spine`
  - 过度强调“deterministic pipeline resolved ...”之类实现视角

根因：

- 主 skill 和动态 evaluator 是两套提示源，helper-level 字符串更容易长期漂移。
- 文档仍部分从“系统怎么实现”出发，而不是从“模型需要知道什么公开 contract”出发。

风险：

- 不一定直接致错，但会持续把模型往“内部实现结构”而不是“公开 contract”上拉。
- 会误导后续开发继续在 helper prompt 里塞实现层术语。

整改要求：

1. helper 级提示只允许出现 block / facet / diagram / relation / coverage / uncertainty 这些公开语义词汇。
2. 清理 `backbone` / `spine` 之类内部 scaffold 术语。
3. `docs/architecture.md` 与运行时主链保持同一心智模型，不再保留旧世界叙述。

---

## 5. 可执行施工计划（按阶段推进）

### Phase 1：先把 repair cluster identity 完全 typed 化

目标：

- closure 不再依赖 `Detail` wording。

涉及文件：

- [C:/Users/ssccv/codrax/internal/types/violation.go](C:/Users/ssccv/codrax/internal/types/violation.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_cluster_closure.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go](C:/Users/ssccv/codrax/internal/orchestrator/repair_execution_plan.go)
- 所有 violation producer

具体动作：

1. 为 `Violation` 增加显式 `ClusterKey`。
2. cluster producer 直接写入 `ClusterKey`。
3. `clusterFingerprintOf(...)` 优先读 `ClusterKey`；旧 Detail 解析只保留 migration fallback。
4. `computeClusterClosure(...)` 改成只按 typed cluster identity 对比，不再以 prose 片段为主。

为什么先做：

- 这是 repair 路由稳定性的前提。
- 不先做这一步，后面的 reviewer / richness / diagram 都可能因为错误 repair owner 被放大噪音。

### Phase 2：把 diagram relation authority 完全 typed 化

目标：

- relation 真值不再依赖 label fallback 作为主路径。

涉及文件：

- [C:/Users/ssccv/codrax/internal/types/diagram_relation.go](C:/Users/ssccv/codrax/internal/types/diagram_relation.go)
- [C:/Users/ssccv/codrax/internal/types/answer_document_v2.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go)
- [C:/Users/ssccv/codrax/internal/skill/diagram_relation_doc.go](C:/Users/ssccv/codrax/internal/skill/diagram_relation_doc.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go)

具体动作：

1. 对所有 load-bearing edge，要求显式 `relation_kind`。
2. `InferRelationFromLabel(...)` 退到 compatibility mode。
3. validator 把“仅靠 label 满足 relation”的情况先升为 migration warning，再逐步升级为默认 reject。
4. prompt/schema/examples 全部改成 typed relation 为默认写法。

为什么第二步做：

- diagram 是用户可见质量的重要部分。
- 不先把 relation authority typed 化，richness 做得再多也可能只是把“看起来更丰富但关系仍然不够真”的图包装得更漂亮。

### Phase 3：统一 richness contract

目标：

- 让“答案足够完整、足够丰富”从多层 advisory 变成统一 contract。

涉及文件：

- [C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go](C:/Users/ssccv/codrax/internal/types/answer_semantic_view.go)
- [C:/Users/ssccv/codrax/internal/types/facet_plan.go](C:/Users/ssccv/codrax/internal/types/facet_plan.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check_block.go)
- [C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go](C:/Users/ssccv/codrax/internal/orchestrator/semantic_quality_reviewer.go)

具体动作：

1. 在 `AnswerSemanticView` 上新增统一 `RichnessContract`。
2. 把当前散落的 `RichnessCandidates`、`ViolRichnessRegression`、`ViolRichnessGlaringGap`、`ViolPrincipalProseUnderfilled` 收敛到统一 contract。
3. reviewer 只处理主合同看不到的剩余“偏薄”问题，不再承担 richness 主判定。

为什么第三步做：

- 你最关心的“答案不能为了少轮次而压薄”本质上就是这里。
- 不把 richness 收成一级合同，系统永远会在“没胡说”和“够不够有用”之间摇摆。

### Phase 4：继续压缩 full emit / patch emit 的双维护面

目标：

- 把两条写入入口继续收敛成同一 mutation worldview。

涉及文件：

- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_v2.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document_patch.go)
- [C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go](C:/Users/ssccv/codrax/internal/tool/answer_document_mutation_runtime.go)
- [C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go](C:/Users/ssccv/codrax/internal/types/answer_document_v2_patch.go)
- [C:/Users/ssccv/codrax/internal/types/retry_state.go](C:/Users/ssccv/codrax/internal/types/retry_state.go)

具体动作：

1. 保证 full emit / patch emit 共用同一份 block schema 解释。
2. retry summary / retained draft / preserve rule 统一由 mutation runtime 生成。
3. 新增字段时，必须有“full path + patch path + retry path”三位一体测试。

为什么第四步做：

- 这不是当前最致命的 correctness 问题，但它是未来最容易引入静默回归的地方。

### Phase 5：清理 helper prompt 和文档术语

目标：

- 让模型与开发者都只看到公开 contract，而不是内部实现 scaffold。

涉及文件：

- [C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go](C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go)
- [C:/Users/ssccv/codrax/internal/skill/defaults.go](C:/Users/ssccv/codrax/internal/skill/defaults.go)
- [C:/Users/ssccv/codrax/docs/architecture.md](C:/Users/ssccv/codrax/docs/architecture.md)

具体动作：

1. 移除 `backbone` / `spine` / 过强实现视角术语。
2. helper prompt 只保留 block / facet / diagram / relation / uncertainty / coverage 词汇。
3. 文档中不再从 V1 / shape / 实现历史角度解释当前主链。

为什么最后做：

- 它不是最先影响 correctness 的问题，但会直接决定后续开发是否继续引入心智漂移。

---

## 6. 审计要求

后续所有整改必须满足：

1. Prompt 中不得出现退休字段、内部 Go 结构体名、模型无法理解的内部术语。
2. 所有新字段必须端到端有 producer、有 consumer、有测试。
3. 不允许 gate 之间相互打架。
4. 不允许为单题或单仓写关键词补丁。
5. 不允许引入新的双协议、双真理源、双语义层。
6. 不允许为了减少轮次牺牲答案丰富度。
7. 不允许留下死代码、静默 fallback、只在注释里存在的“假机制”。
8. 任何兼容 fallback 都必须有退出计划，不能长期留在主路径。
9. diagram relation、repair cluster、richness contract 这三条线必须优先 typed 化，不能继续依赖 prose 或词汇侧推断。

---

## 7. 最终目标

最终系统应达到：

- repair 按 typed cluster closure 推进，而不是按 violation 文本或 kind 集合近似推进；
- diagram relation 的 authority 完全来自 typed edge contract，而不是 surface wording；
- richness / completeness 成为统一的 surface-quality contract；
- full emit / patch emit 共享单一 mutation worldview；
- LLM-facing contract 与 runtime schema 始终一一对应、零漂移。

这才算真正完成当前架构的收口。
