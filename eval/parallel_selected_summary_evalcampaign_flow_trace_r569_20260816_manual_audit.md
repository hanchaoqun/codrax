# r569 人工审计：空表 Flow 补采 × 有限 Trace CPU/策略绑定

- 基线：`main@b3ecca512`
- 运行：`PARALLEL=2 RUNS=1`
- 案例：
  - `read_combo_answer_document_tools`
  - `real_trace_h4_supply_thermal_witness`
- Runner：`1 PASS / 1 FAIL`
- 人工：`2 partial`

## 1. Combo：B907 生效，但请求作用域关系仍未闭环

### 1.1 正向证据

1. B907 的空 participant 软导航在生产回放中生效。第一次 completion 发现缺少 operation carrier 后，
   模型被引导到 `cmd/root.go:4315/4319`，读取并发出了两条真实
   `toolRegistry -> &tool.EmitAnswerDocument{}` / `toolRegistry -> &tool.EmitAnswerDocumentPatch{}`
   registration 证据；r568 的“一次错误 grep 后直接 node-only”不再复现。
2. 两个 `Name()` literal、首次 full / retry patch 的基本分工和对比表均保留；终稿 Mermaid 语法有效，
   四条最终可见边都有显式 `edge_anchors`，未靠系统制造关系。
3. B905 的 fail-closed 红线仍在：第一稿自行画出的 guard/precedence/dispatch 边没有对应 typed 关系，
   validator 正确拒绝；真实 registration/return identity 完整补齐后才接受。

### 1.2 新确认 B908（P1）：Flow operation 有证据，但与请求的关系作用域错位

1. 用户问的是“两工具在 finalizer 里的关系”。Analyzer 虽保留
   `diagram_hint.relation_scope_quote` 和 `finalizer` participant，但把 `finalizer` 错标为
   `incident_required`；该概念没有唯一 parser symbol，participant repair 按设计跳过不可精确升级的概念。
2. Explorer 已读取真正的 `answerDocumentEvaluator.FilterToolSchemas` 条件分支，并发出多个
   `evidence_kind=conditional` 行；这些行只有 condition/anchor，没有显式 subject/object/predicate，
   因而没有形成可供 Finalizer 复制的 guard/selection typed edge。
3. `HasFlowOperationEvidenceForRequest` 只要求当前 AxisFlow 至少有一条合法 operation，全球初始化处的
   registration 已满足该门；它没有证明“在 finalizer 里如何选择”。调查因此在“有真实边但关系作用域不对”
   的状态签收。
4. 终稿图只表达注册和 `Name()` 返回，`finalizer` 是无箭头孤立节点，正文仍宣称两工具“通过
   FilterToolSchemas 在 finalizer 阶段被筛选”，但图没有表达 full/patch 的选择、切换或 guard 关系。
   因此 Runner PASS 不能升级为人工 pass。

### 1.3 次级效率/JSON 信号

1. Explorer 22 轮、8 次 read、18 次 midloop。operation 补采本身只占读取注册点的一个回合；随后
   bounded semantic-descent 从 `FilterToolSchemas` 串行追到 `answerDocumentPatchBaseAvailable`、
   `answerDocumentPatchBaseDocumentInMutable`，每层又经历 read→emit_evidence→completion，产生明显放大。
   这些 helper 对解释 patch-base 可用性有价值，但当前逐层一次一项的施工形成本较高，记 P2 效率债，
   不通过降低证据杆或跳过函数体解决。
2. Finalizer 5 次拒绝中有两次是 `add_blocks` / `replace_blocks` 被模型发成畸形 JSON 字符串；系统未猜改
   结构，正确 fail-closed，并给出 native array 指引。另三次是未证关系或 identity 字段遗漏。
   当前属于 JSON 教学/补丁认知负担的重复生产信号，但不是系统吞答案；继续与异构 patch 案合并观察。

### 1.4 最优方案边界

1. Analyzer JSON 教学统一说明：用于限定“在哪个阶段/模块/容器内”的 surrounding scope，若用户没有把它
   作为关系端点，应使用 `context_only`；真正需要 incident edge 的实体才用 `incident_required`。
   这是跨语言、跨图族角色语义，不针对 `finalizer` 字符串。
2. Explorer 对已读取的条件/选择操作提供更短的 typed endpoint 教学：条件事实若要承担 requested flow
   relation，必须由模型显式给出 enclosing callable、condition/selected schema 的 subject/object/predicate；
   普通 supporting condition 不硬性要求关系端点。
3. Completion 的“存在任意 operation”与“请求作用域关系已覆盖”必须分轴。只有 typed context participant
   能被 parser identity 精确解析时，才可要求 operation 与该 scope 同 owner/file/callable；无法解析时继续
   诚实 unproven，禁止扫描 `relation_scope_quote` 或用户/模型文本猜 scope，也禁止系统造 selection edge。

## 2. H4：有限 Trace 车道正确，CPU/策略对照仍被模型跨行拼接

### 2.1 正向证据

1. 请求只要显式窗内运行量、四态和一个频率影响判定；系统保持
   `bounded_fact_set`/有限车道，3 次 `trace_query` 中只有 timeline/resource 类视图，零
   root-cause/wakeup、零完整 Trace 因果投影。这里“不生成完整投影”是正确 breadth，不是能力丢失。
2. 正文正确给出窗口 `233.190ms`，running `157.248ms`、runnable `5.604ms`、sleep
   `70.338ms`、D/IO `0ms`，并列出 8 个运行 CPU、CPU12 `96.081ms`。
3. 结论边界方向正确：CPU4 的 policy ceiling 存在，但目标 slice 是否真正受到该上限的性能约束仍未证。

### 2.2 新确认 B909（P1）：同 CPU typed join 已在场，但呈现给模型的认知结构仍允许跨行串值

1. Finalizer 上下文已经精确携带：
   - CPU4：target running `35.960ms`，same-CPU representative frequency `558000kHz`，policy
     `558000..2100000kHz/28 rows`，target binding 未证；
   - CPU12：target running `96.081ms`，representative frequency `2075000kHz`，same-CPU policy absent，
     不得借 CPU4 policy 比较；
   - CPU0：只有 policy witness，完整 target-running roster 中目标 absent。
2. 模型仍把 CPU12 的 `2075000kHz` 与 CPU4 的 `2100000kHz` 上限拼成一对，并在正文写成
   “CPU4 观察频率 2075000kHz”；同页系统 typed 事实附注则正确列出 CPU4 只有 `558/640MHz`。
3. 模型只执行了 `pattern=cpu=12` 的 event_search 零匹配，却把查询覆盖范围扩写成“窗口内未捕获到
   cpu_frequency / cpu_frequency_limits 行”；这与 CPU4 的 28 条 direct policy witness 直接冲突。
4. 因此 Runner 的 limit/binding regex 失败不能简单归为词面误报。状态墙钟与最终 unproven 结论可用，
   但支撑它的 CPU 频率事实有物质错误，人工只能判 partial。

### 2.3 最优方案边界

1. 不增加正文关键词扫描/替换，不让系统改写“受限/未受限”结论。把既有 typed join 改成低认知负担的
   same-CPU 独立矩阵：一行只含 target/window/CPU/running/frequency/policy/comparison authority，并明确
   禁止跨行借值；模型仍自行形成 yes/no/mixed/unproven。
2. 零结果覆盖必须保留完整 typed query selector。后续审计是否已有 `event_search` scope carrier可供
   Finalizer直接看见；若已有则只加强结构化并置，若缺失再补 typed coverage 字段，禁止从模型/答案原文
   推断查询范围。
3. H4 oracle 暂不扩词。先让同 CPU矩阵生产回放；只有事实全对而等价自然语言仍被拒时，才把 oracle 当独立
   二级件处理。

## 3. 红线复核

- 没有 active-stream 4ms/固定年龄降级、空答案或旧稿恢复。
- 不扫描用户、模型 thought 或最终答案原文作 hard gate。
- 系统不替模型选择关系、画图、判定 CPU 是否受限或写根因。
- 有限 Trace 不误扩成完整因果诊断；真正 causal-diagnosis 的显式窗投影、自动补齐、链上-only 主因、
  优先级反转/调度延迟/算力供给/D/IO/确定性语义/链上业务线索与实际占用、规则计价双轴均未改动；
  off-chain 邻近/背景仍只能作 support。

## 4. 排期

1. N1 / B909：将现有 same-CPU join 压成独立矩阵并补错窗/跨 CPU 负 pin；风险小、Trace 事实收益高。
2. N2 / B908：统一 surrounding-context participant 教学，并给条件/选择证据补 endpoint 形成路径；先做
   typed prompt/contract pin，再决定是否需要更窄的 completion scope 门。
3. N3 / P2：串行 semantic-descent 批量化与 answer patch JSON 心智减负；必须用异构案例证明不是降证据杆。

