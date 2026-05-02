# Phase 0 真实场景 Trace 数据归档

**收集日期**: 2026-05-02
**Phase 0 commit**: `3870b1d`
**关联**: [semantic_surface_contract_phases.md](./semantic_surface_contract_phases.md) §0 退场标准

本文档归档 Phase 0 落地后在 5 个真实 LLM eval 上观察到的 `ClaimFormOf` 投影数据,作为 Phase 1+ 设计的输入。所有数据来自 `[trace/fev]` DEBUG 行 — Phase 0 不开 gate,纯观测。

---

## 1. 数据收集口径

- **Eval cases**: s1a (mechanism / step_list) + s5a (list_of_symbols / completeness) + m1a (explanation / buckets) + s3a (config trace) + logtri_go (log triage)
- **采集字段**: `claim_form="..."` 在 `[trace/fev]` 行的字符串值
- **每 case 行数**: 受 `len(items) > 0` 渲染门约束;single-shot 多次 builder 调用,数据按调用累积
- **总样本**: 317 行 trace
- **结果**: 5/5 case PASS,无 panic,无行为变化

---

## 2. 全局 ClaimForm 分布(5 case 总计 317 行)

| ClaimForm | 计数 | 占比 | 触发投影规则 |
|---|---:|---:|---|
| `unknown` | 108 | 34% | Rule 5: 无 typed signal(主要来自 `producer="concrete_values"`) |
| `definition_fact` | 41 | 13% | Rule 4: AnchorKind=Definition |
| `call_edge` | 29 | 9% | Rule 4: AnchorKind=Call |
| `guard_condition` | 12 | 4% | Rule 4: AnchorKind=Condition |
| `external_observation` | 12 | 4% | Rule 1: Origin=Log |
| `precedence_role` | 8 | 3% | Rule 3: DiagramRole=Config|Runtime|Override |
| `absence_fact` | 4 | 1% | Rule 2: Scope=Negative |
| `assignment_fact` | 2 | 1% | Rule 4: AnchorKind=Assignment |
| `return_fact` | 0 | 0% | (5 case 未触发) |
| `import_edge` | 0 | 0% | (5 case 未触发) |

**Priority 路径覆盖**: 5 条 priority rule 中 4 条在真实数据中触发(Rule 1/2/3/4)。Rule 5 (fallback to unknown) 也大量触发。

**ClaimForm 枚举覆盖**: 10 个声明值(含 unknown),**8 个**在 5 case 中出现。剩余 2 个(`return_fact` / `import_edge`)不是 bug,是题型未覆盖。

---

## 3. Per-case 分布

### 3.1 s1a — step_list mechanism(`gate.Run 9 项检查顺序`)

| ClaimForm | 计数 |
|---|---:|
| call_edge | 29 |
| definition_fact | 25 |
| guard_condition | 12 |
| unknown | 7 |
| assignment_fact | 2 |
| **总计** | **75** |

**Unknown 率 9%**(7/75)— 最低。LLM `emit_evidence` 主导(高占比 AnchorKind 填充)。

### 3.2 s5a — list_of_symbols(LoopController 实现)

| ClaimForm | 计数 |
|---|---:|
| unknown | 72 |
| definition_fact | 3 |
| **总计** | **75** |

**Unknown 率 96%**(72/75)— 最高。`producer="concrete_values"` 主导,无 AnchorKind 填充。

### 3.3 m1a — explanation + buckets(Turn A/B emit_*)

| ClaimForm | 计数 |
|---|---:|
| unknown | 70 |
| definition_fact | 5 |
| **总计** | **75** |

**Unknown 率 93%**(70/75)— 第二高。同 s5a 模式。

### 3.4 s3a — config trace(三层覆盖优先级)

| ClaimForm | 计数 |
|---|---:|
| precedence_role | 8 |
| definition_fact | 8 |
| unknown | 6 |
| absence_fact | 4 |
| **总计** | **26** |

**Unknown 率 23%**。**precedence_role** 在 config-trace 题型上独家出现 — DiagramRole 投影规则验证成功。**absence_fact** 也独家出现(s3a 是"目标不存在"题型)— ScopeNegative 投影规则验证成功。

### 3.5 logtri_go — log triage(panic 分析)

| ClaimForm | 计数 |
|---|---:|
| unknown | 54 |
| external_observation | 12 |
| **总计** | **66** |

**Unknown 率 82%**。**external_observation** 在 log-triage 题型上独家出现 — Origin=Log 投影规则验证成功。Unknown 仍主要来自 concrete_values。

---

## 4. 关键结构性发现(Phase 1 设计输入)

### 4.1 `producer="concrete_values"` 主导 Unknown

**现状**:`concrete_values` 是 explorer 的 deterministic Tier-3 辅助 emitter,扫描 syntax pattern(return literal / registration / decorator / map entry)产生 evidence,但 NOT 填充 `AnchorKind` / `Origin` / `Scope`。

**结果**:这些 evidence 在 `ClaimFormOf` 走到 Rule 5,落到 `ClaimUnknown`。

**Phase 1+ 含义**:
- Phase 1 `FacetRequirement.AcceptableForms` 必须**接受 `ClaimUnknown` 作为 support tier**(非 principal)
- Phase 4 hard gate 必须**对 `ClaimUnknown` 容错**(不能 reject 仅因为 evidence 是 unknown — 它们是 Tier-3 supportive,不是错的)
- 独立 backlog: `concrete_values` 可优化把 syntax pattern 映射到 AnchorKind(return literal → AnchorReturn,etc.),把 Unknown 率从 34% 降到 ~10%。**不阻塞 P1**。

### 4.2 题型与 Unknown 率强相关

| 题型族 | Unknown 率 | 主导 evidence |
|---|---:|---|
| step_list mechanism (s1a) | 9% | LLM emit_evidence with AnchorKind |
| config trace (s3a) | 23% | LLM emit_evidence + DiagramRole tagging |
| log triage (logtri_go) | 82% | concrete_values + 少量 LogFrame |
| list_of_symbols (s5a) | 96% | 几乎全是 concrete_values |
| explanation (m1a) | 93% | 几乎全是 concrete_values |

**Phase 1 设计含义**:
- mechanism 题型最适合 ClaimForm-driven facet verification
- list_of_symbols / explanation 题型**不能严依赖 ClaimForm**,Phase 1 facet plan 必须支持 fallback 到 `EvidenceItem.Source + AnchorSymbol` 直接锚点匹配
- log_triage 题型可以混合(LogFrame 走 ClaimForm,concrete_values 走 Source 锚点)

### 4.3 5 priority 路径稳定性

每条 priority rule 在真实数据上触发 ≥1 次,无路径短路异常:

| Rule | 真实触发 | Case |
|---|---:|---|
| 1. Origin=Log/Perf → external_observation | 12 | logtri_go |
| 2. Scope=Negative → absence_fact | 4 | s3a |
| 3. DiagramRole non-default → precedence_role | 8 | s3a |
| 4. AnchorKind dispatch | 84 | s1a, s3a, s5a, m1a |
| 5. Fallback → unknown | 209 | 全 case |

**结论**:Phase 0 投影矩阵无需调整,可作为 Phase 1+ 的 contract base 直接使用。

### 4.4 缺数据的 ClaimForm

`return_fact` (Rule 4 + AnchorKind=Return) + `import_edge` (Rule 4 + AnchorKind=Import) 在 5 case 中均未出现。

**评估**:不是 bug。
- `return_fact` 通常出现在 "function X returns what?" 题型 — 5 case 未覆盖该形态(s1a 是顺序,s5a 是列举,m1a 是协作,s3a 是配置,logtri_go 是 panic)
- `import_edge` 通常出现在 "package X 依赖什么?" 题型 — 5 case 未覆盖

**Phase 1 设计含义**:facet plan 不应假设这两个 ClaimForm 高频出现;它们可作为 specialized facet 的支撑(`FacetReturnSemantics` / `FacetDependencyGraph`)。

---

## 5. Phase 1 待对齐项(基于本批数据)

1. **`FacetRequirement.AcceptableForms` 设计**:必须支持 `ClaimUnknown` 作为 SOFT support;HARD facet 不能由全 unknown evidence 满足
2. **`SourceCandidate` 字段**:facet 候选 evidence 的 ID 列表,允许 fallback 路径(当 ClaimForm=unknown 但 Source 命中时)
3. **`ResolveQuestionFamily` 边界**:5 个 family 已覆盖本批 case,但 logtri_go 是 root_cause_trace 还是单独 family?需要 Phase 1 落地时决策
4. **`concrete_values` 优化 backlog**:Phase 1+ 独立 Sprint,降低全局 Unknown 率

---

## 6. Phase 0 退场认证

按 [phases doc §0.6](./semantic_surface_contract_phases.md) 的 4 条退场标准:

| 标准 | 状态 |
|---|---|
| 单元测试覆盖率 ≥80% | ✅ ClaimFormOf / IsValid / AllClaimForms 全部 100% |
| `ClaimFormOf` 在 ≥3 真实 eval 上稳定输出 | ✅ 5 case,无 panic,无矩阵 corner case |
| 无任何 ViolationKind 因 P0 触发 | ✅ P0 不开 gate |
| 5 priority 路径全部触达真实数据 | ✅ |

**Phase 0 → Phase 1 入场就绪**。

---

## 7. 文档版本

| 日期 | 变更 |
|---|---|
| 2026-05-02 | Phase 0 ship 当日 5 case 真实 trace 数据归档 |
