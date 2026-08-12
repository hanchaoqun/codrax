# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T07:03:46Z
- sweep_start_ts: 20260812-000345
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260812-000347 | write_apply,write_patch_oracle | none | 174s | 23 | read=4,repo_map=3,list=0,trace=0,source_lens=1 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-file production patch normalizes whole-number float years/months with `is_integer()` + `int`, rejects fractional floats with `ValueError`, and leaves the supplied tests byte-unchanged. Verification report records the real `python3 -m unittest discover -v` command, four named suite cases plus one behavior probe, exit 0, and changed-path target-behavior coverage. No replan, answer rewrite, or static-only signature. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-000346 | answer_regex,answer_contains,mermaid_edge_count | none | 346s | 35 | read=8,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | Machine oracles only prove names/one edge. Final graph leaves `BusContext` and `Mutable` as disconnected decoration, so it does not answer the requested stage-to-state data flow. Four finalizer rejects expose a precise cross-lane bug: a technical edge such as `Orchestrator.runReadSchedulerLoop -> o.busCtx.Mutable.SetResult` plus a separate visible `BusContext` node was accepted as a connected BusContext. Typed relation authority and business display identity were true independently but not on the same visible endpoint/group. The final graph also exposes internal operation names and `precedence`/`call` labels instead of a concise business-facing component view. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: `2/2`; human: `1/2`.
- The Python write lane is a positive control for actual language execution and honest changed-path coverage.
- `B622-DIAGRAMPARTICIPANTINCIDENTJOIN1/P1-high` is deterministic, not model variance. Participant incidence must bind to the same visible endpoint: an exact structured participant endpoint may keep a business-language label, while an owner/static-binding bridge must show the participant on that endpoint or place the technical endpoint inside its visible Mermaid group. A disconnected business node cannot borrow incidence from an edge elsewhere in the document.
- The fix remains a structural validator correction over typed diagram JSON/evidence. It does not scan user/model/final prose, mint an edge, rewrite the model diagram, or affect explicit-window Trace causal projection and auto-supplement.
