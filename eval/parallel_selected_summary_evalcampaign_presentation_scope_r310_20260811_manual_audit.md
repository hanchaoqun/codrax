# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T12:19:08Z
- sweep_start_ts: 20260811-051907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-051908 | answer_regex,answer_contains,mermaid_edge_count | none | 207s | 37 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 用户明确要求四阶段与 Mutable/BusContext 的数据流；最终图只有 stage precedence 和一条内部 ResetPrescanSummary 调用，Mutable/BusContext 均为断开节点并标“关系未证”。Explorer 因 participant 硬门追加 4 轮补证，但把 `Mutable: ctx.Mutable` 等真实赋值/投影行发成定义/观察，未形成 assignment/data_flow typed row。runner 只验 token/边数而误绿；立 B529 关系证据规划 gap。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-051908 | answer_regex,answer_contains | none | 248s | 34 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | `Mutable` 已恢复且表格完整，但 Analyzer 本轮只留下 BusContext 一个 table-only incident participant，首版 B527 的“至少两个 relation participant”对账未触发。终图把 `ctxbuilder.BuildAgentContext` 的 typed call 画到显示为 BusContext 的节点，同时又声明 BusContext 关系未证，身份/词面自相矛盾；2 次 Finalizer reject 仍由无关 participant 放大。B527 需覆盖零/一 in-scope participant 的 typed 多展示面形。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
