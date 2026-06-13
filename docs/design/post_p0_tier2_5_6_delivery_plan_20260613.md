# Post-P0 Tier-2 #5/#6 Delivery Plan (2026-06-13)

承接 `docs/design/post_p0_roadmap_20260612.md` 第二梯队 #5/#6。本文是本批次的设计账本、执行任务表、验收标准和进度记录。原则沿用 P0 红线：精确信号进硬门禁，嘈声只做软引导；不基于用户散文或模型输出关键字做逻辑判断；不把 case 答案形态写进 prompt；不重复造轮子；先复用已有严格 JSON 修复层、tracequery windowed index、TurnA handoff、normalizer/repomap resolver。

## Scope

| Item | Goal | Current finding |
|---|---|---|
| #5 strict decode backlog | 消化 `strict_decode_registry_test` 的 18 个豁免工具，统一接入 strict decode + schema-driven repair | `repo_map` 和 `trace_query` 已经是参考实现：先 `ApplyStructuredPayloadCompat` / `applyStructuredPayloadCompat`，再 `json.Decoder.DisallowUnknownFields`，失败走 `FailStrictDecodeWithError` / `failStrictDecodeWithError`。剩余豁免是历史 lenient decode 或 bespoke normalize |
| #6a tracequery auto-window | 4 个新增重型视图在大 trace 且带 `pattern`/`span_name` 时必须先 stream search 选窗，再 windowed index 执行 | `thread_timeline`/`ipc_graph`/`wakeup_chain`/`interaction_stats` 已被 heavy guard 识别，但 `traceQueryShouldAutoWindowFromPattern` 未纳入；因为 heavy guard 对带 pattern 的请求放行，这些视图会退回大文件 full index，是 OOM 路径最后一角 |
| #6b TurnA fork merge bound | `mergeTurnAArtifactsForMutable` 并发 fork 汇总点补 count/byte 界，防止 sibling deltas 无界复制进 extractor/finalizer handoff | `internal/agent/turn_a_merge.go` 已有跨窗口 `ToolResults` 上限，但 types 包 fork merge 仍直接 append `ToolResults`/`MCPResponses` 增量 |
| #6c tokenizer suffix peel-lite | ASCII 派生词形候选，如 `extractor` -> `extract`，提升 hit@k；不碰 CJK 路径 | normalizer 当前只有复数 collapse 和 resolver 精确提升。agent symbol resolver 有 role suffix/action prefix，但没有派生词形 exact candidate path |

## System Gaps

1. **Strict decode 接入粒度不统一**  
   根因不是某个工具 schema 文案，而是工具层曾各自解析 JSON。结果是同类模型 JSON 形态错误在不同工具上表现不同：有的结构化修复、有的直接 lenient 接受、有的返回普通错误。系统解法是抽出统一 decode helper，所有工具复用同一顺序：schema-driven normalize -> strict decoder -> typed repair。修复只读 JSON schema 扩展和 Go decoder 错误，不读取用户意图/模型散文。

2. **大 trace pattern scope 没有统一接入重型视图调度**  
   heavy guard 能识别新视图，但 auto-window 视图集合没有同步扩展。带 pattern 的重型调用被认为“有 scope”而绕过 guard，却又不走 stream search + windowed parse，最终 full index。系统解法是把“pattern-windowable heavy views”作为单一结构化集合，guard、auto-window、multi-candidate 策略共享，避免每次新增视图漏一个分支。

3. **TurnA handoff fork merge 缺少总量背压**  
   跨窗口 merge 已经有上限，但并发 fork merge 点没有。根因是 handoff carrier 既承载 rich evidence，又承载工具/外部资源历史，缺少统一 byte accounting。系统解法是把 ToolResult/MCPResponse 的 handoff byte accounting 放进 `internal/types`，在所有 TurnA merge 点复用；裁剪 oldest first，同时保留至少一个高价值 typed/成功结果，避免破坏 structural-empty gate 和外部观察 handoff。

4. **词形派生残差由模型背负**  
   `extractor`/`extract`、`resolver`/`resolve` 这类英语派生词应该在 tokenizer/normalizer 层结构化处理，而不是让模型在 prompt 里记忆。系统解法是 ASCII-only suffix candidate 生成器，只在 exact resolver 失败后使用，候选仍必须命中 repomap symbol table；CJK 和普通语义不进该路径。

5. **性能/内存观测缺少阶段内统一维度**  
   进程级大仓韧性已经有 `GOMEMLIMIT` 和 repomap resume，但 tracequery/repomap 的阶段日志以 elapsed 为主，heap/RSS 风险需要从系统日志间接推断。系统解法是给重型工具阶段加低成本 memory sample：heap alloc/sys、GC count、trace/repo file size、window/full index 标志、events/files count。它只做日志和 ToolResult caveat/observation 的软诊断，不参与硬 gate。

6. **高效工具使用审计仍偏离线**  
   eval 侧有 `tool_usage_compare.py` 能看 repo_map/trace_query/read_file/grep 方向性，但运行时 prompt/handoff 对“先用高效结构化工具”的反馈需要更稳定。系统解法是把工具自己的输出写清可执行 next-call hints，并在 eval/日志分析阶段审计工具序列；生产逻辑不做“用户问了某词就强制某工具”的关键字路由。

## Design

### #5 Unified Strict Decode

- 新增共享 helper：`decodeToolParamsStrict(toolName, raw, schema, dst, hints)`，内部复用现有 `applyStructuredPayloadCompat`、`json.Decoder.DisallowUnknownFields`、`failStrictDecodeWithError`。
- out-of-package 工具继续使用已存在 facade：`tool.ApplyStructuredPayloadCompat` / `tool.FailStrictDecodeWithError`。
- repair 来源只允许三类精确信号：
  - JSON decoder structural error；
  - JSON schema + `x-codrax-*` 注解；
  - typed `MisplacedFieldHint`。
- 禁止从用户请求或模型散文里判断“这是哪个工具意图”。不做关键字匹配。
- 分批消化 `strictDecodeExemptTools`：
  - Batch 5A：只读/命令工具：`exec_command`、`grep`、`read_file`、`list_files`、`run_tests`。
  - Batch 5B：Git/memory/sub-agent/multi-repo focus：`git_diff`、`git_show`、`git_log`、`git_history_search`、`recall_memory`、`list_memory`、`propose_sub_agents`、`emit_multi_repo_focus`。
  - Batch 5C：analysis/log/perf emitters：`emit_analysis`、`emit_investigation_complete`、`emit_log_triage`、`emit_log_segmentation`、`emit_perf_segmentation`。
- 每批移除对应 exemption，并跑 `go test ./internal/tool -run TestAllToolExecutePathsUseStrictDecodeOrFacade`。

### #6a Tracequery Pattern Auto-Window

- 引入单一 predicate：`traceQueryPatternWindowableHeavyView(view)`。它覆盖现有 pattern-windowable views，并纳入 `thread_timeline`、`ipc_graph`、`wakeup_chain`、`interaction_stats`。
- 大 trace + no explicit time/line window + pattern/span:
  1. `StreamEventSearch` 只做 literal substring discovery；
  2. 从 timestamped rows 生成最多 3 个 bounded windows；
  3. 对每个候选 window 使用 `BuildIndexWithOptions(AllowWindowedParse=true)`；
  4. 执行 requested view；
  5. summary/JSON payload/typed observations 保留 `selected_window`、`index_windowed=true`、candidate rank、raw payload ref。
- 新视图多候选策略：若 pattern 命中多个 timestamped rows，最多跑 3 个 windowed candidates；这是有界替代 full index，不降低答案丰富度。
- heavy guard 仍保护无 pattern/span 且无 time/line window 的大 trace。
- 追加性能/内存 debug log：auto-window search/build/run、full build/run、stream search 都记录 elapsed、events、windowed、heap alloc/sys、GC count。

### #6b TurnA Fork Merge Bounds

- 将 ToolResult byte accounting 从 agent 层提升到 `internal/types`，作为 TurnA handoff 公共能力。
- 新增 `BoundTurnAToolResults(results, countCap, byteCap, preserveFn)`：
  - newest-first 保留；
  - oldest-first 输出顺序；
  - 超 cap 时保留一个 `preserveFn` 命中的结果；
  - `preserveFn` 在 agent 跨窗口 merge 继续使用 investigation-class 成功工具；
  - types fork merge 使用成功且有 summary/raw/observations 的结果，保证外部 trace/log/MCP 观察不会全丢。
- 新增 `BoundTurnAMCPResponses`，同样 count/byte cap、oldest drop、至少保留一个成功且有 raw/payload/observations 的 response。
- `mergeTurnAArtifactsForMutable` 在 append fork deltas 后统一 bound `ToolResults` 和 `MCPResponses`，不裁 `EvidenceItems`、`FlowFindings`、`SourceInventoryObservation` 等 typed carriers。
- summary 里不注入“系统补丁答案”；只在 internal logs/debug 或 bounded handoff marker 里记录 dropped count/bytes，避免污染 final answer。

### #6c ASCII Suffix Peel-Lite

- 在 normalizer 增加 `LookupSymbolMorphAlias` 可选 resolver 能力，不改变基础 `SymbolResolver` 接口的零值行为。
- 候选生成规则：
  - ASCII-only，长度 floor，CJK/混合 rune 直接跳过；
  - suffix set 初始只覆盖派生后缀：`er`、`or`、`ion`、`ing`、`ed`；
  - 每条规则生成少量候选，如 `extractor -> extract`、`resolver -> resolve`、`parsing -> parse`；
  - 候选必须 `LookupSymbol(candidate)` 命中 repomap exact/flat symbol table才可提升；
  - 多候选返回按确定性顺序，调用方沿用 rarity confidence。
- 此路径是 lexical normalization，不看用户意图关键字，不看模型输出 prose。
- 增加 unit test 和 benchmark，锁定 CJK 不触发、短词不触发、多候选 bounded。

### Tool Efficiency, Performance, Memory, Handoff

- 工具积极性：保持 prompt 层已有 “Use repo_map/trace_query before ad-hoc grep/awk” 软引导；本批不增加硬路由。eval 后用 `tool_usage_compare.py` 和 representative cases 审计 repo_map/trace_query 使用率、read_file/grep 下降趋势。
- 性能：tracequery 加阶段 log；repomap 已有 source/cache/full/in-memory banner，本批设计不重写 scanner，只在 #5/#6 不引入额外全量扫描。
- 内存：进程级 `GOMEMLIMIT` 已在 `cmd/root.go` 和 `docs/design/large_repo_memory_resilience.md`；本批补工具阶段 heap sample，定位 peak phase。
- Handoff：tracequery typed observations、payload refs、raw refs、candidate windows 必须从前序阶段带到 observation ledger；TurnA bound 优先保留 typed observation-bearing results。
- JSON 修复：所有本批工具输出参数接入统一 strict repair，减少模型对各工具格式差异的心智负担。

## Task List

| Batch | Tasks | Verification | Status |
|---|---|---|---|
| Design | 落本文档；记录 root cause、方案、任务、验收 | doc commit + push | Done |
| 6A | tracequery pattern-windowable heavy view predicate；纳入 4 新视图；多候选 bounded windows；阶段 memory log；新增 tests | `go test ./internal/tool -run 'TestTraceQueryLarge(NewHeavyViews|SpanKeyword|UnboundedNewHeavyViews|NewHeavyViewsBounded)|TestTraceQueryLargeNewHeavyViewsWithPatternAutoWindow|TestTraceQueryLargeNewHeavyViewsPatternMultiCandidateBounded'` | Done |
| 6B | 提升 ToolResult bound utility 到 types；fork merge bound ToolResults/MCPResponses；新增 tests；agent merge 改复用 | `go test ./internal/types ./internal/agent -run 'TurnAArtifacts|MergeTurnAArtifacts|BoundTurnA'` | Done |
| 6C | normalizer morph alias optional resolver；repomap resolver 实现；unit/bench | `go test ./internal/analysis/normalizer ./internal/agent -run 'Normalize|LookupSymbol|MorphAlias'`; `go test ./internal/analysis/normalizer -bench MorphAliasCandidates -run '^$'` | Done |
| 5A | strict decode Batch 5A | `go test ./internal/tool -run 'StrictDecodeRegistry|Builtin|RunTests'` | Pending |
| 5B | strict decode Batch 5B | `go test ./internal/tool -run 'StrictDecodeRegistry|Git|Memory|SubAgents|MultiRepo'` | Pending |
| 5C | strict decode Batch 5C | `go test ./internal/tool -run 'StrictDecodeRegistry|EmitAnalysis|EmitInvestigation|LogSegmentation|PerfSegmentation'` | Pending |
| Final eval | 挑代表 eval，每次并发 2；人工审计答案/日志/工具使用/性能内存 | representative eval logs + gap update | Pending |

## Acceptance Criteria

- `strict_decode_registry_test` 豁免列表清空，或只剩有明确 upstream blocker 且本文记录 owner 的临时项。
- 大 trace + `pattern`/`span_name` + 4 新 heavy views 不构建 full index；summary 和 logs 明示 windowed index。
- 无 pattern/span/time/line 的大 trace heavy view 仍 hit guard，不解析全文件。
- TurnA fork merge 在 pathological sibling deltas 下有 count/byte 上限，同时保留至少一个 successful typed handoff carrier。
- suffix peel-lite 只影响 ASCII lexical candidates，CJK 路径和普通未知词保持原样。
- representative eval 中 repo_map/trace_query 使用率不退化；read_file/grep 不因本批新增 fallback 增长。
- 所有 hard gates 只读 typed flags / decoder errors / schema annotations / integer comparisons；没有用户散文或模型 prose keyword hard gate。

## Progress

- 2026-06-13: 读取 roadmap、strict decode registry、strict repair layer、tracequery large-trace paths、TurnA merge、normalizer/repomap resolver。确认 #6a/#6b/#6c 均可复用现有系统层能力，无需 case patch。
- 2026-06-13: 6A 交付。4 个新增重型视图 `thread_timeline` / `ipc_graph` / `wakeup_chain` / `interaction_stats` 统一进入 pattern auto-window；多 pattern candidate 走最多 3 个 bounded windows；tracequery build/run/stream/auto-window 阶段日志增加 heap alloc/sys 与 GC count。Focused tests 通过。
- 2026-06-13: 6B 交付。TurnA ToolResult byte accounting 提升到 `internal/types`；agent 跨窗口 merge 改复用公共 helper；`mergeTurnAArtifactsForMutable` 并发 fork 汇总点对 ToolResults/MCPResponses 增加 count+byte bound，保留 payload-bearing successful carrier。Focused tests 通过。
- 2026-06-13: 6C 交付。normalizer 增加 ASCII-only bounded morph candidates；repomap 单仓/多仓 resolver 实现 optional `LookupSymbolMorphAlias`，候选必须 exact/flat symbol-table 命中才提升。Focused tests 通过；`BenchmarkMorphAliasCandidates` 约 59ns/op。
