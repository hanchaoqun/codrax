# Operation Skill Manifest Workflows

**Status:** design and task ledger before implementation.  
**Date:** 2026-06-03  
**Scope:** make local `operation_skills[]` workflows a first-class typed
manifest capability. This is operation-lane only and must not change source
analysis, trace/log analysis, MCP read-mode observations, or write-mode code
flows.

## 1. Current Code Audit

The operation lane already has reusable infrastructure:

- `internal/repl/turn_policy.go` owns typed operation routing. No Go-side
  keyword matching is needed or allowed.
- `internal/operation/plan.go` matches operation providers by typed
  `operation_kind` and `target_surface`.
- `internal/operation/capability.go` renders compact operation provider
  descriptors only into the operation planner capability snapshot.
- `cmd/root.go` converts `operation_skills[]` and lazy MCP operation provider
  config into `operation.ProviderInfo`.
- `internal/repl/repl.go` can execute a selected provider after approval,
  consume `next_actions[]`, preserve workflow state, queue follow-up actions,
  and hand results into current-run handoff plus operation memory.

The missing piece is manifest expressiveness. Current local skill config has
description/examples/schema but no typed `workflows[]`; therefore a model can
learn a workflow from prose, but deterministic provider matching cannot see
that `skill:manual_reader` is also an entry provider for
`external_skill_workflow` on `slides`.

## 2. Gap

Real operation requests often look like:

1. read a manual or large artifact;
2. extract command templates or reusable facts;
3. call a second provider to generate an artifact;
4. return to the first provider or planner for final synthesis.

The existing provider result protocol can already represent steps 2-4 through
`next_actions[]` and `workflow_state`. What is not represented at startup is
the workflow catalog that helps the planner choose the right local skill before
execution.

## 3. Red Lines

- `description`, `when_to_use`, `when_not_to_use`, and workflow summaries are
  soft model guidance only.
- Hard execution matching must remain typed:
  `operation_kind` + `target_surface` + configured provider descriptor.
- `command`, `args`, `env`, and `work_dir` must not be rendered into the model
  prompt.
- Empty or absent `workflows[]` must preserve current behavior.
- Operation skill outputs remain operation artifacts/observations, never
  current-source citations.
- Code, trace, log, MCP read-only, and write-mode paths must not be entered or
  rerouted by operation skill prose.

## 4. Target Manifest Shape

```yaml
operation_skills:
  - name: manual_reader

    # Always-visible selection card.
    description: "Read a tool manual, extract command templates, and hand off."
    when_to_use:
      - "Use when the task requires learning an unfamiliar CLI/tool manual."
    when_not_to_use:
      - "Do not use for source-code explanation without an operation goal."
    operation_kinds: ["artifact_generation"]
    operation_surfaces: ["local_file"]
    operation_side_effects: ["local_file_write"]
    operation_requires_confirmation: true

    # Lazy workflow catalog.
    workflows:
      - name: manual_to_deck
        summary: "Read a manual, extract key commands, then call a deck builder."
        entry: true
        operation_kind: external_skill_workflow
        target_surface: slides
        next_providers: ["skill:ppt_builder"]
        return_provider: "skill:manual_reader"
        steps:
          - id: read_manual
            provider: "skill:manual_reader"
            operation_kind: external_skill_workflow
            target_surface: local_file
            description: "Read and summarize the manual."
          - id: build_deck
            provider: "skill:ppt_builder"
            operation_kind: presentation_generation
            target_surface: slides
            description: "Generate a deck from the extracted summary."

    # Execution details, never rendered into prompts.
    input_schema: |
      {"manual_path":"string","output_path":"string"}
    output_contract:
      artifact_refs: true
      payload_ref: true
      next_actions: true
      return_action: true
      workflow_state: true
    command: "./tools/manual_reader"
    args: ["--json"]
    input_mode: stdin_json
```

Backwards-compatible aliases remain supported:

- `operation_description` is still accepted; `description` is the preferred
  prompt-safe field.
- `operation_input_schema` is still accepted; `input_schema` is the preferred
  prompt-safe field.
- `operation_examples` is still accepted; `examples` is the preferred
  prompt-safe field.

## 5. Provider Mapping

Each local skill still produces provider descriptors for
`operation_kinds[]`. In addition:

- every workflow with `entry: true` and `operation_kind` produces an additional
  provider descriptor using the same `skill:<name>` executable provider;
- workflow `target_surface` is added to the descriptor surfaces;
- workflow descriptors carry the prompt-safe workflow catalog;
- duplicate descriptors are normalized by provider name, kind, and tool.

This directly supports a model plan such as:

```json
{
  "operation_kind": "external_skill_workflow",
  "target_surface": "slides"
}
```

without relying on keyword matching.

## 6. Prompt Teaching

The operation capability snapshot should show:

- provider name, kind, surfaces, side effects, approval gate, lazy state;
- compact description;
- `when_to_use` and `when_not_to_use`;
- compact input schema and examples;
- workflow catalog: workflow name, summary, entry flag, typed kind/surface,
  next providers, return provider, and short step list.

The snapshot must stay compact and bounded. It must not include command/env
execution details.

## 7. JSON Compatibility

This batch mostly adds YAML/config fields and prompt-safe provider descriptors,
not new model tool-call JSON fields. Existing provider result JSON repair for
`next_actions`, `return_action`, and `workflow_state` remains the execution
handoff contract.

If a future planner schema adds explicit `workflow_name`, that field must be
added to the same JSON repair/alias layer before it is consumed by hard gates.

## 8. Delivery Tasks

### Batch A: Design Ledger

- [x] Audit existing local skill provider, planner snapshot, provider matching,
      workflow chaining, docs, and tests.
- [x] Record typed manifest workflow design, red lines, mapping, prompt
      teaching, and test plan.

### Batch B: Types and Provider Mapping

- [x] Add prompt-safe workflow config structs under `types`.
- [x] Add preferred `description`, `when_to_use`, `when_not_to_use`,
      `input_schema`, `examples`, `workflows`, and `output_contract` fields to
      `OperationSkillConfig`.
- [x] Add prompt-safe workflow fields to `operation.ProviderInfo`.
- [x] Convert entry workflows into additional provider descriptors.
- [x] Keep command/env/work_dir hidden from prompt descriptors.

### Batch C: Planner Prompt and Tests

- [x] Render workflow catalog and selection guidance in
      `CapabilitySnapshot.RenderForPrompt`.
- [x] Add tests proving entry workflows match typed
      `external_skill_workflow` requests.
- [x] Add tests proving prompt rendering contains workflow guidance but not
      command/env data.
- [x] Add tests proving existing operation skill configs without workflows
      preserve behavior.

### Batch D: Docs and Examples

- [x] Update `codrax.yaml.example` with a workflow-enabled local skill example.
- [x] Update `docs/user_guide.md` and `docs/user_guide.html`.
- [x] Run focused tests.
- [ ] Commit and push the batch.

## 9. Non-goals

- No keyword-based routing.
- No automatic multi-skill execution without approval.
- No source/trace/log prompt changes.
- No direct integration with `internal/skill.Config`.
- No new command executor policy in this batch.
