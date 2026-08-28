# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T11:44:03Z
- sweep_start_ts: 20260828-044401
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-044403 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、四跳 threadpool-400→network-300→cookie-200→app-100 唤醒链、链上 11.000ms IO 等待第一席、三个各 1.000ms 优先级反转候选、实际耗时/规则可消双维度和业务下钻都在。邻近 sleep 与窗口 IO 指数只作支撑/背景，未进入根因排序；完整 Trace 因果投影与同窗自动补采保留。146s 活跃流自然完成，无固定 4ms/4m 降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-044403 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 604s | 45 | read=46,repo_map=2,list=0,trace=0,source_lens=0 | midloop=18,inv=8/0,fin_reject=9,unavail=2,prune=2 | partial | B1375 生产转正：participant-only delta 首轮即发布共享技术端点 `ctxbuilderBuildAgentContext_860bba75bb1a60fb`，模型改用精确 carrier 后成功连接 BusContext 与 Extractor 两个可见组件；最终图语法合法、关系均来自 typed recipe，正文也正确说明 Mutable 是共享状态。9 次成文拒绝暴露新的通用心智负担：结构/atomic 失败不暂存，而 merged-document hard reject 会把合并稿暂存为下一轮基线；即时失败面没有把两种执行阶段做成短小精确状态，模型多次误判应整批重交还是仅补新修正。Orchestrator 最终仍为孤立声明，runner 的 incident-node 总数 oracle 未识别指定角色缺边，记 eval-only 后续量尺，不由系统强画关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
