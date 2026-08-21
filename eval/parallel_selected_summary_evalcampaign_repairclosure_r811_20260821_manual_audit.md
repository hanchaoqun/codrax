# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T12:58:21Z
- sweep_start_ts: 20260821-055821
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-055821 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 精确 2.000–2.020s 用户窗、四节点唤醒依赖链、11.000ms 链上 IO 主席、三席 1.000ms 调度候选、占用/可消除双账、背景隔离、完整 Trace 因果投影与确定性补采均保留，未发生固定 4ms/4m 或旧草稿降级。模型同时披露 target_direct_blocking_authority=not_provided，却又把 IO 写成“导致下游链整体向后推迟/由完整唤醒链传导延迟”，把已证链上候选轻微扩写成未证传播机理；归入 B1269/B1271 的软引导残余，不用答案文本扫描、系统代写或硬门纠正。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-055821 | answer_regex,answer_contains | none | 598s | 51 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=16,inv=5/0,fin_reject=4,unavail=0,prune=1 | partial | 最终答案可用：四阶段表齐全，Mermaid 合法，三个阶段先后关系有 typed anchor，声明的短 ID 未被 addition 另造隐式 participant。相较 r810 的 1279s/20 次拒绝/19 次补丁已显著收敛，但本次没有自然复现 B1293 的“同一可见关系多失败 ref”生产形，因此 B1293 仍以生产接线测试为正证。图仅保留 Analyze→Explore→Extract→Finalize，Orchestrator 与 BusContext 在图中孤立，状态交互只在正文/表格解释，未充分满足“完整时序+主要状态载体”的图形表达；不能靠系统虚构边或放松 typed 门补齐。analyzer 另发生 3 次确定性合同返工：先漏填泛化必填 call_chain_endpoints，后两次把预扫发现的 Orchestrator/Explore 当 CURRENT-request participant，被精确来源门拒绝；这是教学/字段职责 gap，不是来源门错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 2/2 PASS，但人工正确性均为 `partial`；未把 runner PASS 误记为系统闭环。
- B1293 的新事务闭包没有引入 Trace 回归，并显著降低同类读场景的修补锁死风险；本轮没有自然命中它的精确生产形，继续保留 production-replay-pending 状态。
- 新建候选 `B1294-ANALYZERDIAGRAMPARTICIPANTAUTHORITY1/P1`：`diagram_hint.participants` 只应承载当前请求精确点名的关系身份；预扫/源码发现的组件应在 evidence 阶段成为可引用候选，不能伪造 CURRENT-request provenance。保留现有精确来源硬门，根修 analyzer 的 JSON 教学与职责说明，减少模型心智和无效重试。
- Trace 模型的传播机理过述继续按 B1269/B1271 观察并以 typed soft guidance 改进；禁止扫描原始回答硬门或由系统接管结论。
