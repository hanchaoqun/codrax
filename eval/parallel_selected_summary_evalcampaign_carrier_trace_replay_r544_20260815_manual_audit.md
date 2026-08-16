# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T02:24:04Z
- sweep_start_ts: 20260815-192402
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-192405 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 343s | 48 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | B870 typed context was delivered, but the answer still promoted every unresolved pair to “independent”, collapsed two unproven same-direction seats into the 12.115ms subtotal, and claimed no cross-direction overlap/competition without evidence. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-192405 | answer_regex,answer_contains,mermaid_edge_count | none | 402s | 41 | read=16,repo_map=2,list=0,trace=0,source_lens=1 | midloop=12,inv=6/0,fin_reject=2,unavail=0,prune=0 | fail | Explorer accepted the requested exact Stage assignment rows, then completion replayed the stale initializer repair five times. The loop consumed rounds 7–20; Finalizer later dropped requested BusContext/Mutable participants and emitted only stage precedence edges. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Detailed audit

### qf_logic_view_read_pipeline

1. The first StageBinding batch used grounded `initializer` rows without exact
   `subject/object`. The producer correctly published a typed endpoint repair.
2. A later batch recorded all four exact tuples at lines 47/61/73/85 as
   `assignment_fact` rows and reported `Current actionable repair targets: none`.
   Re-emitting the same rows was then correctly classified as a duplicate.
3. Despite that state, `emit_investigation_complete` replayed the original
   repair at log lines 1643, 1763, 2017, 2146 and 2181. The durable obligation
   retained the old `initializer` anchor while the self-contained repair text
   exposed only `subject/object`; the accepted equivalent assignment-shaped
   tuple could therefore never discharge it. This is a deterministic
   contradictory contract, not model fluctuation.
4. The deadlock starved the requested carrier investigation. The final answer
   omitted both `BusContext` and `MutableState`, called BusContext immutable,
   and retained only three stage-precedence edges. B871a therefore remains
   production-blocked; this replay cannot judge its relation-recovery design.
5. Finalizer rejected two drafts containing broad invented edges. The eventual
   deletion avoided publishing unsupported arrows, but did not satisfy the
   requested participant/flow surface. Do not solve this by system-drawing a
   graph; first repair the Explorer contract and replay with complete evidence.

### real_trace_h11_cross_direction_overlap

1. Explicit window selection, Trace query execution, causal projection,
   on-chain-only ranking, actual occupancy/business clues and rule-priced
   eliminable quantities remained present. No active-stream age degradation or
   system-authored conclusion was observed.
2. B870's typed display plan reached Finalizer and correctly said that only the
   7.405ms and 4.710ms lock seats form the exact 12.115ms subtotal, while other
   same-direction and cross-direction relations remain unresolved.
3. The model nevertheless wrote that the four directions were independent and
   mutually non-overlapping, called the extra keva seats independently
   eliminable, and described all four priority-inversion candidates as totaling
   12.115ms. Absence of a relation witness was converted into proof of
   independence. Human correctness therefore fails despite the runner PASS.
4. Before adding any hard answer gate, audit the complete Finalizer context for
   competing instructions and cognitive duplication. The next design must use
   typed relation decisions and model authorship; it must not scan prose or
   replace the model's conclusion.

## Disposition

- `B872-EVIDENCEVALIDATIONREPAIRSTALE1`: P0, confirmed. Fix first with typed
  obligation equivalence and self-contained repair teaching; unrelated or
  partial repairs must stay fail-closed.
- `B871-CARRIERRELATION1`: keep open and production-blocked by B872; replay after
  the P0 fix before adding another relation mechanism.
- `B873-TRACERELATIONDECISIONCONSUMPTION1`: P1, confirmed production failure.
  First audit contradictory/duplicated context, then improve the model's typed
  decision input without answer mutation or prose-keyword gates.
