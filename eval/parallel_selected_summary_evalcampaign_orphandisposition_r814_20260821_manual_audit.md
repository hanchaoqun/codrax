# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T16:19:13Z
- sweep_start_ts: 20260821-091912
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-091913 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 2.000000..2.020000 窗、四节点三条唤醒边、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户、邻近/背景隔离、自动补齐和 Trace 因果投影均完整；无固定 4ms/4m 降级。模型将四节点口语写成“四跳”，并一处把调用点扩写为页缓存完成语义，后文已正确披露未证边界；作为模型措辞观察，不增加正文关键词硬门或系统改写。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-091913 | answer_regex,answer_contains | none | 619s | 45 | read=26,repo_map=3,list=0,trace=0,source_lens=1 | midloop=11,inv=4/0,fin_reject=2,unavail=0,prune=8 | partial | 最终答案、四阶段表和合法 sequenceDiagram 均存在；B1296 的 live ref 单权威生效，B1297 在删边造成 Orchestrator 孤立后要求模型显式选择 retain_as_context，系统未替模型选边或标签。但 explorer 34 轮后，7 条 member-row grounded support 仍未闭合，completion_form 的低增量出口却把它当纯落地形债接受，相关 aggregate 仍以 supporting_coverage/exact_source_support 进入 finalizer；同时初稿 11 条编排交互被关系门删除，最终图只剩三条阶段 precedence 和一个显式保留的孤立 Orchestrator。关系删除本身符合当前证据门，先记表达观察；未闭合 member aggregate 继续进入权威 handoff 是已确认系统 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual verdict

- Runner: 2/2 PASS；人工：Trace pass，read partial。
- B1296/B1297 获得生产正证：opaque ref 不再被冗余坐标拖入循环；删边后孤立节点必须由模型明确删除或作为上下文保留，本轮模型选择保留并提供 `Orchestrator` 标签。
- 新确认 `B1298-MEMBERGROUNDINGCONVERGENCE1/P1`：`member_set_support_refs` 同时承载数组/格式债与“引用不证明该成员责任”的语义 grounding 债，二者却共用 `completion_form` 低增量出口。后者收敛后没有剔除已知无效 aggregate，导致 finalizer 仍看到 `exact_source_support=true` 的 supporting coverage。最优修复是由 validator 返回 typed offending-fact 集；保留有限修补机会，若仍收敛则只剔除 offending aggregate，并保留独立 grounded evidence/checkout stage authority，禁止解析错误字符串或改写模型答案。
- 图表观察暂不立硬修：当前可用 typed additions 只有阶段 precedence，初稿抽象 `Orchestrator -> stage` 并不等同源码直接 call。后续异构架构/时序例继续判断是否缺少可表达的 callback/argument/data-flow provider；不得为本例放宽 call 证据门。
