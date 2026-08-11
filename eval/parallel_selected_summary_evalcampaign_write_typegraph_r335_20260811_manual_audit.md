# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T20:35:13Z
- sweep_start_ts: 20260811-133511
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260811-133513 | write_plan,write_patch_oracle | none | 56s | 22 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单行 C++ patch、范围和 acceptance tests 均正确；本轮 planner 没有提交 inline verification probe，因此计划合规，但没有生产命中 B567 的拒绝臂，记为 not-exercised-r335。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-133513 | answer_regex,answer_contains | none | 407s | 28 | read=31,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=6/1,fin_reject=5,unavail=0,prune=0 | fail | typed implementer 名册与 12 个生产实现表格正确，但系统把显式类型关系请求归入 call_dag；`type_relation` 硬门又没有消费同一份 typed 名册，12 条 implements 边先后被判 unproven。5 次拒绝后模型被迫删除所有边，最终 Mermaid 仅剩节点，并以“保留第一稿”降级交付。runner 的 answer_contains 未覆盖关系完整性，属于假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B567`: implementation remains valid, but this replay is not a production-positive witness because the model emitted no inline probe.
- `B568-TYPERELATIONHANDOFF1/P1-high`: confirmed structural contract break. The graph and finalizer context contain a typed `implements` roster, while diagram validation consumes only citable `EvidenceItem` type-relation rows. When exploration reaches the roster through `repo_map`/typed hints without the deterministic EvidenceItem bridge, the same system both requires the relationship diagram and rejects every true edge.
- This is not a Mermaid rendering failure and not model fluctuation. The model first emitted all relationships in both directions, then obeyed repeated typed-contract rejects and removed them. The fix must bridge typed relation authority; it must not relax the relation gate, infer edges from labels, scan request/answer prose, or have the system author the diagram.
- The 407-second active model stream completed normally. No fixed four-minute elapsed-time degradation or system-authored replacement answer occurred.
