# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T17:31:37Z
- sweep_start_ts: 20260731-103136
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d2_chain_via_networkservice | PASS | eval/results/real_trace_d2_chain_via_networkservice-20260731-103137 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 121s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | 主结论与引擎一致：ThreadPoolForeg-60555 → NetworkService-60595 → CookieMonsterCl-59843 → main-59566，NetworkService 为中转；call-chain/diagnostic 权限正确发布因果投影。模型称“4跳”实为4节点/3边，并把低优先级唤醒候选表述偏强，记模型波动。重复 pair 有多个真实 edge point，批 S 单点通道诚实 unavailable，未任选一次；后续需 typed 多点/首末范围。 |
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260731-103137 | log_regex,answer_regex | none | 171s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终只输出30；terminal 为 complete，2条贡献、3条实体归一、reconcile=pass。过程9个执行批、5条历史拒绝（3次初期模型计划问题，2次多阶段计划被确定性拆批）；产品正确但效率偏低。eval metrics 的 result_summary 被 escaped quote 截成反斜杠，且 action event 使用 `Status` 导致 failed=0，属确定性审计基础设施 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
