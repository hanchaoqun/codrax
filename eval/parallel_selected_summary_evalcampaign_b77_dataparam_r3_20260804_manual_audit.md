# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T13:25:18Z
- sweep_start_ts: 20260804-062516
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260804-062518 | log_regex,answer_regex | none | 175s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终仅输出 `30`；规则 1、决策 5、实体归一 3、贡献 2、reconcile=pass。两条 Alpha 贡献为 20+10，终稿与 typed ledger 一致；qualify 参数合同未误伤 normalize/join/filter/compute 链。但 9 rounds/2 repairs/3 failed edges，主要是提前跨 DAG rank 与一次错误 resolution lineage，仍有规划效率 gap。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260804-062518 | log_regex,answer_regex | none | 297s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `17,0,5`；inactive r3 未进入贡献，GroupA 恢复 17；贡献 4、reconcile=pass，reference projection 按 targets 顺序补 GroupX=0、丢 GroupB。B76 参数消费根修生效。仍有 9 rounds/2 repairs/5 failed edges：无脚本 custom action、三输入 join、跨 stage 提前 compute/reconcile 及首次漏 complete-reference。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
