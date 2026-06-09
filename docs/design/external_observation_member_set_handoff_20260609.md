# External Observation Member-Set Handoff Audit

Date: 2026-06-09

## Trigger

The `trace_query_smartperf_resources` regression run `20260609-223101`
was terminated after the explorer loop failed to close:

- `tool_trace_query=2`, so the trace tool was used successfully.
- `tool_read_file=5`, mostly after the loop drifted toward repo evidence.
- `explorer_iters=36`, `extractor_iters=0`, `finalizer_iters=0`.
- `exit_code=143`, because the long run was killed before synthesis.

The user request was external-observation-only: use `trace_query`
`window_stats` for the 8.0s..9.03s trace window and report
BIO/FileSystem/PageFault resource fields plus Ability/XPower/HiSystemEvent
plugin fields.

## Manual Audit

The first `trace_query` calls produced the data needed by the answer:

- `bio_resource`: `/data/app/base.db`, 2.500 ms, 4096 bytes.
- `filesystem_resource`: `/data/app/base.db`, 3.500 ms, 1024 bytes.
- `page_fault_resource`: `0x1234`, 0.150 ms, 4096 bytes.
- `plugin_event kind=ability_monitor`: `AAFWK / AbilityStart /
  latency_ms / 12.5 / foreground`.
- `plugin_event kind=xpower`: `xpower / xpower_cpu / CPU / 73 /
  foreground`.
- `plugin_event kind=hi_sysevent`: `POWER / THERMAL_REPORT / STAT /
  hot / MINOR`.

The model then tried to close with a principal `aggregate_facts.member_set`
over the external trace events and an `external_only_trace` waiver. The waiver
was accepted, but the exhaustive-enumeration pre-complete check downgraded the
closure because each member lacked current-source typed evidence or
member-specific repo `support_refs`.

After that downgrade, the retry hint pulled the model toward impossible repair:

- It attempted `emit_evidence(source="attached_trace...")`; the tool rejected
  this because `emit_evidence` is intentionally current-repo evidence only.
- It attempted `read_file attached_trace-ce0090bf.txt`; the attached trace was
  not a repo file.
- It emitted repo evidence about renderer/grounding helpers, which was valid
  repo evidence but semantically unrelated to the user's trace enumeration.
- It repeatedly retried `emit_investigation_complete` with variants of trace
  line support refs that cannot be validated as repo citations.

There was no final user-facing answer to audit because the run never reached
extract/finalize.

## Deep Root Cause

This is a system handoff gap, not a trace parser gap and not a one-case prompt
gap.

`trace_query` renders SmartPerf resource/plugin observations as compact
external artifact lines, but the ObservationLedger projection only compiled
some trace-query lines into typed records:

- root-cause ranks
- root evidence
- critical blocking
- state churn
- file IO by inode
- page cache by inode
- storage latency by layer
- IO pressure summary

It did not compile these already-rendered lines:

- `bio_resource`
- `filesystem_resource`
- `page_fault_resource`
- `plugin_event`

So the exact runtime observations were visible in tool-output memory, but not
promoted into the typed observation lane that later prompt, handoff, and answer
consumers can prioritize without reparsing prose.

Separately, `exhaustiveEnumerationMemberSetUsable` already has access to the
same typed evidence-origin model used elsewhere:

- `AnswerAggregateFactEvidenceOrigins`
- `AnswerEvidenceOriginsAreOriginSpecificOnly`

But that exhaustive member-set gate does not honor origin-specific runtime
artifact member sets. It always asks every principal member to satisfy the
current-source evidence/member-support path. That contradicts the existing
schema contract that observation-only runtime artifacts must remain in their
own typed provenance lane and must not be converted into fake repo citations.

The repair text has the same mismatch: it describes the source-code repair
path even when the failed fact is typed as external runtime-artifact
provenance. That creates a retry loop where the model tries invalid
`emit_evidence` and irrelevant repo reads.

## Generalized Gaps

### G1. Tool-output observation loss

Some deterministic trace-query outputs are not projected into
ObservationLedger. Any future external observation family with compact tool
rows can suffer the same loss if the ledger only projects selected row prefixes.

### G2. Origin-specific member sets are over-constrained

The exhaustive-enumeration gate treats all member sets as current-source
enumerations unless members happen to pass repo evidence lookup. For
origin-specific evidence lanes such as runtime artifacts, VCS metadata,
commands, MCP resources, connector resources, or external documents, the
member-set usability decision should use typed origin provenance instead of
repo file:line citation pressure.

### G3. Repair guidance does not branch by evidence lane

The retry hint suggests `emit_evidence` and repo support refs even when typed
origins prove the fact is external-only. This is a system hint problem; the
model should not need to infer that a repair directive is impossible.

### G4. Handoff priority leaks information

Rich trace facts collected early can be lost between explorer, extractor, and
finalizer if they remain only in raw tool text. Important observations should
be carried as typed records with origin, source ref, artifact-local span,
predicate, value, unit, and rich notes.

## Red Lines And Non-goals

- No keyword matching over user intent or model prose to decide hard gates.
- No prompt-only fix.
- No SmartPerf-specific one-case patch.
- No fake repo evidence, generated repo files, or synthetic source citations
  for external trace rows.
- No hard gate on noisy ranking/frequency signals.
- Do not relax current-source enumeration requirements.
- Do not change write-mode blast-radius rules.

## Design

### D1. Promote all trace-query resource/plugin rows into ObservationLedger

Add deterministic row projection for:

- `- bio_resource ...`
- `- filesystem_resource ...`
- `- page_fault_resource ...`
- `- plugin_event ...`

Each record should be:

- `Origin=runtime_artifact`
- `Producer=trace_query`
- `Role=supporting_coverage`
- `GroundingPolicy=soft`
- `ProvenanceLane=artifact_span`
- `SourceRef` inherited from the trace-query tool result banner
- `Span.LineStart` from `line=`
- `Predicate` equal to the row family, or plugin kind for plugin rows
- `Subject` equal to path/address/domain/event when available
- `Object` equal to op/event/metric where available
- `Value` and `Unit` copied from structured row fields
- `RichNotes` carrying stable key/value details such as path, thread, count,
  max latency, bytes, domain, event, metric, value, category, example, and
  callstack

This is not a text-classification layer. It consumes system-authored row
prefixes and key/value fields emitted by `trace_query`.

### D2. Let exhaustive member-set usability honor typed origin provenance

Add a precise predicate used by the exhaustive member-set gate:

```
aggregateMemberSetCanRelyOnOriginSpecificProvenance(ctx, fact)
```

The predicate should:

- call `AnswerAggregateFactEvidenceOrigins(fact, rm)`;
- require `AnswerEvidenceOriginsAreOriginSpecificOnly(origins)`;
- reject mixed/current-source origins;
- apply only after the fact is a complete principal member set with non-empty
  members.

When true, `exhaustiveEnumerationMemberSetUsable` should accept the member set
without current-source member support checks. This preserves strict source
requirements while allowing runtime/VCS/command/MCP/external-document slates
to close through their own typed provenance.

### D3. Make downgrade hints lane-aware

When a member set is origin-specific-only but still structurally invalid, the
hint should explain the external-observation repair path:

- include explicit structured origin/provenance dimensions;
- keep exact members in `members[]`;
- preserve artifact-local coordinates in dimensions/supporting facts when
  available;
- do not try to manufacture repo `emit_evidence` rows for external artifacts.

The existing source-code repair text remains correct for current-source or
mixed-origin member sets.

### D4. Preserve handoff priority into downstream consumers

The new ledger records should flow through the existing prompt projection and
answer-document evaluation paths. No new prompt-only carrier is needed; the
same ObservationLedger infrastructure should carry the information.

## Task Breakdown

### Batch 0: Documentation

- Record this audit and design.
- Commit and push before code changes.

### Batch 1: Trace-query resource/plugin observation projection

- Add parser/projection helpers in `internal/types/observation_ledger.go`.
- Support same-line key/value fields and continuation `callstack=...`.
- Add unit tests for resource rows and plugin rows.
- Verify prompt projection prioritizes these records in runtime-artifact
  requests.
- Commit and push.

### Batch 2: Origin-specific exhaustive member-set handoff

- Add a typed origin-specific member-set predicate in
  `internal/tool/emit_investigation_complete.go`.
- Use it inside `exhaustiveEnumerationMemberSetUsable`.
- Keep current-source member-set support requirements unchanged.
- Add tests for:
  - runtime-artifact enumeration member set without repo refs passes;
  - current-source enumeration member set without support still downgrades;
  - mixed current-source + runtime origin does not bypass current-source
    support.
- Commit and push.

### Batch 3: Lane-aware repair guidance

- Adjust downgrade text for origin-specific-only structural failures.
- Keep repo-evidence guidance for current-source and mixed-origin slates.
- Add tests for the hint text branch without matching user/model prose.
- Commit and push.

### Batch 4: Validation

- Build a fresh eval binary.
- Run targeted evals in batches of two:
  - `trace_query_inode_io_pressure`
  - `trace_query_inode_event_search`
  - `trace_query_smartperf_resources`
  - `trace_query_android_perfetto_sched_blocked`
  - `trace_query_core_topology_supply`
  - `trace_query_frame_timeline_flow`
- Run focused Go tests for the touched packages.
- Commit and push any final test-only adjustments.

## Acceptance Criteria

- SmartPerf resource/plugin rows are available as ObservationLedger runtime
  records.
- An external-only exhaustive trace enumeration can close without fake repo
  citations.
- Current-source enumerations still require repo evidence or member-specific
  support refs.
- The repair hint no longer directs external-only runtime observations toward
  impossible `emit_evidence` repair.
- The new behavior is driven by structured tool rows and typed origins, not
  user text, model prose, or keyword matching.
- No eval regression in the selected trace-query cases.
