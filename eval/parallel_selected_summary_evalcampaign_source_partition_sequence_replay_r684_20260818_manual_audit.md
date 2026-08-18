# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T12:01:32Z
- sweep_start_ts: 20260818-050130
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-050132 | answer_regex,answer_contains | none | 327s | 35 | read=21,repo_map=4,list=0,trace=0,source_lens=0 | midloop=18,inv=7/0,fin_reject=0,unavail=0,prune=0 | fail | 两节形式分开，但配置节错误宣称 `RuntimeSettings` 所有字段均为指针；查找顺序遗漏三个 legacy 路径及 `<exeDir>/codrax/` 形；Mermaid 节把仅用于 rejected/recovered doc 的 `SanitizeDegradedMermaidBlocks` 混成正常 REPL 渲染路径。Runner 的宽 contains/regex 没校验这些事实。Finalizer 分区里 Unit 2 被同一宽 `repl.go` 的 83+ 普通行占满，真正的 config/render 证据落入 Shared 或被全局 1024 候选上限截掉。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-050132 | answer_regex,answer_contains | none | 462s | 34 | read=38,repo_map=2,list=0,trace=0,source_lens=0 | midloop=22,inv=6/0,fin_reject=1,unavail=0,prune=0 | fail | 用户明确要求 `sequenceDiagram`；首稿虽有 sequence，但含大量未证调用边并被正确拒绝，patch 随后改成 `flowchart TD`/`kind=architecture`，最终交付违背载体要求。Runner 从包含已拒绝草稿的整份 `run-1.out` 命中 `sequenceDiagram`，形成假绿。确定性根因在 route：classifier 发 `requires_diagram=true`，但 paraphrase 的 directive 未能逐字锚定，系统静默把整个 hard presentation 权威降为 false；Analyzer 再把 diagram_hint 从 required 正规化为 optional，首稿也未收到 required workflow-stage authority。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings and generalized fixes

1. `B1073-PRESENTATIONPROVENANCEREPAIR1`：`requires_diagram=true` 是结构化精确信号，但其兄弟
   `presentation_directive` 不是当前消息连续逐字段面时，旧实现只警告并把两者一起清空。改为一次有界、
   同 schema 的 provenance repair：模型必须重发完整 `emit_turn_policy`，要么复制当前消息中的最短连续
   原文并保持 true，要么显式 false+空串。系统不扫描请求关键词、不重建 boolean；最终仍做 byte-exact
   provenance 检查。这样明确必需的 `sequenceDiagram` 才能进入 Analyzer/Finalizer 的 required contract，
   并触发现有 checkout-verified stage workflow recipe。
2. `B1074-MULTITOPICAFFINITYRANK1`：多主题提示不再复用全局前 1024 条 enrichment pool。它先在完整的
   accepted typed evidence 上去重/合并，再按每个 typed unit 的 Summary/Entities/Scopes 与证据结构字段做
   软 affinity 排序，最后每题最多展示 16 条。唯一 unit affinity 可把原本 shared 的事实标为
   `topic_affinity_hint`，但提示明确它不证明 ownership/relevance/edge/conclusion，也不进入 validator 或拒答。
3. 源码文档同步纠偏：`RuntimeSettings` 只有需要三态合并的 scalar knob 使用指针；collection/value payload
   并非全是指针。`LoadRuntimeSettings` 使用 strict KnownFields：unknown-only 保留已接受字段并记录警告，
   syntax/type mismatch 仍 fail-loud。
4. 本轮没有 JSON 畸形降级、空答案、旧稿恢复或 active-stream 固定 4ms 降级。两个案例均无 Trace 输入；
   显式时间窗、Trace 因果投影、自动补齐、typed 链上根因、背景 support-only 与模型结论权均未改动。
