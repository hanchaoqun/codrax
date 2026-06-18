# Runtime Artifact Path Capability Gap Audit (2026-06-18)

## Scope

This note closes a residual Batch E/F gap from
`perf_sample_trace_query_integration_20260617.md`: users often provide runtime
artifacts by writing one or more absolute or relative paths in the question
instead of attaching them through `--htrace`, `/htrace`, `--log`, or `/log`.

The system must make those files easy for the model to consume through
`trace_query` / log-artifact lanes, while preserving the architectural boundary:

- deterministic code may recognize an explicit path as a runtime-artifact
  capability;
- deterministic code must not infer the user's analysis intent from suffixes,
  file names, or keywords;
- source-vs-artifact policy remains a typed analyzer decision based on the user
  wording and structured request model.

## Current Gap

- Runtime artifact path recognition is fragmented across `internal/types`,
  `internal/agent`, and `internal/tool`.
- Some entry gates recognize only older trace suffixes:
  `.systrace`, `.htrace`, `.atrace`, `.ftrace`, `.perfetto`, `.trace`.
- New perf lane artifacts can be missed when mentioned only in the question:
  `.perftrace`, `.tracebundle.json`, `perf.data`, and `*.perf.data`.
- Some gates are suffix-only. That is acceptable for recognizing a concrete
  path shape, but it is not enough for commercial UX because users may pass
  generated runtime files without a familiar suffix.
- Bare terms such as `perf.data` can appear in source-code questions. Treating
  the token alone as user intent would violate the prompt red line and cause
  false runtime-artifact routing.

## Design

### Capability, Not Intent

Introduce a single conceptual distinction:

```text
explicit path + artifact capability  !=  user's requested analysis lane
```

Hard gates may use precise capability signals:

- an explicit path-like token in the current request;
- a runtime-artifact path family recognized by the shared type helper;
- or an existing local file whose first bytes/text identify trace/perf content
  (`PERFILE2`, `perf_sample:`, `sched_switch:`, `tracing_mark_write:`,
  tracebundle JSON).

Hard gates must not use noisy intent signals:

- no keyword matching of "analyze trace", "only trace", "against code", etc.;
- no parsing model prose or final-answer text;
- no promotion from "artifact capability exists" to
  `current_source_mode=exclude`.

The analyzer still emits the typed source policy. Mixed requests such as
"compare this trace with current code" keep the current-source lane available.
Requests that explicitly forbid source analysis can be encoded by the model in
`external_observation_policy.current_source_mode=exclude`.

### Path Families

The shared runtime-artifact helper covers:

- logs: `.log`, `attached_log.txt`;
- text traces: `.trace`, `.systrace`, `.htrace`, `.atrace`, `.ftrace`,
  `.perfetto`;
- perf traces and bundles: `.perftrace`, `.tracebundle.json`;
- raw perf data: `perf.data`, `*.perf.data`;
- generated attachment sentinels: `attached_trace.txt`,
  `attached_hitrace.txt`, `attached_atrace.txt`.

`perf.data` as a bare word is only treated as an artifact when it is path-like
or resolves to an existing local file; this avoids misrouting source questions
about the raw perf parser.

### Tool Surface

When capability is present, the explorer may expose `trace_query` and the
analyzer may skip repo pre-scan used only to classify artifact literals. This
does not answer the question and does not remove source tools from later
mixed-source turns. It only prevents wasted broad repo indexing before the
model has emitted a typed request model.

### Structured Policy and Repair

The source lane remains model-authored through
`external_observation_policy`. Any field the model may emit into tool-call JSON
must be handled by the same schema/compat/repair layer as existing analysis
fields:

- valid `external_observation_policy.current_source_mode=exclude` with anchored
  `source_quotes[]` is accepted as the typed signal that current checkout/source
  evidence is out of scope;
- when that policy is valid, the `emit_analysis` compat layer clears
  `diagnostic_profile.current_risk`,
  `diagnostic_profile.historical_regression`, and
  `diagnostic_profile.current_version_check`, while preserving
  `diagnostic_profile.is_diagnostic` for artifact-only cause analysis;
- prompt teaching, tool schema, and tests all state the same contract so the
  model does not have to remember conflicting rules.

### Handoff and Answer Caveats

`trace_query` observations are hard runtime observations even when the trace was
provided by an explicit path rather than `/htrace` or `--htrace`. Completion
handoff therefore waives current-source citation floors only when the request
model does not require current-source evidence.

The final answer caveat layer must not reintroduce source-code guidance for an
artifact-only answer. Runtime-only or principal external-observation surfaces
may keep precise boundaries such as "the trace did not map to current source",
and must keep specific contradictions. Generic soft quality telemetry such as
prose density, optional richness, broad acceptance, or generic coverage caveats
must not become user-visible "combine with source code" advice.

## Task List

- [x] Record the gap and capability-vs-intent design.
- [x] Unify runtime-artifact path family coverage in `internal/types`.
- [x] Update analyzer explicit-path detection to include perf artifacts and
  content sniffing for existing path-like files.
- [x] Keep bare `perf.data` source-code questions out of the artifact shortcut
  unless an actual file path resolves.
- [x] Update `trace_query` lazy exposure so explicit `.perftrace`,
  `.tracebundle.json`, and `perf.data` path questions get the tool.
- [x] Add tests for:
  - `.perftrace` / `.tracebundle.json` / `*.perf.data` path recognition;
  - non-suffixed existing systrace/perf files;
  - bare `perf.data` source-code discussion staying in normal source mode;
  - explicit perf path exposing `trace_query`.
- [x] Ensure path-provided `trace_query` observations participate in completion
  handoff without requiring current-source citations for artifact-only turns.
- [x] Ensure valid source-exclude policy is reflected consistently in prompt,
  tool schema, and JSON repair for `diagnostic_profile` current-status fields.
- [x] Suppress generic runtime-only caveats that would incorrectly advise
  "combine with source code" after the user or request model excluded source.
- [x] Run focused tests, full tests, build, and explicit-path eval batches after
  code changes.

## Validation

- Focused Go tests: `go test ./internal/tool ./internal/agent ./internal/types ./cmd`
- Full Go tests: `go test ./...`
- Build: `make`
- Eval batch, parallel=2:
  - `eval/cases/trace_query_path_question_relative_perftrace.case`
  - `eval/cases/trace_query_path_question_suffixless_trace.case`
- Donghu path eval batch, parallel=2:
  - `eval/cases/trace_query_path_question_relative_donghu_short.case`
  - `eval/cases/trace_query_path_question_absolute_donghu_short.case`

## Non-Goals

- No new model-authored `trace_query` input fields.
- No keyword-based user-intent classifier.
- No automatic download/install of official perf tools in this batch.
- No changes to perf sample ranking semantics.
