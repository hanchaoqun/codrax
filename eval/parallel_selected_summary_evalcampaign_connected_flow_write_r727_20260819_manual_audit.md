# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T10:07:10Z
- sweep_start_ts: 20260819-030709
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260819-030710 | write_apply,answer_regex | none | 150s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 补丁正确且严格 C++ project runner 通过：两份发布头都把 `%.*lg` 修为 `%.*Lg`，long double 两个入口均实际编译运行；但明确验收条目“普通 double 浮点格式化不受影响”没有对应执行断言，fixture 只调用 long-double helper。Controller 仅凭 changed-path `project_runner/target_behavior` 就签 `all_verified`，暴露 B1159：路径级行为覆盖不能代表验收语义维度已逐项覆盖。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-030710 | answer_regex,answer_contains,mermaid_edge_count | none | 364s | 37 | read=19,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=1,unavail=0,prune=0 | partial | B1157 安全生效：终稿没有再把两个关系孤岛伪装成一张已证连通图，也没有造桥；typed evidence 尚未闭合时保留 Mutable/BusContext 的明确未证边界。新确认 B1158：completion 已给 exact `extract_work.go:3-27` 补读，模型读到 line 15 的 `o.busCtx -> BuildAgentContext`，但 PendingRead 被清掉后工具面立刻恢复 grep/repo_map，模型转回宽泛搜索而没有先落 typed relation。根因是限制状态只跟瞬时 PendingRead 绑定，缺少同调度 materialization 生命周期。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

1. B1157 从 `pending-replay` 升为 `production-safe-boundary-r727`：系统能识别模型选边后的多岛图，并在
   typed 证据未证明 bridge 时允许明确边界，不通过强制画边或系统代写答案制造闭环。尚需后续回放取得
   “模型消费真实 bridge 并形成一张连通图”的正臂，不能把本轮 partial 虚报为生产全闭环。
2. B1158 是确定性系统 gap，不是模型波动。`emit_investigation_complete` 的 typed repair 精确导航到
   `internal/orchestrator/extract_work.go:3-27`；模型成功读取 line 15 后，mid-loop coverage 合法清掉
   PendingRead，但 `restrictedToolSurface` 也随之失效。下一轮完整 14-tool surface 恢复，模型改做宽泛
   grep/read，最终只保留未证边界。最优方案是在 exact typed flow-navigation PendingRead 首次出现时，
   为当前 explorer dispatch 锁存 materialization lane，直到模型落 `emit_evidence` 或诚实
   `emit_investigation_complete`；下一 dispatch 自动复位。信号只来自 typed origin+file+line range。
3. B1159 是独立写模式 P1：`changed_path_coverage=covered/project_runner/target_behavior` 只说明被改路径被
   项目 runner 触达，不能自动证明计划中的每个 acceptance criterion 都有执行观察。本轮普通 double
   不回归是用户原始要求、也是计划第 4 条验收，但测试只有两条 long double 调用。后续应建立 typed
   criterion→verification-observation 覆盖账，而不是扫描验收文本或凭测试名猜测语义。
4. 本批没有修改 Trace 查询/投影。显式时间窗、因果投影、自动补齐、链上-only 主因、实际占用与规则
   可消除量双轴继续保持；邻近/背景不得晋升主因。活跃流没有按 4ms 或固定 age 降级。
