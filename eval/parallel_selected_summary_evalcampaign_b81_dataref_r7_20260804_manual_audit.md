# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T14:30:33Z
- sweep_start_ts: 20260804-073032
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260804-073033 | log_regex,answer_regex | none | 32s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `44`；规则、贡献、reconcile 与纯单值合同一致，无 reference authority 误激活。 |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260804-073033 | log_regex,answer_regex | none | 341s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 错误发布 `33,0,0`，应为 `17,0,5`。B80 reference projection 已能按 targets 三键补齐；上游 `apply_entity_resolutions` 却把 Gamma/unmapped 两行错接为 GroupA，5 条污染贡献随后 reconcile 自洽签绿。15 rounds、5 repairs、8 prior errors。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- `EVAL-B80-DATAREFEXEC1/SCOPE1` 的执行分支已真实生效：repair action 的显式 targets path/key 能完成三项 reference
  projection，且普通聚合不回归；本轮失败不再是“执行器不认确权 pair”。
- 新 P0 是值通道错接。typed resolution ledger 中 `Gamma alt→GroupC` 与 `unmapped→unresolved` 均正确，但
  `apply_entity_resolutions` 输出把二者分别写成 `A-one→GroupA`、`A-two→GroupA`，`matched=6/unmatched=0` 与真实
  `matched=5/unmatched=1` 相反。下游 contribution/reconcile 只验证污染后数据的内部一致性，无法发现来源身份错接。
- 机制面需修复 resolution application 的身份优先级：当 resolution ledger 提供 `source_value + source_field` 且 base
  存在对应字段时，该值身份必须先于隐式/局部行号；无 source-value match 必须保持 unmatched，不能让碰巧相同的数字
  locator 抢权。显式 `base_key_fields` 仍是用户声明的最高优先级。
