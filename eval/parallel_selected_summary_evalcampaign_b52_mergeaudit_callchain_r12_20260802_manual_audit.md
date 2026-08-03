# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T10:31:10Z
- sweep_start_ts: 20260803-033108
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-033110 | answer_contains | none | 114s | 20 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 只列 2 个 unique production caller；同一 caller 的两处 callsite 合并为同一行详情；无重复 system carrier。探索期 negative-scope 重试有噪声，但未污染最终集合。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-033110 | primary_answer | none | 138s | 19 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=4,unavail=0,prune=0 | pass | 最终 sequence 完整保留 5 条 typed 调用，reply 使用 -->>，容量条件使用 alt/else，未再伪造 self-call；核心结论正确。两项 advisory：h3 的 citation_ref 错指 countOpenVisits:18（运行时已精确发现但软放行），以及把 stdout 描述为 Append-only；二者不改变调用链答案，但前者登记为独立 typed relation-row citation gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Pair conclusion

- `B52j`：production replay covered。guard/self-call 首稿被拒，最终调用、回复和分支关系合法。
- `B52k`：production replay covered。最终 principal diagram 包含模型已选择且有 typed evidence 的全部调用。
- 新残余 `B52l-CITATION-CARRIER`：r12 的 principal hop 行采用 `caller → callee` 展示 label；现有精确 endpoint-label 完整性车道不解析模型字符串，因而无法用该行建立图闭包。修复应通过 item `citation_ref` 反查同文件同调用点的 citable typed `call_edge`；一处引用映射多个方向时 fail-open。不得以解析 label/text/RawRequest 作为 hard gate。
- 独立 advisory `B52m-RELATION-ROW-CITATION-ALIGNMENT`：h3 显示 `schedule → resolveMaxVisits`，却引用 `countOpenVisits`。系统已经从 typed role/evidence 精确发现并给出正确行 17，但当前只 advisory。该问题属于行引用归属，不是图表达；后续应在不改模型结论的前提下，评估用同一 typed mapping 机械修复 citation metadata。
