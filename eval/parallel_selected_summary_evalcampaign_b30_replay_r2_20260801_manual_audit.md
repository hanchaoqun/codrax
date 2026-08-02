# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T00:55:25Z
- sweep_start_ts: 20260801-175523
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260801-175525 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 31 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Model principal is useful and intact: explicit window, two-axis actual occupation vs rule-eliminable impact, rank, wakeup-chain projection, and supplement all remain. System no longer publishes a prose-derived “正文首因” verdict. However the metric snapshot publishes the same state account twice as 19/20 and 20/21, so the shipped document is internally contradictory. |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260801-175525 | answer_regex,answer_contains | none | 163s | 33 | read=5,repo_map=6,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | LANE2 is covered: no write-only stage is attached to the read lane, and the four canonical read stages, sequence diagram, and table are present. The answer nevertheless invents a simple `Run -> AllMainStages -> dispatch/apply` loop and says `applyStageOutput` stores `FinalAnswer`; production uses analyzer first plus analyzer-emitted TaskGraph scheduling, and the read scheduler consumes `FinalAnswer` directly. Runner's exact-token miss `Mutable` is secondary and by itself is not a production hard-gate justification. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings and generalized disposition

### Trace: diagnostic quotation was incorrectly granted metric-publisher authority

The canonical `state_churn` row already carried the exact `state_account_key` and
published 19 switches / 20 fragments. The second 20 / 21 row came from
`wakeup_chain.root_evidence` with typed predicate `trace_gap` and
`gap_kind=no_eligible_wait`: its diagnostic summary quoted a complete scheduler
state account to explain why chain traversal stopped. The snapshot collector
accepted any deterministic observation containing all scalar tokens, so it
mistook quoted diagnostic context for a second measurement.

Disposition: fix the shared publication authority, not the customer case and not
the scalar values. A state snapshot may be published only by the closed typed
predicate family `state_churn`, `state_drilldown`, wakeup-causal rows, and
root-cause rows. Diagnostic/unknown predicates fail closed even when their
summary contains a complete account. No request text, answer prose, thread name,
case ID, or scalar-equality heuristic participates. Regression coverage keeps
model-query plus exact system supplement single-seated while allowing genuine
independent occurrences without a key to remain separate.

### Read: lane membership is fixed; unsupported control-flow inference remains

The new read-lane guidance prevented the earlier `StageWriteAnalyze` pollution,
so `EVAL-B30-LANE2` is covered by live replay. The remaining false sequence was
supported in part by a stale source comment on `StageOutput.FinalAnswer` claiming
that `applyStageOutput` copies it to `BusContext.FinalAnswer`; production
`applyStageOutput` explicitly does not do so. Correct the comment at source so
future exploration does not receive contradictory evidence.

The broader claim that `Run` loops over `AllMainStages` is still an unsupported
model inference: read production runs analyze first, then executes the
analyzer-emitted TaskGraph through `runTaskGraph -> runReadSchedulerLoop`.
Existing prompt authority already says membership/order is not a call edge.
Keep this as a cross-case evidence-quality watch item rather than adding a
persist-time answer rewrite or a free-prose hard gate after one replay.

### Eval-oracle note

The read runner requires every exact token in the illustrative phrase
`AnalysisIR EvidenceItems AnswerDocument Mutable BusContext`. The final table
included `BusContext` and the requested state-carrier dimension but omitted the
literal word `Mutable`. Because the user wrote “例如”, this exact-token miss is
useful coverage pressure but is not sufficient evidence for a system answer
mutation. Re-evaluate only if omission of the mutable-state role recurs with a
substantive loss of explanation.
