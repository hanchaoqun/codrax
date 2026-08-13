# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T02:11:20Z
- sweep_start_ts: 20260812-191119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260812-191120 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗完整。主线程 running/runnable/sleep/D/IO 账户与缺失 wakeup 边界均披露；主因只取 typed on-chain 席位，优先级反转、调度供给、算力供给与 VerifyClass 确定性语义工作未丢。NetworkService→CookieMonsterCl→目标及 T7→目标的入链证据、有效归因和排序可复核；邻近与背景单列，不晋升主因。实际占用/业务线索与规则可消除量双轴均在。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-191120 | answer_regex,answer_contains | none | 172s | 25 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=5/3,fin_reject=0,unavail=0,prune=0 | fail | 模型正文与 classDiagram 正确列出 12 个 production implementer，12 条边均有 typed `repomap_implementer_relation`。但系统把合法 classDiagram 的终端载体不兼容误报成“部分边未类型化”，又因 single-shot 未携带 router 的 `requires_diagram`、analyzer 把图错铸为 `stage_or_workflow`，追加“输出维度核对”。终局枚举 oracle 还把 3 个 test implementer 重新并入 production 主集合，产生假完整性 caveat。Explorer 的修补提示先建议 excluded-only aggregate、下一合同又拒绝空 member_set，造成 4 次中途修补。均为系统合同 gap，不是模型漏关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and next batch

1. Trace 对照是生产正证：显式窗、因果投影、系统补齐和链上-only 主因均完整；实际耗时/业务
   线索与基于现有规则的可消除量保持两条独立轴。未观察到邻近或背景信号越权加冕。
2. B694：`classDiagram` 是浏览器 Mermaid 合法载体，12 条实现边也都有精确 typed authority；
   旧终端语法 profile 却先铸 `diagram_edge_unsupported`，后置 materializer 再把“载体暂不能渲染”
   误写成“边证据不足”。根修仅在 structured diagram seam 将可无损的 directed class relation
   机械改写为 flowchart；端点、方向、关系类型、成员文本全部来自模型原图，不新增边或结论。
   含 cardinality/复杂语法的图 fail-open，继续由现有 fail-loud 路径处理。
3. B695：单次 CLI/eval 的 typed route classifier 已给出 `requires_diagram=true`，但旧接线只传
   route hint，导致 analyzer 将图从 required 归一为 optional。根修让 single-shot 与 REPL 一样
   携带 presentation directive + diagram-required boolean；同时新增 `requested_answer_dimension.role=diagram`，
   用结构化 diagram block 覆盖该维度，禁止把视觉面误作 stage/workflow 后再复制用户原话。
4. B696：关系 handoff 已按 production scope 排除 3 个 test 实现，终局 divergence oracle 却比较
   repository-wide 15 个实现，制造假枚举 caveat。根修引入 `types.PrincipalSourceScope` 单一投影，
   relation handoff、flow evidence 和 final divergence 共同消费 production/test/docs/aux/all 权威。
5. B697：旧修补文案建议“移入 excluded[] 或辅助 aggregate”，模型照做 excluded-only 空集合后
   被下一合同拒绝。修补教学现只给一个 canonical 合法形：独立、非空、精确 members/value/
   support_refs 的 `role="supporting_coverage"`；明确 excluded-only 空 aggregate 无效。
6. active-stream 4ms 再核：现有行为 pin 分别覆盖“每 4ms 收到部分 SSE bytes，持续越过阈值仍成功”
   与“evaluator budget=4ms、模型 40ms 后返回仍不得成为流年龄门”。因此链接仍有字节活性但 4ms
   尚无完整 answer_document 时不会降级。只有 caller cancel/deadline、no-first-byte、真实 byte
   stall、transport/decode terminal 可重试或披露降级；系统不得以证据合成替代模型答案。
7. 本批没有扫描用户输入、thinking 或最终答案原文作硬门，没有让系统补边、改图事实或代写结论。
   Trace 与 Write 执行路径未动。修复后需以同一 Trace+类图精确二并发复放生产接线。
