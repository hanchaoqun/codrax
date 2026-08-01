# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T04:58:00Z
- sweep_start_ts: 20260731-215758
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260731-215800 | answer_contains | none | 85s | 18 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 日志明确 canonicalized 1 duplicate relation row；最终仅一个 2 行 caller 表，BuildTypedRelationQueryWithResolvedSources 的 290/291 两个调用点保留在同一函数行详情，无第二系统清单。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-215800 | answer_regex,answer_contains | none | 131s | 26 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最终列表和图均为 12 个 production 类型且没有重复清单。但 typed relation roster 被错误投影为 principal=15：analyzer 的 typed test exclusion 被 irrelevant_files→all-scope 自动修复覆盖；本次由模型手工排除 test。另立 EVAL-B18-SCOPE1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### called-by

- completion 日志：
  `canonicalized 1 duplicate typed relation member row(s) onto the accepted candidate identity axis`；
- typed graph roster 与 final answer 均为 2 个 caller functions；
- `BuildTypedRelationQueryWithResolvedSources` 的 line 290、291 保留在同一表格
  行的调用位置和说明中，未丢 call-site 证据；
- 最终仅 summary + table + caveat，没有 aggregate duplicate carrier。

### implements

- 最终答案正确列出 12 个 production 类型并生成 12 条图边，没有 test 类型
  或重复列表；
- analyzer 发出了结构化
  `answer_exclusion_policy.excluded_candidate_roles=["test"]`，但同时把
  `agent_test.go` 放进 `irrelevant_files`；
- source-inventory negative-channel reconciliation 将该 test 路径提升为
  required，并自动合成 `source_scope=all/include_auxiliary=true`；
- relation lane 因而错误显示
  `complete_relation_roster=15; principal=15; auxiliary=0`，与 typed exclusion
  冲突。模型本轮自行只写 12 项，因此最终答案正确但权限协议不稳；
- 登记 `EVAL-B18-SCOPE1`，修复必须消费 typed exclusion 与 deterministic
  SourcePathRole，不扫描“主要”或最终答案文本。
