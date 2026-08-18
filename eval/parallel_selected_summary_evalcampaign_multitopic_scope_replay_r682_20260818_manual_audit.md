# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T11:22:03Z
- sweep_start_ts: 20260818-042202
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-042203 | answer_regex,answer_contains | none | 100s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Windows/Apple/POSIX API 与唯一调用者 `cmd_sleep` 的文字结论正确；但有两条展示绑定错误：POSIX 列表项引用了 Apple 返回行 `src/clock.c:30`，终稿又把该行表述成 POSIX 证据。B1064 未见同名定义串线回归，但最终引用精确性仍不能签 pass。 |
| 2 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-042203 | answer_regex,answer_contains | none | 599s | 41 | read=25,repo_map=3,list=0,trace=0,source_lens=0 | midloop=19,inv=6/0,fin_reject=1,unavail=0,prune=1 | fail | B1068 获生产闭环：probe completion 后系统没有关闭 DAG，`n1_evidence_t0/t1` 以 parallelism=2 真派发，两个题面均获得源码证据并进入终稿；但扁平 Finalizer 证据池把不同主题/机制的事实串线，错误宣称 `SetMermaidGateMode` 是 CLI 覆盖、`proseFallbackRequested`/`diagramRelationFailurePairStrikes` 属于 REPL 渲染降级，并颠倒 `OutcomeFallbackRune` 的源码保留语义。Runner PASS 不能掩盖这些事实错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B1068 的生产回放通过。日志先记录 `n0_probe` completion 被限定在当前 window，随后同时派发
   `n1_evidence_t0` 与 `n1_evidence_t1`（`parallelism=2`），最后才进入 validate/reconcile。两个主题
   evidence 节点都真实执行，不再被 probe 的提前完成吞掉。
2. 组合案的 Runner 由 r681 的 regex 假阴性转为 PASS，但人工仍为 fail。配置主题实际证据只证明
   `codrax.yaml -> LoadRuntimeSettings -> initApp -> SetMermaidGateMode`；没有 typed CLI binding，终稿却把
   `SetMermaidGateMode` 写成 CLI 标志覆盖，并发明“内置默认值 -> YAML -> CLI”的该配置项优先级。
3. Mermaid 主题中，`proseFallbackRequested` 是 Finalizer 缺少可见结构化草稿时的协议恢复状态，
   `diagramRelationFailurePairStrikes` 是答案关系校验重试状态；二者均不属于
   `REPL.renderRichResponse -> RenderMermaidBlocks` 的渲染失败链。终稿把同主题的断开定义事实拼成一条机制，
   属于 evidence-to-final 语义串线，不是模型没读代码。
4. `OutcomeFallbackRune` 的源码注释明确表示仍以窄字符 fallback 成功渲染；真正将围栏改为 `text` 并保留
   源码的是 `OutcomeUnsupportedKind`。终稿把两者都说成“保留原始围栏源码”，再次说明“同主题”不能代替
   typed relation connectivity。
5. 新确认并在审计后施工 `B1069-MULTITOPICEVIDENCEOWNERSHIP1=P1`：Finalizer 原先只渲染 SubTopic 标题和全局扁平 Evidence /
   enrichment rows，丢弃了已存在于 `NodeArtifactLedger` 的 `producer evidence_tN -> EvidenceID` 归属。
   最优方案是用 `CompileInvestigationPlan + NodeArtifactLedger` 生成仅提示的按调查单元证据视图；跨单元或同单元
   但无 typed relation 连通的事实只能作为上下文，不能被描述成同一调用/失败机制。实现已增加 prompt-only
   ownership section：逐 unit 有界列出 exact EvidenceID/位置，将多 owner 或无唯一 owner 的行只显示一次为
   shared；关系型行只授权精确 subject→object，独立定义行明确不授权机制 transition。该 section 只在既有
   multi-topic answer-structure 车道启用，Trace/call-chain 专用 support-plan 车道不被扩域；它不参与 validator，
   不由系统写结论、关系或图。
6. 平台案另保留 `B1070-CITATIONITEMBINDING1=P2-observe`：事实结论正确，但 POSIX 项借用了 Apple return
   行。先随异构回放判断是否为既有 citation selector 的稳定类问题，不为单个行号增加答案扫描或特判。
7. 组合案 599s 包含两个真实 evidence lane 和一次无首字节恢复，不能把全部增量误记为 B1068 性能回归；
   probe 与主题 lane 有重复读取，作为 P2 调度复用债记录。本轮没有畸形 JSON、空答案、旧稿降级或活跃流
   固定 4ms 降级。
8. 本轮没有 Trace 输入；显式时间窗因果投影、自动补齐、链上-only 主因、背景 support-only、实际占用/
   规则可消双轴均未改动。

## Decision

- `B1068-MULTITOPICPROBECOMPLETIONSCOPE1=production-closed-r682`
- `B1069-MULTITOPICEVIDENCEOWNERSHIP1=implemented/typed-soft-context/replay-next`
- `B1070-CITATIONITEMBINDING1=P2-observe/no-case-fit`
- `runner=2/2 PASS; human=0 pass/1 partial/1 fail`
- `active-stream-fixed-4ms-degrade=forbidden/not-observed`
- `system-answer/conclusion/relation/diagram-authorship=none`
- `Trace explicit-window/query/projection/auto-supplement=unchanged`
