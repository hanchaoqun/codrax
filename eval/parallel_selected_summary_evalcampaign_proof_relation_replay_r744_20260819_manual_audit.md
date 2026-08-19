# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T21:57:19Z
- sweep_start_ts: 20260819-145717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-145719 | answer_regex,answer_contains,mermaid_edge_count | none | 549s | 49 | read=21,repo_map=6,list=0,trace=0,source_lens=0 | midloop=18,inv=7/0,fin_reject=4,unavail=0,prune=0 | fail | Runner 只要求至少一条 Mermaid 边，因此给出 PASS；人工审计判定未满足“各阶段与 Mutable/BusContext 之间的数据流”。首稿本来含共享状态总线关系，但 unsupported edge 修补后删到只剩阶段 precedence 与 dispatchStage 的两个局部 call；最终 BusContext/Mutable 成为孤立节点并披露 `Mutable/BusContext` 未证。根因不是无源码证据：探索已读到 `BuildAgentContext(o.busCtx,...)`、`TurnAArtifacts()`、`applyStageOutput`，而是 Analyzer 把用户的两个命名参与者合并为一个 `Mutable/BusContext` participant，同时把 entities 也改成同一合并词，绕过现有 composite-participant validator，使 flow participant coverage 在 investigation complete 记录为 0。B1187 的 source-direction 导航本轮因模型走另一条读取路径未获得生产正臂，只保留单测/本仓只读探针正证。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-145719 | log_regex,write_apply,answer_regex,answer_contains | none | 931s | 26 | read=8,repo_map=2,list=2,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 这次不是 B1188 的非权威 probe 误阻断：三个 verify 的真实 `make check` 都失败，依次为 ids 赋值前使用、五换行仍产出 `[300,300,10]`、最终错误删除全部换行而少 `[300]`，所以生产重规划是正确入口，B1188 精确资格未成立也未触发。系统忠实阻断了坏补丁，但 eval 固定 `pipeline-max-steps=15` 在第三次失败后耗尽；生产默认是 50。第二次重规划的上游语义流持续约 9 分钟且一直有 semantic bytes，系统没有在 4 分钟固定年龄降级，符合活跃流边界；模型生成约 57k 字的反复推理是独立效率/模型波动观察项，不能用固定总龄强切。建议 eval write lane 使用统一较高但仍低于生产默认的预算，不按单 case 特判。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
