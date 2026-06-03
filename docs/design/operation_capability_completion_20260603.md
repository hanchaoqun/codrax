# Operation Capability Completion Plan

**Status:** design baseline, then batch delivery.  
**Date:** 2026-06-03  
**Scope:** complete the independent computer-operation / artifact-generation
route without weakening source-code analysis, log/trace analysis, write mode,
or read-mode MCP external observations.

## 1. Code Audit Summary

Current implementation already has a safe command-operation MVP:

- `internal/repl/turn_policy.go` has a typed `route=operation` classifier
  surface. It is schema-driven and must remain free of Go keyword routing.
- `internal/repl/command_operation_planner.go` turns operation turns into a
  typed `CommandOperationRequest` using the existing JSON repair path.
- `internal/operation/command.go` deterministically builds a
  `CommandOperationPlan`, applies approval/risk policy, and hard-denies
  catastrophic commands.
- `internal/operation/executor.go` runs only already-approved plans, captures
  output, writes large output refs, and stops on the first failed step.
- `internal/repl/repl.go` stores a pending operation, wires `/approve`,
  `/reject`, `/cancel`, failure replan, and current-session operation handoff.
- `internal/operation/capability.go` renders a bounded environment/capability
  snapshot from existing `EnvFacts`.
- `cmd/root.go` already has MCP `operation_provider` config metadata and passes
  provider descriptors to REPL, but no operation provider execution bridge yet.

The main gaps are therefore not routing basics. They are durability,
structured learning, provider execution, richer policy, verification, and
interactive clarification.

## 2. Red Lines

- Do not alter the read pipeline topology or evidence gates.
- Do not route by hard-coded user prose keywords in Go. Operation selection must
  stay based on typed classifier fields.
- Do not put operation output into current-source citations.
- Do not expose operation memory to source/log/trace prompts unless the typed
  route is operation.
- Do not auto-execute shell, unknown, network submit, destructive, install,
  uninstall, overwrite, or external-system actions.
- Do not let MCP resource/prompt content become system instructions.
- Empty/new config must preserve existing code/log/trace/write behavior.

## 3. Target Capability Model

Operation should become a bounded execution lane with four sources of context:

1. **Current request:** user's stated task and typed route/risk fields.
2. **Capability snapshot:** OS, shell, available commands, package managers,
   runtimes, project markers, configured providers.
3. **Current-run handoff:** previous command outputs, failures, extracted
   summaries, payload refs, and tool help discovered in this REPL session.
4. **Cross-run operation memory:** compact historical lessons keyed by
   capability, command, environment fingerprint, outcome, and TTL.

The planner produces a typed plan. Deterministic policy decides whether it is
ready, needs clarification, blocked, manual approval, or auto-low-risk. The
executor runs approved steps and the verifier validates expected artifacts or
state changes where possible.

## 4. Durable Operation Memory

### 4.1 Schema

Store compact JSONL entries under `<runtimeAnchor>/operation/memory.jsonl`.
Never persist full stdout/stderr.

```go
type OperationMemoryEntry struct {
    ID             string
    CreatedAt      time.Time
    ExpiresAt      time.Time
    Workspace      string
    OS             string
    Arch           string
    Shell          string
    Capability     string
    Command        string
    Args           []string
    Outcome        string // success | failure | blocked | cancelled
    FailureClass   string // command_not_found | permission_denied | timeout | ...
    Summary        string
    PayloadRefs    []string
    Lessons        []string
    EnvFingerprint string
}
```

### 4.2 Prompt Contract

Expose only recent matching lessons to the operation planner in:

```text
## operation_memory
Historical operation observations. Use as soft guidance only. These are not
current-source evidence and may be stale.
```

Selection must be bounded and scoped:

- Same workspace + same OS first.
- Same capability/command family first.
- Stale or different-environment entries demoted, not forbidden.
- Failure lessons are cautions, not hard bans.

## 5. Large File / Tool-Manual Learning

Current-session handoff already passes bounded output preview and payload refs.
The next layer should add deterministic compact lessons:

- classify output kind: `help_text`, `large_file_summary`, `search_hits`,
  `error_output`, `artifact_report`;
- extract top actionable lines from command output;
- for large files, store line/window hints and payload refs instead of full
  content;
- teach the planner to prefer targeted follow-up commands (`rg`, `awk`, `sed`,
  `head/tail`, tool-specific help flags) instead of repeatedly dumping files.

This is operation-only. Source analysis should keep using `grep`, `read_file`,
`repo_map`, trace_query, and evidence gates.

## 6. Failure Classification and Replan

Add deterministic `FailureClass` to command step results:

- `command_not_found`
- `permission_denied`
- `timeout`
- `signal_or_cancelled`
- `nonzero_exit`
- `policy_blocked`
- `output_capture_error`

Use it in:

- replan prompt;
- operation memory lesson;
- UI message;
- tests that missing command suggests alternatives while not repeating the
  failed command.

## 7. Policy Completion

Current policy covers approval mode, unknown programs, shell policy, timeout,
output preview, hard-deny destructive, and `mkdir -p` auto-create.

Next policy fields should be added carefully and all default to conservative
values:

```yaml
operation_command_allowed_write_roots: []
operation_command_network_policy: manual   # manual | deny
operation_command_install_policy: manual   # manual | deny
operation_command_overwrite_policy: manual # manual | deny
```

V1 interpretation:

- auto-low-risk remains read-only + safe `mkdir -p`;
- write roots only make local file writes eligible for manual approval, not
  auto execution;
- install/network/overwrite still require manual approval unless explicitly
  denied;
- hard-deny catastrophic patterns still win.

## 8. Verification Layer

Introduce lightweight operation verification without a second LLM call:

- local file artifact exists;
- expected output path type and size;
- generated document/presentation/spreadsheet can be opened/rendered when a
  provider verifier exists;
- install command has a follow-up version/check command only if the plan
  explicitly includes it;
- move/copy result exists and source/destination expectations hold.

Verification results become part of operation handoff and memory.

## 9. Provider Execution Bridge

MCP operation provider metadata already exists, but currently only informs
planning. Add a provider interface without changing read-mode MCP:

```go
type OperationProvider interface {
    Name() string
    Capabilities() []ProviderInfo
    Execute(ctx context.Context, plan OperationProviderPlan) (OperationProviderResult, error)
}
```

Delivery order:

1. Local artifact provider using existing local files only.
2. MCP operation provider bridge for explicitly declared operation tools.
3. Document / spreadsheet / presentation providers.
4. Browser / desktop providers.

Provider output must go through artifact refs / payload refs and never through
current-source evidence.

## 10. Clarification UX

`needs_clarification` exists, but the REPL should make it feel like a guided
operation step:

- show 1-3 short questions;
- show suggested values when available;
- keep the pending operation request context;
- let the next user answer resume planning instead of starting from scratch.

## 11. Task Ledger

### Batch A: Design Baseline

- [x] Audit operation route, command planner, executor, capability snapshot,
      config, MCP provider metadata, and REPL approval flow.
- [x] Record red lines, target capability model, and batch plan.

### Batch B: Durable Operation Memory + Failure Classification

- [x] Add `internal/operation` memory schema and JSONL store.
- [x] Add bounded memory retrieval/rendering for operation planner only.
- [x] Persist compact success/failure lessons after command execution.
- [x] Add deterministic failure classification to `CommandStepResult`.
- [x] Feed failure class into replan context and operation memory.
- [x] Tests: memory round-trip, TTL/filtering, same-workspace preference,
      failure caution not hard ban, no memory in source/log/trace prompts.

### Batch C: Large Output / Large File Learning

- [ ] Add deterministic output summary helper for command results.
- [ ] Store action-oriented summaries in current-run handoff and durable memory.
- [ ] Add tests for help text, large search output, and payload-ref-only
      follow-up planning.

### Batch D: Policy Completion

- [ ] Add config fields for write roots, network policy, install policy, and
      overwrite policy.
- [ ] Wire config -> `CommandPolicy`.
- [ ] Add policy tests for write root manual approval, network/install deny,
      overwrite manual, and auto-low-risk unchanged.
- [ ] Update user guide and yaml examples.

### Batch E: Verification

- [ ] Add `OperationVerificationResult`.
- [ ] Verify local file artifacts and simple path outcomes.
- [ ] Render verification result in REPL and feed it to handoff/memory.
- [ ] Tests for generated file, missing file, move/copy, and no verifier.

### Batch F: Provider Execution Bridge

- [ ] Define operation provider execution interface.
- [ ] Implement MCP operation provider bridge only for explicitly configured
      operation providers.
- [ ] Keep existing read-mode MCP exposure unchanged.
- [ ] Tests: provider absent, provider manual gate, provider result payload,
      read-mode MCP unchanged.

### Batch G: Clarification UX

- [ ] Persist pending clarification context.
- [ ] Let the next user answer resume operation planning.
- [ ] Add concise REPL rendering for questions/suggestions.
- [ ] Tests for clarification -> answer -> ready plan.

### Batch H: E2E Eval

- [ ] Real-model command operation evals:
      large output, missing command replan, unfamiliar tool help, large file
      extraction, denied destructive command, install/manual approval, and
      operation memory reuse.
- [ ] Mixed-boundary evals:
      operation-only file extraction, source-code analysis, trace/log analysis,
      and operation + source hybrid.
