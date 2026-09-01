# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T08:26:25Z
- sweep_start_ts: 20260901-012624
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260901-012625 | write_apply,answer_regex | none | 200s | 28 | read=4,repo_map=2,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 补丁把 `_prefault` 的真值判断改成结构存在判断，并补齐 false/0/空串回归；项目 `make check` 退出 0。runner FAIL 是诚实的 `impact_targets_unverified`：当前仓的 check 只做 Python 源码形状检查，Node/TypeScript 行为探针不可用。最终答复明确“未完全验证”，没有把静态检查冒充生产行为验证。模型内部曾叙述“所有 typed contracts 已通过”，但结构化完成权威没有采信，未污染用户可见结论。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-012625 | answer_regex,answer_contains | none | 776s | 52 | read=56,repo_map=2,list=0,trace=0,source_lens=0 | midloop=28,inv=10/2,fin_reject=6,unavail=0,prune=3 | pass | 最终答案保留合法 sequenceDiagram、四阶段顺序、输入/输出/状态载体表和代码引用；关系门删除无证据 fan-out/state-flow，只保留 `Orchestrator.Run→runAnalyzePhase`、`dispatchStage→applyStageOutput` 与三条 typed stage precedence。B1534 本轮未形成可验生产事件：首个关系 patch 在落地前因畸形 JSON/越权整块替换被拒，故没有“已删 forward、reply 后置悬空”的 unpublished graph。新确认 B1535：初始上下文发布 `principal_node_alias[n1]=analyzer`，live addition executor 却拒绝 n1 为 analyzer 的 typed carrier，给模型形成相互冲突的端点教学并增加至少一轮重试。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
