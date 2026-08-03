# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T10:47:27Z
- sweep_start_ts: 20260803-034726
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-034727 | answer_contains | none | 91s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 最终只列 2 个 distinct production caller；两处 BuildTypedRelationQueryWithResolvedSources callsite 合并为一名 caller，测试文件排除正确，无重复 carrier。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-034727 | primary_answer | none | 103s | 20 | read=4,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/1,fin_reject=2,unavail=0,prune=0 | pass | B52l production trigger：首稿缺 countOpenVisits/insert/AuditLog 三条 principal call，hard typed gate 明确拒绝；修补后 flowchart 完整保留 5 条调用且 guard 节点不再假装 call。残余：正文把 6 nodes 称为“6跳”，并把 stdout 称作审计落库；系统补齐清单若干行同时描述 callsite 与 callee 内部行为，引用只能覆盖其中一侧。核心调用链/容量位置仍正确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52l=production-trigger-covered`：真实首稿的 edge-shaped/citation-selected calls 触发 `principal_call_edge_missing`，最终图闭环；finalizer rejects 从 r12 的 4 降至 2。
- 15-language authority remains structural：14 executable language fixtures are checked against `SupportedReadLanguages()`；Proto has a declarative no-call counterarm。
- `B52m` remains open and is broader than one wrong line：当一个 item 同时叙述 caller→callee 与 callee 内部行为时，单一 citation_ref 只能证明其中一层。最优方向是 typed multi-claim/per-claim citation carrier 或拆项 soft guidance；不得解析模型 prose 后硬改引用/结论。
- 新 advisory `B52n-NODE-EDGE-CARDINALITY-WORDING`：typed aggregate 是 6 nodes，但模型写“6跳”；结构化值本身正确。没有 typed noun-axis carrier 时不应按中文/英文“跳/节点”关键词硬门，先保持模型波动/advisory。
