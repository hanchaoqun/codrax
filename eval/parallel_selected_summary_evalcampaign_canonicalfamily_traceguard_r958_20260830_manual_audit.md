# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T05:45:04Z
- sweep_start_ts: 20260830-224503
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-224504 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、typed 唤醒链、根因排序、双账户、最终 Trace 因果投影与自动补齐均在；但模型主摘要把 NetworkService 的 runnable 调度等待称为“确定性优化工作”，并漏答真正的 `VerifyClass` 0.285ms 语义 span。系统投影随后正确披露 VerifyClass、T7 直接唤醒锚与规则可消 0ms/完成依赖未证，不能抵消模型主回答的类别错误。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260830-224504 | typed_inventory_rowset,dimension_substring,answer_contains | none | 334s | 28 | read=0,repo_map=5,list=0,trace=0,source_lens=5 | midloop=5,inv=7/3,fin_reject=2,unavail=7,prune=0 | pass | 可见答案精确列出 extend=2、foreign func=2、public class=8，12 条声明、文件与 package 完整且未再派生错误文件数。不过 finalizer 上下文同时发布了两组指向同一 foreign 声明的聚合行，内部错误出现 `foreign func=4, family_coverage=14/14`；模型碰巧依据权威行写对，流程仍确认 B1472。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. Runner 的 2/2 PASS 只证明既有结构 oracle；人工结论为 Cangjie pass、Trace fail。
2. `B1471` 的可见主家族分区获得生产正证，但出现新 P1 `B1472-INVENTORYAGGREGATEIDENTITY1`：两个不同显示名的 aggregate 指向相同 typed family+source location，预收敛行被重复计数。修复必须复用 source-inventory principal declaration identity，而不是按 `row_id` 或可见 label 去重。
3. Trace 上下文已经发布 `deterministic_semantic_spans`、T7 宿主、0.285ms、直接唤醒锚及“完成依赖未证”边界；错误来自模型把 scheduler-state row 与 semantic-span row 混类。记 `B1473-TRACESEMANTICCLASSBOUNDARY1`，先用 typed 类别边界做 soft guidance，不扫描/拒绝/改写模型正文。
