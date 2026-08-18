# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T15:17:40Z
- sweep_start_ts: 20260818-081739
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-081740 | answer_regex,answer_contains,mermaid_edge_count | none | 457s | 46 | read=22,repo_map=3,list=0,trace=0,source_lens=0 | midloop=12,inv=6/0,fin_reject=4,unavail=0,prune=0 | fail | B1084 兼容正证：请求明确把 `Mutable/BusContext` 放入关系短句，最终 typed participant scope 没有被闭域规则误删。人工仍不合格：4 次 finalizer reject 后，终图只画四阶段 precedence 与若干局部 append/call，未形成请求所要的各组件到共享载体的数据流；`Mutable` 仍以未证边界收尾。箭头直接显示内部枚举 `data_flow`、`call`，违背业务语言显示指导；末尾仍有系统“输出维度核对”块。Runner 只检查名词和至少一条 Mermaid 边，不能证明关系正确。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260818-081740 | answer_regex,answer_contains | none | 690s | 54 | read=51,repo_map=5,list=0,trace=0,source_lens=1 | midloop=27,inv=13/0,fin_reject=4,unavail=1,prune=1 | fail | B1084 roster 结果正向：最终 Analyzer 合同只有 `analyze/finalizer`，sibling table 的 `AnalysisIR/EvidenceItems/AnswerDocument/Mutable/BusContext` 未成为图硬参与者，表格仍保留全部载体。但后续 `analyze→finalizer` 的 verified 四阶段 endpoint span 未被判为 complete request spine，首轮拒绝后仍注入 24KB 全关系 capsule。终图膨胀为 17 个 participant，混入 Normalize、GraphState、AutoVerdicts、ContractCheck、AnswerReviewer 等次级实现，主时序不清；正文对 `runAnswerReviewerOnSuccess` 条件的描述前后矛盾。4 次成文拒绝、51 次 read，Runner PASS 仅证明格式/名词存在。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

1. `B1084-DIAGRAMSIBLINGSURFACEPARTICIPANT1` 的第一层生产有效：sibling table carrier 不再进入第一案的硬 participant roster；显式关系内 carrier 在第二案保留。未观察到 Trace、活跃流 4ms 或系统代写图回归。
2. 新确认 `B1085-STAGEENDPOINTSPANCOMPLETENESS1`：checkout provider 已从 typed `analyze/finalizer` 选出 `analyze -> explore -> extract -> finalize` 三条相邻 precedence，但 `CoversAllRequiredIncidentParticipants` 仍要求 selected span 的四个 stage 全在 participant roster。端点式时序请求因此永远只能算 supporting subset，触发全池 repair capsule。
3. `B1086-DIAGRAMBUSINESSDISPLAYENFORCEMENT1`：prompt 已要求 visible wording 使用业务语言，但终图仍把 `data_flow/call` 元数据直接显示。不能扫描终稿关键词做硬门；应在结构化 diagram message/edge label schema 上区分 model-authored display label 与 typed relation enum，并以结构字段做 lint/soft repair。
4. 显式 carrier 案说明关系证据供给仍不完整：局部 typed operations 足以证明 append/call，却没有把四阶段各自的产物写入/读取共享 carrier 组织成 request-scoped principal data-flow set，造成 `Mutable` unproven 与 4 轮修补。记 `B1087-REQUESTSCOPEDCARRIERFLOWSET1`，先审计 typed operation projection 与 participant candidate selection，不以系统补画或放松 gate 处理。
5. 两案均为源码 read 模式，无 Trace 查询。Trace 显式窗、因果投影、自动补齐、链上-only 主因与双轴根因表达均未受改动。
