# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T05:07:19Z
- sweep_start_ts: 20260731-220717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260731-220719 | answer_contains | none | 117s | 20 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | typed roster=2 principal；completion 继续 canonicalize 1 条重复 observation，最终仅一个 2 行列表，两个调用点留在同一 caller 详情。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-220719 | answer_regex,answer_contains | none | 126s | 19 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B18e 权限目标通过：roster=15、principal=12、auxiliary=3，列表/图只含 12 个 production，3 个 test 明确在 caveat。但 flowchart 使用 classDiagram `<|--`，兼容层改成 codraxNode 伪节点，关系图源码损坏；另立 EVAL-B18-DIAG1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### called-by

- `complete_relation_roster=2; principal=2; auxiliary=0; unknown=0`；
- completion 仍记录 1 条 duplicate observation canonicalization；
- 最终 summary + 2 行 ordered-list + caveat，无重复载体。

### implements

- analyzer typed scope 为 production 且 `excluded_roles=test`；
- relation roster 恢复为
  `complete=15; principal=12; auxiliary=3; unknown=0`，没有 synthetic
  all-scope；
- 最终 12 项列表、12 条 diagram edges，3 个 test 实现只在 caveat；
- 但模型在 `flowchart TD` body 中使用 classDiagram operator
  `LoopController <|-- analyzerEvaluator`。现有 unsafe-node shim 将 `<` 与
  `--` 改造成 `codraxNode1/codraxNode2`，输出不再是清晰的实现关系；
- 登记 `EVAL-B18-DIAG1`。修复应在 Mermaid source compatibility 层做
  grammar-aware soft rewrite，不新增答案硬拒绝，也不扫描答案 prose。
