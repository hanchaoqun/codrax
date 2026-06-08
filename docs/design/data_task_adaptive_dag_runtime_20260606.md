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
- [x] Add a compact value-distribution preview action for generic field
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

### Batch 165: RecordSet Completeness IR For Exact Data Workflows

The next real-scenario smoke moved in the right direction: the workflow
materialized separate record artifacts for purchase orders, vendors,
categories, and queries instead of a single mixed-schema record set. It then
hit a generic record-materialization gap. `extract_records` defaults to a
bounded sample of 20 records. The artifact already reported the true row count
and sample count, but the workflow still treated the sampled artifact as a
usable input for later exact normalization, contribution, reconciliation, and
final projection. The model noticed the mismatch in reasoning and spent
additional rounds asking to reload the same files with a larger limit.

This is not specific to purchase data. Any exact data task can fail the same
way: CSV aggregation, JSONL statistics, spreadsheet-like audit, OCR-derived
record joins, compliance filtering, or strict output projection. A sample is
valid for schema discovery, but it is not a complete computation input.

The invariant is now:

- record artifacts carry explicit completeness metadata:
  `record_completeness`, `sample_count`, `total_rows`, and child-level
  completeness;
- `extract_records` remains a bounded action, but exact workflows that require
  contributions/reconciliation/final projection default to a high bounded
  record limit before execution;
- explicit diagnostic sampling remains possible through structural
  `sample_only`, `schema_only`, or `preview_only` params;
- no business file names, column names, currencies, dates, or domain keywords
  drive the hard behavior.

Changes:

- [x] Added record-set completeness fields to extract-record artifacts and
      child material artifacts.
- [x] Mark parent extract artifacts as `complete` only when every child record
      set covers its total rows; otherwise mark them `sampled`.
- [x] Added workflow execution normalization that raises `extract_records`
      limits to a bounded full-record limit for exact contribution/reconcile
      workflows when the action is not explicitly marked sample/preview-only.
- [x] Added tests for complete vs sampled extract artifacts.
- [x] Added tests that exact workflows receive full-record extraction limits
      while sample-only extraction remains untouched.

Remaining architecture items:

- [ ] Promote `record_completeness` into a first-class `ArtifactSchemaProjection`
      consumed by action validators before `derive_fields`,
      `normalize_entities`, `filter_records`, `compute_contributions`, and
      `assemble_answer`.
- [ ] Add a deterministic guard that blocks contribution/reconcile/final
      projection when an input artifact is still sampled and no explicit
      sample-only diagnostic path is active.
- [x] Add a value-distribution typed action so schema/value exploration uses
      small diagnostic artifacts, while exact calculation uses complete record
      artifacts.
- [ ] Add any further field-preview actions only when they expose new typed
      observations that `value_distribution` and artifact schema projection do
      not already cover.

### Batch 166: Prompt-Safe Artifact Schema Projection

After Batch 165, a follow-up real-scenario smoke showed the desired execution
behavior: exact `extract_records` actions materialized complete record artifacts
instead of 20-row samples. The next generic bottleneck moved one layer up. The
prompt-side workflow state still projected child artifact samples recursively,
so complete record artifacts could inflate continuation prompts even though
the executor already had durable artifact files and aliases.

This is not a domain issue. Any exact data workflow can produce large
intermediate artifacts: parsed tables, normalized records, joined record sets,
extracted text spans, OCR outputs, filtered subsets, contribution ledgers, or
final projections. The model needs a compact schema/availability projection,
not the full executable data payload.

The invariant is now:

- executor artifacts remain complete and durable for typed actions;
- model prompts receive bounded artifact projections only;
- top-level artifacts, child artifacts, and nested diagnostic artifacts are
  compacted recursively;
- large field catalogs and samples are clamped before entering continuation
  prompts;
- this is a structural prompt-shaping rule, not a business-specific rule.

Changes:

- [x] Replaced one-level artifact sampling with recursive prompt compaction.
- [x] Added fixed budgets for top-level samples, child samples, child counts,
      headers, source paths, summaries, and field values.
- [x] Kept full artifact files and audit artifacts untouched so deterministic
      runners can still consume complete data.
- [x] Added regression coverage that parent, child, and nested diagnostic
      samples are compacted and too-deep child trees are removed from prompt
      projection.

Remaining architecture items:

- [ ] Extract this projection into a typed `ArtifactSchemaProjection` IR
      separate from `dataquery.DataArtifact`, so prompt shape cannot drift with
      executor payload shape.
- [ ] Add artifact projection size metrics to data workflow audit records.
- [ ] Feed projection completeness and row-count metadata directly into action
      validators instead of relying on prompt samples.

### Batch 167: Ledger Graph Prompt Projection

The next real-scenario run confirmed that artifact prompt compaction helped:
continuation request size dropped sharply after full-record extraction. The
workflow then progressed through rule derivation, entity normalization, entity
application, field derivation, and qualification. A new generic bottleneck
appeared as the ledger graph grew: decision rows, entity resolutions,
contribution records, and reconcile groups are useful for audit and runner
state, but full or overly-rich ledger samples should not be the model's main
context mechanism.

This is not tied to any one business task. Any data workflow with item-level
decisions, entity normalization, filtering, joins, contributions, OCR/text
span extraction, or reconciliation can accumulate large ledgers. The model
needs count/status/key projections; deterministic runners and audit files need
the complete ledgers.

The invariant is now:

- complete ledgers stay in `dataquery.Result`, runner seed, and audit artifacts;
- prompts receive a bounded ledger projection with counts, status/decision
  distributions, and small group/metric/role samples;
- entity-resolution prompt samples are capped more aggressively because the
  projection carries the global status distribution;
- artifact access field samples are small examples, not a data transport
  mechanism;
- no domain names, file names, columns, currencies, dates, or business
  categories drive these hard limits.

Changes:

- [x] Added `ledger_projection` to the data result prompt view.
- [x] Summarized decision rows, rule coverage, entity resolutions,
      contributions, and reconcile groups with generic counts and bounded
      samples.
- [x] Reduced entity-resolution prompt samples while preserving complete
      entity ledgers for deterministic execution and audit.
- [x] Reduced `artifact_access.field_samples` to a small bounded preview.
- [x] Added tests for ledger projection and field-sample bounds.

Remaining architecture items:

- [ ] Move ledger projection into a first-class `LedgerGraphProjection` IR in
      `internal/dataworkflow`, alongside the future `ArtifactSchemaProjection`.
- [ ] Add projection-size metrics and per-round body-size attribution to data
      workflow audit records.
- [ ] Feed ledger status distributions into typed evaluator decisions so
      repeated unresolved/ambiguous normalization can trigger a precise next
      action instead of another broad planner turn.

### Batch 168: Source-Field Precedence For Entity Normalization

The next real-scenario run reached the entity-normalization stage and exposed a
generic action-parameter contract gap. A model plan supplied both
`source_field=category_raw` and a broader `source_fields` array containing
additional context fields. The runner treated all of them as source values to
normalize, producing unrelated unresolved entity records for context fields.

This is not specific to categories, vendors, or purchase data. Any
normalization task can have a single primary source value plus contextual
fields: names with ids, labels with descriptions, terms with row keys, accounts
with owners, devices with locations, extracted spans with source locators, and
so on. The primary field must be the executable source-value contract; context
fields must not silently widen the ledger.

The invariant is now:

- singular `source_field` / `name_field` / `value_field` is authoritative when
  present;
- plural `source_fields` / `name_fields` / `value_fields` is used only when no
  singular source field was declared;
- the existing multi-source-field mode remains available by omitting the
  singular field;
- this is a structural action-parameter rule, not a business-domain rule.

Changes:

- [x] Updated `normalize_entities` source-field normalization so explicit
      singular fields take precedence over plural field lists.
- [x] Preserved existing plural-field normalization behavior when no singular
      field is supplied.
- [x] Added regression coverage proving context-style `source_fields` no
      longer create extra entity-resolution rows when `source_field` is set.

Remaining architecture items:

- [ ] Represent primary source fields and context fields as separate typed
      action-param roles in the planner schema instead of relying on alias
      precedence.
- [ ] Add action-result diagnostics showing which source-field mode was used:
      `single_source_field`, `multi_source_fields`, or `inferred_fields`.
- [ ] Extend the same role-precedence audit to other typed actions where a
      singular executable field can be confused with plural context fields.

### Batch 169: Action Input Inheritance For Materialization Nodes

The next real-scenario run exposed a workflow IR boundary bug. A continuation
plan declared the current batch inputs at `TaskPlan.input_paths` and emitted an
`extract_records` action with an `output_artifact`, but the action-level
`input_paths` array was empty. The staging guard correctly requires typed
actions to consume explicit inputs, yet the planner had already supplied those
inputs one level higher in the same typed plan.

This is not specific to purchase orders, attachments, or any business domain.
Any data workflow can express the same shape when a batch starts from known
source materials and the first action materializes them into an artifact:
spreadsheets, JSONL logs, OCR text evidence, web tables, generated extracts, or
plain text records. The system needs one structural normalization step between
batch-level inputs and materialization-node inputs.

The invariant is now:

- `TaskPlan.input_paths` are current-batch inputs;
- materialization/observation actions with empty `input_paths` may inherit
  those batch inputs before guard validation;
- actions with role-sensitive semantics such as joins, normalization,
  enrichment, filtering, contribution computation, reconciliation, and final
  projection must still declare their own explicit input roles;
- the guard remains strict after normalization and does not infer inputs from
  prose.

Changes:

- [x] Added a shape normalizer that copies batch-level `input_paths` into
      empty-input materialization actions: `material_inventory`,
      `inspect_material`, and `extract_records`.
- [x] Reused the same normalizer in execution preparation so continuation,
      repair, fallback, and deferred plans share the same IR boundary rule.
- [x] Left role-sensitive actions unchanged; missing inputs still produce a
      typed staging error instead of being guessed from batch-level paths.
- [x] Added regression coverage for both inherited materialization inputs and
      non-inherited join inputs.

Remaining architecture items:

- [ ] Promote this rule into a first-class `ActionDAG` IR pass in
      `internal/dataworkflow`, instead of keeping it in REPL workflow
      normalization.
- [ ] Expose batch inputs and action inputs as separate objects in planner
      prompts so the model can see when inheritance will happen.
- [ ] Add audit metadata when an action input was inherited from the batch
      contract, including inherited path count and target action id.

### Batch 170: Rule Material Contract For RuleCoverage IR

The follow-up real run reached `derive_rules`, but the candidate plan carried
ordinary record tables together with rule material. The previous runner
fallback converted any structured row into `field=value` text and therefore
created rule coverage records from purchase/order rows. That polluted the
RuleGraph: material coverage had succeeded, but ordinary data facts were
misclassified as workflow rules.

This is a generic IR boundary bug. It can happen with any tabular or JSON data
task: event rows, inventory rows, web-table rows, OCR-extracted records,
transaction rows, or measurement rows. A data record is not a rule just because
it is readable. Rule coverage must come from rule/constraint material,
explicit rule parameters, or the typed validation rule contract.

The invariant is now:

- `derive_rules` may consume rule/constraint materials declared in the coverage
  contract;
- if rule material is available, mixed action inputs are narrowed to that
  material before staging validation;
- ordinary structured rows are not auto-concatenated into rule text;
- input-derived rules require an explicit rule text field such as `text` or
  `rule_text`;
- if no input-derived rules exist, the runner may fall back to
  `coverage_contract.validation_rules`, without treating ordinary data inputs
  as evidence for those rules.

Changes:

- [x] Removed the runner fallback that turned arbitrary record fields into
      `field=value` rule text.
- [x] Only records with explicit rule-text fields can produce input-derived
      rule coverage.
- [x] Source/evidence paths for `derive_rules` are recorded only when explicit
      rules reference inputs or when an input actually produced rules.
- [x] Narrowed `derive_rules` action inputs in REPL workflow normalization to
      declared rule/constraint materials when such materials exist.
- [x] Added regression coverage proving ordinary data CSV rows do not become
      rules and mixed derive-rule inputs are narrowed to declared rule
      materials.

Remaining architecture items:

- [ ] Promote `RuleGraph` and `MaterialGraph` separation into
      `internal/dataworkflow` so the planner sees rule materials, data
      materials, generated artifacts, and ledger requirements as different IR
      objects.
- [ ] Add a typed `rule_material_projection` prompt object that exposes
      rule/constraint sources without including unrelated data artifacts.
- [ ] Add action-level diagnostics when `derive_rules` skipped ordinary records
      because they lacked explicit rule-text fields.

### Batch 171: Candidate Material Inventory Bootstrap For Complex Workflows

The next real run no longer polluted rule coverage, but it revealed a
MaterialGraph problem one stage earlier. The initial plan inspected the
explicitly named core CSV files and then marked material coverage authoritative,
while other local candidate materials remained unclassified. For complex
ledger-driven tasks, that is too optimistic: the system should not decide that
unclassified candidate files are irrelevant, and it should not require the
model to rediscover them from prose later.

This is generic. Any data task may have auxiliary materials that are not named
as exact paths in the user request: rule documents, reference files, extracted
text evidence, OCR/image/PDF sidecars, manifests, lookup tables, samples,
diagnostic files, or generated exports. The model should decide their role for
the current goal, but the system must first provide the full objective material
inventory.

The invariant is now:

- for complex workflows that require multiple validation ledgers or complete
  record sets, the first executable batch bootstraps a `material_inventory`
  action when the plan omits some discovered candidate files;
- candidate materials are recorded as optional/reference candidates, not
  silently promoted to required materials;
- the model still owns business classification: required, reference-only,
  planner-distilled, irrelevant, or needing extraction;
- simple one-file or low-risk data tasks are not forced through inventory.

Changes:

- [x] Added a workflow preparation pass that replaces an incomplete initial
      complex plan with a bounded `material_inventory` action over all
      discovered candidate paths.
- [x] Preserved existing user-explicit material floors and validation ledger
      contracts while representing all candidates as optional reference
      materials awaiting model classification.
- [x] Kept simple plans unchanged so ordinary single-table data reads remain
      fast.
- [x] Added regression coverage for both inventory bootstrap on complex tasks
      and no bootstrap on simple tasks.

Remaining architecture items:

- [ ] Move candidate inventory bootstrap into a first-class `MaterialGraph`
      bootstrap pass in `internal/dataworkflow`.
- [ ] Add workflow-state counters for discovered, classified, required,
      reference-only, planner-distilled, and ignored candidate materials.
- [ ] Teach the evaluator to request material classification when complex
      workflow progress starts from an incomplete candidate inventory.

### Batch 172: Row-Level Entity Application And Join Parameter IR Alignment

The next real run reached typed normalization, enrichment, joining,
contribution, reconciliation, and final projection. It no longer stalled on
large scripts, but the computed answer was still wrong. The workflow had a
closed internal ledger and `reconcile=pass`, yet the contribution set was too
small. Independent artifact inspection showed the loss point:

- source record extraction preserved all records;
- derived contribution-ready records still preserved all records;
- `join_records` reduced the candidate set sharply because many rows had
  unresolved canonical fields;
- those unresolved fields came from an IR mismatch: `normalize_entities`
  emitted one mapping per unique source value, while
  `apply_entity_resolutions` applied mappings by row locator / `item_id`.
  Repeated source values therefore resolved only for the first row and later
  rows were marked unmatched.

This is not domain-specific. Any dataset with repeated names, categories,
statuses, identifiers, labels, units, account names, device names, or
classification values can hit the same failure. The generic invariant is:

- source/reference normalization that is later applied to records must preserve
  row-level application evidence, even when several rows share the same source
  value;
- value-level explicit mappings remain valid, but row-bearing structured
  normalization should not collapse repeated source rows before application;
- join/action parameter contracts must use the same aliases in planner,
  workflow guard, and runner layers.

Changes:

- [x] Changed structured `normalize_entities` derivation to keep row-level
      resolution records by including the generated `item_id` in its
      de-duplication key.
- [x] Preserved explicit mapping expansion semantics; the change applies to
      source records that carry row locators and evidence.
- [x] Verified `apply_entity_resolutions` can now apply repeated source values
      to every base row rather than only the first observed value.
- [x] Added runner support for `left_fields_json`, `right_fields_json`,
      `left_keys_json`, `right_keys_json`, and `join_fields_json` so JSON array
      aliases do not fail after the planner emits structured params.
- [x] Updated the REPL field-contract guard to parse the same join field aliases
      as the runner.
- [x] Added regression coverage for repeated source-value normalization and
      JSON join field aliases.

Remaining architecture items:

- [ ] Promote row-level versus value-level entity resolution into explicit
      `EntityGraph` IR metadata so the planner sees whether a mapping can be
      applied by item locator, source value, or both.
- [ ] Add candidate coverage gates before contribution calculation: when a
      required normalization stage leaves unmatched rows, the workflow should
      require include/exclude decisions or an explicit value-coverage action
      before an inner join can silently shrink the candidate universe.
- [ ] Add row-loss diagnostics from `join_records` into evaluator state:
      left-row count, matched-row count, unmatched key samples, and whether
      downstream contribution/reconcile requires a complete candidate decision
      set.

### Batch 173: Typed Field Violations And Text Field Materialization IR

The next real run improved row-level normalization but still did not converge.
The workflow reached rule coverage and entity application, then repeatedly
blocked on generated artifacts that lacked fields the next action wanted to
consume. The failures were structural:

- a candidate plan attempted to join three record sets in one `join_records`
  action even though the action is binary;
- later plans referenced fields such as downstream filter or join keys that
  were not present on the selected artifact;
- the missing-field fact was available only as a guard error string in recent
  history, so the next planner turn could overlook it inside a large prompt;
- text-derived artifacts contained record text and source locators, but there
  was no first-class typed action for materializing structured fields from a
  text/record field while preserving provenance and keeping only matched rows.

These are generic data-engine gaps. Any workflow that combines generated
records, OCR/text evidence, web-table snippets, JSONL messages, log snippets,
or spreadsheet cells can hit the same class of failure. The invariant is:

- action arity is part of the typed DAG contract; binary actions must not
  consume three inputs and hope the runner chooses a shape;
- field-contract failures must become compact typed workflow state, not only
  prose in prior-round errors;
- text-to-structured-field materialization is a reusable typed action whose
  business meaning is supplied by the model through declared specs while the
  system executes and validates only source fields, patterns, output fields,
  and provenance.

Changes:

- [x] Added domain-neutral `extract_fields` as a typed data action.
- [x] Reused the existing `derive_fields` field-spec parser so
      `field_specs` / `extract_specs` share one structured parameter contract.
- [x] Implemented `extract_fields` runner execution over one existing
      record/text artifact: it applies model-declared regex/parse specs,
      preserves source locator fields, filters matched rows by
      `required_fields`, and materializes a reusable JSON artifact.
- [x] Added `extract_fields` to planner schema, prompt guidance, workflow
      allowed-action contracts, action scaffolds, single-record-set guards,
      artifact preference lists, and capability ranks.
- [x] Tightened `join_records` to exactly two input paths in the shared
      `dataworkflow` action capability table; multi-table joins must now split
      into DAG ranks.
- [x] Promoted recent system-generated missing-field guard errors into
      `workflow_state_json.field_contract_violations`, including action id,
      kind, input alias, missing fields, available field samples, candidate
      artifacts, and repair action hints.
- [x] Added regression coverage for `extract_fields`, join arity contracts, and
      field-contract violation projection into workflow state.

Remaining architecture items:

- [ ] Promote field-contract violations from prompt projection into a durable
      `ActionGraph.Blocked` / `WorkflowDecision` IR object in
      `internal/dataworkflow`.
- [x] Add a typed value-distribution action so a planner can inspect candidate
      values before declaring filters or contribution params, without falling
      back to scripts.
- [ ] Add row-loss and zero-match diagnostics as typed evaluator inputs so
      contribution stages cannot silently continue from an underspecified
      candidate universe.

### Batch 174: Apply-Resolution Role Contract

The next real run reached typed entity application, but a generated artifact
was shaped like an entity-resolution ledger instead of base records. The action
had enough typed inputs, but the first input was a mapping ledger and the real
base record set was listed later. Because `apply_entity_resolutions` still
treated position one as base when `base_path` was absent or wrong, it produced
a misleading artifact that looked successful while preserving mapping rows
instead of source rows. Downstream joins then failed because ordinary base
fields were absent.

This is a generic role-contract gap, not a data-domain rule. Any task that
applies a source-to-canonical mapping can hit it: records may describe people,
devices, accounts, labels, units, locations, services, categories, or arbitrary
user-defined dimensions. The runtime must distinguish artifact roles from list
position:

- a base record artifact preserves the row universe for later filtering,
  joining, contribution, or final projection;
- an entity-resolution artifact provides mapping evidence with locator/source
  and canonical fields;
- when schema evidence clearly identifies one mapping ledger and one base
  record set, the runner may correct the role assignment and must record that
  inference for audit;
- when role evidence is ambiguous, the workflow must fail structurally and ask
  the planner to split the action or set `base_path` explicitly.

Changes:

- [x] Added runner-side entity-resolution role detection based on typed ledger
      fields such as locator/source/canonical columns.
- [x] If the selected base input is an entity-resolution ledger and exactly one
      distinct non-ledger input is available, infer that input as the base
      record artifact and treat the old base as the resolution input.
- [x] Recorded role inference in the output artifact fields so audit logs show
      why the runtime corrected the model plan.
- [x] Added REPL pre-execution guard support for the same role contract so
      field checks validate against the inferred base instead of the mapping
      ledger.
- [x] Added regression coverage proving reversed inputs still preserve base
      rows and that REPL staging does not falsely reject the corrected shape.

Remaining architecture items:

- [ ] Promote artifact roles into durable `ActionGraph` edge metadata instead
      of keeping role inference local to individual action runners.
- [ ] Feed role-correction notes into the workflow evaluator so repeated role
      drift can trigger a compact planner repair instead of repeated action
      retries.
- [ ] Add a role-aware artifact selector in `internal/dataworkflow` so all
      actions with base/reference/mapping roles share one IR contract.

### Batch 175: Zero-Match Filter Violations

The next real run moved past entity application and reached contribution-input
preparation. It produced a `filter_records` artifact with zero output rows from
a non-empty input. The model noticed the result was suspicious, but still
considered continuing with join and contribution calculation over the empty
candidate set. That would create a structurally "complete" but semantically
wrong path.

This is not specific to any business rule. Any data workflow can temporarily
filter to zero rows: log-event selection, spreadsheet cleanup, OCR row
extraction, JSONL filtering, table joins, anomaly selection, or metric
aggregation. Zero can be valid for some user goals, so the fix is not "zero is
always an error". The invariant is narrower:

- when the workflow still requires a contribution ledger or reconciliation,
  a zero-match filter over non-empty input is a typed violation until it is
  diagnosed or explicitly accepted by the output contract;
- downstream joins or contribution calculations must not consume that zero-row
  candidate artifact as if it were a verified eligible set;
- the violation must expose objective diagnostics: input rows, output rows,
  filter fields, filter diagnostics, aliases, and repair hints.

Changes:

- [x] Added `workflow_state_json.zero_match_filter_violations` derived from
      `filter_records` artifacts whose `input_rows>0` and `output_rows=0`
      while contribution/reconcile is still required.
- [x] Added repair hints that point the planner to inspect actual field values
      or rerun `filter_records` against the non-empty input artifact with
      corrected filters.
- [x] Added a staging guard that blocks `join_records`, `compute_contributions`,
      `reconcile_artifacts`, and `assemble_answer` when they consume a
      zero-match filter artifact under a still-required contribution/reconcile
      contract.
- [x] Kept zero-row filters valid for exploratory or all-zero-output tasks by
      tying the guard to the workflow coverage contract, not to the action kind
      alone.
- [x] Added regression coverage for zero-match state projection and blocked
      downstream consumption.

Remaining architecture items:

- [x] Add a domain-neutral value-distribution action so filter repair can
      inspect per-field value counts without relying on `inspect_material`.
- [ ] Feed zero-match violations into a durable `WorkflowDecision` IR object
      rather than prompt projection only.
- [ ] Let output contracts declare an explicit all-zero result expectation so
      zero-match filters can be accepted without extra planner turns when that
      is genuinely the user's goal.

### Batch 176: Locator-Compatible Resolution Keys And Valid Ledger Snapshot

The next real run moved through material extraction, normalization, and entity
application, but the generated `apply_entity_resolutions` artifact reported
`matched_<target>=0` and `unmatched_<target>=base_rows` for every canonical
target. The planner then derived and qualified fields on that all-unmatched
artifact, producing a zero-eligible candidate set and repeated repairs. The
direct cause was structural: the base side used `_source_locator` while the
resolution side used `item_id`; both encoded the same row identity, but the old
matcher compared one as a full locator string and the other as a parsed row
index.

This is not specific to procurement data. Any data workflow can carry row
identity through different equivalent locator forms: `_source_index`,
`source_index`, `_source_locator`, `source_locator`, `item_id`, `row_id`, or
record ids created by previous typed actions. The workflow must compare those
through a typed locator contract, not literal strings.

The same run also showed that repeated repair batches inflated decision and
resolution counts. Historical audit must retain every run, but workflow state
and runner seeds should use a valid deduplicated ledger snapshot so the planner
does not mistake repeated bad attempts for progress.

Changes:

- [x] Normalized explicit base locator keys in `apply_entity_resolutions`.
      When a single base key field is a locator-style field, the runner now
      extracts the row index using the same locator parser already used for
      resolution-side `item_id` / `source_locator` fields.
- [x] Added regression coverage for base `_source_locator` matched against
      resolution `item_id`.
- [x] Added `workflow_state_json.unmatched_resolution_violations` from
      `apply_entity_resolutions` artifacts where every target field has zero
      matches over a non-empty base record set.
- [x] Added a staging guard that blocks downstream record derivation,
      filtering, qualification, enrichment, joining, contribution, reconcile,
      and answer assembly from all-unmatched resolution artifacts while
      contribution/reconcile remains required.
- [x] Added `workflow_state_json.zero_eligible_qualification_violations` from
      `qualify_records` artifacts with non-empty input and zero eligible rows
      while contribution/reconcile remains required.
- [x] Added a staging guard that blocks join/contribution/reconcile/final
      projection from zero-eligible qualification artifacts under a
      still-required contribution/reconcile contract.
- [x] Added `DedupeRowDecisionRecords` and wired it into action-runner seeds,
      current-batch accumulation, workflow handoff, and workflow-state counts.
      Rule, contribution, and entity-resolution ledgers already had analogous
      dedupe.
- [x] Added regression coverage for unmatched-resolution state projection,
      blocked all-unmatched downstream consumption, zero-eligible state
      projection, and blocked zero-eligible downstream consumption.

Remaining architecture items:

- [ ] Promote locator compatibility into a shared `ActionGraph` edge contract
      so apply/enrich/join/compute actions all use the same row-identity IR.
- [ ] Split workflow state into explicit `historical_audit` and
      `effective_snapshot` sections instead of relying on dedupe at each
      projection site.
- [x] Add a typed value-distribution action so zero-eligible qualification
      repair can inspect actual values without broad scripts.
- [ ] Add further field-preview actions only for observation shapes that are
      not covered by value distribution or schema projection.
- [ ] Feed unmatched-resolution and zero-eligible violations into
      `WorkflowDecision` as typed reason codes, not only prompt-visible state.
- [ ] Improve CLI/REPL data workflow process events so business-facing details
      from structured model output (`goal`, `why_this_batch`, `next_batch`,
      action `purpose`, repair reason, and planned action list) are shown before
      internal ledger counters. Keep counters available for audit, but do not
      make repeated system summaries such as "recorded structured signals" the
      primary user-visible text.

### Batch 177: Derive-Field Locator No-Ops And Deferred Failure Audit

The next real run moved past entity resolution and reached the
`prepare_contribution_inputs` stage. The planner emitted a multi-rank typed DAG
whose first executable rank was `derive_fields`. That rank failed because it
included two field specs that copied `_source_index` to `_source_index` and
`_source_locator` to `_source_locator`.

This is a cross-layer contract mismatch, not a business-domain issue. The
planner prompt correctly treats source locator fields as real generic fields
that can support row identity. The runner also preserves these locator fields
automatically on every generated record. Therefore copying a reserved locator
field to the same reserved field is a structural no-op and should not force a
repair. Overwriting or inventing locator fields remains unsafe and must still
be rejected.

The same run showed a data-DAG audit gap: when the first rank failed, the
deferred typed action suffix was discarded with a generic "execution failure"
message. That protected correctness, but it did not expose enough objective
detail for humans or a future workflow IR to decide whether the suffix should
be replayed, rebuilt, or abandoned.

Changes:

- [x] Treat `derive_fields` specs that copy a reserved source locator field to
      the exact same reserved target as no-op specs. The original locator fields
      remain preserved by the runner.
- [x] Keep rejecting true locator overwrites and constant-created locator/index
      fields. The no-op relaxation applies only to same-source/same-target
      `copy` over an existing reserved source field.
- [x] Record skipped no-op locator copies in the generated artifact field
      `noop_reserved_copy_fields` for auditability.
- [x] Teach the data planner prompt that locator fields are already preserved
      and should only be copied into non-reserved aliases when downstream
      actions need such aliases.
- [x] Improve CLI and REPL deferred-queue discard events with the first
      deferred action id/kind and a bounded failure summary. Logs now include
      the same first-action information.
- [x] Add regression coverage for no-op reserved locator copies in
      `derive_fields`.

Remaining architecture items:

- [ ] Split deferred-queue handling into typed states: `ready`, `blocked`,
      `invalidated_by_failed_prefix`, and `replay_after_prefix_repair`. A
      failed prefix should not always discard the suffix; the IR should decide
      whether the suffix still depends on the same output aliases and action
      contracts.
- [ ] Persist deferred-queue invalidation records with typed fields:
      failed_action, failed_kind, failed_error_code, first_deferred_action,
      invalidated_aliases, and replayability.
- [ ] Move locator-field preservation and aliasing into the shared
      `ActionGraph` edge contract so derive/apply/enrich/join/compute actions
      all reason about row identity through one IR.

### Batch 178: Multi-Input Text Field Extraction

The next real run restarted from the updated binary. It reached the continuation
planner after material inventory, and the model emitted a valid data intent:
extract the same structured fields from several text evidence files before
later normalization and contribution calculation. The old workflow contract
rejected the plan because `extract_fields` was treated as a single-record-set
action.

That contract was too narrow. Many domain-neutral data tasks need to apply one
declared extraction spec to a set of same-schema materials: OCR text files,
email snippets, JSONL message payloads, log excerpts, forms, receipts, notes,
or other semi-structured documents. Rejecting this shape forces the planner to
choose broad scripts even when the work is a bounded typed extraction.

The generic invariant is:

- `derive_fields`, `filter_records`, `qualify_records`, and
  `compute_contributions` remain single-record-set actions;
- `extract_fields` may consume one or more same-schema text/record artifacts
  with one extraction spec set and emits one merged structured artifact;
- multiple plain-text inputs default to document/file scope, one output row per
  input material, while explicit `record_scope=line` keeps line-level
  extraction available.

Changes:

- [x] Updated `extract_fields` action capability from `max=1` to
      one-or-more inputs.
- [x] Updated the action runner so `extract_fields` applies the same spec set
      across all input paths, merges matched rows into one artifact, and keeps
      one child source artifact per input.
- [x] Added document-scope extraction for multiple plain-text inputs. The
      runner uses one row per input file by default and keeps line-level mode
      available through `record_scope=line`.
- [x] Added typed text-source alias normalization for `content` / `body` /
      `raw_text` to the actual `text` field when a text artifact exposes that
      field.
- [x] Updated planner prompt and workflow action contracts to describe
      multi-input same-schema `extract_fields`.
- [x] Added regression coverage for multi-input text extraction and updated
      action-capability tests.

Remaining architecture items:

- [ ] Add an explicit `input_schema_group` / `same_schema_group` field to the
      ActionGraph IR so the planner and runner can validate multi-input
      extraction compatibility structurally instead of relying on action kind
      alone.
- [ ] Promote text record scope (`document`, `line`, `paragraph`) into the
      schema for `extract_fields` so the model sees an enum rather than free
      params.
- [ ] Add evaluator diagnostics when an extraction spec matches zero rows
      across multiple inputs, including per-input match counts and sample text
      windows.

### Batch 179: Resolution Status Contract And Canonical-ID Signals

The latest real run completed end-to-end but produced a wrong answer while the
internal reconcile report said `pass`. The workflow had become structurally
self-consistent around an incomplete contribution table. Two generic gaps were
visible:

- source records already carried a canonical-looking id field that also existed
  in the reference table, but entity normalization only used the model-declared
  display/name field. Many records therefore became `unmatched` even though a
  deterministic id-to-reference match was available.
- `qualify_records` and `compute_contributions` relied too much on the model's
  hand-authored bad-status filters. A plan excluded `matched_ambiguous` and
  `unresolved`, but missed the runner's `unmatched` status, so unresolved
  entity rows could still be marked eligible and counted.

This is not specific to purchase data. Any data task that normalizes local
values against a reference set can hit the same failure: account ids, device
ids, product codes, people ids, labels, tags, categories, location codes, or
other dimensions. The invariant is now:

- reference-mode normalization treats a source-side field matching the
  reference `canonical_id_field` as a strong exact structural signal;
- reference lookup also indexes the canonical id itself, not only display or
  alias fields;
- open resolution statuses are not applied as canonical choices by default;
- generated resolution status fields are auto-detected and open statuses are
  blocked before qualification/contribution unless the workflow explicitly
  allows unqualified records.

Changes:

- [x] Added canonical-id source signal expansion in
      `normalize_entities`: when the source record set contains the same field
      as `canonical_id_field`, it is added as an exact source signal.
- [x] Added canonical-id reference lookup indexing so exact source ids can
      resolve through the same generic reference-mode path.
- [x] Updated `apply_entity_resolutions` choice filtering so open statuses
      such as ambiguous/unmatched/unresolved are not applied as canonical
      resolved choices by default.
- [x] Added `unmatched`, `not_applicable`, `not_matched`, and `missing` to the
      default generated-status blocking set and to open resolution status
      classification where applicable.
- [x] Updated generated status auto-detection to include
      `*_resolution_status` fields as typed status fields.
- [x] Updated contribution preflight so a target contribution row with an open
      generated status remains blocked even if the model's filters mentioned
      that status field but failed to exclude every open value.
- [x] Updated planner guidance to prefer positive accepted-status contracts
      over enumerating bad statuses, and to mention source-side canonical-id
      signals for reference normalization.
- [x] Added regression coverage for canonical-id source matching, open-choice
      filtering, auto resolution-status qualification, and incomplete
      status-filter contribution blocking.

Remaining architecture items:

- [ ] Promote generated status fields into a first-class
      `ActionGraph.status_contract` so later actions receive exact required
      statuses from IR instead of inferring them from field names.
- [ ] Feed unresolved/unmatched/ambiguous resolution counts into evaluator
      state as hard blockers for target contribution/reconcile stages when the
      coverage contract requires entity resolution.
- [ ] Add a domain-neutral value-coverage action for reference mappings so the
      workflow can report which source ids/values were covered, missing, or
      ambiguous before contribution calculation.
- [ ] Improve data workflow UX events by surfacing business-facing model
      summaries from typed plan fields (`goal`, `purpose`, `why_this_batch`,
      `next_batch`, action purposes, repair reason) before internal counters;
      keep counters as low-noise audit detail and avoid business-specific
      wording.

### Batch 180: Grouped Record Projection For Multi-Row Materials

The next real run no longer failed on entity-resolution status. It progressed
through material inventory, entity resolution, resolution application, rule
derivation, and text materialization. The new blocker was structural: a text
material had been converted into line-level records, but the fields needed by
later joins and contribution validation were split across neighboring lines
inside the same logical document/group. The planner kept trying `extract_fields`
against individual rows and repeatedly produced zero matched records.

This is a generic data-engine gap, not a document-type or procurement-specific
case. OCR output, PDF text extraction, chat transcripts, log blocks, web page
sections, table footnotes, email bodies, and line-oriented generated artifacts
often represent one logical item as multiple rows/spans. A typed DAG needs a
neutral projection step before field extraction:

- source rows/spans are grouped by model-declared structural keys;
- text/value fields are concatenated in source order into one grouped record;
- source locators and row counts remain auditable;
- business meaning is still supplied by later typed actions, not by the
  grouping action.

Changes:

- [x] Added typed `group_records` action to the data action IR.
- [x] Implemented deterministic runner support for grouping one record artifact
      by `group_field` / `group_fields`, concatenating `text_fields` /
      `source_fields` into `target_field`, and preserving source locators.
- [x] Added `group_records` to action capability ranks and single-record-set
      input contracts.
- [x] Added planner schema, planner guidance, workflow contracts, staging
      guards, field-contract checks, repair hints, and action scaffolds.
- [x] Added regression coverage for `group_records -> extract_fields`, proving
      cross-line fields can be projected without a custom script.
- [x] Added REPL planner guard coverage so empty `group_records` specs fail
      before execution.

Remaining architecture items:

- [ ] Add typed grouped-record diagnostics to evaluator state when repeated
      `extract_fields` attempts produce zero rows on line/span artifacts with
      an obvious grouping key.
- [ ] Promote grouped projection metadata into a formal `ActionGraph.projection`
      IR so future typed actions can declare row-level, grouped, document-level,
      or block-level record shapes without relying on free params.
- [ ] Add value-coverage and candidate-mapping actions so grouped extracted
      records can be compared with base/query/reference artifacts before
      contribution calculation.
- [ ] Upgrade data workflow process events to show business-facing plan/next
      step/action purpose from typed model fields before internal counters.

### Batch 181: Canonical-Enriched Records Are Still Base Records

The next real run reached a multi-dimension normalization stage. It first
applied one entity-resolution artifact to a base record set, producing a new
record artifact with fields such as `*_canonical`, `*_resolution_status`, and
source locators. The deferred next action then tried to apply another
resolution artifact to that enriched record set. The old role detector treated
the enriched record artifact as an entity-resolution ledger because it had both
source locator fields and canonical-looking fields, so the valid deferred edge
was discarded and the planner spent another turn rediscovering the same step.

This is a generic IR bug. A record set that has already been enriched with one
canonical dimension is still a base record set for later dimensions. The
workflow may need to normalize vendor/category, account/region, device/service,
person/team, product/taxonomy, or any other combination in sequence. The system
must distinguish:

- resolution ledgers: explicit mapping rows with fields such as
  `source_value`/`source_field`/`evidence_refs` plus
  `canonical_id`/`canonical_label`;
- enriched base records: ordinary rows that preserve source locators and add
  target-specific fields such as `vendor_id_canonical` or
  `category_resolution_status`.

Changes:

- [x] Tightened REPL/workflow artifact classification so `_source_locator`
      plus `*_canonical` no longer makes a record artifact look like an
      entity-resolution ledger.
- [x] Tightened the deterministic action runner's
      `apply_entity_resolutions` role inference with the same distinction.
- [x] Added runner regression coverage for applying a second resolution to a
      canonical-enriched base record.
- [x] Added workflow staging regression coverage so deferred/preflight guards
      do not discard a valid `apply_entity_resolutions` action over an
      enriched base record.

Remaining architecture items:

- [ ] Promote artifact roles into a first-class `ActionGraph` IR field
      (`record_set`, `resolution_ledger`, `mapping_reference`,
      `diagnostic_summary`, `final_projection`) so future guards do not infer
      roles from field names.
- [ ] Store role confidence and role evidence in audit records so humans can
      see why a deferred action was considered ready or blocked.
- [ ] Surface business-facing workflow progress in CLI/REPL process events by
      rendering model-provided `goal`, action `purpose`, `why_this_batch`,
      `next_batch`, repair reason, and continuation reason ahead of internal
      counters when those fields are present. Keep counters as low-noise audit
      detail and do not add business-specific template text.

### Batch 182: Source-Value Resolution Application From Mapping Ledgers

The next real run progressed through typed normalization but then produced
record artifacts whose canonical target fields were all empty. The model had
emitted valid `normalize_entities` outputs, and its later
`apply_entity_resolutions` plans named the resolution artifact plus
`resolution_key_fields=["source_value"]`. However, the plan omitted
`base_key_fields`. The old runner therefore keyed base rows by row index while
keying resolution rows by `source_value`, so every base row became
`unmatched`. A later left join preserved 50 rows and the evaluator mistook the
row-preserving artifact for a successful query join.

This is a generic IR contract gap, not a business-domain problem. A mapping
ledger often contains rows like `item_id=<source-artifact>#N:<source-field>`,
`source_value=<raw value>`, and `canonical_id=<target value>`. When the model
uses `source_value` as the resolution key but does not repeat the base field,
the system still has precise structural evidence: the field suffix inside
`item_id` names the base field that supplied the source value. The runner can
use that evidence to apply mappings deterministically, while avoiding broad
guessing across unrelated fields.

Changes:

- [x] Added a source-value apply index for `apply_entity_resolutions` when
      `resolution_key_fields` is exactly `source_value` and no
      `base_key_fields` were declared.
- [x] The source-value index is enabled only when resolution records expose a
      base-field signal via `source_field`/`input_field`/`field` or an
      `item_id`/locator suffix such as `#N:<field>`, and that field exists on
      the base record artifact.
- [x] Preserved the existing locator and explicit composite-key paths. The new
      index is a fallback after explicit/default keys fail, not a replacement.
- [x] Added audit fields showing which base fields were used for source-value
      application.
- [x] Added regression coverage for a domain-neutral mapping ledger whose
      resolution key is `source_value` and whose base field is inferred from
      the ledger locator suffix.

Remaining architecture items:

- [ ] Promote apply-resolution key strategy into a first-class
      `ActionGraph.edge_key` IR with strategy names such as `locator`,
      `source_value_field`, and `explicit_composite`, so evaluator and UI can
      reason about successful and failed materialization without inspecting
      free-form params.
- [ ] Treat row-preserving joins with `matches=0` as a typed
      `zero_match_join` contract violation when downstream contribution or
      reconcile is required, rather than letting the evaluator infer success
      from output row count alone.
- [ ] Surface join match rate, unmatched counts, and inferred key strategy in
      business-facing progress details when available, while keeping raw
      counters as low-noise audit data.

### Batch 183: Apply-Resolution Field Scope Contract

The next latest-binary real run no longer produced an all-unmatched
`apply_entity_resolutions` artifact, but it still failed before contribution
calculation. The generated `workflow_entity_resolutions` ledger contained
several mappings per source row: one for a vendor-like field, one for a
category-like field, and sometimes other source values. The model emitted
structured `resolution_specs` with `source_fields`, but the executor treated
that field as descriptive metadata. Because both specs used the row locator
(`item_id` -> `#N`) as the key, each target field saw resolution choices that
belonged to other fields on the same row. Most base rows were then marked
ambiguous, a later join saw zero matches, and contribution calculation had no
rows to aggregate.

This is a generic data-IR issue. Any workflow that applies multiple
source-to-canonical mappings from one shared ledger can hit it: product tags,
accounts, people, devices, regions, categories, vendors, event labels, or
other dimensions. A graph edge from a resolution ledger to a base artifact must
carry both row identity and field scope. The system should consume typed
`source_fields` when present instead of relying on broad row-level matching.

Changes:

- [x] Added `SourceFields` to the deterministic
      `apply_entity_resolutions` spec IR.
- [x] Parsed structured `source_field` / `source_fields` aliases from
      `resolution_specs`.
- [x] Scoped resolution rows by locator/source-field evidence before building
      row-locator and source-value indexes.
- [x] Recorded `source_fields` and `source_scope_rows` on child audit
      artifacts so failures can be inspected without reading model prose.
- [x] Taught the planner that `source_fields` are executable field-scope
      constraints for shared multi-field resolution ledgers.
- [x] Added workflow preflight validation for `source_fields` against the base
      artifact field contract.
- [x] Added regression coverage proving two target fields can be materialized
      from one shared row-locator resolution ledger without cross-field
      ambiguity.

Remaining architecture items:

- [ ] Promote field scope into first-class `ActionGraph.edge_scope` metadata
      instead of keeping it inside action params.
- [ ] Add a typed `zero_match_join` violation when a downstream
      contribution/reconcile stage consumes a join artifact with `matches=0`.
- [ ] Add a domain-neutral semi-structured extraction diagnostic action that
      reports candidate value windows, field/value distributions, and failed
      pattern diagnostics before retrying `extract_fields`.

### Batch 184: Deferred DAG Readiness And User-Facing Process Events

The latest real run showed that field-scoped resolution application now
materializes canonical fields without cross-field ambiguity, but the adaptive
DAG still wastes turns around deferred work. A deferred queue can contain a
blocked node followed by an independent node that is already executable, or it
can fail because a consumer references an artifact/field that has not yet been
materialized. The old runtime treated the queue as a strict list: if the first
node was blocked, the whole queue was discarded and only a low-level log line
survived. The next model turn then had to rediscover the graph state from a
large prompt.

This is a generic IR problem. Any multi-step data task can produce deferred
nodes: extraction, grouping, enrichment, filtering, contribution calculation,
reconciliation, or final projection. Deferred nodes are graph work, not prose,
and blocked reasons are typed workflow facts that should feed the next
evaluator/planner turn.

Changes:

- [x] Changed deferred dispatch to select the first ready dependency rank from
      the queued action graph instead of requiring the queue head to be ready.
- [x] Preserved blocked deferred actions in the remaining queue when an
      independent later ready rank is dispatched.
- [x] Reused the existing artifact/field contract checks before dispatching a
      selected deferred rank, so ready selection does not bypass staging
      safety.
- [x] Recorded deferred blocked reasons as workflow records before discarding
      an unusable queue in both CLI and REPL flows. This lets
      `field_contract_violations` and later continuation prompts consume the
      same structured failure instead of relying on a human log line.
- [x] Added regression coverage proving a blocked deferred prefix no longer
      prevents an independent ready typed action from running.

Remaining architecture items:

- [ ] Move deferred-ready selection into the `dataworkflow` IR package so CLI
      and REPL own rendering only, not graph scheduling rules.
- [ ] Add explicit producer/consumer edges (`ActionGraph.edge_key`,
      `edge_scope`, and artifact aliases) so a consumer can be gated by its
      declared producer rather than by path strings alone.
- [ ] Persist deferred queues and blocked-node diagnostics in workflow audit
      records so interrupted sessions can resume the graph deterministically.
- [ ] Replace internal-only process lines with user-facing process events that
      render model-provided structured content first: workflow `goal`, action
      `purpose`, `why_this_batch`, `next_batch`, repair reason, continuation
      reason, and selected action list. Keep deterministic counters such as
      consumed materials, decisions, contributions, and reconcile status as
      low-noise audit detail. Do not add business-specific text templates.

### Batch 185: Numeric Field-Use Contracts And Repair State

The next real run moved beyond deferred queue readiness but exposed a lower
level typed-data contract gap. A derived artifact carried a field later used by
numeric filtering and contribution calculation, but the field's sampled values
were flag-like strings such as `true` rather than numbers. The workflow then
created a zero-row filter artifact, spent multiple repair rounds inspecting the
same generated artifacts, and eventually surfaced a diagnostic summary instead
of the user's strict final answer.

This is not tied to amount, dates, procurement, invoices, or any business
domain. Any data task can misuse a field according to its declared purpose:
numeric comparisons over status text, aggregation over labels, contribution
values sourced from booleans, or final projection from diagnostic artifacts.
The generic invariant is:

- field names alone are not enough; typed actions also declare field purpose;
- `gt/gte/lt/lte` and non-count contribution values require numeric samples;
- flags/status/text fields can still be used for equality/inclusion/decision
  filters, but must not become numeric contribution values;
- repair prompts should receive typed field-use violations with candidate
  numeric-looking fields, not only raw error prose;
- diagnostic/inspect artifacts are workflow evidence, not final answers for
  strict output contracts while required contribution/reconcile ledgers remain
  absent.

Changes:

- [x] Added runner-level numeric field-use validation for `filter_records`.
      Numeric comparison operators now reject fields whose sampled non-empty
      values are all non-numeric, while still allowing mixed fields that contain
      valid numeric values.
- [x] Added runner-level numeric value validation for
      `compute_contributions`. Non-count contribution actions now reject
      `value_field` samples that are all non-numeric after current filters.
- [x] Classified these failures as `numeric_field_contract` in the unified data
      task violation layer with a repair hint to materialize a numeric field
      through typed `derive_fields(parse_number)`.
- [x] Promoted numeric field-use failures into
      `workflow_state_json.field_contract_violations`, including the input
      alias, misused field, available fields, numeric-looking candidate fields,
      and typed repair hints.
- [x] Added a staging guard for same-batch plans where a non-numeric
      `derive_fields`/`extract_fields` constant is later consumed as a numeric
      filter or contribution value through the derived artifact alias.
- [x] Updated the planner prompt so models know that numeric comparison and
      contribution value fields must be numeric, while status/flag fields remain
      valid for non-numeric filtering and decisions.
- [x] Added regression coverage for non-numeric numeric filters, non-numeric
      contribution value fields, same-batch non-numeric constant reuse, and
      numeric contract promotion into workflow state.

Remaining architecture items:

- [ ] Add a typed `field_use_contract` IR separate from generic field
      availability. It should model required type/purpose such as numeric,
      boolean-like, categorical, locator, free text, group key, and contribution
      value without hard-coding business names.
- [ ] Feed field-use contracts into action scaffolds so the planner sees which
      existing artifacts already contain numeric-looking candidates before it
      emits a repair batch.
- [ ] Add deterministic repair-plan synthesis for unambiguous numeric
      materialization cases: when one input artifact has exactly one
      numeric-looking candidate field and a downstream numeric field is
      invalid, propose a typed `derive_fields(parse_number)` node instead of
      spending a model repair turn.
- [ ] Harden terminal completion so diagnostic/inspect-only results cannot be
      returned as final answers for strict output contracts while required
      contribution, reconcile, or projection stages are still missing.
- [ ] Implement the Batch 184 process-event UX item as a shared event renderer:
      permanent lines should first show model-authored business-facing
      `goal`/`purpose`/`why`/`next` details when available, with internal
      counters as secondary audit detail for CLI and REPL.

### Batch 186: Extraction Diagnostics And Resolution-Key Fallbacks

The next real run showed progress through typed DAG planning, but exposed two
generic convergence gaps below the planner layer:

- Some action params still arrive as JSON strings that contain regular
  expressions or other escaped text. If a model emits `\s` or similar inside a
  JSON string, strict JSON parsing fails before the runner can execute an
  otherwise valid typed action.
- `extract_fields` could produce zero matched records from a non-empty input
  without preserving enough typed diagnostics. The next planner turn then had
  to infer whether the issue was the source field, the pattern, document
  grouping, required fields, or an upstream artifact choice.
- `apply_entity_resolutions` treated explicit key fields as absolute even
  when they produced no choices, and top-level `source_fields` were not
  normalized into the executable resolution spec. This allowed a noisy key
  declaration to block safer row-locator or source-value application.

These are not domain-specific failures. Any semi-structured extraction or
lookup/enrichment workflow can hit the same class of issue: logs, web tables,
PDF/OCR text, JSONL records, spreadsheet cells, support tickets, telemetry
events, or customer-provided reference tables.

Changes:

- [x] Added a shared `parseActionMapListJSON` path for structured action param
      arrays so invalid JSON string escapes and repairable list-shape drift are
      handled before typed spec parsing.
- [x] Routed `derive_fields`, `extract_fields`, and
      `apply_entity_resolutions` structured specs through that shared parser.
- [x] Added `extract_fields` source-field samples, source-field inventory,
      required-field missing counts, and compact zero-match diagnostics to the
      generated artifact.
- [x] Added source child samples for extraction inputs so prompt artifact
      access can show actual source values even when extracted rows are empty.
- [x] Promoted zero-match `extract_fields` artifacts into
      `workflow_state_json.field_contract_violations`, with typed repair hints
      to inspect source samples and repair source fields, grouping, pattern, or
      required-field contracts before contribution/reconcile/projection.
- [x] Normalized top-level `source_fields` into the executable
      `apply_entity_resolutions` spec.
- [x] Let `apply_entity_resolutions` fall back to locator/source-value matches
      when explicit key fields produce no accepted choices, while preserving
      existing exact-key behavior when it succeeds.
- [x] Added regression coverage for invalid regex escapes in JSON-string
      specs, zero-match extraction diagnostics, workflow-state extraction
      violations, and explicit-key-miss resolution fallback.

Remaining architecture items:

- [ ] Add a first-class `field_contract_violation` IR emitted by action
      validators, instead of reconstructing violations from artifact fields or
      error text in REPL/CLI code.
- [ ] Move extraction diagnostics into a reusable value-window/projection
      primitive so `extract_fields`, field filters, contribution values, and
      joins can share one diagnostic shape.
- [ ] Feed zero-match extraction violations into deterministic continuation
      synthesis when the next legal action is an unambiguous diagnostic or
      field-materialization step.
- [ ] Continue the Batch 184 UX work as a shared process-event renderer:
      show model-authored, business-facing `goal`, `purpose`,
      `why_this_batch`, `next_batch`, repair reason, and action list when
      structured fields are available; keep internal counters only as
      secondary audit details. This must stay domain-neutral and must not add
      business-specific templates.

### Batch 187: Version-Aware Zero-Match Filter State

The next real run reached repair-node convergence: the workflow detected that
a previous derived status flag made a filter produce zero eligible rows, then
materialized a repaired artifact where the same output alias had non-zero rows.
However, the cumulative workflow state still reported the older zero-match
filter violation for that alias. A deferred consumer was then blocked as if it
were still reading the stale zero-row artifact, even though a newer compatible
artifact had already superseded it.

This is a generic artifact lineage problem. A DAG runtime must distinguish
historical failed artifacts from the current version of the same logical alias.
The issue can appear in any data workflow: filters, joins, extraction windows,
record expansion, enrichment, contribution candidates, or final projection
artifacts.

Changes:

- [x] Made zero-match filter violation projection version-aware. While scanning
      workflow records newest-first, any newer non-empty `filter_records`
      artifact clears older zero-match issues that share the same alias/id.
- [x] Reused the version-aware projection in the staging guard, so deferred
      joins/contributions are not blocked by stale zero-row artifacts after a
      newer non-empty candidate set exists.
- [x] Added regression coverage proving a newer non-empty alias suppresses the
      older zero-match violation and unblocks the downstream guard.

Remaining architecture items:

- [ ] Move this alias/version logic into a first-class ArtifactGraph IR with
      `logical_alias`, `producer_action`, `version`, `row_count`, `status`, and
      `supersedes` edges, rather than embedding it in REPL workflow projection.
- [ ] Apply the same version-aware suppression to all transient diagnostics:
      zero-match extraction, all-unmatched resolution, zero-eligible
      qualification, missing-field diagnostics, and failed join attempts.
- [ ] Let deferred consumers bind to the latest compatible artifact version by
      field contract, not only by alias string.
- [ ] Surface artifact-version changes in low-noise process events so users see
      that a repair replaced a stale candidate set with a newer usable one.

### Batch 188: Typed Filter Value Contracts

The next real run moved past stale zero-match suppression but exposed a lower
level action-parameter contract bug. The planner correctly chose a typed
`filter_records` repair and supplied a multi-value condition for an existing
status/category-like field. The source sample contained matching values, but
the executor treated the JSON list value as one scalar string, so `op=in`
compared records against the literal text `["a","b"]` instead of the list
items. The workflow then produced another zero-row artifact and spent repair
budget reasoning around a false negative.

This is not specific to status fields or purchase data. Any typed data workflow
can need multi-value filters: categories, tags, accounts, log levels, devices,
labels, owners, regions, enum states, or other discrete dimensions. The generic
invariant is:

- structured filter values must preserve list semantics through planner
  conversion, JSON repair, runner parsing, execution, diagnostics, and
  contribution calculation;
- `filters`, `filters_json`, and scalar convenience forms such as
  `filter_field/filter_value` must share one typed filter contract;
- list parsing may repair structural shape, but must not change business
  meaning, field names, or candidate values;
- diagnostics should report typed match counts and sample values so a true
  zero-match filter is distinguishable from a param-shape bug.

Changes:

- [x] Normalized JSON-list strings in shared filter parsing, including the
      single-condition `filter_field/filter_value` form.
- [x] Added execution-time defensive parsing for `in` and `not_in` so old or
      alternate entrypoints that still pass a JSON-list string keep list
      semantics.
- [x] Kept scalar equality and numeric comparison behavior unchanged; the
      change is scoped to typed filter value interpretation.
- [x] Added regression coverage for `filter_records` with a JSON-list
      `filter_value`, alongside the existing contribution-array filter
      coverage.

Remaining architecture items:

- [ ] Promote action params from `map[string]string` to a typed `ActionParam`
      IR so arrays, objects, numbers, booleans, and strings do not collapse
      into ad hoc JSON strings before execution.
- [ ] Make filter diagnostics carry the normalized typed value shape as
      structured JSON, not only as a display string.
- [ ] Feed typed filter-value shape mismatches into the shared
      `field_contract_violation` / action-param violation IR instead of
      relying on later zero-match artifacts.
- [ ] Continue the shared process-event UX work: render model-authored
      business-facing goal, batch purpose, repair reason, and next-step fields
      before internal counters in both CLI stderr and REPL permanent lines.

### Batch 189: Executable Scaffold Fallback Boundaries

The next implementation audit found that the custom-transform-disabled fallback
had the right intent but was still too weak as an IR mechanism. When a model
tried to repair a typed data workflow by returning another broad script, the
runtime could block the script, but the fallback could still degrade into
schema-only inspection even when the workflow state already exposed concrete
record artifacts and legal typed action scaffolds.

This is not a domain-specific issue. Any data DAG can reach the same boundary:
spreadsheet cleanup, JSONL aggregation, text extraction, lookup enrichment,
OCR-derived records, or multi-file joins. Once free-form scripts are disabled
for a workflow stage, the runtime should continue from objective DAG state,
not ask the model to rediscover the same graph through another prompt turn.

Changes:

- [x] Added a concrete scaffold fallback path for script-disabled repair. When
      the workflow has legal typed action scaffolds with fully concrete inputs
      and fields, the runtime can emit a typed action batch instead of falling
      back to generated-artifact schema inspection.
- [x] Normalized scaffold template enum placeholders into executable typed
      values before dispatch. For example, a model-facing `match_mode` choice
      list is not passed through as an execution value; the fallback uses the
      conservative structural default `exact`.
- [x] Added a generic identifier sanitizer for deterministic scaffold action
      IDs and output artifact IDs.
- [x] Split ordinary record artifacts from generated mapping/rule/diagnostic
      artifacts before building record-action scaffolds. Rule coverage,
      material inventory, schema inspection, reconcile, answer, and generated
      mapping artifacts no longer become ordinary derive/filter/join/normalize
      source candidates.
- [x] Added regression coverage proving that script-disabled repair with a
      concrete source/reference record shape produces `normalize_entities`
      rather than another script or schema-only inspection.
- [x] Preserved the existing generated-artifact inspection fallback when no
      concrete scaffold is executable.

Remaining architecture items:

- [ ] Promote this selection into a first-class `ActionGraph` / `ArtifactGraph`
      IR instead of deriving executable scaffolds inside REPL workflow code.
- [ ] Persist artifact roles such as source record set, reference record set,
      generated mapping, rule coverage, diagnostic, contribution, reconcile,
      and projection as typed graph nodes rather than inferring them from compact
      artifact summaries.
- [ ] Let deterministic fallbacks synthesize more next-stage typed actions when
      the graph state is unambiguous, including `enrich_records`,
      `apply_entity_resolutions`, `filter_records`, and contribution-prep
      actions with explicit field contracts.
- [ ] Continue the shared CLI/REPL process-event UX work: render
      model-authored business-facing goal, batch purpose, action summary,
      repair reason, and next step before internal counters, while keeping full
      raw plan/result artifacts in audit logs.

### Batch 190: Accepted-Graph Deferred Queue And Invalid-Suffix Prefix Salvage

A real long-running data DAG exposed two generic convergence hazards in the
runtime itself:

- preflight could split a candidate graph, save the deferred suffix, and only
  then discover that the visible prefix was still rejected by the final guard;
- a graph batch with a valid first action and an invalid later action was
  rejected wholesale, forcing the planner to rewrite useful work instead of
  letting the valid prefix materialize diagnostic or record artifacts.

Both are ActionGraph state bugs, not business-domain bugs. A deferred queue must
contain only typed work from an accepted graph, and a valid executable prefix
should be allowed to run even when a later suffix needs replanning. This lets
the workflow converge from real artifacts and field contracts rather than
repeatedly asking the model to regenerate the same large batch.

Changes:

- [x] Moved deferred-queue saving in both CLI and REPL data workflows until
      after the preflight plan has no final guard error. Rejected candidate
      graphs no longer seed future deferred work.
- [x] Added a deterministic invalid-suffix fallback. When an action batch fails
      staging but an earlier typed prefix passes the same structural guard, the
      runtime executes that prefix with `continue_after=true` and discards the
      invalid suffix so the next planner/evaluator turn replans from real
      prefix results.
- [x] Kept valid multi-rank DAG behavior unchanged: accepted dependency-rank
      splits still preserve their deferred suffix for later readiness checks.
- [x] Added regression coverage proving a valid `inspect_material` prefix can
      run while an invalid `extract_fields` suffix is kept out of the deferred
      queue.

Remaining architecture items:

- [ ] Promote accepted/rejected/deferred graph transitions into a first-class
      `ActionGraph` state reducer. CLI and REPL should call the same reducer
      instead of coordinating deferred queues in local closures.
- [ ] Persist `accepted_graph_id`, prefix action IDs, discarded suffix action
      IDs, and guard reasons in structured audit records.
- [ ] Add typed `ActionValidation` records with `action_index`, `action_id`,
      `kind`, `repairability`, and dependency/field-contract paths so prefix
      salvage does not rely on prose guard text.
- [ ] Implement the shared process-event UX renderer requested in live review:
      when model-authored structured fields such as `goal`, `why_this_batch`,
      `next_batch`, action purpose, repair reason, and task steps are present,
      show those business-facing summaries before internal counters in both
      CLI stderr and REPL permanent lines. Keep the renderer domain-neutral;
      do not introduce business-specific copy or templates.

### Batch 191: Conditional Derivation IR And Operation Contract Diagnostics

The next real run moved beyond invalid deferred graph contamination and into
typed contribution preparation. It exposed a generic compute-expression gap:
the workflow needed to materialize one or more fields whose value depends on
conditions over existing record fields. The planner tried to express that
intent through an unsupported `derive_fields.operation` and then through a
single-record-set action with multiple inputs. The runtime correctly rejected
those plans, but the repair loop still spent turns rediscovering a capability
that the typed IR did not yet provide.

This is not specific to purchase totals or money. Conditional field derivation
appears in many data tasks: choose a duration source by availability, bucket a
metric by thresholds, assign labels from enum states, pick a fallback value
when a primary field is missing, normalize boolean flags, select a timestamp,
or derive an eligibility reason before grouping and aggregation.

Generic invariants:

- conditional derivation is a field-level operation over one existing record
  artifact; it must not read additional materials or decide business meaning;
- the model supplies the conditions and the value/default fields according to
  the current user task and material evidence;
- the system executes only the shared typed filter semantics already used by
  `filter_records`, `qualify_records`, and `compute_contributions`;
- unsupported operations must fail with an operation-contract error first,
  before emitting misleading `source_field` guidance.

Changes:

- [x] Added domain-neutral `derive_fields` operation aliases
      `case_when`/`case`/`conditional`/`if_then`/`select`.
- [x] Added structured `cases` support. Each case can carry shared typed
      filters plus either `value_field`/`source_field` or a literal `value`,
      with optional `default_field` or `default` fallback.
- [x] Reused the existing contribution/filter predicate implementation so
      conditional derivation, filtering, qualification, and contribution guards
      share one comparison contract.
- [x] Reordered derive-field validation so unsupported operations report the
      unsupported operation and supported operation list before any
      source-field-specific error.
- [x] Updated the data planner prompt and action scaffold to teach
      `case_when` as a generic typed field operation, not as a business-specific
      calculation shortcut.
- [x] Added regression coverage for conditional value selection and
      unsupported-operation diagnostics.

Remaining architecture items:

- [ ] Promote derive-field specs into typed `ExpressionIR` instead of passing
      operation/cases through stringly action params. `ExpressionIR` should
      represent field references, literal values, predicates, numeric parsing,
      string transforms, and default branches as typed nodes.
- [ ] Feed expression-contract errors into structured `ActionValidation`
      records with JSON paths to the offending spec/case, so repair can be
      local and does not require a full batch rewrite.
- [ ] Add action-lineage summaries that show which generated artifact version
      materialized each field. Later `filter_records`, `qualify_records`, and
      `compute_contributions` should bind by field contract and lineage, not
      by alias guessing.
- [ ] Add shared business-facing process events for expression stages: show
      the model-authored purpose/why/next-step text, then compact structural
      counters, keeping stdout/final answers clean.

### Batch 192: Text Numeric Extraction Ambiguity Contract

The next real run exposed a lower-level extraction issue. A typed
`extract_fields` action asked the runner to parse a number from a long text
field that contained several unrelated numeric tokens. The old behavior reused
the generic `parse_number` primitive and therefore selected the first decimal
token. That is structurally unsafe for any text-to-record task where the target
numeric value is embedded among ids, dates, counts, line numbers, durations,
percentages, totals, or other measurements.

This is not a business-domain rule. The runtime does not know which number is
important and does not hardcode file names, labels, currencies, invoice words,
date formats, or domain keywords. The model remains responsible for reading
the user goal and material shape, then choosing the right extraction pattern or
grouping strategy. The system only rejects one precise structural shape:
unanchored numeric parsing over a long or multi-line text field that contains
multiple numeric candidates.

Generic invariants:

- `parse_number` remains valid for already-isolated numeric-looking fields;
- `extract_fields` over long/multi-line text must anchor numeric extraction
  with a model-declared pattern, capture group, or prior grouping/splitting
  step when multiple numeric tokens are present;
- the hard gate is driven by typed action params and token counts, not by
  business words or model prose;
- the repair hint tells the model what structural information is missing
  without deciding the business answer.

Changes:

- [x] Added an ambiguity check for `extract_fields` numeric parsing: if the
      source text is long or multi-line, has multiple decimal tokens, and the
      spec uses unanchored `parse_number`, the action returns a repairable
      structural error.
- [x] Kept ordinary `derive_fields(parse_number)` and isolated numeric fields
      unchanged, so existing structured-table and metric-normalization paths
      are not disturbed.
- [x] Reused a shared decimal-token helper for both ambiguity detection and
      numeric parsing so the contract is internally consistent.
- [x] Updated planner guidance and action scaffolds to prefer
      `regex_extract` with a context pattern/capture group, or a prior
      grouping/splitting step, when text contains several numeric candidates.
- [x] Added regression coverage proving ambiguous text parsing is rejected and
      anchored regex extraction succeeds.

Remaining architecture items:

- [ ] Promote extraction specs into typed `ExtractionIR` with explicit source
      scope, anchor pattern, candidate count, source locator, and repairability
      fields.
- [ ] Emit structured `ActionValidation` records for ambiguous extraction
      with JSON paths to the offending action/spec instead of relying on error
      text.
- [x] Add a domain-neutral value-distribution action so the planner can cheaply
      inspect candidate field values before choosing filters or contribution
      params.
- [ ] Add a dedicated extraction-preview action only if value distribution plus
      typed extraction diagnostics are insufficient for candidate token
      selection.
- [ ] Feed extraction ambiguity diagnostics into the shared business-facing
      process-event renderer so users can see the model is refining a field
      extraction, not blindly retrying.

### Batch 193: Executable Scaffold IR For Script-Disabled Stages

The next real run moved past entity normalization and rule coverage, but the
continuation planner still fell back to a broad `custom_transform` script when
the workflow needed to apply existing entity mappings, qualify records, compute
contributions, reconcile, and assemble the answer. The workflow guard rejected
that script correctly because `custom_transform_disabled=true`, but rejection
alone is not a convergence strategy. The runtime needs an executable typed
scaffold that consumes current graph facts instead of asking the model to
re-plan the same large script from a heavy prompt.

This is a generic DAG/IR issue. Any data workflow can reach a state where a
source-to-canonical ledger already exists and the next atomic step is to apply
that ledger back to base records before filtering, joining, or contribution
calculation. The system must preserve ordinary record artifacts as base
candidates even when they have id/name fields, and must treat only actual
entity-resolution ledgers as resolution inputs.

Generic invariants:

- prompt artifact samples may be bounded, but executable scaffolds should read
  the full artifact schema projection;
- when entity-resolution ledgers are already materialized and contribution or
  reconcile ledgers are still missing, deterministic fallback should prefer
  applying existing ledgers over regenerating mappings or inspecting schemas;
- ordinary records with id/name-like fields are not resolution ledgers and
  must remain eligible as base records;
- scaffold-generated field names are neutral and derived from artifact aliases,
  not business columns, currencies, vendors, categories, or task-specific
  words.

Changes:

- [x] Switched data action scaffold generation to use executable
      `ArtifactSchemaProjection` contract access instead of the bounded prompt
      sample access.
- [x] Added concrete `apply_entity_resolutions` scaffold execution support so
      `custom_transform_disabled` fallback can emit a typed action rather than
      schema inspection or another script repair.
- [x] Prioritized concrete scaffold candidates by workflow state: when entity
      ledgers exist and compute/reconcile is still missing, prefer
      `apply_entity_resolutions` / enrichment / join / qualification /
      contribution scaffolds before fresh normalization.
- [x] Narrowed apply-resolution mapping detection to real entity-resolution
      ledgers. Generic reference-looking records are no longer excluded as
      base inputs just because they contain id/name fields.
- [x] Added regression coverage proving a rejected broad script becomes a
      concrete `apply_entity_resolutions` action with executable params and no
      placeholder fields.

Remaining architecture items:

- [ ] Move scaffold selection into a first-class `ActionGraph` reducer in
      `internal/dataworkflow`, so CLI/REPL do not coordinate fallback order in
      local helper functions.
- [ ] Add typed `GraphTransition` audit records for rejected script plans,
      selected scaffold action, reason code, candidate inputs, and discarded
      alternatives.
- [ ] Add per-action schema lineage so scaffold field names can point to the
      artifact version that produced them, not only to an alias.
- [ ] Extend concrete scaffolds for `qualify_records`, `compute_contributions`,
      and `assemble_answer` where the current workflow state contains enough
      typed field contracts to proceed without another full planner turn.

### Batch 194: Existing Canonical Signal Verification In Apply Stage

The next real run reached typed normalization, application, contribution, and
reconciliation, but the final answer was internally consistent and still
wrong. The root cause was a generic cross-stage contract gap:
`normalize_entities` can treat an existing canonical key/code/id on the source
record as a strong structural signal, but `apply_entity_resolutions` only knew
how to write back accepted mapping-ledger rows. When text matching failed or
was scoped to a raw text field, the apply stage cleared the target canonical
field even though the base record already carried a canonical value that could
be verified against a reference table.

This is not specific to any business domain. Many data workflows have both
noisy descriptive fields and structured canonical identifiers: accounts,
devices, people, locations, SKUs, tags, categories, services, departments, or
other dimensions. A text-normalization miss must not erase an already-valid
structured canonical signal, but the runtime also must not blindly trust source
values as canonical facts.

Generic invariants:

- the model decides whether an existing source field is relevant for the
  current dimension by declaring `existing_id_field`;
- the system verifies that value structurally through `reference_path` and
  `reference_id_field` before using it;
- optional `reference_label_field` and `reference_status_field` /
  `reference_accepted_statuses` refine evidence and filtering, but the system
  does not hardcode any status values;
- duplicate, missing, absent, or status-filtered reference matches remain
  unmatched instead of being silently accepted;
- this fallback applies only when the mapping ledger does not produce exactly
  one accepted choice, so normal resolved mapping rows keep their precedence.

Changes:

- [x] Extended the `apply_entity_resolutions` spec IR with generic
      `existing_id_field`, `existing_label_field`, `reference_path`,
      `reference_id_field`, `reference_label_field`,
      `reference_status_field`, and `reference_accepted_statuses`.
- [x] Added runner-side reference indexing and verification. Existing source
      values are materialized only when the reference input contains exactly
      one structurally valid record under the optional status filter.
- [x] Added audit fields and child artifacts for existing-ID verification:
      verified counts, reference path, reference ID field, label/status fields,
      and candidate counts.
- [x] Added REPL workflow guard checks for the new spec fields so missing base
      or reference fields are caught before execution when artifact schemas are
      visible.
- [x] Updated planner guidance to teach verified existing canonical signals as
      a generic apply-stage capability, not a domain-specific shortcut.
- [x] Updated executable apply-resolution scaffolds to include
      `existing_id_field` and reference verification params when current
      artifact schemas expose an obvious structural match.
- [x] Added regression coverage proving verified existing canonical IDs are
      preserved while missing or filtered reference values remain unmatched.

Remaining architecture items:

- [ ] Promote `apply_entity_resolutions` specs into typed `ApplyResolutionIR`
      instead of string params, with separate mapping-ledger sources,
      existing-signal sources, reference contracts, and target fields.
- [ ] Feed verified-existing counts, duplicate reference counts, and
      unmatched-by-reference diagnostics into evaluator state as typed signals.
- [ ] Add a domain-neutral evidence-fusion action for dimensions where the
      source record has no canonical key/code/id but several descriptive fields
      or auxiliary evidence records can support a canonical choice.
- [ ] Move scaffold selection and verified-reference discovery into the shared
      `ActionGraph` reducer once that reducer exists, so CLI and REPL use the
      same transition logic.

### Batch 195: Locator-Compatible Resolution Application

The next real run improved canonical-signal preservation but exposed a
structural guard mismatch. A valid resolution artifact used `item_id` as its
locator, while the base record set exposed `_source_index`. Both values can
refer to the same record identity when the resolution item id contains a
source-row locator, but the planner guard treated them as unrelated fields and
rejected the next typed `apply_entity_resolutions` batch.

This is a generic locator-contract issue. Data tasks often carry row identity
through different layers as `_source_index`, `source_index`, `_source_line`,
`row_index`, `line`, `item_id`, or `source_locator`. These are not business
fields. They are structural provenance handles. The system should allow safe
alignment when both sides are locator-compatible, while still rejecting
ordinary missing business fields.

Changes:

- [x] Extended REPL workflow guard locator equivalence so base locator fields
      such as `_source_index`, `source_index`, `row_index`, and source-line
      aliases can align with resolution `item_id` / `source_locator` fields.
- [x] Mirrored the same equivalence in the action runner's
      `apply_entity_resolutions` locator matching path.
- [x] Preserved strict field validation for non-locator fields: the guard still
      fails when a model references an ordinary field that is absent from the
      selected artifact schema.
- [x] Added regression coverage proving an apply-resolution plan with
      `resolution_key_fields=["_source_index"]` can consume an `item_id`-based
      resolution artifact when row-locator evidence is compatible.

Remaining architecture items:

- [ ] Promote locator compatibility into a shared `LocatorContract` IR instead
      of duplicating equivalence helpers across planner guard and runner code.
- [ ] Feed locator alignment decisions into graph-transition audit records so
      later debugging can distinguish safe structural equivalence from
      ordinary field inference.
- [ ] Add per-artifact provenance summaries to the `ActionGraph` reducer once
      that reducer owns scaffold selection.

### Batch 196: Text Collection Materials As Typed Record Sets

The next real run then reached a material/evidence boundary. The workflow had
objective material-set handles for local text evidence, but typed actions could
only consume individual files or generated artifacts. When the model needed to
extract fields from many same-schema text snippets, it drifted toward custom
directory traversal scripts and large continuation prompts.

This is not a file-name or business-domain problem. Any data workflow can have
a local collection of same-schema text materials: extracted text evidence,
OCR/text outputs, web snippets, message exports, log fragments, notes, or other
bounded local text files. The runtime should expose such a collection through
the same typed record interface used by CSV/JSON/JSONL/text files, so the model
only supplies the extraction specs and the system handles deterministic
material reading and provenance.

Changes:

- [x] Let typed action record readers consume a directory input as one text
      record per regular child file, sorted by file name.
- [x] Exposed generic fields for each child record: `text`, `file_name`,
      `file_path`, optional `text_truncated`, and the normal source-locator
      virtual fields.
- [x] Kept reads bounded per file and non-recursive so directory materials stay
      predictable and auditable.
- [x] Updated planner guidance to prefer typed `extract_fields` over custom
      directory scripts when a same-schema local text collection is relevant.
- [x] Added regression coverage proving `extract_fields` can read a directory
      material, extract model-declared fields from `text` / `file_name`, and
      preserve child-file source paths.

Remaining architecture items:

- [ ] Promote material-set handles into a typed `MaterialCollectionIR` with
      collection kind, member count, readable fields, child locator policy, and
      bounded-read diagnostics.
- [ ] Add recursive or manifest-driven collection expansion only through a
      typed collection policy, not by implicit directory walking.
- [ ] Surface collection-read summaries in the business-facing process event
      renderer using model-authored purpose/next-step text plus compact
      structural counters.

### Batch 197: Pre-Continuation Typed Graph Fallback

The next real run showed that the workflow could reach a precise typed state:
material coverage was sufficient, full CSV record artifacts existed, entity
resolution was still missing, and the next legal actions were already listed
in `workflow_state_json.allowed_next_actions`. However, the runtime still sent
a large continuation prompt to the model before trying to advance the graph.
That prompt grew past 150 KB and stalled before returning a tool call.

This is a scheduler/IR issue, not a business-domain issue. When the deterministic
workflow state already contains a next stage, available artifact schemas, and
concrete typed action scaffolds, the runtime should try a graph transition
first. The model should be used to interpret business semantics and generate
new graph intent when the state is ambiguous, not to rediscover obvious
structural transitions after every successful batch.

Changes:

- [x] Extended `dataTaskWorkflowNextStageFallbackWithRepo` so
      `normalize_or_enrich_entities`, `prepare_contribution_inputs`, and
      `compute_contributions` can emit a concrete typed action scaffold before
      calling the continuation planner.
- [x] Reused the existing artifact schema projection and scaffold generation
      instead of introducing a second execution path.
- [x] Added generic source/reference role scoring for normalization scaffolds:
      source artifacts with raw/input/source cues and reference artifacts with
      canonical id/code/value plus label/name/description evidence are ranked
      ahead of inverted pairs.
- [x] Kept the scoring structural and domain-neutral. It does not inspect
      business words, file names, currencies, vendors, categories, or task
      output values.
- [x] Updated the custom-transform-disabled fallback to benefit from the same
      next-stage scaffold selection, avoiding a join/script detour when a
      normalization scaffold is structurally available.
- [x] Added regression coverage proving next-stage fallback can directly emit
      `normalize_entities` from existing record artifacts and that the older
      custom-transform-disabled normalize fallback remains stable.

Remaining architecture items:

- [ ] Move this scaffold selection into a first-class `ActionGraph` reducer in
      `internal/dataworkflow` so CLI and REPL no longer coordinate transition
      order through REPL-local helper functions.
- [ ] Persist graph-transition audit records with previous stage, chosen
      action kind, candidate score, input aliases, and rejected alternatives.
- [ ] Add capability metadata for `enrich_records`, `qualify_records`, and
      `compute_contributions` so more post-normalization graph steps can
      advance without a large continuation prompt when schemas are sufficient.
- [ ] Replace scaffold display sampling with separate prompt views and
      executable IR views; prompt scaffolds can stay compact, but reducer
      decisions must see the full artifact schema projection.

### Batch 198: RecordSet Bootstrap After Continuation No-Tool

The next real run avoided the earlier long stall, but the continuation planner
returned reasoning text without an `emit_data_task_plan` tool call after rule
coverage had completed. The deterministic state was still enough to proceed:
material coverage was sufficient, rules were derived, contribution/reconcile
ledgers were missing, and no reusable record artifact existed yet.

This is a generic state-machine gap. A workflow should not fail just because a
planner no-tool response occurred at a stage where the next structural action
is obvious: turn required local materials into reusable record sets. The
system still does not decide business fields or filters here. It only performs
the neutral MaterialGraph -> RecordSetGraph transition.

Changes:

- [x] Added `RecordSetBootstrap` fallback for continuation/no-tool and
      batch-result continuation paths: when the next stage allows
      `extract_records`, material coverage is sufficient, and no record action
      artifact exists, emit a bounded `extract_records` batch over executable
      required materials.
- [x] Skipped `planner_distilled` and `reference_only` materials for this
      bootstrap, and used `text_evidence_path` for `text_evidence_consumed`
      materials.
- [x] Preserved terminal-plan safety: terminal fallback does not invent an
      `extract_records` stage from a blocked/complete plan.
- [x] Added regression coverage for continuation no-tool -> record
      materialization, and for the older terminal fallback safety invariant.

Remaining architecture items:

- [ ] Move material-to-record bootstrap into `internal/dataworkflow` as a
      typed `MaterialGraph -> RecordSetGraph` transition with explicit input
      material roles and readiness checks.
- [ ] Use candidate-file metadata in the reducer so non-text originals,
      related text evidence, directories, and generated artifacts are selected
      through a first-class material-readability contract rather than only
      coverage usage mode.
- [ ] Persist bootstrap decisions in graph-transition audit records, including
      selected paths, skipped material modes, and whether each selected path
      was original material or text evidence.

### Batch 199: IR-First Architecture Audit And Backlog Consolidation

The latest end-to-end code audit and real-scenario run showed that the data
lane is on the right path, but the architecture is still split between two
models:

- `internal/dataquery` already executes typed atomic actions and emits typed
  validation/result objects.
- `internal/dataworkflow` already contains the first IR boundary and shared
  action-capability table.
- The actual workflow reducer, graph transition rules, deferred queue,
  scaffolds, stage guards, material-scope merging, and process-event summaries
  still live mostly in `internal/repl/data_task_workflow.go` and the CLI/REPL
  loops.

That split explains the latest failure mode. The workflow reached a state with
material coverage, record artifacts, rule coverage, and entity-resolution
records, then repeatedly executed `apply_entity_resolutions` over increasingly
long `resolved_records_...` artifacts. The loop was not a procurement-specific
business mistake. It was a missing graph invariant: applying the same ledger to
the same or equivalent record lineage is an idempotent graph edge, not a new
productive node that should consume many workflow rounds while contribution
records remain absent.

The "Remaining architecture items" throughout this document are therefore not
all equal. Some are already covered by later batches or represent longer-term
hardening. The execution backlog is now consolidated into three levels:

P0, blocking current convergence:

- [ ] Move the reducer logic for next stage, allowed actions, missing ledgers,
      artifact readiness, and repeated-node/no-progress detection behind a
      first-class `DataWorkflowState` projection in `internal/dataworkflow`.
      REPL/CLI may adapt/render the state, but should not own workflow truth.
- [ ] Add `ActionDAG` transition records with node id, action kind, input
      aliases, output alias, dependency rank, producer/consumer edge, and
      idempotency key. A repeated edge that cannot add fields, rows, or ledgers
      must be rejected or redirected before execution.
- [ ] Treat `apply_entity_resolutions` as a row-preserving ledger-application
      edge. If a base lineage already contains the target canonical/status
      fields for the same resolution ledger, the next transition must advance
      toward contribution preparation, compute, reconcile, or typed diagnostics
      rather than reapplying the same ledger to a derived artifact.
- [ ] Promote action-level guard failures and zero-progress/repeated-edge
      findings into typed `WorkflowViolation` objects instead of prose-only
      error strings, preserving action id/kind, input aliases, output alias,
      expected graph transition, actual graph transition, and repairability.
- [ ] Add deterministic graph-transition fallbacks for contribution readiness:
      when entity ledgers and record artifacts exist but contribution records
      are absent, choose typed field derivation/filter/qualification/compute
      scaffolds from artifact schema contracts before asking a large
      continuation prompt to rediscover the same graph state.

P1, commercial stability and auditability:

- [ ] Persist ActionDAG, MaterialGraph, ArtifactGraph, and LedgerGraph snapshots
      in data-audit records so a failed customer run can be audited without
      reconstructing state from panel lines.
- [ ] Promote artifact schema projections into a first-class `ArtifactGraph`
      with producer action, aliases, row counts, field origins, lineage,
      version, and diagnostic confidence. Prompt views can stay compact, but
      validators and reducers should consume the full graph.
- [ ] Promote rule/decision/entity/contribution/reconcile/final-projection
      handles into a first-class `LedgerGraph`, including target-vs-audit roles,
      status distributions, de-duplication counts, and missing required handles.
- [ ] Add domain-neutral graph-expansion actions for value coverage,
      value-distribution/field preview, and mapping candidates. The model
      decides business meaning; the system only reports objective overlap,
      missing, ambiguous, and sample evidence.
- [ ] Upgrade CLI/REPL process events to render model-authored business goal,
      batch purpose, next step, and reason from structured plan/evaluation data
      while keeping internal graph counters as low-noise audit details.

P2, hardening and release gates:

- [ ] Add multi-run real-scenario eval gates with correctness checks, ledger
      checks, reconcile checks, volatility reporting, and combined status
      reporting before treating data lane as default-on ready.
- [ ] Add doc lint/status checks so broad backlog items cannot be mistaken for
      completed delivery, and so already-implemented items are periodically
      reconciled against tests and code.

Architectural invariant:

- The model owns business interpretation and may propose graph intent.
- The system owns objective material inventory, typed graph readiness,
  idempotency, schema contracts, ledger/reconcile validation, and audit.
- Hard gates consume typed fields only. Model/user prose is soft guidance unless
  it has been parsed into schema-validated IR.
- Domain-neutral numeric/text extraction contracts must stay generic. For
  example, a text field with multiple numeric tokens cannot be consumed by an
  unanchored `parse_number`; the model must provide a context pattern,
  grouping/splitting step, or a source field with one numeric candidate. This
  applies to any numeric unit or measure, not only dates or amounts.

### Batch 200: Contract-Layer IR Exposure

The IR audit found a low-risk but important split: execution had already
separated workflow-level coverage from current-batch inputs, but the prompt
state still mostly exposed a single compact coverage picture. That forced the
planner and evaluator to infer whether a material was a durable task
requirement, a last-batch executable input, or a temporary helper. The runtime
could guard some mistakes later, but the model did not receive the clean typed
boundary early enough.

This batch makes the boundary explicit without changing business semantics or
runner behavior. The system projects coverage contracts into domain-neutral IR
views:

- `workflow_contract`: durable task requirements, required runner inputs,
  validation-rule count, and ledger requirements.
- `current_batch_contract`: the executable slice for the current plan, or the
  last executed batch when no current plan exists.

The projection is intentionally objective. It does not classify a file as a
business role, does not infer domain meaning from filenames, and does not parse
user prose. It only serializes schema-validated coverage fields that already
exist in the plan/result contracts.

Changes:

- [x] Added `dataworkflow.CoverageContractView` and
      `MaterialContractView` as reusable typed contract projections.
- [x] Exposed `workflow_contract` and `current_batch_contract` in
      `workflow_state_json`.
- [x] Reused one helper for execution-batch contract scoping and prompt-state
      projection so the displayed contract and runner contract do not drift.
- [x] Updated continuation/evaluator rules to consume the two typed contract
      layers instead of relying on one merged coverage view.
- [x] Added regression coverage for required path vs runner-input path
      projection and for durable workflow requirements being distinct from
      current-batch executable inputs.

Remaining architecture items:

- [ ] Move more contract/reducer construction from `internal/repl` into
      `internal/dataworkflow`, starting with material-floor/current-input
      scoping and graph-transition violations.
- [ ] Persist the contract-layer projection in data-audit records together
      with ActionDAG/ArtifactGraph/LedgerGraph snapshots.
- [ ] Add a typed material-promotion transition so the model can request
      promotion from helper/current input to workflow-required only through a
      schema-validated graph action.

### Batch 201: ActionDAG Node Projection And Idempotency Keys

The next IR migration step is to make action graph identity visible as typed
state before moving the full reducer. Previously, REPL-level guards could
detect some repeated work, but there was no reusable action-node projection
that represented the graph edge itself. That makes it too easy for separate
guards, prompts, and deferred-queue code to disagree about whether a batch is
new progress or a replay of the same edge.

This batch adds a domain-neutral ActionDAG projection:

- action kind normalized through the shared capability table;
- dependency rank from `internal/dataworkflow`;
- input aliases and output alias;
- ledger capabilities;
- a stable idempotency key derived from action kind, inputs, output alias,
  structural params, and script digest.

The key is intentionally structural. It does not inspect business meaning,
filenames, model prose, or user intent. It can identify graph replay across
tables, JSONL, text-extraction, OCR evidence, spreadsheet-like transforms,
joins, contribution calculation, and final projection without fitting any
single customer task.

Changes:

- [x] Added `dataworkflow.ActionNodeFor`, `ActionNodesFor`, and
      `ActionIdempotencyKey`.
- [x] Extended `ActionNode` with status, dependency rank, and idempotency key.
- [x] Exposed recent executed nodes and current ready nodes in
      `workflow_state_json.action_graph`.
- [x] Updated continuation/evaluator rules to treat `action_graph` as
      structural audit state for avoiding graph replay, not as business
      evidence.
- [x] Added regression coverage for action-node projection, stable
      idempotency keys, and workflow-state action-graph exposure.

Remaining architecture items:

- [ ] Move repeated-edge/no-progress guards to consume `ActionNode`
      idempotency keys and typed artifact/ledger deltas instead of local
      REPL-only helper logic.
- [x] Persist action graph snapshots in data-audit records. Completed in
      Batches 207 and 214.
- [x] Expose deferred queued nodes as `ActionNode{status=deferred}` in the
      shared reducer.
      Completed for deferred queue audit snapshots in Batch 217.
- [ ] Add typed `WorkflowViolation` records that point to action-node
      idempotency keys, dependency ranks, and blocked input aliases.

### Batch 202: Typed Role Path Normalization And Scaffold Hygiene

A stopped real-scenario run exposed two generic IR boundary gaps before final
correctness could be evaluated:

- the planner emitted typed role paths such as `source_path` and
  `reference_path` for `normalize_entities`, but the executor required those
  paths to also appear in `input_paths`;
- after repair, the workflow generated repeated
  `apply_entity_resolutions` scaffolds over diagnostic child artifacts such as
  `...#base` and over the cumulative `workflow_entity_resolutions` ledger
  handle, causing long `resolved_records_...` chains instead of progressing to
  contribution readiness.

Neither problem is domain-specific. Typed actions often have role paths:
source/reference, left/right, base/mapping, resolution/reference. The system
should normalize those structured role fields into executable action inputs.
Likewise, runner diagnostic children and aggregate workflow ledger handles are
valid audit artifacts, but they are not automatically reusable primary
record-action bases or single-dimension mapping ledgers.

Changes:

- [x] Added role-path normalization that fills action `input_paths` from typed
      params for `normalize_entities`, `enrich_records`, `join_records`, and
      `apply_entity_resolutions`.
- [x] Reused the normalization in both plan-shape repair and execution
      preparation so initial, continuation, repair, fallback, and deferred
      plans share the same boundary.
- [x] Excluded diagnostic child artifacts (`#base`, `#mapping`,
      `#entity_source`, `#entity_reference`, and matching kind suffixes) from
      automatic record-action scaffolds.
- [x] Excluded aggregate workflow ledger handles from automatic
      `apply_entity_resolutions` scaffolds. Explicit model plans may still use
      those handles with concrete `resolution_specs`; the system simply no
      longer invents a generic `workflow_canonical_id` application loop.
- [x] Added regression coverage for role-path input normalization and for
      scaffold filtering of diagnostic children / aggregate workflow ledgers.

Remaining architecture items:

- [x] Promote role-path normalization into `internal/dataworkflow` as an
      ActionDAG normalization pass shared by REPL, CLI, and future batch
      schedulers. Completed in Batch 205.
- [x] Represent diagnostic child artifacts and aggregate ledger handles as
      typed `ArtifactGraph` node classes instead of local REPL predicates.
      Completed in Batch 204.
- [ ] Add typed scaffold eligibility diagnostics so skipped scaffolds are
      visible in audit without becoming user-facing noise.

### Batch 203: Unified WorkflowViolation Projection

The remaining reducer gap is not just whether an error can be detected. Many
errors were already detected, but they were exposed as a mixture of prose
strings and several specialized arrays: field-contract issues, zero-match
filters, unmatched entity-resolution applications, and zero-eligible
qualification outputs. That fragmentation makes the planner and evaluator do
too much interpretation work.

This batch adds a unified typed violation projection while preserving the older
specialized fields for compatibility during migration. It does not change
execution behavior and does not infer business meaning. It only projects
already-typed structural findings into one schema:

- `code`;
- `severity`;
- `repairability`;
- action id/kind and input/output aliases where known;
- missing fields, candidate artifacts, available-field samples, and repair
  action hints.

Changes:

- [x] Added `dataworkflow.WorkflowViolation` and repairability enums.
- [x] Exposed `workflow_state_json.workflow_violations`.
- [x] Projected existing field-contract, zero-match-filter,
      unmatched-resolution, and zero-eligible-record findings into the unified
      violation list.
- [x] Updated continuation/evaluator prompt rules to repair from typed
      violation fields before relying on prose errors.
- [x] Added regression coverage that a field-contract gap appears in
      `workflow_violations` with missing fields and typed repairability.

Remaining architecture items:

- [ ] Make staging guards return typed `WorkflowViolation` objects directly
      instead of formatting prose first and re-parsing some errors later.
- [ ] Link violations to `ActionNode.idempotency_key` and dependency rank once
      the full ActionDAG reducer owns scheduling.
- [x] Persist violations in data-audit snapshots. Completed in Batch 214.
- [ ] Surface concise business-facing repair summaries in CLI/REPL process
      events.

### Batch 204: ArtifactGraph Node Classes

The scaffold hygiene fix first used REPL-local predicates to distinguish
diagnostic child artifacts and aggregate workflow ledger handles. That was a
useful safety patch, but keeping node-class truth in REPL would repeat the
same architectural split we are trying to remove. Artifact classification
belongs in the ArtifactGraph projection.

This batch moves the classification into `internal/dataworkflow`:

- ordinary generated/data artifacts;
- record-shaped artifacts;
- diagnostic child artifacts such as base/mapping/source/reference children;
- aggregate workflow ledger handles.

The classes are structural. They are derived from system artifact id/kind
shape and typed workflow-ledger fields, not from business names or user prose.
REPL scaffolds can now consume `node_class` rather than hard-owning the
classification.

Changes:

- [x] Added `node_class` to `ArtifactSchemaProjection`.
- [x] Added `ArtifactNodeClass`, `IsDiagnosticChildArtifact`, and
      `IsWorkflowLedgerArtifact` helpers in `internal/dataworkflow`.
- [x] Populated `node_class` in both full artifact contract access and compact
      prompt artifact access.
- [x] Rewired REPL scaffold filtering to consume dataworkflow node classes,
      with helper fallback only for older prompt views.
- [x] Added regression coverage for workflow ledger and diagnostic child
      classification.

Remaining architecture items:

- [ ] Move scaffold eligibility fully into an ArtifactGraph-aware reducer
      rather than REPL helper functions.
- [x] Persist artifact node classes and lineage snapshots into data-audit
      records.
      Completed in Batch 212.
- [x] Add `artifact_schema_projection` validation to action guards so typed
      actions consume exact executable schema contracts instead of prompt
      samples. Completed for field-contract guards in Batch 209.

### Batch 205: Shared ActionDAG Role-Path Normalization

Batch 202 fixed role-path drift at the REPL boundary. That was useful but still
left a copy of ActionDAG edge normalization in the wrong layer. If role paths
are structural action parameters, then converting them into executable
`input_paths` is part of the graph normalization pass, not a UI/runtime-local
helper.

This batch moves the pass into `internal/dataworkflow` without changing
business semantics:

- `normalize_entities`, `enrich_records`, `join_records`, and
  `apply_entity_resolutions` can declare role-specific paths such as
  source/reference, left/right, base/record, resolution/reference;
- the shared normalizer appends those role paths to action `input_paths` in
  stable order and deduplicates exact aliases;
- nested resolution specs are parsed only as typed JSON objects/arrays, not as
  prose;
- the normalizer does not infer field mappings, business meaning, filter
  values, or which records should contribute to an answer.

Changes:

- [x] Added `dataworkflow.NormalizeRolePathActionInputs` and
      `ActionRoleInputPaths`.
- [x] Replaced the REPL-local implementation with calls into the shared
      dataworkflow pass.
- [x] Kept both plan-shape normalization and execution preparation on the same
      pass, so initial, continuation, repair, fallback, and deferred plans
      share one boundary.
- [x] Added regression coverage for normalize/enrich/join/apply-resolution
      role paths, nested resolution specs, input ordering, and no-op behavior
      when a plan is already normalized.

Remaining architecture items:

- [x] Make the full ActionDAG reducer call normalization before every graph
      readiness and idempotency calculation, rather than relying on each caller
      to remember the pass. Completed for current ActionGraph projection and
      idempotency keys in Batch 206.
- [x] Persist normalized action nodes and pre-normalization params in
      data-audit snapshots so execution edges can be audited without replaying
      planner prompts. Completed in Batch 207.

### Batch 206: Projection-Native Action Normalization

Batch 205 exposed a shared normalizer, but `ActionNodeFor` and
`ActionIdempotencyKey` could still be called directly with a raw planner action.
That left an avoidable footgun: one caller might see role-path params as graph
inputs while another caller hashes only explicit `input_paths`.

This batch makes ActionGraph projection normalize its own input action before
deriving graph edges or idempotency keys. The behavior remains structural and
domain-neutral: it only copies typed role-path params into action inputs. It
does not inspect filenames for business meaning, parse user prose, or change
the action's field/filter/value semantics.

Changes:

- [x] Added a per-action `dataworkflow.NormalizeRolePathAction` helper.
- [x] Made `ActionNodeFor` normalize before projecting input aliases.
- [x] Made `ActionIdempotencyKey` normalize before hashing structural inputs.
- [x] Added regression coverage that a raw role-param action and an already
      normalized action produce the same idempotency key and graph inputs.

Remaining architecture items:

- [x] Persist normalized action nodes and original planner params together in
      data-audit snapshots. Completed in Batch 207.
- [x] Move scheduler readiness decisions from REPL record slices into a
      durable ActionDAG reducer that stores node status transitions. Completed
      for current graph projection in Batch 208.

### Batch 207: ActionGraph Audit Snapshots

The ActionGraph projection was visible to the planner through
`workflow_state_json`, but full forensic audit still required piecing together
plan JSON, result JSON, and prompt logs. For commercial support this is too
fragile: operators need to inspect the exact graph edge the system believed it
was about to run, including both planner-authored params and normalized
execution inputs.

This batch adds a domain-neutral data-audit artifact beside each plan:

- original planner action;
- normalized action after structural role-path input normalization;
- projected `ActionNode` with input aliases, output alias, dependency rank,
  ledger capabilities, and idempotency key.

It is audit-only. It does not alter scheduling, validation, or model prompts,
and it does not infer business meaning from names or prose.

Changes:

- [x] Added `.actions.json` audit files for data plans with typed actions.
- [x] Logged the full action graph snapshot in debug/audit logs.
- [x] Added compact REPL preview links to the full action graph artifact.
- [x] Added regression coverage proving normalized inputs and idempotency keys
      are persisted for audit.

Remaining architecture items:

- [x] Persist action graph snapshots across terminal workflow summaries, not
      only per-plan artifacts.
      Completed in Batch 214.
- [x] Move scheduler readiness and status transitions from REPL record slices
      into the durable ActionDAG reducer. Completed for current graph
      projection in Batch 208.

### Batch 208: Shared ActionGraph Reducer

The REPL previously built `workflow_state_json.action_graph` by iterating its
private workflow record slice and directly appending executed/failed/ready
nodes. That kept graph status transitions coupled to REPL storage. It also
made future CLI or standalone workflow runtimes likely to reimplement a subtly
different reducer.

This batch introduces a shared, domain-neutral reducer in `internal/dataworkflow`:

- callers provide typed action events with a status and an optional current
  executable action list;
- the reducer projects executed/failed and ready nodes through the same
  normalization and idempotency path;
- recent executed nodes can be limited without changing graph semantics;
- REPL now adapts its records into `ActionEvent` and delegates the reduction.

The reducer still does not decide business correctness or infer user intent.
It only turns typed action batches into graph state.

Changes:

- [x] Added `dataworkflow.ActionEvent`.
- [x] Added `dataworkflow.ReduceActionGraph`.
- [x] Rewired REPL `dataTaskWorkflowActionGraph` to use the shared reducer.
- [x] Added regression coverage for executed/failed status projection, ready
      projection, limit trimming, and role-path normalization during reduction.

Remaining architecture items:

- [ ] Persist reducer event streams, not only projected snapshots, so
      interrupted workflows can replay state transitions exactly.
- [ ] Move deferred queues and blocked-node reasons into the shared reducer
      instead of REPL-specific slices and guard errors.

### Batch 209: ArtifactSchema Field Contract Helper

Field-contract guards were structurally correct but still owned their schema
lookup in REPL helper code. The same exact check will be needed by CLI,
standalone data workflows, and future reducer-level blocked-node decisions.
Keeping the lookup in UI code makes those paths diverge.

This batch moves the schema lookup and missing-field calculation into
`internal/dataworkflow`:

- resolve an `ArtifactSchemaProjection` by id/path/alias;
- compare requested fields against the artifact's exact field list;
- return missing fields only when a schema is known;
- preserve the existing safe behavior that unknown schemas do not hard-block
  a plan by themselves.

This is precise structural validation, not business inference. It does not
guess field meaning or parse model prose.

Changes:

- [x] Added `dataworkflow.ArtifactSchemaByAlias`.
- [x] Added `dataworkflow.MissingFieldsOnArtifactSchema`.
- [x] Rewired REPL missing-field action guards to consume the shared
      `ArtifactSchemaProjection` helper.
- [x] Added regression coverage for alias resolution, duplicate missing-field
      cleanup, and unknown-schema no-op behavior.

Remaining architecture items:

- [x] Move candidate-artifact ranking and repair hints for field-contract
      violations into a shared typed schema-violation builder.
      Completed in Batch 210.
- [x] Feed schema-contract failures into the shared ActionGraph reducer as
      blocked nodes instead of REPL prose guard errors.
      Completed in Batch 211.

### Batch 210: Shared Field-Contract Repair Builder

After Batch 209, missing-field checks used shared schema projection, but the
repair surface still lived in REPL helpers: candidate artifact ranking and
allowed-action repair hints. That is still too close to the UI layer for a
future typed reducer.

This batch moves the domain-neutral pieces into `internal/dataworkflow`:

- rank candidate artifacts by whether they contain all missing fields, then by
  match count and alias;
- format compact candidate labels for existing prompt/UI compatibility;
- derive repair hints only from typed allowed action kinds.

The builder does not inspect business names, user prose, or model prose. It
only compares declared artifact fields and declared allowed action kinds.

Changes:

- [x] Added `dataworkflow.ArtifactFieldCandidate`.
- [x] Added `FieldContractCandidateArtifacts` and
      `FieldContractCandidateLabels`.
- [x] Added `FieldContractRepairHints`.
- [x] Rewired REPL field-contract violations to consume the shared builder.
- [x] Added regression coverage for candidate ranking and allowed-action hints.

Remaining architecture items:

- [ ] Emit full `WorkflowViolation` objects from this builder at the guard
      boundary, instead of formatting prose first and parsing it later.
- [x] Feed schema-contract failures into the shared ActionGraph reducer as
      blocked nodes.
      Completed in Batch 211.

### Batch 211: WorkflowViolation Blocked Action Nodes

Typed violations were available as `workflow_state_json.workflow_violations`,
but ActionGraph still showed only executed/failed/ready nodes. That means a
schema-contract failure could be structurally visible in one part of state but
not in the graph state the scheduler should eventually own.

This batch projects typed violations into blocked ActionGraph nodes:

- `WorkflowViolation` records become `ActionNode{status=blocked}`;
- input aliases, output alias, action id/kind, dependency rank, and ledger
  capability are preserved when known;
- blocked nodes get stable structural keys derived from violation code,
  action identity, input aliases, output alias, and missing fields when the
  original action idempotency key is unavailable.

This remains typed structural state. It does not parse model prose for hard
gates, and it does not infer business semantics from field or artifact names.

Changes:

- [x] Added `dataworkflow.BlockedActionNodesFromViolations`.
- [x] Wired workflow state to populate `action_graph.blocked` from typed
      workflow violations.
- [x] Added regression coverage for blocked-node projection and REPL workflow
      state exposure.

Remaining architecture items:

- [ ] Emit `WorkflowViolation` directly from guard functions before prose
      formatting.
- [ ] Teach the shared reducer to own blocked-node lifecycle and unblock
      transitions after successful repair actions.

### Batch 212: ArtifactGraph Audit Snapshots

Artifact node classes and schema projection are now used by prompts and
field-contract guards, but result audit still required inspecting the full
runner result and reconstructing the graph mentally. That is not enough for
commercial diagnostics when a workflow reuses many generated artifacts.

This batch writes an artifact graph snapshot next to each result audit file:

- projected artifact id/kind/node_class;
- executable aliases and access hints;
- exact fields and row counts when available;
- source paths for lineage.

The snapshot is audit-only and domain-neutral. It records structural artifact
shape; it does not classify business roles or interpret user data.

Changes:

- [x] Added `.artifacts.json` audit files for data results with artifacts.
- [x] Logged full ArtifactGraph projection in audit logs.
- [x] Added compact REPL preview links to the full artifact graph artifact.
- [x] Added regression coverage for node class, fields, aliases, and lineage
      persistence.

Remaining architecture items:

- [x] Persist cumulative ArtifactGraph snapshots across workflow-terminal
      summaries, not only per-result snapshots.
      Completed in Batch 214.
- [ ] Move scaffold eligibility fully into an ArtifactGraph-aware reducer.

### Batch 213: ArtifactGraph Record-Action Eligibility

The first scaffold-safety fixes taught REPL helpers to ignore diagnostic child
artifacts and workflow ledger handles. Batch 204 moved node classes into
ArtifactGraph, but the record-action eligibility predicate still lived in
REPL. That kept a hard structural rule in the UI/runtime layer.

This batch moves the record-action eligibility predicate into
`internal/dataworkflow`:

- an artifact must have a known field contract;
- diagnostic children and workflow ledger handles are not ordinary record
  action bases;
- rule/material/terminal artifacts are not ordinary record action bases;
- record-shaped artifacts and record-producing action outputs are eligible.

The predicate is based only on node class, JSON shape, action kind, and fields.
It does not classify business roles or inspect user/model prose.

Changes:

- [x] Added `dataworkflow.ArtifactUsableForRecordAction`.
- [x] Rewired REPL record-action scaffold filtering to use the shared
      ArtifactGraph helper.
- [x] Reused one REPL adapter from `dataTaskArtifactAccessPrompt` to
      `ArtifactSchemaProjection`.
- [x] Added regression coverage for record, diagnostic child, workflow ledger,
      and rule-artifact eligibility.

Remaining architecture items:

- [ ] Move relation-specific scaffold construction
      (`normalize_entities`, `apply_entity_resolutions`, `enrich_records`,
      `join_records`) into ArtifactGraph-aware typed builders.
- [ ] Emit scaffold skipped/eligible diagnostics into audit snapshots without
      increasing user-facing noise.

### Batch 214: Terminal Graph Audit Snapshots

Per-plan and per-result audit files are useful, but terminal support still had
to reconstruct the final workflow state by walking many batch files. A data
workflow should leave one compact terminal graph snapshot whether it completed,
blocked, exhausted budget, or failed.

This batch writes a terminal audit artifact containing:

- terminal status, reason, rounds, record count, result summary, and last error;
- cumulative ActionGraph, including blocked nodes from typed violations;
- cumulative ArtifactGraph schema projection and lineage;
- workflow violations.

The snapshot is compact and structural. It avoids dumping full records or large
materials, and it does not add business-specific interpretation.

Changes:

- [x] Added terminal `.json` audit files for data workflows.
- [x] Persisted terminal ActionGraph, ArtifactGraph, and WorkflowViolations.
- [x] Logged terminal audit path and full terminal snapshot.
- [x] Added regression coverage for terminal graph snapshot persistence.

Remaining architecture items:

- [x] Persist reducer event streams, not only projected terminal snapshots.
      Completed in Batch 215.
- [x] Add low-noise CLI/REPL links to terminal audit snapshots when useful,
      without polluting strict stdout output.
      Completed for REPL process output in Batch 216; non-REPL CLI stderr link
      remains a separate item.

### Batch 215: Terminal Action Event Streams

Batch 214 persisted projected terminal graphs. Projection is enough for most
debugging, but replay and reducer migration need the event stream as well:
which action batches entered the reducer, and with what executed/failed status.

This batch persists the typed `ActionEvent` stream in terminal audit snapshots:

- REPL records are adapted once into `dataworkflow.ActionEvent`;
- ActionGraph projection and terminal audit consume the same adapter;
- terminal snapshots now carry both `action_events` and projected
  `action_graph`.

This keeps replay data structural and compact: no full materials, no model
prose, and no business-specific interpretation.

Changes:

- [x] Added shared REPL adapter `dataTaskWorkflowActionEvents`.
- [x] Reused the adapter for ActionGraph reduction.
- [x] Persisted `action_events` in terminal data audit snapshots.
- [x] Added regression coverage for terminal action event persistence.

Remaining architecture items:

- [ ] Move action event persistence into a storage-neutral workflow journal so
      CLI and future non-REPL runners do not depend on REPL terminal audit.
- [x] Add terminal audit links to REPL/CLI process output with strict-output
      safety.
      Completed for REPL process output in Batch 216; CLI strict stdout remains
      protected because this path only uses renderer-side process summaries.

### Batch 216: Low-Noise Terminal Audit Link

Terminal graph snapshots are only useful if operators can find them. This batch
adds a low-noise REPL process summary that points to the terminal data-audit
file after a data workflow reaches a terminal state.

The link is emitted through the renderer process lane, not through final answer
text. That keeps strict data output clean and avoids changing CLI stdout
semantics.

Changes:

- [x] Added `emitDataTaskTerminalAuditPath`.
- [x] Wired terminal data workflow logging to emit a compact audit-link summary
      when a renderer is available.
- [x] Added regression coverage for the low-noise terminal audit path summary.

Remaining architecture items:

- [ ] Add an equivalent stderr-only terminal audit path for non-REPL CLI data
      runs that preserves stdout-only final answers.

### Batch 217: Deferred Action Audit Status

Deferred action queues were already saved as plan audit artifacts, but their
ActionGraph audit snapshot reused the default `ready` status. That made audit
state ambiguous: a queued future rank looked like the batch currently being
executed.

This batch marks `scope=deferred` action snapshots as
`ActionNode{status=deferred}`. It does not change dispatch behavior; it makes
the persisted graph state match the runtime queue state.

Changes:

- [x] Action audit snapshots now choose node status from audit scope.
- [x] Deferred queue audits persist deferred action nodes with
      `status=deferred`.
- [x] Added regression coverage for deferred action audit status.

Remaining architecture items:

- [ ] Surface deferred queued nodes in live `workflow_state_json.action_graph`
      once deferred queues move from REPL-local variables into shared workflow
      state.

### Batch 218: Workflow IR Closure Plan

The current data lane has moved a long way from one-shot scripts toward typed
actions, but several contracts are still split between REPL/CLI orchestration
and `internal/dataworkflow`. That split is now the main convergence risk:
runtime guards can reject unsafe graph moves, but too many of them still return
prose first and only later get projected into typed state. Deferred plans are
also still owned by REPL/CLI local variables, so live `workflow_state_json`
does not fully describe the scheduler queue that the system is about to run.

This batch defines the remaining architecture closure items before another
real-scenario run. The goal is not to add business-specific behavior. It is to
make the hard workflow model structural:

- guards produce `WorkflowViolation` directly at the boundary where structural
  facts are known;
- the ActionGraph reducer owns ready/deferred/blocked lifecycle and exposes
  stable idempotency keys, ranks, inputs, and reasons;
- deferred queues become part of live workflow state, not only audit files;
- scaffold builders consume ArtifactGraph projections, not REPL prompt structs;
- terminal workflow evidence is written through a storage-neutral journal that
  REPL and CLI can both render;
- user-facing process events prefer model-authored goal/batch/next-step/reason
  fields when present, while internal counters remain low-noise audit detail;
- complex eval gates validate graph convergence, ledger/reconcile completion,
  and strict-output cleanliness across realistic multi-file workflows.

Task list:

- [ ] Add shared violation builders for field/schema, zero-match filter,
      unmatched-resolution, and zero-eligible states. The builders must not
      parse model prose; they consume typed action/artifact/ledger facts.
- [ ] Update guard entrypoints to return typed violations where the violation
      is structural, keeping prose only as a rendered explanation.
- [ ] Extend the shared ActionGraph reducer to accept executed events, ready
      actions, deferred actions, and blocked violations in one call.
- [ ] Thread deferred queue nodes into live `workflow_state_json.action_graph`
      for both REPL and non-REPL CLI.
- [ ] Move relation-specific scaffold candidate construction into
      ArtifactGraph-aware builders under `internal/dataworkflow`, leaving REPL
      as an adapter/renderer.
- [ ] Add a storage-neutral workflow journal type for terminal graph snapshots,
      event streams, violations, artifact projections, and audit paths.
- [ ] Emit CLI terminal audit links through stderr/progress only; stdout must
      remain the final answer for strict-output and pipeline use.
- [ ] Refresh data process UX so permanent lines show compact business-facing
      goal, batch purpose, next step, and reason when structured fields exist.
- [ ] Add realistic eval gates after the architecture items above are complete;
      do not use another real-scenario run as a substitute for closing known
      IR gaps.

Rationale: this is a generic workflow-runtime closure, not a procurement-data
patch. Any data task with intermediate artifacts can benefit: spreadsheets,
JSONL statistics, text/OCR extraction, web-table cleanup, multi-file joins,
strict projection, or mixed data-plus-code questions. The system remains
responsible for graph state, dependencies, schema, ledgers, and auditability;
the model remains responsible for task semantics and typed action choices.

### Batch 219: Typed Violations, Deferred Graph State, And CLI Journal Link

This batch begins closing Batch 218 by moving more hard workflow state into
shared typed contracts:

- field-contract violations now use a shared `dataworkflow`
  `WorkflowViolation` builder when an action is known;
- the builder fills action kind, output alias, normalized input aliases,
  dependency rank, idempotency key, candidate artifact labels, and repair
  hints from typed action/artifact facts;
- the ActionGraph reducer now accepts executed events, ready actions, deferred
  actions, and blocked violations in one structural input;
- `dataTaskWorkflowStateWithDeferred` can expose queued deferred nodes in live
  `workflow_state_json.action_graph.deferred`;
- the real LLM planner implements optional continuation/evaluator interfaces
  that include deferred queue state, while older planner implementations keep
  the existing interface;
- terminal data audit writing was extracted into a storage-neutral package
  helper shared by REPL and CLI;
- non-REPL CLI data runs now write the same terminal graph snapshot on exit and
  print only a low-noise audit path to the progress writer/stderr, preserving
  stdout for final answers.

Changes:

- [x] Added `NewFieldContractViolation` and generic action-input violation
      builder in `internal/dataworkflow`.
- [x] Added `ReduceActionGraphState` with ready/deferred/blocked lifecycle in
      the shared reducer.
- [x] Routed REPL workflow blocked-node projection through the shared reducer.
- [x] Added `dataTaskWorkflowStateWithDeferred` and deferred-aware prompt
      helpers.
- [x] Added optional deferred-aware planner/evaluator interfaces and wired
      REPL/CLI data paths to use them when available.
- [x] Extracted terminal audit writing from REPL methods into a shared helper.
- [x] Added CLI stderr/progress terminal audit-link emission.
- [x] Added regression coverage for deferred graph projection, typed violation
      projection, and prompt-level deferred node visibility.

Remaining architecture items:

- [ ] Extend typed builders to zero-match filter, unmatched-resolution, and
      zero-eligible qualification guard entrypoints so those guards also stop
      relying on prose-first errors.
- [ ] Move relation-specific scaffold construction
      (`normalize_entities`, `apply_entity_resolutions`, `enrich_records`,
      `join_records`) into ArtifactGraph-aware builders under
      `internal/dataworkflow`.
- [ ] Persist a storage-neutral journal object throughout execution, not only
      at terminal snapshot time.
- [ ] Improve business-facing process summaries by rendering model-provided
      goal/batch/next-step/reason fields before internal counters.
- [ ] Add realistic multi-file eval gates after these IR closures are complete.

### Batch 220: ArtifactGraph-Aware Relation Scaffolds

Relation scaffolds were still mostly built from REPL prompt structs. This batch
moves the relation-candidate construction into `internal/dataworkflow` so the
templates come from ArtifactGraph schema projections:

- record bases are selected by `ArtifactUsableForRecordAction`;
- diagnostic children are excluded from relation inputs;
- lookup/reference/enrichment inputs can be ordinary record artifacts or
  mapping/ledger-shaped artifacts with fields;
- join candidates use shared field intersections from artifact schemas;
- enrich candidates preserve base row cardinality and expose typed
  `lookup_specs`;
- normalize/apply-resolution candidates are structural templates over source,
  reference, and mapping artifacts, not business-domain classifiers.

The builder remains advisory. It does not decide that any relation is business
correct; it only shows structurally plausible typed action shapes that the
model may adapt to the current user goal.

Changes:

- [x] Added `dataworkflow.ActionScaffold` and ArtifactGraph-aware relation
      scaffold builders.
- [x] Rewired REPL relation scaffold rendering to adapt shared scaffolds into
      prompt views.
- [x] Kept relation candidates domain-neutral: only fields, aliases, node
      class, action kind, and allowed actions are used.
- [x] Added regression coverage for relation scaffolds excluding diagnostic
      artifacts while producing enrich/join candidates.
- [x] Preserved existing REPL scaffold tests for enrich/reference-table
      suggestions and rule-artifact exclusion.

Remaining architecture items:

- [ ] Remove or demote old REPL-local relation scaffold helper implementations
      once no fallback/test path depends on them.
- [ ] Add scaffold eligibility/skip diagnostics to audit snapshots without
      increasing user-facing noise.

### Batch 221: Typed Non-Field Guard Violations

Field-contract failures were the first guard class to use shared
`WorkflowViolation` builders. This batch applies the same structural path to
non-field data workflow issues that already have typed issue records:

- zero-match filter artifacts;
- all-unmatched resolution application artifacts;
- zero-eligible qualification artifacts.

The change does not parse model prose. It consumes existing typed issue
objects and constructs action-input violations with normalized action kind,
input aliases, output alias, dependency rank, idempotency key, repairability,
and repair hints. Blocked ActionGraph nodes now come from the same reducer path
for these issue classes.

Changes:

- [x] Replaced local ad hoc non-field `WorkflowViolation` assembly with
      `dataworkflow.NewActionInputViolation`.
- [x] Preserved typed issue details in `workflow_state_json` for planner
      guidance.
- [x] Added regression coverage that zero-match and unmatched-resolution issues
      produce typed workflow violations and blocked graph nodes with stable
      idempotency keys.

Remaining architecture items:

- [ ] Convert guard entrypoint return types from prose strings to typed
      `WorkflowViolation` values, with prose rendered only at the UI/planner
      boundary.
- [ ] Persist violation objects in the workflow journal as first-class events.

### Batch 222: Business-Facing Data Process Details

The data process UI still showed too much internal workflow vocabulary in
permanent lines. This batch keeps deterministic lane/status counters in the
title line, but surfaces model-authored task intent from structured plan fields
inside the low-noise detail block:

- goal;
- current batch purpose;
- next step;
- compact action summary.

Result counters remain in the title line for scanning. Business-facing details
are rendered below the title, so long task intent does not make permanent lines
wide and noisy. CLI uses the same renderer path through stderr/progress; stdout
remains reserved for final answers.

Changes:

- [x] Added current-plan details to REPL data workflow execute/result/evaluate
      events.
- [x] Added the same current-plan details to CLI data workflow progress.
- [x] Kept business-facing details out of title-line inline segments.
- [x] Added regression coverage for deterministic title ordering plus
      business detail rendering below the summary line.

Remaining architecture items:

- [ ] Thread structured evaluation reasons and continuation reasons as typed
      process-event fields instead of only free-form detail strings.
- [ ] Add audit metrics that distinguish user-facing process summaries from
      internal graph counters.

### Batch 223: Storage-Neutral Workflow Journal Type

Terminal graph snapshots were shared by REPL and CLI after Batch 219, but the
JSON type itself still lived in `internal/repl`. This batch moves that terminal
snapshot contract into `internal/dataworkflow` as `WorkflowJournal`.

The journal type is storage-neutral: it does not know whether it will be
rendered by REPL, written by CLI, or consumed by a future non-terminal runner.
It carries typed action events, projected ActionGraph, ArtifactGraph,
WorkflowViolations, terminal status, result summary, and optional process
events. The existing terminal audit file format is preserved.

Changes:

- [x] Added `dataworkflow.WorkflowJournal` and `WorkflowJournalEvent`.
- [x] Rewired terminal data audit writer to marshal the shared journal type.
- [x] Kept `dataTaskTerminalAuditSnapshot` as a type alias so existing tests and
      callers do not fork a second schema.
- [x] Added JSON contract coverage for journal field names and deferred action
      graph content.

Remaining architecture items:

- [ ] Append process events into the journal throughout execution, not only at
      terminal snapshot construction.
- [ ] Persist journal checkpoints before terminal exit so interrupted sessions
      can resume typed graph state.

### Batch 224: Typed Field-Contract Guard Entry

Earlier batches projected field-contract failures into typed workflow state
after the guard had already returned prose. This batch starts the guard-entry
conversion: the missing-field guard now produces a typed `WorkflowViolation`
first and renders the legacy prose message from that object for existing
callers.

Changes:

- [x] Added `dataTaskActionMissingFieldContractViolation` as a typed guard
      helper.
- [x] Rendered the legacy field-contract message from the typed violation.
- [x] Added regression coverage that the guard helper returns code,
      dependency rank, and idempotency key.

Remaining architecture items:

- [ ] Convert zero-match, unmatched-resolution, zero-eligible, dependency, and
      stage guards to the same typed-return shape.
- [ ] Change top-level staging/terminal guard APIs to return typed guard
      results, with prose generated only at UI/planner boundaries.

### Batch 225: Realistic Multi-File Data Eval Gate

The existing data-lane eval set covered scalar table summation, JSON/JSONL/text
filters, and a small join/reconcile case. That is useful but not enough to
guard the IR transition. This batch adds a larger domain-neutral fixture that
requires the workflow to combine multiple independent capabilities:

- material instructions plus three structured record sets;
- reference/mapping application without hard-coded business roles;
- record filtering;
- contribution ledger and reconciliation;
- reference-complete final projection with explicit zero output;
- strict comma-separated final output;
- CLI terminal audit link emission.

The fixture deliberately uses generic observation/label/target terminology so
it validates graph mechanics rather than a specific customer business domain.

Changes:

- [x] Added `data_multifile_reference_projection` eval case.
- [x] Added the `data-multifile-reference` fixture with instructions, source
      records, label mapping, and target reference rows.
- [x] Added hidden log assertions for data routing, non-empty contribution
      ledger, passing reconciliation, and terminal journal audit path.

Remaining architecture items:

- [ ] Add a deterministic preflight for all data eval fixtures that validates
      fixture arithmetic independently of the model runner.
- [ ] Add multi-run volatility gates for the complex data cases once provider
      capacity is stable enough for repeated CI sampling.

### Batch 226: Typed Guard Result And Journal Process Events

The workflow journal had a shared terminal snapshot, but guard APIs still
mostly returned prose strings and terminal snapshots did not include the
process events that led to the final state. This batch adds the shared carrier
needed for incremental migration without changing runtime decisions.

Changes:

- [x] Added `dataworkflow.GuardResult` as a storage-neutral typed guard result
      with code, severity, repairability, message, reason, and workflow
      violations.
- [x] Converted the terminal unfinished-workflow guard to produce a typed
      `GuardResult` first, while preserving the existing string wrapper for
      current REPL/CLI callers.
- [x] Added per-record process events to terminal workflow journals so the
      audit artifact records completed/failed action or script batches, status,
      round order, and compact reason.
- [x] Added regression coverage for guard-result behavior, typed terminal
      guard output, and terminal journal process-event emission.

Remaining architecture items:

- [ ] Convert the main staging guard and action-level guard entrypoints to
      return `GuardResult` directly.
- [ ] Persist workflow journal checkpoints during execution, not only at
      terminal exit.

### Batch 227: Typed Staging Guard Entrypoint

The terminal guard now emits a typed result, but staging still exposed only
prose. This batch adds typed staging entrypoints while keeping the existing
string functions as compatibility wrappers for the current REPL/CLI loops.
The hard decision still comes from deterministic guard conditions; no logic
parses the rendered message.

Changes:

- [x] Added `dataTaskPlanStagingGuardResult`.
- [x] Added `dataTaskWorkflowStagingGuardResult`.
- [x] Kept `dataTaskPlanStagingGuardError` and
      `dataTaskWorkflowStagingGuardError` as thin string renderers.
- [x] Added regression coverage that a ready plan without an executable body
      produces a typed `missing_executable_body` guard result.

Remaining architecture items:

- [ ] Convert large action-level guard internals to emit typed violation/result
      objects directly instead of wrapping rendered messages.
- [ ] Attach staging guard results to workflow journal process events before
      repair/continuation planning.

### Batch 228: Workflow Journal Checkpoints

Terminal journals are useful after a run ends, but long data workflows need
mid-run auditability too. This batch writes checkpoint journals after
successful data batches using the same storage-neutral `WorkflowJournal`
schema as terminal snapshots.

Changes:

- [x] Added `writeDataTaskWorkflowCheckpointFile`.
- [x] CLI data workflows write a checkpoint after each successful data batch.
- [x] REPL data workflows write the same checkpoint after each successful data
      batch.
- [x] Checkpoints include action events, action graph, artifact graph,
      workflow violations, result summary, and process events.
- [x] Added regression coverage that checkpoint files use the shared journal
      schema.

Remaining architecture items:

- [x] Add checkpoint writes for main staging/terminal guard records before the
      next planner call.
- [ ] Add resume-from-checkpoint support as a separate opt-in feature; do not
      silently resume interrupted data tasks yet.
- [ ] Add checkpoint writes for deeper action-level guard records once those
      internals emit typed guard payloads directly.

### Batch 229: Guard Checkpoints Before Repair Planning

Successful-batch checkpoints are not enough for repair audits. When the system
blocks a candidate plan and asks the model to repair or continue, the audit
should record the typed guard that caused the turn. This batch wires checkpoint
journals into the main staging and terminal workflow guards before any repair
planner call.

Changes:

- [x] `WorkflowJournalEvent` can now carry a typed `GuardResult`.
- [x] Checkpoint journals merge guard violations into
      `workflow_violations`.
- [x] CLI writes a guard checkpoint when terminal workflow guard or staging
      guard blocks a plan.
- [x] REPL writes the same guard checkpoint for those guard paths.
- [x] Added regression coverage that checkpoint process events include the
      guard code and payload.

Remaining architecture items:

- [ ] Convert action-level guard internals to produce `GuardResult` directly,
      then checkpoint those deeper guard events with the same journal path.
- [ ] Feed guard checkpoint paths into low-noise CLI/REPL process summaries only
      when verbose/audit output is enabled; keep stdout final-answer clean.

### Batch 230: Typed Action Dependency Guard Entrypoint

Action-level dependency checks were another prose-only boundary. This batch
converts the action dependency guard entrypoint to return `GuardResult` while
preserving the existing string wrapper for current callers. The guard still
uses deterministic action kind, input contract, upstream ledger, and action
spec checks; it does not parse rendered messages.

Changes:

- [x] Added `dataTaskActionDependencyGuardResult`.
- [x] Kept `dataTaskActionDependencyGuardError` as a thin renderer.
- [x] Assigned stable typed codes for common structural classes:
      intra-batch dependency, unavailable input, field-contract guard,
      missing/too-many inputs, missing action spec, and missing upstream
      ledger.
- [x] Added regression coverage for a missing action-spec guard result.

Remaining architecture items:

- [x] Convert the field-contract guard entrypoint and direct missing-field
      helpers to return typed violations/results.
- [x] Convert relation-specific field sub-guards, zero-match,
      unmatched-resolution, and zero-eligible helpers to return typed
      violations/results directly.
- [x] Feed action dependency `GuardResult` objects into checkpoints from
      staging loops once action guard results are propagated through the parent
      staging guard result.

### Batch 231: Typed Field-Contract Guard Result

The missing-field helper already produced `WorkflowViolation`, but the
action-level field-contract guard still returned only prose. This batch moves
the field-contract entrypoint to `GuardResult` and preserves the existing
string wrapper for current callers.

Changes:

- [x] Added `dataTaskActionFieldContractGuardResult`.
- [x] Added `dataTaskActionMissingFieldContractGuardResult`.
- [x] Direct missing-field checks now return a `GuardResult` carrying the
      typed `WorkflowViolation` payload.
- [x] Zero-match, unmatched-resolution, and zero-eligible guards now have stable
      typed guard codes at the field-contract boundary.
- [x] Added regression coverage that missing-field guard results include the
      typed violation payload.

Remaining architecture items:

- [x] Convert relation-specific field sub-guards (`join_records`,
      `normalize_entities`, `enrich_records`, `apply_entity_resolutions`) to
      produce typed field-contract violations instead of wrapped messages.
- [x] Propagate child action guard results through staging guard results so
      checkpoint guard payloads include the deepest typed violation.

### Batch 232: Propagated Typed Action Guards

The action dependency and field-contract entrypoints now emit typed
`GuardResult` values, but parent staging guards could still wrap those messages
under broad `action_staging_guard` / `workflow_action_staging_guard` codes.
That lost useful typed payloads before checkpoint journaling and repair
planning.

This batch keeps the existing deterministic guard ordering, but when a parent
staging guard delegates to an action-level guard, it now returns the matching
child `GuardResult`. The matching is only used to preserve a guard object that
was already produced by deterministic action validation; no business decision
or hard gate is driven by parsed prose.

Changes:

- [x] Added typed `dataTaskActionStagingGuardResult`.
- [x] Added typed `dataTaskWorkflowActionStagingGuardResult`.
- [x] Parent staging guards now preserve child action guard codes and violation
      payloads when the child guard caused the block.
- [x] Added regression coverage that a missing `derive_fields` specification
      surfaces as `missing_action_spec` at the workflow staging boundary.

Remaining architecture items:

- [ ] Convert relation-specific field sub-guards (`join_records`,
      `normalize_entities`, `enrich_records`, `apply_entity_resolutions`) to
      produce typed field-contract violations instead of wrapped messages.
- [x] Persist deeper action guard payloads in per-action process summaries once
      the UX event layer consumes the propagated guard results.

### Batch 233: Typed Zero-Progress Guard Payloads

The workflow state already projected zero-match filters, all-unmatched
resolution outputs, and zero-eligible qualification outputs as typed graph
violations. The staging guard path, however, still turned those conditions into
plain text before repair and checkpoint journaling.

This batch makes those zero-progress blockers return `GuardResult` with an
embedded `WorkflowViolation` that points to the consuming action and blocked
input alias. The rendered message remains user-readable, but repair/evaluator
logic can now consume typed action id, action kind, input alias, idempotency
key, reason, and repair hints. The mechanism is domain-neutral: it applies to
any data task whose prior typed action produced an empty/blocked intermediate
artifact while downstream contribution, reconciliation, or final projection is
still required.

Changes:

- [x] Added typed guard results for zero-match filter artifacts.
- [x] Added typed guard results for all-unmatched resolution artifacts.
- [x] Added typed guard results for zero-eligible qualification artifacts.
- [x] Parent field-contract guard now returns those typed payloads directly
      instead of wrapping their rendered messages.
- [x] Action-dependency staging now propagates field-contract child guard
      results instead of re-wrapping them under a broad guard code.
- [x] Added regression coverage that workflow staging exposes guard code and
      violation payload for all three zero-progress classes.

Remaining architecture items:

- [x] Convert relation-specific field sub-guards (`join_records`,
      `normalize_entities`, `enrich_records`, `apply_entity_resolutions`) to
      produce typed field-contract violations instead of wrapped messages.
- [ ] Feed propagated guard payloads into business-facing process summaries
      without leaking internal graph jargon into normal user output.

### Batch 234: Typed Relation Field Sub-Guards

Relation-style data actions use fields across one or more record artifacts:
joins, reference enrichment, entity normalization, and applying entity
resolutions. Before this batch, the generic field-contract guard could emit a
typed violation for simple single-input actions, but relation-specific helper
guards still returned plain strings and lost the missing-field payload.

This batch converts those relation-specific field-contract helpers to return
`GuardResult` directly. Missing fields now preserve the same
`WorkflowViolation` payload as simpler actions: action identity, input alias,
missing fields, available field sample, candidate artifacts, repair hints, and
idempotency key. Non-field structural relation errors keep stable typed guard
codes, but they do not invent business semantics.

Changes:

- [x] Added typed field-contract guard results for `join_records`.
- [x] Added typed field-contract guard results for `normalize_entities`.
- [x] Added typed field-contract guard results for `enrich_records`.
- [x] Added typed field-contract guard results for
      `apply_entity_resolutions`.
- [x] Kept existing string wrappers as render-only compatibility paths.
- [x] Added regression coverage that all four relation actions expose typed
      missing-field payloads through workflow staging.

Remaining architecture items:

- [ ] Convert remaining non-field relation contract errors into typed
      violations where they have precise action/input/artifact handles.
- [x] Feed propagated guard payloads into business-facing process summaries
      without leaking internal graph jargon into normal user output.

### Batch 235: Business-First Workflow Progress Details

Data workflow progress lines had become useful for system audit but too
internal for customers: title lines could show ledger/material counts while
model-authored goal, batch purpose, next step, and action summary lived below
or were obscured by generic system prose.

This batch keeps the low-noise permanent title line deterministic, but moves
business-facing details ahead of generic system descriptions. Audit counters
remain available below the title as audit detail instead of being promoted into
the permanent line. Failure details are labeled as reasons. The same rendering
helpers are used by REPL and CLI progress, while stdout remains reserved for
the final answer in CLI mode.

Changes:

- [x] Stopped promoting data workflow audit counters into the permanent title
      line.
- [x] Rendered model-authored `goal`, `why_this_batch`, `next_batch`, and
      action summaries before generic workflow prose.
- [x] Labeled failure details as reasons and internal counters as audit
      details.
- [x] Added regression coverage for title-line stability and business-first
      detail rendering.

Remaining architecture items:

- [ ] Convert remaining non-field relation contract errors into typed
      violations where they have precise action/input/artifact handles.
- [x] Add richer structured process-event payloads to the workflow journal so
      external UIs can render the same business-first view without parsing
      terminal text.

### Batch 236: Structured Journal Process Details

Business-first terminal progress is useful, but external UIs and audit tools
should not have to parse terminal strings to recover the same information.
This batch extends storage-neutral workflow journal events with structured
business and audit fields while keeping existing event kind/status/reason
fields intact.

Changes:

- [x] Added `goal`, `batch_purpose`, `next_step`, `action_summary`, and
      `audit_details` fields to `WorkflowJournalEvent`.
- [x] Batch events now persist model-authored goal, batch purpose, next step,
      action summary, and result audit detail.
- [x] Guard checkpoint events now persist the current plan's business context
      and typed guard code.
- [x] Added JSON contract and checkpoint coverage for the new process-event
      payload fields.

Remaining architecture items:

- [x] Convert remaining non-field relation contract errors into typed
      violations where they have precise action/input/artifact handles.
- [ ] Add opt-in resume support that can consume journal checkpoints without
      silently resuming interrupted user work.

### Batch 237: Typed Relation Non-Field Contract Guards

Some relation failures are not missing fields, but they still have precise
action/input handles: conflicting base paths, missing reference path/key for
existing-id verification, incompatible resolution lineage, missing canonical
value fields, and repeated apply-resolution graph edges. These conditions used
stable guard codes but did not always carry a `WorkflowViolation`.

This batch adds a shared action-input guard builder and applies it only where
the system has an objective action/input/artifact handle. Ambiguous role
inference failures remain rendered guard messages unless they can be grounded
to a specific handle without guessing.

Changes:

- [x] Added a shared typed action-input contract guard builder.
- [x] Converted precise apply-resolution non-field contract errors to
      `GuardResult` values with embedded `WorkflowViolation` payloads.
- [x] Converted repeated apply-resolution edges to typed no-progress
      violations.
- [x] Kept string wrappers and rendered messages compatible for existing
      planner repair prompts.
- [x] Added regression coverage for typed repeated apply-resolution edge
      guards.

Remaining architecture items:

- [ ] Add opt-in resume support that can consume journal checkpoints without
      silently resuming interrupted user work.
- [ ] Add targeted tests for each non-field relation guard code as they become
      user-visible in evaluator policy.

### Batch 238: Current Closure Audit And Resume Boundary

The latest review asked to audit all unfinished items before running another
real scenario. The document contains many historical `[ ]` items because it is
both an implementation ledger and an architecture backlog. The closure rule is:
do not mark a broad item complete unless the exact invariant has code, runtime
integration, and regression coverage. For the next real uninterrupted data run,
the current blocker list is narrower than the full release backlog.

Closed P0/P1 blockers for the next real run:

- [x] ActionDAG readiness, rank splitting, deferred queue saving, deferred
      readiness checks, and full deferred staging guards are delivered for the
      live CLI/REPL data workflow.
- [x] Deferred queue state is projected into live `workflow_state_json.action_graph`
      as typed ready/deferred/blocked nodes, not only retained as planner prose.
- [x] Guard paths now emit typed `WorkflowViolation` payloads where the system has
      precise action/input/artifact handles. Generic guard conversion covers the
      remaining non-action prose guards without inventing business semantics.
- [x] Field-contract, zero-progress, zero-match, unmatched-resolution,
      zero-eligible, and relation-role blockers reach workflow state as typed
      signals before later batches can consume invalid artifacts.
- [x] Artifact visibility now separates prompt compaction from hard-gate contract
      fields. Hard gates consume the full newest-first contract view instead of
      lossy prompt samples.
- [x] Material floors, current-batch inputs, optional discovered evidence, and
      generated artifacts are separated enough for current execution; auxiliary
      model-discovered material does not automatically become a workflow hard
      floor.
- [x] Ledger state is de-duplicated for rules, row decisions, entity resolutions,
      and contributions before seeding later actions.
- [x] Relation-specific scaffold/build paths consume shared action capability,
      rank, input-contract, and artifact-role helpers where implemented, rather
      than keeping the critical contracts only in prompt prose.
- [x] Workflow checkpoints and terminal audits persist storage-neutral journal
      snapshots with action graph, artifact graph, typed violations, process
      events, and an explicit resume payload.
- [x] CLI has an explicit opt-in `--data-resume <checkpoint.json>` path. It never
      silently resumes user work, skips the initial planner call, loads the
      checkpoint resume payload, and falls back to typed checkpoint state if the
      continuation planner returns an empty plan shape.
- [x] CLI data progress remains on stderr, final answers remain on stdout, and
      terminal audit paths are printed for scripted/eval inspection.
- [x] REPL/CLI process events prioritize model-authored goal, batch purpose, next
      step, action purpose, and failure reason before internal counters when those
      typed fields are available.
- [x] A real-scenario opt-in gate script exists for complex data tasks. It checks
      non-empty output, terminal audit emission, contribution evidence, reconcile
      status, and optional expected final answer without hard-coding a business
      domain.

Still open, but not blockers for the next single real uninterrupted run:

- [ ] Move the entire reducer and `ValidatedPlanEnvelope` into
      `internal/dataworkflow`. Current CLI/REPL paths already share the same
      reducer helpers and preflight path, so this is a maintainability/reuse
      boundary rather than an immediate correctness blocker.
- [ ] Persist full resumable ActionDAG edge state for interrupted sessions. The
      current explicit resume payload restores records/current/deferred plans and
      is safe for CLI opt-in recovery; richer edge replay is release-hardening.
- [x] Add a domain-neutral value-distribution action. It reduces repair turns
      by letting the workflow inspect actual field values before filters,
      grouping, joins, or contribution params.
- [ ] Add a domain-neutral mapping-candidate action. Current typed relation
      actions can proceed without it, but candidate generation would improve
      convergence for ambiguous reference/entity matching.
- [x] Promote every evaluator reason into a durable `WorkflowDecision` object.
      Completed in Batch 262 for live `workflow_state_json` and
      terminal/checkpoint journals.
- [ ] Multi-run volatility gates and CI status checks. They are required before
      claiming default-on commercial release stability, but provider budget and
      repeated-run variance should be measured after the single-run path is
      stable.
- [ ] Historical checklist de-duplication. Old `[ ]` items should be grouped by
      architecture theme in a docs-cleanup pass, not mechanically flipped to
      `[x]` in this implementation batch.

Decision: run unit gates and build first. If they pass, proceed to the requested
real scenario with the latest binary. If the real scenario still fails, the next
fix must again be a typed, domain-neutral IR/runtime improvement, not a
business-specific patch.

### Batch 239: Relation Scaffold Internal-Lineage Guard

The next real-scenario gate was intentionally interrupted after the workflow
showed a deterministic convergence loop. The model was not the only source of
the loop: after material coverage and entity-resolution work, the runtime
fallback repeatedly selected a concrete `join_records` scaffold during
`prepare_contribution_inputs`. Each batch joined previously joined artifacts on
runtime lineage columns such as `_source` or `_left_index`, emitted another
record artifact, and kept the stage looking productive even though contribution
and reconcile ledgers stayed empty.

This is a generic graph-contract issue. Runtime lineage columns are valuable
for diagnostics, provenance, and resolution replay, but they are not relation
keys that should drive automatic join scaffolds. If a workflow needs to replay
row-level lineage, it should use a typed resolution/enrichment action with
explicit source/target semantics. A default relation join must be grounded on
ordinary materialized fields chosen by the model from artifact schemas.

Changes:

- [x] Added a shared `FieldUsableForRecordJoin` helper in
      `internal/dataworkflow`.
- [x] `JoinRecordScaffolds` now excludes `_`-prefixed runtime lineage fields
      from common join-key candidates.
- [x] REPL/CLI deterministic fallback join scaffolds use the same join-field
      rule, so the runtime cannot bypass the shared scaffold contract.
- [x] Added regression coverage that internal-lineage-only artifacts do not
      produce join scaffolds, while ordinary shared fields still do.
- [x] Added regression coverage that `dataTaskWorkflowNextStageFallback` does
      not keep a contribution workflow alive by auto-joining artifacts whose
      only common fields are internal lineage columns.

Remaining architecture items:

- [x] Add the first typed stage-progress signature to workflow state for
      repeated relation actions with no downstream contribution/reconcile
      progress. The live workflow now emits a `stage_no_progress` violation and
      deterministic fallback stops auto-selecting another `join_records` batch
      after repeated no-progress joins.
- [ ] Extend stage-progress signatures beyond repeated joins to include
      field-set deltas, row-count deltas, ledger deltas, and stage movement for
      every typed action kind.
- [ ] Move the remaining REPL-local scaffold sorting and concrete fallback
      builders into `internal/dataworkflow` so ActionDAG, ArtifactGraph, and
      LedgerGraph own the complete scheduling policy.
- [x] Add a domain-neutral value-distribution preview action so the planner can
      inspect actual field values before filtering/grouping/contribution work
      without inventing scripts or repeating relation joins.

### Batch 240: Diagnostic Artifact Boundary And Progress Detail De-duplication

The next real-scenario run got past the repeated join loop and correctly
surfaced a typed `stage_no_progress` signal. It then exposed a second generic
ArtifactGraph boundary issue: source/diagnostic children emitted by
entity-resolution actions, such as `#entity_resolution_source` and
`#entity_resolutions`, were still visible to relation scaffolds as if they were
ordinary executable mapping ledgers. The deterministic fallback could then try
to apply or join against diagnostic source views, producing field-contract
failures around `canonical_id`/`canonical_label` even though the base record
artifact already carried materialized prefixed canonical fields from a prior
valid apply step.

This is not a domain-data problem. Any data workflow with source-to-canonical
mapping can produce separate executable ledgers and diagnostic/source children.
The invariant is:

- executable DAG nodes may consume record artifacts, mapping ledgers, and
  generated typed artifacts;
- diagnostic/source children may be inspected for schema evidence and audit,
  but must not become automatic relation/action inputs;
- if a planner explicitly names a diagnostic child as a resolution input, the
  guard should return a typed action-input violation before execution;
- terminal progress should show business-facing plan text once at the planning
  or execution boundary, then keep result/evaluate events focused on outcome,
  audit, and next-state checks.

Changes:

- [x] Classified `#entity_resolutions`, `#entity_resolution_source`, and
      `#entity_resolution_reference` artifacts as diagnostic children in the
      shared ArtifactGraph projection.
- [x] Excluded diagnostic children from shared `ApplyResolutionScaffolds`.
- [x] Added REPL/CLI guard coverage that rejects diagnostic artifacts as
      `apply_entity_resolutions` resolution inputs with typed violation code
      `apply_resolution_diagnostic_input`.
- [x] Added regression coverage for ArtifactGraph classification, relation
      scaffolds, and REPL apply-resolution guard behavior.
- [x] De-duplicated data workflow permanent-detail rendering: execute/plan can
      show goal/batch/next-step details, while result/evaluate render outcome
      and audit details instead of repeating the same plan prose.
- [x] Changed action summaries to show action kinds rather than raw generated
      action ids, avoiding low-value truncated ids such as
      `continue_join_records_...` in user-facing permanent lines.
- [x] Added CLI stderr request anchoring (`数据请求` / `data request`) so
      single-shot logs show the original user request without polluting stdout.

Remaining architecture items:

- [ ] Extend the same diagnostic/executable boundary to any future generated
      child kind added by typed actions, ideally by carrying an explicit
      `node_class` from the runner instead of deriving it from id/kind suffixes.
- [ ] Promote repeated apply-resolution no-progress into the broader
      stage-progress signature alongside field-set, row-count, ledger, and
      stage movement deltas for every typed action kind.
- [ ] Move REPL-local relation scaffold sorting and concrete fallback builders
      into `internal/dataworkflow` so ActionDAG/ArtifactGraph/LedgerGraph own
      this policy in one place.

### Batch 241: Order-Independent Resolution Source Lineage

The next real-scenario gate moved past diagnostic-child pollution and surfaced
an ArtifactGraph lineage contract bug. A `normalize_entities` mapping ledger can
carry both the source record artifact and a reference/lookup material in
`source_paths`. The old `apply_entity_resolutions` guard treated
`source_paths[0]` as the only valid source lineage. When a ledger happened to
record `[reference, source]`, the runtime rejected a valid graph edge and kept
falling back to later stages without producing contribution ledgers.

This is not data-domain specific. Any typed normalization step can read a base
record set plus one or more reference/evidence materials, and serialized
lineage order is not a safe hard gate. The invariant is:

- hard compatibility must be based on typed artifact lineage intersection, not
  source-path ordering;
- if the base artifact lineage intersects any declared mapping source path, the
  mapping may proceed to the existing field-contract checks;
- if there is no lineage intersection, the guard still rejects the edge with
  `apply_resolution_lineage_contract`;
- diagnostic/source child artifacts remain non-executable inputs.

Changes:

- [x] Made `apply_entity_resolutions` mapping compatibility order-independent
      across declared source paths.
- [x] Kept incompatible mappings rejected when the base artifact has no lineage
      overlap with the mapping ledger.
- [x] Added regression coverage for both later-source-path compatibility and
      no-overlap rejection.

Remaining architecture items:

- [x] Split mapping ledger lineage into explicit `source_record_paths`,
      `reference_paths`, and `evidence_paths` in runner-produced ArtifactGraph
      metadata so future guards do not need to infer roles from mixed lineage.
- [ ] Move the apply-resolution compatibility guard into `internal/dataworkflow`
      once ArtifactGraph owns relation readiness for both CLI and REPL.

### Batch 242: Relation-Family No-Progress Signatures

The same real-scenario run proved that one narrow no-progress signal was not
enough. After a valid category normalization step, the runtime could still
cycle through `apply_entity_resolutions`, `join_records`, and similar relation
materialization actions. Each action produced another intermediate artifact,
but the required contribution and reconcile ledgers remained empty. The older
guard only counted repeated `join_records`, so repeated apply/enrich/join
families could still consume workflow budget without moving toward the user's
output.

This is a generic DAG-convergence issue. Relation materialization actions are
useful when they create the next executable record set, but they are not proof
of user-goal progress once a workflow is in the contribution/reconcile stage.
The invariant is:

- when contribution or reconcile ledgers are required and still absent,
  consecutive relation materialization results without ledger progress form a
  stage no-progress signature;
- relation materialization currently includes `apply_entity_resolutions`,
  `enrich_records`, and `join_records`;
- after the threshold is reached, deterministic scaffold fallback must stop
  selecting more relation materialization and steer toward field derivation,
  filtering, qualification, contribution calculation, or reconciliation;
- the gate is based on typed action kinds and ledger state only, not model
  prose or business keywords.

Changes:

- [x] Replaced join-only no-progress counting with relation-family
      no-progress counting.
- [x] Updated `stage_no_progress` violations to report the recent relation
      action family and generic repair hints.
- [x] Prevented deterministic fallback from auto-selecting apply/enrich/join
      scaffolds after repeated relation materialization without contribution or
      reconcile progress.
- [x] Added regression coverage for repeated `apply_entity_resolutions`
      no-progress alongside the existing repeated-join coverage.

Remaining architecture items:

- [ ] Extend the progress signature with field-set deltas, row-count deltas,
      ledger deltas, and stage movement so a relation action that truly changes
      the executable schema is distinguished from a no-op relation loop.
- [x] Move relation-family progress signatures into `internal/dataworkflow`
      with ActionDAG/ArtifactGraph/LedgerGraph state instead of keeping the
      policy in REPL-local workflow code.

### Batch 243: Workflow Stage Policy As IR

The architecture audit found that the data lane was still making one core
workflow decision from REPL-local code: the current workflow stage and the
allowed action contracts for that stage. That made the system harder to reason
about because the model-facing prompt, deterministic guards, CLI path, and REPL
path could drift while still appearing to share one "data workflow".

This is a generic workflow-engine issue, not a procurement-data issue. Stage
selection must be derived from typed workflow facts such as material coverage,
ledger requirements, ledger counts, reconciliation state, and answer
projection. It must not read model prose, action ids, or business keywords.

Changes:

- [x] Added `internal/dataworkflow.StageFacts` as the typed reducer input for
      next-stage selection.
- [x] Added shared stage constants and `NextStage` in `internal/dataworkflow`.
- [x] Moved stage-specific allowed action contracts into
      `internal/dataworkflow.ActionContract` and
      `AllowedNextActionContracts`.
- [x] Changed the REPL workflow adapter to delegate next-stage and
      allowed-action decisions to the shared workflow IR.
- [x] Added regression coverage for stage progression and allowed-action
      contract filtering.

Remaining architecture items:

- [x] Move relation-family no-progress signatures into `internal/dataworkflow`
      so stage progress is decided from typed action/ledger deltas rather than
      REPL records.
- [x] Move relation scaffold sorting and concrete fallback builders behind
      ArtifactGraph-aware workflow helpers.
- [ ] Make CLI and REPL consume the same workflow reducer and event sink so
      progress, violations, and terminal failures render consistently without
      duplicating orchestration policy.

### Batch 244: Relation Progress Signatures In Workflow IR

After stage policy moved into `internal/dataworkflow`, the next remaining hard
gate still lived in REPL-local code: repeated relation materialization without
ledger progress. That check is critical for convergence because relation
actions can keep producing intermediate artifacts while the workflow never
reaches contribution or reconciliation ledgers. Keeping it in REPL records made
the policy hard to share with CLI and made future workflow-state reducers
harder to reason about.

The generalized invariant is:

- relation materialization is a typed action family, currently
  `apply_entity_resolutions`, `enrich_records`, and `join_records`;
- a recent run of relation-family result events without contribution or
  reconcile progress is a stage progress violation when the workflow is trying
  to prepare or compute contributions;
- this decision uses only typed action kinds, result presence, errors, ledger
  counts, and stage facts;
- model prose, action ids, business nouns, and UI text do not participate in
  the hard gate.

Changes:

- [x] Added `dataworkflow.ProgressEvent` as the compact typed history view for
      progress checks.
- [x] Added shared relation-family detection and
      `SingleRelationMaterializationKind`.
- [x] Added `RecentRelationNoProgressCount`,
      `RelationNoProgressViolation`, and `WouldRepeatRelationNoProgress` in
      `internal/dataworkflow`.
- [x] Changed REPL workflow code to adapt records into progress events and
      delegate no-progress violation/fallback blocking to the workflow IR.
- [x] Added regression coverage for typed no-progress violation construction
      and relation fallback blocking.

Remaining architecture items:

- [ ] Extend progress signatures with field-set deltas, row-count deltas,
      schema deltas, ledger deltas, and stage movement so productive relation
      materialization is not confused with no-op relation loops.
- [x] Move concrete relation scaffold sorting and fallback construction behind
      ArtifactGraph-aware workflow helpers.
- [ ] Feed relation progress signatures into the shared CLI/REPL event sink so
      users see business-facing progress and compact audit details consistently.

### Batch 245: Scaffold Priority Policy In Workflow IR

The next architecture pass reduced another REPL-local workflow decision:
concrete scaffold prioritization. The data lane uses scaffold candidates to
guide bounded typed recovery when the planner stalls or emits an invalid
terminal plan. Candidate ordering is not UI behavior; it affects which graph
edge the runtime tries next. Keeping that priority map in REPL code made the
workflow harder to share with CLI and harder to audit.

The generalized invariant is:

- scaffold priority must depend on typed stage facts and action kinds;
- when entity/materialization has already happened and contribution/reconcile
  ledgers are still needed, relation materialization scaffolds should be
  considered before later field/filter/contribution scaffolds only through one
  shared policy;
- the priority decision must not read action ids, business words, or model
  prose.

Changes:

- [x] Added `dataworkflow.PrioritizeConcreteScaffolds`.
- [x] Reused `dataworkflow.ActionScaffold` directly in the REPL adapter instead
      of maintaining a parallel scaffold struct.
- [x] Changed REPL fallback candidate selection to delegate scaffold ordering
      to workflow IR.
- [x] Added regression coverage for stage-fact-driven scaffold prioritization.

Remaining architecture items:

- [x] Move concrete scaffold-to-action materialization into
      `internal/dataworkflow` so action ids, output aliases, and params are
      generated from one typed policy.
- [ ] Move fallback plan assembly behind a shared CLI/REPL workflow reducer.
- [ ] Replace remaining REPL-local stage/fallback prose with typed events that
      render business-facing summaries through the shared event sink.

### Batch 246: Concrete Scaffold Materialization

The scaffold audit found a hidden cross-layer contract drift. Generic
normalization scaffolds already emitted structured `source_fields`, but the
old REPL-local concrete-action converter only accepted the older singular
`source_field` shape. That meant some valid typed scaffolds were not executable
and the runtime sometimes appeared stable only because a useful but unsupported
candidate silently failed conversion.

This is a generic data-DAG issue. Scaffold-to-action materialization must
understand the typed action contract, not UI-local assumptions. It should
accept structured arrays/objects where the action runner accepts them, and it
must still reject placeholder fields or ambiguous templates before execution.

Changes:

- [x] Added `dataworkflow.ConcreteActionFromScaffold`.
- [x] Moved concrete action id, output alias, and params materialization for
      normalize/join/apply-resolution scaffolds into workflow IR.
- [x] Accepted both singular `source_field` and structured `source_fields`
      contracts for normalization scaffolds.
- [x] Added `dataworkflow.ConcreteFallbackScaffolds` to keep deterministic
      fallback candidates inside the current workflow stage. In contribution
      stages with entity materialization already complete, deterministic
      fallback no longer returns to a fresh normalize step.
- [x] Changed REPL concrete fallback to delegate scaffold materialization to
      `internal/dataworkflow`.
- [x] Added regression coverage for structured source-field scaffolds, join
      field materialization, and post-entity-stage fallback filtering.

Remaining architecture items:

- [ ] Move full fallback plan assembly into a shared data workflow reducer so
      CLI and REPL no longer each own plan-status, reason, and terminal-error
      recovery behavior.
- [x] Add apply-resolution graph-edge readiness checks that use explicit
      ArtifactGraph lineage roles, not mixed `source_paths`, before
      materializing mapping-to-base edges.
- [ ] Expand concrete scaffold materialization beyond relation actions to
      filter/qualify/contribution projection where the params are fully
      concrete and validated by artifact schema projection.

### Batch 247: Validation Stage And Terminal Guard IR

The architecture audit then moved another hard workflow decision out of the
REPL layer: unfinished validation-stage detection and terminal-plan rejection.
Both CLI and REPL depend on this logic, and it directly controls whether the
workflow should continue, repair, or surface a terminal failure. Keeping it in
the UI package made it too easy for terminal behavior and repair hints to
drift from the actual workflow state machine.

The generalized invariant is:

- missing validation stages are derived from typed facts only: rule coverage,
  decision records, entity resolution, contribution ledger, reconcile, and
  final answer projection;
- terminal statuses are structural control states, not model prose to parse for
  business meaning;
- a terminal plan with unfinished validation stages must produce one typed
  `unfinished_validation_stage` guard with legal next-action hints.

Changes:

- [x] Extended `dataworkflow.StageFacts` with decision-record facts.
- [x] Added `dataworkflow.MissingValidationStages`.
- [x] Added shared terminal-status classification and
      `TerminalWorkflowGuardResult`.
- [x] Changed REPL workflow adapters to delegate missing-stage and terminal
      guard decisions to `internal/dataworkflow`.
- [x] Added regression coverage for missing-stage ordering and terminal guard
      construction.

Remaining architecture items:

- [ ] Move full fallback plan assembly and continuation repair into a shared
      reducer so CLI and REPL consume the same state transition object.
- [ ] Add storage-neutral workflow journal snapshots for reducer inputs and
      terminal guard outputs.
- [ ] Replace raw CLI terminal errors with structured failure events that still
      keep stdout clean.

### Batch 248: Role-Aware Mapping Lineage

The previous source-lineage fix made compatibility order-independent, but the
architecture was still weaker than it should be: mapping artifacts carried
source records, reference records, and evidence refs inside one mixed
`source_paths` list. A guard could avoid ordering bugs, but it still could not
distinguish "this mapping was produced from the base rows" from "this mapping
also read a reference table".

This is a generic ArtifactGraph issue. Any mapping/normalization step may read
source records plus reference/evidence materials. Graph-edge readiness should
use role-specific lineage when it exists and fall back to mixed lineage only
for older or non-role-bearing artifacts.

Changes:

- [x] Extended `dataquery.DataArtifact` with `source_record_paths`,
      `reference_paths`, and `evidence_paths`.
- [x] Projected those lineage roles through
      `dataworkflow.ArtifactSchemaProjection`.
- [x] Added role lineage to `normalize_entities` artifacts and their
      source/reference diagnostic children.
- [x] Extended compact artifact access so CLI/REPL prompt state and guards see
      the same role-specific lineage.
- [x] Changed apply-resolution compatibility to prefer
      `source_record_paths`; mixed `source_paths` is now only a fallback when
      role lineage is absent.
- [x] Added regression coverage for runner lineage output, ArtifactGraph
      projection, and role-preferred apply-resolution guard behavior.

Remaining architecture items:

- [ ] Move the apply-resolution compatibility guard itself into
      `internal/dataworkflow` once relation readiness is represented as a
      first-class graph edge validator.
- [ ] Extend role-aware readiness to other relation edges where role ambiguity
      matters, such as enrichment and joins that consume explicit
      reference-side artifacts.
- [ ] Persist role lineage in workflow journal snapshots for postmortem audit
      and opt-in resume.

### Batch 249: CLI Terminal Failure Event

The CLI data lane already kept final answers on stdout and progress on stderr,
but failed terminal runs still relied too much on the caller's raw `error: ...`
line after the audit path. That was technically correct but poor for user
orientation: users saw an audit artifact path and then an unstructured error
string, with no process-event style explanation.

The generalized invariant is:

- stdout remains reserved for the final answer so strict output contracts and
  shell pipelines are not polluted;
- stderr/progress should contain a low-noise structured failure event with the
  terminal audit path and concise reason;
- the reason is display-only and does not drive workflow logic.

Changes:

- [x] Extended CLI terminal audit rendering to include a compact failure
      reason line for non-complete statuses.
- [x] Kept successful terminal audit rendering unchanged.
- [x] Added regression coverage for the CLI failure-reason event.

Remaining architecture items:

- [ ] Move CLI and REPL process-event rendering behind a shared event sink so
      request, plan, execute, evaluate, repair, terminal audit, and failure
      details use one renderer contract.
- [ ] Add business-facing summaries from typed planner/evaluator fields while
      keeping internal ledger counts as low-noise audit details.

### Batch 250: Typed Value Distribution Diagnostic Action

The next architecture audit focused on why complex data workflows can still
spend too many turns in repair. A repeated pattern was not business-specific:
the workflow had a record artifact and valid field names, but the planner did
not have objective value distributions before choosing filters, grouping keys,
mapping parameters, or contribution inputs. Without a typed diagnostic action,
the planner had to infer values from compact prompt samples, ask for broad
`inspect_material`, or fall back to scripts. That makes convergence depend on
sample luck instead of graph state.

The generic invariant is:

- field-value observation is an atomic read-only DAG node;
- the node consumes exactly one existing record artifact/path and emits a
  reusable diagnostic artifact with per-field non-empty, empty, distinct, and
  top-value counts;
- the system does not assign business meaning to those values and does not
  choose filters or groups from prose;
- deterministic fallback may use this diagnostic node when relation joins would
  otherwise repeat or when values are unknown, but it must not inspect runtime
  lineage fields such as `_source` / `_left_index` as business data;
- exact calculation still happens through typed transform/contribution/
  reconcile/projection actions over complete artifacts.

Changes:

- [x] Added `value_distribution` as a `dataquery.DataActionKind`.
- [x] Implemented `ActionRunner` support for `value_distribution`, including
      single-record input handling, field validation, bounded row/field limits,
      top-value counts, and source-record lineage.
- [x] Added workflow capability and allowed-action contracts for the stages
      where value inspection is structurally useful.
- [x] Taught the data planner schema and prompt to use `value_distribution`
      when actual field values are unclear before filters, joins, mappings,
      grouping, or contribution parameters.
- [x] Added concrete scaffold materialization for `value_distribution`.
      Placeholder field templates are replaced with concrete non-internal
      artifact fields before execution.
- [x] Kept the internal-lineage join guard intact: deterministic fallback still
      cannot auto-join artifacts whose only shared fields are runtime lineage,
      but it may choose the read-only value diagnostic action to make progress.
- [x] Added unit coverage for runner output, workflow contracts, concrete
      scaffold materialization, planner schema teaching, and the internal
      lineage fallback boundary.

Remaining architecture items:

- [ ] Add a domain-neutral mapping-candidate action that proposes candidate
      matches with evidence and ambiguity flags without assigning business
      meaning.
- [ ] Promote value-distribution results into evaluator state as typed
      `field_contract_observation` objects so repair prompts do not need to
      rediscover the diagnostic artifact by alias.
- [ ] Move full fallback plan assembly into the shared workflow reducer so CLI
      and REPL consume the same typed state transition.
- [ ] Add realistic multi-file eval gates that assert value-distribution
      diagnostics appear before zero-match filter/contribution repair loops
      when field values are uncertain.

### Batch 251: Shared Concrete Fallback Plan Reducer

The next IR pass moved another scheduling decision out of the REPL layer. The
workflow already had typed scaffolds and concrete action materialization in
`internal/dataworkflow`, but one path still assembled the fallback `TaskPlan`
inside REPL code: pick a scaffold, materialize an action, merge input paths,
carry coverage/output contracts, attach `continue_after`, and skip actions that
had already run. That is not UI behavior; it is ActionDAG state transition.

The generic invariant is:

- a concrete fallback plan is derived from typed workflow facts, coverage
  contract, output contract, action scaffolds, and previously seen action
  idempotency keys;
- the reducer does not read user/business keywords or model prose to decide
  whether an action is legal;
- REPL/CLI code may adapt local history into typed reducer inputs and render
  the result, but should not own the scheduling policy;
- repeated fallback prevention uses stable action idempotency keys, not UI text.

Changes:

- [x] Added `dataworkflow.BuildConcreteFallbackPlan`.
- [x] Added `dataworkflow.ConcreteFallbackPlanInput` with current plan,
      coverage/output contracts, stage facts, scaffolds, reason prefix, and
      seen action keys.
- [x] Added `dataworkflow.ActionIdempotencyKeys` so adapters can project
      previous actions into reducer-ready replay guards.
- [x] Changed the REPL concrete-scaffold fallback path to call the shared
      reducer instead of assembling the fallback plan locally.
- [x] Kept thin REPL adapters only where prompt/state rendering still needs
      scaffold previews; hard plan assembly now lives in the workflow package.
- [x] Added reducer regression coverage for contract preservation, concrete
      non-internal field materialization, and seen-action suppression.

Remaining architecture items:

- [x] Move the larger next-stage fallback assembly into a shared reducer. It
      still builds extra relation/action scaffolds from REPL-local artifact
      views before calling concrete action materialization. Completed in Batch
      252 by passing `ArtifactSchemaProjection`, allowed-action contracts, and
      progress events into `internal/dataworkflow`.
- [ ] Replace REPL-local fallback signatures with reducer-owned action graph
      replay checks everywhere, including deferred-plan and terminal-repair
      paths.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 252: Next-Stage Concrete Fallback Reducer

The next IR pass moved the bigger next-stage concrete fallback out of the REPL
package. Before this batch, `internal/repl` still inspected workflow state,
assembled relation scaffolds, chose allowed actions, materialized concrete
actions, checked repeated no-progress loops, and returned a continuation plan.
That made the data lane look like it had an IR, while one of the most important
ActionDAG transitions was still owned by UI orchestration code.

The generalized invariant is:

- next-stage fallback is a reducer transition over typed state: stage facts,
  allowed next actions, artifact schema projections, extra scaffolds, seen
  idempotency keys, and typed progress events;
- REPL/CLI adapters may project local history into those typed inputs, but they
  should not own relation scheduling policy;
- the reducer generates concrete actions from artifact fields and schema
  projections only; it does not parse model prose or business-specific user
  words to decide hard behavior;
- repeated relation materialization without ledger/reconcile progress is
  blocked through typed progress events before another automatic relation
  action is emitted.

Changes:

- [x] Added `dataworkflow.NextStageFallbackPlanInput`.
- [x] Added `dataworkflow.BuildNextStageConcreteFallbackPlan`.
- [x] Moved allowed-action filtering, relation scaffold expansion, and
      next-stage fallback plan assembly into `internal/dataworkflow`.
- [x] Extended `BuildConcreteFallbackPlan` with typed progress events and a
      no-progress threshold so repeated relation fallback is stopped in the
      reducer rather than in REPL code.
- [x] Changed REPL next-stage fallback to pass `StageFacts`,
      `ArtifactSchemaProjection`, allowed actions, progress events, and seen
      action keys to the reducer.
- [x] Improved shared relation scaffold builders so executable normalize and
      apply-resolution fallbacks prefer concrete role/direction and target
      field contracts from artifact schemas rather than placeholder templates.
- [x] Added reducer regression coverage for next-stage artifact-schema fallback
      and relation no-progress blocking.
- [x] Kept the fix domain-neutral: no procurement terms, no output-shape
      special cases, and no hard gates on noisy model prose.

Remaining architecture items:

- [x] Move record-materialization, rule/reconcile/final-projection fallback
      assembly into the shared reducer so all continuation transitions share
      one state-machine boundary. Completed in Batch 253 for plan assembly;
      adapter-only context gathering remains outside the reducer.
- [ ] Replace remaining REPL-local fallback signatures with reducer-owned
      action graph replay checks, including deferred-plan and terminal-repair
      paths.
- [x] Separate prompt-only scaffolds from executable scaffolds where a template
      cannot be made concrete from `ArtifactSchemaProjection`. Completed in
      Batch 256.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 253: Reducer-Owned Materialization And Completion Plans

The next reducer pass moved more deterministic fallback plan construction out
of the REPL package. Before this batch, REPL code still owned plan assembly for
record materialization, rule coverage completion, reconcile completion, and
final output projection. These are not UI concerns: they are typed workflow
state transitions that should be generated from contracts, stage facts, and
validated result state.

The generalized invariant is:

- the reducer builds deterministic continuation/completion plans from typed
  contracts and result state;
- adapters may collect environment-bound context such as artifact projections
  or structural reference-key candidates, but plan assembly lives in
  `internal/dataworkflow`;
- final projection changes formatting and missing reference-key projection
  only; it must not alter contribution records, business decisions, or numeric
  values;
- missing ledger completion is driven by typed validation violations and
  existing structured result data, not by parsing model prose.

Changes:

- [x] Added `BuildRecordMaterializationFallbackPlan`.
- [x] Added `RecordMaterializationPaths` and `HasRecordActionArtifact` so
      required-material-to-record-artifact fallback is reducer-owned.
- [x] Added `BuildRequiredOutputProjectionPlan` and
      `ResultNeedsOutputProjection` for terminal `assemble_answer` planning.
- [x] Added `BuildRequiredLedgerCompletionPlan` for missing rule coverage and
      missing reconcile ledger completion.
- [x] Added reducer-owned `RuleCoverageCompletionAction`,
      `BestOutputContract`, `NormalizeCoverageMaterialUseMode`, and
      `PathLooksLikeTextConstraintMaterial` helpers.
- [x] Changed REPL output projection, ledger completion, derive-rules
      fallback, and record-materialization fallback to delegate plan assembly
      to `internal/dataworkflow`.
- [x] Kept environment-bound reference-key inference in the adapter: the
      reducer consumes a typed `ReferenceProjectionGap` instead of reading
      files or inferring business meaning itself.
- [x] Added reducer regression coverage for record materialization,
      assemble-answer projection, and reconcile-ledger completion.

Remaining architecture items:

- [ ] Replace remaining REPL-local fallback signatures with reducer-owned
      action graph replay checks, including deferred-plan and terminal-repair
      paths.
- [ ] Move repeated-node typed repair plans into reducer transitions where
      they depend only on typed violations and contracts. Coverage expansion
      and material discovery plan assembly moved in Batch 254.
- [x] Separate prompt-only scaffolds from executable scaffolds where a template
      cannot be made concrete from `ArtifactSchemaProjection`. Completed in
      Batch 256.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 254: Reducer-Owned Coverage And Discovery Plans

The next reducer pass moved two more deterministic plan builders out of the
REPL package: missing-material coverage expansion and broad material discovery.
The trigger decisions still live in adapters for now because they depend on
history, candidate lists, terminal scheduling checks, and broad custom-action
surface checks. The plan assembly itself is now reducer-owned.

The generalized invariant is:

- when typed adapter state says required materials are missing, the reducer
  builds an atomic coverage batch by objective material shape: text-like
  materials can derive rule coverage when the workflow contract needs rules,
  structured files can materialize record samples, and other materials can be
  inspected;
- when typed adapter state says a broad plan needs inventory first, the
  reducer builds a single material-inventory batch and clears speculative
  material floors for that inventory turn;
- material shape uses file-format metadata only. It does not inspect user
  intent, model prose, or domain-specific names;
- validation-rule breadcrumbs remain audit-only context; they do not drive hard
  scheduling behavior.

Changes:

- [x] Added `BuildCoverageExpansionPlan`.
- [x] Added `BuildMaterialDiscoveryPlan`.
- [x] Added reducer-owned `PathLooksLikeStructuredMaterial` alongside the
      existing text-material shape helper.
- [x] Changed REPL coverage expansion and material discovery fallbacks to pass
      typed missing paths / discovery paths into `internal/dataworkflow`.
- [x] Kept trigger predicates in REPL adapters for now; reducer owns only the
      plan transition once typed inputs are available.
- [x] Added reducer regression coverage for mixed material-shape coverage
      expansion and material discovery floor clearing.

Remaining architecture items:

- [ ] Replace remaining REPL-local fallback signatures with reducer-owned
      action graph replay checks, including deferred-plan and terminal-repair
      paths.
- [ ] Move repeated-node typed repair plans into reducer transitions where
      they depend only on typed violations and contracts. Repeated-failure
      detection and custom-transform guard construction moved in Batch 255;
      deterministic replacement plan generation remains open.
- [x] Separate prompt-only scaffolds from executable scaffolds where a template
      cannot be made concrete from `ArtifactSchemaProjection`. Completed in
      Batch 256.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 255: Typed Repeated-Failure Guards

The next reducer pass moved repeated-failure detection and repeated
custom-transform guard construction out of REPL-local helpers. These checks are
not display behavior: they decide when the workflow should stop retrying the
same failed node or the same broad custom-transform failure class and force the
graph to expand through typed actions.

The generalized invariant is:

- repeated-node detection uses typed `DataTaskViolation` fields such as
  `action_id` and `action_kind`, not model prose;
- custom-transform cooldown guards are typed `WorkflowViolation` /
  `GuardResult` values produced by `internal/dataworkflow`;
- adapters still collect local history and decide whether an action is broad or
  whole-workflow shaped, but the failure counting and guard text no longer live
  in UI code;
- these guards only block unsafe repetition and guide repair; they do not infer
  business semantics or choose domain-specific fixes.

Changes:

- [x] Added `ViolationNodeKey` and `RepeatedNodeFailureFromErrors`.
- [x] Added `RepeatedCustomTransformGuardResult`.
- [x] Added `CustomTransformFailureClassStats`.
- [x] Added `RepeatedCustomTransformClassGuardResult`.
- [x] Changed REPL repeated-node and custom-transform failure helpers to
      delegate typed counting and guard construction to `internal/dataworkflow`.
- [x] Added reducer regression coverage for repeated node detection, repeated
      custom-transform node guard, and repeated custom-transform failure class
      guard.

Remaining architecture items:

- [x] Generate deterministic replacement plans for repeated typed failures
      when the violation carries enough structural context; otherwise continue
      to planner expansion with typed guard state. Completed in Batch 257.
- [ ] Replace remaining REPL-local fallback signatures with reducer-owned
      action graph replay checks, including deferred-plan and terminal-repair
      paths.
- [x] Separate prompt-only scaffolds from executable scaffolds where a template
      cannot be made concrete from `ArtifactSchemaProjection`. Completed in
      Batch 256.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 256: Executable Scaffold Boundary

The next IR pass closed an important scaffold boundary. Before this batch,
`action_scaffold` entries were all the same shape: some were fully concrete
system-generated candidates that could safely become the next typed action,
while others were prompt-only templates that still contained placeholders or
required the model to choose task-specific parameters. The converter happened
to reject many prompt-only templates because placeholders remained, but the
contract was implicit and easy to regress.

This is a generic data-DAG issue, not a business-domain issue. A workflow
runtime can suggest templates to the model, but deterministic execution must
only consume templates whose fields and params have been closed by system-owned
artifact schema projection. The model remains responsible for business meaning;
the reducer owns only structural conversion of executable IR.

The generalized invariant is:

- `ActionScaffold.executable=true` means the scaffold has enough concrete
  artifact fields, input aliases, and action params for the reducer to attempt
  deterministic materialization;
- missing or false `executable` means prompt-only guidance. It may appear in
  `workflow_state_json.action_scaffold`, but cannot be converted directly into
  a `DataAction`;
- executable marking is produced by system builders from
  `ArtifactSchemaProjection`, not by parsing user intent, model prose, or
  domain-specific file names;
- prompt-only templates can still help the planner fill concrete fields for
  `filter_records`, `qualify_records`, `compute_contributions`, or
  `enrich_records`, but execution waits for a real action plan with concrete
  params.

Changes:

- [x] Added `ActionScaffold.Executable` to the workflow IR.
- [x] Made `ConcreteActionFromScaffold` reject prompt-only scaffolds before
      action-kind conversion.
- [x] Marked shared concrete relation scaffolds executable only when the
      reducer can close them from artifact schema projection:
      `normalize_entities`, `join_records`, and `apply_entity_resolutions`.
- [x] Kept `enrich_records` prompt-only until a concrete target field and
      lookup spec are supplied by the planner.
- [x] Marked REPL value-distribution scaffolds executable because the reducer
      can replace placeholder fields with concrete non-internal artifact
      fields.
- [x] Taught the planner prompt to distinguish executable scaffolds from
      prompt-only templates without using that distinction as a business gate.
- [x] Added regression coverage that concrete scaffolds still materialize and
      prompt-only scaffolds cannot enter deterministic fallback.

Remaining architecture items:

- [x] Generate deterministic replacement plans for repeated typed failures
      when the violation carries enough structural context; otherwise continue
      to planner expansion with typed guard state. Completed in Batch 257.
- [ ] Replace remaining REPL-local fallback signatures with reducer-owned
      action graph replay checks, including deferred-plan and terminal-repair
      paths.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 257: Repeated Failure Replacement Transition

The next reducer pass moved the first repeated typed-failure replacement path
out of ad hoc CLI/REPL retry logic. Before this batch, once the same typed node
failed enough times, both CLI and REPL immediately asked the continuation
planner to expand the graph. That was safer than retrying the same node
forever, but it still spent a model turn even when the workflow IR already
contained a concrete executable scaffold that could advance the graph.

This is a generic convergence issue. Repeated failures should first be handled
by typed workflow state, not by prose repair. If the reducer can prove a
replacement action is executable from `ActionScaffold.executable=true`,
current contracts, stage facts, progress events, and seen action keys, it may
emit that atomic continuation. If not, the existing planner expansion path
remains the fallback.

The generalized invariant is:

- repeated-node detection uses typed `DataTaskViolation` action id/kind, not
  model prose;
- deterministic replacement can only be produced from executable scaffolds and
  existing workflow contracts;
- seen action keys and relation no-progress events are checked before the
  replacement is emitted, so the reducer does not replay the same graph edge;
- the replacement plan never chooses business-specific filters, metrics, or
  entity meanings. Those still require a planner-emitted concrete action.

Changes:

- [x] Added `RepeatedFailureReplacementPlanInput` and
      `BuildRepeatedFailureReplacementPlan` in `internal/dataworkflow`.
- [x] The reducer now combines repeated-node detection with
      `BuildConcreteFallbackPlan` so only executable scaffold transitions can
      be emitted deterministically.
- [x] Wired both CLI and REPL data workflows to try the reducer replacement
      before calling the continuation planner on repeated node failure.
- [x] Preserved the existing planner expansion path when no executable
      replacement exists.
- [x] Added reducer regression coverage for executable repeated-failure
      replacement and prompt-only scaffold rejection.

Remaining architecture items:

- [ ] Finish replacing REPL-local fallback signatures with reducer-owned action
      graph replay checks for terminal-repair paths. Deferred-plan replay moved
      in Batch 258.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 258: Reducer-Owned Deferred Dispatch

The next IR pass moved deferred queue rank selection and next/remainder plan
assembly into `internal/dataworkflow`. Before this batch, the adapter owned the
logic that scanned deferred actions, skipped stale nodes, grouped ready actions
by dependency rank, and rebuilt the current plus remaining deferred plans. The
adapter still needs to evaluate environment-bound readiness, such as artifact
aliases and field-contract availability, but the graph replay decision itself
is workflow IR behavior.

This is a generic DAG scheduling issue. Deferred actions are accepted typed
graph nodes, not planner prose. Once the current workflow state says an action
is ready and allowed, rank selection should happen through a shared reducer so
CLI and REPL cannot drift.

The generalized invariant is:

- adapters produce `DeferredActionCandidate` objects with objective readiness,
  optional rewritten actions, and blocked reasons;
- the reducer chooses the first ready allowed dependency rank and preserves all
  unselected actions as the remaining deferred plan;
- custom scripts are not dispatched from deferred replay;
- no business meaning is inferred from action ids, prose, or material names.
  The reducer only consumes typed action kind, dependency rank, allowed action
  set, and readiness flags.

Changes:

- [x] Added `DeferredActionCandidate`, `DeferredDispatchInput`,
      `DeferredDispatchStatus`, and `BuildDeferredDispatchPlan`.
- [x] Moved deferred rank selection and next/remainder plan assembly from
      REPL helpers into `internal/dataworkflow`.
- [x] Kept artifact/material/field readiness in the adapter as a typed
      candidate-generation layer.
- [x] Reused the reducer dispatch in both actual deferred popping and queue
      status reporting.
- [x] Added reducer regression coverage for ready-rank selection and blocked
      deferred queue status.

Remaining architecture items:

- [x] Move terminal-completion repair signatures behind reducer-owned
      transition objects. Terminal next-stage fallback and output projection
      moved in Batch 259; completion repair transition moved in Batch 260.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 259: Reducer-Owned Terminal Next-Stage Fallback

The next IR pass moved terminal next-stage fallback assembly into
`internal/dataworkflow`. Before this batch, terminal plans that ended early
were detected by shared stage facts, but the REPL adapter still owned the
stage-specific switch for `derive_rules`, record materialization, concrete
relation/action scaffolds, reconciliation, and answer projection.

This is a workflow-state transition, not a UI concern. A terminal model status
is only a candidate; if typed facts show unfinished validation stages, the
shared reducer should decide the next safe atomic stage from facts and
contracts. The adapter may still supply environment-bound inputs such as
artifact schema projections, latest result, and reference-key projection gaps,
but it no longer assembles the fallback plan itself.

The generalized invariant is:

- terminal fallback decisions are driven by `StageFacts`, missing validation
  stages, coverage/output contracts, artifact projections, latest result, and
  executable scaffolds;
- the reducer may emit rule coverage, record materialization, concrete next
  action, reconcile, or answer projection plans when the required typed inputs
  exist;
- reference completion remains structural: the adapter computes a typed
  `ReferenceProjectionGap`; the reducer does not read files or assign business
  meaning;
- no terminal status or repair text is parsed for business intent.

Changes:

- [x] Added `WorkflowNextStageFallbackPlanInput` and
      `BuildWorkflowNextStageFallbackPlan`.
- [x] Moved terminal next-stage switch and plan assembly from REPL helpers into
      `internal/dataworkflow`.
- [x] Reused existing reducer-owned record materialization, concrete scaffold,
      required reconcile, and required output projection builders under one
      transition.
- [x] Changed REPL terminal/deterministic continuation fallback to pass typed
      state into the reducer instead of assembling plans locally.
- [x] Added reducer regression coverage for terminal reconcile continuation
      and final answer projection.

Remaining architecture items:

- [x] Move terminal-completion repair signatures behind reducer-owned
      transition objects so completion repair, repair failure fallback, and
      terminal audit failure state share one state-machine result. Completed in
      Batch 260 for completion transition; repair-failure fallback still emits
      through the shared process-event item below.
- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 260: Completion Repair Transition

The next IR pass wrapped terminal completion repair in a reducer-owned
transition object. Before this batch, CLI and REPL both checked a completion
gate, then each tried a deterministic ledger/output repair plan, then each
fell back to the model repair planner. The deterministic plan builders already
lived in `internal/dataworkflow`, but the transition result itself was still
duplicated in adapter code.

This is a generic completion-state problem. Once the workflow has a validation
failure and a latest structured result, the system should first ask the
workflow reducer whether the failure is a known structural completion gap. If
it is, the reducer returns a deterministic continuation plan. If it is not, the
transition explicitly says `needs_planner_repair=true` and the existing repair
planner remains responsible.

The generalized invariant is:

- completion repair transition consumes typed contracts, latest result,
  completion error text, and optional reference projection gap;
- deterministic repair may only complete structural ledgers/projection from
  existing facts, such as missing reconcile or assemble-answer projection;
- unknown or semantic failures do not get patched by the system and are handed
  to planner repair;
- CLI and REPL consume the same transition shape.

Changes:

- [x] Added `CompletionRepairTransitionInput`,
      `CompletionRepairTransition`, and `BuildCompletionRepairTransition`.
- [x] Reused `BuildRequiredLedgerCompletionPlan` under the transition so
      missing required reconcile/projection still uses existing reducer
      builders.
- [x] Changed CLI and REPL terminal-completion and evaluator-completion paths
      to consume the same transition before invoking planner repair.
- [x] Kept adapter-owned reference-gap calculation as typed input; reducer does
      not read local files or infer business meaning.
- [x] Added reducer regression coverage for deterministic completion repair
      and planner-repair fallback.

Remaining architecture items:

- [x] Feed reducer transition results into a shared CLI/REPL process-event
      sink so users see business-facing action intent while audit counters stay
      low-noise. Completed in Batch 261.

### Batch 261: Shared Process Event Sink And Business-Facing Data UX

The next IR pass closed the user-facing transparency gap without moving
scheduling policy back into the REPL layer. Before this batch, CLI, REPL, and
workflow journal snapshots each rebuilt process summaries in slightly different
ways. Business-facing fields such as goal, batch purpose, next step, and action
purpose were available in the typed plan, but the display path still passed
plain strings around and re-classified them from localized prefixes.

This is a generic process-event problem, not a business-domain problem. Data
workflows should expose what the model is trying to do in business terms while
keeping internal counters available as low-noise audit details. The system must
not parse user intent or model prose to decide hard behavior; the display layer
may render already-typed plan fields for transparency.

The generalized invariant is:

- `internal/dataworkflow` owns the typed process event shape used by journals,
  CLI, and REPL;
- process events carry structured fields: goal, batch purpose, next step,
  action summary, audit details, status, reason, and optional guard state;
- action summary prefers `action.purpose` when present and falls back to typed
  action kind, never to business-specific file names or domain keywords;
- REPL/CLI render details from typed markers, not from localized prefix
  matching;
- plan panels may show full business intent; workflow result/evaluate rows keep
  audit counters low-noise and avoid repeating the same goal every batch.

Changes:

- [x] Added `WorkflowProcessEventInput` and
      `BuildWorkflowProcessEvent` in `internal/dataworkflow`.
- [x] Added reducer-level `ActionIntentSummary` and neutral
      `ResultAuditDetails` so terminal/checkpoint journals and UI can share one
      process-event projection.
- [x] Changed checkpoint and terminal journal event construction to use the
      shared process-event builder.
- [x] Changed REPL and CLI plan/workflow details to render typed business
      details through internal markers instead of localized prefix matching.
- [x] Changed data workflow execution details to show batch purpose, next step,
      and action intent without repeating the overall goal on every result and
      evaluation row.
- [x] Kept CLI progress on stderr and final answer on stdout; the request line
      remains emitted through the existing stderr progress channel.
- [x] Added regression coverage for typed process events, neutral result audit
      details, marker stripping, and low-noise result/evaluate rendering.

Remaining architecture items:

- [x] Re-audit the full design document for historical `Remaining architecture
      items` that are already superseded by later batches, and split any truly
      open items into a short current backlog before running the real scenario.
      Current backlog refreshed in Batch 262.

### Batch 262: Durable Workflow Decision IR And Current Backlog Audit

The next IR pass promoted workflow decisions from scattered status/reason
strings into a durable `WorkflowDecision` object. The type already existed, but
live `workflow_state_json`, terminal audit snapshots, and checkpoint snapshots
did not consistently carry it. That left evaluator decisions and typed
violations visible, but not as one compact state-machine result.

This is a generic workflow-state problem. A data workflow should always expose
whether the next state is `continue`, `blocked`, `complete`, or a typed
evaluator status; the reason code should come from typed stage/evaluation/
violation values; the suggested next actions should come from allowed actions
or violation repair hints. The decision object is audit and planner context; it
does not parse user intent or model prose to drive hard gates.

Changes:

- [x] Added `WorkflowDecisionInput` and `BuildWorkflowDecision` in
      `internal/dataworkflow`.
- [x] Projected current workflow decisions into live `workflow_state_json`.
- [x] Promoted the latest evaluator typed status/reason into `WorkflowDecision`
      when available.
- [x] Persisted `decision` in terminal and checkpoint workflow journals.
- [x] Added regression coverage for continue decisions, blocked typed-violation
      decisions, evaluator-decision projection, and journal JSON.
- [x] Re-audited the recent design-log backlog. Historical `[ ]` items remain
      in older ledger sections, but the current architecture backlog below is
      the authoritative list to clear before another real-scenario run.

Current backlog before real-scenario testing:

- [ ] Move remaining REPL adapter-owned trigger predicates behind reducer-owned
      transition inputs where they make scheduling decisions, especially broad
      custom-transform/material-discovery triggers and field/relation guard
      entrypoints that still produce prose wrappers around typed violations.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 263: Reducer-Owned Plan Shape Guard

The next IR pass moved the ready-plan shape guard out of the REPL adapter and
into `internal/dataworkflow`. Before this batch, the REPL decided whether a
ready data plan with no `actions[]` was executable, missing a result emitter,
too complex as a top-level script, or oversized for one bounded batch. Those
decisions are scheduling policy, not UI policy.

This remains domain-neutral. The reducer consumes only typed structural counts:
plan status, whether actions exist, whether a script exists, script line count,
result-emitter presence, input count, required-material count, validation-ledger
count, and configured size limits. It does not inspect business file names,
task prose, or model free text. The REPL still owns environment-bound
projection of local facts such as script line counting; the reducer owns the
decision and typed violation.

Changes:

- [x] Added `PlanShapeGuardInput` and `PlanShapeGuardResult` in
      `internal/dataworkflow`.
- [x] Returned typed `GuardResult` / `GenericViolation` values for
      `missing_executable_body`, `missing_result_emitter`,
      `complex_top_level_script`, and `oversized_data_batch`.
- [x] Changed REPL staging guard code to pass typed plan-shape facts into the
      reducer instead of constructing those guard results locally.
- [x] Kept terminal required-material scheduling after plan-shape validation so
      existing guard priority is preserved.
- [x] Added reducer regression coverage for missing executable body, missing
      result emitter, complex top-level script, oversized script, and action
      batch pass-through.

Current backlog before real-scenario testing:

- [ ] Move the remaining REPL adapter-owned scheduling predicates behind
      reducer-owned transition inputs. The top-level plan-shape guard is done;
      broad custom-transform/material-discovery triggers and field/relation
      guard entrypoints still need the same treatment where they produce hard
      scheduling decisions.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 264: Reducer-Owned Required Material Scheduling Guard

The next IR pass moved terminal required-material scheduling from REPL-local
string construction into `internal/dataworkflow`. Before this batch, the REPL
collected scheduled material consumption from previous results and the current
plan, then directly built a prose error when a terminal batch declared required
runner inputs that were not scheduled for script or typed-action consumption.

This is a generic coverage/scheduling invariant. A terminal data batch may only
complete when every required runner input is either already consumed by prior
workflow facts or scheduled by the current executable batch. The reducer does
not inspect file names, business roles, or task prose. The adapter still
collects environment-bound facts such as consumed paths and action input paths;
the reducer owns the missing-path decision and typed violation.

Changes:

- [x] Added `RequiredMaterialSchedulingGuardInput` and
      `RequiredMaterialSchedulingGuardResult` in `internal/dataworkflow`.
- [x] Returned a typed `required_material_scheduling` violation carrying the
      missing required paths as `input_aliases`.
- [x] Changed REPL plan/workflow staging guards to return the reducer-owned
      guard instead of wrapping a local prose error.
- [x] Kept the older string helper as a compatibility adapter for fallback
      predicates that still only need an error text.
- [x] Added reducer regression coverage for terminal missing paths,
      continuation pass-through, and already scheduled paths.

Current backlog before real-scenario testing:

- [ ] Move the remaining REPL adapter-owned scheduling predicates behind
      reducer-owned transition inputs. Top-level plan shape and terminal
      required-material scheduling are done; broad custom-transform/material-
      discovery triggers and field/relation guard entrypoints still need the
      same treatment where they produce hard scheduling decisions.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 265: Reducer-Owned Material Discovery Transition

The next IR pass moved the broad-material discovery trigger into
`internal/dataworkflow`. Before this batch, the REPL adapter owned both the
facts and the scheduling decision for converting an overly broad script/custom
plan into an objective `material_inventory` batch. The plan builder already
lived in the reducer; the missing piece was the transition predicate.

This remains domain-neutral. The reducer consumes typed workflow facts:
material coverage sufficiency, prior/current material-inventory state,
non-custom action count, candidate material paths, whether the candidate plan
has an executable broad surface, and the configured broad-material limit. It
does not classify materials by business role and does not parse user/model
prose. The adapter still projects local facts such as candidate paths and
action shape.

Changes:

- [x] Added `MaterialDiscoveryTransitionInput` and
      `BuildMaterialDiscoveryTransition` in `internal/dataworkflow`.
- [x] Added `DefaultBroadMaterialDiscoveryLimit` next to other reducer
      defaults.
- [x] Changed REPL `dataTaskMaterialDiscoveryFallback` to pass typed facts into
      the reducer transition instead of owning the full trigger logic.
- [x] Kept `BuildMaterialDiscoveryPlan` unchanged so the existing inventory
      action shape and audit semantics are preserved.
- [x] Added reducer regression coverage for transition trigger, already-covered
      workflows, prior inventory, non-custom action plans, narrow path sets, and
      missing executable surface.

Current backlog before real-scenario testing:

- [ ] Move the remaining REPL adapter-owned scheduling predicates behind
      reducer-owned transition inputs. Top-level plan shape, terminal
      required-material scheduling, and material-discovery transition are done;
      broad custom-transform prerequisite guards and field/relation guard
      entrypoints still need the same treatment where they produce hard
      scheduling decisions.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 266: Reducer-Owned Broad Custom Prerequisite Guard

The next IR pass moved the broad `custom_transform` prerequisite guard into
`internal/dataworkflow`. Before this batch, the REPL adapter computed missing
prerequisite paths and also built the hard guard prose when a broad transform
tried to consume materials that had not been covered by prior typed actions or
results.

This is a generic graph-readiness invariant. A broad leaf fallback may only run
when its declared inputs are already covered by the workflow graph. The reducer
does not compute coverage from files or infer business meaning; it consumes the
typed action shape, a boolean broadness signal, and the missing prerequisite
path set projected by the adapter. The result is a typed guard/violation with
action identity and missing input aliases.

Changes:

- [x] Added `BroadCustomPrerequisiteGuardInput` and
      `BroadCustomPrerequisiteGuardResult` in `internal/dataworkflow`.
- [x] Changed REPL workflow action staging to call the reducer guard instead
      of formatting the broad-prerequisite error inline.
- [x] Changed the existing broad-prerequisite string helper into a compatibility
      adapter over the reducer guard.
- [x] Preserved the existing coverage-set calculation in the REPL adapter until
      the broader ArtifactGraph/MaterialGraph reducers own that projection.
- [x] Added reducer regression coverage for missing prerequisites, non-broad
      actions, covered actions, typed actions, and empty-script transforms.

Current backlog before real-scenario testing:

- [ ] Move the remaining REPL adapter-owned scheduling predicates behind
      reducer-owned transition inputs. Top-level plan shape, terminal
      required-material scheduling, material-discovery transition, and broad
      custom prerequisite guard are done; field/relation guard entrypoints still
      need the same treatment where they produce hard scheduling decisions.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 267: Reducer-Owned Field Contract Guard Result

The next IR pass moved missing-field guard result construction into
`internal/dataworkflow`. Before this batch, field contract violations were
already typed, but the REPL adapter still converted those violations into the
hard guard message and `GuardResult`.

This is a generic schema-contract invariant. The adapter may still project
artifact access and compute missing fields because that depends on local
ArtifactGraph visibility. Once it has a typed `WorkflowViolation`, the reducer
owns the final guard result and user/audit message. No user intent, business
role, or model prose is parsed for this decision.

Changes:

- [x] Added `FieldContractGuardInput` and `FieldContractGuardResult` in
      `internal/dataworkflow`.
- [x] Changed REPL missing-field guard construction to call the reducer guard
      result instead of formatting the message locally.
- [x] Kept schema projection, field matching, and candidate artifact ranking in
      existing typed helpers; no duplicate field-contract system was added.
- [x] Added reducer regression coverage proving the guard uses typed violation
      fields, action index, input alias, missing fields, and candidate artifact
      hints.

Current backlog before real-scenario testing:

- [ ] Move the remaining REPL adapter-owned scheduling predicates behind
      reducer-owned transition inputs. Plan shape, terminal required-material
      scheduling, material discovery, broad custom prerequisites, and field
      contract guard result are done; remaining relation-specific entrypoints
      and progress signatures still need reducer ownership where they make hard
      scheduling decisions.
- [ ] Add a domain-neutral `mapping_candidate` typed action for ambiguous
      source/reference matching. Existing normalization/enrichment can proceed
      without it, but complex multi-file relation tasks still spend extra model
      turns when candidate matching is not explicit.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 268: Generic Progress Signatures

The next IR pass extended data workflow progress events beyond relation-family
no-progress. Before this batch, `ProgressEvent` only carried actions,
result/error presence, contribution count, and reconcile presence. That made
relation materialization convergence too coarse: a join/enrich/apply batch that
changed schema or row shape could still look identical to an unproductive loop
until contribution or reconcile records appeared.

The generalized invariant is that convergence checks should compare typed
structural movement, not model prose and not business-specific fields. A
progress event now carries artifact count, aggregate row count, artifact field
set, decision/rule/entity/contribution ledger counts, reconcile presence,
answer presence, and optional stage. Relation no-progress counts only repeated
relation events with the same generic signature; a field/row/ledger/stage
change breaks the repeated-no-progress streak.

Changes:

- [x] Extended `dataworkflow.ProgressEvent` with generic artifact, schema,
      ledger, answer, and stage fields.
- [x] Added `ProgressSignature` in `internal/dataworkflow`.
- [x] Updated `RecentRelationNoProgressCount` to stop when a recent relation
      result has a different structural progress signature.
- [x] Updated the REPL adapter to populate progress events from `dataquery`
      results, including nested artifact fields and row counts.
- [x] Added reducer regression coverage proving schema/field progress prevents
      a repeated relation no-progress classification.

Current backlog before real-scenario testing:

- [ ] Move remaining relation-specific hard guard entrypoints behind
      reducer-owned transition inputs where they still produce scheduling
      decisions from local wrappers.
- [ ] Finish migrating relation-specific scaffold builders out of REPL-only
      helpers once `mapping_candidate`, normalize, apply, enrich, and join
      scaffolds are all reducer/projection owned.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 271: Reducer-Owned Relation Guard Results

After moving relation scaffold generation into `dataworkflow`, the next split
was execution-time guard result construction. The REPL adapter still needs to
project local artifact facts, parse action params, and decide whether a guard
condition is present. But once it has a precise condition, the typed violation,
message, repairability, action metadata, and idempotency metadata should be
owned by the workflow reducer package instead of being assembled as adapter
prose.

This batch keeps the hard decision signals structural:

- whether an input is a diagnostic child remains an artifact-class check;
- whether source lineage is incompatible remains a role-lineage check over
  artifact projections;
- whether an apply-resolution edge is idempotent remains a target-field plus
  lineage check;
- the model's free-form text is not parsed for any hard decision.

Changes:

- [x] Added reducer-owned `ActionInputContractGuardResult` for generic
      action/input contract violations.
- [x] Added reducer-owned apply-resolution guard builders for diagnostic
      resolution inputs, incompatible source lineage, and idempotent
      no-progress edges.
- [x] Updated the REPL adapter to pass structured facts into these builders
      instead of formatting those guard results inline.
- [x] Changed apply-resolution lineage messaging to prefer role-specific
      `source_record_paths` before broader `source_paths` when reporting the
      mapping source lineage.
- [x] Added reducer regression coverage for generic action-input guards and
      the apply-resolution relation guard builders.

Current backlog before real-scenario testing:

- [ ] Move the remaining action dependency/scheduling guard result builders
      into `dataworkflow`, especially input availability, intra-batch
      dependency, missing action spec, and upstream ledger requirements.
- [ ] Promote action-level field/lineage diagnostics into ArtifactGraph state
      so the evaluator can select latest compatible artifacts by producer,
      role, fields, row count, and status without asking the planner to infer
      them from prompt samples.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 272: Reducer-Owned Action Dependency Guards

The next IR pass moved the remaining common action dependency and scheduling
guard result builders into `internal/dataworkflow`. Before this batch, the
REPL adapter still formatted hard errors for intra-batch dependency, unavailable
input aliases, input-path cardinality, missing action specs, missing
apply-resolution inputs, and upstream ledger prerequisites. Those are all
workflow-graph contracts, not REPL UI concerns.

The adapter still computes local facts that belong to its layer:

- which previous artifacts are currently available;
- whether an action references a future output in the same batch;
- whether a model-authored action supplied a required param family;
- whether contribution or reconcile producers already exist.

But the reducer now owns the resulting `GuardResult`, typed violation metadata,
repairability, and user/audit message for these generic graph failures.

Changes:

- [x] Added reducer-owned guard builders for intra-batch dependency and
      unavailable action inputs.
- [x] Added reducer-owned input-path cardinality guard builder using
      `ActionInputPathContract`.
- [x] Added reducer-owned missing action spec guard builders for generic typed
      actions such as derive/extract/group/expand/filter.
- [x] Added reducer-owned missing apply-resolution input and upstream ledger
      prerequisite guard builders.
- [x] Updated the REPL adapter to keep fact projection local while delegating
      hard guard result/message construction to `dataworkflow`.
- [x] Added reducer regression coverage for these guard builders.

Current backlog before real-scenario testing:

- [ ] Promote action-level field, lineage, row-count, and status diagnostics
      into ArtifactGraph state so evaluator/continuation decisions can consume
      durable graph facts instead of compact prompt samples.
- [ ] Add a reducer-owned workflow decision input for zero-match filters,
      zero-eligible qualification, and unmatched resolution diagnostics; the
      adapter may still project result rows, but the evaluator should receive
      typed violation objects rather than text-only summaries.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 273: Reducer-Owned Empty-Set Diagnostic Guards

The next IR pass moved another family of hard guard results out of the REPL
adapter: empty-set continuation diagnostics. The adapter still derives these
facts from deterministic result artifacts and workflow state, but the guard
result itself is now reducer-owned.

This covers three generic failure classes:

- `filter_records` produced zero rows, but downstream contribution/reconcile
  stages still require non-empty candidates;
- `apply_entity_resolutions` produced a base artifact whose target canonical
  fields are all unmatched while downstream computation still depends on those
  fields;
- `qualify_records` produced zero eligible rows while contribution/reconcile
  work remains.

These are not business rules. They are typed graph convergence signals: a
downstream action should not keep consuming a structurally empty candidate set
when the workflow still requires ledgers or reconciliation.

Changes:

- [x] Added reducer-owned guard builders for zero-match filter artifacts,
      all-unmatched resolution artifacts, and zero-eligible qualification
      artifacts.
- [x] Updated REPL diagnostic guards to pass structured counts, artifact ids,
      field lists, source/base paths, reasons, and repair hints into
      `dataworkflow`.
- [x] Kept row/count/status extraction in the adapter where artifact result
      projection currently lives.
- [x] Added reducer regression coverage for the three empty-set diagnostic
      guard builders.

Current backlog before real-scenario testing:

- [ ] Promote the artifact result diagnostics that feed these guards into a
      durable `ArtifactGraph` state object so evaluator prompts receive typed
      zero-match/unmatched/zero-eligible signals without scanning recent
      record summaries.
- [ ] Add action lineage summaries to workflow state so planner/evaluator can
      choose the latest compatible artifact by producer, role, fields, row
      count, and status.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 269: Domain-Neutral Mapping Candidate Action

The next architecture pass split a relation-stage responsibility that was still
too overloaded. `normalize_entities` previously had to discover candidate
source/reference matches, decide a reusable mapping ledger, and sometimes
repair source/reference direction. `enrich_records` then had to apply lookup
values and also emit mapping evidence. That kept the workflow reliant on
guards around bad behavior instead of giving the DAG a small typed node for the
uncertain part.

The generic invariant is:

- candidate discovery is not the same as mapping decision;
- candidate rows are ordinary generated artifacts, not final entity-resolution
  ledger records;
- the model chooses the task semantics and fields, while the system reads the
  declared source/reference record sets, validates field contracts, and emits
  objective candidate/evidence/ambiguity rows;
- downstream normalization/application remains a later typed action.

This is not tied to any business domain. It applies to names, ids, labels,
categories, accounts, devices, locations, text terms, lookup values, or any
other source/reference relation where a model needs objective candidate
coverage before choosing the next step.

Changes:

- [x] Added first-class `mapping_candidate` as a `DataActionKind`.
- [x] Added a deterministic action runner that consumes a source record set and
      a reference record set, validates declared source/reference/canonical
      fields, and materializes candidate rows with match status, candidate
      counts, evidence, and source/reference lineage.
- [x] Kept candidate rows out of `result.entity_resolutions` so this diagnostic
      action cannot accidentally satisfy `entity_resolution_required`.
- [x] Registered the action in workflow capabilities, dependency rank,
      role-path normalization, stage contracts, relation progress checks, and
      generated-artifact schema handling.
- [x] Added reducer-owned mapping-candidate scaffolds from artifact schema
      projections and exposed them through the REPL adapter.
- [x] Updated planner schema and prompt guidance to teach the action as a
      candidate/evidence step, not as a business mapping decision.
- [x] Added regression coverage for runner behavior, input-path contracts, and
      concrete scaffold conversion.

Current backlog before real-scenario testing:

- [ ] Move remaining relation-specific hard guard entrypoints behind
      reducer-owned transition inputs where they still produce scheduling
      decisions from local wrappers.
- [ ] Finish migrating relation-specific scaffold builders out of REPL-only
      helpers; `mapping_candidate` scaffolds are now reducer-owned, but older
      local helper functions for normalize/enrich/join/apply still need
      removal or replacement.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 270: Reducer-Owned Relation Scaffold Compatibility

The next IR pass removed another split-brain path. Production planning already
used `internal/dataworkflow` scaffold builders, but several old REPL-only
relation helpers still carried compatibility checks and tests. That meant a
test could pass against a path that the runtime no longer used, while the
production `ApplyResolutionScaffolds` builder still lacked some of the old
ArtifactGraph constraints.

This is a generic graph-contract issue, not a data-domain rule. Automatic
relation scaffolds should be generated from typed artifact projections only
when the graph edge is structurally safe:

- workflow-wide ledger handles and diagnostic children are not executable
  relation inputs;
- source/reference lineage must come from role-specific projection fields such
  as `source_record_paths` and `reference_paths` when available, not from a
  positional guess over mixed `source_paths`;
- applying the same resolution ledger to an already-resolved base artifact is
  an idempotent no-progress edge and should not be suggested again;
- existing canonical/id fields on a base artifact may be used only with a
  structurally compatible reference artifact.

Changes:

- [x] Moved apply-resolution scaffold compatibility into
      `dataworkflow.ApplyResolutionScaffolds`.
- [x] Skipped workflow-ledger handles and diagnostic children during automatic
      apply-resolution scaffold generation.
- [x] Stopped treating prior `apply_entity_resolutions` output artifacts as
      reusable resolution ledgers.
- [x] Added role-aware lineage compatibility using `source_record_paths` first,
      with conservative handling when only ambiguous mixed `source_paths` are
      available.
- [x] Skipped scaffold edges that would reapply a resolution ledger already
      present in the base artifact lineage and fields.
- [x] Added generic existing-ID/reference verification hints to reducer-owned
      apply-resolution scaffolds.
- [x] Removed the unused REPL-only relation scaffold helpers for apply,
      normalize, enrich, and join so the runtime and tests do not maintain two
      relation-scaffold systems.
- [x] Migrated regression coverage to `internal/dataworkflow` for diagnostic
      children, workflow ledger handles, repeated application, source-lineage
      compatibility, role-aware lineage ordering, and existing-ID reference
      specs.

Current backlog before real-scenario testing:

- [ ] Move remaining relation-specific hard guard entrypoints behind
      reducer-owned transition inputs where they still produce scheduling
      decisions from local wrappers. Scaffold generation is now reducer-owned;
      execution-time field/lineage guard result construction still has REPL
      adapter pieces that should be reduced to typed workflow inputs.
- [ ] Add ArtifactGraph lineage/action summaries to evaluator state so later
      actions can select latest compatible artifacts by producer, role, fields,
      and row shape instead of relying on aliases alone.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 274: Live ArtifactGraph State For Evaluator Context

The next IR pass promoted generated-artifact state from compact prompt samples
into a live workflow-state graph. Before this batch, terminal/checkpoint audits
could persist an artifact graph, but continuation and evaluator prompts still
mostly consumed `artifact_availability`: a flat, bounded access catalog. That
made later planner/evaluator turns depend on whichever artifacts fit in the
latest compact sample, even though the runtime already had objective artifact
facts: aliases, fields, row counts, producer kind, source/reference/evidence
lineage, executable record usability, and runner diagnostics.

This is a generic data-workflow issue. It is not tied to any domain or material
type. Any multi-step data task can fail to converge if the next action chooses
inputs from a lossy recent sample rather than from durable graph state.

Changes:

- [x] Added `ArtifactGraphState` in `internal/dataworkflow` with nodes,
      node_count, truncation status, alias index, and executable record aliases.
- [x] Projected each artifact node from existing `ArtifactSchemaProjection`
      without adding business-role classification: id, kind, producer kind,
      node class, aliases, shape, fields, row count, role-specific lineage, and
      structural diagnostics from runner metadata.
- [x] Added deterministic graph construction from newest-first artifact
      projections, preserving the existing alias de-duplication and record-input
      usability contracts.
- [x] Exposed `workflow_state_json.artifact_graph` in live data workflow state
      alongside `action_graph`; kept `artifact_availability` as a compact
      prompt/access view rather than the hard state boundary.
- [x] Reused the live artifact graph in terminal and checkpoint journal
      snapshots, removing the separate journal-only projection path.
- [x] Upgraded per-round artifact graph audit files from a projection array to
      the same graph state object.
- [x] Updated planner/evaluator guidance to prefer `artifact_graph` for
      cross-round generated-artifact selection and to treat
      `artifact_availability` as a compact view.
- [x] Added regression coverage for graph aliases, executable record inputs,
      row counts, lineage roles, diagnostics, truncation, live workflow state,
      continuation prompt visibility, and journal JSON shape.

Current backlog before real-scenario testing:

- [ ] Move zero-match/unmatched/zero-eligible fact extraction into reducer-owned
      artifact graph diagnostics where feasible, so the adapter does less
      artifact scanning before emitting typed violations.
- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 275: Reducer-Owned Empty-Set Artifact Diagnostics

The next IR pass moved the remaining artifact scanning for empty-set
diagnostics out of the REPL adapter. Before this batch, the guard result itself
was reducer-owned, but REPL still walked generated artifacts to detect:

- zero-row `filter_records` outputs after non-empty inputs;
- all-unmatched `apply_entity_resolutions` outputs;
- zero-eligible `qualify_records` outputs.

Those facts are not UI concerns. They are objective artifact diagnostics that
belong beside artifact schema projection and ArtifactGraph state. The REPL may
still decide whether downstream contribution/reconcile stages make the issue
currently relevant, but the extraction of issue shape, aliases, row counts,
lineage fields, diagnostic snippets, and repair hints now lives in
`internal/dataworkflow`.

Changes:

- [x] Added reducer-owned artifact diagnostic issue types for zero-match
      filters, unmatched resolutions, and zero-eligible qualification outputs.
- [x] Added reducer-owned artifact/batch scanners that preserve newest-first
      ordering and suppress stale zero-match aliases when a newer positive
      filter artifact exists.
- [x] Changed REPL workflow state assembly to pass only round+artifact batches
      into `dataworkflow`, while retaining the stage-gating condition in the
      adapter.
- [x] Kept emitted `workflow_state_json` field shapes stable while moving the
      type ownership to `dataworkflow`.
- [x] Added reducer regression coverage for projection, truncation, stale
      suppression, unmatched target counts, and zero-eligible diagnostics.
- [x] Re-ran REPL workflow-state and staging-guard regression coverage.

Current backlog before real-scenario testing:

- [ ] Extend progress signatures beyond relation-family no-progress to include
      field-set deltas, row-count deltas, schema deltas, ledger deltas, and
      stage movement for all typed action families.
- [ ] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 276: Generic Progress Window IR

The next IR pass made structural progress visible for all typed data action
families without immediately widening hard no-progress gates. Before this
batch, `ProgressSignature` already included fields, rows, ledger counts, and
answer/reconcile flags, but the runtime used it mainly for relation-family
no-progress detection. Evaluator and continuation prompts still had to infer
whether the graph was moving from recent records and process text.

The generic invariant is:

- hard gates should remain conservative until their signal is precise enough;
- evaluator/planner state should still receive objective progress facts for
  every action family;
- a repeated artifact/ledger/stage signature is a structural signal, not a
  business conclusion;
- real progress means some typed dimension changed: fields, row counts,
  artifact count, ledger counts, reconcile/answer presence, or stage state.

Changes:

- [x] Added reducer-owned `ProgressWindow`, `ProgressFrame`, and
      `ProgressDelta` IR.
- [x] `BuildProgressWindow` now reports latest frame, recent frames, latest
      signature, repeated signature count, row/artifact deltas, added/removed
      fields, ledger deltas, reconcile changes, and answer-presence changes.
- [x] Kept existing relation-family hard no-progress guard unchanged; the new
      generic window is advisory/evaluator state until broader hard policy is
      proven safe.
- [x] Exposed `workflow_state_json.progress_signatures` in live data workflow
      state.
- [x] Persisted the progress window in terminal and checkpoint workflow
      journals.
- [x] Updated planner/evaluator guidance to use progress signatures for
      choosing a different typed action when repeated signatures grow without
      ledger/reconcile/final projection progress.
- [x] Added reducer and REPL regression coverage for field, row, artifact,
      ledger, and repeated-signature projection.

Current backlog before real-scenario testing:

- [x] Add storage-neutral workflow journal/checkpoint persistence only after
      the in-memory IR settles; do not run real-scenario testing before the
      deterministic in-memory graph contracts above are complete. Closed by
      Batch 277.
- [ ] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties.

### Batch 277: Storage-Neutral Journal And Explicit Resume Boundary

The checkpoint backlog was audited after ActionGraph, ArtifactGraph,
reducer-owned diagnostics, and ProgressWindow all became live workflow-state
IR. The storage boundary was already mostly present: data terminal and
checkpoint files were written under `.codrax/data-audit/`, and CLI resume was
explicit through `--data-resume`. The remaining gap was status clarity and
regression coverage: the design still listed journal/checkpoint persistence as
unfinished even though the code path had become a storage-neutral
`WorkflowJournal`.

This is a generic runtime contract, not a data-domain rule:

- persisted workflow state must be typed IR, not reconstructed from logs,
  prompts, model prose, or UI text;
- terminal and checkpoint audit files must carry the same durable graph
  families: action graph, artifact graph, progress window, typed violations,
  decision, process events, and resume payload;
- resume must remain opt-in and data-mode scoped, so a later CLI run cannot
  accidentally continue an old task;
- logs and UI may reference audit paths, but hard resume state comes only from
  the journal `resume` payload.

Changes:

- [x] Confirmed the storage-neutral `WorkflowJournal` includes action events,
      `ActionGraph`, `ArtifactGraphState`, `ProgressWindow`, typed workflow
      violations, workflow decision, process events, and a raw typed resume
      payload.
- [x] Confirmed terminal audit and checkpoint writers use the same journal
      schema and write full JSON artifacts under `.codrax/data-audit/`.
- [x] Confirmed CLI resume is explicit (`--data-resume`) and rejected outside
      `--mode=data`.
- [x] Confirmed resume loads only `journal.resume.records/current_plan/
      deferred_plan`, then selects the next typed batch from deferred actions,
      terminal fallback, next-stage fallback, or the continuation planner.
- [x] Extended regression coverage so terminal and checkpoint journal tests
      assert graph state, progress state, process events, and resume payload
      presence.
- [x] Kept the design domain-neutral: no material-role names, business-field
      heuristics, or prompt/prose parsing were added.

Current backlog before real-scenario testing:

- [x] Add multi-run real-scenario gates and CI/status checks after the single
      latest-binary real scenario passes. Do not use those gates to fit one
      business case; they should check generic output, ledger, reconcile, audit,
      and volatility properties. Closed by Batch 278.

### Batch 278: Multi-Run Real-Scenario Gate Contract

The last deterministic pre-scenario backlog item was the release gate itself.
The repo already had an opt-in `eval/data_real_scenario_gate.sh`, but it was a
single-run check. That could prove one answer shape, but it could not catch
answer volatility across repeated runs and did not inspect the terminal audit
JSON artifact itself.

The gate is intentionally generic:

- it runs only when the operator supplies a local scenario directory and a
  request; private customer-like materials do not enter the repo;
- stdout remains the final answer only, while run paths, progress, and
  diagnostics go to stderr;
- the expected-answer check is optional, but if set it compares normalized
  final stdout;
- multiple runs compare final stdout for volatility without interpreting
  business fields;
- audit checks require typed workflow state families, not business-specific
  material names: action graph, artifact graph, progress window, decision,
  process events, and resume payload;
- contribution and reconcile signals are still configurable gate requirements
  because this gate targets complex calculation workflows, while simpler data
  tasks can run ordinary eval cases.

Changes:

- [x] Added `DATA_REAL_SCENARIO_RUNS` support with stable-answer comparison.
- [x] Added per-run summary TSV under `.codrax/real-scenario-gates/`.
- [x] Added terminal audit file existence and JSON-key checks for action graph,
      artifact graph, progress, decision, process events, and resume payload.
- [x] Kept ledger/reconcile checks on by default while making them explicit
      environment-controlled requirements.
- [x] Added `make eval-data-real` as the local/CI status-check entrypoint.
- [x] Added shell regression coverage with a fake codrax binary for stable
      multi-run PASS and volatile-answer FAIL.

Current backlog before real-scenario testing:

- [x] No deterministic architecture backlog item remains open in the current
      tail ledger. Historical "Remaining architecture items" above are kept as
      chronological audit entries; their current-state closure is reflected in
      the later batches. Real-scenario testing can now be used to validate
      behavior, not to discover known unimplemented IR contracts.

### Batch 279: Transitive Artifact Lineage And Failed Edge Lifecycle

The first real-scenario gate after Batch 278 exposed a remaining IR gap before
the workflow reached contribution calculation. The graph had correctly
materialized entity-resolution ledgers and extracted those ledgers into
record-shaped artifacts. The next typed action,
`apply_entity_resolutions`, was rejected by the hard lineage guard because the
extracted record artifact reported its immediate source as the intermediate
ledger alias, not the original record-set lineage behind that ledger.

This is not specific to any material type, business domain, or field name. Any
adaptive data workflow can produce a mapping/reference ledger, extract it into
a record artifact, and then apply it to a compatible base record set. A hard
gate must reason over the ArtifactGraph lineage closure, not over only the
nearest artifact alias. The same run also showed an ActionDAG lifecycle issue:
an idempotent action edge that had already failed could still appear as ready
or deferred in the live graph projection, which makes the workflow look like it
can simply retry the same edge.

Generic invariants:

- ArtifactGraph lineage is transitive. If artifact B is extracted from artifact
  A, B inherits A's source-record and reference lineage for compatibility
  checks.
- Resolution-source selection must distinguish source and reference roles even
  when raw `source_paths` ordering is unstable or `source_record_paths` is
  polluted by reference lineage. Reference paths are removed first; line
  locators such as `file:12` compare through their material root.
- Hard lineage rejection is allowed only when the source candidates are precise
  and none intersect the base record lineage. Incomplete lineage should not be
  promoted to a hard reject.
- ActionGraph ready/deferred projections must suppress an exact idempotency
  edge that is already failed or blocked. Repair must change the edge, unblock
  it through typed evidence, or move to a different ready node.

Changes:

- [x] Added transitive lineage closure to `ProjectArtifactSchemasNewestFirst`.
- [x] Added shared ArtifactGraph helpers for resolution source candidates,
      lineage-root comparison, resolution/base compatibility, and compact
      lineage summaries.
- [x] Rewired `apply_entity_resolutions` scaffold and REPL staging guards to
      use the shared ArtifactGraph lineage helpers instead of local duplicate
      logic.
- [x] Added line-locator root comparison for structured provenance such as
      `source.csv:42`.
- [x] Updated ActionGraph reduction so failed/blocked idempotent edges do not
      reappear as ready/deferred nodes in workflow state.
- [x] Added regression coverage for transitive ledger extraction lineage,
      non-reference resolution-source selection, and failed-edge suppression.

Current backlog before real-scenario testing:

- [ ] Run the latest binary through the real-scenario gate again. If it still
      fails, treat the next failure as an IR gap only when the terminal journal
      proves a typed graph/state contract is missing; otherwise classify it as
      action semantics, multimodal material extraction, or scenario-data
      limitation before changing architecture.

### Batch 280: Typed Action Contract Alignment And Deferred Readiness

The next real-scenario audit progressed past transitive lineage and into
field extraction, filtering, normalization, enrichment, and deferred typed
actions. Two generic IR gaps remained before another full scenario gate is
useful:

- the planner could emit structurally clear join keys as
  `left_key_fields`/`right_key_fields`, but the runner and REPL field-contract
  guard only treated `left_fields`/`right_fields` as the canonical executable
  contract;
- deferred DAG readiness checked input availability before applying the same
  single-record-set field-contract narrowing used by execution preparation,
  so a deferred action with a unique compatible record artifact could be
  blocked or sent through a multi-input guard even though the executable
  artifact was objectively identifiable.

These are not domain issues. They are cross-layer typed action contract
issues. A workflow should not depend on whether the model used one accepted
field-name alias or another, and the ready/deferred state machine must use the
same structural input-contract logic as the executor.

Generic invariants:

- action parameter aliases that represent the same typed contract must be
  normalized consistently in the planner schema, staging guard, and runner;
- deferred dispatch must operate on the executable action IR, including
  deterministic input narrowing by artifact field coverage;
- ambiguous multi-input single-record actions remain blocked. Narrowing is
  allowed only when exactly one available artifact satisfies all referenced
  fields;
- the hard signal is artifact schema/field coverage, not planner prose,
  material names, or business-domain labels.

Changes:

- [x] Taught `join_records` runner and REPL field-contract guards to accept
      key-field alias families such as `left_key_fields`, `right_key_fields`,
      `base_key_fields`, `lookup_key_fields`, and `reference_key_fields`.
- [x] Factored single-record-set input narrowing into a shared helper that
      consumes current `ArtifactGraph`/artifact availability and action field
      refs.
- [x] Applied the same narrowing before deferred action readiness checks, so a
      deferred typed action can dispatch the executable narrowed action instead
      of being judged on its broader candidate input list.
- [x] Kept ambiguous multi-input single-record actions blocked when no unique
      schema-compatible artifact exists.
- [x] Added regression coverage for join key alias execution and deferred
      field-contract narrowing.

Current backlog before real-scenario testing:

- [ ] Re-run the latest binary through the real-scenario gate after this batch
      passes full tests. If it still fails, classify the next terminal journal
      gap by typed IR family first: action contract, artifact graph/schema,
      ledger graph/reconcile, multimodal extraction, or scenario-data
      limitation.

### Batch 282: File Provenance Fields For Text Record Actions

The next real-scenario run advanced through enrichment, filtering, query join,
and invoice-text extraction planning. The failure moved to a lower-level
record contract: `extract_fields` on concrete text files could read `text`, but
single-file text records did not expose stable file provenance fields such as
`file_name` and `file_path`. Directory text reads already exposed those fields,
so the contract depended on whether the planner supplied a directory or a list
of concrete files. The model then tried to repair by falling back to a broad
script, which the typed workflow correctly disabled.

This is a generic data-runner contract gap. Any data task may need to extract
identifiers, timestamps, source labels, or grouping keys from file names or
paths while extracting fields from file content. That should be available as a
typed provenance field across CSV, JSON, JSONL, text, generated artifacts, and
directory children.

Generic invariants:

- every action record has source provenance virtual fields independent of its
  storage format;
- `file_name` and `file_path` are structural provenance, not business fields;
- `extract_fields` may use those fields the same way it uses `_source_path` or
  `source_locator`;
- this does not parse file names for business meaning in system code. The
  model supplies the extraction pattern, and the runner only exposes objective
  provenance.

Changes:

- [x] Added `file_path` and `file_name` to `actionRecordVirtualFields`.
- [x] Added both fields to `markKnownActionVirtualFields` so schema validation
      accepts model-declared extraction specs over file provenance.
- [x] Added regression coverage proving `extract_fields` can extract a field
      from `file_name` while extracting another field from text content.

Current backlog before real-scenario testing:

- [ ] Re-run the latest binary through the real-scenario gate after this batch
      passes full tests. If it still fails, classify the next terminal journal
      gap by typed IR family first: action contract, artifact graph/schema,
      ledger graph/reconcile, multimodal extraction, or scenario-data
      limitation.

### Batch 281: Non-Regressive Ledger Side-Gaps

The next real-scenario run showed that the workflow could now pass lineage and
deferred input narrowing, but it exposed a deeper state-machine problem. The
graph had already materialized normalized/enriched record artifacts and a
deferred `qualify_records` node. Because no `rule_coverage` records had been
emitted yet, the linear `NextStage` reducer reset to `derive_rules`; the
deferred queue then discarded the ready qualification node as "not allowed in
current workflow stage". The planner repaired by deriving rules, but then
started redoing entity-resolution work that had already succeeded.

This is a generic workflow-IR issue. Rule coverage, decision records,
contribution ledgers, and reconcile reports are validation ledgers; missing
one should remain visible, but it must not erase already materialized ActionDAG
progress. A late ledger side-gap should be repairable without making ready
downstream graph nodes illegal.

Generic invariants:

- material coverage remains the hard first gate;
- missing rule coverage is the next stage only before downstream graph
  progress exists, or after contribution/reconcile work is complete and the
  workflow is about to answer;
- once entity/contribution preparation has materialized, missing rule coverage
  becomes a side ledger gap: `derive_rules` stays allowed, but it no longer
  monopolizes `allowed_next_actions`;
- deferred dispatch filters on the facts-aware allowed action contract, so a
  ready typed node is not discarded merely because an audit ledger also needs
  catch-up;
- final completion still requires all requested ledgers. This change affects
  scheduling, not validation strictness.

Changes:

- [x] Added `StageFacts.HasPostRuleProgress()` and made `NextStage` avoid
      regressing to `derive_rules` after post-rule graph progress exists.
- [x] Kept a catch-up `derive_rules` stage before final answer projection when
      rule coverage is still missing after downstream ledgers are complete.
- [x] Added facts-aware `AllowedNextActionContractsForFacts`, which prepends a
      `derive_rules` side action while preserving the current graph-stage
      actions.
- [x] Updated data workflow state to consume the facts-aware contract instead
      of stage-only contracts.
- [x] Added reducer and REPL regression coverage proving a deferred
      `qualify_records` node remains dispatchable while rule coverage is still
      a missing side ledger.

Current backlog before real-scenario testing:

- [ ] Re-run the latest binary through the real-scenario gate after this batch
      passes full tests. If it still fails, classify the next terminal journal
      gap by typed IR family first: action contract, artifact graph/schema,
      ledger graph/reconcile, multimodal extraction, or scenario-data
      limitation.

### Batch 283: Deferred Queue Lifecycle As Typed IR

Architecture audit found that the adaptive data lane was drifting back toward
"test, observe, patch a guard" because some graph lifecycle choices were still
owned by REPL/CLI loops instead of the workflow IR. The clearest example was
the deferred action queue: after each successful batch both CLI and REPL tried
to pop a ready deferred rank, but if the first deferred action was blocked they
immediately discarded the whole queue. That is a control-flow decision, not a
UI decision. It should be made by typed ActionDAG lifecycle state.

This is domain-neutral. Any multi-step data workflow can emit useful future
typed actions before all dependencies are materialized: table joins, extracted
text fields, reference enrichment, row filtering, contributions, reconcile,
and final projection. A blocked future node should remain in the graph when it
is waiting for a future artifact, allowed-stage transition, or field
materialization. It should be discarded only when typed admission says the
node itself is structurally invalid.

Generic invariants:

- deferred queue status carries typed reason codes, not only prose;
- REPL/CLI render and persist lifecycle decisions, but do not decide them;
- `retain` means "keep the typed graph suffix for later readiness checks"; it
  does not bypass admission, staging guards, field-contract checks, or runner
  validation;
- `discard` is reserved for typed admission rejection or unrecoverable queue
  shape, not for ordinary dependency wait states;
- hard branching reads typed enum fields such as `reason_code`, never model
  prose or localized UI text.

Changes:

- [x] Added deferred blocked reason codes for unavailable inputs, field
      contract waits, stage-not-allowed waits, admission rejection, empty
      queues, and missing allowed actions.
- [x] Added `DecideDeferredQueueLifecycle` in `internal/dataworkflow` so queue
      retain/discard behavior is centralized in the IR package.
- [x] Rewired CLI and REPL post-batch loops to consume the typed lifecycle
      decision instead of always discarding blocked deferred queues.
- [x] Kept retained queues visible through low-noise workflow progress and log
      records without appending a synthetic failure record.
- [x] Added unit coverage proving queue retention is keyed by typed reason code
      and that REPL candidate construction emits typed blocked codes.

P0 IR closure backlog before the next real-scenario gate:

- [ ] Extract plan admission into a single `ActionDAGAdmission` entrypoint
      used by initial, continuation, repair, completion-repair, and deferred
      dispatch paths. REPL/CLI should consume accepted/rejected/deferred IR
      decisions rather than run local preflight closures.
- [ ] Move deferred queue storage into live `ActionGraph`/workflow journal
      instead of an outer REPL/CLI variable. The variable can remain as a
      compatibility adapter only until the reducer owns enqueue/pop/retain.
- [x] Front-load `ArtifactSchemaProjection` into admission so missing fields,
      incompatible shapes, stale aliases, and metadata-vs-record confusion are
      typed `WorkflowViolation` objects before execution or model repair.
- [x] Promote LedgerGraph from count projection to contract graph: rule,
      decision, entity resolution, contribution, reconcile, and final
      projection dependencies now expose required/present/status,
      prerequisites, producer actions, first missing ledger, and next stage.
- [ ] Move terminal completion gates to consume LedgerGraph dependencies
      directly instead of re-deriving missing ledgers in REPL helper code.

P1 follow-up after P0 closure:

- [ ] Add domain-neutral typed actions for evidence attachment, coverage/value
      diff, mapping candidates, and projection validation where existing
      typed actions cannot express the atomic step cleanly.
- [ ] Feed reducer/process events into CLI/REPL UX so users see business goal,
      current action purpose, blockage reason, and next step from structured
      plan fields, with internal counts relegated to audit detail.
- [ ] Upgrade real-scenario gates from exploratory probes to verification:
      multi-run stability, terminal journal assertions, stdout strictness,
      stderr audit visibility, and expected-answer checks.

### Batch 284: ActionDAG Admission Reducer Skeleton

The next P0 step extracted the admission loop itself from the REPL helper
shape. Before this batch, `dataTaskPreflightWorkflowPlan` owned the sequence:
protect plan, split a multi-rank action batch, run staging guard, try a
deterministic fallback, merge deferred remainders, and eventually return a
blocked plan. That state machine was hidden inside the REPL package even
though CLI and REPL both consume it.

This batch does not move every guard yet. The existing guard and deterministic
fallback functions remain in place and are passed as callbacks. The important
architecture change is that the control-flow contract is now owned by
`internal/dataworkflow`: REPL becomes an adapter that supplies current records
and existing guards, while the IR package owns admission decision shape and
rewrite lifecycle.

Generic invariants:

- action admission returns one typed decision shape: accepted plan, original
  plan, deferred remainder, first guard, final guard, reason, and rewritten
  flag;
- admission control flow is deterministic and independent of REPL/CLI UI;
- guard results are carried as typed `GuardResult` alongside legacy error text
  so callers can keep compatibility while later batches stop parsing prose;
- deterministic fallback may rewrite structure, but execution still must pass
  guard validation after the rewrite;
- remainder merging is storage-neutral and uses action graph suffix semantics,
  not REPL-local queue policy.

Changes:

- [x] Added `ActionDAGAdmissionInput` and `ActionDAGAdmissionDecision` in
      `internal/dataworkflow`.
- [x] Added `AdmitActionDAGPlan`, covering protect, prefix fallback, guard,
      deterministic fallback, max-rewrite budget, final blocked guard, reason
      accumulation, and remainder merge.
- [x] Converted REPL `dataTaskPreflightWorkflowPlan` into a thin adapter over
      the shared admission reducer.
- [x] Removed REPL-local preflight remainder merge/reason accumulation logic.
- [x] Added reducer-level unit coverage for split admission, deterministic
      fallback, and typed final guard behavior.
- [x] Kept existing REPL preflight regression tests passing, proving behavior
      parity for the current adapter.

Remaining P0 admission work:

- [ ] Wire completion-repair plans and deferred dispatch through the same
      `ActionDAGAdmissionDecision` shape instead of their current direct
      protect/audit paths.
- [ ] Replace the remaining string-only staging guard callbacks with typed
      `GuardResult` producers so `FinalGuard.Code` preserves precise failure
      class without wrapping as a generic admission error.
- [ ] Feed accepted/deferred/rejected admission decisions into the live
      `ActionGraph` journal so deferred storage can move out of REPL/CLI outer
      variables.

### Batch 285: Typed Guard Results Through Admission

The admission reducer added in Batch 284 can carry typed `GuardResult`, but
the first adapter still fed it a string-only staging error and wrapped that as
`action_dag_admission`. That preserved behavior, but it hid precise failure
classes from the reducer. The codebase already had typed guard producers for
many precise structural checks: missing action inputs, unavailable input
aliases, field-contract violations, intra-batch dependencies, missing specs,
and upstream ledger gaps. The admission adapter should consume those existing
typed results directly.

This is a cross-cutting IR fix, not a task-specific guard. It means future
admission/reducer logic can branch on typed guard codes without parsing
localized UI text or model-facing prose.

Generic invariants:

- admission must preserve the most precise guard code available at the
  boundary;
- legacy `GuardErr` strings remain for existing retry hints and UI messages,
  but hard control flow should read `GuardResult.Code` and violations;
- typed guard propagation must not alter existing staging-guard behavior or
  execution permission;
- generic wrappers are acceptable only when no precise guard result exists.

Changes:

- [x] Rewired `dataTaskPreflightWorkflowPlan` to pass
      `dataTaskWorkflowStagingGuardResult` directly into
      `AdmitActionDAGPlan`.
- [x] Preserved fallback compatibility by continuing to pass
      `guard.ErrorText()` into existing deterministic fallback functions.
- [x] Added regression coverage that a rejected preflight candidate preserves
      the precise `missing_action_inputs` final guard code.

Remaining typed-guard work:

- [ ] Convert the top-level string-only staging checks, such as batch-size,
      script-placement, custom-transform shape, and numeric-constant reuse,
      into typed `GuardResult` producers.
- [ ] Feed admission guard codes into ActionGraph blocked nodes and workflow
      journal entries instead of only storing textual `Err`.

### Batch 286: Admission Decisions In Workflow Journal

The previous batches made admission typed, but terminal/checkpoint journals
still mainly showed action events, artifacts, progress, and batch results. A
reader could inspect plan artifacts to infer that a plan had been split or
rewritten, but the workflow journal did not carry a first-class admission
process event. That keeps the system too close to REPL/CLI-local control flow:
the graph decision is visible in logs, but not in the durable IR audit.

This batch moves admission decisions into the durable journal as compact
summaries. It does not store another full copy of every plan in the event; full
plan/action artifacts remain in `data-audit`. The journal gets enough typed
state for reducers, resume tooling, and UX to understand whether a batch was
accepted, rewritten, or rejected, and which guard code drove the decision.

Generic invariants:

- admission decisions are workflow process events, not UI-only messages;
- journal events carry compact structural summaries: status, rewritten flag,
  plan action count, remainder action count, guard code, final guard code, and
  reason;
- full plans/scripts/actions remain in existing audit artifacts to avoid
  bloating every process event;
- execution behavior is unchanged. Admission events describe decisions already
  made by the reducer; they do not authorize execution.

Changes:

- [x] Added `ActionDAGAdmissionSummary` to workflow journal events.
- [x] Added `BuildAdmissionProcessEvent` and `AdmissionSummary` in
      `internal/dataworkflow`.
- [x] Added an optional admission decision pointer to data workflow records.
- [x] Wired CLI and REPL accepted/rewrite preflight decisions into records
      when the admitted plan is the plan that later executes.
- [x] Emitted admission events before the corresponding batch event in
      workflow journal construction.
- [x] Projected rejected admission `FinalGuard` violations into workflow
      violations and ActionGraph blocked nodes.
- [x] Added regression coverage for journal JSON shape, admission event
      summary, REPL journal event ordering, and rejected-admission blocked
      graph projection.

Remaining P0 journal/action-graph work:

- [ ] Move deferred queue storage from the REPL/CLI outer variable into the
      workflow journal / live ActionGraph state, with the outer variable only
      as a temporary adapter.
- [x] Feed admission guard codes into ActionGraph blocked nodes directly, so a
      rejected admission appears as a blocked graph node even before execution.

### Batch 287: Deferred Queue Snapshot In ActionGraph

The deferred queue still has one remaining architectural split: REPL/CLI hold
the executable deferred `TaskPlan`, while the workflow IR only projected
deferred action nodes. Batch 287 adds the missing structural slot to the graph:
ActionGraph and journal snapshots now carry a compact deferred-queue summary.
This is the first step toward moving queue storage fully into the live
workflow state.

This batch intentionally does not change execution ownership yet. The outer
REPL/CLI queue variable remains the adapter that stores the full executable
plan. The graph now has enough typed state for audits, reducers, and future
resume logic to reason about the queue without scraping UI lines.

Generic invariants:

- ActionGraph can represent both deferred nodes and the deferred queue state
  that owns those nodes;
- checkpoints include ready/blocked counts, first action identity, reason code,
  reason text, and retain/discard lifecycle action;
- ordinary workflow state can project a plan-level queue snapshot without
  recursively recomputing full readiness;
- execution still requires the existing deferred dispatch admission and
  staging guards.

Changes:

- [x] Added `DeferredQueueSnapshot` to `ActionGraph`.
- [x] Added `DeferredQueueSnapshotForPlan` and
      `DeferredQueueSnapshotForStatus` in `internal/dataworkflow`.
- [x] Projected plan-level deferred queue snapshots into workflow state.
- [x] Enriched checkpoint journals with computed deferred status and lifecycle
      decision.
- [x] Added reducer/journal JSON coverage for the new deferred queue snapshot.

Remaining P0 deferred-storage work:

- [x] Store the executable deferred `TaskPlan` in the workflow journal / live
      ActionGraph state so prompt, audit, and resume share the same typed queue
      payload.
- [ ] Let REPL/CLI read and mutate the deferred queue only through a
      reducer-owned queue API; keep the outer variable only as a temporary
      adapter.
- [ ] Make enqueue/pop/retain/discard transitions produce typed ActionGraph
      events instead of mutating an outer variable directly.

### Batch 288: LedgerGraph Dependency Contract

The next P0 gap was the ledger side of the workflow IR. ActionGraph and
ArtifactGraph had already moved toward typed reducer state, but LedgerGraph was
still mostly a count projection. REPL helpers, completion gates, continuation
prompts, and evaluators each inferred missing rule, decision, entity,
contribution, reconcile, or final-answer stages from nearby counters. That is
exactly the kind of duplicated interpretation that makes a system feel like it
is collecting guards instead of converging through one state machine.

This batch promotes LedgerGraph into a domain-neutral dependency contract. It
does not encode business roles such as invoices, contracts, vendors, dates, or
amounts. It only models generic validation ledgers and final projection as
typed nodes with structural status and prerequisites.

Generic invariants:

- ledger nodes expose `required`, `present`, `count`, `status`, `stage`,
  `produces_actions`, `depends_on`, and `missing_prerequisites`;
- statuses are typed: `optional`, `satisfied`, `missing`, or
  `blocked_by_prerequisite`;
- missing material coverage is represented as a structural prerequisite named
  `materials`, not as a business-specific file role;
- downstream ledgers that are already satisfied remain satisfied even when a
  missing upstream audit ledger must be caught up before final projection;
- final projection depends on the required ledger set and exposes the same
  blockage contract as other ledgers;
- prompts may use LedgerGraph as structural guidance, but hard gates still read
  typed reducer fields and not model prose.

Changes:

- [x] Added `LedgerDependency` and extended `LedgerStatus` with structural
      status, stage, producer action kinds, dependencies, and missing
      prerequisites.
- [x] Added `BuildLedgerGraph(StageFacts)` in `internal/dataworkflow`.
- [x] Added first-missing-ledger and next-stage projection to LedgerGraph.
- [x] Exposed `ledger_graph` in `workflow_state_json`.
- [x] Persisted `ledger_graph` in terminal and checkpoint workflow journals.
- [x] Taught continuation/evaluator prompts to use `workflow_state_json.
      ledger_graph` as structural state, without making prompt prose a hard
      gate.
- [x] Added unit coverage for missing prerequisites, satisfied downstream
      ledgers, material-floor blockage, journal JSON, and REPL workflow state.

Remaining P0 LedgerGraph work:

- [x] Move terminal missing-ledger completion gates to consume LedgerGraph
      dependencies directly instead of running separate REPL-local ledger
      checks for required validation ledgers.
- [x] Feed first-missing-ledger and missing-prerequisite summaries into
      `WorkflowDecision` so CLI/REPL UX can show user-relevant blockers without
      repeating internal stage jargon.
- [x] Make deterministic completion/fallback builders consume LedgerGraph for
      missing-ledger repair decisions in the REPL/CLI completion path.
- [x] Split final-answer projection into its own typed OutputProjectionGraph so
      strict output contracts, reference-complete projections, and ordinary
      already-answerable results do not get conflated with validation ledgers.

### Batch 289: LedgerGraph Completion Guard For Missing Ledgers

After Batch 288, the workflow state exposed a typed LedgerGraph, but the
completion gate still reported missing required ledgers by re-running the old
REPL-local interpretation around `ValidateResultAgainstContract`. This batch
keeps dataquery's typed result validation as the source of truth for semantic
ledger correctness, then uses LedgerGraph to explain missing required ledger
dependencies and repair actions.

This is intentionally narrower than a blanket final-answer gate. A real result
may already have a valid answer when no strict output projection is required,
even if the current batch was marked `continue_after`. Conversely, reference
complete output projection has richer structural information than a generic
`final_projection` node. Those output concerns should become a separate
OutputProjectionGraph rather than being forced into validation-ledger logic.

Generic invariants:

- missing validation ledgers are represented by typed LedgerGraph completion
  guards with `missing_workflow_ledger` or `blocked_workflow_ledger`;
- the guard carries producer action hints from the ledger dependency, such as
  `compute_contributions` or `reconcile_artifacts`;
- material coverage errors keep their path-specific diagnostics and are not
  flattened into ledger messages;
- semantic ledger failures, such as empty/invalid ledger records or failed
  reconcile validation, remain dataquery validation errors;
- final-answer projection remains governed by OutputContract/reference
  projection gates until it is promoted into a dedicated output projection IR.

Changes:

- [x] Added `LedgerGraphCompletionGuardResult` in `internal/dataworkflow`.
- [x] Rewired data workflow completion error handling to use the LedgerGraph
      guard only when the typed validation error code is
      `missing_required_ledger`.
- [x] Preserved material coverage diagnostics and reference-complete
      projection diagnostics.
- [x] Added regression coverage for LedgerGraph completion guards and REPL
      completion-gate missing-ledger errors.

Remaining P0 completion/output work:

- [x] Move deterministic completion-repair plan selection for missing ledgers
      to consume LedgerGraph dependencies directly in the REPL/CLI completion
      path instead of dataquery error JSON paths.
- [x] Introduce OutputProjectionGraph for final answer readiness, reference key
      completeness, strict output contract projection, and answer-candidate
      precedence.
- [ ] Feed ledger guard summaries into workflow journal/process events so users
      see the business-relevant next action rather than only a validation
      string.

### Batch 290: LedgerGraph-Driven Completion Repair Plans

Batch 289 made missing required ledgers visible as typed LedgerGraph guards,
but deterministic completion repair still selected plans from legacy
dataquery-error JSON paths. That meant the main data workflow could say
"missing ledger" through the new graph, then choose the repair action through a
separate error-text-derived path. This batch closes that split for the
REPL/CLI completion path.

The migration is deliberately scoped to validation-ledger repair. Final output
projection remains outside LedgerGraph until OutputProjectionGraph exists,
because output readiness has different structure: strict format, reference key
universe, answer-candidate precedence, and projection artifacts.

Generic invariants:

- deterministic missing-ledger repair consumes the first incomplete required
  LedgerGraph dependency;
- a missing rule coverage ledger may produce a `derive_rules` repair plan when
  rule/constraint material or distilled rules are available;
- a missing reconcile ledger may produce a `reconcile_artifacts` repair plan
  only after contribution records already exist;
- the repair builder must not skip an earlier missing contribution ledger in
  order to run reconcile;
- legacy error-text fallback remains only for direct/unmigrated callers; the
  REPL/CLI completion path passes LedgerGraph explicitly.

Changes:

- [x] Added LedgerGraph fields to completion-repair planner inputs.
- [x] Added graph-first deterministic missing-ledger repair selection.
- [x] Rewired REPL/CLI completion repair to pass the completion LedgerGraph.
- [x] Added regression coverage for graph-driven reconcile repair and
      blocked-by-earlier-ledger behavior.

Remaining P0 completion/output work:

- [x] Introduce OutputProjectionGraph and move final-answer readiness out of
      mixed REPL-local output checks for completion-gate decisions.
- [ ] Feed LedgerGraph repair decisions into workflow process events and UX
      summaries.
- [ ] Remove the legacy error-text fallback once all internal callers pass
      typed graph/violation inputs.

### Batch 291: OutputProjectionGraph For Final Answer Readiness

The LedgerGraph work made one boundary explicit: final answer readiness should
not be modeled as another validation ledger. Output projection has different
structure from rule, decision, entity, contribution, and reconcile ledgers:
strict user output contracts, answer-candidate precedence, assemble-answer
artifacts, reference key completeness, and already-answerable freeform results.

This batch introduces OutputProjectionGraph as a separate typed IR. The graph
is domain-neutral; it does not encode any business-specific target fields or
numeric meanings. It only models whether the workflow already has an acceptable
answer, needs an `assemble_answer` projection, or must complete a declared
reference universe before final output.

Generic invariants:

- ordinary answer-present results are not blocked when no strict projection is
  required;
- strict output contracts over reconcile groups require a projection artifact
  or an allowed projection-producing transform;
- reference-complete gaps are represented separately from generic missing
  projection so deterministic repair can preserve the missing key universe;
- OutputProjectionGraph is persisted in workflow state and journals, just like
  ActionGraph, ArtifactGraph, and LedgerGraph;
- prompts may use the graph for guidance, while hard completion decisions read
  typed graph status.

Changes:

- [x] Added `OutputProjectionGraph` and `BuildOutputProjectionGraph`.
- [x] Added typed statuses: `satisfied`, `missing_answer`,
      `missing_projection`, and `incomplete_reference`.
- [x] Exposed `output_projection_graph` in `workflow_state_json`.
- [x] Persisted `output_projection_graph` in terminal and checkpoint journals.
- [x] Rewired completion-gate output readiness checks to consume
      OutputProjectionGraph status while preserving existing detailed reference
      diagnostics.
- [x] Taught continuation/evaluator prompts to read
      `workflow_state_json.output_projection_graph`.
- [x] Added tests for ordinary answer acceptance, strict projection, reference
      incompleteness, journal JSON, state projection, and existing completion
      gate behavior.

Remaining P0 output work:

- [x] Move deterministic output-projection repair plan selection to consume
      OutputProjectionGraph directly instead of recomputing reference gaps.
- [ ] Feed OutputProjectionGraph status and repair action hints into process
      events/UX summaries.

### Batch 292: OutputProjectionGraph-Driven Projection Repair

After OutputProjectionGraph became the completion-gate source for final-answer
readiness, deterministic projection repair still selected `assemble_answer`
from older helper logic. This batch gives projection repair the same graph
input used by completion decisions, so the gate and repair plan converge on one
typed output-readiness contract.

Generic invariants:

- `missing_projection` and `incomplete_reference` statuses are the structural
  reasons that permit deterministic `assemble_answer` repair;
- `satisfied` output graph state must not produce another projection plan;
- reference-complete repair still requires the precise reference gap candidate
  path/field; OutputProjectionGraph decides that the gap exists, while the
  reference candidate carries the verifiable structural details;
- legacy non-graph repair remains only for direct/unmigrated callers.

Changes:

- [x] Added OutputProjectionGraph fields to output-projection and
      completion-repair planner inputs.
- [x] Rewired REPL/CLI output-projection repair paths to pass
      OutputProjectionGraph explicitly.
- [x] Added regression coverage for graph-driven `assemble_answer` repair and
      satisfied-output no-op behavior.

Remaining P0 output/UX work:

- [ ] Feed OutputProjectionGraph status and repair action hints into process
      events/UX summaries.
- [ ] Remove legacy output-projection recomputation once all internal callers
      pass typed output graph state.

### Batch 293: Graph-Derived Workflow Decisions

After LedgerGraph and OutputProjectionGraph started driving completion gates
and deterministic repair plans, `WorkflowDecision` still mostly projected the
latest evaluator status or the broad `allowed_next_actions` set. That left
CLI/REPL renderers with too much responsibility: they could see a graph blocker
in `workflow_state_json`, but had to decide for themselves whether the current
user-visible next step was a missing ledger, a blocked prerequisite, a strict
output projection, or a generic stage.

This batch moves that interpretation into the reducer-owned decision IR. It is
domain-neutral: the decision reads only typed graph status, missing
prerequisites, and producer action kinds. It does not inspect business file
names, target fields, row values, model prose, or customer-specific semantics.

Generic invariants:

- typed violations remain the highest-priority decision source and their repair
  hints override broad allowed actions;
- OutputProjectionGraph `missing_projection` and `incomplete_reference` states
  expose `assemble_answer` as the focused next action;
- LedgerGraph missing ledgers expose a typed reason code such as
  `ledger_missing_contributions` and producer action hints when the ledger is
  ready to produce;
- LedgerGraph blocked ledgers expose a typed reason code such as
  `ledger_blocked_contributions`, but do not pretend the blocked producer is
  executable; the decision keeps prerequisite-stage allowed actions instead;
- evaluator status/reason can still explain the current loop state, while
  graph hints fill missing reason/action fields without relying on prose.

Changes:

- [x] Extended `WorkflowDecisionInput` with LedgerGraph and
      OutputProjectionGraph.
- [x] Added graph-derived reason codes/reasons for missing ledgers, blocked
      ledgers, missing output projection, incomplete reference projection, and
      missing answer states.
- [x] Rewired REPL/CLI workflow state assembly to pass live graph state into
      `BuildWorkflowDecision`.
- [x] Narrowed `WorkflowDecision.NextActions` from broad allowed actions to
      typed violation repair hints or graph producer actions when safe.
- [x] Preserved prerequisite-stage allowed actions when a ledger is blocked by
      another graph node, avoiding non-ready action suggestions.
- [x] Added reducer and REPL workflow-state regression coverage.

Remaining P0 decision/UX work:

- [ ] Feed `WorkflowDecision` graph reason codes into low-noise process events
      so users see the current business goal, batch purpose, blocker, and next
      action without reading raw ledger/stage terminology.
- [ ] Remove legacy error/output recomputation fallbacks after direct callers
      are migrated to typed graph inputs.

### Batch 294: Executable Deferred Queue In ActionGraph

The deferred queue had two representations: REPL/CLI held the executable
`TaskPlan` in an outer variable, while `workflow_state_json.action_graph`
showed only deferred action nodes and a compact queue snapshot. That split made
the graph useful for audit, but not sufficient as the live typed state for a
continuation planner or journal reader.

This batch moves the executable deferred plan payload into ActionGraph itself.
The outer REPL/CLI variable remains a temporary adapter for loop control, but
state, prompts, checkpoints, and journals can now inspect one reducer-owned
graph object: deferred nodes, deferred queue lifecycle/status, and the concrete
deferred plan.

Generic invariants:

- `ActionGraph.Deferred` remains the node projection used for scheduling and
  idempotency display;
- `ActionGraph.DeferredQueue` remains the compact status/lifecycle summary;
- `ActionGraph.DeferredPlan` carries the executable typed plan payload when a
  queue exists;
- the reducer deep-copies action slices and params before exposing the plan, so
  later adapter mutations cannot silently alter the graph snapshot;
- no business semantics or file-name-specific rules are introduced.

Changes:

- [x] Added `deferred_plan` to ActionGraph.
- [x] Taught `ReduceActionGraphState` to clone the deferred TaskPlan into the
      graph, falling back to the deferred action list when only nodes are
      provided.
- [x] Rewired REPL/CLI workflow state assembly to pass the deferred TaskPlan
      into the reducer.
- [x] Verified continuation prompts now expose `action_graph.deferred_plan`
      alongside deferred nodes and queue status.
- [x] Added reducer, journal JSON, and REPL prompt regression coverage.

Remaining P0 deferred-storage work:

- [ ] Finish replacing direct REPL/CLI deferred-plan mutations with
      reducer-owned pop/dispatch transition helpers. Enqueue, retain, discard,
      and clear already use the queue API.
- [ ] Emit pop/dispatch transitions as typed ActionGraph events. Enqueue,
      retain, discard, and clear transitions already enter ActionGraph events.

### Batch 295: Deferred Queue Transition API

After Batch 294, the executable deferred plan lived inside ActionGraph, but the
REPL/CLI loops still mutated the queue by assigning a raw `TaskPlan` variable.
That kept the state machine split: graph state could show the queue, while loop
code still owned enqueue/retain/discard decisions.

This batch introduces a reducer-owned `DeferredQueueState` and typed queue
transition events. It starts by moving enqueue, retain, discard, and clear
through dataworkflow helpers. The remaining pop/dispatch path is intentionally
left as a separate follow-up so the next batch can adjust dispatch semantics
without mixing them with queue storage and audit changes.

Generic invariants:

- the deferred queue is a typed state object with a cloned executable plan and
  bounded transition events;
- enqueue, retain, discard, and clear are structural queue transitions, not UI
  strings;
- ActionGraph exposes `deferred_events` alongside `deferred_plan`,
  `deferred_queue`, and deferred nodes;
- queue transitions do not inspect business roles, file names, row values, or
  model prose;
- old checkpoint callers can still pass a raw deferred plan through the wrapper
  while CLI/REPL main loops use the queue state API.

Changes:

- [x] Added `DeferredQueueState` and `DeferredQueueEvent`.
- [x] Added queue transition helpers for enqueue, retain, discard, and clear.
- [x] Added bounded event retention and plan cloning to keep queue state stable
      under later adapter mutations.
- [x] Added `ActionGraph.DeferredEvents`.
- [x] Rewired CLI and REPL data loops so save/discard/retain/clear go through
      the queue API.
- [x] Added a queue-aware checkpoint writer so deferred transition events are
      included in ActionGraph.
- [x] Added reducer, journal JSON, and REPL workflow-state regression coverage.

Remaining P0 deferred dispatch work:

- [x] Move `dataTaskPopDeferredActionBatch` to return/update
      `DeferredQueueState` through a reducer-owned dispatch transition.
- [x] Replace post-pop `saveDeferredPlan` calls with a dispatch event that
      atomically records the executed rank and remaining queue.

### Batch 296: Deferred Dispatch As Queue Transition

Batch 295 moved enqueue, retain, discard, and clear into the queue state API.
The last direct mutation remained on the successful pop path: after a ready
deferred rank was selected, REPL/CLI executed that rank and then called
`saveDeferredPlan` for the remainder. That made a dispatch look like a fresh
enqueue and lost the atomic relationship between "this rank ran" and "these
actions remain deferred."

This batch promotes pop/dispatch into the same queue transition model. The
selection logic still uses typed readiness, allowed-action, field-contract, and
admission checks; the change is only ownership of the queue mutation and audit
event.

Generic invariants:

- dispatch is one typed transition with dispatched action count, remaining
  action count, first action identity, and readiness status;
- a successful dispatch updates the queue plan to the remainder in the same
  reducer-owned state update;
- REPL/CLI do not call enqueue for a post-dispatch remainder;
- failed dispatch attempts still return typed deferred status and leave the
  queue unchanged;
- no business semantics or prompt prose participate in queue mutation.

Changes:

- [x] Added `DispatchDeferredQueue`.
- [x] Added a queue-aware `dataTaskPopDeferredQueueActionBatch` helper.
- [x] Kept the older raw-plan pop helper as an adapter for existing tests and
      direct callers.
- [x] Rewired CLI and REPL result loops to update deferred queue state through
      dispatch rather than save/enqueue.
- [x] Preserved deferred plan audit/progress for remaining queue payloads
      without adding duplicate enqueue events.
- [x] Added reducer transition coverage.

Remaining P0 deferred work:

- [ ] Consider moving the outer `DeferredQueueState` storage itself into a
      durable workflow runtime object shared by REPL and CLI entrypoints, so
      loop-local variables become purely adapters around a single workflow IR
      handle.

### Batch 297: Admission-Time Artifact Schema Shape Guard

Earlier batches moved field-contract checks to the full internal artifact
contract view, but one generic shape gap remained: an input alias could exist
and therefore pass availability checks even when the artifact was metadata,
diagnostic output, a workflow ledger, or another non-record shape. Record-only
typed actions should not discover that mistake inside the runner after the
batch has already been admitted.

This batch front-loads that schema compatibility check into admission. The
guard uses `ArtifactSchemaProjection` and `ArtifactUsableForRecordAction`,
which are already domain-neutral IR helpers. It does not inspect business file
names, row values, user intent keywords, or model prose.

Generic invariants:

- stale aliases remain input-availability violations;
- missing fields remain field-contract violations with candidate artifact
  hints;
- known aliases with non-record shapes become
  `artifact_schema_incompatible` violations before execution;
- the guard applies only to record-only typed actions such as filtering,
  grouping, joining, normalization, enrichment, and contribution calculation;
- text/record materialization actions such as `extract_fields` are not blocked
  merely because an input is text-shaped.

Changes:

- [x] Added `dataTaskWorkflowArtifactSchemaProjections` as the full schema
      projection source for admission-time checks.
- [x] Reused that projection source for existing full contract artifact access.
- [x] Added an admission-time schema shape guard for record-only actions.
- [x] Emitted typed `artifact_schema_incompatible` workflow violations with
      action/input identity, idempotency key, projection shape/class/kind, and
      repair hints.
- [x] Added regression coverage proving metadata-only artifacts cannot be
      consumed by `filter_records` as if they were record sets.

Remaining schema/IR work:

- [x] Move the action-specific field-reference extraction currently in REPL
      helpers into dataworkflow admission utilities, so field-contract guard
      construction itself no longer lives in the REPL package.

### Batch 298: Field-Reference IR And Decision Process Events

The next audit found two remaining places where the data workflow still behaved
like a REPL-owned system instead of a graph-owned one:

- single-record typed actions extracted their input-field references through
  REPL helper functions, even though those references are part of each action's
  structural contract;
- process events carried plan intent and audit counters, but did not carry the
  reducer's typed `WorkflowDecision`, so UX had to choose between repetitive
  internal counters and generic fallback text.

This batch moves both toward IR ownership. It is deliberately domain-neutral:
field references are read from typed action params such as filters, field specs,
grouping fields, qualification fields, and contribution value/group/filter
fields. The code does not inspect business names, dates, amounts, file names, or
customer-specific terms.

Changes:

- [x] Added `SingleRecordSetActionFieldRefs` and action-specific field-reference
      helpers to `internal/dataworkflow`.
- [x] Rewired the REPL field-contract adapter to call the dataworkflow helpers
      for derive/extract, group, expand, filter, qualify, and contribution
      actions.
- [x] Added regression coverage for structured arrays/objects, delimited field
      params, constant transforms, filter refs, qualification refs, grouping
      refs, and contribution refs.
- [x] Added `decision` to workflow process events and preserved decision status,
      reason code, and next actions as audit details.
- [x] Fed checkpoint guard events and live evaluate progress through the typed
      `WorkflowDecision`, so CLI/REPL can show the current reducer judgment
      without parsing error prose.
- [x] Preserved low-noise suppression of repeated result/evaluate plan details
      while allowing reducer-carried decision reasons to appear as a distinct
      process detail.

Remaining architecture items:

- [x] Move relation-specific field-requirement projection for joins,
      enrichment, mapping candidates, and normalization into
      `internal/dataworkflow`, so REPL adapters no longer interpret those action
      params themselves.
- [ ] Move the remaining resolution-application field-contract guard behind
      the same workflow-IR boundary. It still has role inference and diagnostic
      artifact handling that should be split out carefully rather than moved in
      one broad patch.
- [ ] Replace the remaining REPL-local live workflow loop variables with a
      durable `WorkflowRuntime` handle that owns records, current plan, deferred
      queue state, admission decision, and journal snapshots.
- [ ] Upgrade live CLI/REPL workflow progress calls to consume
      `WorkflowJournalEvent` directly, rather than passing pre-rendered detail
      strings through `emitDataTaskWorkflowAudit`.

### Batch 299: Relation Field Requirements In Workflow IR

Batch 298 moved simple single-record-set field extraction into
`internal/dataworkflow`, but relation-shaped actions still had duplicated
parameter interpretation in the REPL layer. That kept one of the most important
admission inputs outside the workflow IR: which role-path and fields a join,
normalization, mapping-candidate, or enrichment action structurally requires.

This batch moves that requirement projection into dataworkflow helpers. The
helpers only read typed action params and input paths. They do not inspect file
names, row values, business words, prompt prose, or customer-specific concepts.
REPL remains a thin adapter that checks those requirements against the current
artifact access/projection view and emits the existing typed guard result.

Generic invariants:

- relation actions expose role-tagged requirements such as left/right,
  source/reference, and base/lookup;
- explicit paths are marked in the IR so admission can distinguish model-stated
  roles from input-order fallbacks;
- shared join keys are represented as requirements on both sides of a join;
- normalization with explicit mapping/resolution payloads skips
  source/reference field requirements, matching the executor's structured
  mapping path;
- no new hard gate depends on noisy ranking, business labels, or model prose.

Changes:

- [x] Added `ActionFieldRequirement` to `internal/dataworkflow`.
- [x] Added `JoinActionFieldRequirements` with role paths, explicit-path flags,
      and shared-key fallback.
- [x] Added `NormalizeEntityActionFieldRequirements`, reused by mapping
      candidates, including source/reference roles and explicit-mapping skip.
- [x] Added `EnrichActionFieldRequirements` for lookup/enrichment role
      requirements.
- [x] Rewired REPL relation field-contract adapters to consume these workflow
      IR requirements instead of duplicating parameter parsing.
- [x] Added unit coverage for join, normalization, explicit mapping skip, and
      enrichment lookup specs.

Remaining architecture items:

- [ ] Move resolution-application role/path/field requirement projection into
      `internal/dataworkflow` after separating pure requirements from
      diagnostic-artifact role inference.
- [ ] Move relation field-contract guard result construction itself behind an
      admission facade once artifact access/projection is no longer REPL-owned.
- [ ] Add workflow-state snapshots that expose relation requirements alongside
      artifact schema projection, so repair prompts can reference typed
      requirements without rendering REPL-specific text.

### Batch 300: Storage-Neutral Workflow Runtime Handle

The next architectural gap was state ownership rather than another planning
rule. Even after deferred queue transitions moved into dataworkflow, the CLI
and REPL entrypoints still held local variables for deferred queue state and the
latest admission decision. That made the adaptive data workflow look like two
parallel runtimes with shared helper functions.

This batch introduces a small storage-neutral `WorkflowRuntime` handle in
`internal/dataworkflow`. It stores dataquery/dataworkflow IR only: current plan
snapshot, deferred queue state, latest admission decision, and round counters.
It does not import or know about REPL records, UI rendering, files, prompts, or
business semantics.

Generic invariants:

- live runtime state is deep-copied at the workflow IR boundary;
- deferred queue mutations go through runtime methods in both CLI and REPL;
- admission decisions are stored as workflow IR, not entrypoint-local variables;
- runtime state contains no user-domain fields, file-name rules, prompt prose,
  or eval-case-specific logic;
- this is an ownership migration only: existing admission, staging, runner, and
  completion semantics are unchanged.

Changes:

- [x] Added `WorkflowRuntime` with current-plan, deferred-queue, admission, and
      round accessors.
- [x] Added deep-copy support for task plans, coverage materials, admission
      decisions, and guard violations at the runtime boundary.
- [x] Rewired CLI data workflow deferred queue enqueue, dispatch update,
      retain, discard, clear, checkpoint, and decision-progress calls through
      `WorkflowRuntime`.
- [x] Rewired REPL data workflow deferred queue and admission access through
      the same runtime API.
- [x] Stored accepted candidate plans in runtime snapshots without changing the
      existing local loop variable behavior.
- [x] Added runtime unit coverage for current-plan isolation, deferred queue
      ownership, transition events, and admission isolation.

Remaining architecture items:

- [x] Move data workflow records into a package-neutral record IR so later
      runtime/journal code can consume records without depending on REPL
      internals.
- [x] Replace direct `currentPlan = ...` assignments in CLI/REPL loops with a
      reducer/runtime transition helper that records the reason, source, and
      admission decision for every plan switch.
- [ ] Move checkpoint/journal snapshot construction onto `WorkflowRuntime`, so
      CLI/REPL pass records/results into one journal builder instead of
      assembling workflow state themselves.
- [ ] Convert live process-event rendering to consume runtime journal events
      directly.

### Batch 301: Package-Neutral Workflow Records And Event Builder

After introducing `WorkflowRuntime`, the next REPL-specific state was the data
workflow record type itself. The record fields were already pure IR:
`TaskPlan`, `Result`, `Evaluation`, admission decision, and error text. Keeping
that struct in the REPL package forced dataworkflow journal/event code to stay
as entrypoint glue.

This batch moves the record type and process-event derivation into
`internal/dataworkflow` without changing the existing checkpoint JSON field
shape. REPL keeps a type alias as a compatibility adapter, so call sites and
resume files continue to work while later batches move record storage into the
runtime.

Generic invariants:

- workflow records contain dataquery/dataworkflow IR only;
- checkpoint field names are preserved during this migration;
- process-event generation is derived from typed plan/result/admission fields,
  not business labels, file names, or model prose hard gates;
- REPL/CLI rendering remains an adapter over workflow events.

Changes:

- [x] Added `WorkflowRecord` to `internal/dataworkflow`.
- [x] Replaced the REPL-local record struct with an alias to the workflow IR
      type.
- [x] Added `BuildWorkflowJournalEvents(records []WorkflowRecord)` in
      dataworkflow.
- [x] Rewired REPL checkpoint/journal event generation to call the workflow
      event builder.
- [x] Added JSON-shape and event-builder regression coverage.

Remaining architecture items:

- [x] Move record append/update operations into `WorkflowRuntime`, including
      result, error, evaluation, and admission attachment transitions.
- [ ] Move checkpoint/journal snapshot construction onto `WorkflowRuntime`.
- [ ] Convert CLI/REPL progress rendering to consume runtime events directly
      instead of re-deriving details from records.

### Batch 302: Plan Transition Events In Workflow Runtime

The data workflow still had an important state leak: CLI and REPL switched
`currentPlan` through plain assignments in many fallback, repair, completion,
and deferred-dispatch paths. Even after the runtime owned deferred state, those
plan changes had no typed transition record. That made later journal/reducer
work reconstruct intent from surrounding UI/audit calls.

This batch adds plan transition events to `WorkflowRuntime` and wraps the
data-lane plan switches in CLI and REPL. The transition records capture only
structural workflow facts: source, round, action count, first action identity,
and reason. They do not inspect business labels, row values, file names, or
model prose for hard decisions.

Generic invariants:

- every accepted candidate plan records a runtime transition;
- deterministic fallback and deferred-dispatch plan switches record their
  source and reason;
- transition recording deep-copies the plan snapshot;
- transition events are audit/state facts only and do not change scheduling
  semantics;
- operation/source/trace/log/write flows are untouched.

Changes:

- [x] Added `PlanTransitionEvent` and `SwitchCurrentPlan` to
      `WorkflowRuntime`.
- [x] Added bounded plan-transition retention and immutable accessor.
- [x] Rewired CLI accepted-plan, fallback, ledger-completion, and
      deferred-dispatch switches through runtime transitions.
- [x] Rewired REPL data accepted-plan, fallback, ledger-completion, and
      deferred-dispatch switches through runtime transitions.
- [x] Added runtime transition regression coverage.

Remaining architecture items:

- [x] Include plan-transition events in checkpoint/journal snapshots.
- [x] Move record append/update operations into runtime transitions.
- [ ] Convert direct plan-transition event rendering into low-noise business
      process events once the journal owns them.

### Batch 303: Runtime-Owned Workflow Records

After plan switches became runtime transitions, the remaining live history
state was still a CLI/REPL-owned `records` slice. Even though
`WorkflowRecord` had already moved into `internal/dataworkflow`, both
entrypoints were still appending result/error records and mutating the latest
evaluation directly. That kept workflow history ownership split across the
runtime and the UI loops.

This batch makes `WorkflowRuntime` the owner of workflow records. CLI and REPL
still pass snapshots to existing planner/evaluator helpers, but those snapshots
now come from runtime methods instead of local append/mutation. This is an IR
ownership change only; scheduling, prompts, guard semantics, execution, and
terminal answer behavior are unchanged.

Generic invariants:

- runtime records contain only `WorkflowRecord`, `TaskPlan`, `Result`,
  `Evaluation`, and admission IR;
- record accessors return deep copies, including nested result artifacts,
  row decisions, ledgers, reconcile reports, patches, and evaluation slices;
- checkpoint preview records use `RecordsWith` so guard snapshots do not mutate
  live state;
- REPL/CLI remain adapters over runtime-owned state;
- operation, source-code read mode, trace/log analysis, and write mode are not
  touched.

Changes:

- [x] Added `SetRecords`, `Records`, `AppendRecord`, `RecordsWith`,
      `AttachLastEvaluation`, and `AttachLastError` to `WorkflowRuntime`.
- [x] Added structural deep-copy helpers for workflow records and dataquery
      results.
- [x] Rewired CLI data workflow resume, guard, execution failure, validation,
      result, deferred-discard, and evaluator update paths through runtime
      record methods.
- [x] Rewired REPL data workflow guard, non-text material, execution failure,
      validation, result, deferred-discard, and evaluator update paths through
      the same runtime record methods.
- [x] Added regression coverage proving record snapshots cannot mutate live
      runtime state.

Remaining architecture items:

- [x] Move checkpoint/journal snapshot construction onto `WorkflowRuntime`.
- [x] Include plan-transition events in checkpoint/journal snapshots.
- [ ] Replace entrypoint-local `records` variables with narrower snapshot
      helpers after planner/evaluator prompt inputs are runtime-aware.
- [ ] Convert CLI/REPL progress rendering to consume runtime events directly
      instead of re-deriving details from records.

### Batch 304: Runtime-Built Workflow Journal Snapshots

Once runtime owned plans, transitions, deferred queues, and records, checkpoint
and terminal audit files were still assembled by REPL helper functions. That
kept the durable journal shape dependent on entrypoint-local glue and made it
easy to omit runtime-only facts such as plan transitions.

This batch moves the journal assembly boundary into `internal/dataworkflow`.
The REPL/CLI adapters still write files and still compute the existing state
graphs for now, but the final `WorkflowJournal` object is built through
`WorkflowRuntime.BuildJournalSnapshot`. Runtime-owned records and plan
transitions now flow into checkpoint snapshots without parsing UI text or
business-specific fields.

Generic invariants:

- journal assembly consumes typed workflow inputs: records, plans, action
  graph, ledger graph, artifact graph, output graph, progress, decision, and
  guard results;
- resume payloads and action events are generated in `internal/dataworkflow`;
- guard checkpoint previews can pass explicit preview records without mutating
  live runtime records;
- plan transitions are audit facts in the journal, not hard scheduling inputs;
- file I/O remains in REPL/CLI adapters; source/trace/log/write flows are
  unchanged.

Changes:

- [x] Added `plan_transitions` to `WorkflowJournal`.
- [x] Added `WorkflowJournalBuildInput` and
      `WorkflowRuntime.BuildJournalSnapshot`.
- [x] Moved workflow resume payload construction into `dataworkflow`.
- [x] Moved workflow action-event construction into `dataworkflow`.
- [x] Rewired terminal audit and checkpoint writers to use the runtime journal
      builder.
- [x] Rewired CLI/REPL checkpoint call sites to pass the live runtime so
      transition events are preserved.
- [x] Added journal regression coverage for runtime records, guard preview
      records, resume payloads, and plan transitions.

Remaining architecture items:

- [x] Add a package-neutral state snapshot facade for journal/reducer inputs.
- [ ] Move full state-graph construction (`ActionGraph`, `LedgerGraph`,
      `ArtifactGraph`, `ProgressWindow`) behind a runtime reducer input instead
      of REPL-owned `dataTaskWorkflowStateView`.
- [ ] Replace entrypoint-local `records` variables with runtime snapshot
      helpers after planner/evaluator prompts accept runtime state directly.
- [ ] Convert CLI/REPL progress rendering to consume runtime journal/process
      events directly.

### Batch 305: Package-Neutral Workflow State Snapshot Facade

The next reducer boundary is the large REPL-local `dataTaskWorkflowStateView`.
It still owns many prompt-facing samples and repair diagnostics, so moving it
wholesale would be a risky rewrite. However, the durable journal and future
runtime reducer only need a smaller structural subset: action graph, ledger
graph, output graph, artifact graph, progress, typed violations, and workflow
decision.

This batch adds a package-neutral `WorkflowStateSnapshot` facade in
`internal/dataworkflow` and makes journal construction consume that snapshot.
The existing REPL state computation remains in place, but its journal-facing
surface is now one typed IR object instead of a long list of entrypoint fields.

Generic invariants:

- `WorkflowStateSnapshot` contains only structural workflow graphs and typed
  decision/violation facts;
- prompt-only samples, business labels, and UI details stay outside the
  snapshot;
- journal construction prefers the snapshot over legacy individual fields;
- this is a facade for reducer migration, not a semantic scheduling change;
- source/trace/log/write/operation flows are untouched.

Changes:

- [x] Added `WorkflowStateSnapshot` and zero detection in `dataworkflow`.
- [x] Updated `WorkflowJournalBuildInput` to accept a state snapshot.
- [x] Made `WorkflowRuntime.BuildJournalSnapshot` prefer the state snapshot and
      fall back to legacy fields only for older call paths/tests.
- [x] Added a REPL adapter that converts `dataTaskWorkflowStateView` into
      `WorkflowStateSnapshot`.
- [x] Rewired checkpoint and terminal journal builders to pass the snapshot.
- [x] Added regression coverage proving snapshot fields override legacy
      journal inputs.

Remaining architecture items:

- [x] Move stage facts and allowed-next-action derivation behind the package
      neutral state snapshot as the first reducer-facing boundary.
- [ ] Move the rest of the state reducer computations out of REPL into
      dataworkflow inputs/builders.
- [ ] Replace prompt/evaluator record slices with runtime state snapshots plus
      bounded record views.
- [ ] Convert CLI/REPL progress rendering to consume runtime process events
      directly.

### Batch 306: Stage Facts On Workflow State Snapshot

`dataTaskWorkflowStateView` still lives in the REPL adapter, but several hard
scheduling decisions were already being derived from the same structural facts:
material coverage, ledger requirements, ledger counts, reconciliation state,
and answer availability. Keeping those derivations as REPL-local helpers makes
it easy for future CLI, repair, deferred, or resume paths to drift.

This batch moves stage facts onto the package-neutral
`WorkflowStateSnapshot`. The REPL adapter still computes the facts for now, but
the next-stage and allowed-action decisions now read through the shared
snapshot methods. This is a small boundary migration toward a real reducer, not
a new guard for any specific data task.

Generic invariants:

- stage selection depends only on typed workflow facts, not prose, prompt text,
  business labels, or file names;
- allowed action kinds are derived from the same stage contract used by the
  planner schema and workflow guards;
- entrypoints may adapt legacy local state into a snapshot, but they should not
  fork their own stage decision logic;
- no source-code, trace, log, operation, or write-mode flow is touched.

Changes:

- [x] Added `StageFacts` to `WorkflowStateSnapshot`.
- [x] Added snapshot methods for `Facts`, `NextStage`,
      `AllowedNextActionContracts`, and `AllowedNextActions`.
- [x] Rewired REPL workflow stage and allowed-action helper wrappers to read
      through the snapshot methods.
- [x] Added regression coverage proving snapshot-derived stage and action
      contracts remain consistent.

Remaining architecture items:

- [x] Move calculation of `StageFacts` itself out of the REPL
      `dataTaskWorkflowStateView` adapter and into `dataworkflow` reducer
      inputs.
- [ ] Move graph construction for `ActionGraph`, `LedgerGraph`,
      `ArtifactGraph`, `ProgressWindow`, and typed violations behind the same
      reducer boundary.
- [ ] Replace planner/evaluator prompts that consume raw record slices with a
      snapshot plus bounded record views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 307: Stage Facts Reducer Input Builder

Batch 306 made stage decisions read through `WorkflowStateSnapshot`, but the
mapping from workflow coverage/counts into `StageFacts` still lived in a REPL
helper. That left a small but important fork risk: a future CLI, repair,
resume, or deferred path could accidentally rebuild the same facts with a
different contract/count mapping.

This batch moves the mapping into `internal/dataworkflow` as
`StageFactsInput` plus `BuildStageFacts`. The REPL adapter now supplies only
objective inputs: material coverage sufficiency, the workflow coverage
contract view, ledger counts, entity materialization, reconcile presence, and
answer presence. The package owns the fact projection used for next-stage and
allowed-action decisions.

Generic invariants:

- coverage requirements come from the typed workflow contract view;
- ledger progress comes from typed ledger counts and terminal flags;
- no business-specific material roles, field names, numeric units, or prompt
  prose participate in the hard stage facts;
- this is a reducer boundary migration only and does not change source, trace,
  log, operation, or write-mode behavior.

Changes:

- [x] Added `StageFactsInput` in `dataworkflow`.
- [x] Added `BuildStageFacts` to centralize projection from coverage/counts to
      reducer facts.
- [x] Rewired the REPL data workflow stage-facts adapter to call the shared
      builder.
- [x] Reused the shared facts object when building the ledger graph.
- [x] Added regression coverage for coverage booleans, ledger counts, and
      terminal flags flowing into `StageFacts`.

Remaining architecture items:

- [x] Move graph construction for `ActionGraph`, `LedgerGraph`,
      `ArtifactGraph`, `ProgressWindow`, and typed violations behind the same
      reducer boundary.
- [ ] Add a reducer-level input object that collects material coverage,
      coverage contract, plan records, deferred queue, and current plan without
      exposing REPL-local state.
- [ ] Replace planner/evaluator prompts that consume raw record slices with a
      snapshot plus bounded record views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 308: Unified Workflow State Snapshot Builder

After stage facts moved into `dataworkflow`, the REPL adapter still assembled
the major workflow graphs by calling individual reducers and then stitching
their outputs together. That was better than owning the graph semantics, but
still left the entrypoint as the place where action, ledger, artifact, output,
progress, violations, and decision graphs became one state.

This batch adds `BuildWorkflowStateSnapshot` as the package-level aggregation
entrypoint. The REPL still collects objective input facts and UI/prompt helper
data, but graph reduction and decision assembly now meet inside
`internal/dataworkflow`. The change does not introduce new business semantics;
it centralizes existing typed reducers into one IR boundary.

Generic invariants:

- `ActionGraph`, `LedgerGraph`, `OutputProjectionGraph`, `ArtifactGraph`,
  `ProgressWindow`, `WorkflowViolations`, and `WorkflowDecision` are assembled
  through one package-level snapshot builder;
- deferred queue snapshots are produced by the `ActionGraph` reducer, not
  patched in by a REPL caller;
- entrypoints may still collect typed inputs from local records, but they do
  not decide how the graphs compose;
- no prompt prose, business labels, user-domain field names, or case-specific
  units participate in hard workflow decisions.

Changes:

- [x] Added `WorkflowStateSnapshotInput` and `BuildWorkflowStateSnapshot`.
- [x] Rewired REPL data workflow state assembly to use the shared snapshot
      builder for action, ledger, output, artifact, progress, and decision
      graphs.
- [x] Moved deferred-queue snapshot projection into `ReduceActionGraphState`.
- [x] Added regression coverage for snapshot graph/decision aggregation.
- [x] Added regression coverage that deferred-queue summaries come from the
      action graph reducer.

Remaining architecture items:

- [x] Add a reducer-level input object that collects material coverage,
      coverage contract, plan records, deferred queue, and current plan without
      exposing REPL-local state.
- [ ] Replace planner/evaluator prompts that consume raw record slices with a
      snapshot plus bounded record views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.
- [ ] Move typed violation discovery itself behind package-level reducer
      inputs; the current builder consumes typed violations but does not yet
      discover all of them.

### Batch 309: Runtime Record Inputs For Reducer Snapshots

Batch 308 centralized graph aggregation, but the REPL adapter still prepared
several generic inputs itself: action events from records, progress events from
results, newest-first artifact sources, current ready actions, and deferred
queue actions. Those are not UI concerns and do not depend on business meaning.

This batch adds `WorkflowReducerInput` and `BuildWorkflowReducerSnapshot`.
The reducer input accepts package-neutral `WorkflowRecord`, the current plan,
the live deferred queue, typed stage facts, output graph input, optional
overrides, and typed violations. When progress events or artifact sources are
not supplied, `dataworkflow` derives them from records. The REPL keeps its
prompt/UI projection helpers, but the structural snapshot can now be built
from runtime-owned state.

Generic invariants:

- action events are derived from `WorkflowRecord` through
  `BuildWorkflowActionEvents`;
- progress events and newest-first artifact sources are package-level
  projections over records/results;
- current ready actions and deferred queue actions enter the reducer through
  one input object;
- REPL wrappers may remain as compatibility adapters, but they delegate the
  generic projections to `dataworkflow`;
- no domain-specific material roles, field names, numeric units, or prose
  summaries participate in reducer decisions.

Changes:

- [x] Added `WorkflowReducerInput`.
- [x] Added `BuildWorkflowReducerSnapshot`.
- [x] Added `ProgressEventsFromRecords` and record artifact progress
      projection in `dataworkflow`.
- [x] Added `ArtifactsNewestFirst` in `dataworkflow`.
- [x] Rewired REPL state assembly to use the reducer input instead of hand
      assembling action/progress/artifact reducer inputs.
- [x] Added regression coverage proving reducer snapshots derive executed
      actions, current ready actions, artifacts, and progress from records.

Remaining architecture items:

- [ ] Replace planner/evaluator prompts that consume raw record slices with a
      snapshot plus bounded record views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.
- [x] Move typed violation discovery itself behind package-level reducer
      inputs; the current reducer consumes typed violations but does not yet
      discover all of them.

### Batch 310: Workflow Violation Reducer Boundary

The snapshot reducer consumed typed violations, but one important violation
class still lived as a REPL-local discovery function: relation/action
no-progress. The underlying inputs are already package-neutral (`StageFacts`
and `ProgressEvent`), so keeping the discovery outside `dataworkflow` made the
reducer boundary look thinner than it really was.

This batch adds `WorkflowViolationInput` and `BuildWorkflowViolations`. The
builder combines guard/admission violations, package-level no-progress
violations, and additional typed issues supplied by adapters. REPL-specific
field-contract/zero-match/unmatched/eligibility issue discovery remains in the
adapter for now because it still depends on prompt artifact-access views, but
the workflow-level violation reducer now owns composition and no-progress
discovery.

Generic invariants:

- guard violations, progress-derived violations, and adapter-discovered typed
  issues are all `WorkflowViolation` values before they reach scheduling
  decisions;
- no-progress detection depends on typed facts and progress events, not model
  prose or business labels;
- adapters may discover extra typed issues while legacy prompt views exist,
  but they should pass those issues into the reducer instead of composing final
  violation lists themselves;
- no source, trace, log, operation, or write-mode flow is touched.

Changes:

- [x] Added `WorkflowViolationInput`.
- [x] Added `BuildWorkflowViolations`.
- [x] Rewired REPL data workflow typed-violation assembly to delegate
      guard/no-progress/additional composition to `dataworkflow`.
- [x] Removed the REPL-local no-progress violation helper.
- [x] Added regression coverage for combined guard, no-progress, and
      additional typed violations.

Remaining architecture items:

- [x] Move field-contract, zero-match, unmatched-resolution, and zero-eligible
      issue discovery behind package-level artifact/schema inputs instead of
      REPL prompt views.
- [x] Replace planner/evaluator prompts that consume raw record slices with a
      snapshot plus bounded record views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 311: Package-Level Bounded Workflow Record Views

Planner and evaluator prompts still need compact recent-round context, but the
mechanics of choosing the bounded window, clipping action scripts/params,
clipping errors/evaluation reasons, and emitting stable JSON are not REPL
logic. Keeping that code in the REPL made prompt input construction look like
another local workflow state machine.

This batch adds a package-level bounded record-view renderer in
`internal/dataworkflow`. The REPL keeps the existing data-result compactor for
now because it still carries artifact-access and material-set prompt views, but
record slicing and JSON rendering now live behind a package-neutral
`WorkflowRecordView` boundary.

Generic invariants:

- bounded record views are derived from `WorkflowRecord`, not REPL-private
  records;
- action purpose, scripts, params, errors, and evaluation reasons are clipped
  by explicit budget fields;
- result detail is supplied through a typed callback so entrypoint-specific
  prompt views can migrate gradually without changing planner semantics;
- the omission banner continues to point models at `workflow_state_json` as the
  authoritative cumulative state instead of compact samples;
- no prompt prose is used as a hard gate and no business-domain field names are
  introduced.

Changes:

- [x] Added `WorkflowRecordView`.
- [x] Added `WorkflowRecordViewBudget`.
- [x] Added `BuildWorkflowRecordViews` and
      `RenderWorkflowRecordViewsForPrompt`.
- [x] Added package-level action/param/text/slice compactors for record views.
- [x] Rewired REPL `renderDataTaskRecordsForPromptWithBudget` to delegate
      bounded record rendering to `dataworkflow`, keeping only the existing
      result-view callback local.
- [x] Added regression coverage for bounded windows, text clipping, result
      callback rendering, sorted/truncated params, and omission banners.

Remaining architecture items:

- [x] Move result prompt compaction behind package-level artifact/schema
      inputs.
- [x] Move field-contract, zero-match, unmatched-resolution, and zero-eligible
      issue discovery behind package-level artifact/schema inputs instead of
      REPL prompt views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 312: Package-Level Artifact Access Views

Planner/evaluator prompts and REPL-local guards need a compact catalog of
generated artifacts: aliases, JSON shape, fields, access hints, lineage, and a
few field samples. That catalog is not business logic and should not live as a
REPL-private prompt struct. It is an IR view over the artifact/schema graph.

This batch moves artifact-access rendering into `internal/dataworkflow` as
`ArtifactAccessView`. The REPL keeps a type alias while delegating all artifact
access sampling to the package-level implementation. Contract access built from
`ArtifactSchemaProjection` deliberately preserves full fields and lineage
because hard field gates must consume precise schema signals, not bounded prompt
previews.

Generic invariants:

- artifact-access views are derived from objective artifact/schema metadata;
- business roles are not inferred by system code;
- prompt availability views are bounded and may include small field samples;
- contract views derived from schema projections preserve complete fields for
  hard gates;
- access hints are generated from shape metadata only and remain advisory;
- no prompt prose or case-specific field names drive control flow.

Changes:

- [x] Added `ArtifactAccessView`.
- [x] Added `BuildArtifactAccessViews` and
      `BuildArtifactAccessViewsWithFieldSamples`.
- [x] Added `ArtifactAccessViewsFromProjections` for full contract access.
- [x] Rewired REPL artifact availability, contract access, and result prompt
      access sampling to delegate to `dataworkflow`.
- [x] Removed the duplicate REPL-local artifact access sampler, field-sample
      extractor, access-key builder, and shape hint implementation.
- [x] Added regression coverage for schema/lineage/sample rendering, dedupe
      with child traversal, and projection-derived contract views.

Remaining architecture items:

- [x] Move result prompt compaction behind package-level artifact/schema
      inputs.
- [x] Move field-contract, zero-match, unmatched-resolution, and zero-eligible
      issue discovery behind package-level artifact/schema inputs instead of
      REPL prompt views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 313: Package-Level Material Collection Views

Data result prompts expose objective material collections so the planner can
see candidate related text evidence and same-directory material groups without
inventing filesystem traversal. That projection is not REPL workflow logic and
does not assign business roles. It should be a package-level view over artifact
metadata.

This batch adds `MaterialCollectionView` in `internal/dataworkflow` and rewires
the REPL result prompt to delegate material-set handle generation to it. The
existing `material_set_handles` JSON field remains as a prompt/output shape,
but the implementation now belongs to the IR layer.

Generic invariants:

- material collection views use objective source paths and artifact fields;
- related text evidence is surfaced as concrete paths, not business meaning;
- directory groups are candidate collections, not workflow hard floors;
- bounded prompt views never promote optional materials into required
  coverage;
- no user-intent keyword matching or business-domain classification is added.

Changes:

- [x] Added `MaterialCollectionView`.
- [x] Added `BuildMaterialCollectionViews`.
- [x] Rewired REPL `sampleDataTaskMaterialSetHandles` to delegate to
      `dataworkflow`.
- [x] Removed the unused REPL-local material-set sorter/helper.
- [x] Added package-level regression coverage for related text evidence,
      directory grouping, bounded order, and stable related-first rendering.

Remaining architecture items:

- [x] Move result prompt compaction behind package-level artifact/schema
      inputs.
- [ ] Move field-contract, zero-match, unmatched-resolution, and zero-eligible
      issue discovery behind package-level artifact/schema inputs instead of
      REPL prompt views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 314: Package-Level Result Prompt Views

Recent-round prompt context needs a compact view of data results: answer/audit
snippets, ledger counts, bounded decision/rule/contribution/entity samples,
reconcile summaries, artifact previews, artifact access, material collection
handles, and contract warnings. Keeping that compaction in the REPL created
another local state surface and duplicated artifact/material view logic.

This batch moves result prompt compaction into `internal/dataworkflow` as
`ResultPromptView`. The REPL now keeps type aliases and thin compatibility
wrappers for existing call sites, while the actual result projection,
ledger-projection, artifact-sampling, and row/reconcile clipping logic lives in
the IR package.

Generic invariants:

- result prompt views are projections of typed `dataquery.Result` structures;
- hard gates continue to use precise schema/ledger state, not clipped samples;
- bounded samples are for model context and user audit only;
- artifact access and material collection fields reuse the package-level IR
  views introduced in the previous batches;
- no business-domain roles, field names, or intent keywords are introduced.

Changes:

- [x] Added `ResultPromptView`.
- [x] Added `LedgerProjection`.
- [x] Added `ResultPromptViewBudget`.
- [x] Added `BuildResultPromptView` and
      `BuildCompactResultPromptView`.
- [x] Moved artifact sample compaction, answer item counting, reconcile group
      summary, ledger projection, decision/rule/contribution/entity samples,
      and reconcile clipping into `dataworkflow`.
- [x] Rewired REPL result prompt helpers to delegate to `dataworkflow`.
- [x] Added package-level regression coverage for nested artifact prompt
      compaction, ledger projection, related material handles, and bounded
      entity samples.

Remaining architecture items:

- [x] Move field-contract, zero-match, unmatched-resolution, and zero-eligible
      issue discovery behind package-level artifact/schema inputs instead of
      REPL prompt views.
- [ ] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 315: Package-Level Field Contract Issue Discovery

Zero-match filter, unmatched resolution, and zero-eligible qualification issue
discovery already lived in `internal/dataworkflow`, but field-contract issue
discovery still parsed typed guard errors and zero-match extraction artifacts in
the REPL. That left one important workflow diagnostic path tied to REPL prompt
views rather than package-level artifact/schema inputs.

This batch adds `FieldContractIssue` and package-level discovery over
`WorkflowRecord`, `ArtifactAccessView`, and allowed typed action names. The
REPL now delegates field-contract discovery to `dataworkflow`, keeping only the
existing workflow-state eligibility gate and a thin compatibility wrapper for
older tests.

Generic invariants:

- discovery consumes typed workflow records and artifact/schema views;
- regex parsing is limited to system-generated typed guard/error messages, not
  user prose or model narrative;
- candidate artifacts are derived from artifact fields and aliases;
- numeric-looking hints use field samples as soft repair guidance only;
- no business-domain field names or intent keyword matching are introduced.

Changes:

- [x] Added `FieldContractIssue`.
- [x] Added `FieldContractIssueInput`.
- [x] Added `DiscoverFieldContractIssues`.
- [x] Added package-level artifact-access-to-schema projection helpers.
- [x] Moved planning missing-field, numeric contract, and zero-match
      `extract_fields` issue discovery from REPL to `dataworkflow`.
- [x] Rewired REPL workflow state to delegate issue discovery to
      `dataworkflow`.
- [x] Added package-level regression coverage for typed guard errors and
      zero-match extraction artifacts.

Remaining architecture items:

- [x] Convert process rendering to consume runtime/reducer events rather than
      REPL-local stage summaries.

### Batch 316: Package-Level Process Display Projection

Data workflow process lines had one remaining duplicated surface: REPL and CLI
both rendered workflow stage titles, source-lane hints, default explanations,
and model-authored goal/batch/next-step details from local string switches.
That kept user-facing progress close to UI-specific control flow even though
the runtime already emits typed `WorkflowJournalEvent` records.

This batch adds `WorkflowProcessDisplay` as a package-level projection over
typed process events. The data workflow package now owns the stable display
shape: label, deterministic title segments, and low-noise detail lines. REPL
and CLI only choose how to render the projection: bordered permanent lines for
REPL and stderr progress lines for CLI. The projection is intentionally
domain-neutral: it uses model-authored goal, batch purpose, next step, action
summary, typed decisions, guard failures, and audit details; it does not encode
business roles, file names, field names, or intent keywords.

Generic invariants:

- process display is a projection of typed workflow events, not a hard gate;
- deterministic stage/source indicators stay in the title line;
- business-facing goal, batch, next-step, action, decision, failure, and audit
  details stay in low-noise detail lines with typed display keys;
- REPL and CLI share the same display projection and cannot drift by copying
  stage-label switches;
- stdout remains reserved for final answers in CLI data mode.

Changes:

- [x] Added `WorkflowProcessDisplay`.
- [x] Added `WorkflowProcessDisplayDetail` with typed display keys.
- [x] Added `BuildWorkflowProcessDisplay` over `WorkflowJournalEvent`.
- [x] Moved workflow stage title segments and source-lane hints into
      `dataworkflow`.
- [x] Moved default process explanations into the package-level display
      projection.
- [x] Rewired REPL workflow progress to render the shared display projection.
- [x] Rewired CLI workflow progress to render the shared display projection on
      the existing stderr progress sink.
- [x] Added package-level regression coverage for stable title segments,
      model-authored process intent, and default process details.

Remaining architecture items:

- [x] Append live process events through the runtime journal sink at each
      execution/admission/evaluation boundary, so process histories no longer
      need to be reconstructed from records after the fact.
- [ ] Feed reducer decision reasons and typed guard summaries directly into
      `WorkflowProcessEventInput` at every CLI/REPL call site instead of
      passing legacy detail strings.
- [ ] Move the remaining detail-marker compatibility layer out of REPL once
      all progress callers emit typed process events.

### Batch 317: Runtime-Owned Process Event Sink

`WorkflowRuntime` already owned current plan, records, deferred queue,
admission decisions, and plan transitions. Process events were the remaining
piece that journals could only rebuild after the fact from workflow records,
with ad hoc preview records appended by checkpoint writers. That made the
journal less faithful to live execution boundaries and kept later CLI/REPL
event rendering too close to local call-site strings.

This batch adds a storage-neutral process-event sink to `WorkflowRuntime`.
Appending a workflow record now appends the corresponding typed process event
at the same round. Callers can also append explicit process events for
evaluation, guard, admission, or future reducer boundaries. Journal snapshots
prefer runtime live events and append typed preview events only for records
that are intentionally supplied outside the current runtime state.

Generic invariants:

- process events are workflow IR state, not REPL/CLI UI state;
- runtime process events are cloned on input/output and cannot be mutated by
  callers;
- journal snapshots prefer live process events when present;
- preview records used for checkpoints still get typed process events without
  mutating runtime records;
- no prompt prose or business-domain meaning participates in control flow.

Changes:

- [x] Added runtime-owned process-event storage.
- [x] Added `AppendProcessEvent`.
- [x] Added `AppendProcessEventFromInput`.
- [x] Added `ProcessEvents`.
- [x] Made `AppendRecord` append a typed process event for the appended record.
- [x] Made `SetRecords` rebuild process events from the supplied snapshot
      records so resume/checkpoint state stays internally consistent.
- [x] Made journal snapshots prefer runtime live process events, while
      appending typed preview events for records beyond runtime state.
- [x] Added regression coverage for runtime event ownership and live-event
      journal precedence.

Remaining architecture items:

- [x] Feed reducer decision reasons and typed guard summaries directly into
      `WorkflowProcessEventInput` at every CLI/REPL call site instead of
      passing legacy detail strings.
- [ ] Move the remaining detail-marker compatibility layer out of REPL once
      all progress callers emit typed process events.

### Batch 318: Typed Guard Context For Process Details

After the runtime process-event sink existed, REPL/CLI still rendered several
hard-gate repair progress lines by passing a legacy failure detail string. That
preserved visible output but lost typed guard context at the display boundary:
guard code, guard severity, repairability, and plan intent had already existed
as IR, yet the UI bridge only saw clipped prose.

This batch adds a typed guard-context bridge that builds
`WorkflowProcessEventInput` from the current plan and `GuardResult`, then
projects it through the shared process display. Terminal and staging workflow
guards in both REPL and CLI now render repair progress from typed guard events.
Ordinary execution/runtime errors remain plain failure details until they have
typed guard objects; the system does not invent structured meaning from error
text.

Generic invariants:

- typed guard summaries flow through `WorkflowProcessEventInput`;
- display uses typed detail keys/classes and raw values, not rendered-prefix
  parsing;
- guard codes are surfaced as low-noise audit details;
- plain runtime errors are not upgraded to hard structured facts;
- no business-domain field names or prompt prose drive control flow.

Changes:

- [x] Added raw `value` to `WorkflowProcessDisplayDetail` so REPL marker
      compatibility can consume typed values without parsing localized text.
- [x] Added `dataTaskWorkflowGuardContextDetails`.
- [x] Rewired REPL terminal/staging guard repair progress to use typed guard
      context details.
- [x] Rewired CLI terminal/staging guard repair progress to use typed guard
      context details.
- [x] Added regression coverage that typed guard context carries plan intent,
      guard reason, and guard code without double-prefixing the user-visible
      reason.

Remaining architecture items:

- [ ] Move the remaining detail-marker compatibility layer out of REPL once
      all progress callers emit typed process events.

### Batch 319: Typed Process Event Rendering Bridge

The previous batches gave the runtime a process-event sink and taught guard
repair lines to carry typed guard context. The main execution path still had
one UI adapter seam: execute/result/evaluate progress lines converted typed
plan/result/decision state into legacy marker strings before rendering.

This batch adds typed process-event renderers for REPL and CLI. Callers that
already have typed plan/result/decision/guard state can now pass a
`WorkflowJournalEvent` directly to the shared display projection. The renderer
then applies the existing UX rules: deterministic title segments stay compact;
result/evaluate lines avoid repeating business context; business goal/batch
next-step/action, decision, failure, and audit details remain low-noise
permanent details.

Generic invariants:

- event rendering consumes `WorkflowJournalEvent`, not marker strings;
- result progress uses typed `dataquery.Result` audit state;
- evaluate progress uses typed `WorkflowDecision`;
- guard repair progress uses typed `GuardResult`;
- raw runtime errors and ad hoc continuation reasons remain legacy strings
  until they have typed event inputs; the system does not infer structure from
  prose.

Changes:

- [x] Added REPL `emitDataTaskWorkflowEvent`.
- [x] Added CLI `dataTaskCLIWorkflowEventProgress`.
- [x] Added typed event detail rendering that consumes
      `WorkflowProcessDisplayDetail` keys/classes/values directly.
- [x] Rewired REPL execute/result/evaluate progress to typed process events.
- [x] Rewired CLI execute/result/evaluate progress to typed process events.
- [x] Rewired REPL/CLI terminal and staging guard repair progress to the typed
      event renderer.

Remaining architecture items:

- [x] Convert raw continuation/fallback/retry reasons into typed
      `WorkflowProcessEventInput` where they are system decisions.
- [x] Remove the legacy detail-marker compatibility functions after all
      remaining raw-string progress callers have typed event inputs.

### Batch 320: Remove REPL Detail Marker Compatibility

After typed event rendering landed, raw continuation/fallback/retry progress
still passed system decision reasons through the old detail-string path. That
kept marker parsing alive even though the underlying facts were no longer
model prose: they were system decisions such as deferred dispatch, deterministic
fallback, structural patch, or repair/failure status.

This batch converts those decisions into typed process events. The shared
process display now renders `WorkflowJournalEvent.Reason` as a decision detail
or, when the event status is failed/blocked/rejected, as a failure reason.
REPL and CLI progress no longer call the legacy marker path. The old marker
constants, marker constructors, marker parser, old audit/progress functions,
and marker-based tests were removed.

Generic invariants:

- system decisions use typed process event fields;
- failed/blocked/rejected statuses render reason as failure context;
- non-failure continuation/fallback reasons render as decision context;
- no raw progress caller parses localized display text;
- CLI still writes progress to the progress writer/stderr path, not stdout;
- no business-domain roles or field names are introduced.

Changes:

- [x] Added process-display support for `WorkflowJournalEvent.Reason`.
- [x] Added REPL reason/failure process event helpers.
- [x] Added CLI reason/failure process event helpers.
- [x] Rewired REPL continuation, deferred, patch, repair, and fallback
      progress to typed events.
- [x] Rewired CLI continuation, deferred, patch, repair, resume, and fallback
      progress to typed events.
- [x] Rewired plan progress details to typed event detail lines.
- [x] Removed legacy REPL detail-marker functions and the old string parser.
- [x] Updated UX regression tests to assert typed event rendering instead of
      marker stripping.

Remaining architecture items:

- [x] Run focused regression suites and full build/test before considering the
      IR process-event/display gap closed.

### Batch 321: Live Process Events In Runtime And Terminal Audit

Batch 316-320 moved data workflow process display away from REPL marker
strings, but a deeper audit gap remained: CLI/REPL could render typed process
events while terminal audit snapshots still reconstructed process history from
records only. That lost explicit system boundaries such as resume, deferred
queue changes, deterministic continuation, repair/failure context, patch
decisions, execute/result/evaluate progress, and guard-driven repairs.

This batch connects the live process-event stream to `WorkflowRuntime` at the
CLI/REPL entrypoints and carries that stream into terminal audit snapshots. The
renderer remains a consumer of typed events; it does not feed control flow.
Terminal JSON now preserves the same process event IR that users saw in the
process output.

Generic invariants:

- process events are runtime workflow state before they are UI lines;
- CLI and REPL append process events before rendering them;
- terminal audit snapshots prefer live runtime process events when available;
- record-derived events remain a fallback for older/static snapshots;
- process events remain domain-neutral and do not hard-code business roles,
  field names, material names, or user-intent keywords;
- stdout remains reserved for final answers in CLI mode.

Changes:

- [x] Added CLI runtime process-event append wrappers around all typed workflow
      progress rendering.
- [x] Added REPL runtime process-event append wrappers around all typed workflow
      progress rendering.
- [x] Carried CLI runtime process events into terminal audit JSON.
- [x] Let REPL terminal audit capture the active data workflow runtime process
      events when a data turn is active.
- [x] Preserved record-derived event reconstruction for terminal snapshots that
      do not have a live runtime.
- [x] Added regression coverage that a CLI repair workflow terminal audit
      includes live `execute`, `repair`, `result`, and `evaluate` process
      events.

Remaining architecture items:

- [x] Run focused regression suites before continuing the IR closure audit.

### Batch 322: Runtime-Built Terminal Journal Snapshots

Batch 321 carried live process events into terminal audit output, but terminal
JSON was still assembled by passing records and selected fields into the
journal builder. That kept another audit seam open: plan transitions, current
plan, deferred queue, and runtime process events could diverge from the
terminal snapshot if callers forgot to thread one field.

This batch routes CLI and REPL terminal audit writing through
`WorkflowRuntime.BuildJournalSnapshot` whenever a live data workflow runtime is
available. The static writer remains as a compatibility fallback for tests and
older helper paths that only have records.

Generic invariants:

- terminal audit is a runtime snapshot, not a separate records-only projection;
- current plan, deferred plan/queue, plan transitions, process events, and
  state graphs come from the same runtime handle when present;
- records-only reconstruction remains available for static/offline snapshot
  helpers;
- no data-task business semantics or prompt prose participate in hard gates.

Changes:

- [x] Added a terminal audit writer variant that accepts `WorkflowRuntime`.
- [x] Rewired CLI terminal audit to use the runtime-backed writer.
- [x] Rewired REPL terminal audit to use the active data workflow runtime.
- [x] Kept the old records-only writer as a thin fallback wrapper.
- [x] Extended CLI regression coverage to assert terminal audit includes live
      process events, `plan_transitions`, a repair transition, and
      `current_plan`.

Remaining architecture items:

- [x] Run focused regression suites before continuing the IR closure audit.

### Batch 323: Runtime-Owned Round Counters

The runtime-backed journal snapshot closed one audit divergence, but CLI and
REPL still advanced `data_rounds` and `repair_rounds` through local loop
variables. That left a small but important state-ownership gap: terminal
audits, checkpoints, process events, and future resume/reducer logic could
observe different round values if an entrypoint forgot to mirror a local
counter into `WorkflowRuntime`.

This batch makes round advancement a runtime-owned operation. Existing local
variables remain as short-lived mirrors while the larger loop migration
continues, but all data-workflow execution and repair round mutations now go
through the runtime API.

Generic invariants:

- `WorkflowRuntime` owns data/repair round mutation;
- CLI/REPL may mirror returned values for legacy call signatures, but they do
  not independently advance data workflow counters;
- checkpoint resume restores counters into the runtime before continuing;
- helper-returned repair counts are synchronized back into the runtime;
- command-operation repair counters remain outside the data workflow runtime;
- no business-domain roles, field names, prompt prose, or user-intent keywords
  participate in round ownership.

Changes:

- [x] Added clamped `WorkflowRuntime.SetRounds`.
- [x] Added `IncrementDataRound` and `IncrementRepairRound` runtime APIs.
- [x] Rewired CLI data execution rounds to advance through `WorkflowRuntime`.
- [x] Rewired CLI data repair round sync after helper repair planning and
      evaluator-triggered repair.
- [x] Rewired REPL data execution and repair rounds to advance through
      `WorkflowRuntime`.
- [x] Added runtime unit coverage for round ownership and clamped restore.
- [x] Extended CLI terminal-audit regression coverage for persisted
      `data_rounds` and `repair_rounds`.

Remaining architecture items:

- [x] Run focused regression suites and full build/test before continuing the
      IR closure audit.

### Batch 324: Runtime-Owned Deferred Dispatch Mutations

The deferred queue had already moved into `WorkflowRuntime`, but ready dispatch
still had one adapter-owned write path: CLI/REPL computed an updated
`DeferredQueueState` after popping a ready rank and then wrote it back with
`SetDeferredQueue`. That made the runtime a storage holder instead of the owner
of the queue transition, and it left dispatch events vulnerable to caller
drift.

This batch keeps readiness calculation unchanged, but moves the actual
dispatch mutation through `WorkflowRuntime.DispatchDeferred`. The live runtime
now records the dispatch transition and owns the remaining deferred plan.
CLI/REPL only receive the next executable batch and then ask the runtime for
the remaining queue state.

Generic invariants:

- deferred queue transitions are runtime mutations, not adapter assignment;
- dispatch events are recorded by the same runtime handle used for audit and
  workflow state;
- readiness/admission checks remain typed and deterministic;
- CLI/REPL may still compute ready candidates while the larger reducer
  migration continues, but they no longer write updated queue state directly;
- no business-domain roles, field names, or prompt prose control dispatch
  mutation.

Changes:

- [x] Let `WorkflowRuntime.DispatchDeferred` return the cloned updated queue.
- [x] Rewired CLI deferred dispatch to call `DispatchDeferred` instead of
      `SetDeferredQueue(updatedQueue)`.
- [x] Rewired REPL deferred dispatch to call `DispatchDeferred` instead of
      `SetDeferredQueue(updatedQueue)`.
- [x] Added runtime regression coverage for cloned deferred dispatch state and
      typed dispatch events.
- [x] Removed the old adapter helper that returned an externally updated queue
      for callers to write back.

Remaining architecture items:

- [x] Run focused regression suites and full build/test before continuing the
      IR closure audit.

### Batch 325: Workflow State View Type Ownership

The live workflow loop still has adapter code, but the `workflow_state_json`
schema is no longer an adapter concern. Before this batch,
`dataTaskWorkflowStateView` was a large REPL-local struct even though it carried
the core state consumed by planners, evaluators, process events, journal
snapshots, and tests. That made the architectural boundary misleading: the
state looked like REPL UI data, while it was actually data-workflow IR.

This batch moves the workflow-state JSON schema into `internal/dataworkflow` as
`WorkflowStateView`. REPL keeps a type alias while construction helpers are
migrated in smaller steps. The runtime behavior and prompt semantics are
unchanged; only ownership of the schema moves to the workflow package.

Generic invariants:

- `workflow_state_json` is a data-workflow IR schema, not a REPL-owned struct;
- REPL/CLI may adapt and populate the schema, but they do not define it;
- the view remains domain-neutral: it carries materials, action graph, ledger
  graph, artifact graph, progress, scaffold, typed violations, and output
  contract state without business-specific roles;
- moving the type does not alter hard gates, prompts, or current action
  semantics.

Changes:

- [x] Added `dataworkflow.WorkflowStateView` with the existing
      `workflow_state_json` schema.
- [x] Replaced the REPL-local `dataTaskWorkflowStateView` struct with a type
      alias to `dataworkflow.WorkflowStateView`.
- [x] Added dataworkflow-level schema coverage for key workflow-state JSON
      fields.

Remaining architecture items:

- [x] Run focused regression suites and full build/test before continuing the
      IR closure audit.

### Batch 326: Workflow State View Structural Derivations

After moving the `workflow_state_json` schema into `internal/dataworkflow`, the
next adapter-owned seam was structural derivation. REPL wrappers still rebuilt
stage facts, missing validation stages, allowed next actions, and allowed action
contracts from the state view. Those are not UI rules; they are pure workflow
state operations.

This batch moves those derivations onto `WorkflowStateView` methods. The REPL
helpers remain as thin compatibility wrappers while call sites are migrated.
The derivation accepts either the expanded boolean fields or the embedded
workflow contract view, so future constructors can populate the state in a more
compact form without losing reducer behavior.

Generic invariants:

- stage facts are derived from typed state, not REPL logic;
- allowed action contracts/actions are derived by the workflow package;
- missing validation stages are workflow-state methods;
- direct boolean fields and `workflow_contract` requirements are reconciled
  structurally;
- no business-domain fields, regexes, or prompt prose participate in the
  derivation.

Changes:

- [x] Added `WorkflowStateView.Facts`.
- [x] Added `WorkflowStateView.ComputedNextStage`.
- [x] Added `WorkflowStateView.MissingValidationStages`.
- [x] Added `WorkflowStateView.ComputedAllowedNextActionContracts`.
- [x] Added `WorkflowStateView.ComputedAllowedNextActions`.
- [x] Rewired REPL compatibility helpers to delegate to the dataworkflow
      methods.
- [x] Added schema/method regression coverage for contract-backed state facts.

Remaining architecture items:

- [x] Run focused regression suites and full build/test before continuing the
      IR closure audit.
