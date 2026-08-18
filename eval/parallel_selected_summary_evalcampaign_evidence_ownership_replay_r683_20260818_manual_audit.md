# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T11:44:31Z
- sweep_start_ts: 20260818-044429
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260818-044431 | answer_regex,answer_contains | none | 303s | 38 | read=17,repo_map=0,list=1,trace=0,source_lens=0 | midloop=13,inv=4/0,fin_reject=0,unavail=0,prune=0 | partial | Runner 仍是第三条跨行 regex 假阴性；两节完整且 r682 的 Finalizer/REPL 状态串线消失，B1069 方向获正证。事实仍有错：lookup 第三路径误写 `.codrax/codrax.yaml`（源码为 `codrax/codrax.yaml`，且漏 3 个 legacy 路径），“RuntimeSettings 所有字段均为指针”也不成立；`SanitizeDegradedMermaidBlocks` 被误写成移除提示供 LLM，实际是恢复稿交付前语法校验/安全降级。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-044431 | answer_regex,answer_contains | none | 389s | 42 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=2/0,fin_reject=4,unavail=0,prune=0 | partial | 最终表和四 stage ordering 图可用，Mermaid 语法有效且仅保留三条 typed precedence 边；但前四稿反复把 Orchestrator dispatch/self-loop 当 call edge，又把 precedence metadata 与可见边分离，连续 4 次成文拒绝后才收敛。表中 `MutableState.EvidenceItems` 等载体名也不精确，故不能签 pass。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. r683 严格并发恰好两例。组合案 303s、零 Finalizer reject；pipeline 图案 389s、4 次 reject/patch。
   均无畸形 JSON、空答案、旧稿恢复或活跃流固定 4ms 降级。
2. B1069 的软引导产生了正面效果：组合终稿严格分成配置与 Mermaid 两节，不再把
   `proseFallbackRequested`、`diagramRelationFailurePairStrikes` 拼入 REPL 渲染链，也没有再把 YAML setter
   冒充唯一 CLI 覆盖点。Runner FAIL 只是同一旧跨行 oracle；人工不能因此判 fail。
3. 但生产日志否证了“producer node 即语义 owner”这一过强命名。t0 配置 lane 同时产出 Mermaid evidence，
   t1 也带回大量通用 REPL/repomap 行；最初提示甚至把 `renderMermaidFenceBody` 放进 Unit 1，并把无关 REPL
   assignments 放进 Unit 2 前 16 行。`NodeArtifactLedger` 精确证明收集 provenance，却不证明主题语义归属。
4. 因此 B1069 已立即二次收窄：优先用 SubTopic `Entities/Scopes` 中 exact 文件名、路径或目录与
   `EvidenceItem.Source` 匹配；只有一个 unit 命中才归入该 unit。producer lineage 仅在该 unit 完全没有 exact
   source 命中时作为 `producer_lane_fallback`；多命中、无命中及共享 probe 行归 shared。共享显示按 typed
   relation、Explorer 明确选择、salience 排序，不再按随机 EvidenceID 把无关 assignment 顶到前面。标题也从
   ownership 改为 partition hints，明确这是写作分区而非语义所有权。
5. 组合答案仍有三类事实精度问题：配置 lookup 漏/错路径、把包含 map/slice 的 RuntimeSettings 泛化为“所有
   字段均指针”、误述 `SanitizeDegradedMermaidBlocks`。这些说明精确 scope 分区能防跨题串线，但不能替代
   claim→citation 的局部绑定；记 `B1071-MULTITOPICSOURCEASSOCIATION1` 为已施工待回放，并把局部事实绑定继续
   纳入 B1070 异构观察，不用答案关键词或单文件行号特判。
6. pipeline 案确认 P1 `B1072-SEQUENCEWORKFLOWRELATIONREPAIRSTORM1`。模型第一稿画 Orchestrator→各 stage
   的未证 call；validator 正确拒绝，但给出的长提示同时要求保留已有边、补 typed candidate、处理 participant
   boundary。模型随后三轮在未证 dispatch、自环、metadata-only precedence 间摆动，第四次才缩成三条真实
   ordering 边。拒绝本身正确，交互合同认知负担过高；应提供 typed workflow ordering 的紧凑首稿 recipe 和
   只含失败 edge 的结构化删除/替换动作，不能放松边权威或由系统代画。
7. 最终图使用 `analyze -> explorer -> extractor -> finalizer` 三条 precedence，格式合法、关系未丢，说明不需
   新增基于 diagram.body 原文的硬门。最优方案应复用 typed relation candidate/edge identity，统一 sequence、
   flowchart、class 等图族的关系教学，减少模型自行把逻辑顺序画成 call。
8. 本轮未触发 Trace。显式窗因果投影、自动补齐、链上-only 主因、背景 support-only，以及优先级反转、
   调度/算力供给、D/IO、确定性语义工作、业务线索、实际占用/规则可消双轴均保持。

## Decision

- `B1069=production-direction-positive/producer-semantic-owner-overclaim-corrected`
- `B1071-MULTITOPICSOURCEASSOCIATION1=implemented/exact-source-first+producer-fallback/replay-next`
- `B1072-SEQUENCEWORKFLOWRELATIONREPAIRSTORM1=confirmed/P1/next`
- `B1070-CITATIONITEMBINDING1=broadened-observation/no-case-fit`
- `runner=1 PASS/1 regex-false-negative; human=2 partial`
- `active-stream-fixed-4ms-degrade=forbidden/not-observed`
- `system-answer/conclusion/relation/diagram-authorship=none`
- `Trace explicit-window/query/projection/auto-supplement=unchanged`
