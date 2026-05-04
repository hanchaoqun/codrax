# V2 Runtime 收口 — Eval 残留 outlier 深层次根因分析 (2026-05-04)

**触发**: `docs/migration/v2_runtime_consolidation_eval_20260504.md` 6/6 PASS,但 2 个 outlier 模式需深挖根因(用户红线:不写局部补丁,要泛化方案)。

---

## B. u3a run-1 outlier — `citation_ref-inside-claim_use` LLM 误判

### 症状

LLM 在 u3a run-1 finalize 阶段连续 7 iter 同一 schema 错(`unknown field "citation_ref"` 在 claim_use 内)。最终 retry 回收,但 finalizer_iters=10 是 outlier。

### 浅层补丁(不采)

在 schema description 加 "禁止把 citation_ref 放进 claim_use" 警告。已在历史多处加过,LLM 仍犯 — 这是症状级修复,根因未解。

### 深层次根因(三层)

#### 根因 1:JSON Schema 信息密度悖论

schema description 字段越详细(列出 from_node / to_node / facet_id / evidence_id / claim_form / surface_role 6 个 inner field),LLM 越倾向于"既然这里能放这么多,那 citation_ref 也能"。Phase 3-C3 加了 from_node/to_node 后,inner field 列表从 4 → 6,误判概率上升。

**精确信号**:
- m1a 日志中 from_node/to_node mentions 2-3 次/run = LLM 主动尝试新字段
- u3a-1 报错 citation_ref 在 claim_use 也是同一行为模式 — 看到长 inner field 列表,联想"citation 也是 inner",填错位

#### 根因 2:strict-decode 错误信息 LLM-不友好

Go `json.Decoder.DisallowUnknownFields()` 报 `unknown field "X"`,只指字段名,不指位置。LLM 看到错误就尝试"加/删 X",但不知道 X 该放哪。citation_ref 是 schema 中合法字段(items[i].citation_ref),但放错容器 — LLM 看到 "unknown field citation_ref" 就以为 citation_ref 整个字段 wrong,继续乱试。

#### 根因 3:LLM 的 schema 推理是结构性 + 关联性,非位置性

LLM 把 claim_use object 当成"装载 evidence 元数据的 bag",citation_ref 是 evidence 元数据 → 联想填进去。这是 schema-design 层面问题:claim_use object 名字暗示它"管 claim 的所有引用",而 citation_ref 在 schema 上是"item 直系字段"。**命名 vs 结构不匹配**。

### 一类问题泛化

任何 nested object 子字段 ≥ 4 时,LLM 误把 sibling field(同层但不同 parent 的字段)填进 nested object 概率显著上升。当前已知至少 3 处:

- `claim_use` ← `citation_ref`(本次)
- `claim_use` ← `kind`(早期 forensic)
- `value` ← `key`(measurement-scalar 早期混淆)

### 泛化方案(非局部补丁,后续 session 实施)

1. **"forbidden field list" schema 字段**:在每个 nested object 的 description 末尾加 `(forbidden: <field-list>)` 段,列出常被误填的 sibling 字段及其正确容器。例 `claim_use`: `(forbidden: citation_ref → goes on items[i].citation_ref / value.citation_ref / boolean.citation_ref)`。

2. **错误消息增强**:`json.Decoder` 错误捕获时,如果是 known-but-misplaced 字段(grep 整 schema 找该字段名其他出现处),错误改写为 `field "X" exists at <correct path>, not here`。这是 schema-aware 错误增强,适用所有 emit_*。

3. **schema linter / validator-time 提示**:在 emit reject 时如果检测到 known-misplaced field,主动 inject `RepairDirective{Kind: SwapFieldLocation, ...}` 进 retry hint,让下次 retry 看到具体位置指引。

### 推荐落地顺序

方案 2(错误消息增强)ROI 最高 — 一处改动覆盖所有 emit_* 工具的所有 misplaced field 案例。方案 1 是 prompt-side 防御,方案 3 是 hint-side 增强,都适用但增量。

---

## C. m1a-2 + u3a-1 过早 `fail_loud` — RepairPlan cluster 算法

### 症状

cluster=4 violation(richness_regression + facet_uncovered + authority_overreach + diagram_edge_unsupported / + block_coverage_missing + enumeration_label_ungrounded)→ target=fail_loud。但最终都 PASS — 即 fail_loud 后 retry 真的修了。

### 浅层补丁(不采)

调高 EscalateAfterN 阈值。这只是把数字 +1,不解决 cluster 算法层问题。

### 深层次根因(两层)

#### 根因 1:cluster=4 假设"每 cluster 1 owner",4 cluster 4 owner 自动 escalate

A1 算法:`PrimaryOwner = 所有 cluster Owner 中 depth 最深的(因为不能 finalizer-only 修 explore-locus)`。4 cluster 时几乎必有 explore-locus + extract-locus 混合,deepest = explore = 走 back_to_explore;而 EscalateAfterN 检测"上一轮 PrimaryOwner = 本轮"就 escalate。

**问题**:cluster 数多 ≠ 真正深层故障;可能是 finalizer 一次 emit 同时 hit 多个 typed validator 的 normal case(因为 V2 carrier validator 数量 Phase 3+5 后大幅增加)。

**精确信号**:m1a-2 + u3a-1 都 PASS — 实际 finalizer-internal 一轮 retry 就修了,不需 escalate。但 cluster 算法看 4 cluster 直接判 deepest depth + escalate threshold met。

#### 根因 2:cluster owner 与 validator 添加节奏不同步

Session 2 加了 ViolDiagramEdgeUnsupported (Phase 3-C5) + ViolFacetUncovered 重接 (Phase 5-E1) — 但 `defaultCooccurrenceRules` 7 条 + cluster owner mapping 没同步增加。新 violation 走 singleton fallback,各成 1 cluster,自动膨胀 cluster count。

### 一类问题泛化

每次新 typed validator 上线,cluster 算法的 "singleton fallback" 路径会让 cluster count 膨胀,触发 escalate 阈值。这是**算法设计与 codebase 演化节奏不匹配**问题。

每次后续加新 ViolKind(Phase 5-E2 后续 + L4 + R8 等),都会再现该问题:新 violation 没有 cooccurrence rule,各成 singleton,cluster count 攀升。

### 泛化方案(非局部补丁,后续 session 实施)

1. **cluster 数量阈值与 owner-set 阈值分开**:不要 "cluster ≥ N → escalate",改为 "distinct PrimaryOwner ≥ M → escalate"。即 4 个 cluster 全是 finalizer-locus 不算 escalate。

2. **新 validator 上线 checklist**:加新 ViolKind 时,必须同时:
   - (a) 至少 1 条 cooccurrence rule 把它聚到现有 cluster
   - (b) ViolKind→RepairLocus 映射明确
   - (c) Lock test 锁这两项

   本次 Phase 3-C5 + Phase 5-E1 都漏了 (a),所以新 violation 走 singleton。

3. **escalate 决策考虑 finalizer-internal-progress 信号**:retry 间 (ReadSet, Evidence, ChainTerm, CitedRefs) 任一进步 = 不该 escalate(I4 单调进步 invariant)。当前 escalate 只看 owner stable,不看 progress。

### 推荐落地顺序

方案 2(checklist)即刻可落 — 是 process / convention 层。方案 1(distinct owner 阈值)是算法核心改动,需配 lock test。方案 3(progress signal)与 CGEC I4 联动,改动面较大。

推荐 Session 3:方案 2 + 方案 1 同 commit(checklist 落新 ViolKind 上线 → 触发 cluster 算法改 + 加现成的 cooccurrence rules 给 Phase 3-C5/Phase 5-E1)。

---

## 共同教训

两类 outlier 都是**新 feature 上线时旧机制没同步**的产物:

- B 是 schema 描述加新字段 → LLM 误判模式扩散
- C 是 violation 加新 kind → cluster 算法 singleton 膨胀

**红线启示**:加新 typed signal 时,必查"消费这个 signal 的所有上下游路径"是否需同步更新。本次 session 的 R2 4 处同步红线已抓 schema-side 影响(struct + schema desc + skill + retry hint),但漏了:
- error-message side(B 根因 2)
- cooccurrence rule side(C 根因 2)

**建议把 R2 红线扩展为 R2'**:加新 typed signal 时,必查 6 处同步:
1. Go struct
2. Tool schema description
3. Skill prompt
4. Retry hint summary
5. JSON decoder error remap(若有 nested 容器歧义)
6. Cooccurrence rule + RepairLocus 映射(若是 ViolationKind)

第 5、6 是本 eval forensic 揭出的新条目。
