# Operation Material Convergence

## Problem

Operation execution now has two useful but separate loops:

- command operations use a goal loop with replan and continuation;
- MCP/local Skill operation providers use a bounded workflow DAG with
  `next_actions`, `return_action`, and `workflow_state`.

Both loops can produce external material that may be needed for the next
decision:

- command output payloads;
- large files, logs, traces, web pages, manuals, and help text;
- MCP/provider payloads and rowsets;
- Skill artifacts and generated files;
- links/files/manuals mentioned in provider descriptors or workflow steps;
- provider-authored `next_actions` and `return_action` handoffs.

The current handoff shape exposes some of these as raw strings, but it does not
make "this is a saved material that may need bounded follow-up
read/search/extract" a first-class concept. As a result, a model can stop after
seeing a truncated preview or a provider summary even though the full payload
or the linked material is available.

## Current Architecture Audit

### Command Path

`executeCommandOperationPlanAttempt` already owns a bounded command goal loop:

- lint before execution;
- execute command batch;
- replan failed/invalid batches;
- continue when the planner asks for more observations;
- synthesize the final answer from accumulated command records.

This path is structurally sound, but its command-result prompt needs typed
material metadata so the planner does not mistake a preview for the full
artifact.

### Provider / MCP / Skill Path

`executeProviderOperationFlow` already owns a bounded provider workflow DAG:

- execute current provider action;
- parse provider output;
- queue provider `next_actions`;
- queue `return_action`;
- keep workflow state.

This path is also structurally useful, but it has a gap: provider results flow
directly to final answer synthesis once the provider DAG is empty. There is no
shared material ledger that can tell a later command planner or provider
planner, "this payload/artifact/link is the next thing to inspect."

### Prompt Skill / Descriptor Path

Operation provider descriptors and workflow steps are visible to the planner as
selection cards. These descriptors can mention manuals, files, URLs, examples,
or supporting resources. That text is workflow guidance, not proof that the
resource content has been read. The planner must treat it as a material to
fetch/read/pass when relevant.

## Red Lines

- Do not route source-code, trace/log evidence, or write-mode tasks into
  operation by keyword. Typed route/classifier signals remain authoritative.
- Do not make hard gates from noisy preview text or model prose.
- Keep operation materials as external execution observations, not source-code
  citations.
- Keep final answers separate from process details; material previews are for
  planner handoff and REPL execution panels.
- Keep high-risk approvals and destructive denial deterministic.

## Target Design

### Operation Material Record

Add a small typed operation material record used by both command and provider
paths:

- `source`: command, provider, mcp, skill, workflow, descriptor.
- `kind`: payload_ref, artifact_ref, linked_resource, next_action,
  return_action, workflow_state, preview_only.
- `ref`: file path, URI, artifact ID, or compact action reference.
- `role`: saved_payload, generated_artifact, supporting_manual,
  followup_action, return_handoff, workflow_context.
- `summary`: compact human/model readable description.
- `lines/bytes`: when known.
- `complete_preview`: false when the visible output is only a preview.

This record is advisory data for planning and final synthesis. It does not
grant execution permission.

### Unified Material Handoff

Every operation result prompt should include a compact material section:

- command result: derive from `payload_ref`, output kind, output size, and
  preview truncation;
- provider result: derive from `payload_ref`, `artifact_refs`,
  `next_actions`, `return_action`, and `workflow_state`;
- memory/handoff: preserve material refs as operation lane facts.

This makes the model see the same concept regardless of where the output came
from.

### Follow-Up Rule

If the user goal depends on omitted material content and the current
observation is only a preview, summary, payload ref, artifact ref, linked
resource, or descriptor mention, the next step should be one of:

- bounded read/page;
- targeted search;
- context extraction;
- HTML/text/log/trace parsing;
- provider workflow action using that material as input.

The system should not require the model to know every command. The model
chooses the concrete command/provider action; Codrax supplies material refs,
approval policy, budgets, lint, and result handoff.

### Provider Continuation Roadmap

Short term:

- provider answer prompts must not treat provider summaries as complete
  material when material refs exist;
- provider result handoff must expose material refs and workflow actions.

Medium term:

- add a unified operation evaluator that can decide after a provider run:
  `complete`, `continue_command`, `continue_provider`, `needs_approval`,
  `blocked`, `budget_exhausted`, or `partial_answer_possible`;
- allow provider workflow `next_actions` to target command-operation
  extraction in a typed way, not only another provider.

Long term:

- unify command batches and provider actions as node kinds inside one operation
  workflow instance. The command loop and provider DAG then become two
  executors behind a shared goal evaluator and material ledger.

## Implementation Batches

### Batch 1: Material Record and Prompt Handoff

- [x] Add typed operation material helpers.
- [x] Derive command materials from payload refs and output summaries.
- [x] Derive provider materials from payload/artifact refs and workflow
      actions.
- [x] Render materials in command and provider prompt contexts.
- [x] Add tests for command payloads and provider payload/artifact/next-action
      material handoff.

### Batch 2: Descriptor Teaching

- [x] Teach planner that provider/Skill descriptor links/files/manuals are
      resources to read/pass, not already-read content.
- [x] Ensure capability snapshot remains compact and does not leak execution
      details.
- [x] Add tests for provider workflow descriptors with supporting resources.

### Batch 3: Provider Result Guardrails

- [x] Teach provider final-answer prompt to distinguish provider summary from
      full material.
- [x] Preserve material refs in operation memory.
- [x] Add tests that provider final prompt includes material refs and does not
      treat summaries as full content.

### Batch 4: Unified Evaluator (future)

- [ ] Add provider-aware operation evaluator statuses.
- [ ] Let provider completion request command follow-up when a local payload
      ref requires extraction.
- [ ] Let provider next actions target a command-operation extraction node.
- [ ] Keep budgets and approval policy deterministic.

## Status

This document records the architecture correction. The first code batch should
only add shared material handoff and prompt-visible typed metadata. It should
not change route classification, source evidence lanes, trace/log analysis, or
write-mode behavior.
