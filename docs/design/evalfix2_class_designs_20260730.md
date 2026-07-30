# EVALFIX-2 类级泛化设计（2026-07-30）

来源：EVALRUN-1 eval 批跑 gap 报告（eval/evalrun1_gap_analysis_20260730.md）的 8 个候办族，按用户裁定「解决一类问题而非单个 type 的拟合修复」归并为 5 个机制类。每类设计均满足：① 扩既有机制不发明新机制；② 精确 typed 信号驱动硬行为、噪声只作软引导；③ 硬门配 typed 逃逸；④ 5-Q 反过拟合自查（下一个同类实例零新机制代码可修）；⑤ 零答案质量代价。族→类映射：F5+F3→类1；F4+F10→类2；F7→类3；F11→类4；F8→类5（F9 按完成门权属模型裁定维持观察；真机 2 FAIL 词面定性属逐例审读，不在类内）。

状态：设计完成，待逐类裁定后开工。五类相互独立，可任意子集落地。


---

# CLASS 1 — 门拒提示教学 ↔ 初始 prompt 教学奇偶（Gate-Hint / Initial-Prompt Teaching Parity）

设计日期：2026-07-30。来源：`eval/evalrun1_gap_analysis_20260730.md` F5（2/2 复发）+ F3（2/12）。
本文是类级泛化设计，不是单实例补丁。

---

## 1. 问题类定义

**类**：一个硬门（emit-time hard reject / 质量门 / strict-decode 拒绝）的**修复提示（reject hint / retry hint）**向模型教学了一条规则——尤其是"如何满足该门的 typed 逃逸车道枚举"——而该 agent 的**初始 prompt 面**（skill Workflow / OutputFormat / tool schema description）要么完全没教这条规则，要么用漂移的词面教了一个**门谓词读不到的版本**。结果：每个触发该门的 run **结构性地**烧掉一整轮 LLM（被拒 → 靠 hint 学会 → 重发），复发不是随机抖动而是必然。

类的判别特征（三条同时成立即入类）：

1. 存在一个确定性硬门，其拒绝会把一段**指令性教学文本**（不只是"字段 X 非法"，而是"应当怎样做才能过门"）经某个 retry/repair 通道送回模型；
2. 该教学文本描述的规则/逃逸车道，在该 agent 首轮 prompt 的任何面上**不存在或词面漂移**（漂移=prompt 教的谓词与门实际执行的谓词不同源，模型照 prompt 做仍会被拒）；
3. 门本身行为正确（不放松门），浪费只发生在"第一次必然撞门"这一轮。

**本类的根因不是模型笨，是 prompt 歧义**（🔴 feedback：LLM misbehavior = prompt ambiguity）。而 prompt 歧义的根因是**双面教学文本手抄不同源**——与本仓已定谳的"五表手抄"病根（UXR-1 六维审计）、"machinery present, dispatch case missing"（LEDGER-TRIPWIRE / GAP-EVAL-D1 类）同构：两处需要一致的内容各自手写，无机械看护，必然漂移。

---

## 2. 类内已知实例

### 实例 F5 — write_analyzer 精确等值合同虚构（2/2 github_issue 例，各烧 ~20s 一轮）

- **门**：`internal/orchestrator/write_analysis_quality.go :: writeAnalysisIRQualityRejection`。
  谓词（精确、typed）：`Required && Polarity!=observed && Operator∈{equals,not_equals,contains,not_contains,exists,not_exists,raises,not_raises,returns}` 的合同，`Expected` 必须满足以下**任一 typed 逃逸车道**：
  - (a) `strings.Contains(raw_request, expected)` —— raw_request 逐字；
  - (b) `contract.EvidenceRef != ""` 或 `placement.EvidenceRef != ""`；
  - (c) `comparator.EvidenceRef != ""` 或 `comparator.Expected` 在 raw_request 逐字。
  最终轮兜底：`repairWriteAnalysisIRQuality` 软化为 `satisfies`（既有 typed 逃逸，保留不动）。
- **修复提示**（`internal/orchestrator/orchestrator.go:2985`，经 `SetAnalyzerRetryHint` → write_analyzer `BuildInitialInstruction` 的 "## Previous attempt rejected" 段）：**精确枚举了全部三条车道**，并明说"attach placement.evidence_ref or contract evidence_ref when the anchor/expected pair came from inspected evidence rather than raw_request"。
- **初始 prompt**（`internal/skill/defaults.go:1215`，write-analysis-skill Workflow）：教的是"use a value present verbatim in the request/**evidence**, attach grounded comparator evidence, or choose operator=satisfies"。
  **漂移点**：prompt 说"值在 evidence 里逐字出现即可"，但门**看不到模型的阅读史**——prescan 里从文件读到的值，除非附上 `evidence_ref`，门只能按 raw_request 判定。模型照 prompt 字面做（值确实来自 inspection evidence，但没附 evidence_ref）仍然被拒。非 placement 合同的 `evidence_ref` 车道在初始 prompt **只字未提**（1216 行只对 placement 合同教了 evidence_ref）。
  → 这是"词面漂移型"入类：prompt 教了规则的近似版本，hint 教的才是门谓词的真版本。

### 实例 F3 — emit_analysis 字段名近失（required_ ↔ requested_，2/12 例各烧一轮）

- **门**：strict-decode `DisallowUnknownFields`（`json: unknown field "requested_files"`）。
- **修复提示**：`internal/tool/strict_decode_remap.go` 的 remap + R4 sanitize + schema reminder；did-you-mean 一轮重试是**在案设计**（remap 红线：不改错误值）。
- **初始 prompt 缺口**：`internal/tool/emit_analysis.go` schema 同时存在 `required_files` 与庞大的 `requested_*` 家族（`requested_scope` / `requested_fields` / `requested_output` / `requested_verdict_options` / `requested_answer_dimensions`），两族语义相邻、词形近失；schema 无任何消歧教学，emit_analysis 的 `failStrictDecodeWithError` 调用点 hints 传 `nil`（无 typed 近失记录）。
  → 这是"记录缺席型"入类：已知复发的近失连 repair-time typed 记录（hint 表）都没有，prompt-time 消歧更无从谈起。
- 注意边界：报告已定性 did-you-mean 重试机制本身**不改**；本类只做 prompt/hint 表侧的教学前移，观测成本记录维持。

### 历史同类（佐证"类"而非"巧合"）

- 2026-06-13 P0 sweep 教训（memory 在案）："gate 先 make、**prompt 教学先认 dispatch 表面**" —— 同一 recurrence 形；
- s1a/u3a forensic（strict_decode_remap.go 文件头在案）：LLM 为 unknown-field 类错误烧 5-7 轮，当时的修法（MisplacedFieldHint）就是本类 repair-time 半边的既有先例——**prompt-time 半边至今无机械看护**；
- R2' 六处同步 checklist（🔴 feedback_typed_signal_six_spot_sync）：(2) schema description ↔ (3) skill prompt ↔ (4) retry hint ↔ (5) remap 表——**规定了同步义务但纯手动**，本类每个实例都是该 checklist 某两处之间的漂移。

---

## 3. 既有机制盘点（先找半建成的车道）

| 既有机制 | 位置 | 与本类的关系 |
|---|---|---|
| `AnalyzerRetryHint` 共享通道 | `types.MutableState`；写侧 `orchestrator.go:2984`，读侧 `agent/analyzer.go:1716`；消费 `write_analyzer.go:44` / `analyzer.go:342`（consume-once） | **教学回流的 typed 封闭通道**——凡经此通道的字符串，定义上都是"烧完一轮后的教学"，是本类 census 的天然锚点（精确信号：调用点集合封闭可枚举） |
| `MisplacedFieldHint` typed 表 | `internal/tool/strict_decode_remap.go`；per-tool 表（`answerDocumentV2MisplacedHints` 等 3 张） | repair-time 已知错形的 **typed 注册表先例**；缺 (i) wrong-NAME 形（现只有 wrong-container 形，其教学词面 "relocate the value (do not rename)" 对改名类是错误指令）；(ii) prompt-time 对面 |
| LEDGER-TRIPWIRE census+exemption 范式 | `internal/dataworkflow/ledger_completion_tripwire_test.go` | **全集 vs 分派 双向红 + exemption 带 rationale** 的结构 tripwire 范式，本设计逐字复用其骨架 |
| 全名册 prompt 捕获 harness | `internal/agent/prompt_snapshot_test.go :: allPromptSnapshotCaptures`（11 个 BuildInitialInstruction + BuildPromptContext + 动态 schema 投影） | 已存在的"渲染真实产线 prompt 全集"的测试基建——奇偶 tripwire 的 prompt 面直接骑它，不新造渲染路径 |
| skill 默认注册表 + defaults_test pins | `internal/skill/defaults.go` / `defaults_test.go` | 初始 prompt 教学的单一权威载体；`internal/skill` 只依赖 `internal/types`，被 tool/agent/orchestrator 全部可达——**单源常量的天然安放层** |
| 质量门 typed 逃逸 | `repairWriteAnalysisIRQuality`（软化为 satisfies + `Source` 标记 `quality_repaired:*`） | 门自身的逃逸车道已合规，本设计不触碰门行为 |
| prompt 卫生三测 | `TestNoInternalTermsInPrompts` / `TestPromptSnapshot_NoInternalTermsInRenderedOutput` / `TestRemapStrictDecodeError_Sanitize` | 新增教学文本的 ATOMIC checklist 自动化兜底，照常全跑 |

结论：**两个半边都已存在**（教学回流通道、typed 错形注册表、census 范式、prompt 捕获 harness），缺的只是把"同一段教学文本必须同时出现在门 hint 面与初始 prompt 面"这件事**单源化并机械看护**。这正是本仓纪律 (1)"扩展既有机制"的用武之地。

---

## 4. 泛化方案

### 4.1 机制核心：GateTeaching 单源教学注册表（SST）

新文件 `internal/skill/gate_teachings.go`（`internal/skill` 层：tool/agent/orchestrator 均可 import，无环）：

```go
// GateTeaching — 一个硬门的逃逸车道教学，单源。
// Text 是唯一权威：初始 prompt 面与门的修复提示面都必须
// 逐字包含它（由 tripwire 强制）。改教学 = 改这一处。
type GateTeaching struct {
    Key       string          // 稳定 id，如 "write_exact_contract_grounding"
    SkillName string          // 必须承载该教学的 skill（初始 prompt 面）
    Text      string          // LLM-facing 教学句块，过 ATOMIC 7 checklist
}

// 包级 var（非 map 字面量散写）——census 可 AST 枚举，引用点编译期绑定：
var GateTeachingWriteExactContractGrounding = GateTeaching{ ... }

func AllGateTeachings() []GateTeaching   // 全集（tripwire 的 universe）
```

关键设计点：

- **教学文本是常量，两面通过 Go 引用组装**（不是两处手抄同一句）。skill Workflow 行写成 `"...前缀。" + GateTeachingWriteExactContractGrounding.Text + " 后缀..."`；orchestrator 的 retry hint 同样拼接同一常量。奇偶从"靠测试对比两段手写文本"升级为"结构上只有一段文本"——tripwire 只需守住"两面都引用了它"。
- **Text 教的是门的真谓词**（typed 逃逸车道逐条枚举），不是规则的散文近似。F5 的 Text（示意，落地时逐条过 checklist）：

  > For hard operators (equals, not_equals, contains, not_contains, exists, not_exists, raises, not_raises, returns) on a required expected-behavior contract, the expected value must be verifiably grounded in one of three ways: (a) present verbatim in raw_request; (b) the contract (or its placement) carries evidence_ref naming where the value was observed, such as issue text or file:line; (c) a comparator is attached whose expected value is verbatim in raw_request or carries its own evidence_ref. A value you saw during repository inspection counts ONLY if you attach the evidence_ref — the validator checks the emitted fields, not your reading history. When none of these apply, use operator=satisfies (soft behavior text) instead.

### 4.2 Tripwire A — prompt 面覆盖（全集 vs 承载，双向红）

`internal/skill/gate_teaching_parity_test.go`，逐字复用 LEDGER-TRIPWIRE 骨架：

- 正向：对 `AllGateTeachings()` 每条，取 `DefaultRegistry().Get(SkillName)`，拼接 `Goal+Workflow+OutputFormat+Prohibitions`，断言 `strings.Contains(corpus, Text)`（**verbatim substring = 精确信号，允许作硬门**）。失败信息指名 Key 与 SkillName。
- 反向（防 stale）：exemption 表 `gateTeachingPromptExemptions map[Key]rationale`——某教学**故意**只存在于 repair 面（例如教了会 prime 错误行为的消歧、或 prompt 预算裁定不前移的教学）时在此落案；被豁免又实际出现在 prompt 面 = 红（豁免过期）。这是 tripwire 自身的 **typed 逃逸车道**（纪律 (3)）。

### 4.3 Tripwire B — hint 面引用 census（抓"下一个实例"的机关）

`internal/orchestrator/gate_teaching_hint_census_test.go`（AST census，ledger tripwire 同款 go/parser 扫描）：

- **通道全集**（封闭、typed、可枚举）：`SetAnalyzerRetryHint(` 的全部调用点（现状恰两处：`orchestrator.go` 写侧、`agent/analyzer.go` 读侧）＋后续按需纳入的质量门 hint 构造函数。通道集合本身写成表，加通道要过 review。
- **规则**：每个调用点所在函数体内，实参表达式必须（直接或经同函数局部变量）引用某个 `skill.GateTeaching*` var / `skill.AllGateTeachings`；否则该调用点必须出现在 `retryHintTeachingExemptions map["file:function"]rationale`。
- **效果**：下一个人给任何 agent 新写一个硬门＋裸的指令性 retry hint（本类的下一实例），这个测试**在他不写任何新测试代码的情况下变红**，逼迫二选一：注册 GateTeaching（Tripwire A 随即逼迫 prompt 面承载）或落 exemption 带 rationale（显式裁定"这条教学只配 repair-time"）。数据行驱动，机制零改。
- 现状读侧 `analyzer.go:1716`（一致性重试 hint）首落地时预计走 exemption 或顺手注册——census 先红后绿，红的形状就是 F5 的形状，判决力自证。

### 4.4 strict-decode 近失臂 — 扩展 MisplacedFieldHint 为双形，并配 prompt 面奇偶

扩展**既有** typed 表（不建第二注册表，遵守"no parallel taxonomy"）：

```go
type MisplacedFieldHint struct {
    Field          string
    ContainerNames []string   // wrong-container 形（既有）
    CorrectPaths   []string
    CanonicalName  string     // 新增：wrong-NAME 形。非空时 remap 词面走
                               // "the schema has no field %q — the field is named %q;
                               //  rename the key (keep the value unchanged)"
}
```

- remap 红线维持：**不改错误值**，只改 message 词面；改名形与搬家形词面分叉（既有搬家词面 "relocate ... do not rename" 对改名类是反向指令，必须分叉）。
- **Tripwire C**（`internal/tool/strict_decode_hint_parity_test.go`）：对每张 per-tool hint 表的每条 wrong-NAME 记录，断言该 tool 的 `Parameters()` schema JSON 中，`CanonicalName` 字段的 description **逐字包含** wrong token（如 "not requested_files"）——即 schema 自身预教消歧；或 entry 在 exemption 表落案（rationale 典型值："预教该错名会 prime 错误 token，裁定只 repair-time 教"）。
- 语义：hint 表 = "已知复发错形"的 typed 台账；台账每添一行，prompt 面消歧同步义务被机械化——这就是 R2' checklist (2)↔(5) 两处同步的自动化。

### 4.5 首批两个具体应用（类机制的第一次行权）

**应用 1（F5）**：
1. 注册 `GateTeachingWriteExactContractGrounding`（SkillName="write-analysis-skill"，Text 如 4.1）。
2. `defaults.go:1215` 该句中段替换为常量引用（前后文保留：合同何时发、comparator 何时附、satisfies 语义句维持——只把"逃逸车道枚举"这段换成单源）。
3. `orchestrator.go:2985` retry hint 重组为 `"Previous emit_write_analysis attempt was rejected: %v. Re-emit with the rejected fields corrected and all required fields filled (...). " + skill.GateTeachingWriteExactContractGrounding.Text + "（placement 补充句维持）"`。
4. 净效果：初始 prompt 首次教了非 placement 合同的 `evidence_ref` 车道与"阅读史不可见，必须落 typed 字段"这一门谓词真相——F5 的 2/2 撞门轮预期归零。

**应用 2（F3）**：
1. `emit_analysis` 的 decode 调用点（现传 `nil`）挂上 hint 表，首行：`{Field:"requested_files", CanonicalName:"required_files"}`（后续再犯的近失按台账逐行添）。
2. `emit_analysis.go` schema `required_files` 的 description 追加一短句消歧（示意）："Field name is required_files — the requested_* fields (requested_scope, requested_fields, ...) are separate display/profile settings, not this list."（逐字含 wrong token，Tripwire C 转绿）。
3. did-you-mean 一轮重试机制**不动**（在案设计）；观测成本预期 2/12 → 0/12。

---

## 5. 判定信号与红线合规

| 判定点 | 信号 | 精度归类 | 合规 |
|---|---|---|---|
| Tripwire A prompt 覆盖 | 单源常量的 verbatim `strings.Contains` | 精确（逐字子串） | ✅ 硬门允许 |
| Tripwire B 通道 census | `SetAnalyzerRetryHint` 调用点 AST 枚举 + `skill.GateTeaching*` 标识符引用 | 精确（typed 标识符、封闭通道表） | ✅ |
| Tripwire C schema 消歧 | hint 表 typed 记录 ↔ schema description verbatim token | 精确 | ✅ |
| 三个 tripwire 的逃逸 | exemption map + 非空 rationale，双向 stale 检查 | typed | ✅ 纪律 (3) |
| 不做的判定 | 字段名编辑距离/相似度自动判近失 | 嘈声 | ❌ 明确不做（嘈声不进硬门）；近失只按**已发生的复发**人工入台账 |
| 门行为 | `writeAnalysisIRQualityRejection` / repair 软化 / remap 不改值 | — | 全部不动（零行为变更；零答案质量成本） |

**ATOMIC 7 prompt checklist**（新教学文本逐条过）：
- R3：教学枚举的是 typed 车道（operator 枚举、evidence_ref 字段、comparator 结构），非启发式；
- R4：无 stage codename、无 Go 类型名（"the validator" 泛称；不提 orchestrator/quality gate 等内名；落地后跑 `TestNoInternalTermsInPrompts` + prompt snapshot 测试）；
- R5：不代写答案值，只述判定标准；
- R6：全抽象 placeholder（file:line / issue text），无真 case 值——跨 ≥3 类 write 请求（异常修复/输出改动/布局迁移）均适用；
- R7：替换 1215 行旧词面时 grep "verbatim in the request/evidence" 全 LLM-facing 面清剩余出现（含 `orchestrator.go:2985` 旧句），不留 stale；
- SST：教学句唯一权威在常量，两面引用；
- R2'：本设计未新增 typed signal 字段（`CanonicalName` 是 hint 表内部结构，非 LLM 出射 schema 字段），R2' 六处不触发；但 R2' 的 (2)(3)(4)(5) 同步义务由 Tripwire A/C 首次机械化。

---

## 6. 触点文件与实施步骤

| # | 文件（绝对路径） | 动作 |
|---|---|---|
| 1 | `/Users/han/opt/claude/codrax/internal/skill/gate_teachings.go` | 新增：GateTeaching 类型 + `GateTeachingWriteExactContractGrounding` + `AllGateTeachings()` |
| 2 | `/Users/han/opt/claude/codrax/internal/skill/gate_teaching_parity_test.go` | 新增 Tripwire A（含 exemption 表 + 双向 stale 检查） |
| 3 | `/Users/han/opt/claude/codrax/internal/skill/defaults.go` | write-analysis-skill Workflow 1215 行中段换常量引用（先跑红 Tripwire A 再改绿） |
| 4 | `/Users/han/opt/claude/codrax/internal/orchestrator/orchestrator.go` | 2985 行 retry hint 改为拼接同一常量 |
| 5 | `/Users/han/opt/claude/codrax/internal/orchestrator/gate_teaching_hint_census_test.go` | 新增 Tripwire B（AST census；`agent/analyzer.go:1716` 的既有 hint 落 exemption 或注册，rationale 落案） |
| 6 | `/Users/han/opt/claude/codrax/internal/tool/strict_decode_remap.go` | `MisplacedFieldHint` 加 `CanonicalName` + 改名形 remap 词面分叉（不改错误值） |
| 7 | `/Users/han/opt/claude/codrax/internal/tool/emit_analysis.go` | decode 调用点挂 hint 表（requested_files→required_files）；`required_files` schema description 加消歧句 |
| 8 | `/Users/han/opt/claude/codrax/internal/tool/strict_decode_hint_parity_test.go` | 新增 Tripwire C（wrong-NAME 记录 ↔ schema 消歧奇偶 + exemption） |
| 9 | `/Users/han/opt/claude/codrax/internal/tool/strict_decode_remap_test.go` | 改名形词面新 pin（含 R4 sanitize 复验） |

实施顺序（先红后绿纪律，红检禁 `git checkout` 复原）：#1 注册表（Text 空引用）→ #2 Tripwire A **确认红**（prompt 未承载）→ #3/#4 双面接线 → A 绿 → #5 Tripwire B（确认对 2985 旧写法会红的形状，exemption 落案 analyzer.go 点位或顺手注册）→ #6-#9 F3 臂同法先红后绿 → make + 全量 go test + prompt 卫生三测。

---

## 7. 测试与判决性验证

**结构判决（静态，先红后绿各留证）**：
- Tripwire A 在 #3 落地前必须红（红文案 = "teaching write_exact_contract_grounding not carried by skill write-analysis-skill"）——证明它对 F5 现状有判别力，非 vacuous；
- Tripwire B 对"裸指令性 retry hint"的合成用例红（在 test 内对 census 函数喂一个虚构调用点 fixture，或以落地前的 2985 现状留红证）；
- Tripwire C 在 #7 schema 消歧句落地前红；
- 三个 tripwire 各带 vacuous 防护（universe 空 = Fatal，ledger tripwire 同款）。

**行为判决（端到端，过程指标非答案 bar）**：
- github_issue 双例重跑：日志 grep `write_analyze attempt 1/2 failed` / `under-grounded WriteAnalysisIR` 计数 **2/2 → 0/2**；eval 判定维持 PASS（零答案代价——门未动，答案路径唯一变化是少一轮弯路）；
- 通用 12 例重跑：`json: unknown field "requested_` 命中 **2/12 → 0/12**；12 例 PASS 集合不变；
- L1 面：`TestRunMode_ReadByteIdentical` + read e2e 回归照跑（emit_analysis schema description 变更属读模式 LLM 面，read eval 全量须绿；schema 消歧句不含行为语义，预期零漂移）；
- 卫生面：`TestNoInternalTermsInPrompts`、`TestPromptSnapshot_NoInternalTermsInRenderedOutput`、`TestRemapStrictDecodeError_Sanitize` 全绿。

**判决力自查**：唯一行为变更（模型首轮就带 evidence_ref / 正确字段名）必须有正向证据——不只看"拒绝轮消失"，还 grep 通过例的 IR：F5 例的 exact 合同应出现非空 `evidence_ref` 或 raw_request 逐字 expected（而非全被软化成 satisfies——若模型改为全发 satisfies，教学过强，需回调词面，这一点列入验证清单）。

---

## 8. 5-Q 反过拟合自查

1. **下一个实例（新 agent 的新硬门配裸 retry hint）零新代码能被抓吗？** 能。Tripwire B 的通道 census 对任何 `SetAnalyzerRetryHint` 新调用点静态变红；修法是**添数据行**（注册 GateTeaching 或 exemption），机制代码零改。F3 族的下一个近失同理：hint 表添一行，Tripwire C 自动接管 schema 奇偶义务。
2. **机制是否绑定 write_analyzer / emit_analysis 这两个点位？** 否。GateTeaching 以 `SkillName` 泛化到任意 agent；census 的通道表覆盖读写两个 analyzer 的既有共享通道，且通道表可扩（加通道=加一行）。两处首批应用只是行权，不是机制边界。
3. **是否依赖具体词面（改一处教学词面会不会静默漂移）？** 否。教学文本单源常量，两面是 Go 引用——词面改动物理上同时生效于两面；Tripwire A 防"某面把引用改回手抄"。
4. **教学文本本身是否 case-specific？** 否。全部为抽象车道枚举（operator 枚举来自 typed 全集、evidence_ref/file:line 为 placeholder），对异常类/输出类/布局类/不变量类 write 请求同样适用；无任何 eval 例的真值。
5. **门谓词将来扩臂（新逃逸车道）时要同步几处？** 两处：谓词代码 + 单源 Text。Tripwire A/B 保证 Text 到达两面；若担心 Text 与谓词代码本身漂移，operator 列表可后续从 `types.IsKnownWriteBehaviorOperator` 的枚举单源生成进 Text（列为可选强化，不在本批——避免首批过度工程）。

结论：不是 per-SHAPE 修（不是"给这两条 prompt 各补一句"），是 per-CLASS 修（"教学双面必须单源+机械奇偶"），下一实例的成本 = 添数据行。

---

## 9. 风险与不做什么

**不做**：
- ❌ 不把全仓 `errResult`/violation `Repair` 字符串强制纳入注册表——那会产生数百条 exemption 噪音（违反"噪音从源头消除"）。类边界收在**跨轮教学回流通道**（烧一整轮的地方）+ **已知复发台账**（hint 表）；纯 schema 回声型拒绝（"值 X 非法，合法枚举为…"——schema 本身已列枚举，奇偶天然成立）不入类。
- ❌ 不做字段名相似度/编辑距离的自动近失检测（嘈声信号禁作硬门）；近失只按实际复发逐行入 typed 台账。
- ❌ 不放松、不改动任何门的判定与逃逸行为（`writeAnalysisIRQualityRejection`、repair 软化、remap 不改值全部原样）——零答案质量成本的前提。
- ❌ 不把所有 wrong token 预教进所有 schema——只教台账在案的复发项，且 Tripwire C 留 exemption 给"预教会 priming 反效果"的裁定。
- ❌ 不新建与 MisplacedFieldHint 并行的第二张错形注册表（no parallel taxonomy）。

**风险与对策**：
- *教学过强 → 模型防御性全发 satisfies，写侧验证判别力下降*：验证清单含正向证据检查（§7 末段）；Text 明示 satisfies 是"车道都不可用时"的次选。
- *单源常量被未来编辑拆回手抄*：Tripwire A 的 verbatim containment 即刻红。
- *census 的 AST 扫描脆弱（重构改函数名）*：ledger tripwire 同款风险，同款对策——扫描失败即 Fatal（vacuous 防护），逼迫同步维护。
- *read-mode 面被 emit_analysis schema 句触碰*：L1 行为等价 pin + read eval 全量兜底；消歧句为纯命名说明，无行为语义。
- *exemption 表变垃圾场*：反向 stale 检查 + rationale 非空强制（ledger tripwire 同款），且首批 exemption 预算 ≤2 条（analyzer.go:1716 一条视处置而定）。

---

## 10. 落地偏离（EVALFIX-2A 实施记录，2026-07-30）

按 §6 顺序落地，三个 tripwire 均以落地前现状留了真红证（A：teaching 未承载；B：`orchestrator.go:runWriteAnalyzePhase` 裸 hint；C：`required_files` description 未预教 wrong token）。与设计字面的偏离：

1. **`DefaultRegistry()` 不存在**（§4.2 引用）——skill 包实际 API 是 `NewRegistry()` + `RegisterDefaults(r)`；Tripwire A 照既有 defaults_test 惯例取 corpus（`allWorkflowBodies`/`allProhibitionBodies` 双 tier 助手）。
2. **Repair 通道同步分叉**（§4.4 只写了 remap message 分叉）——`strictDecodeToolRepair` 的 hint-match 臂对 wrong-NAME 行原样返回 "Relocate the value ... Do not rename"，正是设计点名的反向指令；故同一判据下分叉出 `tool_param_misnamed_field`（Fields=[CanonicalName]，Hint=改名保值）。Code 是既有开放字符串词汇（同族 `tool_param_unknown_field` / `write_plan_repair_pack`），下游只作 opaque ReasonCode 透传，非 R2' typed signal 新增。
3. **Tripwire B census 范围**——设计说"通道集合本身写成表"；落地形为 WalkDir 全 internal/ 非测试文件扫 `SetAnalyzerRetryHint` 调用点（比文件表更抓"下一个实例"：新文件新调用点无需先登记即被看护），引用判定按设计原文取函数体作用域（"直接或经同函数局部变量"的机械化形）。首批 exemption 恰 1 条：`agent/analyzer.go:ParseOutput`（读侧 hint 为逐次动态 gate Detail 组装，无固定句可单源，rationale 已落表）。
4. **defaults.go:1215 satisfies 语义句维持**——Text 尾句已含 "(soft behavior text)"，与保留的 "operator=satisfies is soft behavior guidance" 句有轻微冗余；按 §4.5 "satisfies 语义句维持" 字面保留，未删。
5. **Tripwire C 附加 vacuous 防护**——wrong-NAME 行数为 0 时 Fatal（设计 §7 只要求 universe 空 Fatal）；最后一行退役时须连同该 Fatal 一起退役，失败文案已说明。

行为判决（§7 端到端 eval 重跑：F5 2/2→0/2、F3 2/12→0/12、正向 evidence_ref 出现率）属 eval 批跑，不在本静态批内，留待下轮 EVALRUN 验证。

状态：已实施（EVALFIX-2B，2026-07-30；落地偏离见本节 §10）。对应 eval gap 报告 `eval/evalrun1_gap_analysis_20260730.md` F4 + F10。

---

## 1. 问题类定义

**类：对"纯确定性子计算"按"新随机工作"的方式重复派发。**

一个子计算是"纯"的，当其输出是 (工件字节内容, 归一化后有效参数) 的确定性函数——同 run 内重复执行必然产出逐字节相同的结果。系统当前把两种纯计算当成有新信息量的随机工作重复付费：

- **形态 A（重复执行）**：同一 run 内，模型对同一工件用完全相同的有效参数重复调用确定性工具（trace_query 是 artifact+params 的纯函数，已有 DET-1 确定性 pin 钉死），每次都全额重跑索引构建 + 视图计算 + blob 落盘。浪费墙钟/token 之外还有一个**结构性下游伤害**：每次调用铸出**新的 blob ref**（`StoreBlobArtifact` 每次生成新文件），而 observation ledger 的既有去重 `dedupeObservationRecords` 的 merge key **含 SourceRef**（`observation_ledger.go:626-651`）——refs 不同 ⇒ key 不同 ⇒ 语义上完全相同的观察逐条存活 ⇒ 显示层出现 [E1][E2] 重复行，与图例的合并承诺矛盾。即：**重复执行不只是浪费，它主动击穿了本已存在的账本去重机制**。
- **形态 B（退化段派发）**：分段两步车道（perf_triage / log_triage 的 Step B）对每个"可提取 kind"的段无条件派发一整轮 LLM，即使段只有 100 字节——一个 100 字节尾段的"摘要"是确定性的（没有可提取的帧/卡顿/启动信息），却独占一轮 LLM 调用。stage 入口已有 `MinBytes` 准入地板（"低于该尺寸的 trace 很少有提取价值"，perf 默认 200 / log 默认 50），**同一语义在段粒度缺席**——这是同一准入判据在两个粒度上的谓词分叉，与 EVALFIX-1 三个已修根因同族。

两形态的统一机理：**派发准入层缺少"这个子计算是否是纯的/退化的"的精确判定**，于是确定性工作被按随机工作计价。

### 类边界（诚实声明纯性）

- 纯性只对 **(工件指纹, 有效参数)** 声明：工件指纹 = 解析后路径 + os.Stat(size, mtime)；有效参数 = strict-decode + 继承/适配（request-model target、logical artifact path）**之后**的参数结构体的规范化 re-marshal。
- trace_query 中**消费 run 期可变注册表的分支不在纯边界内**（auto-window / recipe discovery / heavy-view guard / stream 车道会咨询随 run 增长的 call-window 注册表，两次同参调用可能选出不同窗口）——这些分支明确排除在 memo 之外。
- 失败结果（Success=false）不在纯边界内（重试语义必须真实重跑）。
- memo 生命周期 = **per-task**，与 `traceQueryPublishedBlobRefs` 同边界（`ResetTurnAArtifacts` 清空，context.go:4449），跨 task 不复用（RequestModel/objective 换了，caveat 生成的输入就换了）。

---

## 2. 类内已知实例

| # | 实例 | 证据 | 形态 |
|---|------|------|------|
| 1 | F4：state_churn 例同窗口 4 次相同 `root_cause_rank` + 3 次同参 `window_stats` | eval gap 报告第 16 行 | A |
| 2 | F4 伴生：重复观察行 [E1][E2] 与图例合并承诺矛盾 | 同上；根因见 §1（refs 击穿 merge key） | A 下游 |
| 3 | F10：perf_triage 100 字节尾段独占一轮 LLM | eval gap 报告第 19 行 | B |
| 4 | log_triage Step B 同构缺口（`log_triager.go` ~429 段循环只有 kind 门无尺寸门）——**尚未在 eval 中被抓到但静态可审出**（静态审出的 gap 不留实测兜底红线） | 本设计代码核验 | B |
| 5 | F10 伴生：42.5s 零工具散文轮 | eval gap 报告第 19 行 | 非本类（模型行为 ⇒ prompt 歧义，见 §9） |

---

## 3. 既有机制盘点（先找半建成的车道）

| 机制 | 位置 | 与本设计的关系 |
|------|------|--------------|
| **MutableState run-scoped 注册表族** | `types/context.go`：`traceQueryPublishedBlobRefs`（map、fork 时 clone、merge 时并回、ResetTurnAArtifacts 清空）、`RecordTraceQueryCallWindow`、supplement 一次性闩 | **memo 存储直接复刻这个成熟范式**：同样的 mutex 保护、同样的 fork/merge 语义、同样的 per-task 清空边界。零新生命周期概念。 |
| **Tool 接口的 embeddable mixin 惯例** | `tool/tool.go`：ReadOnly/WriteCapable、EvidenceTool/NavigationTool | 能力声明的既有表达方式；memo 不需要新 mixin（见 §4 决策：钩子放工具内部而非 Registry 层）。 |
| **DET-1 确定性 pin** | `trace_query_det1_determinism_test.go` | **纯性前提已被测试钉死**："identical input must produce identical typed observations across passes"。memo 只是把这个已被证明的性质变现，不新增任何正确性主张。 |
| **ledger 去重** | `types/observation_ledger.go:604 dedupeObservationRecords` + `observationRecordMergeKeyFor`（key 含 SourceRef） | **已存在且正确**——它只是被逐次新铸的 blob ref 击穿。memo 命中返回逐字相同 refs ⇒ merge key 相同 ⇒ 既有去重零改动生效。**不改 merge key**（见 §5 红线）。 |
| **投影层合并臂** | `answer_document_mutation_runtime_tree.go` engine arm C（MergedTwinMirrorRef 等） | 兜底显示层已有；本设计让重复行根本不产生，兜底臂保持现状不动。 |
| **stage 入口 MinBytes 准入地板** | `perf_triager.go:214`（默认 200）/ `log_triager.go:301`（默认 50），配置 `perf_triage_min_bytes` / `log_triage_min_bytes` | **形态 B 的半建成车道**：同一判据（"低于该尺寸无提取价值"）扩到段粒度即可，**零新 knob**。 |
| **段 kind 准入门** | `IsExtractablePerfSegment` / `isExtractableSegment` | 段循环里既有的准入判定点；尺寸地板加在同一点，同为精确信号。 |
| **supplement 系统车道旗标** | `systemTraceSupplementInProgress`（typed bool） | memo 的保守排除臂直接读它（精确信号已存在）。 |
| **strict-decode + params 归一化** | `trace_query.go:256-265` + `applyStructuredPayloadCompat` | key 规范化的输入端已存在：memo key 从 strict-decode 之后的结构体 re-marshal，天然吸收字段顺序/空白噪声。 |

结论：**本类不需要任何全新子系统**——三个半建成车道（run-scoped 注册表范式、ledger 去重、MinBytes 地板）各补最后一段即闭环。

---

## 4. 泛化方案

### 4.1 通用层：run-scoped 纯工具 memo（形态 A）

**新文件 `internal/tool/pure_memo.go`** + `types/context.go` 上的存储：

```go
// types/context.go — MutableState 新增（复刻 traceQueryPublishedBlobRefs 范式）
toolResultMemo map[string]ToolResult   // key = "<tool>\x00<memoKey>"

func (m *MutableState) ToolResultMemo(tool, key string) (ToolResult, bool)
func (m *MutableState) StoreToolResultMemo(tool, key string, r ToolResult) // 仅 Success=true 可存
```

- **Fork/Merge**：`ForkForExploreDispatch` clone map（fork 读得到父的命中）；`MergeExploreFork` 并回 union、先写者胜（memo 是经济性优化不是正确性契约，并发 fork 各算一次可接受）。
- **清空**：`ResetTurnAArtifacts` 置 nil（与 blob-ref 注册表同一行政区）。
- **通用助手**（tool 包）：

```go
// RunPureToolMemo 在诚实纯边界处包住纯核心。miss ⇒ 执行 compute 并存储；
// hit ⇒ 返回存储结果逐字副本 + 追加一行模型面披露。
func RunPureToolMemo(ctx *types.BusContext, toolName, memoKey string,
    compute func() (types.ToolResult, error)) (types.ToolResult, error)
```

命中时对返回副本做且仅做两件事：
1. Summary 尾部追加一行披露（模型面软引导）：`[note: identical <tool> call (same artifact + same effective params) already executed in this task; result reused verbatim — vary parameters to gather new information instead of re-issuing the same query]`。
2. 置 typed 字段 `ToolResult.ReusedFromRunMemo = true`（审计/pin 面精确信号；新增 typed signal 走 R2' 六处同步 checklist）。

**Observations / RawRef / PayloadRef / Refinement / 各 authority 字段逐字保留**——refs 相同正是修复账本去重的机制本体。Observations 的 ObservedAt 保留首次时间（它就是该计算发生的时间，披露行已说明复用）。

**钩子位置决策：工具内部，不在 Registry.Execute。** 理由：trace_query.Execute 在纯核心之前有 run-scoped 副作用（`traceQueryRecordExplicitRuntimeTarget`、`traceQueryRecordCallWindow`——后者注释明言"duplicates are kept, call order is the deterministic tiebreak"，SUPP-CORE 的窗口选举语义依赖重复登记）。Registry 层截断会静默改变 supplement 窗口选举输入，那是借经济性优化之名动第二个子系统的语义。工具内部钩子让副作用照常发生、只跳过纯核心——**memo 命中与未命中对所有 run-scoped 注册表逐字节等效**。

### 4.2 trace_query 接线（实例 1/2 的根修）

在 `trace_query.go:389`（`buildStart := time.Now()` 处，即 auto-window/discovery/guard/stream 四个早退分支**之后**、索引构建之前）插入：

```go
if key, ok := traceQueryMemoKey(ctx, p, path); ok {
    return RunPureToolMemo(ctx, t.Name(), key, func() (types.ToolResult, error) {
        // 现有 buildIndex → Run → marshal → StoreBlob → ToolResult 主体原样搬入
    })
}
// ok=false ⇒ 原路径直跑（typed 逃逸缺省臂）
```

`traceQueryMemoKey` 返回 `(key, ok)`，**ok=false 的车道（全部为精确信号）**：
- `ctx.Mutable.SystemTraceSupplementInProgress()`（系统 supplement 车道保守排除，保持其结果面字节独立）；
- os.Stat 失败（无法指纹化工件 ⇒ 不主张纯性）；
- 配置 kill switch 关闭。

key 组成：`sha256(canonical-json(p 有效参数结构体) + "\x00" + path + "\x00" + sourceLabel + "\x00" + stat.Size + "\x00" + stat.ModTime.UnixNano())`。p 取**继承与适配全部完成后**的值（trace_query.go:327/329 之后），窗口归一化的 caveat 一并进入结果被 memo，无分叉。

### 4.3 段粒度 LLM 派发准入地板（形态 B，实例 3/4 的根修）

`perf_triager.go` runTwoStep 段循环（:323-335）与 `log_triager.go` 同构循环（~429），在既有 kind 门的同一位置补第二个精确谓词：

```go
if !tool.IsExtractablePerfSegment(s.Kind) { continue }
if s.ByteEnd-s.ByteStart < a.settings.MinBytes {   // 复用 stage 入口同一 knob、同一语义
    skippedDegenerate++
    logging.Info("[perf_triage] two-step: segment %s [%d:%d] below min_bytes=%d — degenerate, no LLM dispatch",
        s.Kind, s.ByteStart, s.ByteEnd, a.settings.MinBytes)
    continue
}
```

- **零新 knob**：`perf_triage_min_bytes` / `log_triage_min_bytes` 的文档语义（"低于该尺寸很少有提取价值"）在段粒度逐字成立；调它同时调两个粒度是操作员直觉的正确形。
- 退化段的"确定性摘要"就是"跳过 + 披露"：一个 100 字节段对 PerfBundle 无可贡献成员，coverage 账目本来就按 merged bundle vs rawBytes 诚实计算，跳过段自然落入 residue，不伪造覆盖。
- **披露**（F8 披露文化对齐）：StageReport 追加一句 `skipped N degenerate segments (< min_bytes)`（N>0 时），软信息不进 LLM prompt。

### 4.4 账本/显示面（实例 2）：零新代码

memo 命中 ⇒ 第二次调用的 Observations 携带与首次**逐字相同**的 SourceRef ⇒ `observationRecordMergeKeyFor` 产出相同 key ⇒ `dedupeObservationRecords` 既有合并逻辑吞并 ⇒ 证据索引单 E# 行。**不改 merge key、不加显示层去重臂**——修的是击穿既有机制的源头（"噪音从源头消除"红线的字面应用）。投影层 engine arm C 兜底臂保持不动，继续覆盖"参数不同但结果语义重合"的非本类形态。

### 4.5 配置

`codrax.yaml` 新增指针型 knob（`internal/config/runtime.go` pipeline 组）：

```yaml
pipeline_pure_tool_memo_enabled: true   # 缺省 true；false = 操作员 kill switch
```

无 CLI override（与既有精简 CLI 面一致）。

---

## 5. 判定信号与红线合规

| 判定 | 信号 | 精确性 | 红线合规 |
|------|------|--------|---------|
| memo 命中 | sha256(规范化有效参数 + 路径 + sourceLabel + stat 指纹) 字节相等 | 精确（哈希等值） | 硬行为（跳过执行）配精确信号 ✓ |
| memo 准入排除 | SystemTraceSupplementInProgress typed bool / stat error / config bool | 精确 | 每条硬门配 typed 逃逸车道（§1.6）✓ |
| 段派发地板 | `ByteEnd-ByteStart < MinBytes` 单整数比较 | 精确 | 同上；逃逸 = 操作员调 min_bytes ✓ |
| 模型重复行为矫正 | Summary 披露行 | 噪声面（散文） | 只作软引导，不设硬拒 ✓ |
| ReusedFromRunMemo | 新 typed bool 字段 | 精确 | 走 R2' 六处同步 checklist |

逐条红线核对：
- **因果 token 红线**（`causal_token_registry.go` 单一真源）：**零触碰**——memo 返回的是引擎既有产出的逐字副本，无 token 语义车道移动、无措辞改动。
- **§29.21 证明车道**：memo 复用的观察仍是同一确定性工具见证（同一引擎运行、同一 refs），`CurrentSourceSatisfied` 的"确定性工具见证"前提不被稀释——没有任何模型断言被铸成 current_source 级观察。
- **观察身份**：merge key（`observationRecordMergeKeyFor`）**一个字段都不动**；本设计通过让 refs 相同来让既有身份判定自然生效，而非放宽身份。
- **完成门权属**：不涉及。
- **用户意图优先/系统不硬卡**：memo 从不拒绝任何新计算——任何参数变化 = 新 key = 真实执行；命中返回的是**真答案**而非拒绝，对模型无任何行为门。
- **L1 读模式字节等价**：memo 状态在 MutableState 内、read/write 两模式行为一致，不引入模式分叉；`TestRunMode_ReadByteIdentical` 不受影响（write 机器缺席时行为不变）。
- **prompt 红线**：本设计主体零 prompt 改动；披露行是 tool result 面（与既有 [note: …] 追加惯例同形，如 agent.go prescan note），不是 skill prompt。伴生的散文轮 nudge（§9）若做，单独走 prompt 红线 checklist。

---

## 6. 触点文件与实施步骤

1. **`/Users/han/opt/claude/codrax/internal/types/context.go`**
   - MutableState 新增 `toolResultMemo map[string]ToolResult` + `ToolResultMemo` / `StoreToolResultMemo`（mutex 同锁）；
   - `ForkForExploreDispatch`（:1188）clone；`MergeExploreFork`（:1271）union 并回（先写者胜）；
   - `ResetTurnAArtifacts`（:4449）置 nil。
2. **`/Users/han/opt/claude/codrax/internal/types/`** ToolResult 新增 `ReusedFromRunMemo bool`（R2' 六处同步：类型定义/序列化/投影拷贝/观测面/文档/pin）。
3. **新文件 `/Users/han/opt/claude/codrax/internal/tool/pure_memo.go`**：`RunPureToolMemo` 助手 + 披露行常量 + 包级 enable 开关（`SetPureToolMemoEnabled`，与 `SetLintEnabled` 同形）。
4. **`/Users/han/opt/claude/codrax/internal/tool/trace_query.go`**：`traceQueryMemoKey`（新函数，~40 行）+ :389 处包裹纯核心（主体代码原样移入闭包，diff 最小化）。
5. **`/Users/han/opt/claude/codrax/internal/agent/perf_triager.go`**（:323 循环）与 **`/Users/han/opt/claude/codrax/internal/agent/log_triager.go`**（~429 循环）：段尺寸地板 + skip 计数 + StageReport 披露句。
6. **`/Users/han/opt/claude/codrax/internal/config/runtime.go`**：`PipelinePureToolMemoEnabled *bool` + `cmd/root.go` 接线到 `tool.SetPureToolMemoEnabled`。
7. **`docs/architecture.md`**：§10 knob 表 + 工具层一段（memo 纯边界声明与排除臂）。

实施顺序：2（typed 字段）→ 1（存储）→ 3（助手）→ 4（trace_query 接线）→ 5（段地板）→ 6/7（配置/文档）。每步 `make` + 相关包测试绿后进下一步。

---

## 7. 测试与判决性验证

**新 pin（每条先红后绿或带 MUTATION 自检说明）：**

1. `TestTraceQueryRunMemo_IdenticalCallReusesVerbatimRefs`：同一 BusContext 上同参两次 Execute ⇒ 第二次 `ReusedFromRunMemo=true`、PayloadRef/RawRef 与首次逐字相等、Summary 含披露行；MUTATION：去掉 memo 查询则第二次 refs 必不同（StoreBlobArtifact 每次新文件）——判决性。
2. `TestTraceQueryRunMemo_ParamVariationExecutesFresh`：任一参数字段变化 ⇒ 不命中（refs 不同、无披露行）。
3. `TestTraceQueryRunMemo_SideEffectRegistriesIdentical`：命中调用后 `TraceQueryCallWindows()` 长度照增、cursor 登记照常——证明 4.1 的"副作用逐字节等效"主张。
4. `TestTraceQueryRunMemo_ExclusionLanes`：supplement in-progress / stat 失败 / knob=false 三臂均直跑不 memo。
5. `TestTraceQueryRunMemo_ClearedAtTurnBoundary`：ResetTurnAArtifacts 后同参调用真实重跑。
6. `TestObservationLedger_DuplicateQueryCollapsesToOneRecord`（实例 2 判决）：两次同参 trace_query 的 Observations 进 `CompileObservationLedger` ⇒ 单条记录——**只要 refs 相同这在今天的 dedupe 代码上就该绿**，作为"零新去重代码"主张的判决性证据。
7. `TestPerfTriageTwoStep_DegenerateSegmentSkipsLLMDispatch` / `TestLogTriageTwoStep_...`：构造 <MinBytes 的 extractable-kind 段 ⇒ base.Execute 不被调用（fake 计数）、StageReport 含 skipped 句；≥MinBytes 段照常派发。
8. 既有回归面：`trace_query_det1_determinism_test.go`（纯性前提）、`read_e2e_regression_test.go` + `TestRunMode_ReadByteIdentical`（L1）、observation ledger 全套、`go test ./...` 全绿。

**端到端判决**：state_churn eval 例重跑——trace_query 实际执行次数 7→≤4（memo 命中日志计数）、证据索引重复 [E1][E2] 行消失、答案 bar 不降（零答案代价红线：eval 判词全维持）。F10 例重跑：段派发轮数减一、总墙钟下降、PerfBundle 内容不变（100 字节段本就无贡献成员）。

---

## 8. 5-Q 反过拟合自查

1. **下一个实例零新代码？** 形态 A：任何 view、任何窗口、任何 trace 工件的同参重复——同一 memo 覆盖，零新代码。形态 B：log_triage 同构缺口在本批一并修（不是等下次 eval 抓）；未来任何段 kind 的退化段同样被地板拦下，零新代码。实例 2 类（重复观察行）：任何经 memo 的工具其重复观察自动被既有 ledger 去重吞并，零显示层新代码。
2. **是否绑定特定 view/字段/文案？** 否。memo key 是参数结构体整体规范化哈希，不枚举字段；段地板是字节长度比较，不看 kind 文案。
3. **下一个纯工具的接入成本？** 实现一个 `xxxMemoKey`（声明自己的诚实纯边界）+ 一处 `RunPureToolMemo` 包裹，通用层（存储/生命周期/fork 语义/披露/开关）零改动。未选 Registry 层全自动包裹是**有意的诚实性取舍**（§4.1 副作用论证），不是泛化不足：纯边界必须由懂自己副作用的工具自己声明，框架代声明就是把噪声信号当硬门。
4. **修的是机理还是症状？** 机理：三个半建成机制（run-scoped 注册表、ledger 身份去重、MinBytes 准入）各接通最后一段；[E1][E2] 显示症状不加任何显示层代码而消失，是机理修复的判决性副产品。
5. **会不会反向过拟合（memo 过宽伤正确性）？** 纯边界收窄到已被 DET-1 pin 证明确定性的主路径；四个早退分支、失败结果、supplement 车道、跨 task 全部排除；kill switch 兜底。命中返回真答案，无任何行为被拒——正确性上界 = 现状。

---

## 9. 风险与不做什么

**风险：**
- memo map 内存：per-task 生命周期 + 只存 Success 结果（Summary 是有界 preview，重负载已在 blob 文件里），量级为每 task 数十条，可忽略。
- 并发 fork 重复计算：接受（经济性优化不做跨 fork 协调锁，避免把优化变成同步瓶颈）。
- 段地板误伤真有信息的小段：MinBytes 语义在 stage 粒度已运行长期无投诉；操作员可调；披露句保证可审计。

**不做什么（明确出界）：**
1. **不做跨 run / 落盘持久化 memo**——工件指纹与 run 生命周期外的失效语义是另一个类（baseline cache 已占该生态位），本设计只解决 run 内重复。
2. **不改 `observationRecordMergeKeyFor` / 因果 token 注册表 / 投影层合并臂**——观察身份一个字节不动（§29.21 / 因果 token 红线）。
3. **不在 Registry.Execute 做全自动 memo**——副作用语义论证见 §4.1；对 read_file/grep 等工具不主动接入（read 模式下近纯，但 write 模式 worktree 内容会变，FileReadCoverageStore 的时间语义裁定已在永久 reject 列表——不碰）。
4. **不给模型设"重复查询硬拒"门**——重复是 LLM 行为，硬拒违反"噪声信号不作硬门"与用户意图红线；只做披露行软引导 + memo 让重复变得免费。
5. **42.5s 零工具散文轮不在本类**：那是合法派发内的模型漫游 ⇒ prompt 歧义（perf-triage-skill 教学"直接调用 emit，不要叙述"），若做走 prompt 红线 checklist 单独立件，本设计不夹带 prompt 改动。
6. **不动 supplement 窗口选举 / SUPP-CORE 任何语义**——call-window 注册表在 memo 命中路径照常登记，字节等效有 pin（测试 3）。

---

## 10. 落地偏离（EVALFIX-2B 实施记录，2026-07-30）

按 §6 顺序落地（2→1→3→4→5→6/7），`make` + 全量 `go test ./...` 绿。与设计字面的偏离：

1. **§1 "每次调用铸出新的 blob ref" 前提部分失实**——实测 `StoreBlob` / `StoreBlobArtifact` 的文件名是**内容寻址**的（`<tool>-<sha256(payload)[:4]>`），且 `tracequery.Result` 全字段确定性（无墙钟/遥测字段），故同参重复调用今天就恰好铸出**相同** refs。memo 的价值不变（省掉全额索引重建 + view 运行的墙钟/内存），且把 ref 字节相等从"内容哈希碰巧"升级为"逐字副本的结构保证"；但 §7.1 的 MUTATION 主张（"去掉 memo 则第二次 refs 必不同"）按现状不可成立——pin 1 的判别器改用 typed `ReusedFromRunMemo` + 披露行（kill-switch 臂即 pre-change 行为的活体自检），refs 相等断言保留为账本去重前提 pin。[E1][E2] 重复行的显示层根因由此存疑（refs 相同时 `dedupeObservationRecords` 本就该吞并——见测试 6 今天即绿），留待 eval 重跑复核，不在本批扩面。
2. **TraceSecond 指纹显式并入 key**——§4.2 的 "canonical-json(p 有效参数结构体)" 对 `TraceSecond`（零导出字段、无 MarshalJSON）marshal 为 `{}`，会把时间窗从 key 中抹掉（两个不同窗口同 key = 正确性事故）。落地形：key = sha256(re-marshal(p) + TimeStart/TimeEnd 各自的 (Set, Raw, Float64bits(Seconds)) 指纹 + path + sourceLabel + stat(size, mtime))；unit/fractionDigits/scale 均为 Raw 的纯函数、stringEncoded 全仓无读者，故该三元组是完整诚实指纹。未给 TraceSecond 加 MarshalJSON（会改动全部潜在序列化面，超出本批授权）。
3. **`traceQueryMemoKey` 签名含 sourceLabel**——§6.4 草图为 `(ctx, p, path)`；§4.2 的 key 组成本就列了 sourceLabel，按后者实现。
4. **测试 7 红先证**——以真突变跑留证：临时失效 perf 段地板 ⇒ pin 观测到 3 次 LLM 调用（期望 2）而红，恢复后字节恒等（diff 校验）复绿；log 通道同构。仓内不保留红态，记录于此。
5. **R2' 六处同步的适用面**——`ReusedFromRunMemo` 是系统侧产字段（非 LLM emit schema 字段），tool-schema/skill-prompt/retry-hint 三臂结构性不存在；同步落为：类型定义 + json 序列化 tag + 投影拷贝核验（ToolResult 全仓均为整结构体拷贝，无逐字段投影点；memo 命中路径为整值副本）+ 观测面（命中 INFO 日志行 `[pure_memo] …`）+ 文档（architecture.md §7.2.1 段 + §10 knob 表）+ pin（trace_query_pure_memo_test.go / tool_result_memo_test.go）。
6. **stat 失败排除臂的测试形**——成功走完 Execute 的调用其 path 必然可 stat，全路径不可达；该臂以 `traceQueryMemoKey` 直测钉死（不存在文件 ⇒ ok=false），并配健康臂正例防 vacuous。
7. **段地板披露句形**——落地为 `…; skipped N degenerate segments (< min_bytes=<值>)`（带地板值，比 §4.3 草图多披露一个数），追加于两步车道全部三个出口 StageReport（merged 成功 / 零 partial 降级 / merge nil 降级）。
8. **新增第五条排除臂：run context 已死**——全量跑抓出与既裁 SUPP-CANCEL 暖索引契约的碰撞（`TestTraceQueryCancelModelLaneWarmIndexTypedPartial`：暖过 cache 后 context 取消，同参重呼必须返回 typed cancellation partial、零 faces）；memo 命中会把该重呼复活成成功结果，违反在案裁定。修法为 `traceQueryMemoKey` 加精确排除臂 `contextFromBus(ctx).Err() != nil`（取消/超时统一拒入 memo，查存两向都不参与），取消语义全权留给既有车道；配 key 侧直测臂 + 既有全路径 pin 双看护。

行为判决（§7 端到端：state_churn 执行次数 7→≤4、[E1][E2] 行消失、F10 段派发轮数减一）属 eval 批跑，不在本静态批内，留待下轮 EVALRUN 验证（含偏离 1 的显示层根因复核）。

---

# CLASS 3 设计：机械可判定的答案自相矛盾 —— 确定性断言方向校验（mechanical claim check）

状态：设计稿（未实施）。来源：EVALRUN-1 gap 分析 F7（`eval/evalrun1_gap_analysis_20260730.md` 第 17 行）。
定位：finalize 合同校验层新增一条 **soft-only** 校验 + 一个**可扩展的机械断言校验器注册表**。

---

## 1. 问题类定义

**类**：答案正文中，真值可以**仅凭算术**判定的断言，以自相矛盾的形态出厂。

形式化：正文一句话内出现 `⟨数量A⟩ ⟨比较词⟩ ⟨数量B⟩`（含否定形），其中 A、B 均带显式单位且同量纲。该断言的真假不需要任何证据面、任何语义理解——把两个数按单位归一后做一次比较即可判定。当句面宣称的方向与算术方向相反时，就是本类缺陷。

关键性质（区别于既有 PSG/L4 覆盖面）：

- **不需要证据池**。PSG（prose_scalar_grounding）回答的问题是"这个数是否在证据面发布过"；本类回答的问题是"这句话自己跟自己是否矛盾"。一个两侧数值都完美 grounded 的句子仍然可以方向写反（F7 正是如此：80ms 与 16.67ms 都是真值）。
- **判定是精确的，提取是嘈杂的**。算术比较本身零噪声；但"哪个数、哪个比较词、否定辖域在哪"来自正则提取，是嘈杂信号。按红线，嘈杂信号只能驱动软引导——所以本设计**永远 soft，永不 hard**。
- **类的外延不止方向词**。同一"算术可判"性质还覆盖：百分比合计与声明总和不符、正文声明的计数与所列条目数不符、`A+B=C` 三数俱陈而不等。这些是**同类的下一批家族**，本设计的注册表形态就是为它们准备的落点。

## 2. 类内已知实例

1. **F7（本批触发件）**：blocked_reason 例，正文两次渲染「80ms 未超过 16.67ms 预算」。真值方向为 80 > 16.67（约 4.8×），句面宣称 ≤。L4 self_consistency reviewer（LLM 审阅）未拦——BODY-vs-evidence / BODY 自算术是其已知盲区（gap 报告同条维持观察，logtri_go nil-receiver 叙事同盲区）。
2. **同形历史件（PSG-2 立案背景中的近亲）**：cmp_78_01 曾出现「窗长差假象」类叙述把 2250ms 窗的量绑到 1800ms 窗名下——PSG-2 绑定臂管"值属于哪个窗"，但**不管**"两个窗长谁大谁小"的句面方向断言。
3. **潜在同类（未立案、机制上同判定性）**：
   - 百分比家族：「A 占 63%、B 占 45%」而上下文宣称两者是同一窗口的互斥分量（Σ>100%）；或「合计 92%」与逐项相加不符。
   - 计数家族：正文「共 5 处调用点」而同块枚举列出 3 项。
   - 和式家族：「running 26.9ms + runnable 3.6ms 合计 76.5ms」。
   （注意：wallclock conservation 检查已覆盖"调度态分和超窗"这一**特定语义**的守恒，但它绑定 target_window_states 证据族，不是通用算术判定。）

## 3. 既有机制盘点（先找半建成的车道）

本仓已存在一个成建制的 `prose_*` 确定性正文校验家族，本设计**全部复用其基础设施**，不新建平行体系：

| 既有件 | 位置 | 复用什么 |
|---|---|---|
| PSG 标量提取器 `extractProseScalarTokens` + `proseScalarTokenRE` | `internal/orchestrator/prose_scalar_grounding_check.go:1761` | token 形状（Raw/Unit/Pos/Value/Ulp/Approx）、千分位/标识符左邻护栏、`proseScalarApproxMarked` 约值标记。本设计需要扩单位面（见 §4.2），以**新正则 + 同护栏逻辑**扩展，不动 PSG 自己的两单位正则（其 §25 裁定范围是 ms/% 两族，不得顺手改动）。 |
| 句切分 `proseSentenceSpans` | `prose_wallclock_conservation_check.go:701`（已被 headline_elim / fact_juxtaposition 等共享） | 句内绑定的作用域单元。 |
| 正文单元收集 `collectModelProseUnits` / `collectModelProseBoardUnits` + `proseScalarScanExemptBlock` | `prose_lexicon_board_check.go:299,445` | 扫描面 = 模型作者正文块；系统证据块、caveat、diagram、next_steps 结构性豁免（与 PSG 同一豁免面，天然规避把系统渲染表格当正文误扫）。 |
| soft violation + 一轮闩模式 | `runProseScalarGroundingCheck` 的 one-round latch（part1/part2，`MutableState.MarkProseScalarGroundingHintDelivered`，`internal/types/context.go:5681`） | 反活锁：每 run 至多一次 hint 轮，闩只在"派发面已携带该 kind"时落下。照抄该模式，新开本 lane 自己的闩字段。 |
| 出厂残留披露 chokepoint | `appendProseScalarResidualCaveatToAnswer`（`orchestrator.go:6528/6757` 两处调用）+ `appendRegisteredAnswerCaveatBullet`（CAVSTR 去重注册器） | 消耗完 hint 轮后仍矛盾 → 系统通道一行披露，正文永不被系统改写。 |
| ViolationKind 注册表 | `internal/types/violation_registry.go`（`ViolProseScalarUngrounded` 条目为模板，:1171）+ `violation.go` soft kinds 名册（:1107 附近） | 新 kind 一条 `RegisterViolKind` + soft 名册一行；操作员可经 `pipeline_contract_strict_kinds` yaml 升等（既有 typed 逃逸面）。 |
| 合同检查布线 | `runContractCheck`（`contract_check.go:140`）内 `trace.run("name", fn)` 模式 | 一处布线调用。 |
| 容差纪律 | wallclock 检查的具名 ε + 具名绝对地板（「容差常量禁跨语义借用」已两次立案） | 本 lane 铸自己的具名常量，绝不借 `proseScalarApproxRelTolerance` 等他 lane 常量。 |

**明确不碰**：L4 `runSelfConsistencyReviewV2`（LLM 审阅通道）——本设计是它盲区的确定性补位，不改其 prompt、不改其覆盖面；PSG 的两单位正则与其证据池 scope 门（`HasDeterministicRuntimeQueryObservation`）——本 lane 无证据池概念，scope 与 PSG 各自独立。

## 4. 泛化方案

### 4.1 总体形态：一个注册表，N 个家族扫描器，一个共享装配壳

新文件 `internal/orchestrator/mechanical_claim_check.go`：

```go
// mechanicalClaimFinding：一条已被算术证实的矛盾（provable=算术层已复核，
// 仅提取层残余噪声）。entry/entryZH 双语，与 prose_* 家族同款双面。
type mechanicalClaimFinding struct {
    entry, entryZH string
    blockID        string
    decisive       bool // 矛盾幅度达到"判决级"档（见 §4.4），残留披露只收此档
}

// mechanicalClaimChecker：一个"算术可判断言家族"的扫描器。
// Scan 只做提取+算术判定，装配/闩/披露/上限全在共享壳里。
type mechanicalClaimChecker struct {
    Name string // "numeric_direction"；未来 "percent_sum" / "count_vs_list" / "sum_identity"
    Scan func(unit proseTextUnit, spans [][2]int) []mechanicalClaimFinding
}

// mechanicalClaimCheckers：家族注册表。下一个家族 = 此表加一行 + 一个 Scan 函数。
var mechanicalClaimCheckers = []mechanicalClaimChecker{
    {Name: "numeric_direction", Scan: scanNumericDirectionClaims},
}
```

共享壳 `runMechanicalClaimCheck(doc, mut, lang)`：

1. 闩 part1/part2（照抄 PSG 两段式，新字段 `MechanicalClaimHintDelivered`）。
2. `collectModelProseUnits(doc)` → 每 unit 求 `proseSentenceSpans` 一次 → 依次喂给注册表全部 checker（工作量上限：每 run 扫描 finding 总数 cap 8、token cap 200，具名常量）。
3. 所有家族的 findings **合并为一条** soft violation（与 PSG 同纪律：整 lane 每 run 轮预算恰好一轮），kind = `ViolMechanicalClaimContradiction`（`"mechanical_claim_contradiction"`）。
4. Repair hint（要点，全文过 prompt 红线 checklist 后落地）：逐条列出「块 id + 句面片段 + 两数归一后的算术方向」，指令为——逐条核对：若比较方向词写反则改方向词；若某个数值本身写错则以证据面发布值改正；两者都对而是转述口径问题则重写该句消除歧义。**最小编辑**：只改点名块，其余块逐字节保持（prefer `emit_answer_document_patch`，与 PSG hint 同款收尾）。不注入任何内部管线词汇。
5. 出厂残留披露：`appendMechanicalClaimResidualCaveatToAnswer`，挂在 `orchestrator.go` 既有两处 PSG 披露调用点旁边（同 chokepoint、同 CAVSTR 注册器去重），激活条件 = 本 lane 闩已落 ∧ 对 shipped doc 重扫仍有 `decisive` 档 finding。文案（zh 例）：「以下断言中两个数值的比较方向与其数值本身不一致，请谨慎采信：…」。

### 4.2 首个家族：numeric_direction 扫描器

**量纲与单位归一**（typed，闭集）：

```go
type claimDimension int // dimTime | dimPercent
// 单位表（闭集，一处定义）：
//   时间 → ms 系数：us/µs/μs/微秒=0.001, ms/毫秒=1, s/秒=1000
//   百分比：%/％（无系数）
// 仅同量纲可比；time vs percent 结构性不比。min/h 等暂不入集（见 §9）。
```

新正则 `mechanicalClaimQuantityRE`（数值紧跟单位；沿用 PSG 的左邻护栏——前一字节为字母/数字/`.-_,` 则弃 token，防千分位截读与标识符尾数；`ms`/`s` 右邻字母则弃，防 `5spec`、`K3s` 类误读；`s` 单位额外要求右邻非字母数字）。Approx 标记复用 `proseScalarApproxMarked` 同逻辑（约/≈/~/approximately 前缀窗口）。

**比较词词表**（闭集 + 方向 + 否定翻转；一处定义，zh/en 双语）：

| 声称关系 | zh 基词 | en 基词 |
|---|---|---|
| A > B | 超过 / 超出 / 高于 / 大于 / 多于 / 长于 | exceeds/exceeded/exceeding, greater than, more than, higher than, longer than, above |
| A ≤ B | 低于 / 小于 / 少于 / 短于 / 不足 / 之内 / 以内（前接"在"或窗名） | less than, lower than, below, shorter than, within, no more than, at most |
| 否定前缀（翻转基词方向为其补） | 未 / 不 / 没有 / 并未 / 并不（紧邻基词，≤4 rune 窗口） | not / never / no longer / does not / doesn't / did not（紧邻，≤3 词窗口） |

注意刻意**不收**的词：快于/慢于（时长语义反转歧义）、over/under（英语介词噪声）、多/少 单字（无比较结构）。F7 击中路径：`未`+`超过` → 声称 A ≤ B。

**句内绑定**（保守，全部具名常量）：

- 作用域 = 一个 sentence span。比较词在句内定位后：A = 比较词**左侧最近**的同量纲 token，B = **右侧最近**的同量纲 token；两侧各设 rune 距离缰绳（`mechanicalClaimBindGapRunes = 30`，仿 wallclock 的 keyword-gap 模式）。任一侧缺失、超缰绳 → 该比较词弃判。
- **两侧必须都带显式单位**且同量纲——这是最强的结构性护栏：「超过 3 次」「不应超过 16.67ms」（单数）「如果超过预算」（零数）全部天然不触发。

**跳过条件（保守触发集——precision first，逐条为具名谓词，均单测钉死）**：

1. **规范性/假设句**：比较词缰绳区间内出现情态/条件词（应 / 不应 / 应该 / 需 / 须 / 必须 / 建议 / 如果 / 若 / 假设 / 一旦 / should / must / would / could / may / might / if / unless / expected to）→ 整句弃判。预算规则陈述（"帧预算要求不超过 16.67ms"）不是事实断言。
2. **疑问句**：span 含 `？`/`?` → 弃判。
3. **区间值**：候选 token 紧邻区间标点（`–—~～-..` 连接两数值，仿 `proseScalarWindowSpanRE` 的区间形）→ 该 token 不作比较端（"10–20ms 超过 5ms" 的 10 不单独参与）。
4. **近值/舍入**：归一后 `|vA−vB| ≤ max(ulpA, ulpB)` 或相对差 < 判定余量 → 弃判（"17ms 未超过 16.67ms" 不报——宁松勿严）。
5. **约值**：任一侧 Approx 标记 → 改用约值判定余量（更宽档，见 §4.4）。
6. **同句多比较词**：一句内比较词 >1 个 → 每个比较词独立绑定各自最近 token；同一 token 被两个比较词竞争时弃判（绑定歧义）。
7. **零值**：任一侧 value==0 → 弃判（沿用 PSG"零值不携带测量主张"裁定）。
8. **块类豁免**：系统证据块 / caveat / diagram / next_steps（复用 `proseScalarScanExemptBlock` + `collectModelProseUnits` 现成豁免，零新代码）。

### 4.3 判定

归一到同量纲数值 vA、vB，声称关系 R ∈ {GT, LE}（GE/LT 由否定翻转归并入这两档的补）：

- 声称 GT 而 `vA ≤ vB` 且幅度达标 → 矛盾。
- 声称 LE 而 `vA > vB` 且幅度达标 → 矛盾。

### 4.4 幅度档（本 lane 自铸具名常量，禁借用）

```go
mechanicalClaimDirectionRelMargin       = 0.10 // 普通档：反向幅度须超 10%
mechanicalClaimApproxDirectionRelMargin = 0.25 // 任一侧约值标记时
mechanicalClaimDecisiveRatio            = 2.0  // 判决级档：vA/vB 或 vB/vA ≥ 2×
// 绝对地板（防微小数值的相对幅度虚高）：
mechanicalClaimTimeFloorMS   = 0.5  // 时间量纲
mechanicalClaimPercentFloorP = 0.5  // 百分比量纲（百分点）
```

- **hint 轮**收普通档及以上全部 findings；
- **出厂残留披露**只收 `decisive` 档（≥2×，F7 为 4.8×）——对用户可见面再加一层精度保险。

## 5. 判定信号与红线合规

| 红线 | 合规方式 |
|---|---|
| 精确信号 hard / 嘈杂信号 soft | 算术判定精确，但**提取**（token、比较词、否定辖域、绑定）是嘈杂信号 → 整 lane 永远 soft：`SoftByDefault: true`，`DefaultSeverity: SeveritySoft`，`Promotable: true`（操作员显式升等才到 Medium）。永不 emit-time hard reject，永不改写正文。 |
| 每个硬门配 typed 逃逸 | 本 lane 无硬门。软层的逃逸面：①一轮闩（结构性反活锁，轮预算恰一）；②`pipeline_contract_strict_kinds` / soft kinds yaml 双向 typed 调档（既有 `SetSoftViolationKinds` 面，零新代码）；③hint 文案明示"句子本意正确时改写方向词或口径即可"，不强迫删数。 |
| LLM 出错=prompt 歧义 | F7 属生成期否定滑笔，非 prompt 歧义——**不加初始 prompt 教学**（避免向全部 run 征税）。唯一新增 LLM-facing 文本是 repair hint 与残留 caveat，落地前过 prompt 红线 checklist（ATOMIC 7 条；无内部管线词汇；CN 分支纯 CN）。 |
| 零答案代价 | 正确答案（算术为真 / 触发跳过条件）零影响、零字节变化；假阳性最坏成本 = 一次有界软重试 + （仅 decisive 档误判时）一行谨慎披露；轮预算由闩硬性 ≤1。 |
| 容差常量禁跨语义借用 | §4.4 全部自铸具名常量。 |
| 噪音从源头消除 / 谓词同源 | hint 轮判定器与出厂重扫判定器是**同一个** `Scan`（同一注册表、同一常量），不存在两套识别器分叉（EVALFIX-1 Gap B 的教训反面）。 |
| R2' 六处同步 | 本设计不新增 emit 工具 schema 字段，不触 R2' 面；新增的是 ViolationKind（走 violation registry 自己的两处名册纪律）与 MutableState 闩字段（仿 PSG 既有形）。 |

## 6. 触点文件与实施步骤

1. **新文件** `internal/orchestrator/mechanical_claim_check.go`
   - 注册表 + 共享壳 `runMechanicalClaimCheck`；
   - `scanNumericDirectionClaims`（单位表、比较词表、跳过谓词、判定）；
   - `appendMechanicalClaimResidualCaveatToAnswer` + `mechanicalClaimResidualCaveatMessage(lang, findings)`；
   - 闩辅助 `retryStateListsMechanicalClaimHint`。
2. `internal/types/violation.go` — 新 kind 常量 `ViolMechanicalClaimContradiction ViolationKind = "mechanical_claim_contradiction"` + 加入 `defaultSoftKinds` 对应名册（violation.go :1107 附近的 soft 名册列表）。
3. `internal/types/violation_registry.go` — `RegisterViolKind` 条目（模板：`ViolProseScalarUngrounded` :1171；差异：`DefaultSeverity: SeveritySoft`、`Layer: "answer_oracle"`、`RepairPhase: RepairPhaseConsistency`、`FixableByAgents: [AgentFinalizer]`、无 CaveatFamilyID——残留披露走本 lane 自己的确定性 caveat，与 PSG 同理由注释）。
4. `internal/types/context.go` — `MechanicalClaimHintDelivered` 闩字段 + `Mark…`/getter（仿 :5681 PSG 对）。
5. `internal/orchestrator/contract_check.go` — `runContractCheck` 内、`prose_lexicon_board` 调用之后加一段：
   ```go
   result.Violations = append(result.Violations,
       trace.run("mechanical_claim", func() []types.Violation {
           return runMechanicalClaimCheck(docV2, mut, o.busCtx.Language)
       })...)
   ```
   **不带** `HasDeterministicRuntimeQueryObservation` 门——本类是全 run 通用（这正是与 PSG 的 scope 分界；F7 恰发生在 trace 例但类不限于 trace）。
6. `internal/orchestrator/orchestrator.go` — :6528 / :6757 两处 PSG 披露调用旁各加一行本 lane 披露调用（同 chokepoint，CAVSTR 注册器天然防双写）。
7. 顺序：先落 types（kind+注册表+闩）→ 再落检查器与测试（红→绿）→ 最后布线 + e2e pin（唯一行为变更的发布接线必配正向 pin——2026-07-20 窗教训）。

## 7. 测试与判决性验证

新 `internal/orchestrator/mechanical_claim_check_test.go`：

1. **F7 见证钉**：块文本含「80ms 未超过 16.67ms 预算」→ 恰一条 violation，kind 正确，detail 含两数与方向；en 形 "80ms does not exceed the 16.67ms budget" 同判。**先红后绿**（先写测试对空实现红）。
2. **方向正确句零触发**：「80ms 超过 16.67ms 预算」「16.67ms 未超过 80ms」→ 0 findings（防写反判定符号——判决性正例）。
3. **跳过条件逐条负例**（每条一测，共 ≥10）：规范句（不应超过）、条件句（如果超过）、疑问句、区间值、近值（17 vs 16.67）、约值窄幅、单侧无单位（超过 3 次）、量纲不比（80% vs 16.67ms）、零值、多比较词歧义、系统证据块内同句不扫。
4. **单位归一**：「0.08s 未超过 16.67ms」→ 触发（80ms>16.67ms）；「80µs 未超过 16.67ms」→ 不触发（方向为真）。
5. **闩一次性**：同 run 二次调用（模拟 retry surface 携带 kind → part1 落闩；重建 surface 无 kind → part2 静默）——照抄 PSG 闩测试形。
6. **残留披露**：闩落 + shipped doc 仍含 decisive 矛盾 → caveat 恰一次（CAVSTR 去重）；非 decisive（1.1×）残留 → hint 有、caveat 无（两档分离判决性测试）。
7. **注册表可扩展性钉**：测试用注入一个 fake checker 走共享壳，验证合并为单 violation、cap、双语渲染——证明"下一家族=一表项"的承诺是被测的，不是注释里的。
8. **软性钉**：registry 断言该 kind 默认 soft、Promotable；`hasAnyStrictViolation` 对纯本 kind 集合为 false。
9. **合规验证**：`make` 绿 + `go test ./internal/orchestrator/ ./internal/types/` 绿 + L1 read 字节恒等测试不受影响（本检查在 read 主路径上，需跑 `TestRunMode_ReadByteIdentical` 确认——检查只添加 violation，不改答案字节，预期天然通过）+ 冒烟 eval 必须含 blocked_reason 场景族（补记卅七教训：选例须覆盖改动面场景族）。

## 8. 5-Q 反过拟合自查

1. **下一个实例零新代码能修吗？** 同家族（任意 `数A 比较词 数B` 方向矛盾，zh/en，us/ms/s/%，含否定形）——零新代码。F7 的"预算"一词完全不在触发逻辑里，换成「阈值/上限/基线」或无名词均同判。✔
2. **下一个家族呢？** percent_sum / count_vs_list / sum_identity = 注册表一行 + 一个 Scan 函数；kind、闩、hint 装配、披露、yaml 调档、测试壳全复用。不是零代码，但是"一表项不一子系统"，且该扩展形本身被 §7.7 测试钉住。✔（类的定义在"机械可判"，不在"方向词"。）
3. **是否绑死在触发样本的表面特征上？** 未用 16.67/80、未用"预算"、未用 blocked_reason 场景、未加 trace-run scope 门；比较词表是语言学闭集而非样本词。✔
4. **是否复用而非再造？** token 护栏、句切分、扫描面豁免、闩模式、caveat chokepoint、kind 注册、yaml 逃逸——七件全复用既有机制；新造的只有比较词表、单位归一、注册表壳（此前仓内不存在）。✔
5. **修的是机制还是症状？** 症状=F7 那两句话；机制=「模型算术自矛盾无确定性拦截层」。交付物是拦截层 + 家族扩展点；F7 只是首个 witness fixture。✔

## 9. 风险与不做什么

**风险与对策**

- **假阳性烧软重试**：最大风险。对策已内建：双侧带单位硬结构条件、情态/条件跳过、幅度余量 + 绝对地板、绑定缰绳、歧义弃判、decisive 两档、闩 ≤1 轮。验收标准：对既有 eval 全量 PASS 例重跑，本 kind 触发数必须为 0（真阴性基线判决性验证）。
- **比较词表遗漏（假阴性）**：宁松勿严的 sanctioned 成本。漏词补一行词表即可，属家族内零结构变更。
- **hint 引发过度改写**：hint 已限定最小编辑 + patch 优先 + 点名块（沿 PSG 已验证的收尾话术形）。

**明确不做**

1. **不做 hard gate**：即便 4.8× 的判决级矛盾也不硬拦——提取层噪声不配硬门（红线），decisive 档只用于披露分层。
2. **不动 L4**：不给 LLM reviewer 加算术指令、不改其 prompt；确定性层与 LLM 层各守其位。
3. **不动 PSG**：不扩 `proseScalarTokenRE` 单位面（其 §25 scope 有裁定）、不共享其闩、不搭其 violation kind。
4. **不做求解器**：不处理跨句推理、代词回指、隐式单位（"80 比 16.67 大"无单位不判）、min/h 单位（词面噪声高、暂无 witness，入集需新裁定）、乘法/比率断言（"约 5 倍"）。类边界=单句、双显式同量纲单位、闭集比较词。
5. **不做初始 prompt 教学**：向全 run 征 prompt 税去防一个低频生成滑笔不成比例；hint 轮就是教学面。
6. **不新增 CLI/yaml 开关**：既有 strict/soft kinds yaml 已是本 kind 的操作员面，另开开关即谓词分叉。

## 10. 落地偏离（EVALFIX-2C 实施记录，2026-07-30）

状态：已实施（`internal/orchestrator/mechanical_claim_check.go` + `_test.go`；types 三件 kind/registry/闩；contract_check 布线 + orchestrator 两处披露挂点；先红后绿——F7 见证钉先对空 Scan 实现跑红）。与 §4–§7 逐条对照，偏离如下：

1. **en 词表补 bare `exceed`**：§4.2 词表只列 exceeds/exceeded/exceeding，但 §7.1 钉住的 en 见证形 "80ms does not exceed the 16.67ms budget" 用的是原形 `exceed`（否定式接原形）——词表按 §7.1 的判决性测试补入 `exceed`（ASCII 词界匹配下 `exceeds` 不会被 `exceed` 截读，双保险）。属 spec 内部不一致，按测试钉裁决。
2. **之内/以内 只实现「在」臂**：§4.2 的准入条件为「前接"在"或窗名」；实施为缰绳窗（30 rune）内出现「在」才准入，「窗名」臂未实现（窗名无闭集定义，宁缺勿滥——纯 recall 成本，precision 无损）。补窗名臂 = 该谓词一处扩展。
3. **token cap 作用域**：§4.1 写「每 run …token cap 200」；实施为**每 text unit** cap 200（`Scan(unit, spans)` 的注册表签名被 §7.7 测试钉死，扫描器无 run 级累加器可读）。run 级工作量仍由 finding cap 8 + unit 数封顶。
4. **zh 否定窗不跨标点**：≤4 rune 紧邻窗实施为遇标点即止（「与预期不同，超过 16.67ms」中 不同 的「不」不得被读作否定翻转——真实假阳形，紧邻语义的实现细节收紧，方向=更保守）。
5. **共享壳 lang 参数暂不进 Detail**：violation Detail 维持 prose_* 家族纪律（en LLM/log 面；句面片段原文引用自带语言）；zh 面走本 lane 自己的 decisive 披露 caveat（zh 分支纯 CN）。签名按 §6.5 保留 lang 透传。
6. **§7.9 冒烟 eval / 真阴性基线未在本窗执行**：需 LLM 凭证与长跑批；本窗交付确定性验证全绿（make + go test ./... 83 包 0 FAIL + `TestRunMode_ReadByteIdentical` 绿）。blocked_reason 场景族冒烟与「既有 eval 全量 PASS 例本 kind 触发数=0」基线留给下一次 eval 批跑一并验收。

---

# CLASS 4 — 账本条目数据血统校验（ledger-entry data lineage）设计

状态：设计稿（未实施）。覆盖 eval gap F11（`eval/evalrun1_gap_analysis_20260730.md`）及其问题类。

---

## 1. 问题类定义

**类：零判别力校验 —— 账本记录的 KEY/VALUE 不追溯到任何观测数据，却通过全部形状校验与对账。**

codrax 数据车道的账本体系（`ContributionRecord` / `ReconcileGroup` / `EntityResolutionRecord` / `RowDecision`，`internal/dataquery/dataquery.go`）有两个正交命名空间：

- **schema 命名空间**：字段名（`group_key`、`metric`、`canonical_label`、`value`……以及各自的 alias 注册表）；
- **数据值命名空间**：来自输入文件真实行的观测值（`recordField` 读出的内容）。

现有校验体系（`validateContributionRecords` @ dataquery.go:2326、`validateReconcileReport` @ dataquery.go:2447、`validateComputeContributionFieldContract` @ action_runner.go:8426）只做**内部一致性**：贡献记录有锚点有效果、reconcile 组的 expected/actual 与贡献 Σ 相等、组集双向覆盖。**没有任何一层问："这个 group key 是从数据里来的吗？"** 于是一个 schema 命名空间的 token（字段名）泄漏进数据值命名空间当组键使用时，两侧账本自洽地携带同一个幽灵键，对账 100% 通过——校验对该类错误判别力为零。

类谓词（一句话）：**账本条目中处于"值位置"的键，其血统必须落在一条 typed 溯源车道上（观测字段值组合 / 显式声明的常量标签 / 合成 "all"），否则形状校验的通过是空洞的。**

关键点：血统字段**早已存在**——`ContributionRecord` 携带 `Source` / `SourceLocator` / `EvidenceRefs`（dataquery.go:1040-1052），runner 铸造时逐行填充（action_runner.go:8218-8230）。缺的不是字段，是**对 GroupKey 血统的判定与执法**。

## 2. 类内已知实例

1. **F11 本例（`data_multifile_reference_projection`）**：模型下发 `compute_contributions` 带 `group_key: "canonical_label"`（意图=按该字段分组）。runner 的字段匹配便利逻辑（action_runner.go:8103）是**逐输入**判定：输入记录里存在同名字段→改判为 field 分组；不存在→字段名**静默降级为常量组标签**。结果：47 行全部落入字面组 `"canonical_label"`，count=47；reconcile 组同样报 `group="canonical_label" expected=actual=47`，自洽 → 通过。零判别力。
2. **同类近亲（逐输入分叉）**：多文件输入下 A 文件有该字段、B 文件没有 → A 按字段值分组、B 全落入字面常量组——**同一个 action 的组键语义在输入间分叉**，产出的组集混入幽灵组，reconcile 仍自洽通过。
3. **脚本车道同类**：`add_contribution(group_key="metric", ...)` 之类脚本直接铸造的记录，GroupKey 携带 schema token；`validateContributionRecords` 只查"有锚点有效果"，照过。
4. **可预见的下一实例**：`group_key="canonical_id"` / `"source_locator"` / `"status"`（任何 canonical schema 名或其 alias），或 reconcile 组手写幽灵键（后者今天已被 `reconcile_group_mismatch` 拦——前提是贡献侧没有同名幽灵；本设计把贡献侧的铸造口封死后该前提成立）。
5. **历史已修的同类反面**（先例，见 §3）：`{"id":[...]}` 答案形状里组标签合法地等于字段名 → §15.12 批乙为此开了 `group_key_literal` 显式常量车道。说明本类早已被认识到一半：**逃逸车道已建，执法门从未建**。

## 3. 既有机制盘点（先找半成品车道）

| 机制 | 位置 | 与本设计的关系 |
|---|---|---|
| `group_key_literal` 显式常量通道 | action_runner.go:8044-8053；dataworkflow/scaffold.go:1004-1010（§15.12 批乙） | **现成的 typed 逃逸车道**：值"永远是常量标签、绝不重释为字段名"。本设计直接复用为硬门的逃逸口，零新逃逸机制。 |
| 逐输入字段匹配便利逻辑 | action_runner.go:8103-8107 | 本类的**直接病灶**：判定粒度错误（per-input 而非 per-action），且不存在时静默降级为常量。要改造为全局单次判定。 |
| `missingComputeContributionFields` 多输入跳过车道 | action_runner.go:8113/8482 | 字段解释确定后，缺字段输入已有 typed 跳过车道（`contribution_source_skipped`），复用。 |
| `DataFieldContractError` Role=`contribution_group_key` + 恢复计划 | action_runner.go:8266-8281；dataworkflow/missing_field_recovery.go:135+（`MissingComputeGroupFieldFallbackPlan`） | **现成的 typed 修复路径**：缺组字段 → 确定性生成 enrich/join fallback 计划。新违规码接入同一识别器族。 |
| alias 注册表（`rawAliasString` 各调用点） | dataquery.go:1073/1258 等 | canonical schema 名 + alias 的**闭集来源**——闭集判定的单一出处。 |
| `ContractWarnings` + `result_prompt_view` | dataquery.go:954/2006；dataworkflow/result_prompt_view.go:34/89 | **现成的软引导车道**：脚本车道的血统告警走这里，模型下一轮可见。 |
| EVALFIX-1 Gap B 谓词同源先例 | commit 4fe5e8af8：`reconcileComparableAnswer` 复用 `ValidateAnswer` | 本设计的判定谓词（已知字段集 / schema 闭集）必须单源函数，compute 门与 result 级校验共用——同一纪律。 |
| LEDGER-TRIPWIRE 双向红先例 | MEMORY 补记廿九 | 闭集与 struct JSON tag/alias 表的同步用双向 tripwire 测试钉死。 |
| Reconcile 一致性校验 | dataquery.go:2447-2540 | **不动**。它的职责是 Σ 一致性；血统门在上游把幽灵键掐死后它自动恢复诚实。 |

## 4. 泛化方案

### 4.1 血统四车道（typed 分类，在 compute 时刻判定一次）

runner 铸造的每条贡献的 GroupKey 恰属一条车道：

| 车道 | 判定来源 | 血统 |
|---|---|---|
| `field_values` | `recordCompositeGroupKey` 从观测行组合 | ✅ 直接观测 |
| `declared_literal` | `group_key_literal`（显式声明"这是标签"） | ✅ 显式声明，永不质疑 |
| `constant` | `group_key` 常量且**不与任何命名空间冲突** | ✅ 合法标签（如 `"all_active"`） |
| `synthetic_all` | 隐式 `"all"` | ✅ 合成缺省 |

**不存在第五车道。** 现状的"字段名静默降级为常量"正是企图混入的第五车道，被本设计消灭。

### 4.2 判定算法（`runComputeContributions` 重构，全局单次判定）

现状单循环"逐 path 读取+处理"改为两阶段：

**阶段一（普查）**：读入全部 input paths 的 records（内存已受 `maxRecords` 与 `data_task_max_file_bytes` 双重钳制；如需保守可选择重读实现，见 §9），建立：
- `globalKnownFields`：全输入字段名并集（复用 `markKnownActionFields`）；
- `perPathHasGroupKey[path]`：`actionRecordFieldExistsInRecords` 逐 path 结果。

**阶段二（单次解释判定，替代 8103 的逐输入判定）**：
1. `group_key_literal` 非空 → `declared_literal`，结束（现状语义不变）。
2. `group_key` 常量 ∈ **≥1 个输入**的字段集 → **全局 field 解释**：所有输入统一按字段值分组；缺该字段的输入走既有 typed 车道（多输入=`contribution_source_skipped` 跳过；全部输入都缺时不可能进此分支）。**分叉不复存在——这是 F11 直接病灶的根修。**
3. `group_key` 常量 ∉ 任何输入字段集：
   - ∈ `canonicalLedgerFieldNameSet()`（schema 闭集，§4.3）→ **typed 硬违规** `group_key_schema_name_no_lineage`（新码，走既有 `DataFieldContractError` 类型，Role=`contribution_group_key_lineage`），修复提示双臂：意图分组→先用 enrich_records/join_records/apply_entity_resolutions 物化该字段；意图常量标签→声明 `group_key_literal`。
   - 否则 → `constant` 车道放行（`"all_matching_rows"` 等合法标签行为零变化）。

**阶段三**：既有逐输入贡献循环照旧（解释已定，8103 删除）。

### 4.3 schema 闭集：`canonicalLedgerFieldNameSet()`（单源谓词）

新函数置于 dataquery.go alias 定义旁，返回闭集 = 四类账本 struct 的 JSON tag 全集 ∪ 各 alias 注册表（`"group"`,`"bucket"`,`"canonical_label"`,`"measure"`,`"locator"`…）。**这是可判定的退化情形**：一个"常量组标签"恰好等于账本 schema 名，几乎必然是命名空间混淆而非真实标签；真有此意者有 `group_key_literal` 逃逸口。

单源纪律（Gap B 先例）：compute 门（§4.2 步骤 3）与 result 级软校验（§4.4）**调用同一个函数**，禁止两处各抄一份。

同步纪律（LEDGER-TRIPWIRE 先例）：tripwire 测试用 reflect 枚举四 struct 的 JSON tag + 逐字断言 alias 表内容 == 闭集，双向红（闭集多了或 struct/alias 加字段没进闭集都红）。

### 4.4 脚本车道：软引导（不设硬门——见 §5 理由）

`validateRunnerResult` 内新增 `warnLedgerGroupKeyLineage(res)`：对脚本铸造（非 runner action 产出）的 target 贡献与 reconcile 组，若 GroupKey ∈ `canonicalLedgerFieldNameSet()` ∪（该记录 `Source` 所指 artifact 的 `Headers`/`Fields` 键集）→ 追加一条 `ContractWarnings`（"group key %q 与 schema/字段名同名；若是按字段分组请用 compute_contributions 的 group_key_field，若是常量标签请显式声明"）。经 `result_prompt_view` 天然可见，模型下一轮自纠。**不拦截、不改答案。**

### 4.5 typed 修复路径

1. 违规错误本身携带 `Repairability=NeedsRecompute` + 双臂 RepairHint（§4.2.3）。
2. `dataworkflow/missing_field_recovery.go` 增加对新违规码的识别臂（与 `MissingComputeGroupFieldFallbackPlan` 同族）：当上游存在能供给该字段的 artifact 投影时，确定性生成 enrich/join fallback 计划——模型甚至不需要理解错误就能被推回正轨。
3. 逃逸执行：模型改发 `group_key_literal` → 步骤 1 直接放行（既有语义）。

### 4.6 prompt 教学（软面，过 prompt 红线 checklist）

scaffold.go:882 已有参数提示模板（`"group_key_field": "<existing grouping field, or use group_key for a constant group>"`）。补一句三分语义：`group_key_field`=按已存在字段分组；`group_key`=常量标签（不得与字段名同名）；`group_key_literal`=显式常量（允许与字段名同名）。改动走 ATOMIC 7 条 checklist（R3/R4/R5/R6/R7/SST/R2'）。

## 5. 判定信号与红线合规

| 红线 | 合规论证 |
|---|---|
| 精确信号才配硬门 | runner 车道硬门的两个谓词都精确：①字段存在性=对全量已读 records 的逐字键匹配（`actionRecordFieldExistsInRecords`，与现状同一函数）；②schema 闭集成员判定=有限字符串集合逐字匹配。无 ranker、无相似度、无 sample 截断。 |
| 嘈声信号只作软引导 | 脚本车道"GroupKey ∈ headers"**不能证明**它不是恰好同名的真实观测值（值命名空间与 schema 命名空间可以合法重合）——该信号对脚本车道是嘈声的，故只进 `ContractWarnings` 软车道。这正是"runner 车道我们亲手组的键（血统确知）→硬；脚本车道血统不可知→软"的边界。 |
| 硬门必配 typed 逃逸 | 逃逸口=`group_key_literal`，**既有** typed 通道（§15.12），零新逃逸机制；错误 RepairHint 双臂明示逃逸路径。 |
| 谓词同源（CSP#63 / Gap B） | `canonicalLedgerFieldNameSet()` 单源；compute 门与 result 软校验共用；tripwire 钉同步。 |
| LLM 误行为=prompt 歧义 | `group_key` 参数名天然歧义（"键"既像字段又像标签）——§4.6 prompt 教学消歧；引擎侧门只兜底不替 prompt 干活。 |
| 零答案代价 | 硬门只在"字段名当常量且该字段无处存在"这一**必然产出幽灵账本**的形态上触发；任何今天产出正确答案的运行不落入该形态（落入者其账本本就是假的）。合法常量标签、literal 声明、field 分组行为逐字节不变。 |
| 完成门权属模型 | 本门是账本铸造门（工件正确性），不是完成判定门；不触碰 `emit_investigation_complete` 权属。 |

## 6. 触点文件与实施步骤

1. **internal/dataquery/action_runner.go**
   - `runComputeContributions`（8037-8368）：两阶段重构（§4.2）；删除 8103-8107 逐输入重释；新违规用 `DataFieldContractError`（Role=`contribution_group_key_lineage`，复用既有 `Violation()` 分类链路——typed 错误经 `classifyExecutionFailureLeaf` 自动落 typed 车道，无需 substring 新分支）。
2. **internal/dataquery/dataquery.go**
   - 新增 `canonicalLedgerFieldNameSet()`（置于 alias 定义旁，注释指回各 alias 注册表）；
   - `validateRunnerResult`（1965）挂 `warnLedgerGroupKeyLineage`（软，只 append ContractWarnings）。
3. **internal/dataworkflow/missing_field_recovery.go**：新违规码识别臂 → 复用 enrich/join fallback 计划族。
4. **internal/dataworkflow/scaffold.go:882**：参数提示三分语义一句（过 prompt checklist）。
5. **docs/architecture.md** 数据车道小节：血统四车道 + 闭集 + 逃逸口一段（与 §15.12 批乙记录互引）。
6. R2' 六处同步自查：新违规码走既有 typed 错误类型与既有 Role 分类面，六处清单逐项过（schema 描述/strict-decode/分类器/修复识别器/prompt/docs）。

## 7. 测试与判决性验证

先红后绿纪律（渲染面/行为面 pin 先写红）：

1. **幽灵键复现 pin（先红）**：多输入、无一携带 `canonical_label` 字段、`group_key="canonical_label"` → 现状断言"通过且组=字面名"（红基线）→ 修后断言 typed 违规 `Role=contribution_group_key_lineage` + RepairHint 双臂子串。
2. **分叉消灭 pin**：输入 A 有字段、B 无 → 全局 field 解释；B 走 `contribution_source_skipped`；产出组集只含 A 的观测值，无字面组。
3. **逃逸 pin**：`group_key_literal="canonical_label"` 且输入携带同名字段 → 常量放行（§15.12 既有 pin 保持绿，防回归重释）。
4. **零变化 pin**：`group_key="all_active"`（非 schema 名、无同名字段）→ 行为逐字节不变。
5. **闭集 tripwire（双向红）**：reflect JSON tag 全集 + alias 表 == `canonicalLedgerFieldNameSet()`。
6. **脚本车道软 pin**：脚本铸造贡献 GroupKey ∈ headers → `ContractWarnings` 含血统告警、result 仍通过、答案不变。
7. **修复路径 pin**：新违规码 + 存在供给字段的上游投影 → fallback 计划含 enrich/join 动作。
8. **判决性 e2e**：`eval/cases/data_multifile_reference_projection.case` 重跑——F11 标本上组键必须为观测到的 canonical_label **值**（或 literal 声明），不得出现字面 `"canonical_label"` 组。全量 `make` + `go test ./...` 绿。

## 8. 5-Q 反过拟合自查

1. **下一个实例零新代码能拦吗？** `group_key="metric"`、`"canonical_id"`、`"status"`（有同名字段时）、任意 alias 名——全部落在同一闭集/同一全局判定，零新代码。✅
2. **修的是机制还是标本？** 机制：命名空间混淆的判定粒度（per-input→per-action）+ 血统车道的穷尽枚举（消灭第五车道）。标本 F11 只是首例。✅
3. **谓词有没有第二份手抄？** 闭集单源函数 + tripwire 双向红；字段存在性复用现有同一函数。✅
4. **换一个账本家族还成立吗？** 类谓词（值位置携带 schema token 无血统）对 `EntityResolutionRecord.CanonicalLabel`、脚本 reconcile 组同样成立——软车道（§4.4）已按记录族泛化覆盖；硬门边界由"血统是否确知"而非记录类型划定，家族扩展不需改判定结构。✅
5. **有没有为过标本而收窄？** 无：门不读 eval 期望值、不读请求文本、不含 `canonical_label` 字面量（它只是闭集的普通成员，来自 EntityResolutionRecord 的 JSON tag）。✅

## 9. 风险与不做什么

- **不做**：reconcile 时刻的血统复核。records 在 reconcile 时已丢弃，复核需重读源文件并做值格式比对（格式化噪声→嘈声信号硬门，红线违背）；compute 时刻是最早且唯一的诚实点，铸造口封死后 reconcile 的既有 Σ 校验自动恢复判别力。
- **不做**：脚本车道硬门（§5 第二行论证——headers 成员性对脚本记录是嘈声信号）。若软告警实测无效，升级路径是给 `add_contribution` 加 `group_label` typed 参数（R2' 六处同步），而非把嘈声信号变硬。
- **不做**：给 `ContributionRecord` 新增 `group_key_lineage` 持久字段。runner 车道血统在铸造点即时判定即时执法，无需持久化；避免一次 R2' 六处同步 + schema 面扩张。（若未来跨阶段需要血统信息再立案。）
- **不做**："组键值必须出现在引用源行中"的逐行反查硬门——值经组合（多字段 `/` 拼接）与规范化后与原行值非逐字对应，反查是嘈声的；field 车道的血统由"我们亲手从行值组键"这一构造性事实保证，无需事后反查。
- **风险 1（内存）**：阶段一全量预读使峰值内存从单 path 变为 Σ paths。受 `maxRecords`（默认 10 万/路径）与 `data_task_max_file_bytes` 钳制；如实测超限，退化实现=阶段一只做字段普查后丢弃、阶段二重读（IO 翻倍，语义不变）。
- **风险 2（假阳性）**：用户数据真有一列值恰为 `"canonical_label"` 且模型想拿它当常量标签——落入闭集硬门。逃逸口 `group_key_literal` 一步走出，且 RepairHint 明示；判定为可接受（该形态本身就该显式声明）。
- **风险 3（行为面）**：删除 8103 的逐输入重释改变"单输入、字段存在、用 `group_key` 传字段名"的既有便利路径——全局判定下步骤 2 覆盖同一形态（∈ ≥1 输入字段集 → field 解释），行为一致；pin 4 与既有测试套保证。

## 10. 落地偏离（EVALFIX-2D 实施记录，2026-07-30）

按 §6 顺序落地。先红纪律：pin 1 形态以落地前现状跑了真红证（临时探针：混合输入下携带字段的 a.csv 全部落入字面组 `"canonical_label"`、缺字段的 b.csv 反而按字段值分组——比 F11 记录的更糟，同一 action 组语义实测分叉，零报错）。落地文件：`internal/dataquery/group_key_lineage.go`（新，血统车道注释 + `ContributionGroupKeyLineageRole` + 两阶段助手 + 血统谓词三助手）、`internal/dataquery/action_runner.go`（两阶段重构，8103 逐输入重释删除）、`internal/dataquery/dataquery.go`（alias 注册表变量化 + `canonicalLedgerFieldNameSet` + `warnLedgerGroupKeyLineage`）、`internal/dataworkflow/missing_field_recovery.go`（`computeGroupKeyRepairRole` 双 Role 识别臂）、`internal/dataworkflow/scaffold.go`（Note 三分语义句）、`docs/architecture.md` §13.8。pin 全集：`internal/dataquery/group_key_lineage_test.go`（幽灵键硬拒/全局判定灭分叉/literal 逃逸/零变化/闭集 tripwire 双向+全量 verbatim golden/脚本车道软 pin）+ `internal/dataworkflow/missing_field_recovery_test.go`（lineage Role 修复路径 pin）。与设计字面的偏离：

1. **"新码 `group_key_schema_name_no_lineage`"（§4.2.3）未铸新 Code**——落地为 Code=`field_contract_violation` + Role=`ContributionGroupKeyLineageRole`（"contribution_group_key_lineage"）作 typed 判别子。理由：§6.1 自己要求"复用既有 `Violation()` 分类链路…无需 substring 新分支"，而 `DataFieldContractError.Violation()` 铸的就是 `field_contract_violation`；且 dataworkflow 的显示面（process_display.go 双语 code switch）与 repairability 归类（violation.go:325）都是按 Code 闭式枚举——新 Code 会跌进两处 default 臂（丢双语解说、RepairNeedsTypedAction→NeedsRecompute 降档），Role 判别与既有 `contribution_group_key` 识别臂逐字同构。修复识别臂（§4.5.2）落为 `computeGroupKeyRepairRole` 双 Role 谓词，enrich/join fallback 计划族零复制复用。
2. **闭集单源的具体分工**——canonical JSON tag 侧为手写字面表 `canonicalLedgerFieldTagNames`（tripwire 用 reflect 对四 struct 双向红：加字段没登记红、登记名无 tag 红）；alias 侧不再手抄第二份——四 struct 的 `UnmarshalJSON` 内联 alias 字面全部变量化为包级注册表（`ledgerGroupKeyAliases` 等 21 个），解码器与闭集构造共读同一变量（结构性单源，比"逐字断言 alias 表内容"更强），tripwire 另附闭集全量成员 sorted verbatim golden（79 名），任何增删都是显式过审变更。
3. **§4.4 软告警的车道判定**——`warnLedgerGroupKeyLineage` 挂在 `validateRunnerResult`，以 `len(plan.Actions)==0 && plan.Script!=""` 判"纯脚本批"（typed 计划形状，精确信号）。action 计划内嵌 `custom_transform` 的脚本贡献不告警：区分它们需要 §9.3 明确不做的持久血统字段；纯脚本车道全覆盖，runner 车道在铸造口硬门。headers 集取 artifact `Headers`（数据 schema 面）；未含 `DataArtifact.Fields` 的 key（那是 count/total 等工件元数据，非行字段，纳入纯增噪）。
4. **"双臂 RepairHint"承载面**——两臂完整句放 `RepairHint`（该 error 类型的默认 RepairHint 本就是 prose，先例成立）且 `Message` 重述双臂；pin 断言 enrich_records/join_records/group_key_literal 三子串。
5. **LOC ratchet 触发（预期内）**——action_runner.go 12353 基线不动：血统 concern（车道注释/Role 常量/phase 1+2 助手）+ 三个血统谓词助手（`markKnownActionFields` / `actionRecordFieldExistsInRecords` / `recordCompositeGroupKey`，正是硬门与 field 车道的谓词本体）按 ratchet 本意移入 sibling `group_key_lineage.go`，主文件净缩至基线下。
6. **阶段一取全量预读实现**（§4.2 首选形；§9 风险 1 的重读退化形未用）——峰值内存 Σ paths，受 `maxRecords`（10 万/路径）与 `data_task_max_file_bytes` 双钳制，与设计评估一致。

验证：gofmt 净（触及包）；`go build ./...` 绿；`go test ./...` 83 包全 ok 零 FAIL；§7 判决性 e2e（`data_multifile_reference_projection` eval 重跑）见实施汇报。

状态：已实施（EVALFIX-2D，2026-07-30；落地偏离见本节 §10）。对应 eval gap 报告 F11。

---

# CLASS 5 — 静默 fail-open 车道的统一披露面（typed degradation ledger）

设计文档 · 2026-07-30 · 覆盖 EVALRUN-1 gap 报告 F8（richness 硬转软 / 17 处引文确定性重写 / compat 修复族）；F9 维持完成门裁定不动。

---

## 1. 问题类定义

**类**：设计上的 fail-open 车道（deliberate degradation absorber）——系统在运行中确定性地软化 / 重写 / 降级了某个承诺面，只发一条 WARN 日志（甚至只有 telemetry），**用户交付面零披露**。每条车道各自 ad-hoc：没有清单、没有统一记账点、没有任何一个用户能看到「这份答案里有哪些东西被软化过」的单一位置。

类的三个结构特征（判别一条新车道是否属于本类）：

1. **fail-open 是设计而非缺陷**——车道存在的理由成立（避免 false-fail、容忍 LLM 输出瑕疵），修复方向不是关掉车道；
2. **降级动作确定性发生且可计数**——每次触发都有精确的 typed 事实（一个 bool 翻转、一个 fixed 计数、一次枚举值降档），不是模糊猜测；
3. **披露只到 WARN/telemetry 层**——与仓库既有的披露文化（「凡施修必披露」§29.181、degraded 车道 footer、authority drift caveat）形成张力：同类动作有的披露有的不披露，标准不统一。

**反面（不在类内）**：已经拥有用户面披露的车道（不动）；纯粹的 LLM-工具管道整形（param compat 族——对答案语义零影响，披露反而是噪音）；F9 预算未尽欠探索（完成门权属模型裁定内合法，模型自带披露收尾，红线禁止系统硬卡——只考虑 skill prompt 软引导，且不在本批）。

---

## 2. 类内已知实例（诚实清单 = 交付物的一部分）

全仓 grep（`WARN.*soften/degrad/repair/compat/rewrit`、`facet_softened`、`tool_param_compat`、`structured_payload_compat`、citation backfill 族）后的完整盘点，按三档分类：

### 2.1 答案语义降级（answer_semantics —— 本设计 MUST 披露，v1 名册 3 条）

| # | 车道 | 触点 | 现状 | 典型量级 |
|---|------|------|------|---------|
| A1 | **richness 必答面硬转软**（`facet_softened`） | `internal/types/facet_plan.go:815-828`（HARD→SOFT 降档 + `RichnessTelemetrySignal{Kind:"facet_softened"}`） | typed telemetry 已有；读者=orchestrator.go:6878 WARN 行（`pipeline_richness_softening_warn` 门控）+ evaluator.go:2818 的 G7 prompt 注记。**用户面零披露** | F8 实测每例 0–2 条 |
| A2 | **current-source 引文确定性重写**（quoteless/漂移引文用磁盘真实行覆写模型引文） | `internal/tool/answer_document_citation_quote_normalize.go::normalizeCurrentSourceCitationQuotes`；调用点=`emit_answer_document_v2.go:258/851`、`answer_document_mutation_runtime.go:223`（persist chokepoint）、`answer_document_degraded_export.go:58`（degraded 车道——已自披露，见 2.3） | 返回 `fixed int`，只发 WARN `backfilled %d quoteless citation quote(s)`。**用户面零披露**（degraded 车道除外） | F8 实测一例 **17 处** |
| A3 | **完整性降档 complete→lower_bound** | `internal/agent/extractor.go:2177-2183`（WARN + `AnalyzerDecisionSignal{Kind:"completeness_downgraded"}`） | typed 信号已有，finalizer 用 softened prompt。**用户面零披露**（答案的 claim 强度被系统改变） | 低频但语义重 |

### 2.2 纯管道整形（plumbing —— 永久 log-only，登记在册但**永不**上用户面）

| # | 车道 | 触点 | 理由 |
|---|------|------|------|
| P1 | tool_param_compat（agent 侧参数归一 + analyzer prescan files_only 强制） | `internal/agent/agent.go:3983/4253` | LLM-工具接口整形，答案语义零影响 |
| P2 | structured_payload_compat（string-wrapped array/object 修复） | `internal/tool/structured_payload_compat.go`（7 处 WARN） | 同上 |
| P3 | LLM-corrupted JSON 结构修复 | `internal/agent/agent.go:4008/4491`、`internal/agent/tool_params_repair.go:118` | 同上 |
| P4 | REPL/data 结构化工具参数 compact repair | `internal/repl/structured_tool_params.go:90`、`command_operation_planner.go:845`、`data_task_planner.go:851` | 同上 |
| P5 | 分类器 fallback 链（turn-policy→legacy binary→pipeline） | `internal/repl/repl.go:7182/7493/7503` | 路由层，答案内容未被改写 |
| P6 | multirepo focus selector fallback preview | `internal/orchestrator/multirepo_focus_selector.go:29-38` | preview 本身用户可见，非静默改写 |

> 「噪音从源头消除」红线：把 P 族推上用户面会把披露行变成垃圾桶——诚实分类的另一半就是**诚实地不披露**。

### 2.3 已有披露的车道（self_disclosing —— 一律不动、不重复入账）

| 车道 | 既有披露面 |
|------|-----------|
| degraded 交付车道（含 degraded 车道上的引文回填） | `degradedDeterministicSectionsCaveat` + `degradedSectionDisplayNames`（含 `citation_quote_backfill` 词条，evaluator.go:11283-11354） |
| authority drift（conditional/historical 证据） | `render/apply_authority_hedging.go`（hedge sentinel + BlockCaveat 双语 drift caveat） |
| aggregate_facts 补注/归一族 | Tier2 词面单点三格式（`emit_investigation_complete.go:1504-1538`，「凡施修必披露」） |
| P6 finalize repair 硬顶 residual-concerns caveat | `orchestrator.go:6133/6153`（violation 物化为 user caveat） |
| mermaid 渲染失败 | L7 红线：fence 改写 ` ```text` + `# ·` reason leader（用户面自带） |
| runtime-artifact 引文 detach | `answer_document_runtime_citation_normalize.go::recordRuntimeArtifactCitationDetachDisclosures` |
| 补充采集披露 | `supplement_disclosure` 板块 |

### 2.4 边界待裁（登记，v1 不入名册）

- `write_analyze` softened contracts / fallback IR（orchestrator.go:2953/2996）——write 车道有自己的 status card 披露面，归属待写侧统一盘点；
- `log_triage` two-step degrade（log_triager.go:404）、pre-stage degraded（orchestrator.go:2166）——降级后答案是否携带 pre-stage 缺失的语义影响，待逐例定性后按 §4 的两行成本入册；
- `normalizeUnusedCitationPoolEntries` / exclusion sanitize——前者纯卫生（plumbing），后者是执行用户自己的排除指令（不是系统单方降级），暂 plumbing。

---

## 3. 既有机制盘点（先找半成品车道——本仓惯例）

结论：**收集侧的半成品车道已经存在，本设计是「补上 TBD 的读者」而不是发明新通道。**

1. **`RichnessTelemetrySignal` + `MutableState.AppendRichnessTelemetry/RichnessTelemetry/Reset`**（types/context.go:911-937, 5274-5317）——per-run typed 收集器原型，注释自述「readers are TBD — the signal will surface in operator-facing logs (B6+)」。这就是那条半built lane。
2. **`AnalyzerDecisionSignal` 通道**（context.go:880-897 + AppendAnalyzerDecision）——开放 kind 枚举的第二条 typed 决策通道，`completeness_downgraded` 已在册。
3. **degraded footer 的词面机制**（`degradedSectionDisplayNames` token→{zh,en} 映射 + 「internal token never rides a user surface」+ 未知 token fail-open 到 generic 词）——footer 的措辞纪律直接复用。
4. **`RenderAnswerDocumentWithLastMileSupplements` 单一渲染咽喉**（agent/answer_document_render_export.go，TRUNC 批 §29.10-1，pin=`TestTRUNCFinalAnswerRerenderChokepointStructural`）——所有 FinalAnswer 渲染路径（首稿、auto-repair、恢复稿）已被结构测试钉死必须走此函数。footer 的**唯一注入点**。
5. **`pipeline_richness_softening_warn` + `SetRichnessSofteningWarnEnabled` 函数间接接线模式**（orchestrator.go:6968-6978）——配置逃逸车道的现成范式。
6. **「凡施修必披露」词面单点纪律**（§29.175 T2 / §29.181④）——本设计把该纪律从 aggregate_facts 局部推广为全类统一出口。

---

## 4. 泛化方案

### 4.1 核心：typed degradation ledger（一个收集器 + 一个注册表 + 一个渲染咽喉）

**(a) 车道注册表（单一事实源，closed set）** — 新文件 `internal/types/degradation_ledger.go`：

```go
type DegradationLaneID string       // typed，内部 token，永不上用户面
type DegradationClass string        // "answer_semantics" | "plumbing"（self_disclosing 车道不注册）

const (
    DegradeLaneCitationQuoteRewrite   DegradationLaneID = "citation_quote_rewrite"
    DegradeLaneRichnessFacetSoftened  DegradationLaneID = "richness_facet_softened"
    DegradeLaneCompletenessDowngraded DegradationLaneID = "completeness_downgraded"
)

type DegradationLaneSpec struct {
    Class DegradationClass
    ZH, EN string                    // 用户面显示词，双语齐备（注册表测试强制）
}
var DegradationLaneRegistry = map[DegradationLaneID]DegradationLaneSpec{
    DegradeLaneCitationQuoteRewrite:   {ClassAnswerSemantics, "引用摘录回填", "citation quote backfill"},
    DegradeLaneRichnessFacetSoftened:  {ClassAnswerSemantics, "必答面硬转软", "required-facet softened"},
    DegradeLaneCompletenessDowngraded: {ClassAnswerSemantics, "完整性降档为下界", "completeness downgraded to lower-bound"},
}
```

**(b) per-run 收集器** — `MutableState` 上镜像 RichnessTelemetry 的三件套：

```go
func (m *MutableState) AppendDegradation(lane DegradationLaneID, n int)   // n<=0 no-op；按 lane 累加
func (m *MutableState) DegradationLedger() []DegradationEntry             // lane 分组、注册表序稳定输出
func (m *MutableState) ResetDegradationLedger()                           // 生命周期边界=pipeline start（与 S12/S13 裁定的唯一边界同源）
```

`DegradationEntry{Lane DegradationLaneID; Count int}` —— **不携带 detail 散文**：完整明细留在既有 WARN 日志（logs keep full detail），账本只记 typed lane + 精确计数，杜绝内部散文渗上用户面的可能性。

**(c) 写入侧：优先桥接既有 typed 通道，杜绝双写漂移（五表手抄教训）**

| 车道 | 写入方式 | 新增代码 |
|------|---------|---------|
| A1 facet_softened | **零写者改动**：footer 构建时从 `Mutable.RichnessTelemetry()` 投影 `Kind=="facet_softened"` 条数 → ledger（单向确定性投影，既有通道保持唯一写点；G7 prompt 注记、WARN 行读者全部不动） | 0 行写者 |
| A3 completeness_downgraded | 同上：从 `Mutable.AnalyzerDecisions()` 投影 `Kind=="completeness_downgraded"` | 0 行写者 |
| A2 引文重写 | 调用点已握有 `fixed int`：在 `emit_answer_document_v2.go:258/851` 与 `answer_document_mutation_runtime.go:223` 的既有 WARN 旁各加一行 `ctx.Mutable.AppendDegradation(DegradeLaneCitationQuoteRewrite, fixed)`。**degraded_export.go:58 不加**（该车道经 `citation_quote_backfill` 词条已自披露，加了就是双披露） | 每点 1 行 |

投影规则写死在一个函数 `BuildDegradationLedgerView(mut) []DegradationEntry`（直写条目 + 两条通道投影合并），渲染层只认这个视图——新增「已有 typed 通道」的车道时同样是加一条投影臂，不开第二收集器。

**(d) 渲染侧：唯一咽喉，至多一行 footer**

注入点=`renderAnswerDocumentWithLastMileSupplements`（§3.4 的 TRUNC 咽喉），在系统补充板块之后追加：

```
系统降级披露：引用摘录回填 ×17、必答面硬转软 ×1（完整明细见运行日志）
System degradation disclosure: citation quote backfill ×17, required-facet softened ×1 (full detail in run logs)
```

规则（全部确定性）：
- 仅 `Class==answer_semantics` 且 `Count>0` 的条目参与；分组聚合，**绝不逐事件**；
- 0 条 → **0 字节**（健康路径字节恒等，同 [CGEC] 空快照纪律）；
- 显示词只出自注册表 ZH/EN 对；账本里出现未注册 lane（防御臂）→ fail-open 到 generic 词「确定性降级处理」/"deterministic degradation"，计数保留，绝不发明具体名（degradedSectionDisplayNames 同款纪律）；
- 语言跟 `docLang`，顺序=注册表声明序（稳定输出，golden 可 pin）；
- 配置逃逸：`codrax.yaml :: pipeline_degradation_footer *bool`（缺省 true），经 `SetDegradationFooterEnabled` 函数间接接线（镜像 `SetRichnessSofteningWarnEnabled`）。关掉 footer 不关账本与 WARN——telemetry 永远在录。

**(e) 运维侧**：run 结束在 `emitCGECSummary` 邻位加一条聚合 INFO `[degrade] ledger: citation_quote_rewrite=17 richness_facet_softened=1`（0 条不打印），让 operator 无需翻散落 WARN 即可 grep 全账。

### 4.2 新车道接入成本（类承诺）

发现第 N+1 条静默降级车道时：**注册表 1 行 +（写者 1 行 AppendDegradation ｜或既有 typed 通道 1 条投影臂）**。footer、分组、双语、门控、测试基建零新增。分类为 plumbing 的新车道只入注册表（永不渲染），分类争议在注册表行的 review 中显式裁决——这正是「分类诚实」的机械化落点。

---

## 5. 判定信号与红线合规

| 红线 | 合规论证 |
|------|---------|
| **精确信号硬门/噪声信号软引导** | footer 触发条件=typed lane 枚举 + 精确整数计数（bool 翻转/fixed 计数/枚举降档），零噪声启发式。且 footer 本身不是门——不 block、不改写答案实体，纯披露 |
| **硬门必配 typed escape lane** | 本设计不引入任何硬门；配置逃逸=`pipeline_degradation_footer`；per-lane 逃逸=注册表 Class 字段（typed enum） |
| **系统不可代替 LLM 写用户面板答案** | footer 是系统元数据行，与 citations pool / degraded footer / authority caveat 同一 precedent 车道：render-time transform（ApplyAuthorityHedging 同款），不 backfill 答案实体内容，不动 Mutable 上的存档 doc |
| **噪音从源头消除** | 单点聚合、至多一行、plumbing 永不渲染；detail 散文根本不进账本（结构性防渗漏） |
| **No internal info in LLM prompts** | 账本/footer 均为渲染与日志侧；不进任何 LLM prompt（G7 既有 facet 注记维持原样，不扩展） |
| **完成门权属模型** | F9 零触碰：不入册、不设 lane、不加 gate；skill prompt 软引导另案且非本批 |
| **prompt 红线 checklist** | 本批**零 prompt 改动**，checklist 不触发（如未来给 A 族配 prompt 教学另走 checklist） |
| **零答案质量成本** | 零新增 LLM 调用、零 gating、零重试路径变更；健康 run 渲染字节恒等（有 golden pin）；L1 读模字节恒等测试不受影响（footer 在两臂行为一致） |
| **收敛 spec 禁并行 taxonomy** | 不开第二收集通道：既有 RichnessTelemetry/AnalyzerDecisions 保持唯一写点，ledger 经单向投影消费；直写仅覆盖无 typed 通道的车道 |

---

## 6. 触点文件与实施步骤

1. **`internal/types/degradation_ledger.go`（新）**：LaneID/Class/Spec/Registry + Entry + MutableState 三件套（`internal/types/context.go` 加字段 `degradationLedger map[DegradationLaneID]int` + reset 接线到 pipeline-start 既有 reset 簇）+ `BuildDegradationLedgerView`（直写 + facet_softened / completeness_downgraded 两条投影臂）。
2. **`internal/tool/emit_answer_document_v2.go`（258/851 两处）、`internal/tool/answer_document_mutation_runtime.go`（223）**：WARN 旁各 +1 行 `AppendDegradation(DegradeLaneCitationQuoteRewrite, fixed)`；`answer_document_degraded_export.go` 显式注释「self_disclosing，不入账」。
3. **`internal/agent/answer_document_evaluator.go`**：`renderAnswerDocumentWithLastMileSupplements` 尾部追加 footer 构建（读 `BuildDegradationLedgerView`，双语词表放 evaluator 邻位、纪律注释指向 degradedSectionDisplayNames 同款规则）。
4. **`internal/config/runtime.go`**：`PipelineDegradationFooter *bool`（`pipeline_` 前缀组）；**`cmd/root.go`**：接线 `agent.SetDegradationFooterEnabled`（函数间接，镜像既有两例）。
5. **`internal/orchestrator/orchestrator.go`**：`emitCGECSummary` 邻位 `[degrade] ledger:` 聚合 INFO。
6. **`docs/architecture.md`**：新小节记录注册表=分类单一事实源 + §2 清单三档裁定（含 2.4 待裁项），把「凡施修必披露」的适用边界（answer_semantics 必披露 / plumbing 永不 / self_disclosing 不重复）落为在案裁定。

预估 diff：~300 行含测试；无 schema 变更、无 prompt 变更。

## 7. 测试与判决性验证

1. **单元**：Append 聚合/负数 no-op/Reset 边界；投影臂各一正一负；footer 单行、分组序稳定、双语、0 条 0 字节。
2. **词面纪律 pin**：渲染输出对每个内部 token（`citation_quote_rewrite` 等）做 NOT-contains 断言（internal token never rides user surface）；未注册 lane → generic 词 pin。
3. **注册表闭集 tripwire**（LEDGER-TRIPWIRE 范式，等式不变量而非 count 型）：每个注册 lane 必有非空 ZH+EN+合法 Class；`ClassAnswerSemantics` 车道集合 == footer 渲染臂消费集合（双向）。
4. **咽喉结构 pin**：footer 构建仅存在于 lastMileSupplements 咽喉内（扩展 TestTRUNC 家族——防止旁路渲染路径丢 footer 的同款 TRUNC 事故）。
5. **判决性 e2e**（唯一行为变更必配正向 e2e，RUNSPLIT 教训）：构造触发引文回填 + facet 软化的 run → 断言恰一行 footer 且两组计数正确；健康 run golden 字节恒等；`TestRunMode_ReadByteIdentical` 维持绿。
6. **冒烟 eval 选例必须覆盖改动面**（TWODIM 教训）：选一个已知触发引文回填的既有 fixture（F8 witness 族）跑通，验证答案正文零变化 + footer 出现；qf 基本盘一例验证 0 条时零字节。

## 8. 5-Q 反过拟合自查

1. **下一实例零新机制代码可修？** 是——新静默车道=注册表 1 行 + Append 1 行（或投影臂 1 条）；footer/分组/双语/门控/测试基建全继承。
2. **是否 keyed 在实例特征上？** 否——无字符串匹配、无车道专属渲染分支；渲染是对 typed enum 的通用分组循环；三条 v1 车道只是名册初值。
3. **修的是类根因还是症状？** 根因=「披露各自 ad-hoc、无单一记账点」；方案交付的是记账点 + 分类事实源 + 单一出口，17 处引文只是 witness。
4. **边界是否诚实？** 是——plumbing 永不披露、self_disclosing 不重复、F9 权属裁定零触碰，三个「不做」都有红线依据而非省事。
5. **失效模式是否退化安全？** 是——账本满/空/未注册 lane 全部 fail-open（generic 词或 0 字节），永不 block 出厂；关 footer 不关记录。

## 9. 风险与不做什么

**风险**：
- 双披露（degraded 车道 + footer）——已用「degraded_export 不入账」规避；若未来 degraded 车道重构，闭集 tripwire #3 会暴露集合漂移。
- footer 措辞被误读为答案内容——用「系统降级披露：」前缀锚定系统身份（与「系统补充：」同族词面）。
- Reset 边界错位导致跨 turn 串账——复用 S12/S13 已裁定的 pipeline-start 唯一边界，测试 #1 钉死。

**不做什么**：
- 不给 plumbing 族任何用户面出口（永久裁定候选，理由=噪音红线）；
- 不动任何已披露车道的既有词面（authority/degraded/Tier2/L7）；
- 不把账本喂进任何 LLM prompt，不做「让模型解释降级」的二次调用（零成本红线）；
- 不为 F9 设 lane/gate/强制披露（完成门权属模型）；
- 不重构 RichnessTelemetry/AnalyzerDecisions 为统一通道（现阶段投影桥接足够；通道合并=另案架构议题，FileReadCoverageStore 教训：不同时间语义视图强行统一是回归源）。

## 10. 落地偏离（EVALFIX-2E 实施记录，2026-07-30）

按 §6 全部六步落地（`internal/types/degradation_ledger.go` 三件套 + 注册表 + `BuildDegradationLedgerView` 双投影臂；三处健康引文调用点写者；`renderAnswerDocumentWithLastMileSupplements` 尾部 footer；`pipeline_degradation_footer` + `agent.SetDegradationFooterEnabled` 函数间接；`emitCGECSummary` 邻位 `[degrade] ledger:` INFO；`docs/architecture.md` §6.9 + §14 knob 表）。§7 测试全落：types 单元 7 条、tool 写者桥 2 条（真实 normalize 过盘 + 三调用点/degraded 缺席结构 pin）、agent 8 条（恰一行分组双语判决 e2e、0 条 0 字节、knob-off 字节恒等复原、plumbing 永不披露负 pin、未注册 generic fail-open、internal token NOT-contains、消费集合双向等式、TestTRUNCDegradationFooterChokepointStructural 咽喉结构 pin）；判决 pin 均做过 red 验证（拆线→三红→复原绿；注册表单侧加 lane→tripwire 红）。偏离与备注：

1. **`emit_answer_document_v2.go:258` 调用点微重构**：原行把 fixed 内联穿给 `logCurrentSourceCitationQuoteRepairs`，无局部变量可旁挂 Append——重构为 `if fixed := ...; fixed > 0` 块（行为恒等：log helper 本就 fixed<=0 no-op），Append 与 WARN 同块。
2. **写者经共享 nil-safe helper**：三调用点各 +1 行 `recordCitationQuoteRewriteDegradation(ctx, fixed)`（tool 包内，nil ctx/Mutable/非正数全 no-op），而非裸 `ctx.Mutable.AppendDegradation`——emit 车道存在 ctx 可空的测试路径。degraded_export.go 落显式「self_disclosing，不入账」注释（§6.2 原样）。
3. **注册表声明序落为显式 slice**：Go map 无序，`顺序=注册表声明序` 由 `DegradationLaneRegistryOrder` 承载，闭集 tripwire 对 map↔slice 双向等式钉死（LEDGER-TRIPWIRE 范式，非 count 型）。
4. **双语词表权威位置**：ZH/EN 对按 §4.1(a) 落在 types 注册表（单一事实源）；§6.3「词表放 evaluator 邻位」落为 evaluator 侧仅持 generic fallback 词 + 句式模板（避免第二张词表，五表手抄教训）。
5. **plumbing 六条即刻入册**（§2.2 P1–P6 → tool_param_compat / structured_payload_compat / llm_json_repair / repl_param_repair / classifier_fallback / multirepo_focus_fallback），零写者、负 pin 钉死永不渲染；ZH/EN 词非空是注册表测试强制（未来若改类不至于空标签上面）。
6. **§7.6 冒烟 eval 未在本窗执行**：需真 LLM 往返（F8 witness 族全管线跑），本窗以渲染咽喉判决性 e2e（真实 normalize 写者 → 投影 → footer 恰一行）替代确定性覆盖；推 main 前建议补跑 `eval/run.sh` F8 witness 一例 + qf 基本盘一例。
7. **计数语义备注（非偏离，落账）**：emit 侧 pass 每次 emit 尝试都会跑——被拒稿重试的 Run 账本累计的是「本 Run 确定性重写事件数」（与 WARN 日志逐条对齐，footer 词面「完整明细见运行日志」指向的正是这份账），不是出厂稿内去重后的 distinct 引文数；carry-forward 机制使重试稿引文多已带 quote，实际重复计入有限。
8. **gofmt**：改动 hunk 全部 gofmt-clean；`internal/config/runtime.go` / `cmd/root.go` 存在**先于本批**的无关区段格式漂移（1092+ 行对齐、430 行注释对齐），按最小 diff 纪律不顺手重排。
9. **`[degrade] ledger:` INFO 落 concern 文件**：首版内联在 `emitCGECSummary` 内触发 `TestIRDeliveryHotFileLineRatchet`（orchestrator.go 9139 > 9135）——按 ratchet 本意移 concern 文件 `degradation_ledger_log.go::emitDegradationLedgerSummary`（自带 60 行预算行），`emitCGECSummary` 邻位保留 1 行调用（§6.5「邻位」语义不变），不扩 god-file 预算。

---

# 第六轮自查 sweep 修复记录（R6-0…R6-5，2026-07-30）

来源：`eval/sweep_round6_findings_20260730.md`（六条双复核确认件，全部落在 EVALFIX-2A..2E 批面上）。全部根因修，无词表创可贴；每条假阳性句先红后绿入 pin。验证：gofmt（触及文件全净；仓内先于本批的漂移文件维持不动）+ `go build ./...` + `go test ./...` 83 包 0 FAIL。

- **R6-3（high，2C 否定过匹配）**：zh 臂对 ≤4-rune 窗做裸 `strings.Contains(不/未)`，不仅/不但/不过 全被读成否定翻转；en 臂把括注里的 "not counting/not only" 读成比较词否定。修根（`mechanical_claim_check.go`）：否定判定改**三值 verdict**（none/flip/ambiguous）——zh 窗前向扫描、否定词按前缀最长匹配、闭集非否定复合词（不仅/不但/不过/不止/不光）读过不翻、否定词**紧邻比较词**（中间至多空白）才翻；en 先剥平衡括注（未闭合左括号=比较词在括注内→ambiguous）、"not only/just/merely" 为关联词不翻、否定词是窗内**末词**才翻。**歧义即整条弃判**（skip 标记保留在比较词上，其绑定仍计入争用普查——只更保守）：任何未知 不X 复合形都走 skip，类不因词表缺词而重开（漏词只损 recall 不损 precision）。
- **R6-4（high，2C 跨小句绑定）**：绑定循环只有 30-rune 缰绳，无小句边界，「在 10s 的窗口内，低于 16.67ms 的帧占比 95%」把上一小句的窗长绑成左操作数。修根：操作数候选的 gap 内出现闭集小句边界（，。；、,;.）即拒（近者被拒则更远者 gap 为其超集必同拒——整侧无操作数→该比较词弃判，不跨界绑定）；左侧另加「在…内/中/里」状语闭合臂（gap 含闭合字 + token 前缰绳内有 在 → 拒；误命中只弃判，零 precision 代价）。**【R7-0 更正（2026-07-30）：本臂的「零 precision 代价」断言已被第七轮证伪——该臂按「token 前缰绳内有 在」逐候选拒绝，并非整侧致死：位于 在-开启符之前的更远 token 其 gap 不满足此谓词，会跨过闭合字绑定成左操作数（「总耗时 80ms 在 10ms 的采样间隔内低于 16.67ms…」拒掉 10ms 后绑 80ms，对正确句制造假矛盾——该句 pre-R6 反而静默）。小句边界臂的子集/单调论证不受影响；状语臂已按同一单调性质重造为真屏障，见下文 R7-0 记录。】**括号刻意**不入**边界集（F7 见证 （80ms）未超过（16.67ms） 必须穿括号绑定）。skip-set 表新增 9 行（findings 全部六条复现句 + 无逗号状语形 + zh/en 歧义否定形），并配判别力正 pin（不仅/not only 保基向仍能抓真反向）。
- **R6-0（medium，2B memo key 不完备）**：memoized 核心经 `traceQueryAppendCallCaveats` 把 callCaveat 烙进结果，而其 targetCaveat 分量是**继承前**参数的函数——显式目标与继承目标调用在继承后参数相同、caveat 面不同，撞同一 key 后 memo 端上错误的目标来源披露。修根：`traceQueryMemoKey` 签名加 `callCaveat` 并入哈希（callCaveat=join(窗口归一 caveat, targetCaveat)，正是核心读到的那份输入；窗口分量本就是 p 的确定性函数，重复入哈希无害）。新 pin：显式/继承双调用 e2e（继承调用禁止命中显式条目）+ key 级不等断言。
- **R6-1（medium，2A 矛盾双命令）**：census 注 "remove every one of them" 不受 hint 门控，与同一消息里的 rename/relocate 指令对同一字段发出相反命令。修根（`strict_decode_params.go`）：census 与 remap/repair 共用**同一谓词** `strictDecodeHintFor`（谓词同源红线）；hint 命中时被点名字段离开 roster（它的指令在上文），仅剩其余 unknown 时换词 "the payload also carries these additional unknown fields — fix them in the same retry"（对其余字段的独立命令，不再言及被点名字段）；无剩余则整注静默。未命中车道字节恒等。
- **R6-2（low，2A 嵌套裸名误教）**：CanonicalName 行按裸字段名匹配而 Go unknown-field 错误无路径，嵌套 `source_inventory_profile.requested_files`（requested_fields 近失）被教成顶层改名。修根（`strict_decode_remap.go`）：新单源谓词 `strictDecodeHintFor` —— CanonicalName 行额外要求错名是 **raw payload 顶层成员**（`strictDecodeTopLevelKeyPresent`，解析失败/无 raw fail-closed 不发教学）；wrong-container 行维持裸名匹配（其主题本就是嵌套错位）。remap、`strictDecodeToolRepair`、schema 参数表门、census 门四个消费点全换该谓词（嵌套近失现在落 census did-you-mean + 参数表，repair 回 `tool_param_unknown_field`）。既有 2A 三 pin 改顶层 fixture，另配嵌套负 pin + container 行不受门影响 pin。
- **R6-5（low，2E 降级车道双披露）**：拒稿期的引文重写已入账，Run 走降级恢复车道出厂时，降级 caveat 的 `引用摘录回填` 词条与 footer 的同词条 ×N 同面双披露（§9 的「degraded_export 不入账」只挡了同事件双记账，挡不住跨来源同面双词）。裁定边界：vote1 在设计意图镜下 refuted——§10.7 已落账「footer 计的是本 Run 重写事件数（与 WARN 对齐）」为在案语义，故**不改记账/不改计数语义**，只把 §9 的双披露禁令补齐到跨来源：`MutableState.MarkDegradationLaneSelfDisclosed`（与账本同生命周期、pipeline-start 清）；两处降级 call site 经 `markSelfDisclosedDegradedSections` 在 produced 含 `citation_quote_backfill`（tool 层导出常量，灭字面手抄）时打标；footer 跳过打标车道。账本计数与 operator `[degrade] ledger:` 行**一字不动**（telemetry 永录）。pin：types 三态 + footer 抑制/未打标不抑制 + 结构配对 pin（agent 包内每个 Materialize… call site 6 行内必须跟 mark 调用，防未来车道重开）。

先红后绿证据：R6-3/R6-4 九行 skip-set + 判别力正 pin、R6-0 e2e pin、R6-1 双 census pin、R6-2 嵌套 remap/census 双 pin 均先对修前代码跑红（红形与 findings 复现句逐字一致）后转绿；R6-5 的 Mark 族为新 API（修前不可编译），红先以两名复核者的产线渲染链实测复现为证。

---

# 第七轮自查 sweep 修复记录（R7-0…R7-1，2026-07-30）

来源：round-7 sweep（两条确认件，均落在第六轮修复批自身——「修复批必进下轮 sweep 靶面」教训的直接收获：两条都是 R6-4/R6-5 现写代码里的缺陷）。全部根因修；两条复现场景先红后绿入 pin。

- **R7-0（high，2C 状语臂非整侧致死）**：R6-4 的 在…内 状语拒绝谓词 `mechanicalClaimLeftOperandAdverbialScoped` 按「gap 含闭合字 **且 token 前缰绳内有 在**」逐候选判——第二个合取项使谓词**非单调**：只有坐在状语**内部**的候选（在 在其前）被拒，位于 在-开启符**之前**的更远 token 不满足该项，照常跨过闭合字绑定。活体复现：「总耗时 80ms 在 10ms 的采样间隔内低于 16.67ms 的样本占比达 95%。」——pre-R6 静默（10ms 绑定，真命题），HEAD 上 10ms 被状语臂拒掉后 80ms 绑成左操作数 → 对正确句发假矛盾（红形 verbatim：`80ms 在 10ms 的采样间隔内低于 16.67ms` 判为 80ms ≤ 16.67ms 反向）。修根（`mechanical_claim_check.go`）：状语闭合字改造为与小句边界**同一子集/单调性质**的真 GAP 屏障 `mechanicalClaimAdverbialCloserBetween(sent, from, to)`——gap 内出现「闭合字（内/中/里）且其前缰绳内有配对 在」即拒，配对判定复用 之内/以内 准入门的 `mechanicalClaimWithinAdmitted`（谓词同源，无第二识别器）；近者 gap 是远者 gap 的子集 → 近候选被拒则整侧必死（side-fatal）→ 定语式比较词无侧内操作数即弃判。无配对 在 的词内闭合字（内存/其中/括注「（内部含 GC）」）保持透明。pin：复现句入 skip-set 表（先红后绿，红形与 finding 复现句逐字一致）+ 既有 在…内 两行维持绿 + 新判别力正 pin（`TestMechanicalClaim_AdverbialCloserNeedsPairedZai`：gap 内无配对 在 的闭合字不成屏障，真反向仍抓——杀「见闭合字即拒」的过宽修法）。上文 R6-4 记录的「零 precision 代价」断言同步落更正注（records must tell the truth）。
- **R7-1（medium，2E 打标跨稿存活）**：R6-5 的 `markSelfDisclosedDegradedSections` 在 **MATERIALIZE 时**打 run 级标，但降级恢复稿可能被拒（含 prose-empty fallthrough：mark 后 rendered=false 返回）——标记比被拒稿活得久，后续 CLEAN 重发稿带着陈腐标记渲染：footer 跳过 lane 而干净稿又无自披露 caveat → **披露全失**（恰是 R6-5 要建立的保护）。修根：抑制与**实际出厂的那份 doc** 耦合——run 级标记状态整族退役（`MutableState.MarkDegradationLaneSelfDisclosed`/`DegradationLaneSelfDisclosed`/`degradationSelfDisclosed` 字段/reset 臂全删），footer 渲染时改由 `degradationLaneSelfDisclosedInDoc(doc, spec)` 从**被渲染文档本身**推导：doc.Caveats 中存在「降级 caveat 逐字前导（`degradedSectionsCaveatPrefix{ZH,EN}` 常量，与 `degradedDeterministicSectionsCaveat` 构造端共享同一字面——谓词同源）+ 该 lane 注册表词面」即抑制，双语言面都查（与渲染语言参数解耦）。两个降级 call site 只保留「materialize → caveat 上稿」配对；被拒稿场景无状态可残留（结构性根除，而非在 emit 边界补 reset——后者仍留一份可漂移的 run 级状态）。词面同一性（section 展示词 == lane 注册表词，抑制谓词所倚）新增断言入 pin。pin：`TestDegradationFooterRejectedRecoveryThenCleanRetryDiscloses` 先红后绿（红形 verbatim：干净稿 footer 只剩 `必答面硬转软 ×1`，`引用摘录回填 ×17` 丢失）+ R6-5 抑制语义在 doc-derived 形下全保（同稿词面恰一次/EN 面/无 backfill 词条不抑制/账本计数一字不动）+ 结构配对 pin 重指（`TestDegradedMaterializeCallSitesAppendSelfDisclosingCaveat`：每个 Materialize… call site 12 行内必须跟 `degradedDeterministicSectionsCaveat`——caveat 现在既是用户面自披露也是 footer 抑制载体）+ types 侧 Mark 族 pin 退役（落注指向 agent 包新 pin）。

先红后绿证据：R7-0 复现句在修前 skip-set 上跑红（假矛盾 Detail 逐字复现 finding 场景）后转绿，既有 在…内 行全程绿；R7-1 以 HEAD 产线打标路径（`markSelfDisclosedDegradedSections` + 干净稿渲染）跑红（footer 丢 lane 的完整渲染输出在案）后按 doc-derived 终形重写为常驻 pin（该 pin 同时杀未来任何 run 级抑制状态的重引入）。验证：gofmt 触及文件全净 + `go build ./...` + `go test ./...` 83 包 0 FAIL。

---

# 第八轮自查 sweep 修复记录（R8-0…R8-1，2026-07-30）

来源：round-8 sweep（两条确认件，双复核者各自活体复现；两条都落在第七轮修复批自身——「修复批必进下轮 sweep 靶面」教训连续第二轮兑现：R8-0 在 R7-0 现写代码里，R8-1 在 R7-1 现写代码里）。全部根因修；两条复现场景先红后绿入 pin。

- **R8-0（medium，2C 闭合字配对被缰绳错界）**：R7-0 的 `mechanicalClaimAdverbialCloserBetween` 把配对-在 检索重锚到**闭合字位置**，却沿用了 30-rune 绑定缰绳作为检索上界（`mechanicalClaimWithinAdmitted` 的 leash 循环）——在…内 状语的描述语一旦把 在 推到闭合字 30 runes 之外，闭合字不被识别为屏障，而状语**内部**的量值自己的 gap 仍 ≤30 照常绑定：正确句发决定性假矛盾。活体复现：「在 3680.569s 的完整采样窗口（覆盖三个连续 vsync 刷新周期）内低于 16.67ms 的帧占比达 95%。」——gap 28 runes ≤ 缰绳故 3680.569s 绑成 低于 的左操作数，而 在 距 内 38 runes 配对失败（红形 verbatim：判为 `3680569ms ≤ 16.67ms` 反向，decisive）。修根（`mechanical_claim_check.go`）：**在-配对是小句属性，不是缰绳属性**——`mechanicalClaimWithinAdmitted` 检索上界从缰绳改为**上一个小句边界或句首**（复用 R6-4 闭集 `mechanicalClaimClauseBarrierRune`，谓词同源；新增 `mechanicalClaimNumericInternalPunct`：两侧皆 ASCII 数字的 '.'/',' 是数字内小数点/千分位（3680.569 / 16.67 / 1,234），对边界走查透明——否则修复句自身与既有绿行「实测 12ms 在 16.67ms 预算之内」会被数值内 '.' 截断检索反而变红）。之内/以内 准入门共享同一谓词同步受益（同类 30+ runes 描述语的真比较式此前漏准入，纯 recall 修复）；小句界化对「在 位于上一小句」的场景比缰绳形**更严**（跨逗号不再配对），词内 内/中/里 保护不变（就是配对-在 要求本身：内存/其中 无同小句 在 保持透明）。pin：复现句入 skip-set 表（`adverbial_long_descriptor_zh`，先红后绿，红形与 finding 复现句逐字一致）+ 既有 在…内 三行（R6-4 两行 + R7-0 side-fatal 行）与 R7 判别力正 pin（`TestMechanicalClaim_AdverbialCloserNeedsPairedZai`）全程绿。
- **R8-1（low，2E 抑制谓词吃 LLM 自由文本）**：R7-1 的 `degradationLaneSelfDisclosedInDoc` 对 `doc.Caveats` 做 `strings.Contains` 匹配降级 caveat 前导——但 Caveats 是 **LLM 可写自由文本通道**：一条仅仅**引用**了前导字样 + lane 词面的 caveat（如叙述上游批次曾降级出厂的说明性文字）就把合法 footer 假抑制掉，非降级稿披露丢失（活体探针复现；正是「嘈声信号驱动硬行为」类）。修根（`degradation_disclosure_footer.go`）：抑制信号改锚**系统作者身份**——降级 caveat 由 `degradedDeterministicSectionsCaveat` 程序化构造，前导恒在 caveat 条目 **position 0**（prefix+details verbatim 返回），故前导匹配改 `strings.HasPrefix`（条目起始）；LLM 引用必然把前导嵌在文中（引号/叙述之后），不再命中。lane 词面仍是 Contains，但只在**已过前缀认证的条目内**查——过门后条目主体是系统作词（section 展示词），无自由文本噪声。载体 `Caveats []string` 无 typed/system 字段位（加字段需动 LLM emission schema，超出本件边界且引入第二披露载体——不取）。pin：`TestDegradationFooterQuotingCaveatDoesNotSuppress` 先红后绿（红形 verbatim：引用性 caveat 下 footer 只剩 `必答面硬转软 ×1`，`引用摘录回填 ×17` 被假抑制；zh/en 双面）+ 判别臂同 pin 内钉死（真降级 caveat 前导在 position 0 仍抑制——杀「干脆不抑制」的过宽修法）+ R7-1 全家族 pin（同稿恰一次/EN 面/被拒稿后干净稿披露/结构配对）全程绿。

先红后绿证据：R8-0 复现句在修前 skip-set 上跑红（Detail verbatim：`「3680.569s 的完整采样窗口（覆盖三个连续 vsync 刷新周期）内低于 16.67ms」…声称 3680569ms ≤ 16.67ms，但数值本身 3680569ms > 16.67ms`）后转绿；R8-1 引用性 caveat 探针在修前跑红（footer 输出 verbatim：`系统降级披露：必答面硬转软 ×1（完整明细见运行日志）`——citation lane 丢失）后转绿。验证：gofmt 触及文件全净 + `go build ./...` + `go test ./...` 全量 0 FAIL（本轮与另一并行批次共库跑，橙区文件互不触碰）。
