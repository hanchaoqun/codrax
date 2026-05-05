> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# B6-F6 / B6-F7 设计 + 影响分析

**状态**:design-only,未开发。审计通过后再开 commit。
**起草**:2026-05-04
**关联**:`post_shape_retirement_consolidated_audit.md` §8 Batch B6 残留两项 perf fix。

---

## 0. 共同背景

两项都是 **性能优化**,不是 correctness 修复 — B6-F1..F5/F8 已经把 correctness/observability 主体收掉了。F6/F7 要解决的是 audit 里的 P26/P27 (慢) 和 P13 (retry 全量重发 token 浪费):

| Audit P# | 现象 | F6/F7 哪个负责 |
|---|---|---|
| P27 | analyze 阶段固定 1.5-3.5 min | F6 (pre-scan prompt cache) |
| P26 | 总耗时 5-15 min/run | 两者都贡献,但本质是 LLM 单次调用慢 |
| P13 | finalizer retry 每次全量重发 5-30 KB payload | F7 (retained-draft retry) |

**红线** (两个设计都必须遵守):
- `feedback_no_internal_info_in_llm_prompts.md` — 优化通信路径不允许往 prompt 里塞解释。
- `feedback_root_cause_only.md` — F6 不能把"prompt 太大"用"截短"绕过;F7 不能把"全量重发"用"删 retry"绕过。
- `feedback_eliminate_noise_at_source.md` — F6 的根因是"系统重复发同一段静态 prompt",F7 的根因是"retry 把 LLM 可保留的 in-flight 状态扔了"。两个都要从源头消除,不要在下游加 cache 层。
- L1 read mode byte-identity — 两个改动都不能改变默认行为下的 LLM-facing 内容。

---

## 1. B6-F6 — 分析器 pre-scan prompt cache

### 1.1 问题陈述

`analyze` 阶段最多跑 N 个 pre-scan rounds (默认 `MaxPrescanRounds=2`,多 sub-topic 时可升到 4)。每一 round:

- 组装一个 `system` message: skill (Goal / Workflow / OutputFormat / Prohibitions) + repo_map task_map overview + EvidencePlan 已知 hints。
- 组装若干 `user` / `tool` messages: 上一 round 的 tool 结果。
- 调用 `llm.Chat(...)` (流式)。

**问题**:`system` message 的 90%+ 是跨 round 不变的(skill 静态部分 + repo_map overview),但每次都重新发送。两个直接成本:
1. 网络 + tokenization 时间 (每 round ~1-3 秒;多 round 累加 1.5-3.5 分钟)
2. provider 计费按 input tokens 走,N 次重复 = N × cost

**补充观察**:跑 8 runs 的 log 显示 round 1 的 `INIT msg role=system len=36968` 和 round 2 的 system msg byte-identical。

### 1.2 候选方案 (4 个,按改动幅度排序)

#### A. provider 端 prompt caching (推荐)

**做法**:
- 给 `llm.Adapter` 加 `SupportsPromptCache() bool` + `ChatOptions.CachePolicy` (`{cache_breakpoints: []int}` — 索引指向 `[]Message` 中要做缓存边界的位置)。
- `internal/llm/openai.go` 的 OpenAI / Anthropic adapter 实现:
  - **Anthropic Sonnet/Opus**: 在 system message 的最后一个 `content` block 上加 `{"type":"text","text":"...","cache_control":{"type":"ephemeral"}}`,需要 `anthropic-version: 2023-06-01` + `anthropic-beta: prompt-caching-2024-07-31`。最少 1024 tokens 才生效,5 分钟 TTL,命中节省 ~90% input cost。
  - **OpenAI gpt-4o / o1**: 自动 prompt caching (zero config — provider 检测 prefix 重复),不需要 client 标记;`Usage.PromptTokensDetails.CachedTokens` 字段返回命中数,记 telemetry 即可。
  - **DeepSeek / 通义 / 国产**: 多数无 prompt cache;实现侧 `SupportsPromptCache() = false`,走方案 D 兜底。
- `analyzerEvaluator.BuildInitialInstruction` 不变 (它只产 dynamic 部分,system 由 builder.go 组装)。
- `internal/context/builder.go::RenderMessages` 在产出 system message 时,如果 adapter 支持 cache,把 system 和 repo_map overview 合到一个 message 里,在末尾打 cache breakpoint。

**优点**:
- 真根因:provider 已有的能力,只是没启用。
- 零 prompt-shape 变化 → byte-identical LLM-facing 内容 (L1 红线满足)。
- 兼容多 provider:不支持的 adapter 自动 fallback (零行为变化)。

**缺点**:
- Anthropic API 需要 beta header + 最少 1024 tokens;短 system 不生效。
- TTL 5 分钟,长 retry 链 (B5 retry 中间过去 8 分钟) 会失效。但 analyze 阶段 round 1→2 通常 < 30 秒,正中 TTL。
- DeepSeek / 通义 没 cache → 国产用户拿不到收益,需要方案 D 兜底。

**预期收益**:analyze 阶段 round 2 / round 3 的 input cost 砍 ~90% (Anthropic),OpenAI 自动 ~50% (cached prefix tokens 收 50% 价)。墙钟时间收益 25-50% (后端 cache 命中也省 tokenization 时间)。

#### B. 单次 wide-context call (合并 round 1+2)

**做法**:把 round 1 + round 2 合并成一次 LLM 调用 — system + objective + repo_map overview 一次发,LLM 在同一个 turn 里既调用 pre-scan tools (repo_map / grep / list_files) 又输出 emit_analysis。

**优点**:
- 削掉 1 次 round-trip (~3-15 秒)。
- 不依赖 provider 能力。

**缺点**:
- 改 ReAct 协议:目前 evaluator.ShouldStop 卡 round 数,合并要重写。
- LLM 经验:小模型在一个 turn 里同时 batch 多 tool-use + emit 容易乱。需要 prompt 加 "你可以先调用 ≤3 个 tool 拿信息,然后立刻 emit_analysis" — 但这违反了 `feedback_no_internal_info_in_llm_prompts.md` (内部 round 数对 LLM 不该可见)。
- 不同模型行为差异极大 (claude vs deepseek vs gpt-4o 对 batch tool-use 的支持差很多),会引入跨 provider 的不稳定。

**结论**:不推荐 — 削墙钟但增加协议风险 + 跨 provider 不稳定。

#### C. 客户端 disk cache (基于 prompt hash)

**做法**:把 system message + repo_map overview 用 SHA256 哈希,在 `~/.codrax/cache/llm-prompt/<hash>.json` 缓存对应的 LLM response。下次 hash 一致 → 跳过 Chat 直接用缓存。

**优点**:
- 完全 client-side,不依赖 provider。
- 跨 Run 也命中 (不是 5 分钟 TTL)。

**缺点**:**致命**。LLM 的 response 不是函数式 — 同样的 prompt 不同 round 看到不同的 tool result history,出来的 emit_analysis 不一样。把第 N 次的 response 套到第 N+1 次 = 错误的 round-2 输出 = 全 Run 走错路。
- 唯一能 cache 的是 round 1 的"无 tool history 输入" → 但 round 1 的 system 也带 question text,question 不同就不命中 → cache hit ratio ≈ 0。

**结论**:**否决**。Cache LLM 输出违反 ReAct 的因果链 (`feedback_root_cause_only.md`)。

#### D. system message 复用 (零 cache,纯客户端逻辑)

**做法**:`buildInitialMessages` 把 round 1 build 出来的 system message 缓存在 `MutableState.cachedAnalyzerSystemMsg`。Round 2 复用这个对象 (不重新 assemble),只追加新 tool result。前提:assemble 的输入(skill / objective / RepoRoot)在两个 round 之间不变。

**优点**:
- 节省 2-50 ms per round (assemble 自身开销),不依赖 provider。
- 零行为变化。

**缺点**:
- assemble 不是慢点 — 慢点是发送 + tokenization (server-side)。墙钟收益 < 1%。
- 不解决 input token 重发 cost。

**结论**:**不值得做** — 收益太小,加复杂度。

### 1.3 推荐方案 + 实施

**方案 A** (provider-端 prompt caching) + **方案 D** 跨 provider 兜底:不支持 prompt cache 的 adapter 走 D 节省 client-side assemble 时间(虽然小,但聊胜于无)。

实施切片 (~8-10 commits):

| commit | 范围 | 风险 |
|---|---|---|
| F6-c1 | `Adapter.SupportsPromptCache()` + `ChatOptions.CacheBreakpoints` 接口扩展;所有 adapter 默认 false (零行为变化) | 低 |
| F6-c2 | `ProvidersConfig` yaml 加 `prompt_cache_enabled` 知 旋钮 (默认 nil → 跟 adapter 默认走) | 低 |
| F6-c3 | `internal/llm/openai.go::Chat` 的 Anthropic 分支:当 cache_breakpoints 非空 + adapter 支持时,在 messages JSON 中给指定位置加 `cache_control:ephemeral` 并发 beta header;读 response usage 的 cache hit 字段 | 中 (需要真 Anthropic 端测试) |
| F6-c4 | OpenAI 分支:不需要 client 标记,只读 `usage.prompt_tokens_details.cached_tokens` 写 telemetry | 低 |
| F6-c5 | analyzer 调度路径在 round ≥ 2 时,组装 ChatOptions 时塞 cache breakpoint (索引 = system messages 的最后一个) | 中 |
| F6-c6 | `internal/llm` 单测 + 一个端到端 cassette 测试(用 httptest 模拟 Anthropic prompt-caching 响应) | 中 |
| F6-c7 | telemetry: 在 `[CGEC] summary` 后加 `cache_hits=N/M` 行;失败时 (cache miss / beta header rejected) 记 INFO 不报错 | 低 |
| F6-c8 | docs/architecture.md 新增"prompt cache"小节;codrax.yaml 文档说明启用方式 | 低 |

### 1.4 影响分析

**正向**:
- analyze 阶段 round 2/3 input cost ~90% off (Anthropic),~50% off (OpenAI cached path)。
- 墙钟时间预期减少 25-50% on the analyze stage,对应 0.4-1.5 min/run 收益。
- 跨 Run learning 不受影响 (不是新缓存层,只是 provider 端透明加速)。

**负向 / 风险**:
- Anthropic beta header (`anthropic-beta: prompt-caching-2024-07-31`) 是非 GA API,长期可能变。需要在 README 注明依赖的 provider feature flag。
- 国产 provider (DeepSeek / 通义 / 智谱) 多数无 prompt cache → 这部分用户不受益。需要文档明示。
- 5 分钟 TTL:retry 链长时 cache miss,墙钟时间反而是"假命中"(client 以为命中,server 在 TTL 边缘 miss)。需要 telemetry 看真命中率。
- 对 LLM-facing 内容**完全无变化**(L1 红线)。

**回滚策略**:`prompt_cache_enabled: false` yaml 一键关掉,行为 byte-identical pre-F6。

---

## 2. B6-F7 — Retained-draft retry path

### 2.1 问题陈述

`finalizer` 在 `runContractCheck` fail 时进入 retry:
1. 当前 retry 把整个 message history 重新发,LLM 重新生成完整 `emit_answer_document` payload (包含未变的 blocks + 改正的 blocks)。
2. 一份 V2 doc 典型 5-30 KB JSON;retry 链 3-5 次 = 30-150 KB 输出 token 浪费。
3. LLM 重新生成时,**未要改的 block 经常被自发重写** (不是稳定的"只改你说的部分"),引入新的 violation。

audit P13 / P26 / P37:retry 不是真正"针对违规修补",而是"重新作答"。

### 2.2 候选方案 (3 个)

#### A. delta-emit protocol (理想形态)

**做法**:增加 `emit_answer_document_patch` tool,接受 `{"unchanged": ["b1","b3"], "replace": [{...},{...}]}`。LLM 看到 retry hint 时只发 patch,不重发整个 doc。系统侧把上一次的 `AnswerDocumentV2` deep-copy + 应用 patch 拼出新 doc。

**优点**:
- 真根因解决:LLM 只输出新的 block payload。
- output tokens 节省 50-80% on retry。
- 强制 LLM **focus on the violation** — "保留 b1 b3,只改 b2" 比"重新作答" prompt 更精确。

**缺点**:
- 新 tool schema + 新 validator + 新 retry hint 渲染分支 — 大改。
- LLM 行为不稳定:小模型 (claude-haiku / deepseek-chat) 经常 hallucinate "unchanged" 列表里塞它没看过的 block id。需要严格 validator (id 集合必须 ⊆ 上次 emit 的 id 集合) + 拒绝时降级到全量重发。
- Schema validator 复杂度:patch 后的 doc 仍要过完整 11-stage 验证 (block kind / claim_use / V1-field-detect / facet coverage / oracle 们)。一个 patch 看似只改 b2,但可能让 b3 的 facet 漂移失败。
- retry-on-failure 的 retry-on-failure 怎么办?如果 patch 验证失败,要不要再发 patch?容易陷入 patch-of-patch 死循环 — 需要 retry budget 控制。

#### B. server-side retain + LLM "diff prompt" (中间形态)

**做法**:不引入新 tool。retry 时,系统在 user message 里渲染:

```
Your previous emit_answer_document was rejected because <violation>.

Previous payload (DO NOT change blocks marked [unchanged]):
[unchanged b1] {full json}
[unchanged b3] {full json}
[REVISE] b2 — current content {full json}, must satisfy <violation.Repair>

Please re-emit emit_answer_document. For [unchanged] blocks, copy them VERBATIM. For [REVISE] blocks, apply the repair.
```

LLM 仍调 `emit_answer_document` 全量发,但 prompt 引导它复用未变 block。

**优点**:
- 不改 tool schema → 零兼容风险。
- LLM 看到自己的上一版 + 标记,大概率正确复用。
- 系统侧不需要 patch validator。

**缺点**:
- output tokens **不省**(LLM 还是要发整个 doc)。
- input tokens **多发** (上一版 doc 现在塞进 prompt)。
- 收益主要在"LLM 不再随意重写未要改的 block",不在 token 经济。
- 违反 `feedback_no_internal_info_in_llm_prompts.md` 边界 — 把上一版输出复述给 LLM 是"内部状态信息",不是任务信息。

**结论**:**轻方案,但收益有限 + 微违红线**。可以做,但不应该当主路径。

#### C. retry budget 优化 + "本地验证后重发只发改动 block" (极小改动)

**做法**:不改协议。在 V2 retry hint 里,系统检测出"violation 只涉及 block b2",就在 hint 里加 "the previous emit was acceptable except for block b2; if you re-emit, you may copy other blocks verbatim from your previous output"。LLM 仍然调全量 emit。

**优点**:
- 改动最小 (只是 hint 文本调整)。
- 兼容所有现有 validator。

**缺点**:
- 收益完全靠 LLM 是否听话。实测小模型经常忽略这种"copy verbatim"指令,自发重写。
- 不解决 token 浪费根因。

**结论**:**已经做了一部分** — B2-F1 retry-hint escalation 已有"明确 violation block"逻辑。继续在这条路上加只会带来很小收益。

### 2.3 推荐方案

**两步走**:

**第一步:方案 A (delta-emit protocol)** — 真正的根因修复,需要单独 session 设计 + rollout。
**第二步**(若 A 风险太高 / 时间不够):方案 B 作为兜底。**不做** C(收益太小,不值得 commit cost)。

#### A 实施切片 (~12-15 commits;非平凡 refactor)

| commit | 范围 | 风险 |
|---|---|---|
| F7-c1 | 设计 `AnswerDocumentV2Patch` 类型:`{Unchanged []string, Replace []AnswerBlock, AddCitations []Citation, RemoveCitations []int}` | 低 |
| F7-c2 | `internal/types/answer_document_v2.go` 加 `ApplyPatch(prev, patch)` 函数 + 严格验证 (unchanged ⊆ prev.ids; replace ids 互斥) | 中 |
| F7-c3 | `emit_answer_document_patch` 工具 schema + Execute (in `emit_answer_document_patch.go`)。patch 跑完 ApplyPatch 得到新 doc,然后走和 emit_answer_document 一样的 11-stage 验证 | 中 |
| F7-c4 | `internal/skill/defaults.go::answer-document-skill` 加 patch tool 的 worked example (3 个:加 caveat / 替换 b2 / 改 diagram) | 低 |
| F7-c5 | `answer_document_evaluator.BuildInitialInstruction` 在 `EmitStageRetryAttempt > 0` 时:(a) 渲染 prev doc 到一个 LLM 可见的 reference section;(b) 在 retry hint 中告诉 LLM "you may use emit_answer_document_patch to re-emit only the changed blocks" | 中 |
| F7-c6 | retry 路径中的 ParseOutput:接受 patch tool 调用,resolve 出最终 doc 写到 Mutable | 中 |
| F7-c7 | 失败兜底:patch validate 失败 → 把 patch 拒绝消息塞 retry hint,LLM 下次仍可选择 patch 或全量 emit | 中 |
| F7-c8 | yaml 知 旋钮 `pipeline_finalizer_retained_draft` (默认 false 先 telemetry,稳定后 true) | 低 |
| F7-c9 | 单测:patch 多场景(unchanged 集 / 全替换 / citation 引用变化)+ ApplyPatch 边界 (citation index 漂移) | 中 |
| F7-c10 | telemetry:retry 时记录 `delta_path_used / fallback_to_full_emit / patch_validate_fails` 三个计数器,end-of-Run 行尾输出 | 低 |
| F7-c11 | 真 LLM 端测 (用 anthropic + openai 各跑一次 retry 强制场景),验证主流模型能正确产 patch | **高** (可能要 prompt 微调多轮) |
| F7-c12 | docs/architecture.md / `change-plan-skill` 类比章节 | 低 |

### 2.4 影响分析

**正向**:
- output tokens retry 时省 50-80%。
- LLM 重写 leak → 显著减少 (新 block 不被 hallucinated 影响)。
- retry 收敛速度提升:LLM focus 在违规 block,不会"为了改一个 block 把其他 block 也重新生一遍"。

**负向 / 风险**:
- **复杂度↑↑**:新 tool + 新 validator + 新 retry path + LLM 行为不稳定。
- **小模型可能反噬**:claude-haiku / deepseek-chat 在 patch 协议下输出质量比全量 emit 差(它们更习惯"完整作答")。需要 LLM 端真测验证,可能要在 yaml 加 per-model gate。
- **Citation pool 索引漂移**:patch 改 block 但 citation pool 指向 index 时,删除/插入 citation 会让其他 block 的 CitationRef 失效 — 需要 ApplyPatch 严格保留旧 citation 顺序,或强制 patch 携带 citation 全量重建。后者退化到 50% 不省。
- **跨 provider 不一致**:某些 provider tool_choice="required" 在多 tool 之间选时偏好 deterministic — 可能压根不选 patch tool。
- 默认 false → 真 ship 前要 8+ runs 评估一致性。

**回滚**:`pipeline_finalizer_retained_draft: false` yaml 一键关,走全量 emit;`emit_answer_document_patch` 仍存在但不被推荐。零行为变化。

#### 兜底方案 B 实施切片 (~3-4 commits)

| commit | 范围 |
|---|---|
| F7B-c1 | `answerDocAttachEscalation` 在 attempt ≥ 1 时,把上一次的 doc.Blocks 渲染成 reference (≤ 8 KB,过长截短) |
| F7B-c2 | retry hint 改成"copy unchanged blocks verbatim;revise only listed block ids"模板;明确列出 violation 涉及的 block ids |
| F7B-c3 | 单测:retry hint 文本契约 |
| F7B-c4 | yaml 知 旋钮 `pipeline_finalizer_show_prev_draft` |

收益主要是 "LLM 不再随意重写未要改的 block",token 不省。

### 2.5 决策建议

**F7-A (delta-emit) 不做** — until:
1. 真 8 runs eval 显示 retry token cost > 30% of total run cost (当前未观测);
2. F6 (prompt cache) 已 ship + 稳定运行 1 周;
3. 至少 3 个常用 provider (Anthropic / OpenAI / 国产至少一家) 都验证过对 patch protocol 的接受度。

**F7-B (轻方案) 现在可做**:风险低,收益小但稳定。需要先 lift 一下 `feedback_no_internal_info_in_llm_prompts.md` 的边界 — 把"上一次的输出"明确划归到"任务信息"(因为 retry 的语义本来就是"基于上一次结果")。

---

## 3. 总体优先级 + 后续步骤

| 项 | 复杂度 | 风险 | 收益 | 推荐时机 |
|---|---|---|---|---|
| F6-A (provider prompt cache) | 中 | 中 | 高 (analyze 砍 25-50%) | **优先做** — 单独 session,~8 commits |
| F7-B (retry hint show prev draft) | 低 | 低 | 中 (减少 LLM 重写 leak) | 跟 F6 同 session 兜底,~4 commits |
| F7-A (delta-emit protocol) | 高 | 高 | 高 (retry token -50% +) | **暂不做** — 等 F6 数据 + LLM patch-protocol 验证 |

**未审批前不要开发**。两个设计都需要:
1. 真 8 runs 跑一次 baseline,把 analyze 时长 / retry token 数实测出来作为参照。
2. 选一个 provider 主战场 (建议 Anthropic — prompt cache 收益最大 + 已用量最多),先在 staging key 上验证。
3. F6-c3 + F7-A-c3 都需要真实 LLM 端到端,不能只跑机器测试。

---

## 4. 红线复核 (设计阶段)

- [x] L1 read mode byte-identity:F6 默认 nil = pre-F6 行为;F7-A 默认 false = pre-F7 行为。
- [x] `feedback_no_custom_keyword_matching.md`:两方案都不依赖 question text 匹配。
- [x] `feedback_precise_signals_for_hard_gates.md`:两方案都不引入新 hard gate;telemetry/cache 是 soft signal。
- [x] `feedback_no_system_backfill_to_user_panel.md`:两方案都不动用户面板,只动 LLM-system 通信路径。
- [x] `feedback_eliminate_noise_at_source.md`:F6 真根因 (provider 已支持只是没用);F7-A 真根因 (协议升级不是 hack)。
- [ ] `feedback_no_internal_info_in_llm_prompts.md`:F7-B 轻方案 borderline — "上一次输出复述给 LLM" 算 internal info 还是 task info?需要确认。
