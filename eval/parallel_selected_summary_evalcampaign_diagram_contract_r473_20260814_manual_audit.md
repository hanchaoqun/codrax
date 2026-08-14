# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T07:39:39Z
- sweep_start_ts: 20260814-003937
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260814-003939 | answer_regex,answer_contains,mermaid_edge_count | none | 128s | 27 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | The model authored one valid Mermaid flowchart containing the exact four-stage precedence chain and a concise responsibility explanation. The first structured answer was accepted; no repair, stale-draft fallback, empty-answer recovery, or active-stream age degradation occurred. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260814-003939 | answer_regex,answer_contains,mermaid_edge_count | none | 309s | 35 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | Analyzer retained all six requested participants, closing r472's empty-roster loss. Finalizer correctly rejected invented Orchestrator/stage-to-carrier edges and eventually emitted only proved relations plus disconnected `BusContext`/`Mutable` boundaries. The accepted principal diagram is honest but is still dominated by a disconnected local implementation fragment (`append`, `ToolResults`, `RepoFacts`) and internal source/line wording, while prose describes a fuller stage-to-carrier flow than the graph proves. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Process and answer audit

### qf_diagram_pipeline

- Analyzer emitted the four required stage participants and the requested Mermaid carrier. Explorer found the exact stage binding/order authority without broad source churn.
- Finalizer's first `emit_answer_document` was accepted. The rendered graph has exactly three ordered precedence edges: analyze → explore → extract → finalize. Mermaid syntax rendered successfully.
- The answer is model-authored. The system neither generated nor replaced a node, edge, label, diagram, explanation, or conclusion. No JSON salvage or previous-draft fallback was used.

### qf_logic_view_read_pipeline

- B772 received a production-negative replay: this run's Analyzer already emitted analyzer, explorer, extractor, finalizer, BusContext, and Mutable as typed incident-required participants, so the contradictory-empty-roster gate did not fire. The result nevertheless confirms it has no false positive on a correct slate.
- Explorer completion initially refused to close because BusContext had no citable incident operation. A bounded follow-up then found local `applyStageOutput`/`append` operations touching `ToolResults` and `RepoFacts`. Completion accepted those local incidents even though it still had no typed bridge proving the requested stage-to-carrier flow.
- Finalizer received the stricter truth: two verified relation components, no proved inter-component bridge, and an unproven requested relation spine. It rejected the first two diagrams because they invented assignment/call edges or collided endpoint direction/identity. The third model-authored patch was accepted with the stage precedence component, the separate local append component, and disconnected BusContext/Mutable nodes marked unproven.
- This is evidence-contract granularity drift, not a license to relax relation validation. Explorer's completion notion of “participant covered” is satisfied by any local incident operation, while the final relation authority correctly distinguishes local incidence from the requested cross-participant graph. The mismatch causes avoidable exploration stop, technical-fragment clutter, two form retries, and prose/diagram emphasis drift.
- Register `B773-UNPROVENSPINEDISPLAY1/P1`: when typed authority already says the requested relation spine is unproven, add soft selection/language guidance so the principal visual prioritizes requested participants and the proved request-scoped subset; keep unrelated disconnected local implementation operations in prose or a separate bounded support visual. Preserve uncovered requested participants as visible disconnected nodes with model-authored unproven boundaries.
- B773 must not generate or rewrite diagrams, labels, edges, prose, or conclusions; must not scan raw request/model/final prose; and must not turn soft display advice into a hard validator. It applies across supported languages and diagram families. Trace explicit-window causal projection, auto-supplement, on-chain-only root-cause policy, and active-stream delivery remain untouched.

## Verdict

- Runner: 2/2 PASS. Human: one pass, one partial.
- B771's exact label-collision repair tuple was not production-triggered in this batch; its typed identity parity remains implemented.
- B772 is implemented and safely bypassed a correct participant slate; a deterministic empty-slate production trigger is still pending because the model naturally emitted the correct shape here.
- B773 is the next high-ROI display/context fix. A deeper completion-layer bridge-evidence pass remains separate and must only be designed after a shared typed request-spine authority exists; repeatedly searching merely because local incidence is insufficient would risk non-convergence.
