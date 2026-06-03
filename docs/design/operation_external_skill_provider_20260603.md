# Operation External Skill Provider and Lazy Loading Plan

**Status:** design baseline before implementation.  
**Date:** 2026-06-03  
**Scope:** extend the independent operation/computer-operation lane with
external skill/provider capabilities, compact planner teaching, and optional
lazy provider startup. Keep source-code analysis, log/trace analysis, write
mode, and read-mode MCP external observations unchanged by default.

## 1. Code Audit

Current code already separates operation routing from source analysis:

- `internal/repl/turn_policy.go` owns typed `route=operation` classification.
  The classifier schema includes `computer_operation`, `artifact_generation`,
  `presentation_generation`, `document_generation`, `spreadsheet_generation`,
  `browser_operation`, and `external_skill_workflow`. This must remain the
  routing authority; no Go-side keyword matching may be added.
- `internal/repl/repl.go` dispatches command operations to the command planner
  and non-command operation requests to `operation.BuildPlan`.
- `internal/operation/command.go` and `executor.go` implement the command lane:
  approval policy, execution, bounded output, verification, failure classes,
  current-run handoff, and durable operation memory.
- `internal/operation/plan.go` has `ProviderInfo` and plan-only provider
  matching. It can route an operation to an MCP-backed provider when one is
  configured, but its metadata is too small for model planning.
- `cmd/root.go` loads MCP servers eagerly, builds provider descriptors from
  `mcp_servers[].operation_provider`, and passes them to REPL.
- `internal/mcp` already supports stdio JSON-RPC, `tools/list`,
  `tools/call`, namespaced tool exposure, typed observations, resource refs,
  and large-output protections.
- `internal/skill` is a static prompt configuration registry for pipeline
  stages. It is loaded eagerly by `skill.RegisterDefaults`. It is not an
  external skill runtime and should not be stretched into one.

## 2. Gaps

1. **No operation external-skill manifest/registry.** Configured providers are
   only a flat `[]ProviderInfo`; there is no independent registry that can list
   capability descriptors, preserve examples, or expose compact provider
   summaries to planners.
2. **No real lazy provider startup.** MCP servers are currently started at
   process startup. Operation execution is delayed until `/approve`, but the
   provider process is not.
3. **Provider capability teaching is thin.** The command planner sees local
   command/environment facts and operation memory, but not a compact list of
   external operation skills, their surfaces, side effects, input contract, or
   examples.
4. **Provider result contract is still narrow.** MCP provider execution returns
   status, summary, payload ref, and observation count. It does not yet expose a
   reusable typed provider result shape for artifacts, verification, or memory.
5. **Mixed boundaries must remain typed.** Operation providers must not grab
   ordinary source/log/trace questions. Code/trace/log analysis stays on the
   existing read pipeline unless the typed classifier explicitly emits
   `route=operation`.

## 3. Red Lines

- Empty config and existing MCP config without new operation fields must keep
  current behavior.
- Read-mode MCP tools remain explorer/sub-explorer observations; operation
  providers do not change source evidence/citation gates.
- External skill/provider prompts, examples, and resource text are advisory
  capability metadata, not system instructions.
- Side-effecting execution still requires operation policy and approval.
- Large provider output must remain bounded in the REPL panel and use payload
  refs for full content.
- New model-visible fields must have clear prompt/schema descriptions and JSON
  compatibility tests when they are emitted by a model.

## 4. Target Architecture

Do not extend `internal/skill.Config` for external execution. Add an operation
provider descriptor layer:

```go
type ProviderDescriptor struct {
    Name         string
    Kind         string
    Surfaces     []string
    SideEffects  []string
    RequiresGate bool
    ToolName     string
    Description  string
    InputSchema   string
    Examples      []string
    Source       string // mcp | local | builtin | plugin
    LazyStart    bool
    Loaded       bool
}
```

`ProviderInfo` can carry this descriptor data in V1 to avoid duplicating the
matching path. A future `CapabilityProvider` interface should wrap the actual
execution lifecycle:

```go
type CapabilityProvider interface {
    Descriptor() ProviderDescriptor
    Load(ctx context.Context) error
    Execute(ctx context.Context, req ProviderExecutionRequest) (ProviderExecutionResult, error)
    Verify(ctx context.Context, result ProviderExecutionResult) ProviderVerification
    Close() error
}
```

For V1, MCP is the only executable external provider. Local command execution
continues to use the existing command executor, not this provider interface.

## 5. Lazy MCP Strategy

Add explicit MCP config:

```yaml
mcp_servers:
  - name: slides
    operation_provider: true
    operation_tool: run_operation
    operation_lazy_start: true
    operation_description: "Create and verify local PPTX decks."
    operation_examples:
      - "Generate a 6-slide technical summary deck from the current answer."
```

Behavior:

- Default `operation_lazy_start` is false; existing MCP startup is unchanged.
- When `operation_lazy_start=true`, the server is registered as an operation
  descriptor but skipped by normal startup. It is not visible as a read-mode MCP
  tool until explicitly loaded by operation execution.
- On `/approve`, REPL starts the lazy MCP server, registers it in the MCP
  registry, calls the configured `operation_tool`, and then reuses it for the
  session. Startup failure returns a provider execution error and does not
  enter the source pipeline.
- This option is intended for operation-only providers. Operators who need the
  same MCP server in explorer should leave `operation_lazy_start=false`.

## 6. Planner Teaching

Extend `operation.CapabilitySnapshot` with compact provider descriptors:

```text
## operation_providers
- mcp:slides kind=presentation_generation surfaces=slides effects=local_file_write
  gate=true lazy=true loaded=false tool=run_operation
  description=Create and verify local PPTX decks.
  examples: Generate a 6-slide technical summary deck...
```

Use this only on the operation command planner path. The source analysis,
trace/log prompts, finalizer, and write-mode prompts must not receive operation
provider capability summaries.

## 7. Result and Handoff Contract

Provider results should remain external operation output:

- `status`
- `provider`
- `tool`
- `summary`
- `payload_ref`
- `artifact_refs`
- `observations`
- `verification_status`
- `error`

V1 can keep using the existing `providerOperationResult` and add fields
incrementally. Handoff and memory should store summaries and refs only, never
full raw provider output.

## 8. Delivery Tasks

### Batch A: Descriptor and Design

- [x] Audit operation, MCP, REPL, config, skill, and planner boundaries.
- [x] Record the external skill/provider design, lazy startup strategy, red
      lines, and task list.

### Batch B: Provider Descriptor Registry and Prompt Surface

- [x] Extend `operation.ProviderInfo` with description, input schema, examples,
      source, lazy/loaded state.
- [x] Add bounded provider descriptor rendering to `CapabilitySnapshot`.
- [x] Feed operation providers into the command-operation planner snapshot.
- [x] Add tests proving provider descriptors appear in operation planner
      prompts. Source/log/trace prompts do not call this operation-only
      snapshot path.

### Batch C: Config and Lazy MCP Startup

- [x] Add optional MCP config fields:
      `operation_lazy_start`, `operation_description`,
      `operation_input_schema`, `operation_examples`.
- [x] Split MCP startup into eager configs and lazy operation-only descriptors.
- [x] Add a lazy loader path used by REPL provider execution before
      `CallTool`.
- [x] Preserve eager behavior by default and add tests for all-or-nothing eager
      loading unchanged.

### Batch D: Result Contract and Handoff

- [ ] Expand provider operation result with artifact refs and verification
      status where available.
- [ ] Store provider summaries/payload refs in current-run operation handoff.
- [ ] Add durable operation memory entries for provider successes/failures.
- [ ] Keep provider output out of source citation lanes.

### Batch E: Docs and Eval

- [ ] Update `codrax.yaml.example`.
- [ ] Update user guide MD/HTML with descriptor, dynamic parameters, lazy
      startup, and approval workflow.
- [ ] Add focused tests for:
      - empty config byte-safe behavior;
      - eager MCP unchanged;
      - lazy MCP not started until approval;
      - lazy startup failure surfaces as operation failure;
      - provider descriptor prompt injection is operation-only;
      - source/trace/log questions do not consume operation providers.
- [ ] Run targeted `go test` suites and push each batch.
