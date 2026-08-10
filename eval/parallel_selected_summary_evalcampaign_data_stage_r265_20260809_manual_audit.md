# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T05:30:26Z
- sweep_start_ts: 20260809-223025
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260809-223026 | answer_regex,answer_contains,mermaid_edge_count | none | 343s | 40 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=4/1,fin_reject=2,unavail=0,prune=1 | fail | 两次关系证据拒绝后，最终图只剩 `runAnalyzePhase -> dispatchStage -> BuildAgentContext` 两条辅助调用边；Analyzer/Explorer/Extractor/Finalizer/Mutable/BusContext 均成为无连线节点。正文仍把 TaskGraph/EvidencePlan/HypothesisSet/QualityGate 写成 Analyzer LLM 直接产物。runner 只要求任意 Mermaid edge，继续假绿。 |
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260809-223026 | log_regex,answer_regex | none | 502s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终单行 `17,0,5`、4 份材料、14 条 rule coverage、9 条 decision、4 条 contribution、reconcile=pass 与 terminal audit 均正确。代价是 12 个 data batch、5 次 repair：依赖拆分后的 prefix 仍保留 terminal intent，中间 join/filter 结果反复被“contributions 为空”按终态校验送修；另有 filter inner-schema 与 join alias 冲突。B454 的 generated-status typed carrier 本轮未触发，不能据此关单。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `EVAL-B454-DATAREPAIRFACTCARRIER1`：实现保持全测绿，但 r265 没有进入 generated `*_status` failure；状态仍为 pending targeted production witness，不能用最终正确答案替代分支证据。
- `EVAL-B455-DATADEFERREDPREFIXTERMINAL1`（P1）：依赖 rank 拆分/initial-prefix fallback 已保存 deferred suffix，却沿用模型原计划的 `ContinueAfter=false`。`shouldValidateDataTaskWorkflowResult` 因此把 join/filter prefix 当终态，用完整 contribution/reconcile 合同拒绝中间结果，触发无意义 repair。最优修复是由 typed deferred-queue presence 决定 prefix 为 intermediate；终态合同和最终 answer gate 不放松。
- `EVAL-B452-STAGEDIAGRAMAUTH1` / `EVAL-B453-ANALYZERPROVENANCE1`：第三次 production 复现。应把 incident-required relation 缺口前移到 Explorer typed completion，并给模型精确的 model-vs-deterministic producer provenance；不放宽 Finalizer 关系门、不由系统画图或改写正文。
- 本轮没有 raw request/model prose hard gate，没有系统代写模型结论；Trace 显式窗、自动补齐、链上根因、双轴占时/可消除量及因果投影未进入改动面。
