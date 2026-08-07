# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T04:03:43Z
- sweep_start_ts: 20260806-210341
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-210343 | answer_regex,answer_contains | none | 160s | 21 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass | Correct final class, import-time decorator registration, `run_pipeline -> resolve`, and executor callback handoff. Three optional-diagram/list relation retries end by removing unsupported edges; no selector/value substitution or malformed JSON. The typed recipe was present but not copied. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-210343 | answer_regex,answer_contains | none | 352s | 25 | read=7,repo_map=3,list=1,trace=0,source_lens=0 | midloop=8,inv=8/0,fin_reject=2,unavail=0,prune=0 | fail | S36r works: two invalid story diagrams are rejected and the model removes the optional diagram; no `precedence/guard` relabel escape survives. Final prose nevertheless says `std::fputs(..., stdout)` while the normalized citation/source is `stderr`, and asserts an unseen transfer into Logger. Explorer also spends 20 turns repairing relation-shaped member/support-ref alignment. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `EVAL-B227-RELABELAUTH1` is closed by production replay: the invalid virtual-dispatch edge cannot escape by changing its enum; the model eventually removes the optional graph.
- New P1 `EVAL-B228-CALLLINECTX1`: finalizer Primary Evidence keeps typed caller/callee but drops the grounded call source line. Exact argument facts such as `stderr` disappear, so the model can guess `stdout` even though the source and final citation disagree. Add a bounded grounded `source_line` to call-site evidence context; this is guidance, not a prose hard gate.
- New P1 `EVAL-B229-COPYGRAPH1`: both cases receive exact node aliases and anchor JSON but still rebuild a user-story diagram. Publish the same typed rows as a copy-ready optional flowchart body plus anchor array, explicitly preserving disconnected components and omitting unsupported bridges. The model remains free to omit the diagram.
- New P1 `EVAL-B230-MEMBERMULTIREF1`: relation-shaped member rows may need two source locations, while positional support_refs allow exactly one per member. The C++ explorer eventually solved this by splitting guard and return into six members; keep this open for a carrier-level design rather than broadening refs ad hoc in the diagram batch.
- No raw request/model/final prose scan is proposed. Trace causal projection and auto-supplement remain outside all three source-code answer gaps.

## S36s implementation closure

- `EVAL-B228-CALLLINECTX1` implemented: Primary Evidence now carries a bounded, single-line `source_line` only for Tier-1 grounded call/callback evidence. Recovered candidates are explicitly excluded, so an uncertain nearby snippet cannot acquire exact-fact authority. This preserves arguments such as `stderr`, flags, selectors, and payload fields through explorer → finalizer without inspecting or rewriting the answer.
- `EVAL-B229-COPYGRAPH1` implemented: the authoring capsule compiles its existing citable typed recipes into one optional Mermaid skeleton and one complete `edge_anchors_json` array from the same rows. It follows the typed presentation family (`sequenceDiagram` for sequence; `flowchart TD` for flow/call_dag/architecture), preserves disconnected components, and never synthesizes an actor/story bridge. If no diagram contract or typed hint exists, no skeleton is injected.
- JSON teaching remains single-source: the per-edge examples and full array both marshal `types.DiagramEdgeAnchor`; there is no second handwritten schema or repair parser. The model may copy both artifacts or omit the optional graph; the system does not author or replace visible answer conclusions.
- Regression closure retained for the two explicitly tracked neighboring gaps: sequence display-message arguments cannot alter endpoint identity, and labelled or unlabelled logical arrows cannot replace missing typed relation evidence.
- Verification: focused agent tests, endpoint/relabel hard-gate tests, `git diff --check`, and `go test ./...` all pass. Production acceptance remains r147 with exactly the same two cases in parallel: inspect `source_line` presence, graph reuse/retry count, C++ `stderr`, Python selector/value roles, and final human correctness.
