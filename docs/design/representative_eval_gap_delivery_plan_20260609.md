# Representative Eval Gap Delivery Plan - 2026-06-09

## Objective

Close the representative eval gaps recorded in `docs/design/representative_eval_gap_audit_20260609.md` with commercial-grade, generic system changes. The plan intentionally avoids:

- case-specific constants or output-number patches;
- keyword matching over user intent or model prose;
- prompt-only repairs for runtime invariants;
- changes that affect write-mode stability or read scheduler red lines.

All hard behavior below is driven by typed contracts, typed artifact metadata, ledger state, deterministic validators, and structured terminal/eval records.

## Code Surfaces

| Surface | Files | Responsibility |
|---|---|---|
| Data reference projection | `internal/repl/data_task_workflow.go`, `internal/dataworkflow/fallback_plan.go`, `internal/dataquery/action_runner.go` | Choose the final output reference universe and project reconcile groups into it. |
| Data workflow completion | `internal/dataworkflow/output_projection.go`, `internal/dataworkflow/completion_gate.go`, `internal/repl/data_task_workflow.go` | Decide whether a result is terminal, incomplete, or repairable. |
| Terminal audit | `internal/repl/data_task_cli.go`, `internal/repl/repl.go`, `internal/dataworkflow/journal.go` | Write a single machine-verifiable terminal status and reason. |
| Eval observability | `eval/run.sh`, `eval/convergence_audit.sh`, `eval/runner_lib_test.sh` | Surface data-lane status, rounds, repair signatures, and route logs. |
| Artifact schema stability | `internal/dataquery/action_runner.go`, `internal/dataworkflow/artifact_schema.go`, `internal/dataworkflow/artifact_access_view.go` | Keep generated record-shaped artifacts executable by later typed actions. |
| Answer surface quality | `internal/orchestrator/repair_caveat_materializer.go`, answer-document supplement path under `internal/tool/` | Prevent vague advisory warnings and duplicate system supplements in PASS answers. |
| Rich evidence handoff | `internal/types` aggregate facts/evidence, `internal/tool/answer_document_principal_enum_compile.go`, data artifact lineage | Preserve upstream structured facts, member notes, support refs, evidence summaries, and artifact lineage for backend deterministic consumption. |

## Architectural Decisions

### D1. Explicit Final Reference Path Wins

When `OutputContract.CompleteReference` is true and `ReferencePath` is set, that path is the final output key universe. The key field is resolved structurally within that path: the declared `ReferenceKeyField` participates, and if it is misaligned the resolver may select a better same-path field by typed key overlap. Fallback inference across other paths may only run when the explicit path is absent or unreadable.

Typed behavior:

- A projected answer with mismatched resolved `reference_path` or `reference_key_field` is not complete.
- Non-reference reconcile groups remain valid audit/contribution groups but are excluded from the final projection.
- Missing reference keys are projected as zero/empty values without changing contribution records.
- Complete-reference output contracts have higher specificity than ordinary output format contracts, so later plain output batches cannot drop the final reference authority.

### D2. Reconcile Pass And Final Projection Pass Are Separate

`reconcile=pass` means computed groups are internally consistent. It does not imply that final answer shape, count, reference order, or projection metadata match the output contract.

Typed behavior:

- Completion gate must validate final projection metadata when complete-reference output is required.
- Terminal status cannot be complete while output projection graph is incomplete.

### D3. Terminal Status Has One Source Of Truth

CLI/REPL terminal JSON, terminal log line, and eval verdict must report the same terminal status. A Go function returning without error is not sufficient proof of `complete`.

Typed behavior:

- Terminal status is derived from final answer candidate + workflow state + latest evaluation/guard.
- `repair_node`, `continue_data`, `expand_graph`, `blocked`, `failed`, and `complete` remain distinct.

### D4. Eval Must Show Data-Lane Runtime

Data-route evals do not use analyzer/explorer/finalizer metrics, so the harness must expose data-specific counters instead of showing zero-filled read-pipeline fields only.

Typed behavior:

- Extract terminal JSON status, rounds, repair rounds, record count, result summary, action failure count, and final answer length.
- Keep semantic output failure, terminal-contract failure, and telemetry/log failure as separate reason classes.

### D5. Generated Record Artifacts Stay Record-Executable

Typed actions that output JSON arrays of objects should register stable record aliases and access hints. Later actions should consume those aliases rather than rediscovering shape from model narrative.

Typed behavior:

- Record-shaped generated artifacts carry fields, row count, aliases, and `executable_record_input=true`.
- Mixed-source zero-row extraction diagnostics should point to source narrowing or reference materialization, not repeat the same failing action.

### D6. User-Visible Caveats Must Be Specific

Generic advisory warning text is operator telemetry, not user-facing product output. A visible caveat must name a specific limitation supported by typed state or evidence.

Typed behavior:

- Generic low-confidence/advisory contract warnings stay in logs and summaries.
- Specific hard blockers or verified limitations can render as caveats.
- Supplement tables are rendered only when they repair or complete a missing typed surface.

### D7. Rich Upstream Evidence Must Reach Backend Compilers

Earlier stages often already know the important members, notes, support refs, record lineage, and output-contract metadata. That information must remain in typed handoff state and be consumed by deterministic backend compilers/validators. Final answer quality must not depend on the model repeating the evidence in exactly the shape the compiler prefers.

Typed behavior:

- `aggregate_facts`, `member_notes`, `support_refs`, evidence summaries, artifact lineage, and terminal/output metadata remain the authoritative backend handoff.
- Answer compilers recognize authored prose, table, and list carriers by structured row candidates and location/citation compatibility before adding system supplements.
- Coverage decisions do not parse user intent keywords or model prose intent; model text is used only as the visible authored surface for row/location/description coverage.

### D8. Final Projection Value Binding Is A Typed Completion Requirement

Reference-complete shape metadata is necessary but not sufficient. When a strict values-style answer is verifiable, terminal completion must prove that each reference key's output value is bound to the typed reconcile group value for the selected metric. Prior `final_answer/projection` groups are output lineage and must not participate in business metric inference.

Typed behavior:

- `assemble_answer` infers business metric from reconcile groups after excluding typed final-output projection groups.
- Projection artifacts expose the selected metric as typed metadata for downstream validators.
- Completion gate accepts `reference_projected=true` only when path, key field, count, and verifiable values match typed reconcile groups.
- Deterministic projection fallback carries reference/reconcile/contribution artifact aliases as `input_paths` so upstream evidence remains available to backend actions.

### D9. Stage Topology Authority Is A Typed Handoff

Architecture questions can legitimately enumerate multiple stage namespaces: read-mode pipeline stages, data workflow stages, retry phases, render phases, or test phases. When the analyzer emits a stage-like enumeration, downstream exploration needs authority files that let it distinguish the canonical pipeline topology from a sibling enum family with the same naming shape.

Typed behavior:

- read-mode main stages and conditional pre-stages are exposed through canonical `StageBinding`/topology helpers;
- analyzer required-file candidates merge those authority files as soft disambiguation when a stage-like architecture enumeration is emitted;
- the signal consumes typed scenario/kind/entity shape plus repo symbol/source authority, not user-prose keyword tables or model-authored answer text;
- final answer checks can rely on structured block items, citations, and typed authority anchors rather than rendered prose.

### D10. Explicit Group Fields Preserve Contribution Granularity

`compute_contributions` must distinguish omitted grouping from explicit field grouping. Falling back to `group_key=all` is valid when the action explicitly asks for a constant group or no grouping, but it is invalid when matched target rows lack the caller's `group_key_field`.

Typed behavior:

- explicit `group_key_field(s)` require non-empty values on matched target rows;
- missing explicit group values raise a typed dependency/field contract and carry join/enrich/materialization repair hints;
- diagnostics preserve the matched-row and missing-group counts so backend repair can consume the evidence;
- grouped contributions from enriched/joined records continue to work without changing stable scalar or constant-group scenarios.

## Delivery Batches

### Batch A - Final Reference Authority

Tasks:

1. Add tests for explicit reference contract priority.
   - Construct reconcile groups with extra non-reference groups.
   - Set `OutputContract{CompleteReference:true, ReferencePath:<target>, ReferenceKeyField:<field>}`.
   - Assert final projection uses target keys only.

2. Harden `dataTaskOutputReferenceProjectionGap`.
   - Prefer `ReferenceKeyCandidateForPath(contract.ReferencePath, contract.ReferenceKeyField, ...)`.
   - Treat mismatched projection metadata as incomplete.
   - Do not let broader assemble-action/fallback candidates override explicit contract.

3. Harden `BuildRequiredOutputProjectionPlan`.
   - When `ReferenceGap` is present, emit `assemble_answer` params from the candidate.
   - Preserve delimiter and strict output format.
   - Keep projection typed and deterministic.

4. Add regression for stale/wrong projection metadata.
   - A prior `assemble_answer` artifact using a broad reference path must not satisfy the target contract.

5. Bind final projection values to reconcile groups.
   - Exclude final-output projection groups from business metric inference.
   - Reject value-mismatched strict values projections even when reference metadata matches.
   - Carry reference/reconcile/contribution aliases in fallback `assemble_answer.input_paths`.

Validation:

```bash
go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -run 'Reference|Projection|Assemble|Completion'
```

### Batch B - Terminal Status And Eval Observability

Tasks:

1. Add a terminal status resolver for data CLI/REPL.
   - Input: final result, current plan, records, workflow state, error.
   - Output: one typed terminal status/reason.
   - No model prose parsing.

2. Use the resolver in `RunDataTaskCLI` defer and REPL terminal logging.
   - Terminal JSON and log line use the same status.
   - If final result is not a valid final answer candidate, do not log `complete`.

3. Add structured data route log.
   - Single-shot data path emits a stable `[cmd/route] ... route=data ...` control line.
   - This already exists when classifier route policy runs; make explicit-route/data fast path equally observable.

4. Extend eval metrics.
   - Parse terminal JSON and data logs.
   - Add `data_rounds`, `data_repair_rounds`, `data_terminal_status`, `data_action_failed`, `data_answer_len`.
   - Split reason labels: `data_semantic`, `data_terminal`, `data_log`.

Validation:

```bash
bash eval/runner_lib_test.sh
go test ./internal/repl -run 'DataTask|Terminal|TurnPolicy'
```

### Batch C - Artifact Schema Stability

Tasks:

1. Audit generated artifact registration in `ActionRunner`.
   - Ensure `enrich_records`, `join_records`, `derive_fields`, `extract_fields`, `filter_records` outputs expose record metadata when output is array-of-object records.

2. Add record alias tests.
   - `enrich_records` output consumed by `derive_fields`.
   - `join_records` output consumed by `filter_records` and `compute_contributions`.

3. Improve zero-row diagnostics.
   - When required fields are missing due to mixed source rows, diagnostics should carry field missing counts and candidate source artifacts.
   - Deterministic fallback should prefer source-narrowing or reference join based on artifact schema.

Validation:

```bash
go test ./internal/dataquery ./internal/dataworkflow -run 'Artifact|Record|Schema|Extract|Enrich|Join|Contribution'
```

### Batch D - Answer Surface Quality

Tasks:

1. Gate generic repair caveats.
   - Render generic warning text only for hard/visible blockers.
   - Keep advisory CGEC/contract warning details in logs.

2. Gate system supplement tables.
   - Suppress duplicates when model-authored blocks already cover all typed rows with citations.
   - Keep supplements when they add missing principal rows or repair incomplete structured surface.

3. Add answer surface regression tests.
   - Runtime/current-source PASS answer has no vague generic warning.
   - Multi-repo bucket answer does not duplicate system supplement when authored sections cover required members.

4. Preserve rich evidence handoff in backend consumers.
   - Confirm answer-document compilers consume typed member rows, support refs, member notes, and evidence summaries.
   - Confirm generated data artifacts carry lineage/record-executable metadata into downstream typed actions and eval telemetry.
   - Do not introduce prompt-only instructions, keyword intent matching, or hard gates over model prose.

Validation:

```bash
go test ./internal/orchestrator ./internal/tool -run 'Caveat|Supplement|Principal|AnswerDocument'
```

### Batch E - Topology And Group-Field Contracts

Tasks:

1. Expose read-mode pipeline authority from code.
   - Add helper(s) around canonical read-mode main stages, conditional pre-stages, and authority files.
   - Keep orchestrator and analyzer consumers reading the same source of truth.

2. Add analyzer required-file disambiguation for stage-like architecture enumerations.
   - Use typed scenario/kind/entity structure and repo symbols.
   - Merge authority files softly; do not replace model-selected files or hard-code final answers.

3. Add topology regression tests.
   - A stage-like architecture enumeration that initially names a sibling stage enum still handoffs canonical pipeline authority.
   - Legitimate sibling stage namespaces remain answerable because the authority files are disambiguation evidence, not a hard answer override.

4. Enforce explicit group-field non-empty contracts.
   - `compute_contributions` must not emit `group_key=all` for rows missing an explicit group field.
   - Return typed dependency/field violation with repair hints for join/enrich/materialization.

5. Add contribution regression tests.
   - Mixed-source extract records with group field present only on reference rows fail before emitting `group_key=all`.
   - Enriched/joined records with non-empty group field emit grouped contributions normally.

Validation:

```bash
go test ./internal/agent ./internal/types ./internal/dataquery ./internal/dataworkflow ./internal/repl -run 'RequiredFiles|Stage|Topology|Contribution|Group|Field|Projection'
```

### Batch F - Full Verification

Tasks:

1. Run focused unit tests after each batch.
2. Build current binary with `make`.
3. Re-run representative eval with `PARALLEL=2`.
4. Run a data-focused eval slice covering complete reference, extra contribution groups, unmapped rows, numeric parsing, and multi-file joins.
5. Update audit/design docs with final status.

Representative eval command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_delivery_summary.md \
bash eval/convergence_audit.sh
```

Commercial acceptance criteria:

- `data_multifile_reference_projection` outputs the reference-projected three-item answer.
- Data terminal JSON status and log status agree.
- Eval summary exposes data-lane counters.
- Trace PASS answer has no vague generic warning section.
- Multi-repo answer remains bucketed and excludes unrelated repo content.
- No implementation depends on case-specific values, prompt-only policy, or keyword matching of model prose.

## Current Delivery Delta - 2026-06-09 11:30 CST

The current representative rerun shows all four selected cases passing after commits through `22709693`. The remaining work is no longer about the original semantic failures; it is about commercial-grade robustness, operator clarity, and typed handoff quality. These tasks extend the same system-level principles and must not add case-specific constants, user-intent keyword matching, or model-output prose matching.

### Delta D11. Complete Terminal Status Must Not Carry Stale Final Errors

Typed behavior:

- `status=complete` means the final state has no actionable final error.
- Prior errors remain auditable through bounded lineage metadata.
- Terminal JSON and CLI logs use the same typed resolver.
- Eval tooling should read final-state errors and prior repair lineage separately.

### Delta D12. Data Action Plans Need Structural Pre-Admission

Typed behavior:

- Batch dependency rank boundaries are validated before executing any action.
- Action parameters are checked against artifact schemas before execution.
- Missing fields and dependent-rank crossings emit typed repair hints.
- Execution history should not accumulate avoidable failed actions that were structurally invalid before runtime.

### Delta D13. Accepted Evidence IDs Are Backend Handoff Anchors

Typed behavior:

- Extractor verdict tools receive the accepted evidence-ID ledger as a first-class grounding option.
- Validators resolve accepted IDs to citations deterministically.
- Unknown IDs and unsupported citations still fail loud.
- The implementation consumes typed evidence state, not model-authored explanation text.

### Delta D14. Answer Supplements Are Rendered Only When Typed Coverage Needs Them

Typed behavior:

- Authored blocks, tables, and item rows are normalized into typed carrier coverage.
- Member/citation/location compatibility decides whether principal rows are already covered.
- Supplements render only when they add missing typed rows or repair an incomplete structured surface.
- Visible answer quality is improved without parsing user intent keywords or free-form model rationale.

### Delta D15. Multi-Repo Active Scope Must Survive Tool Handoff

Typed behavior:

- Focus-selector output becomes typed active-subrepo scope available to analyzer and explorer tool contexts.
- Explicit subrepo inventory resolves against exact `root_rel`.
- Parent/primary compatibility fallback is not used for explicit active subrepos that exist.
- Inactive subrepos remain excluded unless the request or focus policy includes them.

### Delta D16. Data Artifact Aliases Should Be Canonicalized For Backend Actions

Typed behavior:

- Action params carry canonical artifact paths.
- Alias variants remain in lineage metadata for debug/audit.
- Backend actions consume canonical aliases plus lineage rather than very long duplicate `input_paths`.

### Delta D17. Rule-Bearing Materials Must Become Typed Rule Coverage Requirements

Typed behavior:

- Required text/constraint materials that participate in strict aggregation workflows upgrade the workflow contract to `rule_coverage_required=true`.
- The upgrade is driven by coverage-material usage mode, material path classification, output-contract strictness, and required validation ledgers.
- `derive_rules` remains the existing typed producer for source-backed rule coverage; the stage reducer and deterministic fallback decide when to schedule it.
- Join/compute/reconcile completion cannot substitute for rule materialization when rule-bearing materials affect inclusion, exclusion, qualification, mapping, or validation.
- Hard gates do not parse action `purpose`, `success_criteria`, user prose keywords, or model-authored output text.

## Current Delta Delivery Batches

### Batch G - Terminal Error Lineage

Tasks:

1. Add terminal error lineage fields and resolver helpers.
   - Complete terminal: empty final `last_error` unless final-state degraded.
   - Non-complete terminal: preserve actionable final error.
   - Prior errors: preserve latest bounded lineage entries with round/action/reason metadata.

2. Update terminal JSON and CLI logging.
   - Use the resolver in the data CLI terminal writer.
   - Keep log `last_error` aligned with terminal JSON final-state error.

3. Add tests.
   - Earlier rejected/deferred action followed by complete result does not leave stale final `last_error`.
   - Blocked/repair terminal still exposes final actionable error.

Validation:

```bash
go test ./internal/repl -run 'DataTask|Terminal|LastError|Lineage'
```

### Batch H - Data Action Pre-Admission

Tasks:

1. Add batch-rank validation.
   - Use workflow/action graph dependency metadata.
   - Reject cross-rank batches before action execution.

2. Add field-contract validation.
   - Use artifact schema field sets.
   - Validate `status_fields`, explicit group fields, join keys, value fields, and reference key fields.

3. Add tests.
   - Invalid cross-rank batch is rejected without execution.
   - Missing field params produce typed repair hints.

Validation:

```bash
go test ./internal/dataworkflow ./internal/repl -run 'Admission|Rank|FieldContract|DataTask'
```

### Batch I - Evidence-ID Verdict Handoff

Tasks:

1. Extend extractor verdict context with accepted evidence IDs.
2. Ensure `emit_hypothesis_verdict` resolves accepted IDs to citations without a repair retry.
3. Add tests for accepted and unknown IDs.

Validation:

```bash
go test ./internal/tool ./internal/orchestrator -run 'Hypothesis|EvidenceID|Verdict|Ground'
```

### Batch J - Answer Carrier Coverage

Tasks:

1. Normalize answer blocks into typed carrier rows.
2. Suppress supplements when authored carriers already cover required typed member rows.
3. Add tests for read-mode stage answers and multi-repo comparison answers.

Validation:

```bash
go test ./internal/tool ./internal/orchestrator -run 'Supplement|Carrier|Principal|AnswerDocument'
```

### Batch K - Multi-Repo Scope Handoff

Tasks:

1. Persist active subrepo focus in typed context.
2. Resolve scoped inventory through exact active `root_rel`.
3. Add tests that scoped inventory cannot return files from a sibling subrepo.

Validation:

```bash
go test ./internal/multigraph ./internal/tool ./internal/orchestrator -run 'MultiRepo|Scope|Inventory|Focus'
```

### Batch L - Alias Normalization

Tasks:

1. Canonicalize data action `input_paths`.
2. Preserve alias lineage separately.
3. Add tests that backend actions consume canonical aliases while terminal artifacts stay compact.

Validation:

```bash
go test ./internal/dataquery ./internal/dataworkflow ./internal/repl -run 'Alias|Lineage|Assemble|Artifact'
```

### Batch M - Representative Verification

Tasks:

1. Run focused unit tests for all implemented batches.
2. Rebuild with `make`.
3. Re-run the representative eval sweep with `PARALLEL=2`.
4. Manually audit final answers and logs.

Representative command:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_final_summary.md \
bash eval/convergence_audit.sh
```

Final commercial acceptance criteria:

- all four representative cases PASS;
- data complete terminals have no stale final `last_error`;
- prior repair errors remain auditable in lineage;
- structurally invalid data action plans are rejected before execution;
- accepted evidence IDs can ground extractor verdicts without retry;
- answer supplements do not duplicate already complete typed authored carriers;
- explicit multi-repo scoped inventory does not cross into sibling subrepos;
- code and docs contain no case-specific constants, prompt-redline workarounds, keyword intent gates, or model-prose hard gates.

### Batch N - Rule Materialization Handoff

Tasks:

1. Normalize strict/aggregation coverage contracts from rule-bearing materials.
   - Use existing coverage-material usage modes and text/constraint material classifier.
   - Require rule coverage when required rule/text materials combine with decision/contribution/reconcile validation ledgers.
   - Keep already explicit `derive_rules` and `rule_coverage_required` plans unchanged.

2. Let existing typed topology schedule the repair.
   - Missing rule coverage should surface as `missing_workflow_ledger`.
   - Deterministic fallback should emit `derive_rules` via `RuleCoverageCompletionAction`.
   - Later compute/reconcile/assemble actions should consume source-backed rule IDs through the existing ledger link validators.

3. Add regressions.
   - Initial typed action plan with required rule material, strict output, and contribution/reconcile ledgers is normalized to require rule coverage.
   - Completion gate rejects a terminal result with decision/contribution/reconcile ledgers but zero source-backed rule coverage.
   - Deterministic fallback emits a typed rule-coverage batch instead of accepting compute/reconcile completion.

Validation:

```bash
go test ./internal/repl ./internal/dataworkflow ./internal/dataquery -run 'RuleCoverage|TextConstraint|WorkflowCompletion|DataTask'
```

Batch N acceptance criteria:

- no hard rule depends on `active`, `inactive`, target label names, or case-specific values;
- no user/model prose is parsed as a hard intent gate;
- rule-bearing material evidence is handed off as source-backed `rule_coverage` records before backend contribution and final projection validators can declare completion.

### Batch O - Completion-Gate Precedence

Tasks:

1. Evaluate the current typed completion gate before status-specific routing.
   - Compute `dataTaskWorkflowCompletionGateGuardResultWithRepo` for every
     evaluation status, not only model-emitted `complete`.
   - Treat an empty current completion gate as `completion_satisfied` for
     noisy/terminal statuses when a typed terminal answer artifact is present.
   - Keep explicit `continue_data` eligible for continuation planning.
   - Keep the existing structural helper as an additional conservative
     completion signal.

2. Preserve repair lineage without letting stale violations override completion.
   - Historical action-dependency and execution errors remain in record lineage.
   - Terminal status comes from the latest typed validation, ledger graph,
     output projection graph, and reference projection graph.
   - Existing repair/fallback behavior remains unchanged when the current gate
     is non-empty.

3. Add regressions.
   - A noisy `repair_node` evaluation over a currently complete result returns
     the answer.
   - An explicit `continue_data` evaluation over an intermediate result still
     schedules the continuation planner.
   - Completion-gate failures still produce the same deterministic repair or
     fallback path.
   - Tests use typed contracts and ledgers only; no user-intent keywords, data
     value keywords, or model-prose matching.

Validation:

```bash
go test ./internal/repl ./internal/dataworkflow ./internal/dataquery -run 'EvaluationDecision|CompletionGate|RuleCoverage|DataTask'
```

Batch O acceptance criteria:

- a stale historical workflow violation cannot force terminal `repair_node`
  after the current typed completion gate is satisfied;
- incomplete latest results still route to repair/fallback through existing
  typed guards;
- final terminal status, terminal JSON, and eval verdict agree on current typed
  completion rather than historical noise.

### Batch P - Reference Candidate Lineage Priority

Tasks:

1. Classify inferred reference-candidate paths by artifact lineage.
   - Treat artifact IDs and aliases for multi-source aggregate artifacts as
     fallback candidates.
   - Keep source materials and single-source child artifacts in the primary
     candidate bucket.
   - Do not demote explicit `reference_path` contracts.

2. Reuse existing scoring within each bucket.
   - Preserve overlap/cardinality/reference-only scoring for primary candidates.
   - Try aggregate fallback candidates only when no primary candidate can repair
     the output projection graph.
   - Preserve current behavior when only aggregate artifacts are available.

3. Add regressions and rerun focused eval.
   - Single-source reference material wins over a combined records artifact even
     when the combined artifact has broader overlap with existing reconcile
     groups.
   - Existing explicit-reference tests continue to pass.
   - The focused data eval terminal is `complete` and answer projection matches
     the reference material universe.

Batch P acceptance criteria:

- reference projection never chooses a multi-source aggregate artifact ahead of
  an atomic candidate solely because the aggregate has more existing group
  overlap;
- no production branch depends on names such as a specific file, field, target,
  label, active status, or user/model prose;
- aggregate artifacts remain usable fallback reference universes for workflows
  that have no atomic reference material.

### Batch Q - Assemble Reference Consumption Handoff

Tasks:

1. Record reference projection material consumption in the runner.
   - When `assemble_answer` uses a reference candidate, attach that path to the
     artifact `source_paths`.
   - Merge the artifact source paths into result `ConsumedPaths`.
   - Keep non-reference assembly unchanged.

2. Add regressions.
   - A required reference material consumed by `assemble_answer` satisfies
     coverage validation.
   - Projection answer, reconcile report, and reference metadata remain stable.

Batch Q acceptance criteria:

- a typed reference projection cannot fail coverage solely because the runner
  omitted the selected `reference_path` from consumed-material handoff;
- no script/prose parsing or material-name keyword gate is introduced.

### Batch R - Completion Fallback Before Repair Planner

Tasks:

1. Build completion fallback for all evaluation statuses.
   - Use the current completion guard and existing validation-failure
     transition logic.
   - Keep deterministic fallback unavailable when the guard cannot produce a
     typed plan.

2. Prioritize deterministic fallback in noisy repair.
   - In `repair_node`, return completion fallback before repair fallback or
     repair planner.
   - Preserve existing repair behavior when no current-state fallback exists.

3. Add regressions.
   - Missing reconcile with contribution records and a noisy `repair_node`
     evaluation returns a `reconcile_artifacts` fallback plan.
   - Completion fallback reason/source remain tied to the typed guard.

Batch R acceptance criteria:

- a stale or noisy repair status cannot bypass an available deterministic typed
  completion fallback for the latest ledger/output graph;
- no prompt changes, prose matching, or data-value keyword logic is introduced.

### Batch S - Typed Completion Retires Stale Violation Lineage

Tasks:

1. Add a reusable typed completion predicate for `WorkflowStateView`.
   - Require current stage `complete`.
   - Require all required ledger dependencies to be satisfied.
   - Require the output projection graph to be `satisfied` with an answer
     present.
   - Keep legacy snapshots conservative by accepting only typed answer presence
     when the graph status is absent.

2. Apply the predicate before historical workflow-violation gating.
   - `NormalizeEvaluationForWorkflowState` should normalize stale/noisy
     evaluator statuses to `complete` when the current typed state proves
     completion.
   - `ConservativeEvaluationFromWorkflowState` should not call the raw
     violation gate for completed current states.
   - The raw `GateEvaluationWithWorkflowViolations` behavior remains unchanged
     for incomplete states and direct callers.

3. Add regressions.
   - Completed typed state plus old action-dependency violation returns
     `complete`.
   - Incomplete typed state plus the same violation returns `repair_node`.
   - Conservative fallback over a completed state ignores stale violations and
     returns `complete`.

Validation:

```bash
go test ./internal/dataworkflow -run 'Evaluation|Completion'
go test ./internal/repl -run 'EvaluationDecision|CompletionGate|ReferenceProjection'
```

Batch S acceptance criteria:

- historical workflow violations remain auditable lineage but cannot override a
  current completed ledger/output graph;
- incomplete current states still fail loud on typed workflow violations;
- implementation uses only typed state fields, ledgers, output projection graph,
  and result structure, with no prompt changes or prose/keyword matching.

### Batch T - Terminal Journal Completion Authority

Tasks:

1. Reuse typed ledger/output completion for journal snapshots.
   - Add a snapshot-level completion predicate.
   - Use it when resolving whether a requested `complete` terminal may override
     an older non-complete base decision.
   - Keep the original preserve-base-status behavior for incomplete snapshots.

2. Retire stale terminal decision fields only after completion is proven.
   - Terminal status and journal decision status become `complete`.
   - Final `last_error` is empty.
   - Historical plan-admission and field-contract failures remain in
     `prior_errors`, `workflow_violations`, and `last_nonterminal_error`.
   - Stale blocked decision reason, violations, and next-actions do not appear
     as the current terminal decision.

3. Add regressions.
   - Complete snapshot plus stale blocked base decision writes terminal
     `complete`.
   - Incomplete snapshot plus blocked base decision remains blocked.

Validation:

```bash
go test ./internal/dataworkflow -run 'Journal|Evaluation|Completion'
go test ./internal/repl -run 'Terminal|DataTask|CompletionGate'
```

Batch T acceptance criteria:

- terminal JSON, terminal logs, and eval status agree with current typed
  completion when ledger/output graphs are satisfied;
- blocked/base decision protection remains intact for incomplete snapshots;
- no implementation branch reads model prose, answer text, file-name keywords,
  or business values as a hard status gate.

## Final Delivery Verification

Implemented batches covered the data workflow system gaps from rule material
handoff through terminal journal completion authority. Final validation:

```bash
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp go test ./internal/repl ./internal/dataworkflow ./internal/dataquery
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp make build
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_final_summary.md \
bash eval/convergence_audit.sh
```

Representative sweep result:

| case | result | key acceptance signal |
|---|---|---|
| `qf_architecture` | PASS | canonical read-mode stages and topology authority preserved |
| `read_combo_trace_current_source_explanation` | PASS | trace parsing, jank threshold, and external-source boundary grounded |
| `data_multifile_reference_projection` | PASS | `data_status=complete`, final answer `17,0,5`, output graph `satisfied` |
| `mr_cross_repo_compare` | PASS | active subrepo buckets remain separated |

Data terminal acceptance:

- terminal status: `complete`;
- final `last_error`: empty;
- `last_nonterminal_error` keeps historical plan-admission lineage only;
- ledger graph next stage: `complete`;
- output projection graph: `satisfied`;
- final answer length: 6 (`17,0,5`);
- data eval flags: none.

The implemented hard decisions use typed contracts, ledgers, artifact lineage,
output projection graphs, structured material coverage, and journal snapshots.
They do not introduce prompt-only redline workarounds, user-intent keyword
matches, model-prose hard gates, file-name constants, or business-value patches.

## Current2 Delivery Delta - Read-Mode Stage Responsibility Handoff

The 16:07 CST representative rerun found one remaining failing case:
`qf_architecture`. The failure was not an incorrect pipeline order or a wrong
stage namespace. The answer already covered the read-mode stages and actor
bindings. The missing commercial-grade handoff was responsibility-level topology
metadata: `StageAnalyze` needs to carry classification and `AnalysisIR` /
`TaskGraph` / `EvidencePlan` / `AnswerContract` as typed stage artifacts so the
final answer surface does not depend on the model restating those terms.

This delta explicitly does not reduce answer richness. The trace case's final
answer is semantically correct and preserves requested dimensions through
multiple carriers; deduping or suppressing those carriers would weaken the
answer. The delivery objective is to enrich typed stage handoff, not to delete
supported content.

### Delta D18. Stage Responsibilities Belong In The Topology Authority

Typed behavior:

- `StageBinding` remains the canonical stage -> agent -> skill authority record;
- each binding may also carry a responsibility summary and primary artifact
  list;
- read-mode main bindings include the artifacts needed by architecture answers:
  `AnalysisIR`, `TaskGraph`, `EvidencePlan`, `AnswerContract`, accepted
  evidence/support outputs, and final answer artifacts;
- finalizer last-mile supplements render responsibility metadata only when the
  existing grounded stage-authority gate is satisfied;
- rich model-authored answer content and requested-dimension supplements are
  preserved.

No production logic may inspect eval regexes, user intent keywords, or
model-authored prose as a hard gate.

### Batch U - Stage Responsibility Authority

Tasks:

1. Extend `internal/types/stage_binding.go`.
   - Add responsibility and primary-artifact fields to `StageBinding`.
   - Populate read-mode pre-stage, main-stage, multi-repo focus, and write-mode
     bindings from code authority.
   - Deep-copy artifact slices from exported helpers to prevent accidental
     mutation of the global authority table.

2. Extend answer-document stage supplement rendering.
   - Use `types.ReadModeMainStageBindings()` instead of duplicating skill
     strings in the finalizer path.
   - Add a responsibility/artifact column to the verified stage-binding
     supplement.
   - Keep the existing grounded-source/requested-stage-workflow gate unchanged.
   - Keep rich authored sections and requested-dimension supplements intact.

3. Add regression coverage.
   - `StageAnalyze` binding includes `Classify`, `AnalysisIR`, `TaskGraph`,
     `EvidencePlan`, and `AnswerContract`.
   - The finalizer supplement renders those artifacts when stage authority is
     grounded.
   - The no-authority test still blocks ungrounded supplements.

4. Validate and rerun representative eval.
   - Run focused `internal/types` and `internal/agent` tests.
   - Rebuild `codrax`.
   - Rerun `qf_architecture`.
   - Rerun the four-case representative sweep with `PARALLEL=2`.

Validation:

```bash
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp \
go test ./internal/types ./internal/agent \
  -run 'TestReadModeStageBindings|TestAnswerDocumentEvaluator_ParseOutput_AppendsVerifiedStageBindingSupplement|TestAnswerDocumentEvaluator_ParseOutput_AppendsStageBindingForRequestedWorkflowDimension|TestAnswerDocumentEvaluator_ParseOutput_DoesNotAppendStageBindingWithoutGroundedSource'

GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp make build

CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case' \
PARALLEL=1 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_qf_after_stage_resp_summary.md \
bash eval/convergence_audit.sh

CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_after_stage_resp_summary.md \
bash eval/convergence_audit.sh
```

Batch U acceptance criteria:

- `qf_architecture` passes with canonical stage responsibility artifacts visible;
- all four representative cases remain PASS;
- trace answer richness is not weakened;
- no code path uses prompt-only fixes, eval-regex matching, user keyword
  matching, model-prose matching, file-name constants, or case-value patches.

### Delta D19. Architecture Prose Sections Can Be Typed Enum Carriers

The first Batch U qf rerun exposed a separate presentation-compiler gap. The
answer had an authored section titled `条件性前置 stage（按需执行）`, but the
principal enum set label was `条件性前置 stage（ADVISORY）`. Because the compiler
treated the parenthetical qualifier as part of the hard category key and only
counted table/list row carriers, it appended a competing missing-member table.
That supplement selected `FallbackResetTarget`, a reset-depth enum outside the
read-mode pipeline stage namespace.

Typed behavior:

- in architecture-explain answers, summary/section blocks can serve as authored
  enum-category carriers when their parenthetical-stripped label matches the
  typed set label;
- direct enumerate/source-inventory answers still require strict row carriers
  and keep missing-member supplements;
- the carrier decision is based on typed `IntentExplain`,
  `ScenarioArchitectureExplain`, block kind, and deterministic label
  normalization;
- no production branch reads eval banned strings, model rationale, user keyword
  intent, field/file-name constants, or business values.

### Batch V - Architecture Carrier Coverage

Tasks:

1. Add parenthetical-stripped category label matching.
   - Normalize `公开函数（func）`, `条件性前置 stage（ADVISORY）`, and similar
     labels to their core category for carrier matching only.
   - Preserve existing exact label scoring as the strongest signal.

2. Let architecture prose sections suppress competing supplements.
   - Apply only for `IntentExplain` + `ScenarioArchitectureExplain`.
   - Accept summary/section blocks whose visible label/text scores against the
     typed set label.
   - Do not change direct enumeration supplement behavior.

3. Add regression coverage.
   - An authored architecture section titled with a different parenthetical
     qualifier suppresses the missing-member supplement.
   - The stale unrelated row does not appear in the visible answer.

Validation:

```bash
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp \
go test ./internal/tool \
  -run 'TestNormalizePrincipalEnumerationRowBlocks_ParentheticalCategoryCarrierSuppressesSupplement|TestNormalizePrincipalEnumerationRowBlocks_AppendsOnlyMissingRowsForPartialMarkdownTable|TestNormalizePrincipalEnumerationRowBlocks_SystemSupplementOmitsEmptyLocationAndNoteColumns'

GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp \
go test ./internal/types ./internal/agent ./internal/tool
```

Batch V acceptance criteria:

- qf architecture answers do not receive stale unrelated enum supplements when
  a prose section already covers the architecture category;
- row supplements remain available for direct enumeration outputs;
- no hard decision consumes model prose intent, eval banned strings, or
  case-specific constants.

## Final Verification After Batches U-V

Validated locally:

```bash
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp go test ./internal/types ./internal/agent ./internal/tool
GOCACHE=/private/tmp/codrax-gocache GOTMPDIR=/private/tmp make build
```

Focused qf:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case' \
PARALLEL=1 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_qf_after_stage_resp2_summary.md \
bash eval/convergence_audit.sh
```

Representative sweep:

```bash
CODRAX_BIN=/Users/han/opt/codrax/codrax \
CASES='eval/cases/qf_architecture.case eval/cases/read_combo_trace_current_source_explanation.case eval/cases/data_multifile_reference_projection.case eval/cases/mr_cross_repo_compare.case' \
PARALLEL=2 RUNS=1 TIMEOUT=1200 \
SUMMARY=eval/results/representative_eval_20260609_after_stage_resp_summary.md \
bash eval/convergence_audit.sh
```

Final sweep result:

| case | result | key acceptance signal |
|---|---|---|
| `qf_architecture` | PASS | stage responsibilities and `AnalysisIR` / `TaskGraph` / `EvidencePlan` / `AnswerContract` visible; no stale `FallbackResetTarget` supplement |
| `read_combo_trace_current_source_explanation` | PASS | rich parse/jank/evidence-boundary answer preserved |
| `data_multifile_reference_projection` | PASS | terminal `complete`, final answer `17,0,5` |
| `mr_cross_repo_compare` | PASS | active subrepo buckets remain separated |

Batches U-V are implemented through typed stage authority, deterministic
answer-surface carrier coverage, and structural tests. They do not introduce
prompt-only fixes, eval-regex branches, user-intent keyword matching,
model-output prose hard gates, file-name constants, or case-value patches.
