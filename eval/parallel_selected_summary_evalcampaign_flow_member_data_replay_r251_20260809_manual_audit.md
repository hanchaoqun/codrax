# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T10:10:40Z
- sweep_start_ts: 20260809-031039
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260809-031040 | answer_regex,answer_contains,mermaid_edge_count | none | 210s | 34 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=1/0,fin_reject=3,unavail=1,prune=0 | fail | 用户要求四个 agent 与 Mutable/BusContext 的真实数据流；最终 Mermaid 只有两条入口调用边，其余节点全部断开，正文却继续宣称四 Agent 必然串行、共享同一 BusContext、逐阶段返回并 apply StageOutput，且把可跳过的 Extract 固定为第三阶段。三次成文拒绝均正确拦住无证 call/return/assignment/precedence 边；根因在上游 completion 用 2 个 support_refs 承载 4 个关系成员、且任意一条 flow edge 即可闭合。 |
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260809-031040 | log_regex,answer_regex | none | 264s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `17`，规则、decisions、contributions 与 reconcile 全部闭合；但过程 7 批、4 次 repair、1 次 action failure。确定性系统自冲突：评估态合法发布 `decision_next_actions=compute_contributions`，续批归一化却因 compute capability 反向铸造 `decision_records_required=true`，同轮 admission 随即拒绝该动作，迫使额外 qualify 批。初始 scalar custom_transform 不能满足已声明 contribution/reconcile 合同也浪费一批，作为独立效率项留档。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `qf_logic_view_read_pipeline`: human FAIL。runner 的 `mermaid_edge_count` 只证明有图和边，不证明用户要求的每个成员/边界/数据载体得到覆盖。现有 relation gate 三次拒绝均有精确证据，不能通过放宽门或自动删图消除；最终断图与过度正文同时存在，说明 B422 已跨 `sequenceDiagram`、`flowchart` 两种图型生产复现。
- B422 的根修仍需 producer-owned requested-member/relation coverage carrier：明确 universe/source，每个成员与每个请求属性分别携带 exact evidence 或 typed `unproven_boundary`；roster/order 与 call/assignment/return/data-flow 关系分轴。完成门只读该结构，不扫描用户请求、thinking、summary、表格或 Mermaid 标签，也不由系统代写答案。
- `data_basic_sum_with_rules`: answer human PASS、process GAP。`compute_contributions` 执行器只产生 contribution records，不产生 decision rows；但 capability 注册为同时产生 decisions 与 contributions，续批 normalization 因此把动作输出能力错误提升为新的前置义务，形成“先推荐、后自拒”的同轮合同自冲突。
- 最优通用修复是纠正 typed action capability 单源：compute 只声明 contributions；decision producers 仅保留实际发射 decision rows 的 filter/qualify（以及显式能发射全结果的 custom fallback）。同时移除 action-runner 中把 compute 当成 decision producer 的镜像手写分支，并增加“推荐动作经 plan normalization 后仍在同一 facts/admission 下可执行”的闭包 pin。
- 初始 custom_transform 的 scalar `emit_result` 与已声明 contribution/reconcile 合同不相容，当前靠执行失败后禁用脚本恢复。该问题需要基于 typed plan/script AST 或显式输出能力证明设计，不能靠扫描用户/模型 prose 或自动改写脚本；本批先留为后续效率项，不与确定性 capability 修复混做。
