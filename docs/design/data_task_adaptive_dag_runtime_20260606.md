# Data Task Adaptive DAG Runtime

## Problem

The data lane was upgraded from one-shot execution to a repairable workflow,
but the core execution unit was still too large: a model-authored Python script
could try to inspect materials, interpret rules, normalize entities, compute
contributions, reconcile totals, and render the final answer in one batch.

That architecture can detect many failures, but it does not reliably converge.
Each repair round rewrites a large procedural script, so a local problem such
as an undeclared input path, a helper misuse, or an oversized batch can cause
the model to keep reshaping the same monolith instead of making progress
toward the user's goal.

The general fix is not a domain-specific prompt or more repair retries. The
data lane needs an adaptive, goal-driven DAG runtime:

```text
user goal
  -> model proposes the next atomic action batch
  -> system executes typed actions
  -> actions produce structured artifacts
  -> evaluator checks progress toward the goal
  -> graph expands, repairs one node, or finishes
```

The DAG is not a fixed pipeline. It grows from observations. A material profile
may reveal that an entity-resolution action is needed; a failed join may reveal
that another inspection node is needed; a reconciliation mismatch may repair
only the contribution node. The system should not ask the model to rewrite an
entire end-to-end script when only one node failed.

## Red Lines

- Do not hard-code business domains, file names, column names, output shapes,
  or customer-specific task semantics.
- Do not hard-gate on user prose or model prose. Hard decisions consume typed
  route, plan status, action kinds, material paths, runner telemetry,
  validator violations, budgets, and output contracts.
- Do not route source-code analysis, trace/log diagnosis, write mode, or mixed
  source questions through the data DAG unless the typed route says data or
  mixed data.
- Do not introduce side effects into data actions. Side effects remain
  operation-lane work with operation risk/approval.
- Do not remove the existing deterministic runner. It becomes the
  `custom_transform` fallback node, not the default architecture for complex
  tasks.

## Architecture

### 1. Typed Data Actions

`TaskPlan` now supports an optional `actions[]` batch. Initial action kinds:

- `material_inventory`: discover objective local material metadata;
- `inspect_material`: inspect concrete materials and emit profiles;
- `extract_records`: convert selected CSV/TSV/JSON/JSONL/text materials into
  bounded generic record samples;
- `derive_rules`: convert model-distilled task rules or validation rules into
  typed `rule_coverage` artifacts;
- `normalize_entities`: emit typed source-to-canonical mapping artifacts;
- `compute_contributions`: read declared local inputs and produce generic
  `contributions` records from field/filter parameters;
- `reconcile_artifacts`: deterministically reconcile accumulated contribution
  groups into `reconcile` groups and a final scalar/list answer;
- `custom_transform`: run a bounded Python transform over declared input files
  only when the typed atomic actions cannot express the step.

Each action is atomic: it should answer one small observation or perform one
small transformation. Multi-stage tasks must split into multiple actions and
set `continue_after=true` when more graph expansion is expected.

### 2. Data Artifacts

`Result` now carries `artifacts[]`. Artifacts are structured, reusable outputs
from action nodes:

- id / kind;
- source paths;
- summary;
- fields;
- headers / samples / row counts;
- child artifacts for inventories or grouped profiles.

Planner, continuation, and evaluator prompts receive compact artifact previews
and counts. Full results remain in the existing audit files/logs.

### 3. Execution Semantics

When `actions[]` is present, REPL and CLI execute the action runner. When no
actions are present, existing `Runner` behavior remains unchanged.

`custom_transform` delegates to the existing safe runner with the action's
own `input_paths` and bounded script. This preserves the current sandbox,
helpers, coverage validation, ledgers, reconcile, output-contract handling,
and patch engine.

### 4. Staging Guard

The old monolithic guard remains for top-level scripts. Action plans add a
node-level guard: a single `custom_transform` script that is too large is
rejected as an oversized atomic action. The repair instruction asks the model
to split the DAG into smaller typed actions rather than rewriting another large
script.

### 5. Typed Repair Loci

Execution failures continue to be classified into typed violations. The first
new action-oriented violation is:

- `undeclared_input_path`: the script/custom transform read a path that was not
  declared in that action's `input_paths`.

The repair hint tells the planner to add the path only if the current action
should consume it; otherwise it should split a prior inspect/extract action or
remove the read. This keeps repair local to the graph node.

## Delivery Ledger

### Batch 1: Action Runtime Foundation

- [x] Record this adaptive DAG design and task ledger.
- [x] Add `DataAction` and `DataArtifact` typed structures.
- [x] Add `actions[]` to `TaskPlan` and `artifacts[]` to `Result`.
- [x] Implement `ActionRunner` with `material_inventory`,
      `inspect_material`, and `custom_transform`.
- [x] Wire REPL data dispatch to execute actions before falling back to the
      existing script runner.
- [x] Wire CLI data dispatch the same way, preserving CLI/REPL semantics.
- [x] Add action-aware prompt context, runner detail, and REPL audit preview.
- [x] Add `undeclared_input_path` typed repair classification.
- [x] Add focused unit tests for action execution and planner JSON
      compatibility.

### Batch 2: Adaptive Planner Contract

- [x] Make the data planner prefer action batches for complex tasks and use
      top-level `script` only for simple bounded transforms.
- [x] Add stronger continuation rules that consume `result.artifacts` as graph
      state rather than script prose.
- [x] Add evaluator checks for artifact progress:
      `complete`, `expand_graph`, `repair_node`, `continue_transform`,
      `blocked`, and `partial_answer_possible`.
- [x] Route evaluator statuses through distinct REPL/CLI branches:
      graph expansion and transform continuation use continuation planning,
      while node repair uses the repair planner with typed action/locus context.
- [x] Add finer DAG-node budget accounting so one bad node cannot consume the
      entire data workflow budget.

### Batch 3: More Deterministic Atomic Ops

- [x] Add `extract_records` for converting a profiled material into generic
      records.
- [x] Add `derive_rules` for converting planner-distilled rules into typed
      rule artifacts.
- [x] Add `normalize_entities` for generic source-to-canonical mapping
      artifacts.
- [x] Add `compute_contributions` for generic contribution tables.
- [x] Add `reconcile_artifacts` for deterministic artifact-level reconciliation.
- [x] Keep all ops domain-neutral: no business-specific fields in Go logic.

Notes: `compute_contributions` accepts generic field, filter, metric,
operation, and item locator parameters. It does not know any business domain.
`reconcile_artifacts` consumes only typed contribution records and recomputes
numeric groups through the existing deterministic reconcile validator.

### Batch 4: Node-Level Repair

- [x] Return typed node failures with action id, action kind, script line,
      JSON path, and repairability.
- [x] Repair only the failed node when possible.
- [x] Escalate to graph expansion when a transform reads undeclared material or
      lacks schema knowledge.
- [x] Use result patching only for structural JSON drift, never for business
      semantics.

### Batch 5: Eval and UX Hardening

- [x] Add eval/unit regression coverage for non-procurement data tasks:
      CSV sum, JSONL filtering, multi-file join, text material filtering,
      strict JSON-only output, and action-only inventory/inspection.
- [x] Add regression checks that source, trace/log, operation, and write-mode
      routes do not see the data DAG unless typed routing selects it.
- [x] Keep REPL previews compact and full artifacts/logs auditable.
- [x] Keep CLI progress on stderr and final answer on stdout.

## 2026-06-06 Real-Scenario Audit

Latest compiled binary was run against a real local data aggregation task with
structured CSV/text materials and a strict single-line output contract. The run
failed before producing stdout. The failure is useful because it exposed
system-level convergence gaps rather than a business-domain gap.

Observed sequence:

1. Initial data plan emitted a 227-line top-level script over six inputs and
   multiple required ledgers.
2. Staging guard correctly rejected the monolithic script as too large for one
   bounded batch.
3. Repair generated `actions[]` but also kept a top-level script. The system
   rejected this shape, then spent multiple model calls asking for the same
   structural fix.
4. A later action batch executed, but `normalize_entities` failed because the
   action omitted `source_field/source_fields/name_fields`.
5. Repair fell back toward a large `custom_transform`, then again produced a
   `normalize_entities` action without the required field parameters.

Root causes:

- `actions[] + top-level script` is a pure structural drift. Asking the model
  to repair it consumes budget even when the safe transform is obvious.
- `normalize_entities` was useful as an atomic node, but its parameter contract
  was brittle. Missing source-field parameters caused runner failure instead of
  a generic schema inference or an earlier structural preflight.
- Repair prompts sometimes expose compact script previews. When the preview is
  truncated, the model can misdiagnose truncation as the cause of a missing
  result. Structural violations must carry precise typed context and should not
  force the model to infer root cause from truncated code snippets.
- The system can detect large scripts, but it still allows the planner to spend
  its first turn producing one. Complex tasks should converge through atomic
  action batches from the start.

### Batch 6: Shape Normalization and Schema Inference

- [x] Add deterministic data plan shape normalization: when a plan contains
      `actions[]`, exactly one empty `custom_transform`, and a top-level
      script, move the script into that `custom_transform` and clear the
      top-level script. This is a safe structural repair; it does not change
      business semantics.
- [x] Apply the same shape normalization in REPL and CLI for initial, repair,
      and continuation plans before rendering/auditing/execution.
- [x] Add generic `normalize_entities` schema inference. If a structured action
      omits source fields, infer candidate source fields from table headers and
      textual value shape, and infer canonical id/label fields from objective
      header structure. This is domain-neutral metadata inference, not a user
      intent hard gate.
- [x] Preserve inference details in action artifacts so downstream prompts and
      audit logs can see which fields were inferred.
- [x] Add regression tests for shape normalization and inferred entity
      normalization.

## 2026-06-06 Second Real-Scenario Audit

After Batch 6, the same real data task no longer looped on
`actions[] + top-level script` or missing `normalize_entities` parameters.
It still failed to converge because two broader architecture gaps remained:

1. Repair turns could silently weaken material coverage. A previous batch
   declared a material as required, then a later repair moved/dropped it while
   fixing an unrelated script or ledger error. This lets the workflow drift
   away from the user's original material coverage goal.
2. Result ledgers still had shape drift. In particular, `result.rows` may be
   emitted as scalar notes rather than structured objects. That is a structural
   JSON-shape problem, not a business-semantics problem, and should be safely
   normalized or patched before asking the model to rewrite a script.

### Batch 7: Coverage Preservation and Scalar Ledger Normalization

- [x] Make non-staged repair plans preserve the previous required material
      contract by default. Staged oversized-batch expansion can still narrow
      the active batch, but ordinary node/script repair cannot silently drop
      coverage obligations.
- [x] Keep required runner inputs synchronized with preserved material
      coverage so later scripts can actually consume inherited materials.
- [x] Normalize scalar `result.rows` entries into generic decision records
      (`decision=observed`, `reason=<scalar>`). This is safe structural
      compatibility only; meaningful ledger/reconcile validators still decide
      whether the result is complete.
- [x] Add regression tests for scalar decision rows and coverage preservation
      across repairs.

## 2026-06-06 Third Real-Scenario Audit

After Batch 7, the workflow preserved cross-round material and ledger
contracts and `derive_rules` could consume its input materials. The same real
task still did not converge. The new failure mode was more architectural:

- The planner understood that it should split the task into actions, but the
  available typed actions were not yet expressive enough for the required
  join/filter/normalization/aggregation chain.
- The planner therefore used `custom_transform` as the intended fallback node,
  but the node-level breadth guard rejected a 123-line transform before the
  deterministic runner and ledger validators could inspect it.
- Subsequent repair turns oscillated between "split more" and "write the
  bounded fallback transform", consuming budget without producing new
  executable evidence.

This is not a reason to return to one-shot scripts. The generic fix is to keep
the top-level one-shot guard strict, while allowing action-level
`custom_transform` to act as a bounded DAG node when typed atomic actions are
not yet expressive enough. Correctness still comes from material coverage,
ledger requirements, reconcile checks, output-contract validation, and audit
artifacts, not from trusting the transform prose.

### Batch 8: Bounded Custom Node Convergence

- [x] Keep rejecting complex top-level scripts for multi-material /
      multi-ledger data tasks.
- [x] Increase the action-level `custom_transform` broad-node threshold from
      120 to 180 lines so a bounded fallback node can execute when it is part
      of the adaptive DAG.
- [x] Preserve the hard node cap at the existing one-shot soft limit for larger
      transforms.
- [x] Add a regression test that complex but bounded action-level transforms
      below the threshold pass staging and are left to deterministic execution
      and validation.

## 2026-06-06 Fourth Real-Scenario Audit

After Batch 8, the workflow reached execution more often, but it still failed
on a contract-boundary issue:

- A previous action could already consume one required material.
- A later `custom_transform` action only needed a subset of the workflow
  materials.
- The custom action's child runner still inherited the full workflow
  `CoverageContract`, so it rejected the node because a different workflow
  material was not declared in that node's `input_paths`.

This is a generic DAG semantics gap. Node execution and workflow completion
must validate different things:

- node execution: only validate the materials that this node is responsible for
  reading;
- workflow completion: validate the cumulative material coverage, ledgers,
  reconcile state, and output contract across all executed nodes.

### Batch 9: Node Coverage vs Workflow Coverage

- [x] Make action-level `custom_transform` child runners receive a node-scoped
      coverage contract derived from that action's declared inputs.
- [x] Keep the outer `ActionRunner` final validation against the full workflow
      contract and cumulative consumed paths.
- [x] Relax the child runner output contract to `freeform` so intermediate
      custom nodes can produce structured artifacts without being forced into
      the final user-facing output shape.
- [x] Add regression coverage where one node consumes material B and a later
      custom node consumes material A; the workflow must pass because cumulative
      coverage contains both A and B.

## 2026-06-06 Fifth Real-Scenario Audit

After node/workflow coverage was separated, the real task progressed further
but still spent budget on a structural placement error:

- a bounded repair script satisfied the custom-node size limit and contained a
  result emitter;
- the planner emitted it as a top-level `script` on a complex plan instead of
  as a `custom_transform` action;
- the staging guard correctly rejected complex top-level scripts, but this
  caused another model repair even though the safe structural intent was clear.

This is another schema-shape problem, not a business-semantics problem. The
system can safely wrap a bounded, emitter-bearing top-level script into a
single `custom_transform` action when the plan is complex but the script is
below the custom-node threshold. Larger scripts remain rejected.

### Batch 10: Bounded Script Structural Wrapping

- [x] Add deterministic normalization that wraps bounded complex top-level
      scripts into a `custom_transform` action.
- [x] Keep oversized scripts rejected; wrapping only applies below the
      action-level custom-node threshold and only when a result emitter exists.
- [x] Preserve input paths, output contract, and coverage contract unchanged so
      workflow-level validation still owns correctness.
- [x] Add regression coverage for the wrapping rule.

## 2026-06-06 Sixth Real-Scenario Audit

After Batch 10, the real task entered the adaptive graph but still failed in a
later repair round. The important distinction was:

- a broad custom transform as the first/only action is still a monolithic
  workflow and should be rejected;
- a bounded custom transform after earlier typed actions can be a legitimate
  final DAG node. The typed actions have already materialized schema/rules/
  intermediate observations, and deterministic material coverage, ledgers,
  reconcile, output contract, and audit validation still decide correctness.

The previous guard treated both shapes the same because it only looked at
script size, input count, and workflow ledgers. That consumed repair budget
after the graph had already moved past the initial one-shot risk.

### Batch 11: Final Bounded Transform After Typed Context

- [x] Keep rejecting top-level complex scripts and first/only broad
      `custom_transform` nodes.
- [x] Allow a bounded final `custom_transform` below the one-shot soft line
      limit when it follows at least two typed non-custom actions.
- [x] Deterministically append a bounded top-level script as a final
      `custom_transform` when an action plan already has enough typed context.
- [x] Apply that normalization even when earlier custom nodes already have
      their own scripts; action scripts remain node-local and the leftover
      top-level script becomes a distinct final transform node.
- [x] Treat one typed action plus one already-scripted custom action as enough
      graph context for this structural normalization. This avoids wasting a
      repair round on leftover top-level script placement while still keeping
      first/only custom monoliths rejected.
- [x] Add regression tests for both boundaries: first broad custom remains
      rejected, final bounded transform after typed context passes.

## 2026-06-06 Seventh Real-Scenario Audit

After Batch 11, the workflow no longer got stuck on top-level script placement.
It continued into node execution and repair, then failed on empty required
ledgers. The recurring pattern was generic:

- model-authored scripts often call helper functions such as
  `add_contribution(...)`;
- the same script may also keep a local list named `contributions` and pass
  that empty local list to `emit_result(..., contributions=contributions)`;
- the explicit empty list overwrote the helper ledger, so deterministic
  validation saw `result.contributions` as empty even though the helper channel
  was the intended source of truth.

This is a structural result-shape problem, not a business-semantics problem.
The same issue can affect decisions, rules, contributions, and entity
resolutions in any data task.

### Batch 12: Helper Ledger Preservation and Reconcile Fill

- [x] Make the Python helper preserve non-empty helper ledgers when
      `emit(...)` or `emit_result(...)` receives an explicit empty ledger list.
- [x] Keep non-empty explicit ledgers authoritative; the preservation only
      handles empty-list shape drift.
- [x] Fill `reconcile.groups` deterministically from contribution records when
      contributions are present but reconcile groups are missing or empty.
      This does not change the final answer or business inclusion decisions.
- [x] Add regression coverage for empty explicit ledger override and
      contribution-derived reconcile groups.

### Remaining Architecture Work

- [ ] Prefer atomic action batches in the initial planner more strongly. The
      staging guard currently catches oversized scripts after the first model
      call; a future planner contract should make complex-task first output
      action-only unless the task is demonstrably simple.
- [ ] Add action-parameter preflight for every typed action. Missing parameters,
      invalid filters, and unsupported field references should become typed
      violations before execution.
- [ ] Add graph-expansion fallbacks for schema gaps. If a typed action cannot
      infer enough schema, automatically run or request `inspect_material` /
      `extract_records` before asking the model to rewrite a transform.
- [ ] Reduce reliance on truncated script previews in repair prompts. Repairs
      should prefer typed violation fields, exact script line excerpts, and
      audit artifact paths over broad script previews.
- [ ] Add a first-class coverage-delta contract. If a workflow genuinely wants
      to downgrade or replace a previously required material, it should express
      that as typed coverage intent with verifier-readable reason and evidence,
      not as an incidental repair side effect.
- [ ] Move more result-shape drift into deterministic patching: scalar rows,
      object/list aliases, missing structural status fields, and group/metric
      normalization should be fixed without changing business meaning; semantic
      errors should still trigger bounded recomputation.

## 2026-06-06 Eighth Real-Scenario Audit

After Batch 12 the helper-ledger issue was fixed, but the real scenario still
failed before producing an answer. The failure was again structural and
domain-neutral:

- a repair converted the task into an action plan, but the local action still
  inherited the full workflow coverage contract;
- a rule-derivation or narrow transform node was therefore required to consume
  unrelated source rows, lookup tables, and text evidence;
- later repairs oscillated between a local node and a broad custom transform,
  exhausting budget without a convergent graph;
- CLI runs did not persist full data-task plan/action scripts to `data-audit`,
  making postmortem harder than the REPL path.

The core design correction is that action nodes and workflow-final validation
must be separate. A node only proves its local artifact. The complete user goal
is checked later by the workflow evaluator and final coverage/reconcile gates.

### Batch 13: Scoped Action Repair and CLI Audit Sink

- [x] Add a typed `oversized_action_plan` violation for broad
      `custom_transform` nodes so repair logic can reason from a schema-level
      code instead of free-form error prose.
- [x] Keep repair coverage scoped when the repair is a typed action batch that
      intentionally narrows materials or validation ledgers. Earlier workflow
      coverage remains in the record list and is still enforced for final
      results.
- [x] Mark scoped repair batches as `continue_after=true`, making them
      intermediate DAG steps instead of forced final answers.
- [x] Skip full workflow-final validation for `continue_after` batches. The
      batch still goes through runner-local validation; final coverage and
      reconcile are checked when the evaluator decides the workflow is complete.
- [x] Add CLI data audit persistence for full plan JSON plus top-level and
      action scripts, matching the REPL auditability expectation without
      polluting stdout.
- [x] Add regression tests for scoped action repair and intermediate-batch
      validation boundaries.

## 2026-06-06 Ninth Real-Scenario Audit

After Batch 13 the same real scenario completed runner execution and
deterministic validation, but stdout still violated a strict output-only user
contract. The final result contained a stringified result object and audit
details instead of the requested single-line answer.

Root cause:

- The script used `emit_result({...})`, passing the complete structured result
  object as the first positional argument.
- The Python helper treated that object as an answer value and stringified it,
  producing an `answer` like `{'answer': '...', 'output_contract': ...}`.
- Downstream rendering correctly honored `explanation_allowed=false`, but it
  received an already-polluted `answer` from the runner.

This is a generic result-contract compatibility issue. Models can legitimately
express structured results either as `emit({...})` or `emit_result({...})`.
The runner should normalize both forms into the same `Result` object before
validation and rendering. The fix is structural only; it does not alter
business calculations, material coverage, ledger records, or answer semantics.

### Batch 14: Structured `emit_result` Normalization

- [x] Make `emit_result({...})` treat a dictionary first argument as a full
      result object instead of stringifying it into `answer`.
- [x] Preserve explicit keyword overrides, helper-ledger merging, and
      non-empty explicit ledgers.
- [x] Keep strict output rendering unchanged: when the normalized output
      contract disallows explanations, only the normalized `answer` reaches
      stdout/final answer.
- [x] Add regression coverage for object-style `emit_result` plus helper-ledger
      preservation.

## 2026-06-06 Tenth Real-Scenario Audit

After Batch 14 the stdout contract was fixed, but the answer values changed
between runs and the latest successful run was still not trustworthy. The run
showed a more important generic workflow gap:

- an earlier plan required six materials;
- a later scoped repair batch narrowed the active node to four required
  materials, which is legitimate for intermediate execution;
- the evaluator then returned `complete`, and the system accepted that typed
  model judgment without re-running the cumulative workflow coverage gate;
- the final result therefore passed node-local validation but did not prove the
  original user-goal material coverage.

This is not a domain issue. Any adaptive data DAG can have local node coverage
that is narrower than the complete workflow target. Model evaluation can guide
the next step, but final completion must be accepted only after deterministic
cumulative coverage, ledger, reconcile, and output-contract checks pass.

### Batch 15: Deterministic Completion Gate

- [x] Add a workflow completion gate that re-checks cumulative material
      coverage and required ledgers even when the model evaluator says
      `complete`.
- [x] Apply the same gate to REPL and CLI data paths.
- [x] Treat completion-gate failure as a repairable typed workflow error
      instead of emitting a partial answer.
- [x] Add regression coverage proving that a scoped intermediate batch cannot
      complete while an earlier workflow-required material remains unconsumed.

## 2026-06-06 Eleventh Real-Scenario Audit

After Batch 15, the real workflow no longer accepted a node-local result as a
complete answer. It continued execution until a typed intermediate action
returned no semantic records:

- `normalize_entities` was used to normalize a status/category-like material,
  but the selected fields and filters produced zero entity-resolution rows;
- `compute_contributions` was used on an auxiliary manifest-like material that
  had no numeric value field and therefore produced no contribution rows;
- both conditions were treated as fatal node execution failures, even though
  an adaptive DAG can legitimately contain inspection/normalization/contribution
  attempts that only prove "this node has no contributing records".

This was still too brittle. A typed action returning zero records is not by
itself evidence that the user goal failed. It is a local artifact that should be
available to the evaluator and later graph steps. The final workflow gates
remain responsible for deciding whether enough records, contributions,
coverage, reconcile data, and output evidence exist to answer the user.

### Batch 16: Zero-Result Typed Action Tolerance

- [x] Make `normalize_entities` return an auditable zero-count artifact instead
      of failing when a structured action consumes its inputs but emits no
      entity resolutions.
- [x] Make `compute_contributions` skip records that do not contain the selected
      value field and return a zero-count artifact when no contribution rows are
      produced.
- [x] Preserve consumed material paths and child artifact summaries for zero
      results, so the workflow evaluator can still reason about what was tried.
- [x] Keep validation strict for non-empty records. Structural or ledger
      violations are still rejected once records exist.
- [x] Add regression tests for zero-result normalization and non-contributing
      contribution inputs.

## 2026-06-06 Twelfth Real-Scenario Audit

After Batch 16 the real workflow completed and produced a strict single-line
answer. The output shape was correct, but manual audit showed it was still not
trustworthy:

- `derive_rules` was present, but it derived records from generic output
  validation text instead of the explicit rules material;
- the final transform read the source rows and invoice text, but its
  contribution records did not link to any derived rule;
- `reconcile.status=pass` only proved the script was internally self-consistent,
  not that the user-specified cleaning rules governed the included/excluded
  items.

This exposed a generic "parallel ledgers without lineage" gap. A data result
can have rule coverage, contribution rows, decisions, and reconcile groups, but
still be weak if the ledgers are not connected. For any rule-driven cleaning,
filtering, joining, or aggregation task, final acceptance must require a
machine-checkable lineage from rule records to the item-level ledgers that
produce the output.

### Batch 17: Rule Coverage Lineage Contract

- [x] Make `derive_rules` consume explicit action input materials before falling
      back to global validation rules. Explicit `input_paths` now produce
      line-backed rule records from the actual material.
- [x] Normalize plans containing `derive_rules` so
      `coverage_contract.rule_coverage_required=true` is enforced even if the
      model omitted the flag.
- [x] When rule coverage is required together with decision, contribution, or
      entity-resolution ledgers, require at least one item-level ledger record
      to link to a known `rule_coverage.rule_id` through `rule_refs`.
- [x] Keep pure rule extraction and pure summary tasks unaffected: the linkage
      requirement only applies when rule coverage is combined with item-level
      ledgers.
- [x] Add regression tests for explicit rules-material priority, derived-rule
      contract normalization, and unlinked rule coverage rejection.

## 2026-06-06 Thirteenth Real-Scenario Audit

Batch 17 correctly rejected a self-consistent but weak result. The next run
then exposed a deeper DAG execution gap:

- the planner split the work into multiple batches;
- one batch derived/inspected materials and another attempted contribution or
  reconciliation actions;
- each batch was executed by a fresh `ActionRunner`, so `reconcile_artifacts`
  could not see contribution records produced by previous batches;
- the planner reacted by collapsing back into broad `custom_transform` nodes,
  leading to repeated oversized-node repairs.

The design intent is an adaptive DAG, but the executor was still treating each
batch as an isolated island. Model-visible history is not enough; typed runner
state must also carry forward previous artifacts, rule coverage, contributions,
entity resolutions, reconcile data, and consumed paths.

### Batch 18: Cross-Batch Runner State Seed

- [x] Add `ActionRunner.Seed` so a batch can start with the latest cumulative
      typed result from earlier workflow rounds.
- [x] Seed artifacts, consumed paths, rule coverage, contributions, entity
      resolutions, and reconcile data before executing the current batch.
- [x] Wire REPL and CLI data workflows to seed action execution from the latest
      workflow result.
- [x] Keep model planning unchanged. The executor only consumes structured
      prior results; it does not infer business semantics from prose.
- [x] Add regression coverage where a later `reconcile_artifacts` action uses
      contribution records produced by an earlier seeded result.

## 2026-06-06 Fourteenth Real-Scenario Audit

After Batch 18, cross-batch typed state worked, but the real workflow still hit
a generic planning failure:

- a broad candidate-material set was promoted into hard required materials;
- the repair loop tried to satisfy that contract with a single large
  `custom_transform` script;
- the staging guard rejected the oversized node, but the next repair still
  tried to rewrite the same broad script instead of first discovering objective
  material structure;
- the workflow then lost convergence even though the user goal was still a
  normal read-only data task.

The systemic issue is not that a particular business file was required. A data
task planner can legitimately see many candidate materials. The executor must
not treat every candidate as a hard script-consumed input before the workflow
has discovered objective material metadata and selected the next bounded node.

### Batch 19: Broad Material Discovery Fallback

- [x] Detect plans that combine a very broad material set with a top-level
      script or broad `custom_transform` before sending them into another
      script-rewrite repair.
- [x] Convert those plans into a deterministic `material_inventory` batch with
      `continue_after=true`, preserving the user goal and success criteria but
      dropping premature hard required-material gates.
- [x] Apply the fallback in both REPL and CLI data workflows.
- [x] Keep the fallback structural: it only uses plan size, action shape, and
      typed material lists. It does not inspect business names, file names, or
      model prose.
- [x] Add regression coverage for broad material-script plans being converted
      into a discovery batch.

## 2026-06-06 Fifteenth Real-Scenario Audit

Batch 19 was still too narrow. The real workflow showed two general failures:

- a single `custom_transform` with a broad material set and a medium-sized
  script bypassed the staging guard because it was below the script-line
  threshold;
- after the cumulative completion gate rejected the result, the model emitted
  a terminal `status=complete` plan and the workflow accepted the latest answer
  without re-running the same gate.

Both are architecture issues, not task-specific errors. Data workflow control
must treat model terminal states as candidates, not authority, and must prefer
objective material discovery before executing broad single-node transforms.

### Batch 20: Terminal Completion Gate and Pre-Execution Discovery

- [x] Re-run the cumulative workflow completion gate when the model emits a
      terminal `complete`/`completed` plan and a latest result exists.
- [x] Route terminal-completion gate failures back through the same repair loop
      in REPL and CLI instead of returning the stale answer.
- [x] Run broad-material discovery fallback before execution as well as inside
      staging-guard repair.
- [x] Avoid repeated discovery loops by skipping the fallback once a prior
      `material_inventory` action exists in the workflow records.
- [x] Add regression tests for terminal completion rejection and one-shot
      material discovery fallback behavior.

## 2026-06-06 Sixteenth Real-Scenario Audit

Batch 20 prevented stale completion, but the next repair still failed on a
generic helper-shape issue:

- the generated script called helpers as both system ledger emitters and local
  list appenders, for example `add_decision(local_rows, item_id=...)`;
- Python rejected this before entering the helper body because the old helper
  signatures had named positional parameters;
- the result was another full-script repair even though the script intent was
  structurally clear.

This is a common LLM shape-drift pattern. The runner should keep one canonical
ledger contract, but helper functions can safely accept an optional local list
sink and append the same canonical record to both the caller's list and the
system ledger.

### Batch 21: Flexible Ledger Helper Sink

- [x] Change `add_decision`, `add_rule_coverage`, `add_resolution`, and
      `add_contribution` to accept `*args/**kwargs`.
- [x] Support an optional first positional local `list` sink while preserving
      the canonical system ledger.
- [x] Keep canonical field normalization and alias handling inside the helper;
      this is not a second result schema.
- [x] Add runner regression coverage proving local list sinks and system
      ledgers remain in sync.

## 2026-06-06 Seventeenth Real-Scenario Audit

Batch 21 fixed helper-call shape drift, but the next real run still did not
converge. The deeper issue is graph shape, not one helper or one business
dataset:

- after a `material_inventory` batch, the planner briefly proposed typed
  actions, but then placed a 180+ line `custom_transform` after two lightweight
  actions;
- the existing guard allowed that node because any two prior typed actions were
  treated as enough context;
- the broad transform did not have all of its required inputs profiled or
  consumed by prior actions, so execution failed on material coverage and the
  repair loop collapsed back to a single large script;
- this can happen for any multi-material data task: CSV joins, JSONL
  aggregations, evidence-backed summaries, OCR-backed calculations, or strict
  output transforms.

The generic rule should be: a broad `custom_transform` is allowed only as a
bounded transform over already-profiled/derived/normalized materials. It cannot
serve as the first node that simultaneously discovers schemas, interprets
rules, joins references, computes contributions, reconciles totals, and renders
the final answer.

### Batch 22: Broad Custom Transform Prerequisite Gate

- [x] Add a workflow staging guard that detects broad `custom_transform` nodes
      using structural signals only: script size, input count, required
      material count, validation ledger count, and prior typed coverage.
- [x] Treat earlier workflow results and earlier non-custom actions in the same
      batch as prerequisite coverage; do not infer coverage from prose or file
      names.
- [x] Reject broad transforms whose required runner inputs are not covered by
      prior material/profile/rule/entity/contribution actions, with a typed
      repair hint that asks for smaller action nodes before the transform.
- [x] Keep the existing plan-only "typed action context" behavior stable, but
      make the actual REPL/CLI workflow execution path require prerequisite
      coverage before accepting a broad transform.
- [x] Update planner/continuation prompt text to explain that `custom_transform`
      is a final bounded transform over known inputs, not a discovery + compute
      + reconcile catch-all.
- [x] Add regression tests covering: allowed narrow custom transform, rejected
      broad transform after insufficient typed context, and allowed broad
      transform when prior records have consumed/profiled all required inputs.

## 2026-06-06 Eighteenth Real-Scenario Audit

Batch 22 stopped broad transforms from running before their inputs were
profiled, but the real run exposed the next architecture gap:

- the model started planning in the right direction: `extract_records` created
  an `all_records`/record artifact, and a later node attempted to compute from
  that intermediate result;
- the prompt described action nodes as reusable, but the executor only kept
  artifacts as in-memory summaries for prompts;
- `resolveActionInputPath` and the Python runner accepted only workspace
  files, so a later action trying to read `all_records` failed with
  `no such file or directory`;
- after that, the model reasonably fell back to one broad script that reread
  the raw files, which the staging guard correctly rejected.

This is a generic DAG execution gap. Any multi-step data task can need
intermediate artifacts: parsed records, distilled rules, normalized entities,
filtered subsets, contribution records, OCR text, merged row sets, or
reconcile summaries. If these artifacts are visible only in the prompt and not
as runner-readable inputs, the workflow cannot converge through small atomic
steps.

### Batch 23: Action Artifact Bus

- [x] Materialize each completed action result into a temporary read-only JSON
      artifact during the `ActionRunner` run.
- [x] Register stable aliases for `action.id`, `action.output_artifact`, and
      the produced artifact id, plus `.json` variants.
- [x] Let later typed actions resolve these aliases through the same
      `readActionRecords` path used for workspace data files.
- [x] Let later `custom_transform` nodes list these aliases in `input_paths`;
      the lower-level runner copies them as declared read-only inputs, so
      `json_load("artifact_id")` works without granting broader filesystem
      access.
- [x] Preserve real workspace-file priority: a true local file with the same
      path still wins over a temporary artifact alias.
- [x] Materialize `extract_records` outputs as generic record arrays with
      `_source_path`, `_source_index`, and `_source_locator` metadata so
      downstream actions have source provenance.
- [x] Update planner and continuation prompt text to teach that action ids and
      `output_artifact` names are reusable JSON materials for later actions.
- [x] Add regression tests proving both a later `custom_transform` and a later
      `compute_contributions` action can consume an earlier `extract_records`
      artifact.

## 2026-06-06 Nineteenth Real-Scenario Audit

Batch 23 made intermediate artifacts runner-readable, but the next real run
exposed two workflow-contract gaps that apply to any multi-step data task:

- `continue_after=true` allowed a top-level script to skip staging checks. A
  non-terminal batch still has to be executable and bounded; otherwise a model
  can submit a debug/probing script that prints material but never emits a
  structured result, and the repair loop starts from a noisy script failure.
- Intermediate typed actions inherited the final workflow coverage contract.
  A `derive_rules` node that legitimately consumes only a rules material was
  rejected because the final answer also needs source rows, contribution
  ledger, and reconcile output. That pushes the planner back toward a single
  large script instead of letting the DAG converge through small nodes.

The generic distinction is now explicit:

- **Node-local validation**: every batch must be executable, bounded, and
  validate only the materials and ledgers that the current action kinds are
  responsible for producing.
- **Workflow-level validation**: the merged material coverage, ledgers,
  reconcile facts, and strict output contract are checked only when a terminal
  result is proposed.

### Batch 24: Node-Local Contract Validation

- [x] Do not let `continue_after=true` bypass top-level script staging guards.
      Non-terminal scripts still need a result emitter and must not be broad
      one-shot transforms.
- [x] Add action-runner node-local validation for intermediate batches:
      `derive_rules` validates rule coverage, `normalize_entities` validates
      entity resolutions, `compute_contributions` validates contributions,
      and `reconcile_artifacts` validates reconcile output.
- [x] Scope intermediate required-material checks to the current action input
      paths instead of the whole final workflow material set.
- [x] Use a freeform intermediate output contract so strict final formats such
      as `plain_single_line` do not reject rule/profile/artifact batches.
- [x] Keep terminal action batches on the full workflow contract.
- [x] Add regression tests proving an intermediate rule-derivation node can
      pass with only its rule material while the same node remains invalid as
      a terminal final answer.
- [x] Add regression tests proving `continue_after` no longer bypasses script
      emitter/bounded-batch checks.

## 2026-06-06 Twentieth Real-Scenario Audit

Batch 24 let intermediate nodes progress without final-contract false
failures. The next real run reached a multi-action plan, but the planner used
a natural artifact path such as `artifacts/compute_contributions.json` in a
later node. The executor only exposed earlier action outputs through exact
aliases like `compute_contributions` or `compute_contributions.json`.

This is a generic DAG usability gap. Models, skills, and future deterministic
actions need a stable namespace for intermediate artifacts, not a hidden set
of alias rules. The system should support both concise action-id aliases and
an explicit `artifacts/` namespace.

### Batch 25: Stable Artifact Namespace

- [x] Register every action artifact under direct aliases:
      `action.id`, `action.id.json`, `action.output_artifact`,
      `action.output_artifact.json`, `artifact.id`, and `artifact.id.json`.
- [x] Also register stable namespace aliases:
      `artifacts/<alias>` and `artifacts/<alias>.json`.
- [x] Preserve real workspace file priority. A real local file with the same
      path still wins over a temporary action artifact.
- [x] Keep artifacts read-only and scoped to the current `ActionRunner` run.
- [x] Add regression coverage proving a later custom transform can read a
      prior action output through `artifacts/<id>.json`.

## 2026-06-06 Twenty-First Real-Scenario Audit

With artifact namespaces available, the real run advanced into a larger action
batch. The planner then placed multiple scripted `custom_transform` nodes in
one batch. Even if each script is individually smaller than the one-shot
limit, multiple fallback scripts in one batch recreate the same problem as a
large hidden script: ambiguous ownership of final answer, duplicated business
logic, and weak stepwise convergence.

This is a graph-shape invariant, not a task-specific rule. A data DAG batch
can contain many typed actions, but scripted fallback nodes must stay singular
and bounded. Multiple transforms should be represented as separate batches
linked by artifacts, or as typed actions that produce reusable records,
contributions, or reconcile summaries.

### Batch 26: Single Custom Transform Per Batch

- [x] Add a staging guard that rejects any actions[] batch containing more
      than one scripted `custom_transform`.
- [x] Keep typed actions unrestricted; the limit applies only to scripted
      fallback transforms.
- [x] Update planner and continuation instructions so models split multiple
      transforms across batches or express them as typed artifact-producing
      actions.
- [x] Add regression coverage for multi-custom rejection.

## 2026-06-06 Twenty-Second Real-Scenario Audit

The next run showed that the material-discovery fallback was too eager. It
converted a plan that already contained typed actions plus a custom fallback
into another `material_inventory` batch. That erased the model's graph
progress and caused repeated discovery loops.

Fallback conversion should only handle pure broad scripts or pure broad custom
plans. Once a plan contains typed actions, the workflow should let the staging
guards reason about the graph shape instead of replacing the whole batch with
inventory.

### Batch 27: Discovery Fallback Scope

- [x] Restrict material-discovery fallback to plans without non-custom typed
      actions.
- [x] Preserve typed action plans so `inspect_material`, `derive_rules`,
      `extract_records`, `normalize_entities`, `compute_contributions`, and
      `reconcile_artifacts` can actually advance the graph.
- [x] Keep pure broad custom plans eligible for discovery fallback.
- [x] Add regression coverage proving typed action plans are not stolen by the
      fallback.

## 2026-06-06 Twenty-Third Real-Scenario Audit

After Batch 27, the same real data task still did not converge. The audit
showed a generic graph-shape problem, not a business-domain problem:

- Repair plans grew into large batches with 7-9 actions. That recreates a
  hidden one-shot workflow even when each action looks typed.
- `reconcile_artifacts` was allowed into a batch before any prior result had
  actually produced contribution records. The executor caught it only at run
  time.
- `compute_contributions` could reference fields that did not exist in any
  input material. It then produced zero contributions, which looked like a
  valid empty result until a later reconcile node failed.

The generic invariant is now stronger:

- A data DAG can use many batches, but each batch must stay small and
  stage-local.
- A node with structural dependencies must have the required predecessor
  signal before it runs.
- A typed calculation node must fail loudly when its declared structural
  fields are absent, while still allowing legitimate zero-result cases when
  the fields exist.

### Batch 28: Action DAG Structural Guardrails

- [x] Add an atomic batch-size guard for `actions[]`. More work should be
      expressed through additional continuation batches, not one giant action
      list.
- [x] Add a typed prerequisite guard for `reconcile_artifacts`: a previous
      result or earlier same-batch action must be capable of producing
      contribution records.
- [x] Avoid model-prose or artifact-name keyword gates. The new hard checks
      use typed action kind and typed result fields only.
- [x] Add `compute_contributions` field-contract validation. `value_field`,
      `group_key_field`, and filter fields must exist in at least one input
      record/header when declared.
- [x] Preserve legitimate zero-result contribution batches when declared
      fields exist but no rows match.
- [x] Add regression coverage for oversized action batches, reconcile without
      contribution predecessors, and missing contribution fields.

### Remaining Follow-Ups

- [ ] Add finer DAG-stage hints so repair planners prefer one or two next
      actions instead of filling the batch limit.
- [ ] Add evaluator checks that distinguish "zero because no rows matched"
      from "zero because the wrong source material was used".
- [ ] Expand domain-neutral evals for joins, contribution ledgers, reconcile
      groups, strict output contracts, and multi-batch graph convergence.

## 2026-06-06 Twenty-Fourth Real-Scenario Audit

After Batch 28, the planner stayed within the batch limit, but a typed
`compute_contributions` node failed on a JSON-shape mismatch:

- `filters_json` used `{"op":"in","value":["paid","accepted"]}`.
- The runner's filter struct accepted only string values.
- The planner then tried to abandon the typed contribution node and fall back
  to a custom script, which weakens the DAG and reintroduces broad-script risk.

This is a generic schema compatibility gap. Structured data tools must accept
reasonable JSON shapes for typed parameters, especially where the schema
semantics already imply lists. Otherwise every harmless shape drift pushes the
model toward bespoke scripts.

### Batch 29: Contribution Filter Shape Compatibility

- [x] Extend `compute_contributions` filter parsing so `filters_json.value`
      accepts strings, scalar JSON values, and arrays of scalar JSON values.
- [x] Normalize array filter values to the existing comma-separated `in`
      representation, preserving deterministic runner semantics.
- [x] Keep unsupported nested/object filter values fail-loud with a typed
      diagnostic.
- [x] Add regression coverage for `op=in` with a JSON array value.

### Remaining Follow-Ups

- [ ] Add similar typed-shape compatibility audits for other data action
      params, especially entity candidates and rule drafts.

## 2026-06-06 Twenty-Fifth Real-Scenario Audit

After Batch 29, the real workflow progressed into multiple bounded action
batches. It still failed before producing a final answer, but the failure was
again a domain-neutral action-DAG contract gap:

- A terminal repair could declare required materials, while no current action
  was responsible for consuming one of those materials.
- The workflow only discovered the gap after execution, through cumulative
  coverage validation.

This is a scheduling contract problem. Terminal batches must prove, before
execution, that every required runner material is either already consumed by a
previous result or scheduled for consumption by the current batch. Merely
listing a path in `input_paths` is not enough when `actions[]` is present.

### Batch 30: Terminal Required-Material Scheduling

- [x] Add a terminal scheduling guard for required runner materials.
- [x] Treat prior result `ConsumedPaths` and action artifact source paths as
      completed consumption.
- [x] Treat current non-inventory action `input_paths` as scheduled
      consumption.
- [x] Keep non-terminal batches free to cover only the next DAG slice.
- [x] Preserve existing one-shot/batch-size guard priority so broad scripts
      still receive the most useful split-batch diagnostic first.
- [x] Add regression coverage for terminal batches that declare unscheduled
      required materials.

## 2026-06-06 Twenty-Sixth Real-Scenario Audit

After Batch 30, the same real workflow reached rule/ledger validation. The
final failure was:

```text
data validation incomplete: rule coverage is required with item ledgers,
but no decision/contribution/entity record links to a rule_id
```

The validation itself is correct. For rule-driven filtering, joining,
aggregation, ranking, or extraction, a result with disconnected rule coverage
and item ledgers is internally under-explained even when the numeric answer is
present. The deeper gap was action-graph usability:

- `derive_rules` can generate rule ids dynamically.
- A same-batch or later `compute_contributions` action often has no explicit
  `rule_refs` because the model has not seen the generated ids yet.
- The `compute_contributions` artifact was materialized as a bare JSON array,
  so follow-up custom nodes guessed the shape inconsistently.

The generic fix is to make typed action artifacts self-describing and to carry
safe lineage defaults across typed actions. This is structural provenance, not
business semantics.

### Batch 31: Typed Artifact Schema and Rule Lineage Defaults

- [x] Materialize `compute_contributions` outputs as self-describing JSON
      objects with `contributions`, `records`, `artifact`, `kind`, `id`,
      `source_paths`, and summary fields.
- [x] Update JSON action-record flattening to prefer standard collection keys
      such as `records`, `rows`, `contributions`, `items`, `data`, and
      `values`, so typed actions can consume self-describing artifacts without
      losing record semantics.
- [x] When rule coverage is required and a `compute_contributions` action has
      no explicit `rule_refs`, inherit currently derived rule ids from prior
      same-run rule artifacts. Explicit `rule_refs` remain authoritative.
- [x] Keep this inheritance limited to typed contribution actions; custom
      scripts still need to emit their own rule refs unless they rely on
      accumulated typed contribution records.
- [x] Add regression coverage for a
      `derive_rules -> compute_contributions -> reconcile -> custom_transform`
      graph where contributions inherit the generated rule id and a later
      custom node reads `artifact["contributions"]`.

### Remaining Follow-Ups

- [ ] Expose artifact shape previews more explicitly in continuation prompts:
      for every artifact alias, show whether `json_load(alias)` returns an
      object, a record array, and which standard collection keys are present.
- [ ] Add a graph-stage guard that discourages same-batch dependencies on
      dynamically generated identifiers when deterministic inheritance is not
      available.
- [ ] Add evaluator checks that flag silent material downgrades from required
      to optional/reference-only unless a typed coverage-intent reason and
      evidence are present.

## 2026-06-06 Twenty-Seventh Real-Scenario Audit

After Batch 31, a fresh real run still did not converge. The important failures
were domain-neutral and structural:

- The workflow repeatedly fell back from typed action DAG steps to large
  `custom_transform` scripts when the join/material relationship was unclear.
- A repair script referenced table fields that did not exist in the inspected
  CSV headers, for example using a derived field name instead of the actual
  source header. This should be caught before execution.
- Required directory materials such as material groups were declared as
  `script_consumed` in a terminal transform. A terminal script can consume
  concrete runner-readable files, extracted text, or distilled notes, but a
  directory-level required material is not a precise consumption contract.
- The repair prompt had enough high-level guidance, but the system did not
  convert known headers and material shape into hard structural constraints
  for later scripts.

The generic fix is not to encode any purchasing or vendor-specific rule. It is
to strengthen data-lane contracts:

- A custom script that iterates structured rows must reference fields that
  exist in the source material or a prior artifact.
- A terminal required material must have a verifiable consumption mode. Directory
  materials must first be expanded/profiled/extracted into concrete child
  materials or distilled into typed rules/constraints.
- Repair errors must include the script line and available fields so the model
  can correct the exact node instead of rewriting the whole graph.

### Batch 32: Custom Transform Field and Directory Contracts

- [x] Add pre-execution static validation for simple `csv_rows` / `tsv_rows`
      row-field references in `custom_transform` scripts.
- [x] Include source path, missing field, script line, and known headers in the
      typed diagnostic.
- [x] Teach `ClassifyExecutionError` to classify this as a data field contract
      violation and surface the model-authored script line as repair context.
- [x] Reject terminal `script_consumed` required directory materials for
      `custom_transform`; require concrete child material consumption,
      `text_evidence_consumed`, or `planner_distilled`.
- [x] Add regression coverage for both guards.

## 2026-06-06 Twenty-Eighth Real-Scenario Audit

After Batch 32, the workflow improved: it stopped trying to execute the initial
large script and successfully produced a first action batch with derived rules,
material profiles, and entity-resolution artifacts. The next failure exposed a
deeper DAG-runtime gap:

- Action artifacts were materialized only inside one `ActionRunner.Run` call and
  then cleaned up. A later data batch could see artifact metadata in
  `previous_data_rounds`, but could not reliably `json_load` the artifact.
- The continuation planner's `candidate_data_files` still listed only workspace
  files. Generated artifacts such as `cleaning_rules_result.json` and
  `entity_mappings_result.json` were not first-class candidate materials.
- Staging guards treated artifact source paths as covered, but not artifact ids,
  output aliases, or `artifacts/<alias>.json` paths. A real, generated artifact
  could therefore be reported as "not covered by prior typed actions/results".
- Coverage preservation could keep a stale, model-invented required material
  path alive across repairs even after the next plan tried to downgrade the same
  material path to optional/reference-only.

The generic fix is a Data Artifact Ledger. It is not domain-specific: every
typed action output becomes a durable, aliasable, read-only material for later
DAG batches, and coverage preservation respects explicit structured role
changes in the next plan.

### Batch 33: Cross-Batch Artifact Ledger

- [x] Persist data action artifacts under the configured data temp root instead
      of deleting them at the end of one action-run batch.
- [x] Add `artifact_path` and `artifact_aliases` metadata to every materialized
      action artifact.
- [x] Seed later `ActionRunner` batches with prior artifact aliases so
      `json_load("artifact_id.json")` and `json_load("artifacts/<id>.json")`
      can resolve across batches.
- [x] Treat artifact ids and aliases as covered materials in staging guards.
- [x] Add generated artifacts to continuation/repair candidate materials so the
      planner can see usable read-only JSON inputs.
- [x] Let a repaired plan move the same material path from required to optional
      or reference-only without the previous required role being reintroduced by
      merge logic.
- [x] Add regression coverage for cross-batch artifact consumption and
      coverage-role override.

## 2026-06-06 Twenty-Ninth Real-Scenario Audit

After Batch 33, the real workflow converged and emitted a strict single-line
answer. Manual audit still found a correctness gap: the final script consumed a
text rule material directly, but it freely reinterpreted the rules and omitted
some valid values from the rule text. The runner produced contributions and a
passing reconcile report, but the reconcile only proved internal arithmetic
consistency for the script's own interpretation.

The domain-neutral root cause is that "read a rules/instructions text file in a
script" is not equivalent to "turn the rules into auditable constraints and link
the item-level ledger to those constraints." This affects any data task driven
by textual instructions, policy docs, contracts, schemas, examples, or user
provided rule notes.

### Batch 34: Text Constraint Rule-Coverage Gate

- [x] Add a staging guard for terminal data calculations that consume required
      text/rule/instruction materials and emit decision/contribution/reconcile
      ledgers without `rule_coverage_required=true`.
- [x] Keep the guard structural: it keys on runner-readable text material file
      types, terminal calculation shape, and validation-ledger requirements,
      not on business keywords or user prose.
- [x] Classify the guard as
      `text_constraint_rule_coverage_required` with a repair hint to add
      `derive_rules` or emit linked rule coverage records.
- [x] Add regression coverage proving the guard fires only for the structured
      text-rule + audited-calculation shape and does not replace existing
      broad-transform prerequisite checks when rule coverage is already enabled.

### Remaining Follow-Ups

- [ ] Add artifact shape previews to continuation prompts so the model can
      reliably consume typed action artifacts without guessing JSON layout.
- [ ] Add more typed actions for generic join-candidate discovery and field
      mapping proposals, so uncertain joins can converge without a large script.
- [ ] Add evaluator checks that reject a zero/empty final result when there are
      no supporting contribution records for a task that requires contribution
      ledgers.
- [ ] Add rule-to-ledger semantic audits that compare derived rule coverage,
      item-level decisions/contributions, and final aggregate groups without
      relying on business-specific keywords.

## 2026-06-06 Thirtieth Real-Scenario Audit

The next real run did not stall in execution, but it spent the repair budget on
structural planning loops before producing any result:

- The model often emitted a whole intended DAG in one `actions[]` array. The
  system correctly rejected batches above the atomic action limit, but then
  asked the model to rewrite the whole plan. This made the model oscillate
  between "too many actions" and "custom transform too broad" instead of making
  deterministic forward progress.
- A same-batch custom transform that depended on earlier action outputs such as
  `vendor_resolutions.json` was reported as missing prerequisite coverage. The
  dependency checker marked earlier action input files as covered, but did not
  mark earlier `output_artifact` names or action ids as covered DAG materials.
- A repair plan could remove a previously required material to satisfy the
  terminal scheduling guard. That makes the staging guard pass by weakening the
  goal coverage instead of genuinely covering the material.

These are workflow-control issues, not purchasing-specific issues. They apply
to any data task with a multi-step graph: CSV/JSON aggregation, text-rule
driven filtering, cross-file joins, OCR/text extraction, strict-output
rendering, or mixed material transformations.

### Batch 35: Deterministic Atomic Batch Convergence

- [x] Normalize oversized `actions[]` plans by trimming to the next executable
      atomic prefix and setting `continue_after=true`, preserving the global
      coverage contract for later batches.
- [x] Treat same-batch earlier action ids and `output_artifact` aliases as
      covered materials for later action dependency checks.
- [x] Classify terminal required-material scheduling failures as
      `terminal_required_material_not_scheduled` so repair cannot silently drop
      required materials.
- [x] Add regression coverage for atomic batch trimming, same-batch artifact
      dependencies, and required-material preservation.

## 2026-06-06 Thirty-First Real-Scenario Audit

After Batch 35 the workflow made clear forward progress: it derived rules,
inspected materials, ran a bounded transform, produced a strict single-line
answer, and the evaluator marked the result complete. Completion validation
then correctly rejected the result because the plan required an entity
resolution ledger but the result did not include one.

The follow-up behavior exposed two generic gaps:

- The completion repair path sent the entire problem back to the planner. The
  planner rewrote the calculation instead of adding the single missing ledger.
- A later repair attempted `compute_contributions` over a derived field
  (`amount_integer`) that did not exist in any input record. Typed
  contribution actions are reliable only over existing fields; derived fields
  must first be materialized by a prior bounded node.
- Terminal transforms could generate local rule IDs and link contributions to
  those local rules, while ignoring source-backed rules derived from the
  actual rule material. That gives the appearance of auditability without
  proving that item ledgers followed the source rules.

These are structural convergence and provenance issues. They are independent
of procurement, money, or any specific file name.

### Batch 36: Required-Ledger Completion and Rule Provenance

- [x] Classify `missing_required_ledger` with a structured JSON path such as
      `/rule_coverage`, `/contributions`, `/entity_resolutions`, or
      `/reconcile`.
- [x] Add deterministic required-ledger completion for safe cases:
      `derive_rules` for missing rule coverage, `normalize_entities` for
      missing entity-resolution ledgers, and `reconcile_artifacts` for missing
      reconcile ledgers when contributions already exist.
- [x] Preserve the previous computed answer when a later action batch only
      augments ledgers/artifacts. Ledger completion must not force the model to
      recompute or replace the user-facing answer with an artifact summary.
- [x] Require item ledgers to link to source-backed rule coverage when such
      source-backed rules are available, preventing a terminal transform from
      self-certifying with only local rule IDs.
- [x] Classify `compute_contributions` field-contract failures separately from
      generic action failures so repair knows that a derived field must be
      materialized first or handled by a bounded transform.
- [x] Add regression coverage for answer-preserving ledger completion,
      deterministic entity-ledger completion plans, and source-backed rule
      linkage.

### Remaining Follow-Ups

- [ ] Add a typed field-derivation/materialized-record action for reusable
      normalized fields before `compute_contributions`. Today, derived fields
      still require a bounded `custom_transform`.
- [ ] Add provenance-aware completion for contribution and decision ledgers.
      These cannot be synthesized safely by the system because they encode
      task semantics; repair must be a focused bounded node with precise typed
      violations.
- [ ] Improve prompt ergonomics for source-backed rule IDs so the planner can
      link item ledgers to derived rules without loading large rule materials
      into context.

## 2026-06-06 Thirty-Second Real-Scenario Audit

After Batch 36 the same real task reached a correct, reconciled strict
single-line answer. The evaluator marked the result complete. The workflow then
continued because a required material was still missing from the consumed
material set. A focused material-coverage repair consumed that material, but
the repair introduced auxiliary `all/count` contribution records. The final
reconcile gate then treated those auxiliary counts as business metric
contributions and required them to appear in `reconcile.groups`, which pushed
the workflow back into a large-script repair loop.

This is a generic scope problem:

- Some contribution records explain the final answer metric and must be
  reconciled.
- Other contribution records are audit, sample, material coverage, diagnostic,
  or intermediate statistics. They are useful for traceability, but they must
  not expand the final answer's metric surface.
- Without a typed scope, late audit completion can pollute a correct result and
  force unnecessary recomputation.

This applies to any data task where the graph collects supporting evidence
after a computed answer: CSV/JSON aggregation, text extraction, OCR-derived
materials, web/material summaries, cross-file joins, and strict-output
rendering.

### Batch 37: Target-vs-Audit Contribution Scope

- [x] Add a generic `role` field to contribution records. `role=target` marks
      final-answer metric contributions; `role=audit`, `role=intermediate`, or
      related auxiliary roles mark traceability/supporting records.
- [x] Teach the runner helpers and planner prompt to use the role field without
      changing business semantics. Task-specific aliases remain non-authority;
      the role is structural metadata only.
- [x] Mark implicit `compute_contributions` material-count actions as
      `role=audit` when the model did not explicitly provide a metric, group,
      value field, or operation. Explicit metrics/groups remain target
      contributions by default.
- [x] Make reconcile validation require complete coverage for target
      contribution groups, while allowing auxiliary contribution groups to
      exist without forcing final metric groups. If auxiliary groups are
      explicitly reported, their numeric values are still checked.
- [x] Preserve the existing hard gate for missing target contribution groups.
      This keeps numeric answer validation strict while preventing audit
      records from widening the output contract.
- [x] Add regression coverage for implicit audit-count records and for
      auxiliary contributions that must not force extra reconcile groups.

### Remaining Follow-Ups

- [ ] Add a focused required-material impact action: when a correct reconciled
      answer exists but a required material is unconsumed, read and classify
      that material as "no answer impact" or "requires recompute" before
      asking for a new transform. This keeps coverage repair from drifting into
      broad recomputation.
- [ ] Add field-derivation/materialized-record nodes so reusable derived
      fields can feed `compute_contributions` without custom scripts.
- [ ] Continue expanding domain-neutral eval cases for auxiliary audit
      statistics, target-only reconciliation, and late material coverage.

## 2026-06-06 Thirty-Third Real-Scenario Audit

The next real run showed another generic convergence problem. Multiple repair
turns failed on minor schema drift such as a model-authored transform reading
`amount` while the objective CSV header was `amount_raw`. The runner already
had precise field-contract diagnostics, but a pure "rewrite the script" repair
loop made the model spend several turns on the same local mismatch.

The safe generic fix is not to guess business fields. It is to support
unambiguous structural aliases:

- exact header names always win;
- a requested field may resolve to a single header with a neutral suffix such
  as `_raw`, `_value`, or `_text`;
- semantic suffixes such as `_code` are not stripped, because mapping
  `category_code` to `category_raw` requires task logic or taxonomy data and
  must remain explicit.

This benefits any structured data task where raw/source columns carry common
suffixes while preserving correctness for fields that require semantic
translation.

### Batch 38: Unique Loose Field Alias Support

- [x] Add action-runner field-contract support for unique neutral suffix
      aliases. A script reference to `amount` no longer fails preflight when
      the only compatible header is `amount_raw`.
- [x] Keep unsafe semantic mappings rejected. `category_code` does not resolve
      to `category_raw`; the model must use an explicit mapping/action.
- [x] Make CSV/TSV/JSONL helper rows use the same unique alias logic at
      runtime so execution matches action preflight.
- [x] Add regression tests for accepted neutral aliases and rejected semantic
      aliases.

### Remaining Follow-Ups

- [ ] Extend the same row wrapper to JSON arrays and nested records where the
      structure is objectively row-like.
- [ ] Add a typed field-derivation/materialized-record action so semantic
      mappings such as raw label to canonical code can be represented without
      one-off scripts.

## 2026-06-06 Thirty-Fourth Real-Scenario Audit

The next real run showed that the adaptive DAG could still "forget" useful
work. A batch may contain several atomic actions. If an early action succeeds
and a later action fails, the old runtime returned only the error. Successful
artifacts, consumed material paths, rule records, contribution records, and
entity resolutions from the earlier actions in the same batch were not added to
the workflow records.

That made later repair/continuation turns see an emptier graph than reality:

- prerequisite guards concluded that broad fallback transforms still had no
  prior material coverage;
- prompts lacked the generated artifacts that should guide the next atomic
  action;
- the model was nudged back toward re-reading materials or writing another
  broad transform instead of continuing from the successful subgraph.

This is a generic DAG-runtime problem, not a data-domain problem. In a
goal-directed workflow, a failed node must not erase successful predecessor
nodes from the handoff.

### Batch 39: Partial Action Result Handoff

- [x] Make `ActionRunner` return a partial `Result` alongside an action error
      when earlier actions in the same batch succeeded.
- [x] Preserve only structural handoff signals: artifacts, consumed material
      paths, rule coverage, contributions, entity resolutions, reconcile
      state, metrics, and audit summary. A partial result is never treated as a
      final answer by itself.
- [x] Make CLI data workflow append failed records with the partial result
      when such signals exist, so later repair/continuation planners can see
      real progress.
- [x] Make REPL data workflow use the same partial-result handoff behavior.
- [x] Add regression coverage where an early atomic action succeeds and a
      later action fails; the returned error must still carry the early
      artifact and consumed material path.

### Remaining Follow-Ups

- [ ] Add a deterministic continuation fallback that can expand a broad
      fallback transform into typed prerequisite actions when the planner keeps
      returning a rejected monolith despite available partial artifacts.
- [ ] Add action-level progress summaries to CLI stderr and REPL logs that show
      partial artifacts preserved after a batch failure.

## 2026-06-06 Thirty-Fifth Real-Scenario Audit

After Batch 39, partial action results were visible to later repair turns. The
next real run progressed further, but a new contract-boundary bug appeared:

- an earlier action produced reusable generated artifacts such as extracted
  record JSON;
- those artifacts were correctly exposed to the planner as candidate inputs;
- a later repair plan then declared the generated artifact alias as a
  `coverage_contract.required_materials` entry with
  `usage_mode=script_consumed`;
- workflow completion rejected the result because that generated artifact
  was not consumed as if it were an original user material.

This is a generic DAG state layering problem. Action artifacts are internal
workflow state. They may be read by later actions, but they are not themselves
user/source materials that the final material-coverage gate must prove. The
coverage contract should track objective materials and text evidence; the
artifact bus should track reusable intermediate state.

### Batch 40: Generated Artifact Coverage Boundary

- [x] Keep generated action artifacts visible as candidate files so later
      actions and transforms can consume aliases such as `records` or
      `artifacts/records.json`.
- [x] Build a workflow generated-artifact alias set from previous action
      results, including artifact ids, alias fields, and materialized artifact
      paths.
- [x] Filter generated artifacts out of workflow `required_materials` when
      they are declared as `script_consumed`. This prevents internal DAG state
      from widening the final source-material coverage contract.
- [x] Preserve original source materials and explicit text-evidence coverage.
      `text_evidence_consumed` still validates the source material through its
      extracted evidence path.
- [x] Apply the boundary after repair/continuation coverage merges so a later
      model turn cannot reintroduce generated artifacts as hard source
      obligations.
- [x] Add regression coverage proving that generated artifacts remain
      available to later actions while final required runner inputs only
      include objective source materials.

### Remaining Follow-Ups

- [ ] Add a first-class artifact-dependency contract distinct from material
      coverage. It should let plans say "this node consumes artifact X" without
      overloading source-material coverage.
- [ ] Teach continuation prompts to describe generated artifacts as workflow
      state and required materials as objective source/evidence coverage.

## 2026-06-06 Thirty-Sixth Real-Scenario Audit

After Batch 40, generated artifacts no longer widened workflow material
coverage. The next run progressed into a second batch and then failed on a
different generic artifact issue:

- a previous action produced a generated JSON artifact;
- a later `custom_transform` loaded that artifact and assumed it was an object
  with a `get(...)` method;
- the actual generated JSON was an array, so Python raised
  `AttributeError: 'list' object has no attribute 'get'`;
- the model then asked the user to clarify the generated artifact structure.

That question should never reach the user. Generated artifact structure is
system-owned workflow state. The system must expose enough objective shape
metadata and helpers for the planner to inspect or consume artifacts without
asking for human business input.

### Batch 41: Generated Artifact Shape Metadata and JSON Record Helper

- [x] Add `json_shape` metadata to materialized action artifacts. The shape is
      derived from the emitted JSON payload and summarizes array/object/wrapper
      structure without loading full payloads into prompts.
- [x] Surface `json_shape` in generated-artifact candidate samples so
      continuation and repair planners can choose the correct access pattern.
- [x] Add a sandbox helper `json_records(path)` that reads array JSON,
      wrapper-object JSON (`records`, `rows`, `items`, `data`, `rules`,
      `rule_coverage`, `contributions`, `entity_resolutions`, `children`), or a
      single object as a generic record list.
- [x] Update helper safety checks, unknown-helper hints, planner prompt, and
      repair prompt to teach `json_records(path)`.
- [x] Classify list/object `.get(...)` mismatches as `json_shape_mismatch`
      with a repair hint to use artifact shape metadata or `json_records(path)`
      instead of asking the user.
- [x] Add regression coverage for artifact shape metadata and
      `json_records(...)` over both generated array artifacts and wrapper JSON.

### Remaining Follow-Ups

- [ ] Add deterministic artifact inspection actions that can summarize
      materialized artifact payloads more deeply when `json_shape` is not
      sufficient.
- [ ] Add typed repair routing for `json_shape_mismatch` so it prefers a local
      transform-node repair before broad graph replanning.

## 2026-06-06 Thirty-Seventh Real-Scenario Audit

After Batch 41, generated artifact shape was visible to the planner, but the
real scenario still spent several rounds oscillating between broad
`custom_transform` plans and coverage repairs. The important observation is
systemic rather than business-specific:

- the workflow guard can deterministically identify missing prerequisite
  materials for a broad transform;
- the workflow guard can deterministically identify terminal required materials
  that have not been consumed by any typed action or script;
- however, the next step previously went back to the LLM repair planner, so the
  model repeatedly rediscovered the same structural issue and sometimes planned
  another broad transform instead of executing the next coverage node.

This is a generic DAG convergence gap. When the system already knows the
missing objective material paths, it should create the next bounded coverage
batch itself. The model should still decide business meaning and final
computation, but structural material coverage should not consume LLM repair
budget.

### Batch 42: Deterministic Coverage Expansion Fallback

- [x] Add a workflow helper that converts structural coverage gaps into the
      next atomic data batch:
      - text/constraint materials -> `derive_rules` when rule coverage is
        required;
      - structured materials (`csv`, `tsv`, `json`, `jsonl`) ->
        `extract_records`;
      - other materials -> `inspect_material`.
- [x] Use the helper for both terminal required-material scheduling gaps and
      broad `custom_transform` prerequisite gaps.
- [x] Mark the generated batch as `continue_after=true` so it is an
      intermediate DAG node rather than a final answer attempt.
- [x] Wire the fallback into both CLI and REPL data workflows before LLM repair
      is invoked, keeping CLI/REPL semantics aligned.
- [x] Add regression coverage for terminal required-material expansion and
      broad-transform prerequisite expansion without relying on any business
      filename or domain keyword.

### Remaining Follow-Ups

- [ ] Extend deterministic expansion to typed ledger gaps where a safe generic
      completion action exists, before invoking broad repair.
- [ ] Add a workflow-level artifact dependency contract so coverage expansion
      can distinguish objective source materials from internal DAG artifacts
      without relying on alias sets alone.

## 2026-06-06 Thirty-Eighth Real-Scenario Audit

After Batch 42, the workflow reached a real calculation batch. The partial
result already contained the expected strict output payload, but validation
failed because reconcile groups were modeled only as contribution-group checks:

- target contributions were grouped by output elements such as query ids;
- the result also emitted one final-output reconcile group containing the exact
  final answer string;
- the validator rejected that final-output group because it had no matching
  contribution group.

This is a generic schema gap. Some data tasks need per-group reconciliation;
others need final payload reconciliation for a composite string, object, or
ordered list. These are different scopes and should be represented explicitly.

### Batch 43: Answer-Scoped Reconcile Groups

- [x] Add `scope` / `role` fields to `reconcile.groups` so plans can declare
      `scope="answer"` or `role="answer"` for final-output reconciliation.
- [x] Keep ordinary reconcile groups strict: they must still match target
      contribution `group_key` + `metric` and their numeric sums.
- [x] Validate answer-scoped groups by requiring their expected/actual value to
      match the final result answer exactly.
- [x] Structurally infer answer-scoped reconciliation when a group expected or
      actual value exactly equals the final answer payload. This handles models
      that omit the scope field without depending on business filenames or user
      intent keywords.
- [x] Update planner teaching so future plans can use answer-scope explicitly
      for composite final outputs.
- [x] Add regression coverage proving ordinary missing contribution-group
      checks still fail, while answer-scoped reconciliation passes only when it
      matches the final answer.

### Remaining Follow-Ups

- [ ] Add a dedicated helper for answer-level reconciliation so scripts do not
      have to hand-build the group JSON.
- [ ] Teach the evaluator to prefer answer-scope reconcile for strict composite
      output formats when per-output-element group checks are not necessary.

## 2026-06-06 Thirty-Ninth Real-Scenario Audit

After Batch 43, the data workflow could complete and reconcile the final
answer shape, but a real run still produced a self-consistent wrong result.
The important systemic gap was material coverage drift across repair and
continuation:

- the user explicitly named several local candidate materials in the request;
- an earlier plan could include those materials;
- later repair or continuation plans were allowed to downgrade or omit some of
  them while still producing a valid-looking contribution/reconcile payload;
- the evaluator then saw a structurally consistent answer, but the calculation
  was based on an incomplete material set.

This is not specific to any procurement file. Any data task can be wrong if a
model quietly drops a user-named input such as `rules.json`, `measurements.tsv`,
`notes.pdf`, an attachment index, or a reference table during repair.

### Batch 44: User-Explicit Material Floor

- [x] Add a deterministic material-floor helper that scans the discovered
      candidate inventory against the raw user request using exact candidate
      path or basename-with-extension matches.
- [x] Promote exact user-mentioned candidate materials into
      `coverage_contract.required_materials` with a verifiable `usage_mode`.
      Text/structured materials are marked `script_consumed`; non-text
      materials with extracted text candidates are marked
      `text_evidence_consumed`.
- [x] Remove promoted materials from `optional_materials` and merge their
      runner-consumable paths into `input_paths`.
- [x] Apply the material floor to initial plans, repair plans, continuation
      plans, deterministic coverage-expansion plans, and ledger-completion
      plans in both CLI and REPL data workflows.
- [x] Keep the mechanism generic and precise: it does not match business
      concepts, column names, or fuzzy material descriptions; only objective
      candidate file identifiers can trigger the floor.
- [x] Add regression coverage for exact file-path/basename matching,
      non-text-with-text-evidence handling, fuzzy non-matches, and optional to
      required promotion.

### Remaining Follow-Ups

- [ ] Add a generic material-reference graph. When a required material is an
      index or manifest that contains path-like references to other discovered
      local materials, expose those referenced materials as candidate graph
      nodes for the next planner batch without assigning business roles.
- [ ] Add evaluator checks that distinguish “all explicitly named materials
      were consumed” from “all transitive referenced materials were considered”
      so future workflows can close both levels independently.

## 2026-06-06 Fortieth Real-Scenario Audit

After Batch 44, a real data run preserved and consumed all user-explicit
candidate materials, but still returned an incorrect final payload. The runner
had accepted a result where:

- target contribution records existed;
- ordinary reconcile groups were internally `pass`;
- the final `answer` was a numeric list that did not match those reconcile
  group values.

The evaluator noticed the contradiction in natural language, but eventually
accepted the result. This is a generic contract gap: once a data task asks for
both a final payload and a contribution/reconcile ledger, the system must
deterministically verify that the final payload is actually derived from the
ledger. The model should not be the final authority for this structural check.

### Batch 45: Final Answer vs Ordinary Reconcile Alignment

- [x] Add deterministic validation for numeric final answers against ordinary
      reconcile groups. When the answer is a single numeric value or a
      comma-separated numeric sequence and the reconcile groups contain the
      same number of ordinary numeric values, their multisets must match.
- [x] Keep the check domain-neutral: it does not assume query ids, monetary
      values, procurement data, or ordering semantics. It only compares generic
      numeric payload values with generic reconcile group actual/expected
      values.
- [x] Preserve answer-scoped reconcile semantics. A group explicitly scoped to
      the final answer still validates against the complete final payload and
      does not get reinterpreted as per-group reconciliation.
- [x] Add regression coverage proving ordinary reconcile groups can back a
      numeric list answer, and that a mismatched answer is rejected even when
      group-level reconciliation itself is marked `pass`.

### Remaining Follow-Ups

- [ ] Add a helper that renders strict final payloads directly from reconcile
      groups or contribution groups, so scripts do not hand-copy numeric
      sequences.
- [ ] Extend the deterministic alignment check to structured JSON/table
      outputs when the output contract exposes enough shape information.

## 2026-06-06 Forty-First Real-Scenario Audit

After Batch 45, the system correctly rejected a final answer that did not
match ordinary reconcile groups, but the next repair loop still failed to
converge. The generic issue was not the specific data domain. It was that
rule identity in the adaptive DAG was unstable:

- a `derive_rules` action could receive newline text such as
  `RULE_ID: rule text`;
- the runner converted those lines into generated ids like `rule_1`;
- a later action naturally cited the visible stable ids in `rule_refs`;
- validation then reported unknown rule ids, even though the graph did contain
  semantically related rule records.

This is a graph-contract problem. If a planner declares stable ids for generic
rules, later rows, contribution records, entity resolutions, and reconcile
records must be able to cite those ids without guessing internal generated
names. The solution must stay domain-neutral: stable ids are syntactic
identifiers, not business labels.

### Batch 46: Stable Rule IDs Across Adaptive DAG Nodes

- [x] Parse explicit rule ids from generic `derive_rules.params.rules` lines
      written as `ID: text`, `ID = text`, or `ID - text`.
- [x] Apply the same ID extraction to string entries inside `rules_json`.
- [x] Preserve explicit ids through `rule_coverage.rule_id` so downstream
      generic ledgers can cite them via `rule_refs`.
- [x] When a `derive_rules` action lists input materials and explicit rule
      records do not already carry evidence, attach the action input path as a
      source-backed evidence reference. This lets the graph prove the rule came
      from a real material without requiring business-specific line matching.
- [x] Update planner teaching so models know that stable generic rule ids
      should be declared once and cited exactly later.
- [x] Add regression coverage for explicit ID preservation and for a later
      `custom_transform` action referencing the derived rule id.

### Remaining Follow-Ups

- [ ] Add a rule-artifact helper or read-only generated JSON contract that
      makes available rule ids easier for custom transforms to consume without
      copying ids by hand.
- [ ] Add a typed repair path for unknown `rule_refs` that can distinguish
      safe structural id drift from genuine missing rule derivation.

## 2026-06-06 Forty-Second Real-Scenario Audit

After Batch 46, the workflow progressed further but still failed to converge.
The failures were generic adaptive-DAG issues:

- generated rows and JSON artifacts did not consistently expose source
  locator metadata, so custom transforms guessed fields such as
  `_source_index`;
- scripts that used helper-ledger APIs could still omit decision rows or rule
  support notes, leaving the validator to discover the gap only at the final
  result boundary;
- the same `custom_transform` node could fail repeatedly before the workflow
  was forced to expand into smaller typed actions, spending budget on the same
  free-form script shape.

These are not domain errors. They are structural convergence issues: the data
runtime must make generic evidence/locator fields available by construction,
must safely patch unambiguous ledger shape gaps, and must stop retrying the
same failed free-form node after a small number of typed failures.

### Batch 47: Helper Metadata, Ledger Patch, And Node Failure Fuse

- [x] Annotate records returned by `csv_rows`, `tsv_rows`, `json_records`, and
      `jsonl_rows` with generic source metadata such as `_source`,
      `_source_index`, `_row_index`, `_line`, and `_source_locator`.
- [x] Keep the metadata domain-neutral and helper-local. It does not invent
      business semantics, does not alter routing, and remains available only
      to data scripts that already read the material.
- [x] Make `add_rule_coverage` add a structural support note when a helper
      caller provides a rule id/text/status but no explicit evidence or note.
      Raw JSON results remain subject to validator checks.
- [x] Extend the deterministic data result patch engine so a contribution
      ledger can produce minimal include decision records when
      `decision_records_required=true` but `rows` is omitted. The patch does
      not change contribution values or final answers.
- [x] Add a workflow staging fuse for repeated `custom_transform` node
      failures keyed by typed `action_id|action_kind`. Once the same node has
      failed the configured threshold, the next plan must replace it with
      smaller typed actions or a genuinely narrow new transform.
- [x] Add regression coverage for row metadata, contribution-to-decision
      ledger patching, and repeated custom-transform fuse behavior.

### Remaining Follow-Ups

- [ ] Add a stricter typed-action planner affordance for derived fields. When
      a calculation needs fields that are not present in raw material, the
      system should steer toward a small enrichment artifact before final
      contribution/reconcile actions.
- [x] Add a generated-artifact shape catalog to planner prompts so models can
      consume prior artifacts through `json_records(...)` without guessing
      object-vs-array shape.
- [ ] Continue reducing reliance on terminal `custom_transform` for complex
      contribution computation by expanding typed contribution/action params
      rather than increasing script budgets.

## 2026-06-06 Forty-Third Real-Scenario Audit

After Batch 47, the real scenario showed that the adaptive DAG skeleton was
working, but convergence was still weak:

- the same broad free-form computation could return under a different
  `action_id`, bypassing the per-node failure fuse;
- planner repairs had to infer generated artifact shape from compact samples,
  and repeatedly treated array-shaped artifacts as dictionaries;
- the workflow spent multiple rounds on large custom transforms even after the
  failure class was clearly structural rather than business-specific.

This is a generic workflow-contract issue. The system must reason about failed
free-form script *classes*, not only individual node names, and generated
artifacts need an explicit access contract that models can consume without
guessing. The fix is deliberately domain-neutral: it does not know vendors,
orders, invoices, or any other business role.

### Batch 48: Workflow-Level Transform Fuse And Artifact Access Catalog

- [x] Add a workflow-level `custom_transform` failure-class fuse. Once broad
      custom transforms have failed repeatedly in the same data workflow, a
      new broad custom transform with a different `action_id` is rejected
      before execution.
- [x] Keep narrow custom transforms available. The fuse applies to broad
      transforms that read many materials or look like a whole-workflow
      calculation; it still allows a small transform over one known generated
      artifact.
- [x] Add `artifact_access` to the compact data result prompt view. It exposes
      generated artifact ids, aliases, JSON shape, source paths, and a generic
      access hint.
- [x] Teach planner prompts to treat `result.artifact_access` as the
      authoritative generated-artifact access catalog. Array-shaped artifacts
      should be read with `json_records(alias)` or iterated as lists, not
      accessed as dictionaries.
- [x] Add regression coverage proving a changed `action_id` cannot bypass
      repeated broad-transform failures, and proving continuation prompts
      include the generated-artifact access catalog.

### Remaining Follow-Ups

- [ ] Add a typed derived-field/enrichment action so models can create small
      reusable artifacts for fields derived from existing columns, text, or
      prior generated artifacts.
- [ ] Add a generated artifact shape/access section to REPL/CLI low-noise
      process output so human audit can see the same contract the model sees.
- [ ] Add a higher-level transform-class budget that prefers typed actions
      before the workflow reaches repair budget exhaustion.

## 2026-06-06 Forty-Fourth Real-Scenario Audit

After Batch 48, a fresh real run avoided the previous long custom-transform
loop, but exposed two generic structural gaps:

- a plan could carry the same short script both on a `custom_transform` action
  and at the top level. The runner correctly rejects `actions[] + top-level
  script`, but the duplicate form is a safe shape drift that the system can
  normalize without asking the model;
- a `custom_transform` may emit a useful generic object such as
  `{content, line_count}` while not setting the canonical `answer` field.
  The Result schema then drops the extra fields, so the next planner only sees
  an empty answer instead of a reusable material/artifact.

Neither issue is business-domain-specific. They affect any data task that uses
small custom actions to read rules, profile text, summarize schema, or produce
intermediate material.

### Batch 49: Duplicate Script Normalization And Emit Payload Preservation

- [x] Normalize duplicate `actions[] + top-level script` plans when the
      top-level script is equivalent to an existing action script after
      comment/blank-line removal. This removes a structural duplicate without
      changing executable semantics.
- [x] Preserve non-canonical fields emitted by data scripts as an
      `emitted_payload` artifact. This makes small generic custom actions
      reusable even when they emit `{content: ...}` or other intermediate
      payloads instead of a final answer.
- [x] Include payload keys, JSON shape, and a compact sample in the artifact so
      later planner turns can consume it through the same artifact access
      catalog.
- [x] Add regression coverage for duplicate script normalization and extra
      emit payload preservation.

### Remaining Follow-Ups

- [ ] Prefer typed `inspect_material` / `derive_rules` for rule/text reads
      before allowing a custom action whose only purpose is to mirror text.
- [ ] Add evaluator guidance that an empty `answer` can still be useful when
      the result contains non-final artifacts, and should not be treated as a
      failed read by itself.

## 2026-06-06 Forty-Fifth Real-Scenario Audit

After Batch 49, another real run exposed two more schema-shape drift issues
that are independent of the procurement scenario:

- the model sometimes compressed a list of file paths into one
  comma-separated string inside `input_paths`, `required_materials.path`, or
  `required_materials.text_evidence_path`. The runner then tried to open the
  whole comma-joined value as one path;
- generated artifacts were advertised with flat aliases such as
  `artifacts/<artifact>.json`, while a natural model-produced reference can
  also be `artifacts/<action>/<artifact>.json`.

Both are structural representation problems. The system can repair them
deterministically without interpreting business semantics or changing any
computed values.

### Batch 50: Path-List Shape Repair And Namespaced Artifact Aliases

- [x] Normalize comma-separated path-like strings in data task plan fields
      back into arrays before staging/execution. The splitter is conservative:
      it only splits when every comma-separated token looks path-like, so
      prose containing commas remains intact.
- [x] Apply the same normalization to coverage materials, including paired
      `path` and `text_evidence_path` values, while keeping material metadata
      domain-neutral.
- [x] Extend generated artifact aliases to include
      `artifacts/<action>/<artifact>` and
      `artifacts/<action>/<artifact>.json` forms.
- [x] Add regression coverage for comma-separated path-list repair, prose
      non-splitting, paired coverage material expansion, and action-namespaced
      artifact consumption.

### Remaining Follow-Ups

- [ ] Add a planner-visible warning when a previous repair normalized path
      lists, so the model learns to keep arrays as JSON arrays in subsequent
      batches.
- [ ] Consider exposing generated artifact aliases in a more compact table in
      the REPL/CLI process output for human audit parity with the prompt.

## 2026-06-06 Forty-Sixth Real-Scenario Audit

After Batch 50, the workflow still showed a deeper architectural gap: the
model repeatedly returned to broad `custom_transform` scripts because the
typed action set could filter and aggregate records, but could not create a
generic joined record set. Any task requiring multi-table association therefore
had no reliable atomic path before `compute_contributions`.

This is not a procurement-specific issue. Many data tasks need the same shape:
join two materials by one or more fields, produce an auditable intermediate
record artifact, then filter/group/sum/count/reconcile it. Without this
primitive, the system overuses free-form scripts and repair loops.

### Batch 51: Generic `join_records` Atomic Action

- [x] Add `join_records` as a first-class `DataActionKind`.
- [x] Implement deterministic read-only joins over two structured inputs.
      The action supports `input_paths` or `left_path`/`right_path`, plus
      `left_fields`/`right_fields` for single-field or composite joins.
- [x] Preserve source lineage in joined rows through `_left_source`,
      `_left_line`, `_right_source`, and `_right_line` fields.
- [x] Materialize joined rows as a reusable JSON artifact that later
      `compute_contributions` actions can consume by alias.
- [x] Keep default field naming generic: non-conflicting fields keep their
      original names; collisions are prefixed rather than overwritten.
- [x] Teach the data planner schema/prompt and workflow repair hints to prefer
      `join_records` before falling back to broad custom transforms.
- [x] Add regression coverage for `join_records -> compute_contributions ->
      reconcile_artifacts` without any Python script.

### Remaining Follow-Ups

- [ ] Add more typed transforms that can derive simple fields from existing
      records without free-form scripts, such as date/year extraction,
      numeric normalization, and conditional field selection. These should
      remain generic and field-driven.
- [ ] Add planner/evaluator pressure to use `join_records` when prior failures
      show broad custom transforms are only doing record association.

## 2026-06-06 Forty-Seventh Real-Scenario Audit

After Batch 51, the real scenario produced the correct final scalar sequence,
but validation still rejected the result because the reconcile payload included
an additional `overall/query_count` group. The contribution ledger correctly
contained the target monetary groups; the extra group was an audit statistic,
not a target answer metric.

This gap is generic. Many data tasks include auxiliary checks such as row
counts, sample counts, material coverage counts, or answer-shape checks next
to target metric reconciliation. The validator should require target groups to
match target contributions, while allowing explicitly non-target audit groups
to coexist without changing the answer.

### Batch 52: Target-Only Reconcile Validation

- [x] Add `reconcileGroupParticipatesInTargetCheck` mirroring contribution
      role semantics.
- [x] Treat reconcile groups with role/scope such as `audit`,
      `diagnostic`, `coverage`, `sample`, `intermediate`, or `material` as
      non-target checks. They are preserved for audit but do not have to match
      target contribution group keys.
- [x] Keep answer-scoped groups validated against the final answer.
- [x] Keep default/target reconcile groups strict: if they do not match target
      contributions, validation still fails.
- [x] Add regression coverage proving audit groups do not block a valid
      contribution-backed answer, while the same group without an audit role
      still fails.

### Remaining Follow-Ups

- [ ] Teach planner prompts to mark auxiliary reconcile groups with role or
      scope explicitly, so fewer results need repair.
- [ ] Add compact REPL/CLI display for target-vs-audit reconcile group counts.

## 2026-06-06 Forty-Eighth Real-Scenario Audit

A later run exposed a control-plane robustness issue: the data evaluator
returned reasoning/prose but no required `emit_data_task_evaluation` tool
call. The data workflow treated that as a fatal error and exited, even though
the deterministic state was enough to continue conservatively.

This is a generic direct-LLM planner/evaluator failure mode. It is not related
to any data domain and should be handled like other structured tool-call parse
failures: compact repair first, deterministic fallback second.

### Batch 53: Data Evaluator No-Tool Recovery

- [x] Add a compact no-tool retry path for `EvaluateDataTask`. The retry
      prompt contains the original evaluation context plus bounded previews of
      the previous prose/reasoning response and asks for exactly one
      `emit_data_task_evaluation` call.
- [x] If the retry also returns no tool call, fall back to a conservative
      deterministic evaluation from workflow state instead of failing the
      whole data task.
- [x] Fallback policy is intentionally non-completing: errors become
      `repair_node`, artifact-producing rounds become `continue_transform`,
      and otherwise the workflow continues as `continue_data`.
- [x] Add regression coverage for no-tool retry success and repeated no-tool
      fallback.

### Remaining Follow-Ups

- [ ] Promote this direct-planner no-tool recovery pattern into a shared helper
      for operation/data/external-skill planners so every direct LLM lane gets
      the same repair and fallback behavior.

## 2026-06-06 Fifty-Fourth Real-Scenario Audit

After Batch 53, a full real data-task run still overused broad
`custom_transform` scripts for a generic reason: the DAG had `join_records`,
but no typed way to materialize canonical or derived fields before the join.
When a later join key was not present on the base record set yet, the model
fell back to one large script that performed mapping, joining, filtering,
contribution generation, reconciliation, and final rendering together.

This is not tied to any business domain. Spreadsheet, JSONL, web-extracted,
OCR-extracted, and multi-file data tasks all need the same pattern:
materialize a generic lookup/mapping result on base records before downstream
joins and aggregations.

### Batch 54: Generic `enrich_records` Atomic Action

- [x] Add `enrich_records` as a first-class `DataActionKind`.
- [x] Implement deterministic read-only enrichment over a base record set and
      one or more mapping/reference record sets.
- [x] Support exact and containment-based matching through typed parameters:
      `base_path`, `mapping_path`/`reference_path`, source/reference fields,
      value/target fields, optional filters, and `mapping_specs_json`.
- [x] Preserve lineage in enriched rows through source, line/index, status,
      evidence, and match summary fields.
- [x] Materialize enriched rows as reusable JSON artifacts for
      `join_records`, `compute_contributions`, `reconcile_artifacts`, or a
      later narrow `custom_transform`.
- [x] Teach planner schema/prompt and workflow repair hints that derived join
      keys should first be materialized with `enrich_records`; `join_records`
      should not be asked to join on fields that do not exist yet.
- [x] Add regression coverage for `enrich_records -> join_records ->
      compute_contributions -> reconcile_artifacts` without a large script.

### Remaining Follow-Ups

- [ ] Add more generic typed transforms for field derivation where
      enrichment is not enough, such as date component extraction, numeric
      normalization, conditional field selection, and projection. These must
      stay field-driven and domain-neutral.
- [ ] Continue reducing planner reliance on broad scripts by routing repeated
      script failures toward typed enrich/join/contribution/reconcile actions.

## 2026-06-06 Fifty-Fifth Real-Scenario Audit

The next real run exposed a different convergence gap. The workflow had
already produced material coverage artifacts for the required materials, yet
the planner/evaluator/fallback loop kept converting compute attempts back into
coverage or material discovery batches. The system had no explicit phase
state saying "coverage is sufficient; move to computation, contribution, and
reconciliation."

This is a control-plane issue, not a data-domain issue. Any complex data task
can fail the same way if the planner receives compact previews and mistakes
"not computed yet" for "not read yet".

### Batch 55: Workflow Phase State And Coverage-Loop Guard

- [x] Add a domain-neutral `workflow_state_json` view for data continuation
      and evaluation prompts. It reports required material coverage, missing
      materials, rule coverage, decision records, entity resolutions,
      contribution records, reconcile presence, answer presence, and the
      recommended next workflow stage.
- [x] Add a staging guard that rejects coverage-only batches after required
      runner materials are already covered. The guard asks for compute-stage
      atomic actions (`normalize_entities`, `enrich_records`, `join_records`,
      `compute_contributions`, `reconcile_artifacts`) or one narrow
      transform over generated artifacts.
- [x] Prevent material-discovery fallback from stealing a plan after material
      coverage is already sufficient.
- [x] Add pre-execution typed action guards for empty `input_paths` on actions
      that cannot run without concrete materials or generated artifact aliases.
- [x] Add regression coverage for empty typed action plans, coverage-only loop
      rejection, and disabled material-discovery fallback after sufficient
      coverage.

### Remaining Follow-Ups

- [ ] Add a deterministic next-step scaffold when coverage is sufficient and
      the planner still cannot produce a compute-stage action after repeated
      attempts. The scaffold should be typed and generic, not a domain script.
- [ ] Add richer phase metrics to REPL/CLI progress output, for example
      "materials covered / contributions missing / reconcile missing", while
      keeping stdout clean in CLI mode.
- [x] Continue building typed field-derivation actions so compute-stage plans
      can avoid free-form scripts even for messy semi-structured tasks.

## 2026-06-06 Fifty-Sixth Real-Scenario Audit

After the workflow moved out of material discovery, the next failure mode was
still generic: compute-stage plans needed fields that did not exist yet, such
as date components, parsed numeric values, normalized strings, or mapped
labels. Without a field-level typed action, the planner fell back to broad
`custom_transform` scripts to derive those fields before filtering, joining,
and aggregating.

This is not tied to one business dataset. Any data task can need task-specific
fields derived from existing records before a generic contribution or join
action can run.

### Batch 56: Generic `derive_fields` Atomic Action

- [x] Add a domain-neutral `derive_fields` action kind.
- [x] Implement deterministic field derivation over existing CSV/TSV/JSON/
      JSONL/text action records.
- [x] Support generic operations only: copy, trim, lower, upper,
      regex_extract, regex_replace, parse_number, map/lookup, substring,
      prefix, suffix, year/extract_year, constant, and optional numeric
      multiplier/divisor scaling.
- [x] Keep examples and tests field-neutral. `derive_fields` does not know
      about domain notions such as dates, money, status, invoices, or purchase
      orders; it only transforms existing fields into derived fields requested
      by the current task.
- [x] Materialize the derived records as reusable JSON artifacts with source
      lineage and artifact aliases for later `join_records`,
      `enrich_records`, `compute_contributions`, or bounded transforms.
- [x] Teach the data planner schema and prompts when to use `derive_fields`
      before joins, filters, or aggregations.
- [x] Update workflow guard hints so repeated script failures are steered
      toward typed field derivation instead of another broad script.
- [x] Add regression coverage for `derive_fields -> compute_contributions ->
      reconcile_artifacts` without a free-form script.

### Remaining Follow-Ups

- [ ] Add a deterministic next-step scaffold when coverage is sufficient and
      repeated attempts still cannot produce any compute-stage action.
- [ ] Add richer phase metrics to REPL/CLI progress output, for example
      "materials covered / contributions missing / reconcile missing", while
      keeping stdout clean in CLI mode.
- [ ] Continue reducing terminal `custom_transform` reliance by adding more
      typed actions for projections, record reshaping, and structured final
      answer assembly.

## 2026-06-06 Fifty-Seventh Real-Scenario Audit

The next run showed an important shift: the adaptive DAG no longer got stuck on
large script retries. It executed field derivation, entity normalization,
record enrichment, material coverage expansion, and a final transform. The
final answer was still wrong.

The failure class is generic:

- the final transform produced an answer, contribution records, and a reconcile
  report that were internally consistent;
- the evaluator therefore marked the batch complete;
- however, the contribution ledger mixed atomic sourced items with aggregate
  summary rows, and the reconcile pass only proved that the transform agreed
  with itself;
- rules/reference/evidence materials were not sufficiently projected into the
  final target contributions through rule refs, entity-resolution refs, or
  source-backed decision records.

This is a ledger semantics issue, not a domain-specific data issue. Any data
task with filtering, joining, ranking, or aggregation can produce a wrong but
self-consistent result if "target contribution" is allowed to mean either a
source item or an aggregate summary.

### Batch 57: Atomic Target Contribution Anchor

- [x] Strengthen contribution validation: contributions that participate in
      final reconciliation must be atomic sourced items and must include
      `source`, `source_locator`, or `evidence_refs`.
- [x] Keep `item_id` as an item identifier only; it no longer by itself proves
      the contribution has a source anchor for final reconciliation.
- [x] Teach the planner that aggregate summary rows belong in
      `reconcile.groups` or `role=intermediate/audit`, not as target
      contributions.
- [x] Add a typed violation code and repair hint for missing target
      contribution source anchors.
- [x] Keep existing sourced contribution tests passing.

### Remaining Follow-Ups

- [ ] Add a material-influence graph so required materials can be traced into
      final target contributions through rule coverage, entity resolutions,
      decisions, and generated artifacts.
- [ ] Add independent target-item coverage checks for tasks whose final output
      is keyed by a declared target set. The model should define the target set
      from the task/materials; the system should verify every target has an
      output/reconcile entry.
- [ ] Add rule-projection checks so source-backed rules/constraints are linked
      from affected decisions, contributions, or entity resolutions before a
      complex data task can complete.
- [ ] Continue replacing broad terminal transforms with typed projection and
      final-answer assembly actions.

## 2026-06-06 Fifty-Eighth Real-Scenario Audit

The next real run showed three more generic convergence gaps. None of them is
specific to one dataset or business domain:

1. A material-coverage batch produced a human-readable `answer`, and the
   workflow state treated it as if a final answer existed. The root cause was
   that the state machine used the current batch output contract; coverage
   batches often use `freeform`, while the user-level workflow contract may be
   strict `plain_single_line`, `csv_line`, `json_only`, etc.
2. A `derive_fields` action without a field specification reached the runner
   and failed there, which encouraged the model to repair by emitting a broad
   script. Typed action completeness must be checked before execution.
3. A `derive_fields` action was planned over multiple unrelated input schemas.
   Field derivation is a single-record-set operation; multi-source work must
   be split or preceded by a join/enrichment artifact.

The deeper pattern is that the adaptive DAG must enforce stage and action
contracts deterministically. Prompt guidance is useful, but the workflow
controller must prevent a later batch from collapsing several unfinished
validation stages into one script.

### Batch 58: Workflow Contract and Atomic Action Boundaries

- [x] Promote the output contract to workflow scope. The most specific
      user/workflow contract survives intermediate coverage or audit batches,
      so `freeform` summaries cannot satisfy a strict final answer contract.
- [x] Treat an `answer` as final only when it satisfies the workflow output
      contract and all required ledgers/reconcile artifacts for the current
      coverage contract.
- [x] Add regression coverage proving that an intermediate coverage summary is
      not a final answer, while a strict answer with required contribution and
      reconcile records is accepted.
- [x] Add a staging guard for incomplete `derive_fields` actions: they must
      provide `field_specs_json` or a single source/target/operation spec.
- [x] Add a staging guard that `derive_fields` may consume only one record set
      per action. Different schemas must be split into separate actions or
      joined/materialized first.
- [x] Add a workflow-stage guard: when multiple validation stages are still
      unfinished after material/rule coverage, one scripted
      `custom_transform` cannot cross all remaining stages. The planner must
      emit the next atomic stage and continue the DAG.

### Remaining Follow-Ups

- [x] Feed the workflow stage guard back into the planner as a typed
      `allowed_next_actions` view instead of relying on a failed plan to teach
      the next attempt.
- [x] Add deterministic material-set expansion handles for directories or
      material groups so the model does not write scripts just to enumerate
      related files.
- [ ] Add typed projection/final-answer assembly actions so strict final
      output can be produced from reconcile artifacts without a broad
      terminal script.
- [ ] Add a material-influence graph and target-set coverage checker so
      required materials and target output keys are traceable into final
      contributions and reconcile groups.

### Batch 59: Typed Next-Action Contract

Real-data runs showed that the planner could understand
`workflow_state_json.next_stage` in prose yet still try an action from a later
stage, forcing the system to reject and re-prompt. This was a workflow
contract problem, not a domain-specific data problem.

The generic fix is to make the next legal DAG actions explicit:

- [x] Extend `workflow_state_json` with `allowed_next_actions`, derived from
      the same deterministic `next_stage` used by the evaluator.
- [x] Teach continuation prompts that `allowed_next_actions` is the structural
      contract for the next bounded batch.
- [x] Add a workflow staging guard: after at least one data batch exists, an
      action outside the current `allowed_next_actions` set is rejected before
      execution with a typed repair hint.
- [x] Keep the guard scoped to the data workflow path only. Initial data
      planning, source-code analysis, trace/log analysis, write mode, and
      command operation paths do not consume this contract.
- [x] Add regression coverage for the exposed state and the guard that blocks
      jumping from `derive_rules` directly to `compute_contributions`.

This is intentionally domain-neutral. It does not know whether the task is
finance, inventory, logs-as-data, or document extraction; it only enforces that
the next batch advances the current data DAG stage.

### Batch 60: Action Boundary Contracts

The same real-data run showed that a model can obey the allowed action name but
still misuse the action shape, for example treating a same-record field
derivation action as a lookup/reference-table mapping action. This is not a
business-domain issue; it is a tool-contract clarity issue.

- [x] Extend `workflow_state_json` with
      `allowed_next_action_contracts`. Each contract carries the action kind,
      input boundary, use case, and expected output.
- [x] Teach continuation prompts to read these contracts before writing action
      params. The prompt now explicitly separates same-record derivation,
      entity normalization, enrichment, joins, contribution calculation,
      reconciliation, and final projection.
- [x] Strengthen the `derive_fields` multi-input staging error so the repair
      path points to `normalize_entities`, `enrich_records`, or `join_records`
      instead of retrying the same invalid shape.
- [x] Keep these contracts as data-workflow metadata only; they are not used by
      source-code, trace/log, write-mode, or command-operation routing.

This moves one more class of repairs from "fail first, then explain" to
"plan with the action boundary visible up front".

### Batch 61: Intermediate Transform Role

The next real-data run showed that the cross-stage `custom_transform` guard was
too coarse. A model may need one bounded Python transform to materialize a
reusable intermediate record set when typed actions cannot express that exact
projection yet. That is different from a terminal all-in-one transform that
computes, reconciles, and assembles the final answer in one batch.

- [x] Treat a single scripted `custom_transform` with `continue_after=true` as
      an intermediate artifact node. It can produce one reusable artifact or
      one ledger slice and then return to the evaluator.
- [x] Continue rejecting terminal all-in-one custom transforms when multiple
      validation stages remain unfinished.
- [x] Continue rejecting batches that combine an intermediate scripted
      `custom_transform` with later compute/reconcile actions in the same
      batch.
- [x] Ensure `continue_after=true` batches cannot satisfy final-answer
      detection even if the intermediate runner result contains an answer-like
      summary.
- [x] Teach the data planner this distinction in the continuation prompt.
- [x] Add regression coverage for all three boundaries above.

This keeps the DAG adaptive: the system can accept one carefully scoped
intermediate transform, inspect its artifact, and only then plan compute,
reconcile, and final projection.

### Batch 62: Prompt-Facing Material Set Handles

The real-data run also showed that material coverage could be marked sufficient
for explicitly listed files while the model still had to reason about related
material groups such as extracted text, document shards, or directory-based
evidence. The system should not decide the business role of those groups, but it
can expose objective handles so the planner can expand the relevant concrete
members in a bounded next batch.

- [x] Add `result.material_set_handles` to data workflow prompt records.
- [x] Generate handles from existing objective artifact metadata: directory
      groups and related text evidence discovered by material
      inventory/inspection.
- [x] Keep handles advisory and concrete. They contain paths and access hints;
      they do not label a file as an invoice, contract, rule, or any other
      business role.
- [x] Teach the planner to expand only the concrete `member_paths` or
      `text_evidence_paths` needed by the current data goal before compute.
- [x] Add regression coverage for directory handles and related-text handles.

This closes the first version of material-set handles without adding
domain-specific file roles. The remaining larger item is a material influence
graph that proves which material groups actually feed final contributions.
