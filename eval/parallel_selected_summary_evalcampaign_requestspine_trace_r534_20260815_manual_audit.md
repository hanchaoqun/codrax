# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T22:46:33Z
- sweep_start_ts: 20260815-154632
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260815-154633 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 227s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 生产正证：显式 11.000..11.008s 窗、Trace 因果投影和系统补齐均在；链上 #1 仅为 app-20 runnable 调度供给 5.000ms，rival/pressure 留在背景；实际占时与规则可消除量双轴齐全。模型正文另造了无 typed 测量的“切换开销 1.6ms（4×0.4）”，把目标线程 running 占比误称 CPU 实际利用率，并写错若干片段数；系统投影未复写这些结论。记 B861 软引导/结构化单位语义 gap，不用 prose 硬门。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260815-154633 | answer_regex,answer_contains | none | 256s | 39 | read=13,repo_map=4,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=2 | fail | B859 正证：首稿和终稿均无 write_analyze 污染，四阶段主序列正确。但 analyzer 修 participant 合同时把 diagram_hint.required 降为 false，而 required diagram dimension 仍为 true；stage spine 未启用，关系校验把图称为 optional，模型 patch 删除用户明确必需的 sequenceDiagram，机器仍误判 PASS。确认 B860 typed 合同一致性 P0；单次拒绝/单次 patch，不是 B857 瀑布。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Machine: `2/2 PASS`; human: `0 pass / 1 partial / 1 fail`.
- `B859-REQUESTSPINECONTEXTPRECEDENCE1` achieved its intended context-priority effect: no cross-mode `write_analyze` contamination survived.
- New P0 `B860-DIAGRAMREQUIREDCONTRACTCONSISTENCY1`: two schema-validated carriers disagreed on whether the same diagram was required. The false sibling value disabled the request-spine provider and allowed deletion of an explicitly required visual.
- New P1 `B861-TRACEDERIVEDMETRICCALIBER1`: the typed Trace answer correctly separated on-chain attribution and background pressure, but model-authored prose derived unsupported switch-overhead and mislabeled target-thread running share as CPU utilization. Keep this on structured unit/caliber guidance and heterogeneous replay; do not scan or rewrite model prose.
- No empty answer, malformed JSON salvage, stale-draft fallback, Mermaid parser failure, active-stream fixed-age degradation, system-authored diagram, or system-authored conclusion occurred.
