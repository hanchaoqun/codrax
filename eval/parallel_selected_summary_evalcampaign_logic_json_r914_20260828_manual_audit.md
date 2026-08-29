# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T00:16:21Z
- sweep_start_ts: 20260828-171620
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260828-171621 | log_regex,answer_regex | none | 35s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | data lane 正确读取完整 `instructions.md` 与 `users.json`，按源顺序筛出 active 用户，最终用户答案严格只有 `{"ids":["u1","u3"]}`，无代码围栏、解释或内部枚举；首批即闭合，零重试。过程中的模型自疑没有改变 authoritative material coverage 或最终结果。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-171621 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 750s | 53 | read=43,repo_map=3,list=0,trace=0,source_lens=1 | midloop=42,inv=22/0,fin_reject=7,unavail=0,prune=0 | partial | Mermaid 语法可渲染，Analyzer→Explorer→Extractor→Finalizer 三条 precedence 与各组件职责基本正确；但成文经历 7 次 reject/patch。B1425：同一 `bus.Mutable -> AgentContext.Mutable` typed edge 的修补提示要求复用既有 `BC_mutable`，可执行 lease 却同时允许新 `Mutable`/技术 alias，模型四次在互斥端点间失败，最终通过新增隐式 `Mutable` 节点，和图内既有 `BC_mutable[Mutable]` 形成身份分裂，关系表达仍怪异。已改为 mapped participant side 有现存精确节点时只允许这些节点，无现存节点才给 participant fallback；技术 identity 留在 anchor。B1426 另立：单一六参与者关系图被 analyzer 拆成 5 sub-topic，触发 5 路重复调查、43 reads、22 completion attempts、750s；应按 typed relation scope 合并证据目标，不能按耗时硬停。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
