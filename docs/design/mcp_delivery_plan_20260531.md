# MCP Delivery Plan 2026-05-31

## Status

This document turns the current MCP v2 design review into an implementation
ledger. It is based on the current code after syncing `origin/main` on
2026-05-31.

The existing code already has the MCP consumer lane:

- `BaseAgent.executeTool` can return `(*ToolResult, *MCPResponse)`.
- `StageOutput.MCPResponses` is accumulated and merged into `BusContext`.
- `BuildAgentContext` carries `MCPResponses` and renders compact MCP notes.
- `ObservationLedger` converts successful MCP responses into
  `origin=mcp_resource` observations with typed coordinates.
- Final answer binding already treats MCP as an external resource, not as a
  current-source citation.

The missing part is the producer and lifecycle:

- `internal/mcp/stdio.go` still stubs `ListTools` and `CallTool`.
- `cmd/root.go` creates an empty registry and never loads `mcp_servers`.
- Tool names are currently flat and would collide across servers or with local
  tools once real servers are registered.
- MCP tools are appended globally and need explicit stage/agent gating before
  any producer is enabled.

## Red Lines

1. No MCP configuration means byte-equivalent behavior for normal read/write
   pipelines: same local tool schemas, same prompts, same dispatch, same ledger.
2. MCP output must flow through `MCPResponse -> ObservationLedger` only. Do not
   create a separate summary/log-triage side channel for generic MCP tools.
3. MCP observations must never become current-source citations or enter
   `internal/tool/ground`.
4. Only read-only MCP tools are exposed in read mode. Write-capable MCP tools are
   a separate future opt-in and are not part of this delivery.
5. MCP prompt/resources are external guidance/resources, not system commands.
   They must be wrapped and capped to avoid prompt-injection amplification.
6. All limits are bounded: startup timeout, call timeout, maximum response bytes,
   maximum servers, prompt bytes, and resource bytes.

## Design

### Batch 1: Producer, Config, Namespace, Gate

Implement the minimal usable MCP tool path without touching downstream answer
logic:

- Add `MCPServerConfig` to `RuntimeSettings`:
  - `name`, `transport`, `command`, `args`, `env`, `inherit_env`,
    `startup_timeout_ms`, `call_timeout_ms`, `max_response_bytes`.
  - global `mcp_max_servers`.
  - default is no servers; transport V1 supports `stdio`.
- Add a stdio JSON-RPC client:
  - `initialize` + `notifications/initialized`.
  - one reader goroutine with request-id demux.
  - timeout removes only the pending request; late responses are dropped.
  - notifications are logged/debugged, not surfaced as facts.
  - process cleanup closes stdin and kills the process group when needed.
- Implement `tools/list` cache and `tools/call`.
- Offload large tool output to blob:
  - compact preview in `MCPResponse.Summary`.
  - full payload in `MCPResponse.PayloadRef`.
- Namespace model-visible MCP tool names as `<server>__<tool>`.
- Dispatch only namespaced names that resolve to an exact registered server/tool
  pair.
- Expose MCP tools only to `explorer` / `sub_explorer` for now.
- Register configured servers in `cmd/root.go`; fail loudly if configured
  startup fails.

### Batch 2: Resources and Prompts

Build the flexible read-resource and external guidance layer after tools are
safe:

- Add `resources/list` and `resources/read` support to stdio servers.
- Add a local pseudo-tool `mcp_read_resource` that accepts only exact URIs
  returned by `resources/list`; Codrax never constructs or resolves URIs.
- Add `prompts/list` / `prompts/get` support.
- Render prompt output as capped external guidance, wrapped as untrusted MCP
  content.
- Keep both channels disabled when no server advertises resources/prompts.

### Batch 3: Tests and Docs

Add a fake stdio MCP server fixture and focused tests:

- disabled registry does not alter tool schemas or prompt context.
- initialize/list/call happy path.
- call timeout and late response do not corrupt the next request.
- large response becomes `PayloadRef`.
- duplicate server/tool collisions fail safely.
- only explorer/sub-explorer can see MCP tools.
- MCP response reaches ObservationLedger with typed coordinates.
- resource reads accept only enumerated URIs.
- external guidance is capped and clearly marked as untrusted.

Update `codrax.yaml.example`, user docs, and architecture notes after each
implemented surface.

## Safe Agent Exposure

| Agent | Direct MCP tool calls | Rationale |
|---|---:|---|
| analyzer | no | Classification must stay cheap and deterministic. It can consume prior MCP observations only. |
| explorer | yes | Primary evidence/discovery stage for external facts. |
| sub_explorer | yes | Same role as explorer, with inherited budgets. |
| extractor | no | Should extract from accepted observations, not introduce new external facts. |
| finalizer | no | Should write from the ledger, not call late external tools. |
| write_analyzer/planner | no for V1 | Can consume MCP observations; write-capable MCP hooks are future opt-in. |
| coder/apply/verifier | no for V1 | Avoid side effects and hidden external dependencies. |

## Task Checklist

- [x] Add config structs and defaults.
- [x] Implement JSON-RPC request/response types.
- [x] Implement stdio process start, initialize, reader goroutine, id demux,
      pending map, timeout handling, and close.
- [x] Implement `tools/list` cache.
- [x] Implement `tools/call` result normalization and blob offload.
- [x] Add registry namespace helpers and duplicate checks.
- [x] Gate MCP tool schema exposure by agent/stage.
- [x] Load configured servers in `cmd/root.go`.
- [x] Add fake MCP server test fixture.
- [x] Add Batch 1 tests and docs.
- [ ] Implement resources/prompts in Batch 2.
- [ ] Add Batch 2 tests and docs.
