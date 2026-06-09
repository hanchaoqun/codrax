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
