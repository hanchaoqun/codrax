# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T16:46:58Z
- sweep_start_ts: 20260812-094657
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260812-094659 | answer_regex,answer_contains | none | 141s | 25 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B659 is production-positive: the finalizer receives 12 exact typed directed relations, emits a legal Mermaid graph with every production implementer pointing to `LoopController`, and retains the 12-file table. No finalizer rejection or patch; three test-only implementers remain outside the principal roster. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-094659 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 150s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B658 is production-positive: prose keeps 11 scheduler wait intervals / 36.757ms separate from 12 blocked_reason records / Σ39.157ms. However the model still treats a capped display as three missing physical occurrences and estimates ≈9.8ms despite the complete 11-row target roster. It also labels both 65.912ms and `74.915−65.912=9.003ms` as the compute-supply deficit. These expose two typed role gaps, B660/B661. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusions

### B659 production closure

The type-relation replay closes B659 in production. Finalizer diagnostics now report
`explicit_typed_directed_relations=12`; the model authored all 12 visible
`implementer -->|implements| LoopController` edges in canonical direction on the first
attempt. The graph is valid Mermaid, the implementation/file table is complete for the
production scope, and there was no emit rejection, retry patch, system-authored diagram,
or prose hard gate.

### B658 production closure and two new generalized gaps

B658 also closes its intended production claim: the model separately reports the exact
11 target D-state intervals totaling 36.757ms and the independent kernel census of 12
records totaling 39.157ms, explicitly saying the rulers cannot be exchanged or merged.
The prior 8/4/4 reconstruction is gone.

The same answer nevertheless contains two new contradictions:

1. The final prompt's principal recap contains all 11 occurrence rows, but a generic
   compacted-view disclosure later says some candidate views are incomplete. The model
   converts that display cap into “three trace occurrences were not listed” and estimates
   their remainder as about 9.8ms. Candidate display completeness and physical target
   occurrence completeness lacked an explicit typed priority relation.
2. The typed projection carries 74.915ms measured running and 65.912ms effective
   attribution. It calls 65.912ms the supply-fold deficit, but the final decision tail did
   not carry the already-existing `ideal + deficit = folded running` equation. The model
   therefore also labels `74.915−65.912=9.003ms` as a supply deficit. The two labels cannot
   both be true; in this fold 9.003ms is ideal-equivalent running and 65.912ms is the
   engine-published low-frequency supply deficit.

B660 adds a final typed relation only when one deterministic result provides an exact,
complete, same-result target-wait roster. It states that root-cause/blocking/display caps
do not downgrade this physical roster and forbids missing-occurrence or residual-duration
inference. B661 publishes the existing supply-fold variables and exact equation, plus the
typed relationship between measured occupancy, effective attribution, and the deficit.
Inconsistent folds fail closed. Both changes are prompt-only typed facts: they do not
scan the user's request or model/final prose, reject an answer, or rewrite a conclusion.

Targeted production wiring pins and the core `agent/types/tool/orchestrator` suites pass.
B660/B661 need a later exact-two production replay; B659 and B658 are now production
positive.
