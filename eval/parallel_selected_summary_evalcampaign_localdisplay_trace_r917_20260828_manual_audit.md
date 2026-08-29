# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T01:43:02Z
- sweep_start_ts: 20260828-184301
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-184302 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、threadpool-400→network-300→cookie-200→app-100 四跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时与规则可消双账以及完整「Trace 因果投影」均在。正文明确 threadpool IO 只是链上前置重叠，未把未证的直接阻塞关系写成结论；邻近/背景未升为主因，零成文拒绝、零降级旧稿。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-184302 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 636s | 59 | read=28,repo_map=3,list=1,trace=0,source_lens=1 | midloop=31,inv=10/0,fin_reject=10,unavail=0,prune=0 | partial | B1428 获生产正证：BusContext→BuildAgentContext、Extractor→BuildAgentContext、BusContext→Mutable 三条 typed addition 通过后置 identity gate 并保留到最终图。组合后的 typed 图已连接全部请求参与者，因此移除 BusContext/Mutable 的 unproven 边界是正确行为，并非 local-only 合同冲突。确定性新 GAP 是两条同端点 `applyStageOutput→BusContext` 未证 data_flow：producer 给出 failure_ref，却因重复 prior anchor 将两条都标成 target_carrier=unknown、allowed_actions=[]；模型被告知删除但没有可执行 ref，造成 10 次拒绝。最终图合法但三条阶段 precedence 重复显示，载体交互仍偏薄，整体仅 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed follow-up

- `B1430-REPEATEDPRIORANCHORFAILURECAPABILITY1`（P1）：同一可见端点对存在多条同关系、同技术身份但各有独立 body occurrence 的未证 prior anchors 时，失败引用必须按 parser-owned `body_occurrence` 铸造成独立 `visible_body_edge/remove` 能力；不能发射 `target_carrier=unknown, allowed_actions=[]` 的伪可执行引用。
- 修复必须保持模型选择：系统只恢复每个失败在不可变拒绝草稿中的精确 occurrence 和对应 anchor occurrence，不代模型选择删除/保留、关系、方向、标签或布局；请求、thinking、正文和 Mermaid message 均不作为事实或硬门信号。
