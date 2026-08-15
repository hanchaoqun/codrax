# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T17:17:59Z
- sweep_start_ts: 20260815-101758
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-101800 | write_plan,write_patch_oracle | none | 52s | 24 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只读计划准确定位 `main.py:20`，使用单个 structured replace 把 `retrun` 改为 `return`；未 apply、未扩大改动面，Python dry-build 通过。摘要有一次“`retrun` 被误写为 `retrun`”笔误，但不影响精确 patch。 |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-101800 | answer_regex,answer_contains | none | 170s | 31 | read=5,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 模型事实正确：总数 1、完整成员 `explorer`、注册与 `Name()` 证据均在。但生产日志仍为 `fact_authority=advisory_model_inference/principal_contract=not_authorized`，继续发布“证据支持稍弱” caveat。后置刷新段实际执行但零变更。冷读确认 16,397→160 压缩保留 bridge、丢弃其 `DerivedFrom` terminal，属于 typed 证据引用闭包被压缩破坏；runner PASS 不能关 B844。r517 的无关 hitrace citation 本轮未复现，但不足以关闭 B846。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Conclusion

- Machine: `2/2 PASS`; human: `1 pass / 1 partial`.
- B844 仍未生产闭环。`typed_relation_authority_refresh` 已执行；失败点已从“刷新时机”进一步收敛为压缩器破坏 `DerivedFrom` 引用闭包。
- 下一修复必须是通用 evidence-graph compaction：按 stable evidence ID 保留被选中节点的传递依赖闭包，并在固定预算内淘汰低优先孤立项；不得按 `SubExplorer`、某个文件、最终答案词面或用户关键词特判。
- B846 本轮未复现，维持 P1 open；一次不复现不等于 citation pool/remap 合同已经闭环。
- 本批没有修改 Trace 路径。显式时间窗、因果投影、系统自动补齐、typed on-chain-only 主因、背景 support-only、实际占用/业务线索与规则计价可消除量双轴均保持；系统不替模型写结论。
- 活跃字节流没有固定年龄降级；4ms/4s/4m 都不是结束信号。
