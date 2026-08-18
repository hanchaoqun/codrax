# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T13:19:21Z
- sweep_start_ts: 20260818-061919
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-061921 | answer_regex,answer_contains | none | 288s | 34 | read=16,repo_map=1,list=0,trace=0,source_lens=0 | midloop=15,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | 两个题面虽分栏，但机制归属仍错：配置只写成项目根目录 `codrax.yaml`，遗漏六级 lookup 与 first-hit-wins；CLI 覆写仍被泛化为全量 flags。更严重的是“REPL Mermaid 降级”被第 1 路误导到 `internal/preview` 浏览器服务，终稿据此声称 HTTP 500 且无回退，忽略 REPL 的 `renderRichResponse -> RenderMermaidBlocks` 文本/ASCII 路径。证据分区没有串题，失败来自每个 unit 缺少精确 owner/mechanism closure。Runner 的宽正则不能证明答案正确。 |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260818-061921 | answer_regex,answer_contains | none | 372s | 43 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=13,inv=3/0,fin_reject=4,unavail=0,prune=2 | fail | B1078 正向回放：`typed_named_participant_relation_coverage` 为 `incident=[]`，日志不再发布 `AnalysisIR.MarkHypothesis -> nil` candidate，终稿也没有 `AnalysisIR -> BusContext` 伪边。sequence 最终只画三条 stage precedence，并把 AnalysisIR/AnswerDocument 诚实保留为断开 participant。仍有两类正确性缺口：表格漏用户明确要求的 `Mutable`，且把 `RenderAnswerDocumentWithLastMileSupplements` 错归给 AgentFinalizer，实际是 Orchestrator 在结构化文档之后调用。4 次拒绝分属漏 block kind、未证/错标关系、边 metadata 与可见边不一致、断开 participant 不可见；不是同一互斥合同。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Deterministic findings

1. `B1078-TERMINALRETURNCOMPONENTJOIN1` 获生产闭环。共享返回值 sink 隔离后，多个无关函数共同返回同拼写值不再连成跨操作关系分量；直接 requested return 的正向能力未被此次回放否定。
2. `B1079-DIAGRAMPARTICIPANTSURFACEPROVENANCE1` 仍开放。Analyzer 仍把 sibling table 示例里的 `AnalysisIR/AnswerDocument` 注册成图参与者。系统现在只能诚实要求可见断开节点，尚不能从 typed presentation provenance 阻止错误 roster 进入图合同。
3. `B1080-PATCHPAYLOADSTALEEDGECHURN1` 本轮未复现 r686 的“reasoning 说删、patch 原样保留”稳定形。拒绝从 6 降到 4，且第 4/5 轮 reasoning 与 payload 对齐；继续观察，不降低 typed relation gate。
4. `B1076-MULTITOPICMECHANISMCLOSURE1` 再次确认并扩成明确生产 witness：unit 分区/证据 ownership 已正确，Explorer 仍会用主题相似的 sibling subsystem（browser preview）回答命名 owner（REPL）的问题。最优修向是 typed unit owner + mechanism completion 的软补采/导航，不是扫描用户或终稿关键词，也不是强制某个文件名。
5. 两案均有过高 Explorer 轮次；Pipeline 还出现 JSON-string compatibility recovery 和 outer block kind 漏填。当前证据不足以证明系统存在互斥 JSON 合同，先作为结构化成文心智/教学观察，不用放松 schema 或让系统代写答案。
6. 本批无 Trace 输入，显式时间窗、Trace 因果投影、自动补齐、链上-only 主因与背景 support-only 均未触碰；活跃流未发生固定 4ms 降级。
