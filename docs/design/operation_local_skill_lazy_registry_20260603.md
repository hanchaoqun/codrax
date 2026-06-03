# Operation Local Skill Lazy Registry

**Status:** design and task ledger before implementation.  
**Date:** 2026-06-03  
**Scope:** add a local `operation_skills[]` manifest registry for lazy
computer-operation/artifact providers. Keep source analysis, trace/log analysis,
MCP external observations, and write-mode flows unchanged unless the typed
operation route selects an operation provider.

## 1. Current Code Audit

The operation lane already has the right separation points:

- `internal/repl/turn_policy.go` owns the typed route decision. Operation
  routing must keep using this structured classifier output; no Go-side keyword
  matching is allowed.
- `internal/repl/repl.go` has two operation paths:
  - command operations use `operation.BuildCommandOperationPlan` and the
    command executor;
  - provider operations use `operation.BuildPlan` and wait for `/approve`.
- `internal/operation/plan.go` defines `ProviderInfo`, matching, and plan
  generation. MCP provider descriptors already reuse this shape.
- `internal/operation/capability.go` injects compact provider descriptors into
  the operation planner capability snapshot only. Source/trace/log prompts do
  not consume this surface.
- `cmd/root.go` currently builds operation providers from
  `mcp_servers[].operation_provider` and supports lazy MCP startup with
  `operation_lazy_start`.
- `internal/repl/repl.go` already stores provider execution results in the
  current-run handoff and operation memory, using summaries, artifact refs,
  payload refs, and observation counts instead of source citations.

The missing piece is a **local manifest-backed provider source**. The existing
`internal/skill` package is prompt-skill infrastructure for LLM stages and must
not be reused for side-effecting local execution.

## 2. Gap

`operation_skills[]` local manifest lazy skills are not implemented:

- there is no `codrax.yaml` schema for local operation skill descriptors;
- planner teaching cannot list local external skills such as PPT generators,
  browser helpers, desktop automation wrappers, or company-local command
  drivers;
- there is no lazy local executor that starts only after typed operation
  routing and user approval;
- there is no local skill result parser that can convert structured JSON or
  plain text into the existing provider result/handoff/memory lane.

MCP lazy providers cover remote/tool-server operation providers, but not local
script/binary/plugin skills distributed with a workspace or operator profile.

## 3. Red Lines

- Empty `operation_skills[]` must be a no-op.
- Do not add keyword-based route decisions. Only typed operation policy may
  enter this lane.
- Do not change repo_map, source citation gates, trace_query, grep/read_file,
  finalizer, or write-mode evidence contracts.
- Operation skill descriptors are advisory capability metadata, not system
  instructions.
- Local skill execution is side-effect capable and must obey the existing
  operation approval policy. Default is manual approval.
- Large local skill output must be bounded in REPL/CLI panels. Full output must
  go to payload/blob refs.
- Local skill output is an operation artifact/observation lane. It must not be
  treated as current-source evidence.

## 4. Target Config

Add `operation_skills[]` to `codrax.yaml`:

```yaml
operation_skills:
  - name: local_ppt
    operation_kinds: [presentation_generation, artifact_generation]
    operation_surfaces: [slides, local_file]
    operation_side_effects: [local_file_write]
    operation_requires_confirmation: true
    operation_description: "Generate and verify local PPTX decks."
    operation_input_schema: |
      {
        "topic": "string",
        "output_path": "string",
        "style": "optional string"
      }
    operation_examples:
      - "Generate a 6-slide technical summary deck in ./out/summary.pptx."
    operation_lazy_start: true
    command: "./tools/local_ppt_skill"
    args: ["--json"]
    input_mode: stdin_json
    work_dir: "."
    timeout_ms: 60000
    max_output_bytes: 262144
```

Field semantics:

- `name`: stable local provider id, exposed as `skill:<name>`.
- `operation_kinds`: typed operation kinds the provider can handle.
- `operation_surfaces`: surfaces such as `slides`, `office_doc`,
  `spreadsheet`, `browser`, `desktop`, or `local_file`.
- `operation_side_effects`: declarative side effects for planner and policy
  display.
- `operation_requires_confirmation`: defaults to true for local skills.
- `operation_description`, `operation_input_schema`, `operation_examples`:
  compact planner teaching.
- `operation_lazy_start`: defaults true. Startup reads the descriptor only;
  execution launches the command after approval.
- `command`, `args`, `work_dir`, `env`, `inherit_env`: local executable
  invocation. Commands are executed without a shell.
- `input_mode`: `stdin_json` by default; `args_json` appends one JSON argument.
- `timeout_ms`, `max_output_bytes`: execution bounds.

## 5. Execution Request Envelope

Local skills receive a compact JSON envelope:

```json
{
  "request": "user request text",
  "operation": "presentation_generation",
  "operation_kind": "presentation_generation",
  "target_surface": "slides",
  "risk_level": "medium",
  "side_effects": ["local_file_write"],
  "requires_confirmation": true,
  "provider": "skill:local_ppt",
  "tool": "run",
  "repo_root": "/abs/workspace"
}
```

This keeps parameter handling dynamic: the model chooses the operation intent
and target surface through the existing typed route, while the local skill owns
its domain-specific parsing and execution.

## 6. Result Contract

Local skill stdout may be either JSON or plain text.

Structured JSON result:

```json
{
  "success": true,
  "summary": "Created 6 slides and verified render.",
  "artifact_refs": ["out/summary.pptx"],
  "payload_ref": "",
  "verification_status": "verified",
  "verification_summary": "PPTX rendered without layout overflow.",
  "observations": ["Used template default.pptx"]
}
```

Plain text result:

- non-empty stdout becomes the summary;
- large stdout is compacted into a preview and payload ref;
- stderr is shown only on failure or as a short diagnostic.

Both shapes are converted to the existing `providerOperationResult` and then
flow through current-run handoff and durable operation memory.

## 7. Batch Tasks

### Batch F: Design Ledger

- [x] Audit operation planner, MCP lazy provider, config, REPL execution, and
      handoff/memory paths.
- [x] Record the local operation skill registry design, red lines, config,
      request/result contract, and delivery task list.

### Batch G: Config and Descriptor Registry

- [x] Add `types.OperationSkillConfig`.
- [x] Add `RuntimeSettings.OperationSkills`.
- [x] Pass operation skill configs from `cmd/root.go` to REPL.
- [x] Convert configs to `operation.ProviderInfo` descriptors with
      `Source="skill"` and `Name="skill:<name>"`.
- [x] Preserve existing MCP provider behavior and descriptor ordering.
- [x] Add tests for empty config, descriptor normalization, and capability
      snapshot rendering.

### Batch H: Lazy Local Skill Executor

- [x] Add REPL local skill execution branch for `skill:<name>` providers.
- [x] Launch commands without shell expansion.
- [x] Support `stdin_json` and `args_json` input modes.
- [x] Enforce timeout and bounded output.
- [x] Parse structured JSON results and plain text fallback.
- [x] Store large output via existing payload refs.
- [x] Feed results into provider handoff and operation memory.
- [x] Add tests for success, large output, and missing config. Non-zero exit
      and timeout are handled by the shared executor branch and can be expanded
      with fixture-specific tests if future regressions appear.

### Batch I: Docs, Examples, and Eval

- [x] Update `codrax.yaml.example`.
- [x] Update `docs/user_guide.md` and `docs/user_guide.html`.
- [x] Add a tiny local operation skill fixture used by tests.
- [x] Add end-to-end tests proving:
      - [x] local skill appears only in operation planner capability snapshots;
      - [x] source pipeline is not entered for local skill operation execution;
      - [x] approval gates execution;
      - [x] provider output is not emitted as source evidence;
      - [x] handoff and memory include summaries and refs.
- [x] Run focused Go tests and push each batch.

## 8. Non-goals

- No desktop/browser/PPT implementation is bundled in this batch. The registry
  makes those providers pluggable.
- No compatibility bridge to `internal/skill.Config`.
- No automatic route override based on provider names or user prose keywords.
- No hard gates based on local skill summaries.
