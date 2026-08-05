# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T21:25:23Z
- sweep_start_ts: 20260805-142521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260805-142523 | typed_inventory_rowset,answer_contains | none | 115s | 21 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=1,unavail=5,prune=0 | pass | 生产答案完整列出 4 个 @Entry 与 2 个 @Builder 成员及精确位置；runner 仅因要求完整展示标题“@Entry 标记的 ArkTS 页面入口”、而答案使用“@Entry 页面入口”误判。首次 emit 已有完整 Markdown 表，结构修复时模型误把全文替换为无 columns 的 cells 表，造成“列 2/3/4”；属 patch 全替换教学心智负担，不影响成员事实。source_inventory 完整后又尝试 5 次不可用 read_file，记模型/工具面波动。 |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260805-142523 | log_attachment,answer_contains | log_triage | 285s | 18 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正确区分仓颉 `demo.bridge.ohSum:18` 越界触发、`checkout:42` 调用传播，以及 ArkTS `NativeBridge.invokeOhSum:33:11` 包装暴露、`HomePage.computeTotal:54:7` 上层调用；明确是外部运行时快照且当前仓无交集。log triage 首次 evidence 非 verbatim 后精确修复；Analyzer 首字节超时后重试成功，是 285s 主因，不新增代码门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
