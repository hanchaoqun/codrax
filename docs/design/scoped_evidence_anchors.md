# 五维 Evidence Anchor Scope 模型设计 v1

> **状态**：v1 设计**已冻结**（2026-05-01 / 19:00）。所有决策（D1-D8 + Q1-Q7）已锁定，进入阶段 0。
>
> **触发问题**：s3a config-trace 题目 `explore_mid_loop_hint_budget` 不存在时，答案退化为只讲 default 层（DefaultExploreHeuristics / ResolvedExploreHeuristics），把 codrax.yaml / CLI 层完全丢了。根因不在过滤器某一处，而在 evidence 模型只支持「per-line content 证明」一种 anchor 形态。
>
> **范围**：扩展 `EvidenceItem` 的 anchor 模型从单一 `(file, line, content)` 升级为带 `Scope` 维度的多形态。**LLM 必须**显式 emit `scope` 字段（无系统兜底，无向后兼容）；触及 28 个生产文件 + 42 个测试文件，预估 ~6.5-7 天工程量，分 8 个阶段在本 session 内全部 ship。

## 1. 背景与症状（保留）

### 1.1 实测退化

s3a 测试问题：「`explore_mid_loop_hint_budget` 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级」。该 key **不存在**于代码或配置中。

老 PASS 找到同族 key + 三层完整 cite。新 PASS 引用区**完全丢失** codrax.yaml / runtime.go 锚点。LLM 实际 emit 了 codrax.yaml:300/304 两个同族注释行 anchor，但被 `validatedEvidenceDiagramRole` 的 `evidenceLooksIllustrative` 判定降级，沉到 surfaceEvidence ranking 底部。

### 1.2 系统性根因

`validateConfigTraceAbsenceCitationFocus` 要求 exact-absent + config-trace 模式下，每条 citation 必须 per-line 通过 `validatedEvidenceDiagramRole` 验证。这条约束的隐含前提：**evidence anchor = (file, line, line-content)，且 line-content 能逐字节证明该 role**。

但配置追溯需要的事实远不止逐行内容：

| # | 事实类别 | 证明性质 | 当前能 cite？ |
|---|---|---|---|
| **1** | 「X 在 file F 的 line N 被定义为 V」 | 逐行内容直接 | ✓ |
| **2** | 「F 是 config 层的 canonical 文件」 | 文件身份 | ✗ |
| **3** | 「explore_* group 不接受 CLI flag」 | 跨文件契约 | ✗ |
| **4** | 「X 不存在于 F 中」 | 全文件穷举 + 反向证明 | ✗ |
| **5** | 「X 应该在 group G 下，但 G 整段缺失」 | 结构定位 + 缺席共同证明 | ✗ |

类别 2-5 全是 **schema-level / structural** 事实，与目标存在性独立。

### 1.3 设计目标

- 类别 2-5 的事实有合法的 anchor 通道
- per-scope grounder 验证规则独立、互不冲突
- finalizer citations[] 仍是统一渲染入口
- LLM **强制**设 scope 字段（红线 R3）
- 现有 line-only 假设的所有读写点全部审计 + 改造（不留遗留）

## 2. 设计原则（红线）

| # | 原则 | 内核 |
|---|---|---|
| **R1** | LLM hint 优先 + 强校验 | LLM 必须显式 emit `scope` 字段；schema 校验在 emit_evidence 入口拒绝 missing-scope |
| **R2** | per-scope grounder 独立 | 每个 scope 的 ground 路径独立函数，互相不调用；修一个不破坏另一个 |
| **R3** | 强制 LLM 设字段 | 不向后兼容，不做 system 兜底推断；零值 scope = 失败 |
| **R4** | 渲染统一 | citations[] 仍是单一通道，per-scope 渲染细节由 Citation.Scope 字段决定 |
| **R5** | 反向 anchor 是一等公民 | 缺席 / 不适用 / schema-not-found 必须有 anchor 通道，不能藏在 prose |
| **R6** | 跨文件 anchor 必须可重现验证 | Crossfile scope 的 grounder 实际跨文件查询，不接受 LLM 单方面声明 |
| **R7** | 不破坏现有红线 | `feedback_no_custom_keyword_matching` / `user_intent_over_system_gates` / `eliminate_noise_at_source` 全部仍成立 |
| **R8** | 增量上线 | 5 个 scope 不一次性切换，按 §16 阶段顺序 ship |
| **R9** | 字段级 dispatch | 凡 `EvidenceItem.Source != "" && LineStart > 0` 的现有写法都要走过 §6 表逐处审计 |

## 3. 五个 Scope 的精确语义

### 3.1 `ScopeLine`

| 维度 | 定义 |
|---|---|
| 触发场景 | 「X 在 file F 的 line N 被定义为 V」、「F:N 处的执行点」 |
| EvidenceItem 字段 | `Source` + `LineStart` + `LineEnd?`(单行可不填) + `AnchorKind` + `AnchorSymbol` |
| Grounder | 现有 `tier1LineText` / `tier2SymbolTable` / R1-R5 recovery |
| Citation 渲染 | `file:line` |
| EvidenceKind 配套 | direct / conditional / mechanism / relationship / concrete_value |

### 3.2 `ScopeLineRange`

| 维度 | 定义 |
|---|---|
| 触发场景 | 「ExploreHeuristics struct 整体」、「DefaultExploreHeuristics 函数体」、「codrax.yaml Mid-loop detection 注释块」 |
| EvidenceItem 字段 | `Source` + `LineStart` + `LineEnd`（**必填且 > LineStart**）+ `AnchorKind` |
| Grounder（新） | `groundScopeLineRange`：file 存在 + LineEnd ≤ totalLines + LineEnd > LineStart + (option) 段首段尾包含 anchor token |
| Citation 渲染 | `file:lineStart-lineEnd` |
| EvidenceKind 配套 | definition / mechanism / direct |

### 3.3 `ScopeSection`

| 维度 | 定义 |
|---|---|
| 触发场景 | 命名 schema 节：YAML top-level group / Go const block / JSON 节 |
| EvidenceItem 字段 | `Source` + `SectionPath`（必填，如 `explore_*` 或 `agents.env_recommender`）+ `LineStart?` + `LineEnd?` |
| Grounder（新） | `groundScopeSection`：解析对应文件类型（yaml.v3 / go/ast / encoding/json / TOML）并定位 SectionPath 真实存在 |
| Citation 渲染 | `file [section: <path>]` 或 `file:start-end [section: <path>]`（line range 已知时） |
| 解析器 | yaml.v3（已存在）+ go/ast（已存在）+ TOML（pelletier）+ INI（自写小 parser） |

### 3.4 `ScopeFile`

| 维度 | 定义 |
|---|---|
| 触发场景 | 「F 是 config 层的 canonical 文件」、「runtime.go 是 RuntimeSettings 的载体」 |
| EvidenceItem 字段 | `Source`（必填）+ `LineStart=0`（明确不指向行）+ `FileRoleLabel`（必填枚举） |
| FileRoleLabel | `config_canonical` / `cli_registration` / `default_struct` / `manifest` / `schema` |
| Grounder（新） | `groundScopeFile`：文件存在 + FileRoleLabel 对应的 path-shape 校验（如 config_canonical 要求 `LooksLikeConfigFilePath`） |
| Citation 渲染 | `file [layer: <label>]` |
| EvidenceKind 配套 | direct |

### 3.5 `ScopeCrossfile`

| 维度 | 定义 |
|---|---|
| 触发场景 | 「explore_* group 不接受 CLI flag」、「foo handler 在 routes.go 注册并在 handler.go 实现」、registry 模式 |
| EvidenceItem 字段 | `CrossfileQuery`（必填：files + pattern + context?）+ `CrossfileAssertion`（必填：kind + count?） |
| CrossfileAssertion.Kind | `exists` / `forbidden` / `count_eq` |
| Grounder（新） | `groundScopeCrossfile`：跨 query.Files 实际 grep query.Pattern + 计数符合 assertion |
| Citation 渲染 | `cross-file: <human-readable-summary>` |
| 性能预算 | files ≤ 5（schema 层硬限）+ 缓存 per-(query, assertion) 结果 |

### 3.6 `ScopeNegative`

| 维度 | 定义 |
|---|---|
| 触发场景 | 显式声明缺席：「key X 不在 codrax.yaml」、「symbol Y 不在 file F」 |
| EvidenceItem 字段 | `NegativeQuery`（必填：file + pattern + section?）+ `NegativeScope`（必填：file/range/section/struct_fields） |
| Grounder（新） | `groundScopeNegative`：file 存在 + 按 NegativeScope 限定查询范围 + 实际 grep 必须 0 命中 |
| Citation 渲染 | `<file/section> [absence: <pattern>]` |
| EvidenceKind 配套 | absent（**激活当前已废弃的 EvidenceAbsent**）+ 选用此 Kind 时 system 强制要求 ScopeNegative |

## 4. 数据结构（精确）

### 4.1 `internal/types/evidence.go`：`EvidenceItem` 扩展

```go
type EvidenceItem struct {
    ID          string       `json:"id"`
    Kind        EvidenceKind `json:"kind"`
    Subject     string       `json:"subject,omitempty"`
    Predicate   string       `json:"predicate,omitempty"`
    Object      string       `json:"object,omitempty"`
    Summary     string       `json:"summary,omitempty"`
    Condition   string       `json:"condition,omitempty"`
    Source      string       `json:"source,omitempty"`
    EvidenceRef string       `json:"evidence_ref,omitempty"`
    LineStart   int          `json:"line_start,omitempty"`
    LineEnd     int          `json:"line_end,omitempty"`
    DerivedFrom []string     `json:"derived_from,omitempty"`
    Confidence  float64      `json:"confidence,omitempty"`
    Producer    string       `json:"producer,omitempty"`

    // Role fields（保留，与 Scope 正交）
    ContextRole          EvidenceContextRole `json:"context_role,omitempty"`
    DiagramRole          EvidenceDiagramRole `json:"diagram_role,omitempty"`
    RequestedDiagramRole EvidenceDiagramRole `json:"requested_diagram_role,omitempty"`

    // Anchor fields（保留，ScopeLine / LineRange 用）
    AnchorKind   AnchorKind `json:"anchor_kind,omitempty"`
    AnchorSymbol string     `json:"anchor_symbol,omitempty"`
    OwnerSymbol  string     `json:"owner_symbol,omitempty"`
    Snippet      string     `json:"snippet,omitempty"`

    // Grounding output
    GroundingStatus GroundingStatus `json:"grounding_status,omitempty"`
    GroundingTier   GroundingTier   `json:"grounding_tier,omitempty"`
    GroundingNote   string          `json:"grounding_note,omitempty"`

    // ====== 2026-05+ NEW: Scope 维度 ======
    Scope EvidenceScope `json:"scope"`           // 必填，零值非法

    // 仅在 Scope=Section 时填写
    SectionPath string `json:"section_path,omitempty"`

    // 仅在 Scope=File 时填写
    FileRoleLabel FileRoleLabel `json:"file_role_label,omitempty"`

    // 仅在 Scope=Crossfile 时填写
    CrossfileQuery     *CrossfileQuery     `json:"crossfile_query,omitempty"`
    CrossfileAssertion *CrossfileAssertion `json:"crossfile_assertion,omitempty"`

    // 仅在 Scope=Negative 时填写
    NegativeQuery *NegativeQuery `json:"negative_query,omitempty"`
    NegativeScope NegativeScope  `json:"negative_scope,omitempty"`
}

// EvidenceScope is the anchor-shape dimension. Required field on every
// EvidenceItem; zero value is invalid.
type EvidenceScope string

const (
    ScopeLine      EvidenceScope = "line"
    ScopeLineRange EvidenceScope = "line_range"
    ScopeSection   EvidenceScope = "section"
    ScopeFile      EvidenceScope = "file"
    ScopeCrossfile EvidenceScope = "crossfile"
    ScopeNegative  EvidenceScope = "negative"
)

func (s EvidenceScope) IsValid() bool { /* ... */ }
func AllEvidenceScopes() []EvidenceScope { /* ... */ }

type FileRoleLabel string

const (
    FileRoleConfigCanonical FileRoleLabel = "config_canonical"
    FileRoleCLIRegistration FileRoleLabel = "cli_registration"
    FileRoleDefaultStruct   FileRoleLabel = "default_struct"
    FileRoleManifest        FileRoleLabel = "manifest"
    FileRoleSchema          FileRoleLabel = "schema"
)

type CrossfileQuery struct {
    Files   []string `json:"files"`
    Pattern string   `json:"pattern"`              // regex
    Context string   `json:"context,omitempty"`    // optional: limit to section / function name
}

type CrossfileAssertion struct {
    Kind  CrossfileAssertionKind `json:"kind"`
    Count int                    `json:"count,omitempty"`
}

type CrossfileAssertionKind string

const (
    CrossfileExists    CrossfileAssertionKind = "exists"
    CrossfileForbidden CrossfileAssertionKind = "forbidden"
    CrossfileCountEq   CrossfileAssertionKind = "count_eq"
)

type NegativeQuery struct {
    File    string `json:"file"`
    Pattern string `json:"pattern"`
    Section string `json:"section,omitempty"`
}

type NegativeScope string

const (
    NegativeScopeFile         NegativeScope = "file"
    NegativeScopeRange        NegativeScope = "range"
    NegativeScopeSection      NegativeScope = "section"
    NegativeScopeStructFields NegativeScope = "struct_fields"
)
```

### 4.2 `Citation` 扩展

`internal/types/answer_document.go::Citation` 加 Scope 维度：

```go
type Citation struct {
    File  string
    Line  int           // 0 when Scope != Line/LineRange
    Quote string

    // 2026-05+ NEW
    Scope EvidenceScope        `json:"scope"`
    LineEnd int                `json:"line_end,omitempty"`
    SectionPath string         `json:"section_path,omitempty"`
    FileRoleLabel FileRoleLabel `json:"file_role_label,omitempty"`
    CrossfileSummary string    `json:"crossfile_summary,omitempty"`  // 渲染用：人读总结
    NegativePattern string     `json:"negative_pattern,omitempty"`
}
```

### 4.3 `StableEvidenceID` 扩展

```go
func StableEvidenceID(item EvidenceItem) string {
    h := fnv.New64a()
    parts := []string{
        string(item.Kind),
        string(item.Scope),         // NEW: scope 进入 hash
        item.Subject, item.Predicate, item.Object, item.Condition,
        item.Source,
        fmt.Sprintf("%d:%d", item.LineStart, item.LineEnd),
    }
    switch item.Scope {
    case ScopeSection:
        parts = append(parts, item.SectionPath)
    case ScopeFile:
        parts = append(parts, string(item.FileRoleLabel))
    case ScopeCrossfile:
        if item.CrossfileQuery != nil {
            parts = append(parts, strings.Join(item.CrossfileQuery.Files, ","), item.CrossfileQuery.Pattern)
        }
    case ScopeNegative:
        if item.NegativeQuery != nil {
            parts = append(parts, item.NegativeQuery.File, item.NegativeQuery.Pattern, item.NegativeQuery.Section)
        }
    }
    _, _ = h.Write([]byte(strings.Join(parts, "\x1f")))
    return fmt.Sprintf("ev-%x", h.Sum64())
}
```

签名从 `(kind, subject, ..., source, lineStart, lineEnd)` 改为 `(item EvidenceItem)`，所有 7 处调用点同步更新。

## 5. emit_evidence schema 扩展

`internal/tool/emit_evidence.go::Execute` 的 JSON schema items[i] 加：

```json
{
  "scope": {
    "type": "string",
    "enum": ["line", "line_range", "section", "file", "crossfile", "negative"],
    "description": "REQUIRED. Anchor scope. Pick the scope that matches what your evidence proves: line=specific code location; line_range=multi-line definition block; section=named YAML/Go schema section; file=file's identity as a layer (e.g. codrax.yaml as the config layer); crossfile=cross-file contract verified by query; negative=confirmed absence of a target."
  },
  "line_end": { "type": "integer" },
  "section_path": { "type": "string" },
  "file_role_label": {
    "type": "string",
    "enum": ["config_canonical", "cli_registration", "default_struct", "manifest", "schema"]
  },
  "crossfile_query": {
    "type": "object",
    "properties": {
      "files": {"type":"array", "items":{"type":"string"}, "maxItems": 5},
      "pattern": {"type":"string"},
      "context": {"type":"string"}
    }
  },
  "crossfile_assertion": {
    "type": "object",
    "properties": {
      "kind": {"type":"string", "enum":["exists","forbidden","count_eq"]},
      "count": {"type":"integer"}
    }
  },
  "negative_query": {
    "type": "object",
    "properties": {
      "file": {"type":"string"},
      "pattern": {"type":"string"},
      "section": {"type":"string"}
    }
  },
  "negative_scope": {
    "type": "string",
    "enum": ["file","range","section","struct_fields"]
  }
}
```

**Schema-level 校验规则**（拒绝 emit）：
- `scope` 缺失 → reject
- `scope=line` AND (`source==""` OR `line_start<=0` OR `anchor_kind==""`) → reject
- `scope=line_range` AND (`line_end<=line_start`) → reject
- `scope=section` AND `section_path==""` → reject
- `scope=file` AND `file_role_label==""` → reject
- `scope=crossfile` AND (`crossfile_query==nil` OR `len(files)>5` OR `pattern==""` OR `crossfile_assertion==nil`) → reject
- `scope=negative` AND (`negative_query==nil` OR `negative_scope==""`) → reject
- `kind=absent` AND `scope!=negative` → reject（`EvidenceAbsent` 重新激活，但**仅**与 ScopeNegative 配套）

## 6. **完整消费点清单（28 生产文件 + 42 测试文件）**

### 6.1 生产代码逐文件改动

| # | 文件 | 改动性质 | 关键函数 / 字段 |
|---|---|---|---|
| 1 | `internal/types/evidence.go` | **核心扩展** | `EvidenceItem` 加 7 字段；新增 `EvidenceScope` / `FileRoleLabel` / `CrossfileQuery` / `CrossfileAssertion` / `NegativeQuery` / `NegativeScope` 类型；`StableEvidenceID` 改签名；`AllEvidenceScopes()` / `IsValid()` 等 helper；`EvidenceCountsTowardTier1Floor` 加 scope 分支（schema-level scopes 不入 Tier-1 floor）；`EvidenceCountsTowardTier1FloorInContext` 同步；`EvidenceItem.IsCitable()` 加 scope 分支；`EvidenceAbsent` kind 重新激活 |
| 2 | `internal/types/exact_lookup.go` | 高频读改造 | 30+ 处 `item.Source` / `item.DiagramRole` / `item.ContextRole` 读 → 加 scope 分支：File/Crossfile/Negative scopes 使用专属验证路径，不走原 `LooksLikeConfigFilePath` 路径；`ExactResolutionEvidenceCanSatisfyRelatedContext` 等接受 ScopeFile + FileRoleLabel=config_canonical 作为合法 |
| 3 | `internal/types/context.go` | 中频改造 | `AppendEvidence` 入口加 scope 校验（生产无效 scope panic）；`StableEvidenceID` 调用同步；`filterEvidenceItemsByScope`（write-mode 用）保持原 `req.Scope` 语义不变（不和 EvidenceScope 混淆，命名上保持区分） |
| 4 | `internal/types/evidence_closure.go` | 中频改造 | `RecordReadFile` 等 closure 操作只跟踪 ScopeLine / ScopeLineRange 的覆盖；schema-level scopes 走独立 `RecordSchemaAnchor`（新方法）追踪 |
| 5 | `internal/types/answer_surface_plan.go` | **核心改造** | `BuildAnswerSurfacePlan`：评定 SurfaceEvidence 时 schema-level scopes 自动入 surfaceEvidence pool，不被 line-only ranker 沉底；`CollectLogObservedAnchors` / `CollectDriftBoundedSurfaceItems` 仅消费 ScopeLine / ScopeLineRange；`collectAllowedExactContextItems` 等接受 ScopeFile + FileRoleLabel 作为合法 anchor 类型 |
| 6 | `internal/types/answer_document.go` | 中频改造 | `Citation` 加 6 字段；`AnswerDocument.IsZero` / `CloneAnswerDocument` 同步；`Citation` 的渲染走 §8 |
| 7 | `internal/types/subagent.go` | 低频改造 | `SubagentResult.EvidenceItems` 字段类型不变（仍是 slice），消费方按 scope 分发 |
| 8 | `internal/types/diagram_contract_support.go` | 低频改造 | `EffectiveDiagramContract` 输入接受 schema-level evidence 作为 grounded support kinds |
| 9 | `internal/tool/ground/ground.go` | **核心改造** | 现有 `GroundItem` 重命名为 `groundScopeLine`；新增 dispatcher `GroundItemScoped` |
| 10 | `internal/tool/ground/scope_dispatch.go` | **新增** | 5 个 grounder 函数 + 共享 helpers + 缓存 `parsedSections` / `crossfileQueryCache` |
| 11 | `internal/tool/ground/coverage.go` | 低频改造 | `RefreshClosureCoverage` 仅追踪 ScopeLine / LineRange；schema-level scopes 单独 `RefreshClosureSchemaAnchors` |
| 12 | `internal/tool/emit_evidence.go` | **核心改造** | schema 加 7 字段 + scope 校验逻辑；`buildEmitEvidenceItem` 设置 Scope 字段；`Execute` 调用 `GroundItemScoped` 替换 `GroundItem`；`validatedEvidenceContextRole` / `validatedEvidenceDiagramRole` per-scope 分发：File scope 直通 / Negative scope 直接 mark absence-support / Section / LineRange 走 line 路径；`evidenceLooksIllustrative` 仅 ScopeLine 触发；`stampEvidenceOwnerSymbol` / `stabilizeXxx` 系列仅 ScopeLine / LineRange 适用 |
| 13 | `internal/tool/emit_answer_document.go` | **核心改造** | 32 个函数全部 audit。重点改造点：(a) `validateConfigTraceAbsenceCitationFocus` 接受 ScopeFile/Crossfile/Negative 作合法 cite；(b) `validatedEvidenceDiagramRole` 在 ScopeFile 场景下基于 FileRoleLabel 直接派 EvidenceDiagramRole；(c) `buildSummaryDiagramAllowlist` 加入 schema-level scope cite；(d) `pruneExplanationCitationsForSurface` per-scope 排序；(e) `matchingEvidenceForCitation` 比对扩展到 scope 字段；(f) `buildEmitAnswerDocumentCitations` 渲染 Citation.Scope/LineEnd/SectionPath/etc.；(g) Citation schema 校验规则 |
| 14 | `internal/tool/emit_answer_document_enrich.go` | 低频改造 | `BuildContext` 调用同步；不引入新 scope 改 |
| 15 | `internal/tool/emit_answer_symbol.go` | 中频改造 | symbol anchor 验证：仅 ScopeLine / LineRange 的 evidence 视为合法 anchor；schema-level scope 不能匿 anchor |
| 16 | `internal/tool/emit_investigation_complete.go` | 中频改造 | `RefreshClosureCoverage` 调用同步；closure refresh per scope 路径 |
| 17 | `internal/tool/multipath/decision.go` | 中频改造 | `ExtractKeywordAnchors` 仅消费 ScopeLine / LineRange；schema-level scopes 不进多路径 anchor 流 |
| 18 | `internal/tool/log_source_drift_surface.go` | 低频改造 | drift 处理仅 ScopeLine / LineRange；schema-level evidence 不参与 drift 重建 |
| 19 | `internal/agent/explorer.go` | 中频改造 | `filterEvidenceByPrimaryFiles` / `balanceEvidenceAcrossPrimaryFiles`：schema-level scopes 豁免 primary-file 平衡（独立通道）；`scalarRoleLocateEvidenceReady` / `evidenceSurfaceTailSet` / `exactAbsenceClosureReady` 加 scope 分支；mid-loop hint 当 exact-absent + config-trace 时主动建议 LLM emit ScopeFile / ScopeCrossfile（hint 文本里说明） |
| 20 | `internal/agent/extractor.go` | 中频改造 | 答案 symbol 合成路径：仅 ScopeLine / LineRange 的 evidence 提取 file:line literal；schema-level scopes 通过 prose 解释（不在 symbols[] 直出）；hypothesis verdict 渲染包含所有 scopes |
| 21 | `internal/agent/evidence.go` | 中频改造 | `parseEvidenceItems`（legacy `[ABSENT]` 解析）走 ScopeNegative 路径；`StableEvidenceID` 调用同步 |
| 22 | `internal/agent/exact_resolution_scope.go` | 高频改造 | `scopeShapingDiagramRole` / `exactResolutionAnchoredFiles` / `exactResolutionEvidenceMentionsCandidate` 等 18+ 函数加 scope 分支；File scope evidence 视为 layer-anchored、不追求逐行 ground |
| 23 | `internal/agent/sub_explorer.go` | 低频改造 | EvidenceItems handoff 不变；merge / dedup 按 StableEvidenceID（已 scope-aware） |
| 24 | `internal/agent/explorer_erm.go` | 中频改造 | `checkRequirementSatisfaction` / ERM 平衡评分：schema-level scopes 视为同等满足度但不入 Tier-1 floor 计数 |
| 25 | `internal/agent/answer_document_evaluator.go` | 中频改造 | 答案文档评估器接受 schema-level scopes 作合法答案 anchor |
| 26 | `internal/agent/mechanism_scan.go` | 低频改造 | 确定性产出 `EvidenceItem` 时手动设置 `Scope=ScopeLine`（不依赖 LLM） |
| 27 | `internal/agent/stage_report_render.go` | 低频改造 | `formatEvidenceLineForReport` 加 scope 渲染 case |
| 28 | `internal/analysis/dataflow/lower.go` | 低频改造 | dataflow 产出 `EvidenceItem` 时手动设置 `Scope=ScopeLine` |
| 29 | `internal/analysis/dataflow/engine.go` | 低频改造 | StableEvidenceID 调用同步 |
| 30 | `internal/analysis/dataflow/types.go` | 低频改造 | 内部 ev 类型加 scope 转换 |
| 31 | `internal/analysis/criterion/eval.go` | 中频改造 | `evalRelationAbsent` 仅消费 ScopeLine / LineRange；`evalNoRelevantEvidence` 接受全 scope；`evalEvidenceCount` 通用；`evalExternalArtifactDecoded` 通用 |
| 32 | `internal/analysis/criterion/grammar.go` | 低频改造 | 仅类型引用 |
| 33 | `internal/orchestrator/cgec_enforcers.go` | 中频改造 | `hashEvidenceIDs` / `hashChainTerminals` 通过 `StableEvidenceID` 间接 scope-aware；`it.Kind == EvidenceDataflowPath` 路径不变（dataflow 永远 ScopeLine） |
| 34 | `internal/orchestrator/tier1_floor.go` | 中频改造 | Tier1 floor 计数仅纳入 ScopeLine / LineRange / Section；schema-level（File/Crossfile/Negative）不入 Tier1 denominator |
| 35 | `internal/orchestrator/contract_check.go` | 低频改造 | 跑 contract.Check 时把全 scope evidence 扔进 Env.Evidence |
| 36 | `internal/orchestrator/orchestrator.go` | 低频改造 | EvidenceItems 流转，scope 透传不变 |
| 37 | `internal/orchestrator/evidence_utilization_log.go` | 低频改造 | 日志加 scope 字段 |
| 38 | `internal/render/answerdoc.go` | 中频改造 | `renderAnswerDocCitationPool` per-scope 渲染分支（§8）；`lookupCitation` 不变；`renderAnswerDocSnippets` 仅渲染 ScopeLine / LineRange 类 snippets |
| 39 | `internal/render/event.go` | 低频改造 | EventEmitEvidence 渲染加 scope 字段（dock 显示） |
| 40 | `internal/render/mermaid_render.go` | 低频改造 | mermaid 渲染层不感知 scope |
| 41 | `internal/context/builder.go` | 中频改造 | `filterEvidenceItemsByScope`（write-mode）保留原意；`AgentContext.EvidenceItems` 流转 scope 透传；`logBundleAuthoritativeFrames` 不受影响 |
| 42 | `internal/skill/defaults.go` | **prompt 改造** | explore-skill / change-plan-skill / answer-document-skill 增加 scope 选用提示（§7）；`emit_evidence` schema 描述同步 |
| 43 | `internal/skill/analysis_contract.go` | 低频改造 | analysis 上下文 prompt 同步 |

### 6.2 测试文件清单（42 文件需更新）

| Layer | 文件 |
|---|---|
| `internal/agent/` | answer_document_evaluator_test.go, chain_dedup_test.go, emit_evidence_merge_test.go, evidence_test.go, exact_resolution_scope_test.go, explorer_chain_promotion_range_test.go, explorer_chain_promotion_test.go, explorer_erm_test.go, explorer_evaluator_test.go, explorer_t123_test.go, explorer_test.go, extractor_axis_test.go, extractor_test.go, stage_report_render_test.go, sub_explorer_test.go, turn_a_merge_test.go |
| `internal/analysis/` | criterion/eval_test.go |
| `internal/context/` | builder_test.go |
| `internal/orchestrator/` | apply_stage_output_dedup_test.go, cgec_enforcers_test.go, contract_check_test.go, orchestrator_dag_test.go, read_e2e_regression_test.go, scheduler_test.go, tier1_floor_test.go, two_turn_e2e_test.go, validate_stuck_test.go |
| `internal/tool/` | emit_answer_document_seed_fidelity_test.go, emit_answer_document_test.go, emit_answer_symbol_test.go, emit_evidence_test.go, emit_investigation_complete_precomplete_test.go, emit_investigation_complete_test.go, ground/ground_test.go, ground/ground_unicode_test.go, multipath/decision_test.go |
| `internal/types/` | answer_surface_drift_tiers_test.go, diagram_contract_support_test.go, evidence_display_test.go, evidence_surface_test.go, exact_lookup_test.go, requirement_kind_closure_test.go, turn_a_artifacts_test.go |

每个测试构造 `EvidenceItem{...}` 时必须显式设 `Scope:` 字段；缺失会被新校验拒绝（panic 或 emit reject）。

### 6.3 命名清单（避免冲突）

仓库已存在 `Scope` 标识符的位置：
- `internal/tool/emit_write_analysis.go` 的 `WriteScope` / `Task.Scope`
- `internal/orchestrator/plan_critic.go::PlanCriticIn.Scope`
- `internal/context/builder.go::filterEvidenceItemsByScope` 的 `req.Scope`
- `internal/tool/repomap/index/extract_go.go::goLocalScope`

**为避免歧义，新类型使用 `EvidenceScope`** + scope-prefixed 函数（`groundScopeLine` 等）+ `EvidenceItem.Scope` 字段。`filterEvidenceItemsByScope` (write mode) 保持名字不变（参数仍是 `req.Scope`，与 EvidenceScope 不冲突）。

## 7. Skill prompt 教育（强校验配套）

### 7.1 explore-skill

新增 PHASE 2 规则：

> When you call emit_evidence, you MUST set the `scope` field on every item:
> - `scope=line` — pointing at a specific line of code (current default for most call/condition/assignment evidence)
> - `scope=line_range` — anchoring a multi-line block (struct definition, function body, comment block); REQUIRES `line_end > line_start`
> - `scope=section` — naming a structural schema section (YAML top-level group, Go const block); REQUIRES `section_path` (e.g. `"explore_*"` or `"agents.env_recommender"`)
> - `scope=file` — citing a file's identity as a layer (e.g. "codrax.yaml is the canonical config file"); REQUIRES `file_role_label` ∈ {`config_canonical`, `cli_registration`, `default_struct`, `manifest`, `schema`}; do NOT set line_start
> - `scope=crossfile` — asserting a cross-file contract verified by query (e.g. "no CLI flag for explore_* group"); REQUIRES `crossfile_query` and `crossfile_assertion`; the system will RE-RUN your query and reject if assertion fails
> - `scope=negative` — confirming an absence (e.g. "X is not in codrax.yaml"); REQUIRES `negative_query` and `negative_scope`; use `kind=absent`
>
> The scope is REQUIRED. Emits without scope are rejected. Pick the scope that matches the actual proof shape — do not always default to `line`.

### 7.2 change-plan-skill / answer-document-skill

引导提示：

> When citing layer-canonical files (e.g. config files as a config layer), use `scope=file`. When asserting cross-file contracts, use `scope=crossfile`. When confirming absence, use `scope=negative`. The grounder validates each scope independently; mis-using a scope is rejected and you must re-emit with the correct one.

## 8. Citation 渲染（per-scope）

`internal/render/answerdoc.go::renderAnswerDocCitationPool` 按 `Citation.Scope` 切换：

| Scope | 渲染样式 | 示例 |
|---|---|---|
| line | `\`<file>:<line>\`` | `\`internal/agent/foo.go:42\`` |
| line_range | `\`<file>:<lineStart>-<lineEnd>\`` | `\`internal/types/config.go:796-870\`` |
| section | `\`<file> [section: <SectionPath>]\`` | `\`codrax.yaml [section: explore_*]\`` |
| file | `\`<file> [layer: <FileRoleLabel>]\`` | `\`codrax.yaml [layer: config_canonical]\`` |
| crossfile | `cross-file contract: <CrossfileSummary>` | `cross-file contract: explore_* 在 cmd/root.go 中无 CLI flag 注册` |
| negative | `\`<file>\` [absence: \`<NegativePattern>\`]` | `\`codrax.yaml\` [absence: \`explore_mid_loop_hint_budget\`]` |

`lookupCitation` 仍按 CitationRef integer 解析；CitationRef 跨 scope 通用。

## 9. Per-Scope Grounder 实现细节

### 9.1 入口分发（`internal/tool/ground/scope_dispatch.go` 新增）

```go
func GroundItemScoped(it *types.EvidenceItem, gc *Context) Report {
    if !it.Scope.IsValid() {
        return Report{Status: GroundingUngrounded, Note: "missing or invalid scope"}
    }
    switch it.Scope {
    case types.ScopeLine:      return groundScopeLine(it, gc)
    case types.ScopeLineRange: return groundScopeLineRange(it, gc)
    case types.ScopeSection:   return groundScopeSection(it, gc)
    case types.ScopeFile:      return groundScopeFile(it, gc)
    case types.ScopeCrossfile: return groundScopeCrossfile(it, gc)
    case types.ScopeNegative:  return groundScopeNegative(it, gc)
    }
    return Report{Status: GroundingUngrounded}
}
```

### 9.2 各 grounder 验证规则

| Scope | 必填字段 | 步骤 | 通过条件 | 失败码 |
|---|---|---|---|---|
| Line | Source, LineStart, AnchorKind | 现有 tier1/tier2/R1-R5 | 现有 | 现有 |
| LineRange | Source, LineStart, LineEnd | (1)file 存在 (2)LineEnd > LineStart (3)LineEnd ≤ totalLines | 全 ✓ | range_invalid / range_overflow |
| Section | Source, SectionPath | (1)file 存在 (2)per-ext 解析（yaml/go/json/toml）(3)SectionPath 在解析结果中 | section 存在 | section_not_found / parse_failed |
| File | Source, FileRoleLabel | (1)file 存在 (2)FileRoleLabel 与 file shape 一致（如 config_canonical → LooksLikeConfigFilePath） | label 匹配 path-shape | file_not_found / role_mismatch |
| Crossfile | CrossfileQuery, CrossfileAssertion | (1)Files 全部存在 (2)实际 grep query.Pattern (3)断言 | assertion 通过 | query_failed / assertion_violated |
| Negative | NegativeQuery, NegativeScope | (1)file 存在 (2)按 NegativeScope 限定查询范围 (3)实际 grep 必须 0 命中 | 0 命中 | pattern_found / scope_invalid |

### 9.3 性能预算 + 缓存

| Scope | 单次成本 | 缓解 |
|---|---|---|
| Line / LineRange | μs | 现有 LineIndex 缓存 |
| Section | ms（首次解析） | `ground.Context.parsedSections map[string]*sectionIndex` LRU 200 |
| File | μs（仅 stat） | 无 |
| Crossfile / Negative | 10-100ms | `ground.Context.crossfileQueryCache map[crossfileQueryKey]Result` LRU 200 |

跨 scope 累积上限：单 dispatch 内 ground 总 wall-time 5s 截断（保护宽松投递）。

## 10. CGEC / Closure 互动

`EvidenceClosure` 新增方法：

```go
func (c *EvidenceClosure) RecordSchemaAnchor(item EvidenceItem)
```

仅 schema-level scopes（File / Section / Crossfile / Negative）调用此方法投递。Tier-1 floor 仅算 ScopeLine / LineRange（保守起见 Section 也算 line-shaped）；File / Crossfile / Negative **不**入 Tier-1 denominator（它们是 schema 事实，无 line 内容可被 read_file 验证）。

`EvidenceCountsTowardTier1Floor`：

```go
func EvidenceCountsTowardTier1Floor(ev EvidenceItem) bool {
    if ev.Kind == EvidenceUnresolved { return false }
    switch ev.ContextRole {
    case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
        return false
    }
    switch ev.Scope {
    case ScopeFile, ScopeCrossfile, ScopeNegative:
        return false  // schema-level; not line-anchorable
    }
    return true
}
```

## 11. s3a 端到端走通示例

应用本设计后，s3a 应产出：

```
explorer.emit_evidence:
  1. {scope:line, source:internal/types/config.go, line_start:876,
      kind:direct, anchor_kind:definition, anchor_symbol:DefaultExploreHeuristics}
  2. {scope:line_range, source:internal/types/config.go, line_start:796, line_end:870,
      kind:definition, anchor_kind:definition, anchor_symbol:ExploreHeuristics}
  3. {scope:negative, kind:absent, negative_query:{file:internal/types/config.go,
      pattern:"MidLoopHintBudget"}, negative_scope:struct_fields}
  4. {scope:file, source:codrax.yaml, file_role_label:config_canonical, kind:direct}
  5. {scope:negative, kind:absent, negative_query:{file:codrax.yaml,
      pattern:"explore_mid_loop_hint_budget"}, negative_scope:file}
  6. {scope:crossfile, kind:direct,
      crossfile_query:{files:[cmd/root.go], pattern:"flag\\..*\\bExplore\\w+"},
      crossfile_assertion:{kind:forbidden}}

finalizer citations[]:
  [0] config.go:876                                            [Line]      → 默认值层
  [1] config.go:796-870                                        [LineRange] → 结构层定义
  [2] config.go [absence: MidLoopHintBudget in struct fields]  [Negative]  → 缺席（结构层）
  [3] codrax.yaml [layer: config_canonical]                    [File]      → 配置层
  [4] codrax.yaml [absence: explore_mid_loop_hint_budget]      [Negative]  → 缺席（配置层）
  [5] cross-file: explore_* 无 CLI flag 注册                    [Crossfile] → 覆盖层（不适用）
```

answer 散文：

> `explore_mid_loop_hint_budget` 在当前代码 / 配置中均不存在。三层覆盖优先级层级机制：
> - **代码默认值层**：`DefaultExploreHeuristics()` (citations[0]) 是所有 explore_* 默认值的入口；该 key 不在 `ExploreHeuristics` struct (citations[1]) 字段集合中（缺席：citations[2]）
> - **配置层**：`codrax.yaml` (citations[3]) 是 codrax 的 canonical config 层文件，但该文件中无此 key (citations[4])
> - **覆盖层**：`explore_*` group 整体不接受 CLI flag (citations[5])，因此该层对 explore_* 类 key 不适用

## 12. 与现有红线的兼容

| 红线 | 本设计兼容 |
|---|---|
| `feedback_no_custom_keyword_matching` | scope 选用基于 source path / line shape / 解析结果，不引入关键字表 ✓ |
| `feedback_user_intent_over_system_gates` | LLM 主导 scope 选择；system 仅校验合规性 ✓ |
| `feedback_eliminate_noise_at_source` | 在 evidence 模型本身解决 ✓ |
| `feedback_root_cause_only` | 直接攻 anchor 模型只支持 line-only 这一根因 ✓ |
| 写模式 W1/W1b/L3/L5 | 仅扩 read 路径 evidence 模型，不动写模式 ✓ |
| `feedback_two_stage_iteration_cap` | 与 ShouldStop / iter cap 无关 ✓ |

## 13. 风险与未解问题

| # | 风险 | 缓解 |
|---|---|---|
| 1 | LLM 不会用新 scope 字段 | 强 schema 校验拒绝 emit；skill prompt 教育；红线 R3 保证 emit reject 是显式失败而非沉默退化 |
| 2 | 未严格列出的下游消费方漏改 | §6.1 表 28+5 个文件全 audit，单测覆盖 + ship 阶段 eval 验证 |
| 3 | yaml/go AST 解析边界（语法错误） | parsedSections 缓存 nil 结果；调用方 fallback 走 line scope |
| 4 | LLM 误用 ScopeFile（任意文件随便 cite） | grounder 必须验证 FileRoleLabel 与 file shape 一致；不一致拒绝 |
| 5 | Crossfile query 性能巨大（grep 全 repo） | files ≤ 5 schema 强制；超过 reject |
| 6 | 测试一次性大批量更新风险（破坏现有信号） | 阶段化 ship；每阶段单独 ship + eval |
| 7 | 老的 EvidenceAbsent kind 重新激活与 emit_investigation_complete.absence_justification 的语义重叠 | 文档化：emit_investigation_complete 是 whole-answer 缺席；EvidenceAbsent + ScopeNegative 是 per-fact 缺席；两者并存正交 |

## 14. 工程规模评估

| 阶段 | 改动文件 | 估算 LOC | 测试 LOC | 周期 |
|---|---|---|---|---|
| **阶段 0**：types + Scope 定义 + StableEvidenceID | 8 | ~400 | ~200 | 0.5 天 |
| **阶段 1**：emit_evidence schema + LLM 强校验 | 3 | ~250 | ~250 | 0.5 天 |
| **阶段 2**：ground/scope_dispatch + ScopeLine wrap（5 grounder 桩） | 4 | ~300 | ~300 | 1 天 |
| **阶段 3**：ScopeFile + ScopeNegative grounder 实装（解 s3a） | 6 | ~400 | ~350 | 1 天 |
| **阶段 4**：ScopeLineRange + ScopeSection grounder（含 yaml/Go AST） | 5 | ~500 | ~400 | 1.5 天 |
| **阶段 5**：ScopeCrossfile grounder + 性能 cache | 3 | ~350 | ~300 | 1 天 |
| **阶段 6**：finalizer / surface plan / closure 5-scope 路径 | 8 | ~700 | ~500 | 1.5 天 |
| **阶段 7**：Citation 渲染 + 测试更新 + skill prompt | 6 | ~300 | ~200 | 0.5 天 |
| **阶段 8**：dataflow / mechanism_scan / extractor / explorer scope 标注 | 8 | ~400 | ~250 | 1 天 |
| **合计** | **~35 个唯一文件** | **~3600 LOC** | **~2750 LOC** | **~7 天** |

注：实际改动比初稿估算大（3600 vs 2350），因为消费点比初估多 8 个文件。仍在用户接受的 6.5-7 天工程量内。

## 15. 落地节奏（本 session 内分批 ship）

| 阶段 | 单测 + s3a 表现 | ship gate（每阶段必须通过） |
|---|---|---|
| 0 | 编译通过；StableEvidenceID 测试 pass；EvidenceItem.Scope 必填，新建 EvidenceItem 缺 scope 测试报错 | go test ./... 全绿 |
| 1 | emit_evidence schema 校验拒绝 missing scope；现有所有调用点设 ScopeLine | go test ./... 全绿；现有 eval 题型不退化（logtri_go / s1a / s3a 老答案） |
| 2 | scope_dispatch 工作；ScopeLine 走原 GroundItem 路径 | go test ./... + s3a 仍 PASS |
| 3 | s3a 出现 codrax.yaml 文件级 anchor + Negative absence anchor | s3a 答案中 codrax.yaml 作为 cited anchor 出现 |
| 4 | s3a 增 ExploreHeuristics struct range + Section anchor | s3a 答案带 line range 引用 |
| 5 | s3a 出现「explore_* 无 CLI flag」cross-file 锚点 | s3a 三层全 surface |
| 6 | finalizer 渲染开启 per-scope，整体丰富 | s3a 答案丰富度恢复或超过老 PASS |
| 7 | Citation 渲染 + 测试 + skill prompt | go test ./... 全绿 + skill prompt 验证 LLM 实测能选 scope |
| 8 | 确定性 producer（dataflow / mechanism_scan）扫尾 | go test ./... 全绿 + 5 个其他配置题不退化 |

每阶段 ship 后跑：
- `go test ./... + go vet + make`
- s3a + logtri_go + s1a 三个 eval（确保零退化 + 目标提升）

## 16. 对比 4 补丁方案

| 维度 | 4 补丁（A/B/C/D） | 本设计 |
|---|---|---|
| 解 schema-level 事实（类别 2-5） | 部分（A 微解类别 2） | 全解 ✓ |
| 解 Crossfile 契约（类别 3） | 不解 | ScopeCrossfile ✓ |
| 解 Negative 缺席（类别 4） | 不解 | ScopeNegative ✓ |
| LLM 协议升级 | 无 | 强 schema 校验 ✓ |
| 工程量 | ~400 LOC | ~3600 LOC |
| 触及核心数据结构 | 否 | 是（EvidenceItem） |
| 影响测试数 | 0-2 | 42 |
| 应对未来变种 | 漏 | 可外推 |
| 增量回滚 | 单点 | per-阶段（§15） |

## 17. 决策点（已锁，等用户复核）

| # | 决策 | 锁定值 |
|---|---|---|
| D1 | 走完整 5-scope 设计 | ✓ 5-scope |
| D2 | 分批 ship 节奏 | 本 session 内分阶段 ship，**不另开 session**；8 阶段连续推进 |
| D3 | LLM 协议 | **强制设字段**，不做系统兜底推断 |
| D4 | 向后兼容 | **不考虑**，老 EvidenceItem{} 构造点全部需 scope 字段；缺 scope = panic / reject |
| D5 | 工程量 | 接受 ~7 天（含测试 + 8 阶段独立 ship） |
| D6 | NegativeScope=struct_fields v1 是否含 | **含**（用 Go AST 增量解析；s3a 类问题需要） |
| D7 | Section grounder 语言覆盖 | v1：YAML + Go + JSON + TOML；其他语言（rust/swift/...）按需要后续追加 |
| D8 | 与 fbf5363 / 99e7779 关系 | **独立**，本设计不动 diagram seed fidelity 已 ship 路径 |

## 18. 决策（已锁，2026-05-01）

| # | 问题 | 锁定 |
|---|---|---|
| Q1 | s3a 退化先打 hotfix 还是直接走完整 ship | **直接走 8 阶段，无 hotfix**。中间阶段 eval 信号更真实 |
| Q2 | FileRoleLabel 是否支持自定义注册 | **v1 固定 5 枚举值**；视后续需要再开放 |
| Q3 | Section grounder lazy vs eager | **lazy + cache** |
| Q4 | parsedSections 缓存失效策略 | **file mtime 失效**；session 内不实时刷新 |
| Q5 | ScopeSection 是否计入 Tier-1 floor | **计入**（与 LineRange 同等，仍是 line-shaped） |
| Q6 | drift-bounded mode 是否需要 schema-level 投递 | **不需要**，drift 仅 ScopeLine / LineRange |
| Q7 | 旧 `[ABSENT]` 解析路径迁移 | **自动设 Scope=ScopeNegative + 默认 NegativeScope=File + 简单 NegativeQuery** |

---

**Status**: v1 已冻结。开始阶段 0。
