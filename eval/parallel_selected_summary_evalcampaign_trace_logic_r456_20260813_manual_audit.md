# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T21:01:27Z
- sweep_start_ts: 20260813-140126
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-140127 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 241s | 39 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B739B/B741/B742 production positive: causal scope, non-scalar compound answer, exact user window, 157.248+5.604+70.338+0=233.190ms authority, root query, system supplement and full Trace causal projection all present. Failure is a conclusion-composition contradiction: the answer correctly states policy-ceiling presence does not prove target binding, then concludes the target frequency was constrained. B743 narrows soft typed guidance so a target-specific yes/no verdict is governed by target_binding_status. No answer scan/rewrite or hard gate. Initial analyzer retry also exposes B746: runtime-artifact event names were incorrectly checked as repository symbols by subtopic coherence. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-140127 | answer_regex,answer_contains,mermaid_edge_count | none | 701s | 43 | read=31,repo_map=3,list=0,trace=0,source_lens=0 | midloop=20,inv=8/0,fin_reject=2,unavail=0,prune=0 | fail | Runner counted a syntactically valid graph, but the requested BusContext/Mutable data-flow relations disappeared. Code and prose prove dispatchStage passes o.busCtx to BuildAgentContext and output is merged to Mutable, while the typed relation vocabulary models only caller->callee and assignments, not call-argument/carrier flow. Completion therefore churned through 60 explorer iterations/31 reads, then finalizer rejected the real carrier arrows and forced both participants into disconnected unproven boundaries. B744 is a generalized typed call-argument/carrier relation gap, not model fluctuation; it must be fixed without synthetic edges or prose-keyword gates. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
