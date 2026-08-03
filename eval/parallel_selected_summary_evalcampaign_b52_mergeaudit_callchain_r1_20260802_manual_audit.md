# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T06:51:32Z
- sweep_start_ts: 20260802-235131
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260802-235132 | answer_contains | none | 108s | 22 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 完整答案、源码与 2398 行日志审计：production caller roster 正确且完备（2 个函数），零 finalizer reject；第 295 行说明把实际的 `TypedRelationKindsForResolvedSources` 误写成当前 caller 名，是不改变集合的轻微表述错误。另有 2 次 quote 回填与 2 次 citation-ref 机械修复，未改变主值。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260802-235132 | primary_answer | none | 300s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=12,unavail=0,prune=0 | fail | 完整答案、fixture 源码与 3459 行日志审计：调用/容量位置大体正确，但 6 轮结构化成文均未闭合，最终发送 rejected draft + 19.6KB raw thinking；reply `-->>` 已不再要求反向 call，剩余 reject 是 call evidence 的 bare callee（`schedule/insert/record`）无法匹配图的 `Owner.method`。引用回填累计 26 次。答案还把 stdout `System.out.println` 叙述为“审计落库”，超出代码事实。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
