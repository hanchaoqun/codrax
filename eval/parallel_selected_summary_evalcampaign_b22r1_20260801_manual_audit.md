# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T15:15:47Z
- sweep_start_ts: 20260801-081546
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260801-081547 | write_plan,write_patch_oracle | none | 78s | 19 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan-only 正确生成单文件单行 `kind=patch`，`retrun`→`return`，未改主仓。write analyzer 的未绑定 hard expected 与 planner 的 tab/space `old_text` 各被 typed reject/repair 一次后收敛；最终 diff 可 apply，验收项与 fixture 一致，属于安全自恢复而非新 gap。 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-081547 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 31 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 显式 5.000–5.007s 窗、两维报告、根因候选排序、唤醒链、代表窗、窗内可消除量及系统补采均完整；typed `frame_causality=unproven` 已结构化接管结论并移除 model definite-cause prose，B19-CAUSAL1 获正证。但“确定性优化点”把 VerifyClass 完整 span 5.000ms 写成“有效成本/71.4%”，同一投影明细的 typed 有效归因是 `span∩chain=4.600ms`，形成原始占时与规则可消除量混轴硬矛盾；runner oracle 未检查跨区块口径一致性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
