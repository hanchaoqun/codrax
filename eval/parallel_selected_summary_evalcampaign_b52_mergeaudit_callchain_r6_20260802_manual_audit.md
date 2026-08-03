# Selected Eval Manual Audit Scaffold

- date: 2026-08-03T08:54:26Z
- sweep_start_ts: 20260803-015424
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260803-015426 | primary_answer | none | 165s | 19 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | 模型读完 5 文件，但未为 VisitController.create→VisitService.schedule 发射 citable call-edge；countOpenVisits 也仅为 recovered/lead。class-participant sequence 因缺席证据连续拒绝后被删。最终列表基本正确，但仍把 stdout println 说成“完成落库”，整答 FAIL；登记 B52f。 |
| 2 | qf_called_by_typed_relation_query | PASS | eval/results/qf_called_by_typed_relation_query-20260803-015426 | answer_contains | none | 193s | 21 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 只列 2 个 direct production caller，BuildTypedRelationQueryWithResolvedSources 的两处调用合并为一个函数席，TypedRelationKindsForRequest 为第二席；零系统重复清单/间接 upstream 扩界。该轮没有再次产生 omitted-role sibling，B52e 触发臂由生产 Execute pin 验收。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

- B52e 无回退：called-by 的最终答案及 finalizer principal lane 都只有 2 项，系统没有重新注入辅助集合；本轮模型未产生隐式 sibling，因此确定性正臂仍以 `EmitInvestigationComplete.Execute` 接线测试为主证。
- Java 的新问题不在 Mermaid 表达器：现有 class participant + exact message operation 正反测试已覆盖同端点多消息。失败发生在更上游——read 已覆盖调用行，但 completion 只交付路径 member-set/部分关系证据，定义和闭包摘要无法合法证明调用方向。最小泛化方案是对 typed `QFCallChain` 加跨语言软证据交接指导，要求每个已读、load-bearing 的直接调用各发一条 grounded call-edge；不自动造证、不放宽 final hard gate、不强制画图。
