# Relation Member-Set Handoff Gap

Date: 2026-05-27

## Customer Symptom

`../../customlogs/subagent_err.log` contains two turns about the same relation
question:

- `哪个agent可以调用subagent，为什么，请详细说明`
- follow-up correction: `我问的是哪个“agent”可以调用subagent！Orchestrator不是agent!`

The explorer eventually produced useful model-authored prose: the Agent layer
uses `propose_sub_agents`, Orchestrator is a dispatcher rather than an Agent,
and the tool is name-scoped. The final answer still drifted into a wrong
principal set: it listed Orchestrator or multiple generic Agents instead of the
qualifying Agent member.

## Ground Truth From Code

The current repository answer is:

- the principal qualifying Agent is `explorer`;
- Orchestrator dispatches/executed proposals but is not an Agent answer member;
- Analyzer/Finalizer do not automatically get `propose_sub_agents` unless a
  same-name SubAgent is registered in the future.

Current-code anchors:

- `internal/agent/agent.go`: `buildToolSchemas` injects `ProposeSubAgents` only
  when `SubAgents.Get(string(b.name))` succeeds, and then scopes the schema to
  the current Agent name.
- `internal/agent/subagent.go`: `RegisterDefaultSubAgents` registers only
  `NewSubExplorer`.
- `internal/agent/sub_explorer.go`: `SubExplorer.Name()` returns `explorer`.
- `internal/tool/propose_sub_agents.go`: the tool documentation is name-scoped.
- `internal/orchestrator/orchestrator.go`: Orchestrator extracts and runs the
  proposal, which is a scheduler/executor relation, not the qualifying Agent
  member itself.

## Existing Mechanisms

The system already has most of the correct infrastructure:

- `types.RequiresRelationMemberSetHandoff` describes the strict typed contract
  for relation lookup questions that need a principal `aggregate_facts.member_set`.
- `emit_investigation_complete` has a relation pre-complete downgrade that asks
  the model to emit a typed `member_set` before closing.
- finalizer renders the `Required Principal Member Set` contract and the
  pre-emit oracle rejects missing members when such a set exists.
- `TypedRelationHint`, `TypedRelationCandidateSource`, and the relation dossier
  docs define navigation/advisory relation carriers without making graph rows
  final answers.
- `InvestigationNotes` preserve bounded explorer prose, but they are advisory
  and intentionally weaker than typed evidence/aggregate facts.

## Root Cause

The failure is not a finalizer-only bug. The upstream typed route did not carry
the relation answer as a principal member set.

In the customer log, the analyzer rendered the turn as:

- `intent=explain`
- `question_kind=mechanism`
- `scenario=architecture_explain`
- entities: `Agent`, `SubAgent`, `Orchestrator`

That route lets exploration close without `aggregate_facts.member_set`. Because
the model's correct direct conclusion existed only in free-form exploration
prose, finalizer had to reconstruct the answer from scattered evidence. It then
mixed two different relation roles:

- **qualifying member relation:** which Agent has the sub-agent proposal tool;
- **execution relation:** which scheduler/runtime executes a proposal after an
  Agent emits one.

The structured information that should have survived downstream was:

1. principal relation member set: `explorer`;
2. explicit exclusion/boundary: Orchestrator is a dispatcher, not an Agent
   answer member; Analyzer/Finalizer lack same-name SubAgent registration under
   the current default registry;
3. citable code chain: tool injection gate, default registry, `SubExplorer.Name`,
   tool name scope, and Orchestrator execution path.

## Redlines

- Do not match raw user keywords or assistant prose to decide control flow.
- Do not hard-code Codrax-specific relations such as `explorer` or
  `propose_sub_agents` into general logic.
- Do not synthesize missing answer members from raw prose or graph guesses.
- Do not let system-generated relation candidates replace the model's chosen
  answer.
- Hard rejection is allowed only when the typed request shape, exact carrier,
  grounded same-member evidence, and local repair path are all present.

## Generalized Design

### 1. Preserve Strict Contract Where Typed Shape Is Clear

Keep the current strict relation-member-set handoff for requests already typed
as set-valued relation lookups: `is_relational_lookup=true` plus count,
category/enumeration, or another machine-checkable principal-set obligation.
This is the only path that may block `emit_investigation_complete`.

### 2. Add Advisory Relation-Surface Handoff For Ambiguous Mechanism Routes

For routes that are relation-shaped but not strict enough to hard block, render
a compact finalizer advisory section from structured carriers only:

- accepted `EvidenceRelationship` / `EvidenceRegistration` rows;
- citable relation-like `EvidenceItem` rows with subject/object/anchor fields;
- existing typed relation hints and source-inventory relation observations;
- model-authored aggregate facts if present.

This section must say it is advisory, not the final answer. Its job is to make
role boundaries visible to finalizer so it does not confuse a qualifying member
with a dispatcher, helper, registry, or runtime.

Trigger source: typed fields and structured evidence only. No raw prompt text,
no localized keyword table, and no parsing of free-form model prose.

### 3. Nudge, Do Not Rewrite, When Closure Lacks A Member Set

If exploration closes without a principal `member_set` but structured relation
evidence exists, finalizer should see an advisory "relation roles observed"
handoff. The system may recommend that the answer start with any model-emitted
principal member set if one exists, but it must not invent one.

For future improvement, analyzer can emit a clearer typed signal for
"relation-qualified member lookup" when the LLM is confident. That should be a
schema field or profile, not a Go keyword heuristic.

### 4. Keep Relation Evidence Richness End To End

Relation evidence rows should preserve:

- relation role: qualifying member, dispatcher/executor, registry, helper,
  exclusion/boundary, or unknown when not known;
- source file and line;
- subject/object/anchor surfaces;
- current-source vs external/artifact provenance;
- salience/context role so finalizer can distinguish principal facts from
  supporting architecture.

Existing enrichment limits should remain bounded, but relation rows should not
be starved by generic context rows when the request carries typed relation
shape.

## Task List

- [x] Record the customer symptom, root cause, redlines, and design.
- [ ] Audit current analyzer/explorer/finalizer relation handoff paths and
      identify which existing code can be reused.
- [ ] Add a bounded advisory relation-surface handoff section for finalizer,
      sourced only from structured relation evidence/carriers.
- [ ] Preserve/raise relation evidence rows in typed exploration enrichment
      without turning them into principal answer members.
- [ ] Add regression tests proving:
      - raw objective text alone does not create a relation handoff;
      - structured relation evidence does render an advisory handoff;
      - strict `RequiresRelationMemberSetHandoff` behavior remains unchanged for
        mechanism-only relation explanations;
      - an accepted principal relation `member_set` still wins over advisory
        rows.
- [ ] Run focused unit tests.
- [ ] Commit and push each completed batch.

## Expected Effect

For the customer case, finalizer should receive structured relation role context
showing that "Agent tool eligibility" and "Orchestrator execution" are separate
relations. If the model emits a principal `member_set`, existing hard gates
preserve it. If it does not, the system still does not fabricate `explorer`, but
it gives finalizer enough structured context to avoid substituting dispatcher
roles for qualifying members.

The same design generalizes to:

- which handlers route to an endpoint;
- which classes implement an interface or override a method;
- which modules import/export a symbol;
- which config sources set a key;
- which runtime/log/trace observation maps to a current-source anchor;
- multi-repo and mixed-language relation investigations.
