# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T00:42:08Z
- sweep_start_ts: 20260819-174206
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-174208 | answer_regex,answer_contains,mermaid_edge_count | none | 489s | 43 | read=10,repo_map=3,list=0,trace=0,source_lens=1 | midloop=7,inv=4/0,fin_reject=4,unavail=1,prune=0 | fail-system | 终稿列出不少真实阶段事实，但把 `Orchestrator.hasReusableTurnBSlateForFinalize -> o.busCtx.Mutable.Emitted*` 两条 getter 读取误写成 Extractor 向 Mutable 传递；图无业务标签、重复 `Extract --> Mutable`，并把未证 Mutable boundary 画成已连关系。日志确认两类系统放大器：Mermaid 展示引号被当成标签正文，正确 `|"runAnalyzePhase"|` 被拒；同一可见节点对对应两条不同 exact anchor 时，endpoint-retarget 检查因聚合冲突直接跳过。不是单纯模型波动。 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-174208 | log_regex,write_apply,answer_regex,answer_contains | none | 1202s | 30 | read=17,repo_map=3,list=0,trace=0,source_lens=2 | fail-model+safety-handoff-gap | B1202 生产正证：活跃写修复超过旧 900s 后继续到 1202s，没有按固定总年龄降级。模型三轮都试图把既有五换行回归改成错误预期，确定性 protected-test guard 正确拒绝；但被拒候选仍被记成 active plan，durable workflow 虽 blocked，原已执行计划的 final report 未同步，验收读到旧 `in_progress/missing`。需要修候选接纳顺序与终态按 canonical plan 轴投影，不能降低测试合同。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
