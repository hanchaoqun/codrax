# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T03:25:11Z
- sweep_start_ts: 20260810-202509
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260810-202511 | write_plan,write_patch_oracle | none | 66s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B502 production-positive: planner consumed the new single-carrier teaching, emitted one unique change-local probe on its first accepted plan, and produced the exact one-line patch. No duplicate-id repair, replan, or answer-authoring mutation occurred. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260810-202511 | answer_regex,answer_contains,mermaid_edge_count | none | 699s | 43 | read=48,repo_map=4,list=0,trace=0,source_lens=1 | midloop=31,inv=11/0,fin_reject=6,unavail=0,prune=1 | fail | B500 v4 production-positive/closed for its scoped carrier: finalizer StageReport published `flow_findings=0` instead of r289's 57 unrelated rows while retaining `total=14260, complete=false`. The final prose was broadly accurate, but the graph only retained three exact Orchestrator call edges, left all requested high-level components disconnected, and omitted visible `Mutable`; the runner therefore correctly failed `missing:Mutable`. Analyzer emitted `Mutable (pipeline state)` as `context_only` even though the canonical typed teaching says every named carrier whose data-flow connection is requested is `incident_required` and must be copied verbatim. No independent precise typed carrier contradicted that role, so a deterministic hard promotion would require forbidden raw-request/prose interpretation. Record B503 as high-impact model/analyzer variance, not a safe hard-gate patch. B501's layered soft recipe was present but not consumed; it remains production-unclosed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 1/2; human: 1/2.
- `B500-STAGEREPORTSCOPE1` v4 is production-closed on the bypass it was designed to remove: no file/proximity sibling relation survived into the finalizer-facing StageReport, while full upstream totals remained visible.
- `B502-PLANPROBECARRIER1` is production-positive and may be closed after this direct first-attempt witness; the validator remains intact.
- `B501-LAYEREDGRAPH1` is implemented but not production-closed. The source-derived recipe reached the finalizer, yet this model preferred disconnected high-level nodes plus an exact low-level call graph.
- `B503-PARTROLEVAR1` is a model/analyzer contract violation in this single run: the existing SSOT already gives the exact carrier rule, but the analyzer emitted both the wrong role and a decorated identity. There is no second precise typed signal from which runtime code can safely infer the opposite without scanning the request or trusting another noisy model-authored list. Keep as P2/high-impact watch and require heterogeneous recurrence before changing the schema or gate.
- The QF path also remains operationally expensive (63 explorer iterations, 31 mid-loop injections, 11 completion calls, one history prune). That churn needs a separate typed lifecycle audit before any budget increase or early-stop change; it is not evidence that the relation gate should be weakened.
- No Trace code or prompt was changed in this batch. Explicit windows, auto-supplement, causal projection, on-chain-only root-cause authority, cause-family preservation, and background separation remain untouched.
