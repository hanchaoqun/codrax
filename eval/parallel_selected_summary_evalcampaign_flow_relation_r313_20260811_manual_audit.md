# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T13:29:08Z
- sweep_start_ts: 20260811-062907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260811-062909 | answer_regex,answer_contains | none | 175s | 22 | read=6,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | B530 production-positive. Explorer's first evidence batch now included `dispatchOnce -> fetch` and `send -> nextDelay`; completion then used the exact already-read AST handoff to name the still-missing same-statement sibling `send -> sleep` at transport.ts:29. The model re-emitted that edge and the final diagram preserved run -> fetchUser -> send -> dispatchOnce -> fetch plus the retry nextDelay/sleep branch and sleep -> setTimeout. No system-authored edge or answer appeared. One member-decoration form repair remains low-value churn, not a relation gap. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-062909 | answer_regex,answer_contains,mermaid_edge_count | none | 186s | 33 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | fail | B531 confirmed. Analyzer correctly emitted separate incident_required participants for Mutable and BusContext, matching the explicit request for data flow among the four stages and those carriers. `reconcileDiagramParticipantsWithRequestedRelationSurface` then silently deleted both because a model-authored stage/workflow dimension quote named only the stages while another required responsibility dimension existed. Explorer therefore had no participant relation obligation and never reached B529c; Finalizer's first data-flow edges were rejected, and its patch left Mutable/BusContext disconnected while prose still claimed that all stages exchange data through them. This is deterministic system IR mutation on a noisy cross-surface inference, not model variance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
