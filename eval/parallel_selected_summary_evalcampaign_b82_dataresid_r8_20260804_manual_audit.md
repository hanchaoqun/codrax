# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T14:52:37Z
- sweep_start_ts: 20260804-075235
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260804-075237 | log_regex,answer_regex | none | 232s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `17,0,5`；4 条 contribution 为 GroupA 10+7、GroupB 4、GroupC 5，reconcile/projection 同源；unmapped 未污染 GroupA。13 rounds、2 repairs、5 prior errors。 |
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260804-075237 | log_regex,answer_regex | none | 396s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `30`；Alpha 两条 contribution、reconcile pass。但 13 rounds、2 repairs、8 prior errors：normalize 自动交换 source/reference 后父 lineage 仍按原 input 顺序反写，合法 apply 被 lineage hard guard 两次拒绝，模型最终绕到 join。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- `EVAL-B81-DATARESID1` production 闭环：原 witness 的 Gamma/unmapped 污染消失，contribution count 从错误 5 恢复为 4，
  exact answer 与 reference projection 同时正确。
- 新确认 `EVAL-B82-DATALINROLE1`：`name_resolutions` 的 typed child 明确记载
  `items_records#entity_source` 与 `canonical_records#entity_reference`；parent 却记成
  `source_record_paths=[canonical_records] / reference_paths=[items_records]`。hard guard 因此错误声称 resolution source 是
  canonical_records，并拒绝应用到 items_records 及其 derive descendant items_with_amount。
- 该问题来自角色自动交换后的 parent lineage 没有消费执行器已经铸造的 typed child roles，不是模型参数歧义。修复应使
  parent role lineage 从实际 source/reference child 单源生成；不得放宽 incompatible-lineage guard。
