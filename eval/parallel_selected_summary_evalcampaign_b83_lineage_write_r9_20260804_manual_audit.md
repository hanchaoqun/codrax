# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T15:07:51Z
- sweep_start_ts: 20260804-080750
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260804-080751 | write_apply,write_patch_oracle,answer_contains | none | 147s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单文件单行 `retrun→return` patch；apply 成功，1 个测试通过，workflow verified；未触碰主仓。 |
| 1 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260804-080751 | log_regex,answer_regex | none | 388s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `30`，2 条 Alpha contribution、reconcile pass；10 rounds、2 repairs、7 prior errors。上轮两次错误 `apply_resolution_lineage_contract` 均消失，父/子角色修复 production 生效。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- `EVAL-B82-DATALINROLE1` production 闭环：同一 join witness 不再把 resolution source 误报为 canonical_records，也不再
  拒绝 source descendant；data rounds 13→10，最终值与 ledger 不变。
- 仍有 7 个 prior errors，均属于已在账的 DAG rank/allowed-next-action 上下文问题（空 input、跨 rank、stage 不允许），
  不应并入 lineage 修复，也不通过放宽 guard 处理。
- write 异构席完整经过 analyze→plan→apply→verify→finish；计划只有一个精确 patch，验证报告通过，证明 data 权威修复未
  影响 write controller 或隔离 worktree 红线。
