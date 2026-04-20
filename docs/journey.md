# Codrax 的曲折之路

> 一份写给非 LLM 工程师也能读懂的演进记录：我们做错了什么、最后怎么修的、系统和模型各自负责什么、今天还差在哪里。

## 1. 这个项目想解决什么

Codrax 是一个**只读的代码分析助手**：你给它一个仓库和一个自然语言问题，它跑一条 4 阶段流水线（`analyze → explore → extract → finalize`），最后输出一份带行号引用、可被人类核对的结构化答案。它不改代码、不改文件、不调外部副作用接口。

听起来很简单，但要做到「答的内容是真的」远比「答得像那么回事」难。这篇文档讲的就是这件事的曲折之路。

---

## 2. 三类反复出现的难题

写到 2026-04 这个版本，回头看，所有踩过的坑都可以归到三类：

1. **多跳推理难** —— 用户的问题需要把多个文件的多个事实串成一条链，LLM 在第二、第三跳就开始飘。
2. **模型幻觉难控** —— LLM 会编出看起来非常合理、但仓库里根本不存在的函数、文件、路径。
3. **信号放大难治** —— 上游某一步犯了一个小错误，下游每一层都用「覆盖率」而不是「相关性」做判断，最后把小错误放大成完全错误的答案。

下面用真实出现过的 case 说明这三类问题，以及我们怎么从「拍脑袋打补丁」走到「定义一个跨 stage 的契约」。

---

## 3. 案例一：多跳推理 —— "explorer 默认用哪个 skill?"

### 现象

用户问：「codrax 的 explorer agent 默认用哪个 skill？」答案在源码里写得清清楚楚：`explore-skill`，定义在 `internal/skill/defaults.go:14`。

但有几次跑，模型反复重试 3 次，最后报「answer-contract validation exhausted」，并且把答案写成了 `explorer`（agent 名）而不是 `explore-skill`（skill 名）。

### 为什么

这是一个典型的**多跳问题**：要从「explorer agent」跳到「它的 default skill」，再跳到「这个 skill 的字面量值」。我们的系统里同时存在两条证据通道：

- **LLM 调出来的 `EvidenceItem`** —— 必须来自 LLM 实际 `read_file` 过的文件（叫 ReadSet）。
- **确定性的 chain producer** —— 会扫描更大范围（叫 ScannedSet），把它认为相关的代码片段塞进 prompt。

冲突区就是 `ScannedSet \ ReadSet`：chain 把答案锚在一个 LLM 没读过的文件里，prompt 告诉 LLM 「以这条 chain 为准」，但 grounder（接地校验器）严格只接受 ReadSet 里的引用，于是把这条 chain 全部 drop 掉。LLM 重试，再产生同样的 chain，再被 drop——三次重试全部白跑。

### 失败的尝试

最初的本能反应是「再加一层兜底」：

- 在 grounder 里放宽规则，允许某些情况下接受 ScannedSet 的引用 → 立刻引来一波幻觉，因为 LLM 没真读过那些文件。
- 在 prompt 里更激烈地告诉 LLM「请相信我们给你的 chain」→ 没用，因为问题不在 LLM 想不想信，而在引用没法对账。
- 加白名单，把项目特有的「skill 字面量」识别成特殊路径 → 短期能修，但只对 codrax 自己有用，换个仓库就废了。

### 真正的解法：CGEC（Citation-Grounded Evidence Closure）

我们退一步问：「这是哪条不变量被破坏了？」答案是**没有不变量** —— 两条证据通道从未约定共享一个范围。

于是定义了 4 条跨 stage 的强契约（每条都有独立的 enforcer 在代码里把关）：

| # | 不变量 | 用大白话说 |
|---|---|---|
| **I1** | prompt 里所有 `file:line` ⊆ ReadSet | 「我递给模型看的引用，必须是模型真读过的文件」 |
| **I2** | 所有 emit 接受的 citation ⊆ ReadSet | 「模型要写进答案的引用，也必须是它真读过的」 |
| **I3** | `emit_investigation_complete` ⇒ 合约能通过 | 「模型说『查完了』前，先在内部跑一次合约模拟，跑不过不让它收工」 |
| **I4** | 重试必须改变 ReadSet / EvidenceCount / ChainTermSet 至少一个 | 「重试不能交出和上次一模一样的东西」 |

外加一个叫 **Lazy Auto-Read** 的机制：当 chain 锚到了 ReadSet 之外的文件，系统自动把这个文件加入 PendingReads，下一轮强制让 LLM 读一下。

修完之后，同一个问题一轮就答对，日志里出现 `CGEC E2: forced-read 2 file(s) the LLM skipped`。

### 写给非工程师的类比

把 LLM 想成一个实习律师，他在法庭上引用案例。我们以前的做法是：他随口说一个案例号，法官（grounder）查不到就当庭驳回，然后让他「再想想」—— 他想出来的还是同一个错案例号。

CGEC 的做法是：法官每次驳回都会直接把「你应该去查这本卷宗」写在反驳意见里（PendingReads），下一次开庭实习律师必须先翻那本卷宗（Lazy Auto-Read），并且每次新陈述至少要带一条新材料（不变量 I4）。

---

## 4. 案例二：模型幻觉 —— "用 codrax 看 glamour 仓库"

### 现象

有人用 `codrax --repo /path/to/glamour` 让我们分析一个**别人的开源仓库**（glamour，一个 markdown 渲染库）。结果答案里出现的全是 codrax 自己的内部模块名（`internal/render/renderer.go` 等），完全没碰 glamour 的真实代码。

第一反应：**LLM 幻觉了**。我们甚至已经为此写了一份待办备忘录《LLM-foreign-repo-citation-hallucination》。

### 真正的根因

后来在调试日志里发现一行非常诡异的输出：

```
repo_map: full rescan (234 files)        ← glamour 的 .go 文件数，正确
repo_map: incremental (288 files)        ← codrax 自己的 .go 文件数
```

同一次运行里两条输出交替出现。这意味着 LLM 调 `repo_map` 时根本没问题，是工具自己有时候扫 glamour、有时候扫 codrax。

根因是：所有接收 `path` 参数的工具（`repo_map` / `grep` / `read_file` / `list_files` / `git_diff` / `git_log` / `exec_command`），在 LLM 传 `path: "."` 或 `path: "internal/foo.go"` 时，都用**进程当前目录**去解析相对路径，而不是用 `--repo` 指向的仓库根。当用户的 CWD 恰好是 codrax 自己的源码目录时，这些相对路径就解析到了 codrax 自己。

### 解法

写了一个 `resolveToolPath(ctx, p)` 工具函数，把 `ctx.RepoRoot` 一致地穿到 7 个工具的 Execute 入口；同时在程序启动时把 `--repo` 解析成绝对路径并去掉 symlink。

这次的真正代价不是代码——是诊断方向。我们差点把一个**工具层 bug** 当成 **LLM 幻觉**去修。结果就是写一堆 prompt 工程，加一堆"反幻觉提示词"，根本不会有效果。

### 留下的纪律

这个 case 后来被写成一条强约束记到长期记忆里：**当 LLM 在外部仓库上输出"奇怪"的内容时，先看日志里 `repo_map: full rescan (N files)` 的 N 是不是匹配你自己的仓库；如果是，就是工具层范围 bug，不是 LLM 幻觉。**

---

## 5. 案例三：信号放大 —— Explorer 把上游噪声当真理

### 现象

有一阵我们尝试一个叫 `preRead` 的优化：让 analyzer 把它觉得相关的文件直接预读进去，避免 explorer 反复试探。结果 5 道测试题里有 3 道更差了。回滚之后做复盘，发现真正的问题不在 `preRead`。

### 4 个放大器

Explorer 阶段（也就是真正去读源码、收集证据的那一步）有 4 个地方在用「覆盖率/数量」代替「相关性」做判断：

1. **Phase 0 闸门** —— 看「有没有跑 grep」「发现了多少文件」就判断「探索得够不够」，但不评估这些文件跟用户主问题的距离。
2. **深读入口合并候选集** —— 把 `readSet + preScannedFiles + allScoredFiles` 一锅端给下一轮，第一轮排序偏一点，第二轮加倍偏。
3. **`concrete_values` 全扫** —— 注释写明「scan all files from the keyword search」，第一轮的广度误差直接驱动第二轮的深度浪费。
4. **HasEnoughFacts 多维质量检查** —— 看 `toolDiversity` / `fileCoverage` / `directCount`，但没有「最近这批文件对用户主问题的解释力增量」这一维。

结论是：**analyzer 选错文件可以救，但 explorer 把任何噪声都当信号无脑放大就救不了。** 任何「把更多上游输入塞给下游」的改动，必须先在沿途引入一个相关性过滤（比如和 AnswerSubject 比对、和主实体比对、按 PredicateAxis 打分），否则就是在造放大器。

这条纪律目前是新功能的默认拒绝项。

---

## 6. 案例四：模型一边写一边吞 —— Finalizer 萎缩问题

### 现象

最终答案阶段（finalizer）有时候会出现这样的现象：第一次迭代生成了一段 800 字的丰富解释，但下一轮工具调用之后，提交进 `summary` 字段的只剩 60 个字。用户拿到的是被自己「压缩」过的精简版。

### 三层修法

这个问题代表了一类典型的「LLM 的中间产物比最终产物质量更高」失配。修法是分三层防御：

1. **Skill 提示词（L1）** —— 在 `answer-document-skill` 里写明「从一开始就在工具调用内部组织最终文本」，不要先写一段然后总结。
2. **重试 hint（L2）** —— 当评估器发现 summary 比上一轮的草稿短得多，下一次给 LLM 的提示改成「把你上一段原话**逐字**抄进 summary[]，然后从同一份草稿里抽出结构化字段」。
3. **硬兜底 salvage（L3）** —— 如果模型还是没改回来，系统在 `ParseOutput` 里直接把上一段草稿原文抄回 summary，按 shape 决定上限做 UTF-8 边界裁剪，并附上一行双语 caveat 告诉用户「以下文本是从中间草稿恢复出来的」。

第一版 salvage 只对解释类（ShapeExplanation）启用。后来真实日志出现非解释类也萎缩，于是把 salvage 扩展到所有 shape，每个 shape 按比例配置 floor（解释类 1.0×、step 列表 0.5×、布尔 3/8×、数值 0.25×）。

---

## 7. 系统与模型的边界：谁负责什么

经过上面这些坑，整个项目最重要的一条架构原则被磨出来了：

> **LLM 只负责自然语言理解和 ReAct 探索，所有可以确定性推导的东西全部由代码完成。**

这条边界落到代码上是这样的：

### 7.1 LLM 的职责（4 个明确接触点）

| 阶段 | LLM 干什么 | 调用哪些工具 |
|---|---|---|
| **analyze** | 理解请求、给出 `RequestModel`（intent / complexity / sub_topics / answer_shape / predicates 等结构化分类）。1–2 轮 pre-scan，禁止 `read_file` / `exec_command` | `repo_map` / `grep files_only=true` / `list_files`，最后 `emit_analysis` |
| **explore** | 提一组 hypothesis，调工具找证据，发出结构化 `EvidenceItem` | 全套只读工具 + `emit_evidence` + `emit_investigation_complete` |
| **extract** | 把 Turn A 的原始证据按目标 shape 组织成 `AnswerSymbol` / `HypothesisVerdict` | `emit_answer_symbol` / `emit_hypothesis_verdict` |
| **finalize** | 写人话，并把所有引用对账到 ReadSet | `emit_answer_document` |

LLM 不写 TaskGraph，不评估 SuccessCriteria，不决定要不要重试，不决定 hypothesis 怎么打分，不决定哪些是 RequiredFiles。

### 7.2 系统（确定性代码）的职责

`internal/analysis/` 下有 14 个**纯函数**子包，全部不调 LLM、不调工具、不读文件系统：

| 子包 | 干什么 |
|---|---|
| `normalizer` | 把请求归一成 TermGraph |
| `compiler` | 根据场景模板把 RequestModel 编译成 TaskGraph + EvidencePlan + AnswerContract |
| `budget` / `sourcemix` | 多维证据预算（每个工具能调几次） |
| `risk` | 6 维风险矩阵 |
| `hdp` / `priority` / `binder` | 假设生成、4 维打分、节点绑定 |
| `counterfactual` | 复杂 + 模糊请求时的反事实分支扩展 |
| `gate` | 7 项质量门（覆盖、DAG 闭包、预算合理性、契约完整、假设覆盖、可评 criterion、pending 字段格式） |
| `criterion` / `stopcond` / `contract` | 19 种 Kind 的运行时评估器 |
| `dataflow` | source → sink 链路 |

所有 `EntryConditions` / `SuccessCriteria` / `StopConditions` / `RequiredEvidence` / `FalsificationCondition` / `AcceptanceTests` 字段都不是「写给人看的注释」，而是会被 criterion 评估器**真正执行**的合约。

### 7.3 一条强红线：通用化优先于本项目成功

写在 `feedback_generalization_over_project_success.md` 里的红线：

- ❌ 禁止：项目特定字符串白名单、项目特定后缀匹配、项目特定 ZH/EN cue 表、enum 枚举值用项目特定术语命名。
- ✅ 允许：跨语言通用的标识符 shape、HTTP / middleware / actor 等业界概念、文件后缀约定、纯算法。
- 🟡 灰区：先问再写。

「**宁可接受失败，也不要虚假的成功；允许失败但不允许错误的方向越走越远**」。session 11 审计删完所有 codrax-specific boost 之后，确实有一道题的答案变错了——我们没有回退。

### 7.4 Fail-loud 而不是静默兜底

Analyzer 如果 LLM 没调 `emit_analysis`，stage 直接报错并触发重试，绝不静默合成一个零值 IR 给下游用。这条「**fail-loud 合约**」让所有契约违例都暴露在最近的 stage，而不是漂到最终答案再炸。

---

## 8. 当前的不足

诚实地列一下今天还没解决的问题：

### 8.1 用户等待时间长

- 单次问答典型耗时 30 秒到 2 分钟。原因：4 阶段串行 + LLM 的 ReAct 多轮 + grounder 校验 + 失败时的重试。
- 没有流式中间结果——目前只在 stderr 打 thinking trace，stdout 在最后一次性给完整答案。
- 一些 reasoning 模型把所有思考塞进一个 `<think>` 块，对外看就是「卡很久才出第一个字」。

### 8.2 不支持代码修改

定位就是只读分析，永远不会有 `--write` 这种参数。如果你需要改代码，请把 codrax 的答案当作输入，再交给别的工具（或人）执行。这是有意为之的边界，能避免「分析错了直接污染源码」的灾难，但也意味着 codrax 不是一个 agentic IDE。

### 8.3 仍然依赖 LLM 的关键词创造力

Explorer 第一次扫描的关键词是 LLM 想出来的。如果模型问错关键词，会先去读一批不相关的文件，CGEC 的 Lazy Auto-Read 是兜底但不是矫正。session 5 之后我们没有继续动 explorer 的初选逻辑。

### 8.4 引用数量门偶尔不达标

修完所有路径解析之后，glamour 仓库上的回归测试答案锚点已经全部正确，但 `CitationReq:1` 这个数量门偶尔仍然不达标——这是另一类独立问题（数量阈值，不是路径错配），还在排队。

### 8.5 LLM 仍会在 quote 里编 prose

Grounder 会把无法对账的 quote 文本擦掉，但没办法「教会」LLM 不再编。如果换更高质量的模型，这一类问题会自然减少；但作为系统纪律，**永远不能因为模型变好而把校验放松**。

### 8.6 没有 CI、没有 lint

依赖只有 `gopkg.in/yaml.v3`，Go 1.22.5。当前所有质量保证靠 `go test ./...` 和 5-Q 端到端 audit。规模再大一点这套方法会顶不住。

### 8.7 多实例并发安全是兜底而不是优化

日志、blob 会话、memory 锁全是用 PID + 文件锁兜底的「不会冲突」，没做 multi-tenant 真正的隔离。多用户长期共用同一台机器还需要进一步加固。

---

## 9. 一句话总结

如果说这个项目走到今天学会了一件事，那就是：

> **不要试图让 LLM 变得更"听话"，而要让系统的契约更难被破坏。**

每一次靠"加更狠的 prompt"解决的问题，都会在下一个版本以另一种形式回来。每一次定义清楚一条不变量、并写一个 enforcer 让它无法被违反的修法，下一次 bug 出现时都能被 grep 到对应的契约名。

这就是为什么这个项目里 LLM 调用很少，但 `internal/analysis/` 下有 14 个子包；为什么 emit_* 工具会拒绝不合规的入参；为什么有 CGEC 这种听起来很学术的缩写。这不是过度工程，是被 bug 一步步逼出来的纪律。
