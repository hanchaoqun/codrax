# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T12:57:37Z
- sweep_start_ts: 20260901-055737
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-055737 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 144s | 45 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | 精确 10ms 窗、已证唤醒链、链上根因排序、实际占时/规则可消双账户、业务 span、邻近/背景隔离、帧因果边界、Trace 因果投影与确定性补采均在。模型本轮没有提交可选 `trace_root_causes` 选择载体，所以默认 sibling `.root-causes.json` 按旧合同不生成；这不是 B1545 的双层字符串恢复回归，也不表示根因为零。显式 `--root-causes-out` 的 B1543 强交付路径在该形下应写 `status=unavailable` typed envelope；B1545 本轮未命中生产正臂。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-055737 | answer_regex,answer_contains | none | 524s | 54 | read=25,repo_map=4,list=0,trace=0,source_lens=0 | midloop=14,inv=4/1,fin_reject=6,unavail=0,prune=4 | fail | B1546 的 analyzer fail-loud 获得生产正证：首轮无请求锚的 Orchestrator/BusContext 等 roster 被立即拒绝，模型第二轮只保留请求明确点名的 analyze/finalizer；成文拒绝从 r1010 的 12 次降到 6 次。但最终图仍把 typed stage ordering 画成 AgentAnalyzer→AgentFinalizer 的直接消息边，并保留孤立 MutableState；正文四个 stage 又重复引用同一 runAnalyzePhase 行，不能精确证明 Explore/Extract/Finalize。说明错误 roster 后移已缓解但阶段顺序候选、reader actor 与引用精度仍未闭环，不能用机器 PASS 收账。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
