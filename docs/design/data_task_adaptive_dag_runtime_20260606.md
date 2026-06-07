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

## Status Index

The checklist sections below are both an implementation ledger and an
architecture backlog. A large number of remaining `[ ]` items is expected at
this stage: many were intentionally left open because a later batch delivered a
narrow executable slice but not the whole architecture boundary.

Current shipped invariants:

- data tasks run as an adaptive DAG of bounded typed actions before falling
  back to custom transforms;
- material coverage, current-batch inputs, deferred DAG ranks, and generated
  artifact availability are no longer treated as one flat list;
- typed actions can produce material profiles, record artifacts, rule coverage,
  entity-resolution evidence, contribution records, reconcile reports, and
  strict answer projections;
- REPL and CLI expose data-lane progress and audit artifacts without putting
  process details into the final answer channel.

Partially shipped architecture themes:

- artifact schema contracts exist for hard gates, but are only beginning to
  move into a first-class workflow IR;
- action DAG scheduling exists in the CLI/REPL workflow loop, but the durable
  action graph, dependency snapshots, and resumable queues are still open;
- material coverage and current-batch inputs are split in execution, but the
  Material Graph is not yet a standalone reducer;
- ledger and reconcile checks exist, but Ledger Graph lineage and typed
  evaluator violation objects are still open;
- `custom_transform` is constrained and no longer the intended main path, but
  more typed actions and deterministic scaffolds are needed before it can stay
  a rare leaf fallback in complex customer runs.

Open backlog must be grouped under these few architecture axes rather than
accumulating one-off REPL guards:

- first-class `DataWorkflowState` IR and deterministic state reducer;
- Action DAG readiness, dependency, lineage, and resumability;
- Material Graph coverage, influence, promotion, and evidence scope;
- Ledger Graph completeness, de-duplication, reconcile, and final projection;
- typed evaluator violations and repair planning over IR, not prose;
- real-scenario eval suites with repeated-run volatility statistics and
  CI/status gates.

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

### Batch 6: Structured Action Parameters

- [x] Let action `params` carry structured arrays/objects in the tool schema
      instead of forcing every value to be a string.
- [x] Normalize structured param aliases into the runner's existing typed
      contracts, such as `field_specs -> field_specs_json`,
      `filters -> filters_json`, `mapping_specs -> mapping_specs_json`,
      `resolutions -> resolutions_json`, and `rules -> rules_json`.
- [x] Serialize structured params with `json.Marshal` instead of `fmt.Sprint`
      so arrays and objects remain valid JSON and do not become Go debug
      strings.
- [x] Teach the planner prompt to prefer structured params for arrays/objects
      and reserve `*_json` only for already serialized strings.
- [x] Add unit coverage for structured params at both the REPL planner
      conversion layer and the deterministic action runner layer.

Rationale: a data action parameter is often structurally rich: field specs,
filters, join key arrays, mapping specs, rule lists, or entity mappings. Forcing
these into nested JSON strings makes model repair spend budget on escaping
rather than on the user goal. This batch is domain-neutral: it improves any
data task that needs typed action parameters, including CSV/JSONL aggregation,
multi-file joins, text extraction, entity normalization, and strict output
projection.

### Batch 7: Workflow Contract and Dependency Convergence

- [x] Keep the workflow-level coverage contract rooted in durable task
      requirements. Failed planning/guard records must not add new model-authored
      required materials to the global contract. They may preserve validation
      ledger requirements and user-pinned material floors, but generated
      artifacts and speculative future inputs remain action dependencies, not
      root coverage requirements.
- [x] Gate deferred DAG nodes on dependency availability before scheduling them.
      A deferred action can run only when each declared input is either a
      covered source material or a generated artifact alias visible in the
      cumulative artifact catalog. If not, discard the stale deferred queue and
      let the evaluator/planner expand the graph from current facts.
- [ ] Canonicalize lookup/enrichment action roles before execution. The
      planner may say `base_path`/`lookup_path`, `base_fields`/`lookup_fields`,
      or older `mapping_path`/`mapping_source_fields`; the runner should
      normalize these into one role IR and validate against artifact fields.
      Role correction is structural only: it may swap base/reference artifacts
      when field evidence proves the roles are inverted, but it must not change
      business semantics or invented values.
- [x] Surface short workflow intent in process events. `◇ 数据工作流 · ...`
      should include compact goal/next-step/repair/continuation summaries in
      permanent lines, while full plans, scripts, and errors remain auditable in
      log files and data-audit artifacts. Final answers stay clean.

Rationale: the adaptive data DAG must converge by executing only ready nodes.
Generated artifacts such as joined tables or contribution tables are graph
dependencies, not source materials that should reopen the material-coverage
stage. This distinction is domain-neutral and applies to any data task with
intermediate artifacts: table joins, JSON transformations, extracted text
evidence, record expansion, entity normalization, contribution computation, and
final projection.

Status refresh: the coverage-contract split landed in the later material-floor
batches; deferred DAG readiness landed in the deferred queue/runtime guard; and
REPL/CLI process events now keep deterministic counters in the title line while
rendering goal, batch purpose, next step, and repair detail in low-noise detail
blocks. Lookup/enrichment role canonicalization remains open because the latest
real run still shows field-lineage drift between entity-resolution artifacts and
downstream enrichment.

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
- [x] Add typed projection/final-answer assembly actions so strict final
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

### Batch 63: Material Influence Edge and Typed Answer Projection

The latest real-data run exposed another generic DAG gap. A plan can list a
text/rule/constraint material at the workflow level while emitting a
`derive_rules` action without `input_paths`. The action then derives only from
generic validation text, and later workflow state treats the real material as
missing. This is a material influence edge problem, not a business-domain
problem: a required material must be connected to the typed action that is
supposed to consume or distill it.

The same run also showed that strict final output should not require a broad
terminal script once contribution and reconcile ledgers already exist. The
system should project the final answer from typed reconcile groups.

- [x] Normalize `derive_rules` actions that omit `input_paths` by filling
      them from required text/rule/constraint materials declared in the
      coverage contract. This uses objective material usage modes and file
      shape, not business labels or user-prose keywords.
- [x] Preserve the distinction between planner-distilled and script/action
      consumed materials. Only materials declared for runner/action
      consumption are auto-linked.
- [x] Add `assemble_answer` as a typed data action. It projects existing
      reconcile groups into the workflow output contract without changing
      business decisions, numeric values, or contribution membership.
- [x] Support generic projection modes: values, key-values, JSON groups, and
      Markdown table; support deterministic ordering and delimiter selection.
- [x] Add deterministic validation that the projected answer still satisfies
      the output contract and matches reconcile group values.
- [x] Teach the planner schema, prompt, and allowed-next-action contracts about
      `assemble_answer`.
- [x] Add regression coverage for required-material input normalization,
      assemble-before-reconcile rejection, and reconcile-group value
      projection.

Remaining larger architecture item: build a full material influence graph so
required materials can be traced through generated artifacts, rule coverage,
entity resolutions, decisions, contribution records, reconcile groups, and the
final projected answer.

### Batch 64: Script-Failure Cooldown for Atomic DAG Convergence

The same real run showed that, after material and rule coverage were fixed, the
planner could still fall back to a large `custom_transform` for normalization,
join, contribution calculation, reconcile, and final answer assembly. The
existing size guard rejected that script, but the next repair attempt could
emit another broad script with a different action id. This is a generic
workflow-control gap: if a script node fails, the next batch should not keep
retrying free-form code in the same stage.

The fix is a structural cooldown, not a prompt-only hint:

- [x] Add `custom_transform_failures` and `custom_transform_disabled` to
      `workflow_state_json`.
- [x] Derive the cooldown from typed workflow records: the latest relevant data
      event is a failed scripted `custom_transform`, and no later successful
      non-script typed action has advanced the graph.
- [x] Keep coverage/rule-only actions from prematurely releasing the cooldown.
      A successful `inspect_material`, `extract_records`, or `derive_rules`
      batch can cover prerequisites, but it does not prove the compute graph
      has advanced. The cooldown is released only by typed progress in
      derivation, normalization, enrichment, join, contribution, reconcile, or
      answer assembly stages.
- [x] Filter `custom_transform` out of `allowed_next_actions` and
      `allowed_next_action_contracts` while the cooldown is active.
- [x] Let the existing allowed-action guard reject another script before
      execution. This prevents id-renaming from bypassing the prior failure.
- [x] Teach continuation prompts that `custom_transform_disabled=true` means
      the next batch must move through typed atomic actions until a non-script
      action succeeds.
- [x] Add regression coverage proving the filtered state and guard behavior.

This remains domain-neutral. It does not forbid custom scripts globally; it
turns off the script fallback only after a script failure, and only inside the
data workflow path. Source analysis, trace/log analysis, command operation, and
write mode do not consume this state.

### Remaining Follow-Ups

- [x] Add the first compact typed-compute scaffold: generated artifact access
      now carries field-level contracts, and join artifacts preserve predicted
      output fields even when the current join returns zero rows.
- [x] Add compact action-shape skeletons for the current artifact fields. The
      workflow state now exposes neutral templates for `derive_fields`,
      `join_records`, and `compute_contributions`.
- [x] Include compact field-value samples in artifact access. The planner can
      now see a few objective examples for each generated artifact field
      without treating the sample as exhaustive data.
- [ ] Continue expanding scaffold quality for hard workflows. The next
      increment should rank skeletons by graph progress and prefer
      graph-progressing templates, while still avoiding business-specific
      decisions.
- [ ] Continue expanding typed compute actions so common record transforms can
      be represented as `derive_fields`, `normalize_entities`,
      `enrich_records`, `join_records`, and `compute_contributions` instead of
      needing broad Python fallback scripts.
- [ ] Add an evaluator-side budget signal for long LLM planning turns in data
      workflows. If no tool call arrives near timeout, retry with a compact
      workflow state rather than the full previous-round material history.

### Batch 65: Artifact Field Contracts for Typed Compute

The next real-data run showed a more precise generic gap. After material
coverage was complete, the planner correctly tried a typed
`derive_fields -> join_records -> compute_contributions` path, but it had to
guess which fields existed after each generated artifact. In particular,
`join_records` previously validated join keys against the union of both input
schemas. A field that existed only on the right input could accidentally pass
as a left key, producing an empty join instead of an early structural error.
After that, later actions guessed names such as collision-suffixed status
fields.

This is not a domain problem. Any data workflow that joins, enriches, filters,
or aggregates generated artifacts needs a field-level contract between
actions.

- [x] Make `join_records` validate left join fields against the left input and
      right join fields against the right input separately.
- [x] Keep zero-row join artifacts useful by emitting predicted output headers
      based on left/right headers, prefixes, and collision policy.
- [x] Store those predicted headers in artifact metadata so downstream
      planners can see the available field names even when a join produced no
      rows in the current sample/window.
- [x] Extend `result.artifact_access` with a compact `fields` list derived
      from artifact headers and action metadata.
- [x] Teach continuation prompts that typed actions must reference fields from
      `artifact_access.fields`; missing fields should be materialized first by
      `derive_fields`, `enrich_records`, or `join_records`.
- [x] Add regression coverage for side-specific join-key validation, zero-row
      join field contracts, and prompt-visible artifact fields.

Remaining architecture item: generate typed action-shape skeletons from these
field contracts. The field list is now available; the next step is to propose
small valid action templates such as "derive field X from artifact A" or
"compute contributions over artifact B using fields ..." without hard-coding
any business semantics.

### Batch 66: Material Group Coverage Semantics

The next real-data run used a required material that represented a group of
files rather than one concrete file. The workflow expanded and consumed several
member files under that group, but coverage still compared the required path by
exact string and therefore kept reporting the group as missing.

This is also domain-neutral. Datasets often contain a directory of fragments,
pages, extracted text, shard files, logs, OCR outputs, or generated evidence
records. Once the planner declares that the group is required and the workflow
consumes concrete members, the coverage contract should recognize that the
group has been reached. Otherwise the DAG keeps returning to coverage instead
of moving to transform/compute stages.

- [x] Normalize covered material paths before comparing them with required
      runner inputs.
- [x] Keep exact file coverage strict for paths with an extension.
- [x] For extensionless required paths, treat consumed member paths under
      `required_path/` as coverage for that material group.
- [x] Reuse the same coverage predicate in both workflow-state computation and
      custom-transform prerequisite checks so UI, planner state, and guards do
      not diverge.
- [x] Add regression coverage for a directory-like material group and for an
      unrelated similarly named file path.

Remaining architecture item: material-set handles should eventually expose a
stable group id and member count in addition to path-prefix behavior. That
would allow stronger coverage accounting for non-directory logical groups, for
example pages selected from a PDF or rows selected from a table.

### Batch 67: Compact Typed Action Scaffold

The material-group and field-contract fixes moved the real-data workflow into
the compute stage, but the model still spent a long planning turn translating
artifact schemas into valid typed actions. The system already knew the
available artifact aliases and fields; it should expose compact action
skeletons so the planner can adapt a valid shape instead of mentally rebuilding
the graph.

This is structural help only. The scaffold does not decide which field is
semantically correct for a user's business rule.

- [x] Add `workflow_state_json.action_scaffold`.
- [x] Generate neutral `derive_fields` templates for one-artifact field
      derivations.
- [x] Generate neutral `join_records` templates for artifact pairs that share
      at least one field name, with side-specific join guidance.
- [x] Generate neutral `compute_contributions` templates for artifacts that
      have fields, including placeholders for value/group/filter fields.
- [x] Keep templates bounded and field-backed. They use only artifact aliases
      and `artifact_access.fields`; they do not infer business meaning from
      field names.
- [x] Teach the continuation prompt to prefer adapting `action_scaffold` over
      inventing fields or writing broad scripts.
- [x] Add regression coverage for scaffold emission.

Remaining architecture item: rank scaffold options by expected graph progress
so the model sees the most graph-progressing templates first.

### Batch 68: Objective Field Samples for Artifact Access

Real workflows still wasted planning turns after the field contract was
available, because the planner could see field names but not a bounded example
of their values. That made it harder to choose generic filter/group/value fields
for typed actions without falling back to a broad script.

The fix is not to teach any business-specific field. The system now exposes a
small, objective `field_samples` map next to `artifact_access.fields`. Samples
are copied only from already-generated artifact samples; they are bounded,
deduplicated, and treated as examples rather than complete data.

- [x] Add `artifact_access.field_samples` to continuation/evaluation prompt
      state.
- [x] Derive samples from generated artifact `Sample` JSON rows only; do not
      read additional files or infer semantic roles.
- [x] Keep the sample map bounded by field count and value count.
- [x] Teach the data planner that `field_samples` are examples, not exhaustive
      values, and that typed actions must still reference fields from
      `artifact_access.fields`.
- [x] Add prompt regression coverage for visible field samples.

Remaining architecture item: rank scaffolds by expected progress and attach
the most relevant sample-bearing artifacts first. Ranking must remain structural
(graph state, available ledgers, action success/failure), not based on
hard-coded task keywords.

### Batch 69: Required-Material-First Coverage Fallback

A real workflow showed that the deterministic conversion from a broad transform
to an atomic coverage batch could under-cover required materials. The old
fallback treated material paths listed in the not-yet-executed current plan as
"scheduled" and therefore omitted them from the replacement coverage batch. If
that broad plan was replaced before execution, those materials were never
actually consumed, so the evaluator had to send the workflow back through
coverage repair.

This is a DAG scheduling bug, not a task-specific mistake. A replacement node
must be built from already observed graph state, not from intentions in the
node it is replacing.

- [x] Compute coverage-fallback missing materials from the merged workflow
      coverage contract and already covered material paths.
- [x] Do not count input paths from the current unexecuted plan as covered or
      scheduled for the replacement fallback.
- [x] Keep previously covered result artifacts and consumed paths as valid
      coverage facts.
- [x] Do not steal narrow executable data scripts. Coverage fallback is used
      only when the current plan structurally needs decomposition, for example
      oversized/complex scripts, unscheduled terminal requirements, or broad
      custom transforms over uncovered prerequisites.
- [x] Add regression coverage for intermediate broad plans with
      `continue_after=true`: required text/rule materials and structured
      materials are both emitted into the atomic coverage batch before compute.
- [x] Update existing expectations so replacement fallback covers all still
      missing required materials, not just materials absent from the current
      plan.

Remaining architecture item: when the missing list is very large, split
coverage into prioritized batches based on structural workflow needs
(required-runner inputs first, then optional/reference groups), while keeping
the split deterministic and business-agnostic.

### Batch 70: Safe Typed-Action Parameter Normalization

The next real-data run reached typed compute planning and exposed a smaller,
but important, schema-drift class. The planner emitted a `derive_fields` map
spec whose `mapping` value was a JSON object encoded as a string. The meaning
was unambiguous, but the action runner rejected it before execution and spent a
repair turn asking the model to rewrite the plan.

This should be handled by the same generic principle as result patching:
normalize safe structure only, never business semantics. A JSON string that
decodes to an object is a structural representation issue. A non-object string,
or any value that would require deciding a rule, amount, entity, filter, or
include/exclude choice, must remain a validation error.

- [x] Let `derive_fields` accept `mapping` either as an object or as a JSON
      string that decodes to an object.
- [x] Preserve the existing hard failure for empty strings, non-JSON strings,
      arrays, and other unsupported shapes.
- [x] Keep scalar values as strings after normalization; do not infer numeric
      or domain-specific types.
- [x] Add regression coverage for `mapping` supplied as a JSON object string.

Remaining architecture item: generalize typed-action parameter normalization
into a shared schema-aware layer for other action params (`filters_json`,
`field_specs_json`, mapping specs, join key lists) with typed violations and a
bounded repair budget. That layer must remain structural and must not infer
business meaning.

### Batch 71: Typed Aggregation Contract Backfill

Another real-data run showed that a model can correctly route a request as a
typed data aggregation while omitting `contribution_ledger_required` and
`reconcile_required` from the first data plan. If the system accepts that weak
contract, the workflow may conclude that material coverage is enough and move
straight to final-answer projection. The planner then sees only
`reconcile_artifacts` / `assemble_answer` even though no contributions exist.

This is a contract-shape problem, not a domain problem. The classifier already
emits a typed `data_task_kind` / `operation`; when that precise signal says
`data_aggregation` and the plan is structurally complex, the workflow should
require contribution and reconcile ledgers before final projection.

- [x] Add policy-aware data plan normalization.
- [x] Use only typed route policy (`data_task_kind` / `operation`) plus
      structural plan complexity (multiple materials, multiple inputs, or
      actions) to backfill aggregation ledgers.
- [x] Leave simple one-file data scripts and non-aggregation data tasks
      unchanged.
- [x] Extend action-implied contract normalization: `compute_contributions`
      implies contribution ledger; `reconcile_artifacts` / `assemble_answer`
      imply reconcile.
- [x] Route REPL and CLI initial, repair, continuation, and next-batch plans
      through the same policy-aware normalization path.
- [x] Add regression coverage proving complex typed aggregations get
      contribution/reconcile contracts while simple scripts do not.

Remaining architecture item: move more task contract inference into the typed
classifier / data planner schema so the planner can express aggregation,
projection-only, extraction-only, and summary-only goals explicitly. The
normalizer should remain a safety net, not the primary planner.

### Batch 72: Current-Stage Prefix Fallback

Real runs still spent repair turns when the planner emitted a useful typed
prefix for the current DAG stage followed by a scripted `custom_transform` that
crossed later validation stages. The guard was correct to reject the whole
batch, but the system could already identify the safe prefix.

The generic fix is to treat this as a DAG scheduling problem. If the workflow
has a successful prior result, the current stage exposes allowed action kinds,
and the plan starts with valid non-script typed actions, Codrax can execute
that prefix as the current bounded batch and leave the scripted suffix for a
future planner turn.

- [x] Add deterministic current-stage prefix fallback.
- [x] Stop the prefix before the first action outside
      `workflow_state_json.allowed_next_actions`.
- [x] Stop the prefix before a scripted `custom_transform` when multiple
      validation stages remain unfinished.
- [x] Preserve coverage contract, output contract, input paths, and batch
      context; set `continue_after=true` so the evaluator advances the graph
      after the prefix result.
- [x] Wire the fallback into both REPL and CLI before LLM repair.
- [x] Add regression coverage for trimming a typed `derive_fields` prefix
      before a scripted all-in-one transform.

Remaining architecture item: extend prefix fallback into a fuller plan-graph
scheduler that can split multiple valid typed prefixes across batches when the
planner emits a larger DAG. The first version remains intentionally
conservative and serial.

### Batch 73: Artifact Field Contracts And Enrichment Scaffolds

The procurement real-data run exposed a generic typed-action composition gap:
`normalize_entities` correctly produced source-to-canonical resolution records,
but those records are not the original task rows. A later contribution step
needs the canonical/reference field materialized back onto the base records,
while keeping the base row fields such as amount, status, timestamp, and source
locator. Without an explicit scaffold, the planner tends to feed resolution rows
directly into compute actions, loses the original fields, then falls back toward
large scripted transforms.

This is not a procurement-specific issue. Any data task that resolves names,
IDs, categories, accounts, devices, people, labels, or other source values
before joining/filtering/aggregating needs a generic source-row projection
pattern:

1. create or read a mapping / entity-resolution artifact;
2. apply it back to a base record artifact with `enrich_records`;
3. continue joins, derived fields, contributions, reconcile, and answer
   projection over the enriched task rows.

- [x] Infer field contracts from materialized JSON action payloads and expose
      them as artifact headers / `output_headers`.
- [x] Include artifact `kind` in `artifact_access` so the planner can
      distinguish record artifacts from mapping / resolution artifacts using
      structural metadata.
- [x] Add `enrich_records` action scaffolds when a base record artifact and a
      mapping/resolution artifact are both available.
- [x] Scaffold the domain-neutral resolution application pattern:
      `source_value -> canonical_id|canonical_label` copied into a new
      canonical/reference field on base rows.
- [x] Add regression coverage for materialized payload field contracts and
      `enrich_records` scaffold generation.

Remaining architecture item: add scaffold ranking and typed field-flow tracking
so the planner sees the most likely next record-preserving action first, without
the system deciding business semantics.

### Batch 74: Multi-Source Derived Fields

Another real-data iteration showed a domain-neutral action gap: data cleaning
often needs to compose several existing fields into a reusable key, search
surface, fallback value, or normalized helper column before later
enrichment/join/contribution steps. The previous `derive_fields` action only
accepted one `source_field`, so models either tried unsupported pseudo-params or
fell back toward a broad custom script.

The fix is to extend the typed field action, not to add domain-specific
normalizers. The system remains responsible only for structural field
composition; the model still decides which fields matter for the current user
goal.

- [x] Add `source_fields` to `derive_fields` specs.
- [x] Add `concat` / `join_fields` for generic multi-field composition.
- [x] Add `coalesce` / `first_non_empty` for generic fallback-field selection.
- [x] Validate every `source_fields` entry against the input artifact fields.
- [x] Update planner instructions and action scaffold templates.
- [x] Add regression coverage for multi-source concat and coalesce.

Remaining architecture item: expand typed field transforms only when they are
domain-neutral and compositional. Business-rule interpretation still belongs to
the model and must remain auditable through rule coverage, decisions,
contributions, and reconcile.

### Batch 75: Deterministic Final Projection From Reconcile

Real-data testing reached a structurally strong terminal state: material
coverage, rule coverage, entity resolutions, contribution records, and
`reconcile_artifacts` all existed, and reconcile status was `pass`. The final
answer was still empty because the evaluator declared the task complete before
an explicit output-projection node materialized `result.answer`.

This is a workflow convergence problem, not a domain problem. Once a data DAG
has reconciled groups, final output should be a typed projection step governed
by `output_contract`, not a fresh large script or a hand-written final answer.
The same rule applies to generic sums, counts, rankings, grouped joins, OCR
extraction totals, JSONL statistics, and strict JSON/CSV/table outputs.

- [x] Add a completion gate for `result.answer == ""` when reconcile groups
      are available and an output contract is present.
- [x] Convert that structural gap into a deterministic `assemble_answer`
      terminal batch.
- [x] Preserve coverage, output contract, goal, success criteria, and
      delimiter/value-field settings without changing business decisions or
      numeric values.
- [x] Wire the deterministic projection path through the same completion
      helper used by REPL and CLI.
- [x] Add regression coverage proving a reconciled-but-empty result cannot be
      accepted as complete and instead schedules `assemble_answer`.

Remaining architecture item: add richer projection policies for non-scalar
artifacts, such as selecting named columns from reconciled groups or assembling
multi-section outputs. Those policies must remain typed and contract-driven;
they must not parse model prose or infer business semantics from file names.

### Batch 76: Actions DAG Owns Executable Scripts

The next real-data run exposed a plan-shape convergence issue. After several
typed DAG batches, the repair planner emitted an `actions[]` plan with a
`compute_contributions` action but placed a 150+ line executable script at the
top-level `script` field. The staging guard correctly rejected the mixed
shape, because top-level scripts are only valid for simple non-actions plans.
However, rejecting it late consumed the remaining repair budget.

The generic contract is now stricter and simpler: once `actions[]` is present,
the executable DAG is the source of truth. Script text must either belong to a
specific `custom_transform` action or be removed as a stray top-level script.
This prevents mixed-plan drift from blocking the workflow before typed actions
can run and receive normal runner/evaluator feedback.

- [x] Preserve existing normalization that moves a top-level script into a
      single empty `custom_transform` action.
- [x] Preserve existing normalization that appends a bounded top-level script
      after enough typed/custom context exists.
- [x] Add a final structural normalization pass for `actions[] + top-level
      script`: if the script cannot be safely assigned to a custom action,
      remove it and keep `actions[]` as the executable DAG contract.
- [x] Keep the staging guard strict for unnormalized plans so tests and
      debugging still catch illegal mixed shapes.
- [x] Add regression coverage for a single typed action plus stray top-level
      script.

Remaining architecture item: expose field/action form builders so models can
fill typed action params without embedding large JSON strings or fallback
scripts. This should reduce how often stray scripts are emitted in the first
place.

### Batch 77: Typed Stage Advancement From Historical Material Progress

The next real-data run exposed a workflow-state regression. Earlier batches had
successfully consumed materials, derived rules, and extracted records. A later
batch no longer carried `required_materials`, so the state builder computed
`required=0` and incorrectly set `material_coverage_sufficient=false`. That
sent the workflow back to `cover_required_materials`, restricted
`allowed_next_actions` to coverage actions, and prevented the planner from
using typed compute actions such as `derive_fields`, `enrich_records`,
`join_records`, `compute_contributions`, and `reconcile_artifacts`.

This is a generic DAG-state problem. A later bounded batch should not have to
repeat the whole material contract just to keep progress alive. Once the
workflow has line/file-backed consumed paths or generated artifacts, material
coverage must not regress only because the current batch focuses on the next
stage.

- [x] Treat historical successful material progress as sufficient material
      coverage when the merged current contract has no remaining required
      runner materials.
- [x] Keep initial plans conservative: before any successful material progress,
      `required=0` still does not auto-complete coverage.
- [x] Let `next_stage` and `allowed_next_actions` advance from that typed
      progress into rule, entity, contribution, reconcile, or answer projection
      stages.
- [x] Add regression coverage proving historical consumed paths/artifacts keep
      coverage sufficient even when a later plan omits `required_materials`.

Remaining architecture item: expose a compact, explicit `stage_progress`
summary in `workflow_state_json` so the planner can see which objective
milestones are satisfied without reverse-engineering them from verbose history.

### Batch 78: Enrichment Field Contract Preflight

The next real-data run advanced correctly from material coverage into compute
actions, but exposed a generic typed-action contract gap. The planner selected
`enrich_records`, yet mixed up base artifacts and mapping artifacts. Because
the action did not validate declared fields before enrichment, the workflow
could create structurally valid but semantically unusable artifacts and only
fail several downstream actions later.

This is not specific to any business domain. Any data task that enriches,
maps, or joins records can fail the same way when a source field, mapping key,
or mapping value field is declared against the wrong input. The stable fix is
to make `enrich_records` reject inconsistent field contracts at the action
boundary with a compact list of available fields.

- [x] Validate `source_field` against the base input before building an
      enrichment lookup.
- [x] Validate `mapping_source_fields` and `mapping_value_field` against each
      mapping input before lookup construction.
- [x] Include the affected input path and a bounded available-field list in
      the typed error so the planner can repair the action without guessing.
- [x] Preserve case-insensitive matching and header/sample-derived field
      discovery for CSV/JSON/text-derived artifacts.
- [x] Add regression coverage for swapped base/mapping inputs and missing
      mapping value fields.

Remaining architecture item: expose action-field contracts directly in the
planner state so candidate `base_path` and `mapping_path` choices can be
scored before the model emits a plan. Runtime preflight prevents bad artifacts;
planner-side field cards should reduce repair turns.

### Batch 79: Compute Stage Before Final Projection

The next real-data run showed a workflow-state issue rather than an action
implementation issue. After material coverage succeeded, the current contract
had no contribution/reconcile flags because the turn was forced into data mode
without a more precise data subtype. The state machine therefore advanced to
`emit_output_contract_answer` even though the only available outputs were
intermediate material artifacts, not a computed answer.

The generic correction is to treat "covered materials but no final answer" as a
compute/transform stage unless an explicit validation contract says otherwise.
This does not force every data task to produce a contribution ledger; it simply
lets the next batch derive, enrich, join, compute, or run one narrow transform
over known artifacts before final projection.

A second bug came from coverage fallback using a shape guard that ignored prior
workflow records. That could convert a later compute plan back into a material
coverage batch even after earlier records had already consumed the required
materials.

- [x] When material coverage is sufficient and there is no answer or explicit
      unfinished validation stage, route the next stage to the generic
      compute/transform action set instead of final projection.
- [x] Keep final completion strict: once a real answer is emitted, the workflow
      still completes without requiring contribution/reconcile unless the typed
      contract requested them.
- [x] Make coverage fallback rely on historical scheduled/consumed materials
      instead of a no-record staging guard.
- [x] Add regression coverage for intermediate artifact summaries and
      historical material coverage preservation.

Remaining architecture item: add a typed goal-shape profile to data plans
(`summary`, `filter`, `transform`, `aggregate`, `join`, `strict_projection`,
etc.) so forced data mode can retain the user's explicit lane choice while
still getting precise validation contracts from the planner. This must be a
schema field, not prose parsing.

### Batch 80: Typed DAG Rank Prefix Execution

The next real-data run reached compute planning but still attempted to emit a
multi-rank batch: derive or normalize mappings, enrich records with those new
mappings, compute contributions, and sometimes reconcile or assemble the final
answer in one plan. Although the runner can materialize artifacts, this shape
is brittle because later actions are planned before the system has real
artifact fields, samples, and aliases from earlier actions.

The generic DAG runtime should converge through materialized typed ranks:
produce fields or mappings, observe the resulting contract, then plan the next
rank. This applies to all data tasks that need staged transformation: record
cleaning, entity normalization, multi-table joins, OCR/text extraction plus
calculation, grouped statistics, and strict output projection.

- [x] Add a typed dependency rank for data actions:
      rules, field/entity derivation, enrichment/join, contribution,
      reconcile, and answer assembly.
- [x] When multiple validation stages remain and a plan crosses ranks, reject
      it before execution with a typed planning error.
- [x] Add a prefix fallback that keeps only the first action rank, sets
      `continue_after=true`, and lets the next batch consume the materialized
      artifacts and field contracts.
- [x] Teach the planner to avoid crossing dependent DAG ranks in one batch.
- [x] Add regression coverage for cross-rank typed plans.

Remaining architecture item: expose rank-specific form builders and artifact
cards in `workflow_state_json` so the model can pick valid next-rank inputs
without guessing aliases or field names.

### Batch 81: Preserve Validation Contracts Through Material Discovery

Another real-data run started with a broad script over many materials. The
runtime correctly converted it into a `material_inventory` discovery batch, but
that fallback dropped validation flags such as decision records,
contribution records, and reconcile. A discovery step is only an early DAG node;
it must not weaken the terminal quality contract.

This is generic across data tasks: inventory, inspection, and sampling actions
may change what the planner knows, but they must preserve the typed contract
that describes what the final answer must prove.

- [x] Make broad-material discovery fallback inherit final validation flags
      such as decision records, contribution records, and reconcile.
- [x] Keep required-material floors out of the raw discovery fallback to avoid
      blocking coverage loops; outer user-explicit material protection may
      re-apply them after fallback construction.
- [x] Append fallback rationale to validation rules without replacing existing
      validation rules.
- [x] Add regression coverage proving decision/contribution/reconcile flags
      survive material discovery fallback.

Remaining architecture item: separate "discovery contract" and "final quality
contract" in the schema so intermediate plans can be concise while final
requirements remain explicit and immutable.

### Batch 82: Strict-Output Shape Implies Auditable Result Contract

Forced data mode can bypass the classifier's precise data subtype. A planner may
therefore emit a generic `data_task` plan with many materials and a strict
single-line/CSV/JSON/table output contract, yet omit decision, contribution, or
reconcile requirements. The workflow then has too little typed signal to know
that intermediate material artifacts are not enough.

The generic fix is shape-based and schema-driven: complex strict-output data
plans need an auditable result contract. This does not inspect user prose or
business terms. It relies on material count, input count, action count, script
size, and the output contract.

- [x] Add a structural normalizer for complex strict-output data plans.
- [x] Enable decision records, contribution records, and reconcile for those
      plans when the planner omitted them.
- [x] Keep simple single-material strict-output plans light-weight.
- [x] Apply the normalizer before and after plan-shape normalization.
- [x] Add regression coverage for complex versus simple strict-output plans.

Remaining architecture item: replace this shape heuristic with an explicit
typed goal-shape field emitted by the data planner, while keeping the shape
normalizer as a conservative fallback for malformed/underspecified plans.

### Batch 83: Persistent Typed Artifact Alias Protocol

The next real-data run showed that generated artifacts were not consistently
available under the aliases that later batches referenced. Child aliases and
source-specific aliases such as `<artifact>#records` could be mentioned by the
planner, but the runner sometimes treated them as ordinary workspace files or
lost parent artifact paths when carrying state across batches.

This is a generic DAG handoff issue. Any multi-step data workflow needs stable
read-only artifact handles so a later action can consume exactly the table or
record set produced earlier, without rediscovering raw materials or inventing
paths.

- [x] Register child artifact aliases to the same materialized JSON payload as
      the parent when the child is only a logical view.
- [x] Carry parent artifact file paths to child aliases in the action seed so
      later batches can resolve generated handles.
- [x] Support source-record aliases by filtering materialized records through
      their `_source_path` lineage when available.
- [x] Accumulate prior artifacts, consumed paths, ledgers, metrics, reconcile,
      and answer/output state in the action-runner seed instead of keeping only
      the latest successful batch.
- [x] Add regression coverage for generated artifact aliases and source-record
      aliases across batches.

Remaining architecture item: promote artifact handles into a first-class
`MaterialGraph` with typed nodes, parent/child relations, and field contracts,
so both planner and runner consume the same handle registry.

### Batch 84: Entity Resolution Defaults And Shape Normalization

Explicit entity-resolution payloads emitted by the model may omit fields that
the structured input mode fills automatically, such as status or reason. This
is a schema ergonomics issue, not a business-rule issue. The system can safely
apply declared defaults to missing metadata, but it must not invent canonical
business values.

- [x] Apply `default_status` and action reason defaults to explicit
      `normalize_entities` records before validation.
- [x] Keep canonical ids, labels, evidence refs, and rule refs model- or
      material-provided; do not synthesize business semantics.
- [x] Add regression coverage for explicit entity resolution records without
      status.

Remaining architecture item: extend typed-result patching so safe metadata
normalization happens in one shared layer for decisions, entity resolutions,
contributions, and reconcile rows.

### Batch 85: Declared-Input Completion For Bounded Custom Nodes

One real run produced a bounded custom action whose script referenced a path
already declared in the top-level plan inputs, but omitted it from the action's
own `input_paths`. The runner correctly rejects undeclared reads, but this
specific case is safely repairable: the path is already part of the plan's
declared material set, so adding it to the bounded node does not grant access
to a new material.

- [x] Scan bounded custom action scripts for exact quoted path literals.
- [x] If the literal matches a top-level declared input path, add it to the
      action-local `input_paths`.
- [x] Do not add undeclared paths, glob patterns, or dynamically constructed
      paths.
- [x] Add regression coverage for declared path literal completion.

Remaining architecture item: move this into a general action-parameter repair
layer that records safe repairs alongside the plan audit file.

### Batch 86: Reference Enrichment Scaffold And Multi-Field Matching

The latest real-data run exposed a deeper typed-action gap. The system had
separate pieces for deriving fields, normalizing entity-resolution rows, and
enriching base rows, but it did not strongly guide the planner toward the most
generic shape: apply a reference table or mapping artifact to base rows to
materialize a canonical/reference field, then join or aggregate on that field.
The model therefore oscillated between an unsuitable `normalize_entities`
action and broad `custom_transform` scripts.

The fix is domain-neutral. Many data tasks require the same pattern: map a
free-form row value, description, label, tag, unit, region, status, device, or
other cue through a reference table before join/filter/aggregation.

- [x] Teach `enrich_records` to accept multiple base `source_fields`, not only
      one `source_field`.
- [x] Select the first reliable mapping hit across those source fields and
      preserve match status/evidence on the enriched row.
- [x] Add soft scaffolds for reference-like artifacts using structural field
      contracts such as canonical/code/value fields plus text/term/name/label
      candidate fields. This remains soft guidance, not a hard gate.
- [x] Prefer latest successful artifacts in `artifact_access` so the planner
      sees current field contracts instead of stale early discovery tables.
- [x] Keep custom transforms cooled down after repeated script failures until
      typed actions, reconcile, or answer assembly have a structurally valid
      narrow next step.
- [x] Update planner teaching for `source_fields` and add runner/scaffold
      regression coverage.

Remaining architecture item: add a dedicated typed `apply_mapping` or
`project_reference` action if `enrich_records` continues to carry too much
surface area. It should still be domain-neutral and should consume explicit
field contracts rather than business-specific roles.

### Batch 87: Invalid Record-Action Fallback Before Model Repair

The next compiled real-data run failed early because the initial plan used a
single `derive_fields` action over several unrelated input schemas and then the
repair LLM returned only narrative content without a replacement tool call. The
guard was correct, but the recovery path was too dependent on the model.

This is a generic workflow-control issue. When a structured action is
deterministically invalid yet can be safely downgraded to a bounded discovery
node, the system should continue the DAG without asking the model to repair the
same shape.

- [x] Add a deterministic fallback for invalid `derive_fields` actions that
      have multiple inputs or no field spec.
- [x] Convert that action into a bounded `extract_records` batch over the same
      declared inputs, preserving the final validation contract and
      continuation state.
- [x] Consume the fallback in both CLI and REPL data workflows before invoking
      LLM repair.
- [x] Add regression coverage for invalid-record-action fallback.

Remaining architecture item: expand this class into a general typed-action
fallback table: invalid action shape -> safe predecessor action, with audit
records explaining why the workflow stepped back.

### Batch 88: Entity/Join Stage Materialization And Typed-First Scheduling

The next compiled real-data run showed that the adaptive DAG could now make
real progress through typed actions, but it still spent unnecessary rounds in
the entity/enrichment stage. The system only considered `entity_resolution`
records as satisfying that stage. In many generic data workflows, however, the
needed entity/reference relationship is already materialized by a reusable
`enrich_records` or `join_records` artifact with concrete fields. Keeping the
workflow blocked on a formal entity-resolution count caused the planner to
invent placeholder mappings before it could move to `compute_contributions`.

This is a general DAG-state issue, not a domain rule. A typed data workflow
should advance from normalization/enrichment to contribution calculation when
either explicit entity-resolution rows exist, or a typed action has already
materialized a reusable normalized/enriched/joined record artifact that later
compute actions can consume.

- [x] Add `entity_stage_materialized` to `workflow_state_json` as a typed state
      signal.
- [x] Treat successful `normalize_entities`, `enrich_records`, and
      `join_records` artifacts as satisfying the entity/enrichment stage for
      DAG scheduling purposes.
- [x] Keep explicit entity-resolution rows as stronger evidence; the new signal
      only prevents stage deadlock when reusable typed artifacts already exist.
- [x] Move complex ledgered workflows to typed-first scheduling before a broad
      script failure is required. If contribution/reconcile/decision/entity
      validation remains, `custom_transform` is filtered out of intermediate
      stages until only final projection remains.
- [x] Normalize evaluator `continue_transform` decisions into typed graph
      continuation when the workflow says custom transforms are disabled.
- [x] Add regression coverage for joined-artifact stage advancement and typed
      custom-transform filtering.

Remaining architecture items:

- [ ] Build a first-class Material Influence Graph so artifact lineage, field
      contracts, contribution inputs, and final projection dependencies are
      visible as typed edges rather than inferred from compact samples.
- [ ] Add a typed final projection assembler that consumes reconcile groups and
      output contracts directly, reducing the chance that final answers are
      manually re-summarized by the model.
- [ ] Expand typed compute actions for common projection/filter/grouping
      patterns so models do not need to fall back to custom scripts for ordinary
      aggregation steps.

### Batch 89: Required Material Scope Convergence

The following real-data run improved typed DAG convergence, but it exposed a
different generic failure mode: the planner widened `required_materials` from a
small core set to every plausible supporting file in one continuation batch.
Because required materials are a hard coverage gate, the workflow kept chasing
candidate attachments instead of progressing from covered core data into
normalization, contribution, reconcile, and final projection.

This is not a domain-specific attachment problem. Any data task can have many
candidate materials: supporting documents, OCR inputs, reference files,
examples, logs, external tool payloads, generated artifacts, or secondary
tables. Only the materials required by the current executable batch or already
established as hard requirements should block workflow progress. Other
candidates must remain optional until a later typed action chooses to inspect or
consume them.

- [x] Constrain continuation-time hard `required_materials` to the previous
      workflow hard requirements plus materials actually consumed by the
      current executable batch.
- [x] Demote newly declared, non-consumed required materials to
      `optional_materials` instead of letting them expand the hard coverage
      gate.
- [x] Preserve user/material-floor requirements and previous hard requirements;
      the new rule only prevents continuation batches from locking broad
      candidate sets as hard requirements.
- [x] Keep runner safety unchanged: actions can still read only declared input
      paths, and a later batch can promote any optional material by explicitly
      consuming it.
- [x] Add regression coverage for hard-required preservation, current-batch
      required promotion, and candidate demotion.

Remaining architecture items:

- [ ] Promote candidate/optional materials into a typed Material Influence
      Graph so future planners can ask for "inspect candidate set X" or
      "promote candidate Y because evidence gap Z exists" without rewriting the
      entire coverage contract.
- [ ] Add evaluator budget signals for "coverage is good enough for compute"
      versus "specific evidence gap remains", so optional material exploration
      is driven by typed gaps rather than broad speculation.

### Batch 90: Typed Action Field Contract Normalization

The next real-data run no longer regressed to a one-shot script, but it exposed
a generic typed-action contract gap. The planner used the correct DAG shape, yet
it still had to guess exact generated-artifact fields such as canonical value
fields and base/reference roles for `enrich_records`. It also copied a
slash-separated operation example (`year/extract_year`) as if it were a literal
enum value. Those failures are not tied to any business domain: any data task
that derives fields, maps reference rows, enriches base rows, or joins generated
artifacts can hit the same structure drift.

The fix is intentionally structural only. The runner may normalize unambiguous
action-shape drift, but it must not infer business membership, numeric values,
rules, or final answers.

- [x] Normalize slash-separated derive operation examples into concrete enum
      values for known pairs such as `year/extract_year`,
      `concat/join_fields`, and `coalesce/first_non_empty`.
- [x] Update planner teaching so supported operations are listed as discrete
      enum values rather than slash pairs that models may copy literally.
- [x] Add deterministic `enrich_records` role repair when fields prove the base
      and mapping inputs are inverted: the declared source fields are present
      only on the mapping input, while the declared mapping source/value fields
      are present on the base input.
- [x] Add deterministic mapping-value-field inference for generated/reference
      artifacts when the requested value field is absent but a clear
      canonical/value/code/id/label field exists.
- [x] Keep hard boundaries: no repair changes filters, membership decisions,
      contribution values, rule interpretation, or final output. Ambiguous or
      semantically missing fields still fail and go through typed repair.
- [x] Add regression tests for operation enum normalization, value-field
      inference, base/mapping inversion repair, and truly unrepairable field
      gaps.

Remaining architecture items:

- [ ] Move artifact-field contracts into a first-class typed schema object
      rather than compact prompt samples, so planners can choose fields from
      explicit base/reference/value roles.
- [ ] Add typed violation objects for action contract failures, including
      JSON path, available fields, and repairability class, instead of relying
      on prose error text.
- [ ] Let the evaluator request a narrow `inspect_artifact_schema` action when
      a generated artifact is too large or too compactly sampled for reliable
      field selection.

### Batch 91: Direct Data LLM Retry And Conservative Fallback

The real-data run after Batch 90 failed before reaching a data conclusion
because the data evaluator's direct LLM call hit a transient stream stall. The
ordinary agent pipeline already treats stream stalls, first-byte timeouts, EOF,
and network errors as retryable, but the newer data planner/evaluator path was
calling the adapter directly and therefore had a thinner recovery surface.

This is a generic reliability gap for all direct structured data tools:
planner, continuation planner, evaluator, result patcher, and compact JSON
repair. A single transient provider hiccup must not kill an otherwise valid
data workflow.

- [x] Add a shared data-lane tool-call wrapper for required-tool LLM calls.
- [x] Retry only precise transient errors accepted by
      `llm.IsRetryableDispatchError`; do not retry schema errors, policy
      blocks, business-rule uncertainty, or exhausted provider retries.
- [x] Apply the wrapper to data task planning, continuation, repair,
      evaluation, result patching, and compact structured-tool repair.
- [x] For evaluator calls, fall back to deterministic conservative workflow
      evaluation after retry budget exhaustion instead of aborting the whole
      data task.
- [x] Add regression coverage for transient evaluator retry and bounded
      fallback.

Remaining architecture items:

- [ ] Surface retry/fallback state in the data workflow event stream with the
      same low-noise UX used by agent stages, so users can distinguish network
      recovery from data-repair loops.
- [ ] Consider sharing the direct structured-call wrapper with operation
      planners and external skill planners once their retry/fallback semantics
      are aligned.

### Batch 92: Current-Batch Scope Preparation

The next compiled real-data run confirmed a generic convergence gap in the
plan-preparation layer. A continuation plan can carry a workflow-wide coverage
contract while the executable next batch only needs a subset of materials. When
the workflow displays or executes that plan without deriving the current batch
scope first, users see misleading summaries such as every batch requiring all
workflow materials, and the runner may receive more paths than the atomic node
should consume.

This is not a domain-specific material problem. Any adaptive data DAG needs a
clear distinction between:

- workflow hard requirements that must eventually be covered;
- current-batch action inputs that are scheduled for this execution;
- optional or candidate materials that may be promoted later.

Changes:

- [x] Add a single `prepareDataTaskWorkflowPlanForExecution` path used by both
      REPL and CLI data workflows.
- [x] Apply user/material floor preservation, path-list normalization,
      custom-action input normalization, and current-batch scoping in that one
      path.
- [x] Scope `input_paths` to concrete current-batch action inputs when actions
      are present, while keeping the workflow coverage contract available for
      validation and continuation.
- [x] Preserve script-only/simple plans so non-DAG data tasks still execute
      normally.
- [x] Update plan summary wording from ambiguous "required materials" to
      workflow-scoped required materials.
- [x] Add regression coverage for initial current-batch action scoping and
      script-only plan preservation.

Remaining architecture items:

- [ ] Make UI summaries show both workflow-level and current-batch-level
      material counts when useful, without turning every data batch into a
      noisy debug dump.
- [ ] Move material-scope derivation into a first-class typed graph edge so the
      runner, evaluator, and UI all read the same batch/workflow distinction.

### Batch 93: Terminal Raw-Material Script Escape Hatch

The following real-data run made real typed progress through material coverage,
rule coverage, and entity/enrichment materialization. The workflow then reached
the final/terminal phase and the planner emitted a large `custom_transform`
that reread original materials and attempted to redo cleaning, joining,
aggregation, reconcile, and final answer assembly in one script. This is the
same architectural anti-pattern in a narrower place: coverage was sufficient,
but the terminal script used that coverage as permission to bypass the typed
DAG and recompute the whole workflow.

The generic rule is:

- custom scripts may remain as a bounded fallback for small transformations;
- final-stage custom scripts may lightly project generated artifacts;
- once contribution/reconcile workflow progress exists, a terminal custom
  script must not reread original materials and recompute the data workflow.

This guard is structural. It uses only typed workflow state, action kind,
script size, action input paths, and generated-artifact aliases. It does not
inspect business labels, filenames, or model prose.

Changes:

- [x] Add a terminal raw-material `custom_transform` guard. At the final answer
      stage, a large or multi-input custom script that reads original material
      paths is rejected before execution.
- [x] Allow narrow final projection scripts over generated artifact aliases, so
      simple output formatting still has an escape hatch.
- [x] Direct rejected terminal scripts toward typed graph actions:
      `derive_fields`, `normalize_entities`, `enrich_records`, `join_records`,
      `compute_contributions`, `reconcile_artifacts`, and `assemble_answer`.
- [x] Reuse existing generated-artifact alias tracking instead of creating a
      parallel material registry.
- [x] Add regression coverage for rejecting terminal raw-material scripts and
      allowing generated-artifact projection scripts.

Remaining architecture items:

- [ ] Continue expanding typed compute/projection actions so terminal
      `custom_transform` becomes rare rather than merely guarded.
- [ ] Add a first-class Material Influence Graph linking original materials,
      generated artifacts, rule coverage, contribution records, reconcile
      groups, and final projection dependencies.
- [ ] Add evaluator-side guidance that, when the final stage is blocked by this
      guard, the next plan should use `assemble_answer` or a contribution /
      reconcile typed action rather than another renamed script node.

### Batch 94: Generic Row-Expansion Atomic Action

The next real-data run exposed a generic typed-action gap. The planner needed
to turn a field containing several values into one record per value before
later matching, enrichment, joining, or contribution computation. It tried to
use `derive_fields` with a regex extraction. That is the wrong abstraction:
`derive_fields` can add or transform columns, but it must not change row
cardinality. Without a row-expansion primitive, the planner tends to fall back
to a broad script or to misuse field derivation.

This is not specific to any business domain. The same shape appears whenever a
record contains aliases, tags, labels, roles, identifiers, categories, terms,
owners, accounts, hosts, or any other delimited list that must become separate
facts before downstream work.

Changes:

- [x] Add a domain-neutral `expand_records` data action kind.
- [x] Implement the action runner: one input record artifact/path, a
      `source_field`, a `target_field`, optional delimiter/split pattern,
      optional original-value retention, de-duplication, output limits, source
      metadata, and a reusable JSON artifact.
- [x] Add workflow staging guards: `expand_records` must declare exactly one
      record-set input and a source field; it cannot be used as a multi-table
      lookup or join substitute.
- [x] Teach the planner schema, prompt, workflow state, allowed-action
      contracts, and scaffolds that `expand_records` changes row cardinality
      while `derive_fields` does not.
- [x] Keep the action domain-neutral. It knows only record fields and split
      parameters, not what a value means.
- [x] Add regression coverage for runner behavior and workflow guard errors.

Remaining architecture items:

- [ ] Add more typed compute/projection actions so row expansion can feed
      contribution and reconcile nodes without a final broad script.
- [ ] Add evaluator checks that detect when a planner repeatedly tries to use
      column derivation for row-cardinality work and nudge it toward
      `expand_records` using typed workflow state rather than prose matching.

### Batch 95: Covered Materials Are Not Permission for One Big Script

The next real-data run successfully produced a strict-output answer, but the
repair path revealed a deeper convergence bug. The first plan had already
inspected several materials, then an empty custom action failed. The repair
plan replaced the failed node with a large `custom_transform` over all already
covered materials and emitted decisions, contributions, reconcile, and the
final answer in one script.

The existing guard correctly rejected broad custom scripts when prerequisite
materials had not been covered. However, once those inputs appeared in a
partial failed result, the guard treated material coverage as enough and skipped
the "whole workflow in one script" check. That is the wrong invariant:
coverage means "the workflow knows these materials exist and has some shape
information"; it does not mean "a free-form script may bypass the remaining
typed DAG stages."

Changes:

- [x] Tighten the broad `custom_transform` staging guard. If the action reads a
      broad surface and looks like it is replacing several data DAG stages,
      reject it even when all its input materials were previously covered.
- [x] Keep narrow custom transforms over known artifacts allowed. The new guard
      only fires on typed action kind, script size, input breadth, validation
      ledger requirements, and workflow shape.
- [x] Update the repair guidance to say that prior material coverage is not a
      valid substitute for `derive_fields`, `expand_records`,
      `normalize_entities`, `enrich_records`, `join_records`,
      `compute_contributions`, and `reconcile_artifacts`.
- [x] Add regression coverage for both branches: missing prerequisites and
      already-covered inputs that still try to run the whole workflow in one
      script.

Remaining architecture items:

- [ ] Move this concept into the evaluator's typed state summary so repair
      prompts can explain the next allowed DAG rank before the model emits the
      next plan, reducing guard/retry churn.
- [ ] Continue replacing broad script fallback with typed contribution and
      final projection actions so the system can independently verify
      computation rather than accepting script-self-reconcile.

### Batch 96: Idempotent Typed Fallbacks and No Self-Referential Enrichment

The next real-data run moved further through the typed DAG, but exposed a
workflow-level convergence bug. A `join_records` node failed because one input
artifact lacked a join field. The deterministic missing-field fallback tried to
materialize that field with `enrich_records`, which is the right generic shape.
However, once a similarly named enriched artifact already existed, the
historical fallback kept regenerating the same plan and, in one case, selected
the same artifact as both mapping input and output. The workflow then spun in a
tight loop without executing a new batch or consuming retry budget.

This is not domain-specific. Any data workflow that uses deterministic
fallbacks to repair missing fields, mappings, derived columns, joins, or
projection artifacts needs the same invariant: a fallback must either create a
new reusable artifact or decline and let the evaluator/planner choose a
different node. It must never self-reference, repeat an already attempted plan,
or keep emitting an already materialized output.

Changes:

- [x] Add artifact-existence checks before generating missing-join enrichment
      fallback plans. If the target output artifact already exists, the
      fallback declines so later planning can use it directly.
- [x] Reject self-referential fallback plans where the output artifact is the
      same as the base input or mapping input.
- [x] Add a domain-neutral fallback-plan signature over action kind, inputs,
      output artifact, and params. Historical fallbacks decline when an
      equivalent plan has already appeared in workflow records.
- [x] Add regression coverage for existing-output suppression and duplicate
      fallback-plan suppression.

Remaining architecture items:

- [ ] Promote fallback signatures into a shared typed workflow loop helper so
      coverage expansion, material discovery, stage-prefix trimming, and
      missing-field fallback all use the same anti-spin guard.
- [ ] Add evaluator-visible "last fallback declined because output already
      exists" and "last fallback duplicate" signals, so the next model plan is
      guided toward using the materialized artifact rather than retrying the
      same repair.

### Batch 97: Contribution Ledger as Generic Item-Level Decision Evidence

The next real-data run reached a healthier typed DAG shape:
material coverage, typed enrichment, typed join, and typed contribution
calculation all executed. It then failed at reconciliation because the workflow
had a contribution ledger but no separate `rows` decision ledger. The model
responded by trying to generate another custom script to create rows, which is
exactly the wrong direction: for aggregation, filtering, joins, and projection
tasks, each contribution record already has the item id, source locator,
group, metric, value, operation, role, evidence refs, and rule refs needed to
serve as generic item-level decision evidence.

This is a structural ledger-alignment issue, not a procurement-specific issue.
The system should not force the model to maintain two independent ledgers for
the same item-level evidence. When a typed action emits validated
contributions, the action runner should carry derived decision records forward
so later reconciliation and answer assembly can stay on the typed DAG path.

Changes:

- [x] Make `ActionRunner` carry `Rows` through partial, seed, custom, and final
      results.
- [x] Derive generic include/exclude `RowDecision` records from each validated
      contribution record produced by `compute_contributions`.
- [x] Ensure seeded contributions also satisfy decision-record requirements
      when a later `reconcile_artifacts` action runs in a separate batch.
- [x] Add action-runner regression tests for contribution-backed decision rows
      and seed contribution reconciliation.

Remaining architecture items:

- [ ] Expose evaluator-visible ledger lineage so the planner sees that
      `contributions -> rows -> reconcile -> answer` is already satisfied and
      does not ask for duplicate custom ledgers.
- [ ] Generalize this pattern to future ledger-producing typed actions:
      any action that emits item-level effects should declare which validation
      ledgers it can satisfy.

### Batch 98: Narrow Typed-Context Custom Transform Exemption

The next real-data run showed that the planner still escaped back to a large
script after the system requested rule coverage. The repair plan had one
`derive_rules` action followed by a `custom_transform` that read many original
materials and emitted decisions, contributions, reconcile, and the final
answer. The previous staging guard treated "there is typed action context
before this script" as enough to exempt scripts below the soft line limit.
That was too broad: a typed predecessor only makes a custom transform safe
when the script is a narrow projection or a single bounded artifact transform,
not when it consumes a broad material surface and satisfies multiple validation
ledgers at once.

Changes:

- [x] Tighten `dataTaskActionLooksLikeWholeWorkflow`: typed context only
      exempts a custom transform when the custom action has a narrow input
      surface, at most one required material, and at most one validation
      ledger.
- [x] Keep narrow final/projection transforms allowed.
- [x] Add regression coverage for the repair pattern that triggered the real
      run failure: `derive_rules + broad custom_transform` must still be
      rejected.

Remaining architecture items:

- [ ] Move the "narrow transform" criteria into the workflow-state prompt so
      the model sees the same structural boundary before emitting the next
      plan.
- [ ] Continue pushing terminal calculation toward typed contributions,
      reconcile, and answer assembly instead of script-self-reconcile.

### Batch 99: Typed Action Script Boundary

The follow-up real-data run reached typed action planning, but the model put a
Python script on an `enrich_records` action. The executor then treated it as a
typed enrich action and failed because typed enrich requires params such as
`mapping_specs_json`, not a free-form script. The model then tried to repair by
switching back to `custom_transform`, re-opening the large-script path.

This is a generic schema boundary: typed actions are declarative action nodes.
Only `custom_transform` carries model-authored code. If a typed action has a
script, the plan is structurally invalid and should be repaired before
execution, with a precise message that the model must express the operation via
`input_paths`, `output_artifact`, and `params`.

Changes:

- [x] Add initial-plan and workflow-plan guards rejecting scripts on typed
      actions.
- [x] Keep `custom_transform` as the only action kind allowed to carry script.
- [x] Add regression coverage for both guard entry points.

Remaining architecture items:

- [ ] Add schema examples for each typed action in the workflow-state scaffold
      so the model can repair params without inventing a script body.
- [ ] Add model-side typed repair hints that quote the offending action kind
      and expected params shape when a typed action script is rejected.

### Batch 100: Single Executable Contract for `actions[]` Plans

The next run showed another script escape hatch. The model emitted typed
actions plus a top-level `script`; the normalizer appended that script as a
final `custom_transform` action. Even though later guards trimmed or rejected
some cases, this normalization step kept reintroducing free-form script nodes
inside an already-typed workflow, especially after `custom_transform` had been
disabled by prior failures.

The stable contract is simpler: when `actions[]` exists, it is the executable
DAG. Top-level `script` is not part of the executable contract and must not be
moved or appended into the DAG. Simple no-action script tasks can still be
wrapped as a single bounded `custom_transform`, but mixed `actions[] + script`
plans are normalized by removing the stray top-level script.

Changes:

- [x] Stop moving top-level script into an empty `custom_transform` action
      when `actions[]` is present.
- [x] Stop appending top-level script as a final `custom_transform` action
      when `actions[]` is present.
- [x] Keep no-action bounded scripts supported via the existing wrapper.
- [x] Refresh normalization tests to enforce the single-contract rule.

Remaining architecture items:

- [ ] Surface this contract in the workflow-state prompt in one concise rule:
      `actions[]` is the whole executable DAG; top-level script is ignored.
- [ ] Add a typed planner repair code for `mixed_actions_and_top_script` so the
      model can repair without repeating the same mixed shape.

### Batch 101: Missing Filter Field Should Not Empty Reference Actions

The next real run correctly moved from material/rule coverage into a
normalization stage, but `normalize_entities` returned zero records because the
model supplied `filter_field=status` on a taxonomy/reference table that had no
`status` column. The old executor applied that filter literally, so every row
was dropped and the workflow failed `entity_resolution_required`.

This is a generic typed-action issue, not a procurement-specific rule. Data
tasks often use reference tables, dictionaries, label maps, enum maps, account
maps, device maps, or other canonical tables. A filter that references a field
absent from the current record set cannot be semantically evaluated. The
executor should not silently turn that into an empty artifact. The safer
generic behavior is to ignore only the non-existent filter for that specific
record set, record it in artifact metadata, and keep real filters that match
existing fields strict.

Changes:

- [x] Add `effectiveActionFiltersForRecords` to drop filters whose field is not
      present in the current headers/records.
- [x] Apply the same filter normalization to `normalize_entities`,
      `enrich_records` mapping lookups, and `compute_contributions`.
- [x] Record ignored filter fields in the action artifact metadata for audit.
- [x] Add regression coverage for a reference-table `normalize_entities`
      action with an absent filter column.

Remaining architecture items:

- [ ] Promote ignored-filter metadata into the workflow evaluator so repeated
      ignored filters can guide the next plan without becoming a hard gate.
- [ ] Add a planner hint that filters must name fields present in the specific
      action input, while the runner will ignore non-existent filter fields
      rather than produce an empty artifact.

### Batch 102: Reference/Resolution Application Must Be Reusable

The next real run reached a better typed DAG shape: material/rule coverage was
complete, a first enrichment created reusable base rows, and a later
normalization action produced source-to-canonical entity mappings. The workflow
still spent several rounds around the same generic boundary: mapping artifacts
and reference tables were understood, but the canonical values were not reliably
applied back onto the base record set before join/aggregation.

This is a system gap for any data workflow with local dictionaries, taxonomy
tables, enum maps, aliases, user/device/account maps, extracted OCR labels, or
model-produced entity-resolution artifacts. The runner must make the common
shape "base rows + mapping/reference rows -> enriched base rows" robust without
requiring the model to hand-write perfect field parameters every time.

Changes:

- [x] Keep `enrich_records` as the domain-neutral primitive for applying
      reference/resolution records to base rows.
- [x] Index delimited reference terms as individual lookup candidates. This
      makes exact matching work for generic alias/tag/label/term cells while
      preserving the original line-backed evidence.
- [x] Add regression coverage for exact matching against delimited reference
      terms.
- [x] Add regression coverage for `normalize_entities -> enrich_records`, where
      an entity-resolution artifact is applied back to a separate base record
      artifact.
- [x] Exclude negative/exclude/except-style reference columns from inferred
      positive mapping-source fields in both runner inference and planner
      scaffolds.反例字段不应被当作同义词或正向标签。

Remaining architecture items:

- [ ] Add an explicit planner scaffold variant for "apply existing
      entity-resolution artifact to base rows" so the model sees the exact
      field shape (`source_value`, `canonical_id`, `canonical_label`) without
      rediscovering it from samples.
- [ ] Promote `ignored_filter_fields` and `matches_<field>` metadata into the
      workflow evaluator. Low/no-match enrichments should trigger a structural
      graph repair before downstream aggregation, not a late wrong answer.
- [ ] Add a dedicated, domain-neutral `apply_mappings` action only if
      `enrich_records` continues to be too overloaded. It should be a strict
      specialization of `base rows + source/canonical mappings -> enriched
      rows`, not a business-specific operation.

### Batch 103: Compact Continuation State Instead of Replaying Long History

The next real run showed the workflow was no longer failing on one giant
initial script, but the continuation/evaluator prompts grew from about 108 KB
to 170 KB as each round replayed historical actions, long params, result
samples, and artifact previews. This is a generic DAG orchestration problem:
any data task can produce large regexes, schema fragments, mapping specs,
record samples, or previous answers. Replaying them every round makes the model
slow and increases the chance that it plans from stale history instead of the
authoritative workflow state.

The stable contract is that continuation needs a compact state machine view,
not a full transcript. `workflow_state_json` remains authoritative for
cumulative coverage, counts, next-stage, allowed actions, and scaffold. Recent
rounds are kept only as short previews to explain the last few transitions.

Changes:

- [x] Split data workflow history rendering into full and compact variants.
- [x] Use compact history for continuation, evaluator, and result-patch
      prompts while keeping the richer renderer available for focused repair
      contexts.
- [x] Clamp action `params` values in prompt history. Large
      `field_specs_json`, mapping specs, schemas, or regexes are shown as short
      previews rather than replayed indefinitely.
- [x] Limit compact continuation to the last few rounds and explicitly mark
      omitted older rounds, with `workflow_state_json` as the cumulative truth.
- [x] Use a compact continuation candidate-file view. Initial planning can still
      see samples, but continuation carries only path/type/size/headers/counts
      so attachments and text evidence samples are not replayed every round.
- [x] Add a regression test with large params/scripts/history to keep
      continuation prompt size bounded.

Remaining architecture items:

- [ ] Add evaluator budget signals such as `rounds_remaining`,
      `recommended_next_stage`, and `budget_exhaustion_risk` so the model can
      choose smaller batches earlier instead of waiting for repair failures.
- [ ] Promote `ignored_filter_fields`, low-match enrichments, and artifact
      no-match counts into the compact workflow state for better structural
      graph repair without increasing prompt size.
- [ ] Add a per-round prompt-size metric to the data audit log so slow
      customer runs can be diagnosed without reading debug LLM payloads.

### Batch 104: Contribution Input Preparation Is Not the Same as Compute

The next real run showed a more subtle state-machine gap. After material
coverage and rule coverage were complete, `workflow_state_json.next_stage`
reported `compute_contributions`. The allowed actions still included
`derive_fields`, `expand_records`, `normalize_entities`, `enrich_records`, and
`join_records`, but the stage name led the model and evaluator to talk as if
aggregation itself was already ready. In real data workflows, missing
contributions often mean "prepare the contribution inputs first": parse values,
derive grouping/filter fields, apply mappings, join with target/query/reference
records, and only then aggregate.

This is not domain-specific. It applies to CSV/JSONL statistics, inventory
matching, audit sampling, OCR-derived records, spreadsheet joins, and any task
where contribution rows depend on intermediate structure.

Changes:

- [x] Add a distinct `prepare_contribution_inputs` workflow stage for
      contribution-ledger tasks that have no contributions yet.
- [x] Keep `compute_contributions` available in that stage, but describe it as
      legal only when the input artifact already contains every value, group,
      and filter field named by the action params.
- [x] Keep existing prerequisite actions (`derive_fields`, `expand_records`,
      `normalize_entities`, `enrich_records`, `join_records`) available in the
      same stage so the model can choose the next atomic preparation step.
- [x] Update regression tests so simple non-ledger tasks can still use the old
      compute stage, while ledgered workflows use the clearer preparation stage.

Remaining architecture items:

- [ ] Add structural readiness signals to workflow state: candidate
      contribution artifacts, available numeric/value fields, likely grouping
      fields, and missing fields from the last rejected `compute_contributions`
      action.
- [ ] Promote low/no-match enrichment and missing-join-field evidence into the
      evaluator so it can recommend prepare/enrich/join instead of merely
      relying on the model's free-form reasoning.
- [ ] Add a typed compute-readiness guard that rejects `compute_contributions`
      before execution when its named value/group/filter fields are absent from
      the input artifact and points to the exact available fields.

### Batch 105: Separate Workflow Material Requirements From Current Batch Inputs

The next real run exposed another generic coverage-contract issue. The model
identified several user-mentioned materials in prose, but emitted only one file
as `required_materials` in the JSON. Existing normalization also narrowed
required materials to the current executable batch so the runner would not
force every file to be consumed at once. That is correct for execution, but it
accidentally erased the global workflow requirement and made coverage appear
complete too early.

The stable split is:

- Current batch required materials: only what this atomic action must consume.
- Workflow required materials: user-pinned or previously accepted required
  materials that still need coverage before the whole data goal is complete.

Changes:

- [x] Preserve deterministic user-pinned material floors as workflow-level
      required even when they are deferred outside the current batch.
- [x] Keep model-authored future materials outside the current batch optional,
      so noisy over-declarations do not create spurious hard gates.
- [x] Teach the cumulative workflow contract to promote optional entries marked
      `Required=true` back into the workflow required set while keeping runner
      execution scoped to the current batch.
- [x] Add regression coverage for the split between current-batch runner
      requirements and global workflow material requirements.

Remaining architecture items:

- [ ] Represent workflow material requirements and current executable inputs as
      separate typed fields in the plan/state schema, instead of overloading
      `coverage_contract.required_materials`.
- [ ] Add compact state counters for user-pinned materials covered/deferred so
      the evaluator can guide expansion without reading the full candidate list.
- [ ] Add audit-log metrics for material-floor promotion and current-batch
      narrowing decisions.

### Batch 106: Script Cooldown Must Not Look Like Compute Capability Failure

The next real run reached the intended adaptive DAG shape: all workflow
materials were covered and `derive_rules` produced rule coverage. The model then
misread `custom_transform_disabled=true` as "data computation is impossible",
even though `allowed_next_actions` still exposed typed compute/preparation
actions such as `derive_fields`, `expand_records`, `normalize_entities`,
`enrich_records`, `join_records`, and `compute_contributions`.

This is a generic contract bug, not a procurement-data issue. Any data workflow
can temporarily disable free-form scripts after a broad script failure while
still having a healthy typed execution path. If the state field looks like a
capability barrier, the evaluator may choose `blocked` or the planner may fall
back to another large script, which recreates the non-convergent loop.

Changes:

- [x] Add a `custom_transform_disabled_note` to the workflow state view,
      explicitly saying the flag disables only free-form scripts and that typed
      actions remain executable.
- [x] Teach continuation planning that `custom_transform_disabled=true` is not
      a compute-capability failure and that the next batch should follow
      `allowed_next_actions` / `allowed_next_action_contracts`.
- [x] Teach evaluation that this flag is not a reason to mark the task blocked;
      it should keep the workflow moving through typed actions.
- [x] Add regression coverage for both continuation and evaluator prompts so
      the contract cannot silently regress.

Remaining architecture items:

- [ ] Add an explicit `execution_capabilities` object to workflow state so
      scripts, typed actions, multimodal extraction, and operation side effects
      are independently represented.
- [ ] Make blocked/needs-clarification decisions consume only typed
      capability/coverage signals, never inferred prose about the disabled
      script path.
- [ ] Add typed-action progress metrics per workflow stage so the evaluator can
      distinguish "no compute path exists" from "next compute preparation step
      is still missing".

### Batch 107: Validate Candidate Plans Against Prior Accepted Workflow State

The next real run showed a guard-ordering problem. After `derive_fields`
failed because it omitted `input_paths`, the repair model emitted a narrower
candidate plan that re-read rules through `custom_transform`. The candidate's
own coverage contract was narrower than the accepted workflow state, and the
allowed-action guard recomputed state using that candidate. That let the
candidate partially redefine the rules used to validate itself.

The stable invariant is: a candidate batch must be validated against the last
accepted workflow state. The candidate can request new materials or produce new
artifacts, but it cannot shrink the accumulated material/validation contract or
reopen a disabled script path before the guard decides whether it is legal.

Changes:

- [x] Change workflow allowed-action validation to compute
      `workflow_state_json` from prior accepted records only.
- [x] Change stage-progress validation to use the same prior-state view so a
      candidate plan cannot hide unfinished validation stages by narrowing its
      own coverage contract.
- [x] Add a regression test where a candidate plan narrows required materials
      and emits `custom_transform`; the guard must still reject it using the
      prior `prepare_contribution_inputs` state.

Remaining architecture items:

- [ ] Split prior accepted workflow state and candidate delta into separate
      typed objects in the planner/runner boundary, so guards never need to
      infer which fields belong to history versus proposal.
- [ ] Add audit metrics when a candidate plan attempts to narrow workflow
      requirements, so real customer logs make this visible without reading
      the full JSON.
- [ ] Add staged repair hints that prefer "same action kind plus missing
      params" when the failure is a local action-shape issue, instead of
      allowing repair to jump back to an earlier workflow stage.

### Batch 108: Cumulative Artifact Availability Must Survive Prompt Compaction

The next real run stopped falling back to scripts, but it still re-extracted
materials because the latest compact `result.artifact_access` sample omitted
older generated artifacts. The model treated a sample omission as proof that
`attachment` or `category` artifacts did not exist, even though
`workflow_state_json.material_coverage_sufficient=true` and earlier rounds had
already generated them.

The generic fix is to separate two concepts:

- `result.artifact_access`: a small sample of the latest result for local
  inspection and field examples.
- `workflow_state_json.artifact_availability`: a cumulative compact catalog of
  generated artifacts and aliases across accepted rounds.

The second is the stable inventory the planner should use to decide whether an
artifact exists. It contains no row samples, so it remains small and works for
CSV/JSONL/text/OCR-derived/spreadsheet data tasks without binding to any
business domain.

Changes:

- [x] Add cumulative `artifact_availability`,
      `artifact_availability_count`, and `artifact_availability_truncated` to
      workflow state.
- [x] Keep availability sample-free: id, kind, aliases, source paths, fields,
      and JSON shape only.
- [x] Teach continuation/evaluation prompts that older artifacts listed in
      workflow state are still available even if the latest result sample omits
      them.
- [x] Add regression coverage for cumulative availability in continuation
      prompts.

Remaining architecture items:

- [ ] Add action-scaffold templates that directly reference the best available
      artifact aliases from `artifact_availability`, not only the latest result
      sample.
- [ ] Add a guard against re-extracting a material when an equivalent artifact
      is already available and material coverage is sufficient, unless the plan
      explicitly requests a different projection/sample shape.
- [ ] Add audit metrics for "sample omission avoided" and "duplicate
      extraction rejected" so large customer runs are easier to diagnose.

### Batch 109: Separate Workflow Material Floors From Current-Batch Inputs

The next real run showed a different scope leak. The workflow correctly
covered the user-explicit materials, then discovered additional auxiliary text
evidence while preparing entity resolution. The candidate plan marked those
auxiliary files as `required=true`, and the old merge logic treated that as a
new workflow hard gate. The next round then reported
`material_coverage_sufficient=false` and asked for more evidence instead of
continuing toward typed contribution/reconcile actions.

This is not specific to invoices, contracts, or procurement data. Any data task
may discover temporary helper materials: lookup snippets, OCR text, sampled
pages, intermediate exports, reference rows, or diagnostic files. Those
materials can be valid inputs for the current action, but they must not
automatically become permanent workflow coverage floors.

The invariant is now:

- user-explicit candidate materials are workflow hard floors;
- model-discovered materials are current-batch inputs unless they are promoted
  by a precise system signal;
- current-batch coverage validation checks what this batch claims to consume;
- workflow coverage validation checks the accumulated hard floors and ledger
  requirements.

Changes:

- [x] Split current-batch coverage from cumulative workflow coverage when
      preparing a plan for execution.
- [x] Keep ledger/validation flags cumulative while limiting current-batch
      required runner inputs to the action batch.
- [x] Prevent `optional_materials.required=true` from promoting a material to a
      workflow hard floor unless it carries the deterministic user-explicit
      material purpose.
- [x] When user-explicit material floors exist, demote newly discovered
      auxiliary materials to optional workflow materials even if the model
      marked them required for the current batch.
- [x] Add regression coverage for both "auxiliary evidence does not become a
      workflow hard gate" and "current execution does not carry historical
      required materials".

Remaining architecture items:

- [ ] Expose `workflow_contract` and `current_batch_contract` as separate
      prompt objects so the model does not have to infer the distinction from a
      single coverage contract.
- [ ] Add typed audit metrics for material-scope changes: user floor, current
      input, optional evidence, generated artifact, and rejected promotion.
- [ ] Add a planner-side material-promotion action that can explicitly request
      upgrading a discovered material to workflow-required only when the
      evaluator has a precise structural reason.

### Batch 110: Deferred Typed Action Queue For Multi-Rank DAG Plans

The next real run no longer inflated workflow-required materials. It reached
typed field derivation, enrichment, and join preparation. However, every time a
model emitted a useful multi-rank DAG plan, the existing guard trimmed it to
the current rank and discarded the rest. The system then had to call the model
again to rediscover the next rank from a large prompt. That produced repeated
planning, large contexts, and sometimes fragile hand-written mapping detours.

This is a runtime orchestration gap. If the model proposes a valid ordered DAG
batch and the system trims it because only one dependency rank should run at a
time, the trimmed suffix is not prose. It is typed work the system can keep as
a deferred queue. After the prefix materializes artifacts, the runtime can
dispatch the next rank deterministically if the workflow state still allows
that action kind.

Changes:

- [x] Extend the stage-prefix fallback to return both the executable prefix and
      the deferred action suffix.
- [x] Add an internal deferred action queue for CLI data workflows.
- [x] Add the same deferred action queue to REPL data workflows.
- [x] After a successful batch result, pop the next deferred rank before
      calling the evaluator/continuation model.
- [x] Preserve existing rank guards: deferred dispatch still consumes typed
      action kinds and workflow state, not user/model prose.
- [x] Add regression coverage that a three-rank plan is split into prefix,
      then automatically resumes the next rank from the deferred queue.

Remaining architecture items:

- [ ] Persist deferred queues in the workflow audit record so interrupted
      sessions can resume the typed graph without asking the model to recreate
      it.
- [x] Add artifact-readiness checks before dispatching a deferred rank, so the
      queue can skip already-materialized actions and request graph expansion
      only when a dependency is genuinely absent.
- [x] Expose deferred-queue length in REPL/CLI data workflow progress with low
      noise, e.g. "deferred ranks: 2".

### Batch 111: Field Contract And Filter Diagnostics For Typed Contributions

The real procurement-style data run proved that deferred typed ranks work: the
system split a multi-rank DAG and resumed subsequent ranks without asking the
model to recreate the whole plan. The next failure moved to a lower-level
contract problem. `compute_contributions` produced an empty ledger because the
planner referenced fields/values that did not match the current generated
artifact. The old runner allowed one especially risky behavior: if a filter
field was absent from one input, it silently ignored that filter for that input
and only wrote `ignored_filter_fields` into a child artifact. Downstream
validation then failed later with the generic message "contributions is empty",
without enough objective field/value diagnostics for repair.

This is not a purchase-data problem. Any data workflow can hit it: web tables,
JSONL logs, OCR-derived records, spreadsheets, extracted text spans, or joined
intermediate artifacts. The invariant is now:

- typed action parameters are part of the structural contract;
- a filter field that does not exist in the action input is a typed execution
  error, not a soft ignored condition;
- when a contribution action legitimately runs but yields zero rows under a
  required contribution ledger, the error must include compact per-input
  diagnostics: total rows, rows matching filters, rows missing value fields,
  available fields, and field samples;
- zero contribution results remain allowed for non-ledger exploratory/audit
  actions, because "zero" can be a valid result for some data tasks.

Changes:

- [x] Stop silently ignoring missing `compute_contributions` filter fields.
- [x] Return an action-level error with available fields and compact
      `field_samples` when filter fields are absent.
- [x] When a required contribution ledger would be empty, return an
      action-level diagnostic instead of falling through to a generic result
      validation error.
- [x] Add per-input diagnostic counters:
      `total`, `filter_matched`, `contributions`, and `missing_value`.
- [x] Keep zero-contribution artifacts valid for non-required exploratory
      contribution actions.
- [x] Add regression coverage for missing filter fields and empty required
      contribution ledgers.

Remaining architecture items:

- [ ] Promote generated artifact field catalogs from prompt-only samples into
      an executable `artifact_schema_projection` contract consumed by action
      validators before model repair.
- [ ] Add a deterministic filter preview action/result that reports value
      distributions and match counts without requiring the model to invent an
      `inspect_material` detour.
- [ ] Let `compute_contributions` optionally consume an expected group artifact
      so valid zero totals can still produce explicit zero-valued contribution
      or reconcile groups when the user asks for all groups.
- [ ] Persist field-contract diagnostics into data audit records as structured
      JSON, not only as error text.

### Batch 112: Artifact Visibility, Row Filtering, And Data UX Transparency

The next live run exposed a different convergence failure. The workflow had
already produced the right kind of intermediate artifacts, but the compact
prompt catalog surfaced old seed artifacts ahead of the newest materialized
outputs. The model could not reliably tell whether a just-created enriched or
joined artifact existed, so it spent long reasoning turns trying to rediscover
or rebuild work that had already succeeded. The same run also showed that row
selection was too often expressed as large scripts or misused `derive_fields`
actions, even though "keep/drop rows based on existing fields" is a generic
data operation.

This is a workflow-runtime issue, not a business-domain issue. The generic
invariants are:

- newly materialized artifacts must be easier to discover than older seed
  artifacts;
- artifact catalogs must deduplicate aliases and logical paths before sampling
  for prompts;
- row filtering over existing fields is a first-class typed action, not a
  special case inside contribution calculation or a custom script;
- data REPL/CLI progress should keep deterministic status in the title line
  and move goal/batch/next-step prose into a low-noise detail block.

Changes:

- [x] Prioritize newest materialized artifacts in cumulative
      `artifact_availability`.
- [x] Deduplicate artifact access entries by alias/path/id before prompt
      sampling and count reporting.
- [x] Add typed `filter_records` action for single-record-set row selection.
- [x] Reuse the existing field-filter parser and comparison semantics across
      `filter_records`, entity normalization, and contribution computation.
- [x] Add support for generic filter operations `not_in`, `exists`,
      `not_exists`, and `not_empty` where the prompt already taught equivalent
      concepts.
- [x] Add planner schema, prompt rules, workflow contracts, staging guards,
      and action scaffolds for `filter_records`.
- [x] Split data-plan UX: title lines keep compact deterministic counters;
      goal, batch purpose, next step, action summary, and failure detail render
      in low-noise detail blocks.
- [x] Add regression tests for newest-artifact visibility, filter action
      execution, workflow staging, planner teaching, and data UX splitting.

Remaining architecture items:

- [ ] Add typed `artifact_schema_projection` so action validators can consume
      exact generated artifact schemas without relying on prompt sampling.
- [ ] Add a compact value-distribution preview action for generic field
      exploration before filtering or grouping.
- [ ] Persist deferred action queues and artifact visibility snapshots across
      interrupted sessions.

### Batch 113: Join/Enrich Boundary And Lookup Contract Consistency

After Batch 112, the real workflow moved further: the planner used typed
actions and `filter_records`, but it still tried to use ordinary joins for
reference enrichment and later attempted a broad custom transform to compare
mapping coverage. The underlying issue is generic: a relational inner join is
not the same operation as applying a reference value onto base rows. If a
mapping/reference table is incomplete, an inner join drops unmatched base
records and hides the coverage gap. The system already had `join_type=left`
support in the executor, but the model-facing contracts did not make that
distinction prominent enough during continuation/repair.

Changes:

- [x] Teach the data planner that `join_records` defaults to `join_type=inner`
      and drops unmatched left rows.
- [x] Teach the data planner and workflow contracts to use `join_type=left`
      when left/base records must remain visible for diagnostics or later
      filtering.
- [x] Prefer `enrich_records` for applying lookup/reference values onto base
      records.
- [x] Update join scaffolds with explicit `inner|left` guidance and a note
      about row preservation.
- [x] Update deterministic missing-join-field fallbacks to emit the current
      role-explicit `lookup_specs_json` contract instead of old mapping-spec
      field names.
- [x] Update regression tests to enforce the new lookup contract.

Remaining architecture items:

- [ ] Add a domain-neutral `coverage_diff` / value coverage action that
      compares a source field against reference/mapping fields and returns
      covered, missing, ambiguous, and sample counts.
- [ ] Add a domain-neutral mapping-candidate action over source values and
      reference rows. The model should choose the business meaning; the system
      should provide candidate matches, status, evidence, and ambiguity flags.
- [ ] Let the evaluator detect severe row-loss after joins and recommend
      `join_type=left`, `enrich_records`, or coverage-diff expansion before
      allowing contribution/reconcile stages.

### Batch 114: Mapping Ledger Convergence And Enrichment Evidence

The next real run showed that the typed DAG is now progressing through
material coverage, derivation, and filtering, but it can still stall at the
normalization/enrichment boundary. The model emitted a `normalize_entities`
action with `resolutions_json=[]` plus structured inputs. The executor treated
the explicit empty array as authoritative and produced zero entity-resolution
records, which triggered repair. The repair planner then tried to fall back to
`custom_transform`, even though the workflow state had already disabled
free-form scripts in favor of typed actions.

This is not a purchase-specific issue. Any data task with lookup/reference
materials can hit the same class of failure: empty generated mapping lists,
reference-table enrichment that does not produce an audit ledger, or repair
plans that drift back to scripts after a typed-stage failure. The generic
invariants are:

- an explicit empty mapping list is not enough to suppress structured input
  derivation when the action has declared input records;
- applying lookup/reference values to base rows is a source-to-canonical
  mapping step and should produce entity-resolution evidence, not only an
  enriched row artifact;
- `custom_transform_disabled=true` must be treated as a structural workflow
  contract. Repair can explain the failed script path, but executable repair
  must stay inside typed actions unless the workflow later re-enables scripts.

Changes:

- [x] `normalize_entities` now falls back to structured input derivation when
      `resolutions_json` / `mappings_json` parses to an empty array and
      `input_paths` are present.
- [x] `enrich_records` now emits generic `entity_resolutions` for each lookup
      application, including matched, unmatched, ambiguous, and missing-source
      statuses with source and mapping evidence refs.
- [x] Added regression tests for empty explicit mapping fallback and enrichment
      ledger emission.

Remaining architecture items:

- [ ] Add a typed workflow guard entrypoint shared by initial, continuation,
      repair, completion-repair, and deferred-plan paths, so invalid repair
      plans are not rendered as ready plans before being rejected.
- [ ] Add a domain-neutral `coverage_diff` / value coverage action that
      compares source values against reference fields before enrichment or
      contribution calculation.
- [ ] Add a domain-neutral mapping-candidate action that produces candidate
      matches, ambiguity, confidence, and evidence without asking the model to
      hand-author mapping arrays or scripts.
- [ ] Feed enrichment row-loss and unmatched/ambiguous mapping counts into the
      evaluator as first-class continuation signals before allowing contribution
      and reconcile stages.

### Batch 115: Schema-Bearing Empty Record Artifacts

The next real run showed that typed joins now exist, but zero-match joins still
sent the workflow into a confusing repair path:

- `join_records` correctly produced no rows for the selected keys.
- The materialized JSON payload was a Go nil slice, which serialized as
  `null`.
- Later `filter_records` / planner turns saw `json_shape=null` or a synthetic
  `value` field instead of a zero-row record set with the predicted output
  schema.
- The model then wrote diagnostic scripts to inspect generated artifacts rather
  than continuing through typed field/materialization actions.

This is a generic record-artifact contract bug. A zero-row intermediate table
is not an absent value. It is a valid, schema-bearing record set. The same
shape matters for joins, filters, expansions, derived fields, OCR/text-derived
records, and any generated artifact whose current sample window is empty.

Changes:

- [x] Materialize empty generated record sets as a wrapper object containing
      `records: []`, `headers`, and structural diagnostics instead of `null`.
- [x] Teach typed JSON record readers to recover wrapper headers even when the
      `records` array is empty.
- [x] Keep non-empty record artifacts unchanged, preserving existing array
      semantics for ordinary generated rows.
- [x] Make artifact access hints prefer `json_records(alias)` for wrapper
      objects that contain `records`.
- [x] Add regression coverage proving a zero-match `join_records` artifact can
      feed a later `filter_records` action without losing field contracts.
- [x] Improve REPL data-process preview color to a readable low-noise tone:
      below final-answer emphasis but above the dim permanent-system line.

Remaining architecture items:

- [ ] Feed zero-row join diagnostics (`left_rows`, `right_rows`,
      `join_fields`, key samples, and row-loss ratios) into evaluator state so
      the next batch can prefer `derive_fields`, `enrich_records`, or
      `join_type=left` diagnostics before contribution/reconcile stages.
- [ ] Add a domain-neutral value-coverage action that compares source key
      values with reference key values and returns covered/missing/ambiguous
      counts without assigning business meaning.
- [ ] Rank action scaffolds by current workflow stage and graph progress so
      generated wrapper artifacts expose the most useful next typed actions
      first.

### Batch 116: Final Projection State, Idempotent Contributions

The next real run progressed through the adaptive DAG and reached typed
`reconcile_artifacts` / `assemble_answer`, but still produced an incorrect
terminal shape:

- a previous intermediate answer such as `4 artifact(s)` was carried as
  `Seed.Answer` and overwrote the answer projected by the later
  `assemble_answer` action;
- repairing/re-running contribution actions carried historical contribution
  records forward and could double-count the same source/group/value record;
- the model sometimes used `group_key=<field-name>` when it meant
  `group_key_field=<field-name>`, which collapsed all contributions into a
  single literal group;
- final answer projection could ignore an explicit reference set and therefore
  omit zero-valued groups required by the output contract.

These are generic DAG state-contract bugs, not domain-specific calculation
rules. A typed data workflow needs idempotent accumulated facts and a clear
precedence order: current answer projection beats stale seed summaries, and
validated contribution groups beat artifact-count previews.

Changes:

- [x] Added generic contribution-record de-duplication by source locator,
      group, metric, value, operation, and role before reconciliation.
- [x] Prevented stale `Seed.Answer` from overriding a current
      `assemble_answer` projection.
- [x] Treat artifact-count answers (`N artifact(s)` / `artifacts,N`) as
      intermediate summaries, not final-answer candidates, in workflow
      completion and handoff merging.
- [x] When `compute_contributions` receives `group_key` that exactly matches
      an input field and no `group_key_field`, treat it as the field form and
      record the inference in diagnostics. Literal grouping remains available
      when the value does not match an input field.
- [x] Let `assemble_answer` complete missing groups with zero when the action
      supplies a reference input and a key field. This supports generic
      “output one value per reference item” tasks without business-specific
      code.
- [x] Added regression tests for group-key field inference, reference-key
      completion, and current assembled answers overriding stale intermediate
      seed summaries.

Remaining architecture items:

- [ ] Surface contribution de-duplication counts and inferred group-key mode in
      workflow evaluator state so the model can see when a repair was
      idempotently collapsed.
- [ ] Add a typed projection validator that compares output item count/order
      against a declared reference key set when the model states one in the
      output contract.
- [ ] Extend action scaffolds so `assemble_answer` examples distinguish
      literal groups, field groups, and reference-complete value lists without
      relying on prose.

### Batch 117: Deterministic Continuation After Planner No-Tool

The next run showed a different failure class: after material coverage became
sufficient, the continuation planner streamed useful reasoning but returned no
`emit_data_task_plan` tool call. The workflow exited with
`data task planner returned no tool_call` even though deterministic workflow
state already contained:

- `next_stage=derive_rules`;
- `allowed_next_actions=["derive_rules"]`;
- a covered rule/constraint material;
- missing validation ledgers that require the next typed stage.

This is a direct-planner robustness gap. A missing tool call should not fail a
typed DAG when the current workflow state has a precise next stage and a
deterministic fallback plan can be built.

Changes:

- [x] Added a shared deterministic continuation fallback that uses
      `workflow_state.next_stage` to build the same typed stage-completion
      plans previously used only for terminal-plan repair.
- [x] Wired the fallback into both CLI and REPL continuation paths before
      returning a planner no-tool/parse failure to the user.
- [x] Added regression coverage for no-tool continuation falling back to a
      `derive_rules` action.

Remaining architecture items:

- [ ] Extend the same deterministic fallback to repeated-node expansion paths
      where the model continuation planner fails after a repeated action error.
- [ ] Emit a concise workflow event when fallback is used so users can see that
      the system continued from typed state rather than model prose.

### Batch 118: Stage-Aware Validation For Typed Atomic Batches

Another real run exposed that a typed intermediate batch can omit
`continue_after=true`. The batch contained only `normalize_entities`, while the
overall task still required decision records, contribution records, and
reconciliation. The runner validated it as a final result, failed because
`rows` was empty, and repair pushed the model back toward a broad script.

This is a stage-validation bug. A typed action batch should be validated
against what its action kinds can produce. If the batch cannot possibly satisfy
the full coverage contract in one step, the runner should validate it as an
intermediate artifact batch even when the model forgot `continue_after`.

Changes:

- [x] Added action-capability detection for full-contract satisfaction.
- [x] If a typed action batch cannot produce required downstream ledgers
      (for example normalize-only with decision/contribution/reconcile still
      required), validate it with the intermediate coverage contract.
- [x] Kept required-material coverage strict: the intermediate relaxation only
      applies when required materials have already been consumed by previous
      batches or the current action inputs. Missing required inputs still fail
      loudly.
- [x] Preserved strict validation for batches that can produce final ledgers,
      such as `custom_transform`, `compute_contributions`, `reconcile_artifacts`,
      and `assemble_answer` where applicable.
- [x] Added regression coverage proving a normalize-only typed batch with
      final ledgers required is accepted as intermediate progress rather than
      rejected for missing decision rows.
- [x] Added regression coverage preserving the older invariant that a terminal
      action cannot ignore missing required materials.

Remaining architecture items:

- [ ] Surface the “validated as intermediate because downstream ledgers remain”
      reason in evaluator state and REPL/CLI low-noise progress.
- [ ] Add per-action capability metadata to the tool schema/prompt so the model
      sees which ledgers each typed action can actually satisfy.

### Batch 119: Field-Contract Driven Repair And Diagnostics

The next real run progressed through rule coverage, material extraction,
field derivation, enrichment, and a first join, but then failed in the
filter/join repair loop. The failures were structural rather than
domain-specific:

- `join_records` used a sorted `input_paths` list when `left_path/right_path`
  were omitted, so the first two paths could become the wrong left/right
  record sets;
- `filter_records` rejected a malformed `filters_json` where the model had
  concatenated filter arrays/objects even though the intended filter list was
  structurally recoverable;
- after material coverage was already sufficient, the workflow rejected
  `inspect_material` even when the action targeted a generated artifact alias
  for schema diagnosis, forcing the model to guess field names;
- repair prompts and guard errors still mentioned a “narrow custom_transform”
  even when `workflow_state_json.custom_transform_disabled=true`, which
  encouraged fallback scripts in a typed stage.

These are generic data-DAG contract issues. The system should let typed actions
repair themselves from objective artifact fields, and should not rely on path
order, prose guesses, or free-form scripts after a typed workflow stage has
disabled them.

Changes:

- [x] Preserved `join_records` input order and added side inference from
      `left_fields/right_fields`: when explicit paths are missing, the runner
      chooses record sets that actually contain the requested left/right
      fields instead of blindly using the first two inputs.
- [x] Added a schema-aware `filters_json` shape repair path that only repairs
      JSON list/object concatenation and then reuses the same typed/draft value
      compatibility parser. It does not change field names or filter values.
- [x] Allowed `inspect_material` diagnostics for generated artifact aliases
      after material coverage is sufficient, while still rejecting repeated
      coverage-only inspection of original materials.
- [x] Removed misleading “one narrow custom_transform” guidance from the
      material-coverage loop and continuation prompt when the workflow should
      advance through typed actions.
- [x] Added regression coverage for join side inference, repaired filter JSON
      list shape, generated-artifact diagnostic inspection, and preserved
      blocking of repeated original-material coverage.

Remaining architecture items:

- [ ] Add typed `field_contract_violation` repair plans that can directly
      suggest a generated-artifact diagnostic or a field materialization action
      without waiting for another full planner call.
- [ ] Persist action-level field lineage into evaluator state so later
      `filter_records`, `join_records`, and `compute_contributions` can select
      from known compatible artifacts more reliably.
- [ ] Add a compact typed-compute scaffold for common “filter + join +
      contribution + reconcile + assemble” progressions so the model sees the
      next legal atomic stage without inventing a multi-rank plan.

### Batch 120: Record Artifact Preference And Filter Diagnostics

A follow-up real run progressed further into typed actions, but still failed
around generated artifact reuse and filtering. The structural problems were
generic:

- a generated artifact alias could be overwritten by a later diagnostic or
  metadata-only artifact with the same alias, causing a typed action to read
  `{id, kind, summary, fields, children}` metadata as if it were business
  records;
- JSON wrapper headers could advertise predicted fields even when non-empty
  records did not actually contain those fields, so filters were guided by a
  schema contract that was not executable against the current record set;
- boolean-like filter markers generated by earlier typed actions (`yes`,
  `valid`, `matched`) were not comparable with later JSON boolean filters
  (`true`), causing empty result sets even when the semantic flag was clear;
- zero-match filter and contribution failures did not give the model enough
  typed diagnostics to see which filter condition removed all rows.

These are data-engine contract gaps rather than business-domain gaps. Generated
record artifacts must remain the preferred executable inputs, field checks must
use actual records when records exist, and typed filters must emit structured
diagnostics before the model retries.

Changes:

- [x] Added artifact-file preference for action aliases. Record-shaped payloads
      now outrank metadata-only artifact summaries when the same alias appears
      across batches.
- [x] Prevented metadata-only artifact JSON objects from being flattened into
      fake business records.
- [x] Changed filter/contribution field existence checks to prefer actual
      record keys when records are non-empty, using predicted headers only for
      empty record sets.
- [x] Added boolean-like equality for typed filters (`yes`/`true`,
      `valid`/`true`, `matched`/`true`, and corresponding false-like values)
      while preserving strict equality for ordinary strings.
- [x] Added compact per-filter diagnostics with total rows, combined match
      count, per-filter match count, and sample values. `filter_records` and
      `compute_contributions` now include these diagnostics in artifact fields
      and failure messages.
- [x] Added regression coverage for bool-like filters, predicted-header
      rejection on non-empty JSON records, and seed-alias preference for record
      artifacts over metadata artifacts.

Remaining architecture items:

- [ ] Promote filter diagnostics into a typed `field_contract_violation` /
      `zero_match_filter` repair object consumed by the workflow evaluator.
- [ ] Add action lineage summaries so the planner can select the latest
      executable artifact for a field set without relying on aliases alone.
- [ ] Add deterministic “inspect actual values for failed filter fields”
      fallback when a filter returns zero rows and the next stage requires a
      non-empty contribution ledger.

### Batch 121: Graph Edge Availability And Script-Disabled Repair Signals

The next real run no longer failed at basic material coverage, but it still
spent repair budget on two generic orchestration mistakes:

- a later batch consumed an intermediate alias that had appeared only in a
  trimmed/deferred plan suffix and had never actually materialized;
- once the workflow had disabled free-form scripts, repair plans still received
  less precise errors such as "multiple custom_transform scripts", which
  encouraged script splitting instead of returning to typed actions.

These are graph-runtime issues, not business-domain issues. A data DAG edge is
ready only when the input is a covered source material, a prior generated
artifact alias, or an output alias from an earlier action in the same accepted
batch. A deferred or imagined future action output is not executable state.
Coverage actions remain allowed to read source materials directly; the
availability guard is for compute-stage consumption of concrete inputs, not for
blocking material discovery or source-backed rule extraction.

Changes:

- [x] Added a typed action input-availability guard for immediate workflow
      plans, using previous results plus same-batch earlier output aliases as
      the executable dependency set.
- [x] Preserved valid same-batch dependencies: a later typed action may consume
      an alias produced by an earlier typed action in the same batch.
- [x] Rejected unavailable typed inputs before runner execution with a precise
      structural message that asks the planner to use covered source materials,
      generated artifact aliases, or earlier batch actions.
- [x] Promoted `custom_transform_disabled=true` to a higher-priority workflow
      guard. When scripts are disabled, the planner now sees that reason before
      lower-level script-count or broad-script errors.
- [x] Added regression coverage for unavailable future artifact consumption,
      same-batch output alias consumption, and script-disabled guard ordering.

Remaining architecture items:

- [ ] Persist dependency snapshots in data audit records so a failed plan can
      be reconstructed without reading full prompts.
- [ ] Promote unavailable-input violations into typed evaluator state so the
      next planner call can choose a diagnostic/materialization action without
      parsing error prose.
- [ ] Add concise REPL/CLI progress when the system continues from a
      deterministic guard/fallback rather than a model-authored plan.

### Batch 123: Field-Contract Readiness For Typed DAG Dispatch

The following real run showed a more specific graph problem. A deferred
`filter_records` node was ready by alias name, but it consumed an
entity-resolution artifact as if it were the original business record set. The
alias existed, so the scheduler dispatched the node; the runner then failed
because fields such as derived numeric/status/date fields were absent from the
mapping ledger.

This is generic. Any adaptive data workflow can produce several artifact roles:
record sets, lookup/reference mappings, diagnostic summaries, rule coverage,
contribution ledgers, and final projections. A typed action edge is executable
only when both the input alias and the fields required by that action's typed
params are available on the selected artifact.

Changes:

- [x] Added a scheduler-side field-contract preflight for typed actions using
      only `action.kind`, `action.params`, and
      `workflow_state_json.artifact_availability.fields`.
- [x] Checked `filter_records`, `compute_contributions`, `join_records`,
      `enrich_records`, `normalize_entities`, `expand_records`, and
      `derive_fields` before dispatch.
- [x] Kept the guard conservative: it only blocks when the input artifact is
      precisely known and exposes a field catalog. Raw source files or unknown
      shapes still fall through to the deterministic runner.
- [x] Allowed sequential `derive_fields` specs in the same action: a later spec
      may consume a field produced by an earlier spec.
- [x] Deferred queues now wait for both alias availability and field-contract
      readiness before dispatching the next rank.
- [x] Added regression coverage for deferred field readiness, missing filter
      fields on a mapping artifact, and legal sequential field derivation.

Remaining architecture items:

- [ ] Promote field-contract failures into typed evaluator state instead of
      relying on textual guard messages in the next planner prompt.
- [ ] Add action-lineage summaries so the planner can distinguish record-set,
      mapping, diagnostic, contribution, reconcile, and final-projection
      artifacts without relying on alias names alone.
- [ ] Add a generic value-coverage / zero-match diagnostic action so empty
      joins and filters can be repaired from objective distributions rather
      than from broad retry planning.

### Batch 122: Reference-Universe Validation For Explicit Entity Resolutions

Another real run showed that entity-resolution actions can drift if a model
authors explicit mappings against a declared reference table but uses canonical
values outside that reference universe. This is structurally unsafe for any
data task with lookup/reference materials: it can silently invent normalized
keys that later joins cannot match.

Changes:

- [x] When `normalize_entities` receives explicit resolution arrays and a
      declared `reference_path`/`lookup_path`/`mapping_path`, validate
      resolved `canonical_id` values against the declared reference fields.
- [x] Use only typed action params and reference-table records for the check.
      The validator does not infer business semantics from prose or filenames.
- [x] Return a compact structural error with invalid values, reference path,
      fields used for the universe, and an allowed-value sample.
- [x] Add regression coverage proving an explicit canonical value outside the
      reference table is rejected before it can poison downstream joins.

Remaining architecture items:

- [ ] Preserve canonical field lineage in entity-resolution artifacts, for
      example which reference field supplied `canonical_id`, so downstream
      `enrich_records` and `join_records` can reason from executable structure
      instead of prompt samples.
- [ ] Add structural alias resolution for entity-resolution artifacts: if a
      downstream lookup asks for a missing value field but the mapping payload
      is a standard `source_value -> canonical_id` resolution set, the runner
      should either use `canonical_id` with diagnostics or emit a typed
      field-contract violation.
- [x] Deduplicate accumulated entity-resolution records across batches so long
      workflows do not repeatedly re-prompt identical mapping ledgers.
- [ ] Surface entity-resolution dedupe counts in evaluator state to make
      compaction visible to repair/continuation planning.

### Batch 124: Normalize-Entities Role And Field Readiness

The latest compiled binary was run again on the same real data workflow. It
made clear progress through atomic DAG ranks and deferred actions, but failed
when a `normalize_entities` action consumed a prior entity-resolution artifact
as if it were the original record set. The alias existed, so dependency
availability passed; the action then reached the runner and failed because the
declared source fields were not present on that input.

This is generic. Source/reference normalization is a structural edge in many
data tasks: tables, JSONL records, extracted text spans, OCR rows, lookup
lists, accounts, tags, labels, entities, or any other dimension. A normalization
node may derive a mapping ledger from one record set, or resolve a source
record set against a reference record set. The scheduler must validate the
declared source/reference fields before execution, without assigning business
meaning to those fields.

Changes:

- [x] Added scheduler-side field-contract preflight for `normalize_entities`.
- [x] Checked source fields and filter fields against the source input, and
      reference fields plus canonical id/label fields against the reference
      input when those fields are declared.
- [x] Preserved explicit mapping mode: non-empty `resolutions` / `mappings`
      payloads still flow to the runner's canonical-universe validation rather
      than being treated as source/reference derivation.
- [x] Matched the runner's structural side inference: when no explicit
      source/reference paths are provided and field evidence proves the two
      inputs are reversed, the preflight validates the inferred sides instead
      of failing a valid plan.
- [x] Added regression coverage for missing normalize source fields on a
      mapping artifact and for legal implicit source/reference role swapping.

Remaining architecture items:

- [ ] Promote normalize/enrich/join field-contract failures into typed
      evaluator state, so repair can choose the next diagnostic/materialization
      action from structured violations instead of textual guard messages.
- [ ] Add action-lineage summaries that distinguish record-set artifacts from
      mapping ledgers, diagnostic summaries, and final projections in a compact
      planner-visible catalog.
- [ ] Add a domain-neutral value-coverage action for source/reference pairs so
      the system can report covered, missing, ambiguous, and sample values
      before enrichment, joins, contribution computation, or final projection.

### Checklist Refresh

The unchecked items in this design are not all stale. They fall into three
buckets:

- Delivered and refreshed inline: material-floor separation, deferred action
  queues, dependency readiness, field readiness for typed actions, script
  disabling, structured params, row filtering, empty-record wrappers, output
  projection precedence, executable-body staging, and normalize
  source/reference preflight.
- Partially delivered but intentionally left open: first-class
  `artifact_schema_projection`, typed evaluator violation objects, action
  lineage summaries, workflow/current-batch contract split in prompts, and
  deterministic value-coverage diagnostics. Current code has pieces of these,
  but not the full architecture described by the checklist item.
- Still open backlog: persisted deferred queues for interrupted sessions,
  resumable audit snapshots, planner-side material promotion actions, richer
  value/mapping-candidate actions, and evaluator budget/lineage signals.

Future checklist updates must only mark an item complete when code and
regression coverage demonstrate the exact invariant. If a later batch partially
covers an older item, leave the older item open and add a status note rather
than turning a broad architecture item into a misleading `[x]`.

2026-06-07 audit refresh: the many remaining `[ ]` entries are not a single
unfinished implementation batch. They are architecture backlog grouped by
capability boundary. Items are considered complete only when all three are
true: the typed contract exists, both CLI and REPL runtime paths consume it,
and regression coverage proves the stated invariant. Later batches have already
covered several older themes in narrower form, such as deferred readiness,
field-contract preflight, artifact preference, and low-noise data workflow UX;
those older broad items remain open when the exact broader invariant is still
missing, for example persisted interrupted-session state, typed evaluator
violation objects, first-class lineage/action schemas, value-coverage actions,
and resumable audit metrics.

### Batch 125: Repair No-Tool Continuation Fallback

The next real run advanced past the previous normalize/source-reference
failure. It then hit a different orchestration gap: after material coverage was
sufficient and scripts were disabled for the compute stage, the model first
tried a broad `custom_transform`, then repaired into a coverage-only batch. The
system rejected both correctly, but the repair planner then streamed reasoning
without an `emit_data_task_plan` tool call, causing the workflow to stop even
though typed workflow state was still available.

This is a planner robustness issue, not a domain issue. If a repair planner
fails structurally after a guard error, and the workflow already has durable
typed records, the runtime should try continuation from current state before
declaring failure. Continuation may still choose typed actions based on
workflow state, artifact catalogs, and allowed next actions; the system does
not invent business semantics.

Changes:

- [x] Added a shared structural classifier for repair-planner no-tool failures
      (`data task planner returned no tool_call` and compact repair no-tool).
- [x] CLI data workflow now falls back from repair no-tool to
      `ContinueDataTask` using current typed records and artifact candidates.
- [x] REPL data workflow now applies the same fallback in terminal-plan,
      terminal-workflow, staging/coverage, execution, completion-gate, and
      evaluator-requested repair paths.
- [x] The fallback preserves workflow material coverage and reuses existing
      plan normalization/protection before display or execution.
- [x] REPL emits a low-noise `数据工作流 · 继续` event when this path is used, so
      the user can audit that the system continued from typed state rather than
      silently retrying.
- [x] Added regression coverage proving CLI repair no-tool continues with a
      continuation plan instead of failing immediately.

Remaining architecture items:

- [ ] Add a deterministic prepare-contribution scaffold for cases where both
      repair and continuation return no tool call while workflow state still
      exposes `next_stage=prepare_contribution_inputs`.
- [ ] Promote guard failures such as coverage-only-after-sufficient-materials
      into typed evaluator state so continuation planning sees them as
      structured violation objects rather than text.
- [ ] Add per-run metrics for repair no-tool fallback count and continuation
      recovery success/failure to the data audit artifact.

### Batch 126: Executable-Body Staging Guard

A subsequent real run demonstrated that a planner can emit `status=ready`
with input paths, output contract, and validation requirements, but with no
typed actions and no executable script. The old runtime let that plan reach the
runner, which then failed with `data task plan has empty script`. That was a
wasted repair round and made the workflow look less deliberate than it was.

This is a generic plan contract issue. A ready data batch must be executable:
either it contains typed data actions, or it contains a bounded script that
emits a structured result. Inputs and prose are not execution.

Changes:

- [x] Added a staging guard for `ready` plans with neither `actions[]` nor a
      script body.
- [x] Reused the existing deterministic coverage fallback when such a plan
      carries uncovered required materials, so the workflow starts with a
      material-coverage action batch instead of executing an empty plan.
- [x] Added regression coverage for both the executable-body guard and the
      deterministic coverage fallback path.

Remaining architecture items:

- [ ] Promote executable-body violations into the same typed evaluator
      violation stream planned for other staging and field-contract failures.
- [ ] Add per-run audit metrics for empty-plan avoidance so customer traces can
      show when the runtime saved a repair round.

### Batch 127: Text Constraint Coverage For Auditable Ledgers

The next validation run no longer executed an empty plan first. It immediately
converted missing material coverage into an atomic coverage batch. However,
text/constraint materials in that batch were still sometimes handled by
`inspect_material`, which exposes only a compact preview. For data tasks that
also require decision records, contribution ledgers, or reconciliation, a
preview can leave the planner without enough durable rule facts and cause
repeated attempts to reread the same material.

This is a generic material-coverage issue. When a text-like material is part of
an auditable data computation, coverage should prefer a typed rule/constraint
artifact over a short preview. The system still does not infer the business
meaning of the rules; it only chooses the structural action that creates
durable rule records for later typed stages.

Changes:

- [x] Coverage expansion now routes text-like required materials through
      `derive_rules` when the workflow contract requires multiple validation
      ledgers, even if the model did not explicitly set
      `rule_coverage_required=true`.
- [x] Structured/tabular required materials still route through
      `extract_records`, and other materials still route through
      `inspect_material`.
- [x] Added regression coverage proving ledger-heavy data tasks derive text
      rules during coverage instead of relying on a preview-only inspection.

Remaining architecture items:

- [ ] Promote derived rule artifacts into a first-class rule catalog in
      workflow state, with concise counts and source locators for the planner.
- [ ] Add typed links from decisions/contributions/entity resolutions back to
      derived rule IDs when later stages consume rule artifacts.

### Batch 128: Record Qualification Bridge Before Contributions

The latest gold-backed real run changed the diagnosis. A previous "correct"
sample output was not the official reference. The official answer showed that
the workflow was no longer only overcounting ambiguous rows; it was also
undercounting rows that require evidence-backed field completion or item-level
eligibility before contribution calculation. The common failure class is not a
purchase-specific rule. It is a graph-contract gap:

- `filter_records` can select rows by existing fields;
- `compute_contributions` can sum/count already-eligible rows;
- but there was no domain-neutral typed action that turns model-understood
  rule/evidence eligibility into auditable include/exclude decisions and a
  reusable eligible record artifact.

The new invariant is:

- the model decides which record-level conditions matter for the current user
  goal and rule set;
- the system only executes typed conditions over existing fields: include
  filters, reject filters, required non-empty fields, required evidence fields,
  generated status fields, accepted/blocked status values, and output mode;
- the action emits generic decision rows plus either an eligible-only or
  annotated record artifact;
- contribution calculation should consume an eligible artifact when rule or
  evidence qualification is required, instead of silently mixing eligibility
  decisions with aggregation.

Changes:

- [x] Added domain-neutral typed action `qualify_records`.
- [x] `qualify_records` supports structured `filters`, `reject_filters`,
      `required_fields`, `evidence_fields`, `status_fields`,
      `accepted_statuses`, `blocked_statuses`, `pass_field`, `reason_field`,
      and `output_mode=filter|annotate`.
- [x] The runner emits include/exclude `RowDecision` records and materializes a
      reusable record artifact for downstream typed actions.
- [x] Planner schema, prompt guidance, workflow allowed-action contracts, and
      action scaffolds now expose `qualify_records` as the generic bridge
      before `compute_contributions`.
- [x] Contribution calculation now rejects rule-qualified target rows with
      unresolved generated status fields unless the model has explicitly
      filtered/qualified those fields or opted into `allow_unqualified_records`.
- [x] Added regression coverage for `qualify_records` feeding contribution
      calculation and for direct contribution failing on unresolved generated
      status fields under rule coverage.

Remaining architecture items:

- [ ] Promote field/status/evidence qualification failures into typed evaluator
      violation objects instead of text-only repair hints.
- [ ] Add material influence graph support so a record field that references
      external evidence can trigger a typed evidence-expansion action before
      qualification when the rule catalog requires it.
- [ ] Add value-coverage and mapping-candidate actions so the model can inspect
      source/reference coverage before choosing qualification conditions.
- [ ] Add a typed final projection validator that checks output item count,
      order, and reference completeness against declared output contracts.

### Batch 129: Current-Batch Source Input Availability

The next real run immediately used the new qualification-aware workflow state
and planned an attachment-evidence preparation step. The first repair then
correctly replaced a disallowed coverage-stage action with a typed
`join_records` action, but the scheduler still reported
`attachment_manifest.csv` and `evidence_index.csv` as unavailable even though
they were present in the same plan's top-level `input_paths`.

This is a generic DAG availability bug. A data batch has two kinds of available
inputs:

- previously materialized workflow facts, such as consumed materials and
  generated artifacts;
- source materials explicitly declared by the current batch's top-level
  `input_paths`.

Action inputs must be allowed to consume either set. Treating current-batch
source inputs as unavailable causes the model to waste repair rounds rewriting
otherwise valid typed actions.

Changes:

- [x] `dataTaskWorkflowCoveredMaterialPaths` now marks current plan
      `input_paths` as available for the current batch.
- [x] Same-batch generated artifact guards remain intact: a future generated
      alias is still unavailable until an earlier action in the same batch
      creates it.
- [x] Updated regression coverage so current top-level input paths are accepted
      while future/generated aliases remain guarded.

Remaining architecture items:

- [ ] Promote unavailable-input diagnostics into typed evaluator state so the
      model receives structured source/current/deferred/generated availability
      categories.
- [ ] Surface current-batch source input counts in low-noise workflow progress
      when a repair plan is accepted after an availability failure.

### Batch 130: Role-Specific Reference Filters And Empty Ledger Artifacts

The next real run advanced past material availability and began typed
normalization. It then hit a generic source/reference contract problem:

- the model planned a source-to-reference `normalize_entities` action;
- it supplied a generic filter that was intended for the reference/lookup side;
- the runner applied generic filters to source rows, producing zero source
  matches;
- the empty entity ledger materialized as JSON `null`;
- validation then failed only at the global ledger layer, and repair drifted
  back toward a broad `custom_transform`.

This is not a domain or purchase-data issue. Any typed data task can normalize
source records against a reference table: labels, ids, categories, accounts,
devices, people, locations, web-table keys, OCR-extracted values, or JSON
records. Such actions need role-specific filters and schema-bearing empty
outputs.

The new invariant is:

- `source_filters` apply to source/base rows;
- `reference_filters` apply to reference/lookup rows;
- when generic `filters` are used in a two-sided normalization, the runner may
  assign each filter structurally by field availability and match counts;
- an empty ledger artifact is still an artifact and must not serialize as
  `null`;
- if a required typed ledger action produces zero records, the action-level
  error should include compact field/filter diagnostics before global
  validation retries.

Changes:

- [x] Added role-aware filter routing for `normalize_entities` reference mode.
- [x] Added explicit `source_filters` / `reference_filters` / related structured
      param aliases.
- [x] Kept the generic `filters` path structural: a filter moves to the
      reference side only when source match count is zero and reference match
      count is positive, or when the field exists only on that side.
- [x] Added action-level diagnostics for required `normalize_entities` batches
      that produce zero entity-resolution records.
- [x] Materialized empty entity/rule/contribution/decision ledgers as
      schema-bearing wrapper objects instead of JSON `null`.
- [x] Taught generated JSON record readers to flatten ledger wrappers such as
      `entity_resolutions` and `rule_coverage`.
- [x] Added regression coverage for role-routed reference filters and
      non-null empty entity ledger artifacts.

Remaining architecture items:

- [ ] Promote zero-ledger typed action diagnostics into structured evaluator
      violation objects instead of relying on error text.
- [ ] Add a deterministic value-coverage action so the model can inspect source
      and reference value overlap before choosing normalization filters.
- [ ] Add typed repair fallback for failed normalization that proposes
      `inspect_material`, `derive_fields`, `enrich_records`, or adjusted
      role-specific filters without invoking a broad script.
- [ ] Surface role-filter routing diagnostics in low-noise REPL/CLI progress
      when a filter is auto-assigned by structural match counts.

### Batch 131: No Silent Fallback From Reference Mapping To Source-Only Normalize

The follow-up run showed that source/reference normalization can still fail in
a subtler way: a plan may declare reference-style parameters, but if reference
mode does not activate for any structural reason, the older runner could fall
back to single-input self-normalization. That produces superficially valid
`entity_resolutions` rows but without canonical ids from the reference table,
so downstream enrichment and joins become guesswork.

This is a generic typed-action contract issue. If a data action states that it
is mapping source values to a reference/lookup universe, silent source-only
fallback is worse than a loud structural error.

Changes:

- [x] Added a guard that rejects `normalize_entities` plans that declare
      source/reference mapping inputs but fail to activate reference mode.
- [x] The guard is structural: it looks at typed action params such as
      source/reference paths, source fields, reference fields, lookup fields,
      and canonical id/label fields. It does not inspect business words.
- [x] Added regression coverage proving reference normalization emits
      canonical ids from a reference table, including Unicode/source-alias
      values and JSON-array field-list params.

Remaining architecture items:

- [ ] Add value-overlap diagnostics to the guard error so repair sees source
      sample values, reference candidate values, and per-mode match counts.
- [ ] Add field-lineage summaries for generated entity-resolution artifacts so
      later `enrich_records` can select source/canonical fields without
      guessing from compact samples.
- [ ] Preflight all model-generated continuation/repair plans before REPL/CLI
      permanent rendering so plans rejected by workflow guards do not appear as
      ready plans.

### Batch 132: Do Not Invent Entity Mapping Fallbacks

The next real run revealed an over-correction in deterministic continuation.
When a prior batch had only inspected materials, the fallback saw that
`entity_resolution_required=true` and generated a broad `normalize_entities`
action over all covered materials. This produced thousands of source-only
resolution records and made the workflow appear to have completed entity
resolution, even though no precise source/reference field contract had been
chosen.

This is a generic data-DAG risk: deterministic fallback may safely complete
mechanical stages whose inputs are already structurally unambiguous, such as
rule derivation from rule materials, reconciliation from contribution ledgers,
or final answer projection from reconcile groups. It must not invent semantic
mapping between arbitrary materials.

Changes:

- [x] Removed deterministic entity-resolution completion fallback for missing
      `/entity_resolutions` ledgers.
- [x] Removed next-stage fallback that generated `continue_entity_resolution`
      over all covered materials.
- [x] Kept rule/reconcile/answer fallbacks, where the structural inputs are
      already precise.
- [x] Updated regression coverage so an entity stage without a field contract
      does not auto-generate a broad `normalize_entities` action.

Remaining architecture items:

- [ ] Add a typed `mapping_candidate` / `value_coverage` action so the system
      can offer objective source/reference overlap diagnostics without
      deciding business semantics.
- [ ] Build field-contract-aware normalize scaffolds from existing artifact
      schemas, so the model receives precise source/reference candidate pairs
      instead of a generic entity-resolution requirement.
- [ ] Feed “entity stage requires planner-selected field contract” into
      evaluator state and REPL/CLI progress.

### Batch 133: Keep Broad Free-Form Scripts Out Of Multi-Ledger DAG Nodes

The next run avoided deterministic entity-mapping invention, but the initial
planner still tried to collapse a broad data workflow into one
`custom_transform`. The first script had no result emitter, and the repair
added an emitter while keeping the same structural problem: one free-form node
was trying to consume a wide source-material surface and satisfy multiple
validation ledgers at once.

This is a generic workflow-shaping issue. A free-form script can still be a
useful escape hatch for one bounded transform over known artifacts, but it must
not replace the adaptive DAG when the task requires several independent
validation ledgers such as rule coverage, per-record decisions, contributions,
reconciliation, and final projection. The guard must be structural, not
business-specific: it looks at action kind, input surface, raw/generated input
boundary, script size, and required ledger count.

Changes:

- [x] Added an initial-plan guard for short `custom_transform` scripts that
      read a broad input surface while the plan requires three or more
      validation ledgers.
- [x] Added the same workflow-stage guard for broad raw-material
      `custom_transform` actions after prior progress. The workflow guard
      excludes generated-artifact projections so final formatting and bounded
      artifact transforms remain available.
- [x] Preserved more specific guard ordering: long scripts still report the
      bounded-script size problem, terminal raw-material scripts still report
      the terminal projection boundary, and missing prerequisites still report
      missing typed coverage.
- [x] Added regression coverage proving an emitter-bearing broad script is
      rejected, while a bounded two-ledger action-level transform remains
      allowed.

Remaining architecture items:

- [ ] Preflight all model-generated continuation/repair plans before permanent
      REPL/CLI rendering so plans rejected by guards do not appear as ready.
- [ ] Add a compact typed-compute scaffold for common
      filter/qualify/normalize/enrich/join/contribute/reconcile/project
      progressions, generated from current workflow state and artifact schemas.
- [ ] Add evaluator feedback that distinguishes "bounded custom transform is
      allowed" from "custom transform would collapse multiple outstanding
      ledgers", so repair can converge without repeatedly proposing scripts.

### Batch 134: Apply Entity Resolution Ledgers Back Onto Records

The next real run moved away from one-shot scripts and progressed through a
typed DAG: normalization produced entity-resolution artifacts, derived fields
produced record artifacts, and material coverage became sufficient. The
workflow then stalled at a generic boundary: before contribution calculation,
base records needed canonical fields from existing entity-resolution ledgers.
The model tried to build a "master record" table with `custom_transform`
because no typed action expressed "apply these existing mappings back onto
these rows".

This is not procurement-specific. Any data task that resolves names, ids,
labels, categories, accounts, devices, people, locations, tags, or other
dimensions has the same structural handoff:

1. `normalize_entities` emits an auditable source-to-canonical ledger.
2. Later filtering/joining/contribution needs canonical fields on the original
   task records.
3. The system must apply the existing ledger deterministically rather than ask
   the model to write a bespoke merge script.

Changes:

- [x] Added typed action `apply_entity_resolutions`.
- [x] The action consumes a base record artifact plus one or more
      entity-resolution artifacts, then materializes target id/label/status
      fields on the base rows.
- [x] Default keying is structural: base record `_source_index` / runner index
      is matched to `#N` parsed from resolution `item_id` / source locator. The
      model can override with explicit `base_key_fields` and
      `resolution_key_fields`.
- [x] Added `resolution_specs` / `apply_specs` structured params and JSON alias
      normalization.
- [x] Added planner schema, prompt guidance, workflow allowed-action contracts,
      dependency rank, process scaffolds, and guard support.
- [x] Added regression coverage proving a generic source/reference mapping can
      be normalized, applied back onto base rows, and then used for
      contribution/reconcile without a script.

Remaining architecture items:

- [ ] Add artifact lineage summaries that explicitly show which generated
      canonical fields came from which resolution artifact and source key.
- [x] Add field-contract guard coverage for `apply_entity_resolutions` so
      invalid base/resolution key-field specs are rejected before execution with
      typed diagnostics.
- [ ] Add compact typed-compute scaffold generation that chains
      normalize_entities -> apply_entity_resolutions -> qualify_records ->
      compute_contributions -> reconcile_artifacts -> assemble_answer when the
      current workflow state and artifact schemas make that progression legal.
- [ ] Preflight model-generated plans before permanent rendering so rejected
      custom-transform fallback plans are not shown as ready.

### Batch 135: Apply-Resolution Base Path And Field Contract Convergence

The next real run showed that the new typed action existed but its structural
contract was still too loose. The model emitted a valid-looking
`apply_entity_resolutions` plan with `input_paths` plus a structured
`resolution_specs` object, but the runner still treated the first input path as
the base when the spec carried a more precise `base_path`. The same plan used a
resolution `item_id` locator such as `artifact#N:field` as a key while the base
record used `_source_index`. Without structural locator normalization, the
action could fail or produce no matches, pushing repair back toward broad
scripts.

This is a generic artifact-contract issue. Any adaptive data DAG can produce a
mapping ledger and later apply it back to base records: source/reference
normalization, lookup enrichment, OCR/text-extraction alignment, row
qualification, or multi-file joins. The invariant is now:

- action-level `base_path` and per-spec `base_path` are typed structural
  contracts, not hints;
- if the base is not the first `input_path`, the explicit `base_path` wins;
- locator-like resolution fields that contain `#N` can align to base
  `_source_index` without requiring the model to hand-normalize keys;
- invalid base/resolution field references are rejected by workflow guards
  before runner execution.

Changes:

- [x] Preserved `base_path` in `apply_entity_resolutions` spec parsing and
      rejected conflicting top-level/spec base paths.
- [x] Finalized default resolution paths only after the base path is known, so
      path order is no longer the only role signal.
- [x] Normalized explicit locator key fields such as `item_id` or
      `source_locator` when they contain a structural `#N` source index.
- [x] Treated runner virtual base locators (`_source_index`, `source_index`,
      source line/index aliases) as valid base key fields for field-contract
      checks.
- [x] Added workflow preflight for `apply_entity_resolutions` base and
      resolution field contracts, including canonical-value source fields.
- [x] Updated planner guidance so models know when to supply `base_path` and
      how structural locators align.
- [x] Added regression coverage for reversed input path order, per-spec
      `base_path`, locator-key alignment, and workflow guard diagnostics.

Checklist audit note: the remaining unchecked items in this document are still
intentional backlog unless a later batch marks them complete with code and
tests. This batch only promotes the `apply_entity_resolutions` field-contract
guard item because it is now implemented and covered.

Remaining architecture items:

- [ ] Add artifact lineage summaries that explicitly show which generated
      canonical fields came from which resolution artifact and source key.
- [ ] Add compact typed-compute scaffold generation that chains
      normalize_entities -> apply_entity_resolutions -> qualify_records ->
      compute_contributions -> reconcile_artifacts -> assemble_answer when the
      current workflow state and artifact schemas make that progression legal.
- [ ] Preflight model-generated plans before permanent rendering so rejected
      custom-transform fallback plans are not shown as ready.

### Batch 136: Script-Disabled Repair Convergence And Ledger Compaction

The next full real run did not hang, but it proved that the workflow could
still waste rounds after a field-contract failure. The planner correctly
received objective missing-field diagnostics for a generated artifact, reasoned
that `custom_transform_disabled=true`, and still emitted another scripted
repair. The guard rejected it, but the CLI/REPL staging loops did not route
that rejected repair through the shared deterministic workflow fallback. At the
same time, repeated entity-resolution artifacts were accumulated verbatim
across batches, inflating prompt state from hundreds to thousands of mapping
records.

These are generic data-DAG convergence issues, not task-domain issues:

- if a workflow stage disables free-form scripts, a repair plan containing
  `custom_transform` is structurally invalid and should be converted to a typed
  fallback or generated-artifact schema diagnostic instead of immediately
  failing the user;
- the same structural fallback path must be used by CLI and REPL initial,
  continuation, and repair staging loops;
- accumulated validation ledgers should be idempotent facts, not append-only
  prompt bloat.

Changes:

- [x] Added deterministic no-emitter fallback for complex top-level scripts:
      exploratory scripts without `emit`/`emit_result` now become atomic
      `derive_rules` / `extract_records` / `inspect_material` observation
      batches when the typed contract shows a complex data task.
- [x] Added deterministic fallback for `custom_transform_disabled=true` repair
      plans. It first tries the existing typed next-stage fallback; if that is
      not precise enough, it emits a generated-artifact `inspect_material`
      schema diagnostic over already materialized aliases.
- [x] Routed later CLI and REPL workflow staging guards through the shared
      deterministic fallback before asking the model for another repair.
- [x] Added entity-resolution ledger de-duplication in the runner seed and
      accumulation paths, matching the existing contribution-ledger
      idempotency pattern.
- [x] Added regression coverage for no-emitter observation fallback,
      script-disabled generated-artifact diagnostics, and entity-resolution
      de-duplication.

Checklist audit note: the document intentionally keeps many `[ ]` items as
active architecture backlog. A checked item must correspond to code plus tests,
or to an explicit later batch that supersedes it. This batch refreshed only the
items actually delivered here; remaining unchecked items are not assumed stale.

Remaining architecture items:

- [ ] Promote missing-field and zero-match filter diagnostics into typed
      evaluator state so the next planner turn can choose from known generated
      artifacts without re-reading long prose errors.
- [ ] Add compact typed-compute scaffold generation for common
      filter/qualify/apply-resolution/join/contribution/reconcile/projection
      progressions, grounded in artifact schemas and allowed-next-action state.
- [ ] Surface entity-resolution and contribution de-duplication counts in
      REPL/CLI workflow progress and evaluator prompts.
- [ ] Preflight model-generated repair plans before permanent rendering so
      rejected script fallbacks do not appear as ready plans.

### Batch 137: Workflow Ledger Handles And Extract Window Contract

The next real run confirmed that Batch 136 reduced broad scripts, but it also
exposed two domain-neutral contract leaks:

- `extract_records` runner read `params.limit`, while planner/fallback paths
  often emitted `params.max_records`. The intended full bounded extraction was
  silently capped at the default 20-record sample, which made later planning
  believe only sample windows were available.
- The workflow accumulated hundreds of entity-resolution and rule-coverage
  records, but those ledgers were visible only as prompt counts/samples. They
  were not registered as reusable generated artifacts, so the planner could say
  "entity resolutions exist" while still having no stable path to feed into
  `apply_entity_resolutions`, `qualify_records`, or later typed actions.

These are generic adaptive-DAG issues. Any data task can accumulate structured
facts across batches: row decisions, rule coverage, entity mappings,
contribution records, and reconcile groups. Those facts must be executable
read-only graph artifacts, not prose-only history.

Changes:

- [x] Made `extract_records` accept both `limit` and `max_records`, preserving
      `limit` compatibility while letting planner/fallback code request larger
      bounded windows through the same `max_records` idiom used by other typed
      actions.
- [x] Updated invalid-record-action fallback to emit both `limit` and
      `max_records`, avoiding another cross-layer naming drift.
- [x] Added prompt guidance that `extract_records` is a materialization/sample
      action and that full-file downstream work should request a sufficiently
      large bounded window and verify row/sample counts.
- [x] Materialized seed workflow ledgers as generated read-only artifacts:
      `workflow_decision_records`, `workflow_rule_coverage`,
      `workflow_contributions`, and `workflow_entity_resolutions`.
- [x] Registered those workflow ledger aliases through the existing artifact
      file registry, so typed actions and `json_records(alias)` can consume
      them without a special side channel.
- [x] Taught the JSON record reader to treat `entity_resolutions` and
      `rule_coverage` wrapper arrays as records, matching the existing support
      for `records`, `rows`, and `contributions`.
- [x] Added planner guidance that workflow ledger handles are read-only JSON
      artifacts and should be consumed directly instead of reconstructing
      accumulated ledgers from prose.
- [x] Added regression coverage for `max_records` extraction and applying seed
      entity-resolution ledgers through the `workflow_entity_resolutions`
      handle.

Checklist audit note: the unchecked items above are not stale by default. This
batch completes the specific "accumulated ledger has no executable artifact"
and "extract window contract drift" gaps, but leaves broader evaluator and
lineage work open.

Remaining architecture items:

- [ ] Promote workflow ledger handles into evaluator state with compact counts,
      newest alias, and de-duplication stats so the next planner turn sees both
      availability and reliability.
- [ ] Add artifact lineage summaries that show which generated canonical
      fields were applied from which workflow ledger handle and source key.
- [ ] Add a compact typed-compute scaffold that prefers
      `workflow_entity_resolutions` for apply-resolution stages when the base
      record artifact lacks canonical fields.
- [ ] Persist workflow ledger handles and deferred queues across interrupted
      sessions, rather than only within one in-process workflow run.

### Batch 138: Action Coverage Capability For Discovery Nodes

The next real run exposed an earlier contract leak in the adaptive DAG. A
broad, oversized plan was correctly converted into a typed `material_inventory`
fallback. However, the fallback still carried workflow-level
`required_materials` into the current batch. The runner then treated
`material_inventory.input_paths` as if the action had consumed those materials
and failed with a script-consumption error. That pushed the planner toward a
large repair script even though the correct next step was simply to continue
from the discovery artifact.

This is not specific to any file name, table type, or business domain. Data
actions have different relationships to their inputs:

- discovery actions may list candidate paths without consuming their content;
- profiling/materialization actions can consume source materials and generate
  evidence;
- transform/compute actions consume record artifacts and may satisfy ledger or
  final-output contracts.

The invariant is now:

- an action's `input_paths` count as material-coverage evidence only when that
  action kind can actually consume those inputs;
- `material_inventory` produces objective workspace metadata but does not
  satisfy `script_consumed` / `text_evidence_consumed` coverage;
- terminal validation remains strict: discovery cannot masquerade as final
  material coverage;
- intermediate validation remains local: a discovery batch may finish and let
  the workflow planner/evaluator decide the next typed action.

Changes:

- [x] Added a generic action capability helper for whether `input_paths` can
      satisfy material coverage.
- [x] Excluded `material_inventory` inputs from intermediate material
      consumption checks and from "all required materials are covered" terminal
      capability detection.
- [x] Kept all actual consuming actions on the existing strict path, including
      inspect/materialization/derive/filter/normalize/join/compute/custom
      actions.
- [x] Added regression coverage proving `material_inventory + continue_after`
      does not fail because required materials were not consumed.
- [x] Preserved the terminal regression that a non-final action still cannot
      bypass workflow material coverage.

Checklist audit note: the many unchecked items in this document remain active
architecture backlog unless a later batch explicitly checks them with code and
tests. This batch refreshes only the discovery-node coverage capability gap.

Remaining architecture items:

- [ ] Move action capability metadata into a shared typed table used by the
      runner, planner prompt, workflow guards, and low-noise progress events.
- [ ] Surface "validated as discovery/intermediate" as a compact evaluator
      signal so the next planner turn does not infer failure from missing final
      ledgers.
- [ ] Add audit metrics for action coverage capability decisions: discovery,
      source-consuming, artifact-consuming, ledger-producing, and finalizing.

### Batch 139: Script-Disabled Diagnostic Artifact Ranking

The next real run progressed through material coverage, rule derivation, entity
resolution, and applying resolutions to the base records. The planner then
attempted to fall back to a broad `custom_transform` for eligibility filtering
and final aggregation. The workflow correctly rejected the script-disabled
plan, but its deterministic diagnostic fallback inspected the generated
resolution/mapping artifacts named by the failed plan instead of the newest
rich record artifact that had just been materialized.

This is not specific to any business data shape. When a free-form script is
blocked, the next typed repair needs objective schema information for the
record artifact most likely to feed the next legal action. Failed-plan
`input_paths` are useful hints, but they are not always the best diagnostic
target: a model may reference old mapping artifacts while the workflow already
has a newer executable record set.

The invariant is now:

- script-disabled repair diagnostics choose from both the failed plan inputs
  and the cumulative generated-artifact catalog;
- generated artifacts are ranked by structural usefulness: newest availability,
  richer record fields, executable JSON shape, and non-mapping record-set
  character;
- mapping/entity-resolution artifacts remain eligible but are lower priority
  than a richer current record set when the workflow needs downstream
  derive/filter/contribution actions;
- the ranking uses typed artifact metadata only, not business-specific file
  names or model prose.

Changes:

- [x] Replaced first-match diagnostic input selection with a scored generated
      artifact ranking.
- [x] Kept failed-plan generated inputs as candidates while allowing newer
      richer record artifacts to outrank old mapping artifacts.
- [x] Preserved existing generated-artifact safety checks: the fallback still
      inspects only aliases already present in the workflow artifact catalog.
- [x] Added regression coverage where a script-disabled plan names old mapping
      artifacts but the fallback inspects the latest rich record artifact first.

Checklist audit note: this batch refreshes one concrete unchecked lineage/
diagnostic gap. Broader items such as first-class artifact lineage summaries,
typed value-coverage actions, and compact typed-compute scaffolds remain
unchecked until they are implemented with code and regression tests.

Remaining architecture items:

- [ ] Promote the generated-artifact ranking signal into workflow evaluator
      state so planner prompts can see why a record artifact was preferred.
- [ ] Add a deterministic typed-compute scaffold after script-disabled repair
      that proposes the next legal derive/filter/contribution action from the
      inspected artifact schema instead of requiring another long planner turn.
- [ ] Persist diagnostic artifact-ranking decisions in the data audit record as
      structured JSON.

### Batch 140: Entity Reference Role Inference And Checklist Audit Discipline

The next real run showed that the typed DAG was no longer blocked by material
coverage or script-disabled diagnostics. It reached entity normalization, but a
reference-first/source-second `normalize_entities` action failed because the
runner assumed `input_paths[0]` was the source and `input_paths[1]` was the
reference unless the source fields were entirely absent from the first input.
That covered one half of role inversion, but missed the equally structural
case where the first input contains the canonical identifier field and the
second input contains the source value field.

This is not tied to any domain or file naming convention. Any data task can
provide lookup/reference records before base/source records: taxonomy maps,
account maps, device maps, identity maps, status maps, unit maps, or extracted
text evidence. The system should respect explicit `source_path` and
`reference_path`, but when paths are implicit it can use field-contract
evidence to infer the safer orientation.

The invariant is now:

- explicit source/reference paths remain authoritative;
- implicit source/reference roles are inferred only from typed field contracts:
  declared source fields, declared reference fields, and canonical id fields;
- inference never uses business-specific names, file names, user prose, or
  model reasoning text;
- the runner records a compact role-inference diagnostic when it swaps roles,
  so later repair/evaluator prompts can audit the structural decision.

Changes:

- [x] Added a field-contract scorer for `normalize_entities` source/reference
      orientation.
- [x] Let `normalize_entities` swap implicit source/reference inputs when the
      reversed orientation has stronger structural evidence, including the
      common reference-first/source-second case.
- [x] Preserved explicit `source_path` / `reference_path` behavior; no
      automatic role correction happens when the user or planner provides
      explicit roles.
- [x] Added source/reference diagnostic fields recording the structural role
      inference.
- [x] Added regression coverage for reference-first input order using only
      generic record/reference fields.

Checklist audit note: a full-document audit found many unchecked items because
this document intentionally tracks architecture backlog, not only the current
patch queue. An unchecked item must stay unchecked unless the exact stated
invariant has code, prompt/schema/runtime integration where relevant, and
regression coverage. Narrow later batches may satisfy part of an older broad
item; in that case the narrow batch is recorded separately and the broad item
remains open.

Remaining architecture items:

- [ ] Promote `normalize_entities` role-inference diagnostics into evaluator
      state so continuation prompts can see source/reference corrections
      without reading child artifacts.
- [ ] Generalize the same field-contract scoring table across
      `enrich_records`, `join_records`, and future source/reference typed
      actions instead of each action owning bespoke role inference.
- [ ] Add a compact field-contract violation object for role-mismatch failures
      so repair can request schema inspection or role correction without a full
      planner rewrite.

### Batch 141: Intra-Batch Artifact Dependencies And Action Output Semantics

The next real run showed that the planner could still emit dependent actions
inside a single batch: one action produced an artifact alias and a later action
in the same batch consumed that alias as if its field contract were already
known. This is especially fragile when the producer is `normalize_entities`,
because that action emits an entity-resolution mapping ledger, not a rewritten
source record table. The later action then reasoned over an imagined artifact
shape and failed or repaired through broad scripts.

This is a generic adaptive-DAG scheduling gap. Any action can produce an
artifact whose real shape, fields, and ledger semantics are known only after
execution: extraction, derivation, filtering, entity normalization, enrichment,
joins, contribution computation, reconciliation, and final projection. If a
later action consumes that artifact, it should run in the next DAG rank after
the producer materializes real metadata.

The invariant is now:

- independent actions may share a batch;
- if an action consumes a prior action's `id` or `output_artifact`, the batch
  is split at that dependency boundary;
- the producer rank executes first and the dependent suffix is retained as a
  typed deferred plan;
- dependency detection uses only typed action fields (`input_paths`, `id`,
  `output_artifact`), not file names, business terms, or model prose;
- `normalize_entities` prompt guidance explicitly says it emits a mapping
  ledger; base records must be materialized with `apply_entity_resolutions` or
  `enrich_records` before later filtering, joining, or contribution
  calculation.

Changes:

- [x] Added a staging guard for intra-batch artifact dependencies.
- [x] Added deterministic prefix fallback that splits the plan into producer
      prefix and deferred dependent suffix even for initial plans with no prior
      workflow records.
- [x] Preserved independent same-batch actions: the guard triggers only when a
      later action references a previous action's typed output alias.
- [x] Updated source-to-reference normalization prompt guidance to distinguish
      mapping-ledger production from record-table materialization.
- [x] Added regression coverage proving an initial dependent batch is split
      before execution.

Remaining architecture items:

- [ ] Attach explicit output capability metadata to every action kind so the
      planner and guard can say "this action produces records",
      "this action produces a mapping ledger", or "this action produces a
      reconcile/final projection" without relying on prose.
- [ ] Expose deferred dependent suffix length and producer/consumer aliases in
      low-noise REPL/CLI workflow progress.
- [ ] Add evaluator signals for "producer produced zero usable records" vs.
      "producer produced mapping ledger but consumer expected record fields",
      so repair can choose apply/enrich/diagnose without full replanning.

### Batch 142: Workflow Ledger De-Duplication

The next real run demonstrated that typed DAG execution was progressing through
rules, entity normalization, application, filtering, and field derivation.
However, the cumulative workflow state repeatedly re-seeded old rule coverage
into each new action result. Because the runner then appended seed ledgers
back into the output result, rule coverage counts doubled across rounds:
48 -> 96 -> 192 -> 384 -> 6144 -> 12288. That inflates prompts, slows model
turns, and makes audit counters look alarming even when no new rules were
learned.

This is not specific to rule text or the current data task. Any cumulative
workflow ledger can be reintroduced across batches: rule coverage, decision
records, entity resolutions, contribution records, and reconcile summaries.
Entity resolutions and contributions already had de-duplication; rule coverage
needed the same treatment.

The invariant is now:

- seeded rule coverage is de-duplicated before action execution;
- newly derived rule coverage is de-duplicated when appended to the cumulative
  rule ledger;
- partial/failure results, last-result merges, and final outputs expose
  de-duplicated rule coverage;
- de-duplication uses typed rule fields (`rule_id`, `rule_text`, `status`,
  `notes`, sorted `evidence_refs`), not prompt prose or task-specific words;
- first occurrence order is preserved for stable audit output.

Changes:

- [x] Added `DedupeRuleCoverageRecords`.
- [x] Applied rule-coverage de-duplication to action-runner seed loading,
      derive-rules accumulation, partial results, last-result merging, and
      final outputs.
- [x] Added regression coverage for direct rule-coverage de-duplication.
- [x] Added regression coverage proving duplicate seed + action rule coverage
      does not inflate the runner result.

Remaining architecture items:

- [ ] Add de-duplication metrics to workflow/evaluator state, e.g. rule
      records collapsed, entity records collapsed, contribution records
      collapsed.
- [ ] Apply the same first-class de-duplication audit to row decisions and
      reconcile groups if later real runs show repeated accumulation there.
- [ ] Compact prompt state so large ledgers are represented by counts, stable
      handles, samples, and diagnostics rather than repeated full records.

### Batch 143: Action Input Order Is A Typed Contract

The next real run exposed a lower-level runtime contract bug. The planner
emitted the right `apply_entity_resolutions` shape:

- base source records first;
- entity-resolution artifacts after the base;
- no explicit `base_path`, so the first input path should be the base.

The runner cleaned `action.InputPaths` with a helper that also sorted strings.
That changed the typed role order before execution. A resolution artifact whose
path sorted before the source records became the implicit base, so the output
artifact looked like an entity-resolution ledger with canonical fields instead
of a source record table with canonical fields. Downstream typed actions then
failed to find ordinary source fields such as identifiers, status, amount, or
multi-value attachment keys and the workflow drifted into schema inspection and
blocked broad-script repairs.

This is not specific to the current data task. Input order is part of the typed
contract for many actions:

- `normalize_entities` when implicit source/reference roles are used;
- `apply_entity_resolutions` when implicit base/resolution roles are used;
- `enrich_records` when implicit base/mapping roles are used;
- `join_records` when implicit left/right roles are used;
- single-input actions such as derive/filter/expand/qualify/compute when the
  planner relies on the first input as the current record set;
- bounded `custom_transform` input availability and audit order.

The invariant is now:

- role-bearing action input paths are cleaned and de-duplicated while
  preserving planner-declared order;
- sorting remains available for unordered sets such as evidence references,
  aliases, and deterministic audit keys;
- the runner must not change typed role semantics during normalization;
- regression coverage must include an input order where the base path sorts
  after the resolution path.

Changes:

- [x] Added an order-preserving path cleaner for action/plan input paths.
- [x] Replaced sorted input-path cleaning across typed action runners:
      inspect/extract/derive-rules/normalize/derive/expand/filter/qualify/
      apply-resolution/enrich/compute and bounded custom transforms.
- [x] Kept existing sorted normalization for unordered evidence/audit/alias
      lists.
- [x] Added regression coverage proving `apply_entity_resolutions` keeps the
      first input as the base even when the base path sorts after the
      resolution path.

Checklist audit note:

- Batch 6 is complete as marked.
- Batch 7 is partially complete: coverage-contract separation, deferred
  readiness, and workflow intent UX are delivered; full lookup/enrichment role
  canonicalization remains open.
- Batches 109-120 have their "Changes" sections completed as marked. Their
  "Remaining architecture items" are not stale checkboxes; they are intentionally
  preserved backlog until the exact invariant has code, runtime integration,
  and regression tests.

Remaining architecture items:

- [ ] Add a shared role-bearing input contract table for every typed action,
      so planner prompts, staging guards, and runners all agree which positions
      are ordered roles and which inputs are unordered sets.
- [ ] Promote action input order diagnostics into workflow evaluator state,
      including when a runner inferred a base/source/reference role from
      explicit fields or from first-input order.
- [ ] Add an audit metric for role-preserving input normalization so future
      customer traces can distinguish "model chose this role order" from
      "system normalized this input set".

### Batch 144: Field-Contract Narrowing For Single-Record Actions

The next real run got past the input-order bug and continued farther through
the adaptive DAG, but then failed in a different generic way. After a
`compute_contributions` attempt reported missing fields, the repair planner
emitted a `derive_fields` action with seven input paths. The guard correctly
rejected it because `derive_fields` is a single-record-set action, but the
workflow then spent a repair turn on a structural issue the system could have
resolved deterministically: the requested source field existed on exactly one
declared input artifact.

This is not tied to purchase data, dates, statuses, or money. Any typed data
task can have a planner over-include already-covered materials when the next
action only needs one current record set: CSV cleanup, JSONL statistics, web
table normalization, OCR-derived records, device/person/account joins, or
strict final projection. The generic rule is:

- single-record-set actions may receive extra current-batch inputs from an
  over-broad repair plan;
- if objective artifact field contracts prove that exactly one input contains
  every field the action reads, the workflow may narrow `input_paths` to that
  one input before staging;
- if zero or multiple inputs match, the system must not guess. The existing
  staging guard remains authoritative and asks the planner to split or join
  record sets first;
- this narrowing changes only action input scope, not business semantics,
  filters, values, mappings, or final answers.

Changes:

- [x] Added shared pre-execution narrowing for single-record-set typed actions:
      `derive_fields`, `expand_records`, `filter_records`, and
      `qualify_records`.
- [x] Derived the action's read fields from existing typed params and structured
      specs, including `field_specs_json`, filters, reject filters, required
      fields, evidence fields, status fields, and source/value fields.
- [x] Used `workflow_state_json.artifact_availability` field contracts as the
      only hard signal. No prompt prose, field-name keywords, or business
      domain rules are used.
- [x] Kept ambiguous matches unchanged so the normal single-record-set guard
      still blocks unsafe plans.
- [x] Added regression coverage for unique-field narrowing and ambiguous-input
      preservation.
- [x] Added regression coverage for top-level `apply_entity_resolutions`
      `resolution_path` inheritance across multiple specs.
- [x] Added regression coverage for `normalize_entities` selecting the
      non-source input as the reference when `source_path` is explicit.

Remaining architecture items:

- [ ] Surface input narrowing as a typed workflow event/audit metric, including
      action id, original inputs, selected input, and required field set.
- [ ] Extend the same field-contract narrowing to future single-record-set
      actions as they are introduced, ideally from a shared action capability
      table rather than per-kind switch statements.
- [ ] Feed repeated single-record-set guard failures into evaluator state so
      the planner sees whether the issue is "choose one existing artifact",
      "join/enrich first", or "field truly absent".

### Batch 145: Executable Artifact Handles For Schema Diagnostics

The next real run got past entity-application and single-record-set narrowing,
then exposed another generic graph-contract issue. A planner referenced a
future artifact (`qualified_po_records.json`) that did not exist yet. The
availability guard correctly rejected that plan. The repair planner then
attempted a broad `custom_transform`; because the workflow was already in a
typed stage with `custom_transform_disabled=true`, the deterministic fallback
tried to inspect generated artifact schemas before another typed repair.

The fallback was structurally too permissive. It selected generated aliases
from the flattened artifact catalog, but that catalog contains more than
executable record sets: rule coverage nodes, entity-resolution ledgers,
metadata summaries, diagnostic artifacts, reconcile reports, and final answer
artifacts can all carry aliases. In the real run, the diagnostic fallback
selected rule identifiers (`rule_1`, `rule_2`, `rule_3`) instead of a
record-shaped generated artifact. That made the next planner turn reason from
the wrong handle.

This is not specific to procurement, rules, or any field name. The invariant is
now:

- a schema-diagnostic fallback for typed data repair may inspect generated
  artifacts only when they are executable record-like artifacts;
- typed non-record artifacts such as `derive_rules`, `material_inventory`,
  `inspect_material`, `normalize_entities`, `reconcile_artifacts`, and
  `assemble_answer` are not valid record-schema diagnostics;
- known record-producing action kinds are accepted from typed kind/shape, even
  if fields are not fully sampled yet;
- unknown artifacts must have a record-like shape and must not look like a pure
  mapping/ledger before the fallback can inspect them;
- the decision uses artifact kind, JSON shape, and typed field contracts, not
  prompt prose, business terms, or file-name keywords.

Changes:

- [x] Added a typed executable-record filter for generated schema diagnostic
      fallback inputs.
- [x] Kept known record-producing actions eligible:
      `extract_records`, `derive_fields`, `expand_records`, `filter_records`,
      `qualify_records`, `apply_entity_resolutions`, `enrich_records`,
      `join_records`, `compute_contributions`, and bounded
      `custom_transform` record outputs.
- [x] Excluded non-record typed artifacts such as rule coverage, inventory,
      inspect, normalization ledgers, reconcile reports, and final answer
      artifacts from schema-diagnostic fallback input selection.
- [x] Added regression coverage proving rule artifacts are skipped when a
      generated record artifact is available.
- [x] Re-ran targeted REPL data workflow regression tests for custom-transform
      disabled fallback and single-record-set narrowing.

Backlog audit:

- The large number of unchecked items in this document is not all stale
      progress. Most are intentionally preserved architecture backlog from
      real-run failures.
- Some backlog is duplicated across batches under different wording. The main
      recurring themes are:
      artifact schema projection, action lineage, value-coverage diagnostics,
      typed evaluator violations, output projection validation, process-event
      audit metrics, and persisted deferred workflow state.
- Future document cleanup should group repeated backlog by architecture theme
      after each batch is either implemented or explicitly deferred. A checkbox
      should only become `[x]` when code, runtime integration, and regression
      coverage all exist.

Remaining architecture items:

- [ ] Promote executable artifact handle selection into a shared
      `artifact_schema_projection` object so planners, guards, fallbacks, and
      runners all consume the same record/mapping/rule/reconcile classifications.
- [ ] Add evaluator-visible diagnostics when a model references a future or
      unavailable artifact, including the closest available generated record
      handles and why each unavailable handle was rejected.
- [ ] Add an audit metric for fallback diagnostic selection: candidate aliases,
      rejected non-record aliases, selected executable record aliases, and
      whether the selection came from plan inputs or newest artifact ranking.

### Batch 146: Deferred Queue Audit And CLI Result Transparency

The next real run showed that a valid multi-rank typed plan was split into an
executable prefix and a deferred suffix. The prefix completed and produced
entity-resolution artifacts, but the CLI path immediately called the evaluator
and continuation planner instead of visibly resuming the deferred suffix. The
old logs did not show whether the deferred queue had been saved, was empty,
failed an alias/field readiness check, or was discarded by a later path. The
same run also showed that CLI data results had only compact summary logging,
while REPL had richer result artifacts.

This is a generic adaptive-DAG observability problem. A deferred suffix is
typed workflow state, not hidden planner prose. If the runtime keeps, resumes,
blocks, or discards it, that transition must be auditable from precise
structural state. Result artifacts must also be inspectable in CLI runs, because
real customer/eval reproductions often run outside REPL.

Changes:

- [x] Added a shared deferred-queue status helper that reports action count,
      first action, ready count, blocked count, and a structural blocked reason
      derived from alias availability, workflow allowed actions, and field
      contracts.
- [x] CLI and REPL now log/display when a deferred typed action queue is saved.
- [x] CLI and REPL now log/display when a deferred queue is discarded instead
      of silently clearing it.
- [x] Deferred queue reasons are structural: unavailable input aliases,
      workflow-stage action mismatch, or typed field-contract failures. They do
      not parse planner prose or business wording.
- [x] CLI data runs now write full result JSON audit artifacts and print the
      full result JSON to debug logs, matching REPL auditability.
- [x] Data-audit file names now include microseconds, preventing same-second
      plan/result/error/material artifacts from overwriting each other.
- [x] Regression coverage now exercises deferred
      `apply_entity_resolutions` resumption with multi-resolution
      `resolution_specs_json`, matching the typed parameter shape seen in real
      runs.

Checklist refresh:

- The earlier unchecked items about persisted deferred queues,
      artifact-readiness checks, and deferred progress are only partially
      covered by this batch. In-memory queue visibility and readiness reasons
      are now implemented; interrupted-session persistence is still open.
- The broader `artifact_schema_projection`, action lineage, typed evaluator
      violation, and value-coverage backlog remains open. This batch improves
      observability and diagnosis; it does not collapse those larger
      architecture items into `[x]`.

Remaining architecture items:

- [ ] Persist deferred queue snapshots and blocked reasons in the workflow
      audit record so interrupted sessions can resume without asking the model
      to recreate the graph.
- [ ] Promote deferred blocked reasons into typed evaluator state instead of
      only displaying/logging them.
- [ ] Add a shared audit artifact writer for CLI/REPL data plans, results,
      errors, scripts, and material extraction outputs so future audit
      expansion does not duplicate file naming or logging behavior.

### Batch 147: Record Materialization Is Not Coverage Looping

The next instrumented run proved that deferred queue visibility was useful: the
runtime saved the deferred suffix, then reported that the first deferred action
was blocked by missing fields. The deeper cause was earlier in the graph. A
planner emitted a reasonable `extract_records` batch to materialize covered CSV
sources into executable record artifacts. The old coverage-loop guard treated
all `extract_records` actions as coverage-only once required materials were
covered, rejected the batch, and pushed the workflow into repair. Downstream
deferred actions then tried to consume aliases/fields that had never been
materialized.

This is not specific to CSV files or purchase data. In an adaptive data DAG,
"material coverage" and "record materialization" are distinct:

- coverage proves that source materials have been discovered/consumed;
- record materialization creates reusable typed artifacts for later atomic
  actions;
- `inspect_material` and `material_inventory` remain coverage/diagnostic
  actions;
- `extract_records` is coverage-like only when it does not declare a reusable
  output artifact. When it declares `output_artifact`, it is a legitimate
  compute-stage preparation action over an already covered source material.

Changes:

- [x] Changed the coverage-loop guard so `extract_records` with a concrete
      `output_artifact` is no longer classified as coverage-only.
- [x] Kept repeat `inspect_material` / `material_inventory` coverage looping
      blocked after required material coverage is sufficient.
- [x] Added regression coverage proving record materialization remains allowed
      after material coverage is sufficient.
- [x] Confirmed through the real-run stderr/logs that deferred queue blocked
      reasons now surface the downstream effect of missing materialized record
      artifacts.

Remaining architecture items:

- [ ] Add a typed artifact materialization requirement to workflow state so the
      evaluator can ask for missing record artifacts directly instead of
      waiting for a deferred action to fail readiness.
- [ ] Add typed lineage from source material -> record artifact -> derived
      artifact so later planners can select executable record sets without
      relying on prompt samples.

### Batch 148: Record Materialization In Later Workflow Stages

The next real run confirmed the Batch 147 coverage-loop fix, but exposed the
same semantic split one layer later. A planner emitted an `extract_records`
batch that correctly declared reusable output artifacts for already covered
source materials. The coverage-loop guard no longer rejected it, but the
workflow stage guard still treated `extract_records` as illegal once
`next_stage=normalize_or_enrich_entities`. The repair prompt then taught the
model the wrong lesson: abandon typed record materialization and fall back to a
broad script.

This is a generic adaptive-DAG contract bug. A workflow stage such as
normalization, enrichment, filtering, contribution calculation, or answer
projection may still need a record artifact before it can make progress.
Material coverage is not enough; source materials and generated payloads must
also be materialized into executable record sets when downstream typed actions
need fields.

Changes:

- [x] Allowed `extract_records` as a later-stage typed action only when it
      declares a concrete `output_artifact` and consumes explicit current-batch
      input paths or generated artifact aliases.
- [x] Kept coverage-only `extract_records` blocked after material coverage is
      sufficient when it has no reusable output artifact.
- [x] Added `extract_records` to workflow-stage action contracts for
      normalization/enrichment, contribution-input preparation, and contribution
      computation as a record-materialization action, not as repeated coverage.
- [x] Updated continuation prompt guidance so the model sees the distinction
      between coverage-only extraction and record artifact materialization.
- [x] Updated coverage-loop and stage-guard error text so repair turns do not
      steer the model back to broad custom scripts.
- [x] Added regression coverage for record materialization at the entity stage,
      current-batch materialization over newly introduced inputs, and rejecting
      post-coverage extraction without `output_artifact`.

Checklist audit:

- The large unchecked sections throughout this document are not all stale
      progress. Many are intentionally retained architecture backlog:
      persisted workflow state, executable artifact schema projection,
      value-coverage/mapping-candidate actions, typed evaluator violation
      objects, action lineage, and durable resume.
- Do not mark a broad backlog item complete merely because one symptom is
      improved. A checkbox becomes `[x]` only when code, prompt contract,
      runtime integration, and regression coverage satisfy that specific
      invariant.

Remaining architecture items:

- [ ] Add a workflow-state field for "record artifacts required but absent" so
      the evaluator can request materialization before a downstream deferred
      action fails readiness.
- [ ] Promote record-materialization contracts into executable
      `artifact_schema_projection` so guards can distinguish raw source,
      record artifact, metadata artifact, rule ledger, mapping ledger,
      contribution ledger, reconcile report, and final projection without
      relying on compact prompt samples.
- [ ] Add low-noise workflow events when the runtime accepts later-stage
      `extract_records` specifically as record materialization.

### Batch 149: Side-Aware Entity Resolution Application

The next real run progressed into typed normalization, materialization,
filtering, and joins. It then exposed a deeper generic correctness bug:
multiple source dimensions were accumulated into the same
`workflow_entity_resolutions` ledger, but `apply_entity_resolutions` built its
lookup index from row locators only. The planner had emitted typed filters that
selected the intended resolution subset, but the runner ignored those filters.
As a result, a base row could receive a canonical value from the wrong
dimension before later filters, joins, and contribution stages ran.

This is not specific to any business domain. Any data task can accumulate
several source-to-canonical dimensions in one workflow ledger: names, labels,
categories, accounts, devices, locations, ids, roles, or other reference
values. Applying those mappings must be side-aware and namespace-preserving.

Changes:

- [x] Added `GenericFilters`, `BaseFilters`, and `ResolutionFilters` to the
      executable `apply_entity_resolutions` spec.
- [x] Parsed action-level `filters` plus explicit `base_filters` /
      `resolution_filters` / `mapping_filters` / `reference_filters` for
      `apply_entity_resolutions`.
- [x] Routed generic filters by objective field availability and match counts:
      fields present only on resolution records filter the resolution side;
      fields present only on base records filter the base side.
- [x] Applied resolution-side filters before building the resolution index, so
      mixed workflow ledgers cannot cross-apply unrelated dimensions when the
      plan declares a typed selector.
- [x] Added compact filter diagnostics to `apply_entity_resolutions` child
      artifacts so future repair turns can see how filters were routed.
- [x] Updated planner guidance and action scaffolds to prefer explicit
      resolution-side filters when one ledger contains multiple dimensions.
- [x] Added regression coverage proving two dimensions in one workflow ledger
      apply independently through typed filters.

Checklist audit:

- The unchecked backlog above remains real. This batch closes one concrete
      invariant: typed filters on resolution application must affect execution.
      It does not complete broader items such as value-coverage actions,
      artifact schema projection, persistent lineage, or typed evaluator
      violation objects.

Remaining architecture items:

- [ ] Add first-class entity-resolution namespace fields, such as source
      artifact, source field, and target dimension, to the ledger schema so
      filtering can become an explicit typed join condition instead of relying
      on item/source locator shape.
- [ ] Promote side-routing diagnostics into evaluator state so repeated
      cross-dimension ambiguity can trigger deterministic repair without a full
      planner round.
- [ ] Add a domain-neutral value-coverage action before resolution application
      when a resolution subset has many unmatched or ambiguous keys.

### Batch 150: Stable Resolution Output Schema And Partial Source Derivation

The next real run with deferred queue audit exposed a field-contract gap after
entity resolution application. A prior batch had produced a record artifact
whose metadata declared target fields, but all rows were unmatched on that
dimension. Because the runner only wrote target fields when a row matched, the
materialized record payload and headers did not contain the declared target
columns. A later deferred `derive_fields` action then saw those fields as
missing and discarded the queue.

The same run also showed a generic multi-source derivation issue. Operations
such as `coalesce` and `concat` are designed to use whichever declared source
fields are available and non-empty. The old validator treated every listed
source field as mandatory, so a fallback field that was intentionally absent
could block a valid bounded DAG step.

These are domain-neutral schema stability bugs. Any data task that performs
mapping, lookup, enrichment, OCR/text extraction, JSON transformation, row
filtering, contribution computation, or final projection can have unmatched
rows and fallback field lists. The runtime must preserve declared output
schemas and distinguish required source fields from optional fallback sources.

Changes:

- [x] `apply_entity_resolutions` now materializes declared target ID and label
      fields on every output row, even when the specific row is unmatched or
      ambiguous.
- [x] Output artifact headers now include those declared target fields, so
      downstream guards and planners see a stable executable record schema.
- [x] Unmatched rows keep empty target values plus the declared status field,
      preserving auditability without inventing business values.
- [x] `derive_fields` validation now allows partial source availability for
      multi-source operations whose semantics are fallback/aggregation over
      fields: `coalesce`, `first_non_empty`, `concat`, `join`, and
      `join_fields`.
- [x] REPL/CLI workflow staging now applies the same partial-source semantics,
      so deferred typed actions are not discarded before the deterministic
      runner can execute them.
- [x] Added regression coverage for unmatched resolution target fields,
      partial-source `derive_fields` execution, and workflow guard acceptance
      of fallback field lists.

Checklist audit:

- The remaining unchecked items in earlier batches are intentionally retained
      when they describe broader architecture, such as first-class lineage,
      value-coverage actions, typed evaluator violation objects, and persistent
      resume state. This batch closes only the stable record-schema and
      fallback-source invariants proven by code and tests.
- The deferred-queue progress checklist item in Batch 110 has been refreshed:
      later runtime/audit batches implemented in-memory queue length, first
      action, ready/blocked counts, and structural block reasons in CLI/REPL.
      Persisting the queue across interrupted sessions remains open.

Remaining architecture items:

- [ ] Promote generated artifact schemas into a first-class executable
      `artifact_schema_projection`, so validators and prompts consume the same
      exact field contract without relying on compact samples.
- [ ] Add action lineage summaries from source material to generated record
      artifact to derived/joined/contribution artifacts, so planners can pick
      the newest compatible artifact deterministically.
- [ ] Feed stable-output-schema fixes and partial-source inferences into typed
      evaluator state, not only artifact diagnostics, so later repair turns can
      explain why a field is present but empty for unmatched records.

### Batch 151: Field-Contract Candidate Hints And Alias Scope Isolation

The next live run advanced beyond stable entity-resolution output fields and
deferred partial-source derivation. It then exposed two generic workflow
diagnostic gaps:

- a deferred action consumed a record artifact that lacked a needed field, even
  though another already-materialized artifact contained that field;
- a source material path was shadowed by a parent material-inventory metadata
  artifact because parent artifact aliases recursively included child aliases.

These are not domain-specific. Any adaptive data workflow can produce multiple
record artifacts with overlapping lineage: expanded rows, filtered rows,
joined rows, extracted text records, OCR records, mapping ledgers, and
intermediate projections. When an action selects the wrong artifact, the repair
loop needs objective candidate artifacts from the field catalog. And when a
parent artifact has children, the parent must not claim child aliases; otherwise
metadata objects can masquerade as executable source/record inputs.

Changes:

- [x] Field-contract guard errors now include candidate artifact hints when
      existing artifacts contain all or part of the missing field set. The
      hint is structural: alias plus matched fields, not business prose.
- [x] Deferred-queue blocked reasons inherit the same richer field-contract
      message, so CLI/REPL logs show why a queued action is blocked and which
      artifacts may satisfy the field need.
- [x] Artifact aliases are now scoped to the artifact itself. Children are
      still walked and exposed independently, but their aliases are no longer
      copied into the parent artifact.
- [x] This prevents parent metadata artifacts such as material inventories from
      shadowing child/source paths in `artifact_access` lookup.
- [x] Added regression coverage for missing-field candidate artifact hints and
      parent/child alias shadowing.

Checklist audit:

- This batch closes one narrower part of the earlier action-lineage backlog:
      repair turns now get concrete candidate artifacts for missing fields.
      It does not complete full lineage or automatic artifact substitution.
- The broader lineage item remains open because the system still should expose
      typed source -> generated artifact -> derived artifact ancestry and use
      it in evaluator state before planner repair.

Remaining architecture items:

- [ ] Add first-class action lineage summaries so the planner can choose among
      multiple artifacts with related fields without relying only on aliases.
- [ ] Promote field-contract failures into typed evaluator violation objects
      with candidate artifact lists, instead of embedding them only in guard
      text.
- [ ] Add a deterministic field-compatible artifact substitution only for
      cases where structural lineage proves equivalence. Do not silently swap
      inputs based solely on matching field names.

### Batch 152: Deferred Dispatch Uses Full Workflow Guard

The next live run confirmed that deferred typed ranks improve convergence, but
also exposed a stale-queue contract leak. After `derive_fields` materialized a
record artifact, the runtime resumed a deferred `qualify_records` action whose
inputs were all visible. The action was still invalid because `qualify_records`
is a single-record-set operation and the deferred action carried two
`input_paths`. The old deferred dispatcher checked input availability and field
contracts, but did not re-run the full workflow staging guard before permanent
render/execution.

This is a generic adaptive-DAG issue. Deferred actions are not prose, but they
are also not permanently valid just because they came from an earlier accepted
plan. A later workflow state may make an action stale, over-broad, or invalid
for its action contract. Every deferred rank must be treated as a fresh typed
candidate over current facts.

Changes:

- [x] Re-run the full `dataTaskWorkflowStagingGuardError` on every deferred
      dispatch candidate after readiness/rank splitting and before execution.
- [x] Block stale deferred ranks that violate action contracts, such as
      single-record-set actions with multiple inputs, instead of rendering them
      as ready and discovering the problem during execution/repair.
- [x] Surface the same full guard reason through deferred queue status so
      CLI/REPL progress says why the queue was discarded.
- [x] Keep the behavior domain-neutral: the dispatcher validates typed action
      contracts and current workflow state, not business-specific values or
      model prose.
- [x] Add regression coverage for a deferred `qualify_records` action whose
      inputs exist but whose action shape is invalid.

Checklist audit:

- This tightens the Batch 110 deferred readiness item: deferred ranks now
      require both available dependencies and a current full staging-guard
      pass.
- It does not complete persistent deferred-queue resume state or automatic
      field-compatible substitution, which remain separate architecture items.

Remaining architecture items:

- [ ] Persist deferred queues in the workflow audit record so interrupted
      sessions can resume typed graph state across process restarts.
- [ ] Promote blocked deferred guard reasons into typed evaluator state so the
      next planner turn can expand from the precise violation object rather
      than from low-noise progress text.
- [ ] Add deterministic field-compatible substitution only when structural
      lineage proves equivalence, not merely matching field names.

### Batch 153: Payload Header Union For Sparse Record Sets

The latest real run exposed a schema-reporting gap. The data workflow had
actual entity-resolution samples containing `canonical_id` and
`canonical_label`, but the compact artifact catalog told the planner that the
resolution artifact only had fields such as `item_id`, `source_value`, and
`status`. The model correctly noticed the mismatch, but then spent additional
rounds re-normalizing and retrying joins because the executable catalog looked
untrustworthy.

This is not specific to entity resolution or procurement data. Any generated
record set may be sparse: the first row can be unresolved, empty, partial, or
diagnostic-only while later rows carry the fields needed by downstream
actions. Schema inference must therefore summarize the record set contract,
not the first non-empty object.

Changes:

- [x] `dataActionArtifactPayloadHeaders` now unions keys across a bounded
      sample of array records instead of stopping after the first object.
- [x] Wrapper payloads now prefer real record containers such as `records`,
      `rows`, `items`, `contributions`, `entity_resolutions`, and
      `rule_coverage` before falling back to outer metadata keys.
- [x] Empty wrapper record sets use explicit `headers` as the executable field
      contract instead of exposing wrapper-only fields such as diagnostics.
- [x] Added regression coverage for sparse array records whose later rows carry
      additional fields and for empty wrappers with explicit headers.

Checklist audit:

- This closes one concrete field-catalog correctness bug. It does not complete
      the broader `artifact_schema_projection` backlog, because prompts and
      validators still do not consume a single first-class schema object across
      every action boundary.

Remaining architecture items:

- [ ] Promote generated artifact schemas into a first-class executable
      `artifact_schema_projection` shared by planner prompts, staging guards,
      deferred dispatch, and action validators.
- [ ] Add typed schema-drift audit metrics when inferred record fields differ
      from wrapper headers, action-declared output fields, or sample records.

### Batch 154: Preflight Model Candidate Plans Before Permanent Rendering

The same real run also showed a UX and auditability leak: a model-generated
repair or continuation plan could be rendered and written as `ready` before the
workflow staging guard converted it into a safer typed fallback. The runtime
was generally safe because the invalid plan was rejected before execution, but
users and logs still saw misleading ready plans and broad scripts that the
system never intended to run.

The invariant is now:

- model-authored initial, repair, and continuation candidates are preflighted
      through the same typed workflow staging guard before permanent display or
      audit;
- if a deterministic fallback exists, the original rejected candidate is kept
      as a workflow record with its guard reason, while REPL/CLI display the
      executable fallback plan and any deferred typed suffix;
- the preflight path remains structural: it consumes action kinds,
      dependencies, material coverage, and typed field contracts, not model
      prose or business keywords.

Changes:

- [x] Added shared `dataTaskPreflightWorkflowPlan` for candidate-plan
      acceptance.
- [x] CLI data workflow now uses the shared preflight for initial plans,
      repair plans, continuation plans, and evaluator-requested repairs before
      audit/progress output.
- [x] REPL data workflow now uses the same preflight for the matching
      model-generated candidate paths before permanent panel rendering and
      data-audit writes.
- [x] Preflight preserves deferred DAG suffixes produced by deterministic
      fallbacks such as intra-batch dependency splitting.
- [x] Added regression coverage proving a candidate plan with a same-batch
      artifact dependency is rewritten into an executable prefix plus deferred
      suffix before audit/display.

Checklist audit:

- This completes the narrower "preflight before permanent rendering" invariant
      for model candidate plans in current CLI/REPL data workflows.
- It does not complete the larger typed evaluator violation backlog. Guard
      reasons are still mostly text; future work should promote them into
      structured evaluator objects.

Remaining architecture items:

- [ ] Promote preflight guard failures into typed evaluator state so the next
      planner turn receives precise violation codes, JSON paths, and candidate
      artifacts instead of low-noise text summaries.
- [ ] Add per-run audit counters for candidate plans rewritten before display,
      including fallback kind, deferred suffix length, and rejected action kind.

### Batch 155: Checklist Status Consolidation

The user review correctly called out that the design document now contains many
unchecked items. This is partly expected and partly a documentation risk. The
document has become both an implementation ledger and an architecture backlog,
so older broad checklist items can look stale after later batches deliver
narrower slices.

Audit outcome:

- Completed items marked `[x]` in Batches 109-154 are backed by code and
      regression coverage unless the item explicitly says it is a status note.
- Several older unchecked items have been partially covered by later batches,
      but are intentionally not marked complete because the original item asks
      for a broader architecture boundary than the delivered slice.
- The biggest remaining open themes are first-class artifact schema contracts,
      typed evaluator violation objects, value/mapping coverage actions,
      action lineage summaries, persisted/resumable DAG state, and structured
      audit metrics.

Consolidated status for the review hotspots:

- Workflow material floors vs current-batch inputs: delivered for execution
      behavior in Batch 109; prompt objects and promotion metrics remain open.
- Deferred DAG queue readiness: delivered in-memory queue splitting,
      readiness checks, full staging-guard rechecks, and low-noise visibility
      across Batches 110, 124, and 152; interrupted-session persistence remains
      open.
- Lookup/enrichment role consistency: partially delivered through join/enrich
      guidance, role-explicit lookup contracts, normalize side inference, and
      apply-resolution actions in Batches 113, 134, 140, 149, and 151; a
      shared role-bearing input IR for every typed action remains open.
- Field/filter diagnostics: delivered missing-field errors, empty-ledger
      diagnostics, filter diagnostics, schema-aware filter repair, and
      candidate artifact hints across Batches 111, 119, 120, and 151; typed
      evaluator violation objects and deterministic value-coverage actions
      remain open.
- Artifact visibility and schema: delivered newest-artifact preference,
      deduplicated access hints, record artifact preference, schema-bearing
      empty records, and sparse-header union across Batches 112, 115, 120, and
      153; a single executable `artifact_schema_projection` remains open.
- UX transparency: delivered split title/detail data-plan UX, full audit
      artifact paths/logs, deferred queue status, result JSON audit, and
      preflight-before-rendering across Batches 112, 124, 152, and 154; typed
      audit counters remain open.
- Preflight before permanent rendering: delivered for current CLI/REPL model
      candidate plans in Batch 154; future new model-plan entrypoints must use
      the shared preflight helper.

Documentation policy:

- Do not bulk-convert old `[ ]` items to `[x]` only because a later batch
      delivered a related symptom fix.
- When a later batch narrows an older open item, add a status note or a
      consolidated status bullet like this one.
- Mark an older item complete only when the exact architecture boundary is
      implemented in code, consumed by both CLI and REPL paths when applicable,
      and covered by regression tests.

Remaining architecture items:

- [ ] Add a short status index near the top of this document so reviewers can
      distinguish shipped invariants, partially shipped architecture themes,
      and open backlog without scanning all batches.
- [ ] Add a CI-style doc lint that detects newly added broad checklist items
      without a clear "done means" boundary.

### Batch 156: Locator Fields, Stable Join Schemas, And Input Availability

The next compiled real-scenario run progressed through the adaptive DAG but
still spent long turns around generated-artifact reuse. The failures were
structural:

- a `derive_fields` action created `row_index` with a constant value, which
      destroyed the join key cardinality needed by later actions;
- a left join with zero right-side matches preserved left rows but did not keep
      the predicted right-side schema visible on each generated record;
- an action could reference a future bare alias because top-level
      `plan.input_paths` were merged into the availability set before the
      action dependency guard ran.

These are not tied to any business domain. They apply to any adaptive data DAG
that needs stable source locators, joins, filtering, contribution calculation,
or final projection over intermediate artifacts.

Changes:

- [x] Expose source locator virtual fields to `derive_fields`, including
      `_source_path`, `_source_index`, `_source_line`, `_source_locator`, and
      non-reserved readable aliases such as `source_index` and `row_index`.
- [x] Reject `derive_fields` attempts to overwrite reserved system locator
      fields such as `_source_index`.
- [x] Reject constant derivation into locator-like target fields such as
      `row_index` or `source_index`; models must copy or parse real source
      locator values instead of inventing row identity.
- [x] Preserve predicted joined headers for left-join unmatched rows and fill
      missing right-side fields with blank values so downstream action field
      contracts remain executable.
- [x] Add join diagnostics for matched left rows, unmatched left rows,
      match rate, and zero-match status.
- [x] Split workflow availability from top-level plan inputs. Existing
      generated artifacts and prior same-batch outputs are available facts;
      current top-level inputs only count when they carry structural external
      material path signals such as file extension, slash, or absolute/relative
      path syntax.
- [x] Add regression coverage for locator-copy derivation, constant-index
      rejection, left-join unmatched schema preservation, future-alias
      rejection, and nested artifact alias availability.

Audit outcome:

- The review concern about many unchecked items was valid. This batch does not
      mark broad backlog items complete unless the exact code boundary was
      delivered. It narrows the "artifact schema/availability" theme but still
      leaves the larger `artifact_schema_projection` and typed evaluator
      violation objects open.

Remaining architecture items:

- [ ] Promote join/filter/contribution diagnostics into typed evaluator
      violation objects rather than only prompt text and artifact fields.
- [ ] Add executable `artifact_schema_projection` records so validators can
      reason about generated artifact fields without relying on sampled prompt
      catalogs.
- [ ] Add action lineage summaries that name the producer action and exact
      source locator fields used for every generated join/filter/contribution
      artifact.

### Batch 157: Preserve Base Rows When Applying Sparse Resolution Ledgers

The next real-scenario run showed a generic row-cardinality leak. The workflow
correctly built source records and entity-resolution ledgers, but an
`apply_entity_resolutions` action used base/source filters to select only rows
that needed mapping. The old runner then treated those filters as an output
row filter. Later actions consumed a six-row generated artifact even though the
base record artifact still had fifty rows. The DAG could not converge because
the record universe had already been accidentally narrowed.

This is not tied to a specific business field. Any data task can have sparse
resolution evidence: only some rows need a lookup, OCR extraction, human-like
mapping, normalization, enrichment, or reference repair. Applying that sparse
evidence must not silently drop the rest of the base table unless the plan
explicitly asks for a filtered output.

The invariant is now:

- `apply_entity_resolutions` is a left-application action by default;
- base/source filters mark which base rows are eligible to receive a mapping,
      not which rows survive in the output;
- non-eligible base rows remain visible with `not_applicable` status fields;
- eligible rows without a matching resolution remain visible with the existing
      unmatched status;
- row shrinking remains available only through an explicit structural mode
      such as `base_filter_mode=filter_output`, or through a separate
      `filter_records` action.

Changes:

- [x] Changed `apply_entity_resolutions` to preserve all base rows by default
      even when base/source filters are present.
- [x] Added `base_filter_mode` / `source_filter_mode` / `output_mode` support
      for explicit output filtering when a filtered result is intentional.
- [x] Added artifact diagnostics for `base_filter_mode` and
      `not_applicable_<target_field>` counts.
- [x] Kept unmatched and ambiguous diagnostics for eligible rows unchanged.
- [x] Updated initial planner prompt, continuation prompt, action scaffolds,
      and workflow action contracts so the model sees one consistent
      domain-neutral apply-resolution contract.
- [x] Added regression coverage for default base-row preservation and explicit
      filtered-output mode.

Audit outcome:

- This narrows the "mapping/enrichment boundary" and "row-loss after joins or
      resolution application" backlog, but it does not complete the broader
      action-lineage or artifact-schema-projection architecture items.
- The document still has many unchecked items because they represent broader
      architecture boundaries. They should remain unchecked until code,
      prompts, CLI/REPL paths, and regression tests cover the full boundary.

Remaining architecture items:

- [ ] Add action lineage summaries so a later planner can distinguish full
      base-row artifacts from sparse resolution/helper artifacts without
      relying on alias names.
- [ ] Feed row-cardinality changes from apply/enrich/join/filter actions into
      typed evaluator state as structured diagnostics.
- [ ] Add a compact deterministic scaffold that assembles the common sequence
      `apply_entity_resolutions -> derive/coalesce fields -> qualify/filter ->
      compute_contributions -> reconcile -> assemble_answer` from current
      artifact contracts when the model repeatedly restates the same next
      steps without emitting a ready batch.

### Batch 158: Separate Prompt Artifact Samples From Hard-Gate Schemas

The follow-up real run progressed further: `apply_entity_resolutions` now
preserved the full base record set and a later join produced contribution
candidates. The next failure was a hard-gate false negative. A generated join
artifact really contained fields such as `status_is_valid`, `is_cny`,
`amount_system`, and `query_id`, but the workflow field guard reused the
prompt-facing compact artifact view. That view intentionally clamps fields to
keep the model prompt small. Because `status_is_valid` appeared after the
compact field limit, the guard reported it as missing and pushed the workflow
into unnecessary repair/join rounds.

This is a generic architecture issue. Prompt views are lossy by design. Hard
structural gates must consume precise contract signals, not lossy prompt
samples.

The invariant is now:

- prompt/UX artifact availability may stay compact and low-noise;
- workflow hard gates must use an internal contract artifact view with full
      aliases and full field lists;
- the contract view is newest-first and deduplicated independently from prompt
      sampling;
- field-contract decisions must not depend on the prompt field limit.

Changes:

- [x] Added `dataTaskWorkflowArtifactContractAccess` for internal hard gates.
- [x] Switched `dataTaskActionFieldContractGuardError` to the full contract
      artifact view instead of prompt-sampled availability.
- [x] Kept prompt-facing `artifact_availability` compact, preserving context
      size and UX behavior.
- [x] Added regression coverage where a required field appears after the
      prompt compact field limit; the staging guard now accepts it because the
      full contract schema contains it.

Audit outcome:

- This directly narrows the `artifact_schema_projection` theme but does not
      fully complete it. The new contract view is still derived from artifact
      headers/fields rather than a persisted typed schema object with lineage
      and producer metadata.

Remaining architecture items:

- [ ] Promote the full contract artifact view into a persisted
      `artifact_schema_projection` with producer action, aliases, row count,
      field origins, and confidence/diagnostic metadata.
- [ ] Feed field-contract false-negative/repair statistics into audit metrics
      so customer runs can explain whether loops were caused by planner
      mistakes, schema truncation, or real missing fields.
- [ ] Add deterministic continuation scaffolds that consume the full contract
      view directly when the model repeatedly restates a valid compute/reconcile
      next step without emitting an executable batch.

### Batch 159: First-Class Data Workflow IR Boundary

The latest architecture review identified the main risk in the current
direction: convergence was increasingly achieved by adding REPL/CLI guards
around model mistakes. Those guards are necessary for safety, but they should
not become the primary architecture. The data lane needs a clearer
intermediate representation and state machine so the model is naturally
constrained by typed state, graph readiness, and ledger contracts instead of
being repeatedly blocked after emitting invalid plans.

The latest real-scenario run was intentionally stopped after enough evidence
was collected. It progressed through material inspection, record extraction,
and entity normalization, which shows the typed DAG direction is still correct.
It also confirmed the next architecture gap: the workflow state is still too
scattered across REPL prompt views, staging guards, action availability helpers,
and evaluator summaries. That makes it easy to fix a symptom while leaving the
underlying state model implicit.

2026-06-07 smoke update: a freshly built binary was run again against the same
real local data aggregation directory. The first model candidate still proposed
a broad `custom_transform`; the system rejected/converted it into material
discovery and resumed typed actions. That is safe, but it proves the next
quality bar is not another one-off guard. The model should receive an explicit
workflow IR with current stage, action capability, graph readiness, and ledger
requirements before candidate rendering/execution, so the first executable
batch is naturally typed and bounded.

The architecture boundary is now:

- `DataWorkflowState` is a first-class IR, not a REPL rendering detail;
- Action DAG, Material Graph, and Ledger Graph are separate projections of the
  same workflow facts;
- prompt views are lossy summaries derived from the IR;
- hard gates consume precise IR projections and typed violations;
- `custom_transform` remains a leaf fallback for bounded gaps, not the main
  convergence mechanism;
- eval must include real complex cases, repeated-run volatility checks, and
  status gates instead of only small mechanism fixtures.

Changes:

- [x] Added `internal/dataworkflow` as the first package-level boundary for
      data workflow IR types: `WorkflowState`, `MaterialGraph`,
      `ActionGraph`, `LedgerGraph`, workflow progress, decisions, and
      artifact schema projections.
- [x] Moved hard-gate artifact schema projection into the new data workflow
      boundary through `ProjectArtifactSchemasNewestFirst`.
- [x] Kept prompt-facing artifact previews compact while switching
      field-contract guards to the full schema projection.
- [x] Made executable alias/path identity outrank debug IDs in artifact schema
      projection so a newer record artifact with the same execution alias is
      preferred over an older summary/metadata artifact.
- [x] Added a shared action capability table with dependency ranks,
      ledger-producing capabilities, and leaf-fallback metadata.
- [x] Switched REPL workflow action-kind normalization, dependency-rank checks,
      and typed ledger requirement promotion to consume the shared capability
      table instead of separate local switches.
- [x] Added regression coverage for full-field preservation, newest-alias
      preference, child artifact projection, action dependency ranks,
      ledger-producing capabilities, and `custom_transform` leaf-fallback
      classification.
- [x] Added this status index so broad unchecked items remain visible as real
      architecture backlog rather than stale progress markers.

Done means for the next architecture phase:

- move the workflow-state reducer out of REPL into `internal/dataworkflow`;
- represent Action DAG nodes, dependencies, deferred queues, and readiness as
  typed state consumed by both CLI and REPL;
- represent Material Graph floors, current inputs, optional evidence, generated
  artifacts, and promotion/rejection reasons as typed state;
- represent Ledger Graph requirements, available ledger handles, reconcile
  groups, final projection, and de-duplication metrics as typed state;
- make evaluator decisions consume typed state and typed violations rather than
  low-noise prose summaries;
- keep all user/model prose as soft guidance unless a schema-validated typed
  field carries the hard decision signal;
- add real-scenario eval fixtures with multi-run variance reporting and
  publish a combined status gate before considering data lane default-on
  readiness.

Remaining architecture items:

- [ ] Move `dataTaskWorkflowStateView` construction and workflow progress
      calculation into `internal/dataworkflow` as a deterministic reducer.
- [ ] Move action capability tables and allowed-next-action contracts into
      `internal/dataworkflow`, with REPL/CLI using them as adapters.
- [ ] Persist an Action DAG snapshot containing executed, ready, deferred, and
      blocked nodes plus dependency aliases and producer stages.
- [ ] Persist Material Graph snapshots that separate workflow hard floors,
      current-batch inputs, optional evidence, generated artifacts, and
      rejected promotions.
- [ ] Persist Ledger Graph snapshots with rule, decision, entity-resolution,
      contribution, reconcile, and final-projection handles.
- [ ] Promote guard failures into typed violation objects with action id,
      action kind, JSON path or field path, expected shape, actual shape, and
      repairability.
- [ ] Add domain-neutral value-coverage and mapping-candidate actions as graph
      expansion tools, not business-specific patches.
- [ ] Build real-scenario eval gates that run representative data tasks
      multiple times, compare final answer correctness, inspect required
      ledgers/reconcile state, and report volatility before merge/release.

### Batch 160: Initial Action DAG Rank Preflight

The next freshly built real-scenario run showed a different convergence leak.
The first model plan no longer started as one large top-level Python script,
but it still emitted a multi-rank typed action batch:

- material discovery / record extraction;
- rule derivation;
- entity normalization.

Because there was no accepted workflow record yet, the existing stage-prefix
fallback did not run. The batch reached execution, and the later
`normalize_entities` node failed because its structured inputs were not yet
represented as materialized artifact aliases. The system recovered on the next
round, but this is still "model first collides with the wall, system repairs
afterward".

This is not a procurement-specific issue. Any data task can begin with an
over-eager graph: inspect + extract + normalize, extract + filter + join,
derive + compute + reconcile, or material discovery plus a downstream
projection. The initial preflight must treat the candidate as an Action DAG,
not as a flat list of actions.

The invariant is now:

- action dependency rank belongs in `internal/dataworkflow`, not REPL-only
  prompt code;
- before the first successful workflow record exists, a candidate batch that
  crosses typed dependency ranks is split before audit display/execution;
- rank-0 discovery/materialization actions may remain with the first non-zero
  rank, but later ranks become a deferred suffix;
- this split is structural only: it does not inspect business prose, field
  names, file names, or customer-specific semantics.

Changes:

- [x] Add a dataworkflow helper that splits the first executable action-rank
      prefix from a candidate Action DAG using the shared capability table.
- [x] Wire initial no-history preflight through that helper before execution
      and before permanent "ready plan" display.
- [x] Keep existing deferred-queue handling so the suffix can resume only after
      the prefix materializes real artifacts and field contracts.
- [x] Add regression coverage for an initial multi-rank typed plan being split
      before execution.

Remaining architecture items:

- [ ] Promote the deferred suffix itself into persisted Action DAG state rather
      than carrying it as REPL-local plan remainder.
- [ ] Add typed action parameter contracts so a node such as
      `normalize_entities` can be rejected with a structured contract violation
      before execution even when it is the first rank in a future batch.
- [ ] Move the full `dataTaskWorkflowStateView` reducer into
      `internal/dataworkflow` so initial, continuation, repair, deferred, and
      completion paths all consume the same state machine.

### Batch 161: Closed Deterministic Preflight Rewrites

The Batch 160 real-scenario smoke confirmed that initial rank splitting works,
but it exposed a second orchestration gap: a deterministic fallback can produce
a candidate prefix that is still not executable. In the observed run, an
intra-batch dependency split isolated a broad `custom_transform`; that prefix
then failed the broad-script guard during execution and only later converted to
material discovery.

This is a workflow-closure problem, not a business-domain issue. Any fallback
may create a new candidate plan: rank-prefix trimming, intra-batch dependency
splitting, material discovery conversion, coverage expansion, disabled-script
fallback, or no-tool deterministic continuation. A fallback result must be
validated before it is rendered as ready or executed.

The invariant is now:

- preflight is a bounded deterministic rewrite loop;
- every fallback-produced candidate is re-run through staging/preflight;
- deferred suffixes are preserved when a prefix needs another deterministic
  rewrite;
- precise prerequisite and intra-batch dependency diagnostics still win over
  broad rank splitting when the direct guard is more informative;
- no fallback uses model prose, file names, column names, or business-domain
  keywords as hard signals.

Changes:

- [x] Changed data workflow preflight into a bounded rewrite loop with a small
      maximum rewrite budget.
- [x] Revalidated fallback-produced plans before permanent display/execution.
- [x] Preserved deferred action suffixes across chained rewrites.
- [x] Kept initial rank splitting as a preflight rewrite rather than a direct
      staging-guard error, preserving existing precise prerequisite diagnostics.
- [x] Added regression coverage for a broad custom-transform prefix being
      rewritten again into material discovery while preserving the original
      dependent suffix.

Remaining architecture items:

- [ ] Promote the preflight rewrite chain into typed evaluator/audit state so
      users can inspect which rewrite rules fired, in what order, and which
      suffixes were preserved.
- [ ] Persist deferred suffixes and rewrite reasons in the Action DAG snapshot.
- [ ] Add typed action parameter contracts so invalid node shapes can be
      rejected before execution even when no fallback rewrite is needed.

### Batch 162: Deferred Normalize Source Artifact Compatibility

The next smoke run advanced past initial discovery and extraction. It then hit
a deferred-queue block: a `normalize_entities` node had been deferred after a
previous normalization/enrichment node, but its source-side input pointed at an
intermediate artifact that did not contain the source field required by the
normalization contract. A compatible source record artifact with that field was
already present in the workflow artifact schema catalog.

This is a generic Action DAG lineage issue. Independent normalization nodes can
be accidentally serialized through the wrong intermediate artifact. The system
should not infer business semantics, but it can use structural field contracts
to select a compatible source record artifact when all required source fields
are present.

The invariant is now:

- this rewrite is source-side only for `normalize_entities`;
- it consumes artifact schema projections and required source field names;
- it does not change fields, values, mappings, filters, reference inputs, or
  business rules;
- it does not apply to contribution/join/final projection actions, because
  those often intentionally wait for a specific generated artifact;
- if no structurally compatible artifact exists, the deferred node remains
  blocked and the evaluator/planner must expand the graph.

Changes:

- [x] Added a deferred-queue rewrite that redirects a blocked
      `normalize_entities` source input to an existing artifact containing all
      required source fields.
- [x] Kept contribution and other downstream compute actions strict: they do
      not substitute a different artifact when the declared generated input is
      missing required fields.
- [x] Added regression coverage for the normalize source-side rewrite.
- [x] Preserved existing regression coverage that compute contributions wait
      for the exact generated artifact and required fields.

Remaining architecture items:

- [ ] Generalize this into a typed Action DAG lineage engine with explicit
      producer/consumer roles and safe substitution policies per action kind.
- [ ] Persist compatibility rewrites and rejected substitutions in audit state.
- [ ] Add value-coverage/mapping-candidate actions so normalization can inspect
      source/reference overlap without hand-authored mappings or broad scripts.

### Batch 163: Action Input Path Contracts In Workflow IR

The latest smoke also showed that some structural action requirements still
lived only in REPL staging code. For example, `derive_fields`, `filter_records`,
`qualify_records`, and `compute_contributions` are single-record-set actions:
they need one existing source material or generated artifact before they can
run. Keeping those constraints as ad hoc REPL switches makes it harder to share
the same truth across CLI, REPL, planner hints, deterministic fallbacks, and
future action-schema projection.

This is not a data-domain rule. It is an Action DAG contract: every typed node
has objective input-shape requirements independent of what the records mean.
Business meaning remains with the model; structural execution requirements
belong in workflow IR.

Changes:

- [x] Added `ActionInputPathContract` to `dataworkflow.ActionCapability`.
- [x] Declared domain-neutral input path requirements for typed actions whose
      shape is already structurally clear: material inspection/extraction,
      single-record-set transforms, joins, and contribution computation.
- [x] Updated REPL workflow staging to consume the shared capability table
      instead of duplicating the input-path hard gate in a local switch.
- [x] Preserved action-specific repair guidance where it carries structural
      value, such as contribution inputs needing existing record artifacts.
- [x] Added regression coverage for the capability-level input contracts and
      re-ran focused workflow tests.

Remaining architecture items:

- [ ] Extend action contracts from input-path counts to full typed parameter
      schemas: required params, alternative params, field refs, output alias
      requirements, and safe structured defaults.
- [ ] Move contract validation into `internal/dataworkflow` so CLI, REPL,
      continuation, repair, deferred execution, and completion repair all call
      one validator before any plan is displayed as executable.
- [ ] Emit typed contract violations with JSON paths and repairability labels
      instead of only prose guard strings.
- [ ] Expose the action contract table to planner prompts as compact schema
      facts, not as business examples or keyword instructions.

### Batch 164: Validated-Only Data Plan Display

The next compiled real-scenario smoke confirmed that input-path contracts are
being enforced: a candidate continuation batch emitted `derive_fields` without
`input_paths`, and preflight produced the correct structural failure. However,
the REPL/CLI display path still rendered that candidate as `数据计划 · 就绪`
before the loop repaired it. That is misleading: a model-authored candidate
plan is not an executable plan until it passes deterministic preflight.

This is a workflow-state boundary issue, not a data-domain issue. The UI and
CLI should distinguish:

- candidate plans emitted by the model;
- rejected plans that are retained for audit and repair context;
- validated executable plans that may be shown as ready and then executed.

The invariant is now:

- preflight keeps historical guard reasons for audit/rewrite state;
- preflight separately marks `FinalGuardErr` when the final candidate is still
  not executable;
- plans with `FinalGuardErr` are written to audit logs with a rejected scope
  but are not rendered as ready plans in REPL or CLI progress;
- successful deterministic rewrites remain displayable and executable.

Changes:

- [x] Split preflight guard state into historical `GuardErr` and final
      `FinalGuardErr`.
- [x] Updated REPL data plan acceptance so non-displayable candidates are
      audited as rejected instead of shown as ready.
- [x] Updated CLI data plan progress the same way; stderr progress now only
      shows validated executable plans.
- [x] Added regression coverage for both successful fallback displayability and
      rejected candidate marking.

Remaining architecture items:

- [ ] Replace ad hoc REPL/CLI acceptance closures with a shared
      `ValidatedPlanEnvelope` returned by `internal/dataworkflow`.
- [ ] Persist rejected candidate summaries, rewrite chains, and deferred suffix
      snapshots in one Action DAG audit record.
- [ ] Move direct fallback branches in the execution loop behind the same
      validated-plan acceptance path so every generated plan goes through the
      same final display gate.
