# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T04:27:43Z
- sweep_start_ts: 20260731-212742
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-212743 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 34579.490–34579.500s 窗、根因排序、两跳唤醒链、Trace 因果投影、窗内可消除量、枚举覆盖边界和成文前 critical_blocking_calls 自动补采均保留；B18b 无 Trace 回退。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-212743 | answer_regex,answer_contains | none | 199s | 24 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 主列表与 Mermaid 都只含 12 个 production 实现；3 个 test 实现仅作为 excluded 披露。typed roster 保持 15=12+3，source inventory 正确降为 support_only。相同 12 项又被系统 aggregate supplement 重复展示一次，另立 EVAL-B18-DUP1，不影响本案成员正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### LoopController

- `emit_investigation_complete` 在第一次有效汇总即接受了精确的 12 项
  production `member_set`，没有再把 3 个 test 实现提升为 principal；
- 日志中的完整 relation roster 仍为
  `complete=15, principal=12, auxiliary=3, unknown=0`，因此修复没有删除
  audit 候选；
- source-inventory authority 两次显示
  `authority=false ... reasons=support_only`，机械 type inventory 没有再覆盖
  typed relation 主集合；
- 相比 r1 的 273s / 10 次 midloop / 10 次 repo_map / 8 次 source_lens，
  r2 为 199s / 3 次 midloop / 3 次 repo_map / 0 次 source_lens。单次回放只证明
  权限争用和重复探索显著减少，不作为稳定性能比例承诺；
- 最终答案的模型 `ordered_list` 已完整列出 12 项，系统随后又追加
  “来自已验收的结构化调查清单”的相同 12 项。相同症状也出现在 B16
  called-by 关系案，因此登记为跨 relation kind 的结构化载体重复发布
  `EVAL-B18-DUP1`，不是 LoopController 或模型波动特例。

### H8 explicit-window sentinel

- 3 次 trace_query、0 次 midloop、1 次 investigation completion；
- 保留目标窗状态账、`NetworkService-60595` 第一根因、直接唤醒点与完整
  wakeup chain；
- 保留 `trace-causal-projection`、窗内可消除量总览、
  `enumeration_status=incomplete` 覆盖边界和 system supplement；
- 因而 B18b 的完成权限收敛没有影响有明确时间窗的 Trace 因果投影、根因
  排序、唤醒链、可消除量或自动补齐。
