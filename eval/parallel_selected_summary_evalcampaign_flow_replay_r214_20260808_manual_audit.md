# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T13:55:27Z
- sweep_start_ts: 20260808-065526
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-065528 | answer_regex,answer_contains | none | 215s | 36 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Typed flow ran, but 16,543 findings flooded handoff; finalizer deleted all typed edge ownership while preserving the same visible arrows, then asserted unsupported universal MutableState transfer and cited unrelated helper lines. |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-065528 | answer_regex,answer_contains | none | 366s | 23 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Stage roster/order is useful, but the system required pairwise precedence evidence that the model had no typed authoring carrier for; metadata deletion preserved the same arrows. Several component products/slots and the “longest stage” claim remain unsupported. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed human audit

### qf_logic_view_read_pipeline

- The new typed `predicate_axis=flow` took the intended deterministic path: `dataflow.Analyze` consumed 6,888 evidence rows and produced 19,889 findings; relevance filtering still handed 16,543 paths to the finalizer. That proves production wiring, but the volume crowded out the small set of producer/transfer/consumer facts needed for the answer.
- The first answer draft attached typed call/precedence/contain/guard/observe/return/assignment owners to the diagram. The validator correctly rejected unsupported owners. The repair then removed every `edge_anchors` entry while retaining the same visible arrow topology, explicitly relying on the presentation-only escape. For a typed flow request this changes metadata, not the factual claim, so runner PASS is not correctness.
- The final prose says all intermediate results and evidence are written to `MutableState`; the cited roster proves fields and containers, not universal transfer through one merge path. `internal/agent/explorer.go:343` is also not evidence for the claimed snapshot write. The answer therefore repeats the membership→transfer error from r212 despite the axis fix.
- One finalizer reject was justified and finite. The gap is evidence ownership and handoff scale, not malformed JSON or excessive retry.

### qf_diagram_pipeline

- Typed flow generated 81 findings, but most were repository-wide paths unrelated to the six requested stages. The stage/member roster and conditional pre-stage distinction are useful; the retained flow slate did not provide a compact pairwise stage-order authority.
- The model correctly chose `relation_kind=precedence` for visible stage order, but the only authored row was a definition whose summary narrated the ordered list. The validator correctly refuses to read summary prose as relation authority; however, `emit_evidence` exposed no generic source-order/precedence anchor, making its own requested repair unrepresentable.
- The repair deleted all precedence metadata while preserving the same arrows. The answer also introduces weakly grounded surfaces such as `ExploreDispatch slot`, `ExtractIR`, and “Explore is the longest stage”. These are not supported merely by enum order or stage binding.
- The emitted “阶段绑定核对” supplement is explicitly labeled as typed source replay and says it does not replace the model answer. It did not delete or rewrite the model conclusion and is not the failure here.

## Filed gaps and disposition

- `EVAL-B356-FLOWEDGEOWNERSHIP1=P1/HIGH`: a typed flow answer cannot turn the same factual arrows into presentation-only content by deleting relation metadata.
- `EVAL-B357-FLOWHANDOFFCOMPACTION1=P1/HIGH`: repository-scale deterministic flow findings need a bounded, diversity-preserving handoff with explicit emitted/total/complete coverage.
- `EVAL-B358-FLOWPRECEDENCECARRIER1=P0/REDLINE`: a hard precedence contract existed without a typed, groundable authoring carrier. Add a cross-language bounded source-order carrier; never infer it from summaries or answer prose.
- `EVAL-B355-INTRAPROCSEQUENCEAUTH1=P1` remains open and is the next independent architecture batch after B356–B358 close.
