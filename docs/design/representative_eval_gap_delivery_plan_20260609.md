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

## Architectural Decisions

### D1. Explicit Final Reference Contract Wins

When `OutputContract.CompleteReference` is true and `ReferencePath/ReferenceKeyField` are set, that pair is the final output key universe. Fallback inference may only run when the explicit contract is absent or unreadable.

Typed behavior:

- A projected answer with mismatched `reference_path` or `reference_key_field` is not complete.
- Non-reference reconcile groups remain valid audit/contribution groups but are excluded from the final projection.
- Missing reference keys are projected as zero/empty values without changing contribution records.

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

Validation:

```bash
go test ./internal/orchestrator ./internal/tool -run 'Caveat|Supplement|Principal|AnswerDocument'
```

### Batch E - Full Verification

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

