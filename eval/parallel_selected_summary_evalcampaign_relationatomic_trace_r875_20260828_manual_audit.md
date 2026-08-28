# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T08:13:21Z
- sweep_start_ts: 20260828-011319
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-011321 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗与 3 次 typed query；四跳唤醒链、11ms 链上 IO 第一席、三个独立 1ms 优先级候选、实际占时/规则可消双账户、业务下钻、背景隔离与完整 Trace 因果投影均在。无固定 4ms/4m/活动流时长降级，成文零拒绝。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-011321 | answer_regex,answer_contains,mermaid_edge_count | none | 338s | 36 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=4,unavail=0,prune=0 | fail | B1361 的未授权 endpoint 当代拒绝生效，但 producer 未发布技术端点安全 ID：模型合理使用 ctxbuilderBuildAgentContext 被拒，被迫改用带点 identity。随后 source repair 把多行 subgraph 标题拆成 codraxNode 伪节点并让两个关系复用同一节点；最终图畸形、疑似不可渲染且关系表达失真。正文职责基本可读，但若干状态写入表述还需异构复核。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
