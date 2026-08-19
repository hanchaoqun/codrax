# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T09:35:05Z
- sweep_start_ts: 20260819-023503
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260819-023505 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面完整保留 5.000–5.007s 显式窗、Trace 因果投影、链上席位、实际占时/业务线索与规则计价两轴；模型却把“帧因果未证”的 0.800ms runnable 候选写成“直接瓶颈”，并在客户正文复制 causal_conclusion/frame_evidence_status/runnable_wait/chain_relevance 等协议词。typed 上下文已明确 unproven/absent 与读者语言要求，判为模型措辞波动/软教学残余；不以答案关键词硬门拟合。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-023505 | answer_regex,answer_contains,mermaid_edge_count | none | 418s | 37 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=18,inv=7/0,fin_reject=2,unavail=0,prune=0 | partial | B1156 生产正证：有界导航命中 extract_work.go:15，并取得 types.AgentExtractor/o.busCtx → BuildAgentContext 两条 typed 参数边；两次 finalizer reject 都是删除无证 Orchestrator 调用与补 participant incidence 的正确纠偏。最终图仍把四阶段与 BusContext/Mutable 画成两个岛，暴露 B1157：逐参与者合法不等于 requested flow 连通。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Trace 人工审计

- 精确窗口、目标四态、worker 唤醒、跨 CPU、0.800ms runnable 等待、5.000ms VerifyClass 实际占时及其规则可消除 0.000ms 均在答案中；背景席未晋升主因，`Trace 因果投影` 未丢失。
- 系统补齐的加冕行明确带“帧因果未证”，覆盖边界也明确“未找到可绑定到目标的帧或截止期证据”。因此模型正文中的“造成帧延迟的直接瓶颈”超过现有证据权限；这次不增加原文扫描/关键词拒绝，也不由系统改写模型结论。
- `causal_conclusion=unproven` 等协议词仍泄漏到客户正文。当前 finalizer 上下文已有 typed 读者语言卡，未发现相反合同；保留为跨模型回放观察项，不以单案例硬拟合。

## QF 人工审计与 B1157

- B1156 已命中真正的跨组件 binding：模型读取 `internal/orchestrator/extract_work.go:15`，证据池与成文 recipe 均包含 `types.AgentExtractor -> BuildAgentContext` 和 `o.busCtx -> BuildAgentContext`。旧的六文件截断/局部 Mutable 饥饿已闭环。
- 最终图中的每条边单独合法，但参与者关系分成 `{Analyzer, Explorer, Extractor, Finalizer}` 与 `{BusContext, Mutable}` 两个组件，未回答“六个组件之间的数据流”。Runner 的 edge-count oracle 没有识别该语义缺口。
- B1157 根修使用 typed 弱连通组件而非答案文字：completion 在证据仍分岛时只追加一次有界 bridge 调查；finalizer 仅当完整 typed evidence 已证明全体参与者连通时，要求模型可见锚定边保持连通，并发布现有 join-frontier 候选。证据不连通时仍允许显式 `unproven` 边界；系统不创建证据、边、图或答案。
- 活跃流未按 4ms/固定年龄降级；本轮无 JSON/mermaid 恢复或 liveness 异常。
