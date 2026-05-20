# Unified Evidence / Answer Contract Design

Date: 2026-05-20

Status: incremental implementation in progress

Owner goal: stop accumulating one-case compatibility patches. The read-mode
pipeline should preserve user intent and model-authored rich answers while
letting the system repair only structurally safe mistakes. Hard gates must
consume typed, precise signals; noisy signals and model prose may guide but must
not silently change control flow.

## 1. Why This Design Exists

Recent fixes correctly moved away from the worst failure mode: treating
`is_history_lookup` as a scalar-answer shape. That fixed customer-visible
collapses such as "latest merged feature" turning into `value: <commit>`.
However the fixes are still spread across request traits, aggregate-fact role
normalizers, explorer forced-read gates, finalizer support-ref checks, and
parallel exploration convergence. This is safer than keyword matching, but it is
not the final architecture.

The root problem is that the system currently mixes three independent axes:

- where the evidence came from;
- what answer surface the user requested;
- which grounding rule a claim needs.

When those axes are inferred indirectly from predicates such as
`is_history_lookup`, `is_scalar_answer`, `scenario=architecture_explain`, or
`aggregate_facts.kind=member_set`, small analyzer drift can push a valid answer
into the wrong downstream gate. The common symptoms are:

- VCS metadata forced into current-source `file:line` support refs;
- command/count results forced through `emit_evidence` file scopes;
- runtime log/trace frames leaking into repo citations;
- system-generated tables overriding or duplicating richer model tables;
- finalizer retries caused by optional support data, not true answer defects.

This document defines a unified typed contract that all stages can share.

## 2. Current Code Inventory

The codebase already has useful pieces. The design should reuse them instead of
creating a parallel stack.

### Existing Strengths

- `RequestModel.Predicates.IsHistoryLookup` is now documented as a source
  signal, not answer shape (`internal/types/analysis_ir.go`).
- `ArtifactObservationProfile`, `ClaimOriginLog`, `ClaimOriginPerf`,
  `LogPerfSubKind`, and `FacetObservedArtifactFact` provide a typed runtime
  artifact lane.
- `EvidenceItem.Origin`, `Authority`, `DriftReason`, `LogPerfSubKind`, and
  `LoadBearingSummary` already carry authority and origin-like data for
  evidence rows.
- `AnswerAggregateFact.Kind`, `Role`, `Provenance`, `Dimensions`,
  `SupportRefs`, and `Members` provide a model-authored aggregate handoff.
- `AnswerSupportPlan` separates principal evidence lanes from uncertainty,
  current-code path, artifact, and enrichment lanes.
- `StableInvestigationAggregateFacts()` already protects downstream stages from
  failed/downgraded `emit_investigation_complete` attempts.
- Recent `HistoryLookupPrefersVCSNarrativePrincipal` and
  `IsHistoryBackedCurrentCodeExplanation` helpers are typed-only and avoid raw
  request/prose keyword matching.

### Existing Gaps

- VCS metadata and VCS diff facts do not have first-class origins comparable to
  log/perf origins. Commit hashes are currently handled through a safe but
  temporary decorated-hash compatibility shim.
- Command measurements such as line counts are not first-class citation-grade
  evidence, so models still try to encode them as fake file evidence.
- `aggregate_facts.provenance` is a free string. It can annotate provenance but
  does not drive a closed-enum gate policy.
- Answer shape is distributed across old predicates, family routing, semantic
  views, aggregate kind, and block requirements. There is no single typed list
  of requested outputs such as `scalar`, `mechanism`, `diagram`, or `comparison`.
- Finalizer / renderer safety rules are still partly block-kind based. They can
  add tables because a member set exists even when the requested output is a
  narrative explanation.
- Reviewer and contract gates sometimes see different surfaces: pre-repair
  block text, normalized document, rendered panel, aggregate support lanes, or
  evidence pool.

## 3. Design Principles

1. User intent wins. The system may not narrow a history/diff/log/source-code
   question to a scalar just because one scalar evidence fact exists.
2. Model-authored rich output wins when structurally valid. System supplements
   may fill missing holes only as clearly marked additions; they must not replace
   or flatten a valid model table/prose/diagram.
3. Evidence origin is orthogonal to answer surface. `vcs_metadata` can support a
   scalar, list, comparison, diagnostic, or diagram answer. `current_source` can
   support the same answer shapes.
4. Hard gates use only closed typed fields and deterministic comparisons.
   Heuristics, ranker scores, grep counts, model thoughts, and rendered prose
   remain soft guidance.
5. Optional support-lane defects should be repaired or disclosed. They should
   not force a finalizer rewrite unless the principal answer itself is
   certainly wrong or unsafe to publish.
6. Rich evidence summaries are answer-grade enrichment. When the explorer
   produced grounded, scoped, high-value summary text, downstream prompts must
   preserve it through a deduped enrichment lane.

## 4. Unified Contract

The unified contract has four typed axes.

### 4.1 Evidence Origin

`EvidenceOrigin` answers "where did this fact come from?"

Initial closed enum:

- `current_source`: current checkout source/config/docs file evidence.
- `vcs_metadata`: git metadata such as commit id, subject, author, date, branch,
  merge parent, and file list/stat metadata.
- `vcs_diff`: git patch/hunk/name-only/stat content that may refer to old file
  paths, old symbols, and deleted lines.
- `runtime_artifact`: attached logs, traces, dumps, or perf artifacts.
- `command_measurement`: deterministic read-only command result such as a line
  count, file count, checksum, or bounded grep count.
- `repo_negative_search`: scoped repo query with `result_count=0`.
- `cross_repo_index`: repo-map / multi-repo index metadata not tied to one
  source line.
- `system_inference`: deterministic system-derived support, never principal
  unless paired with a stronger origin.

Mapping to existing code:

- `EvidenceItem.Origin=ClaimOriginLog/Perf` maps to `runtime_artifact`.
- `EvidenceItem.Origin=ClaimOriginCurrentRepo` maps to `current_source`.
- `AnswerAggregateFact.Provenance` should be migrated from free text into
  explicit origin dimensions. The free string remains for audit notes.
- Structured git tools (`git_log`, `git_show`, `git_diff`,
  `git_history_search`) should emit aggregate/evidence facts tagged
  `vcs_metadata` or `vcs_diff`.
- Safe count / measurement tools should emit `command_measurement`.

### 4.2 Requested Output

`RequestedOutput` answers "what kind of visible answer did the user request?"

Initial closed enum:

- `summary`
- `scalar`
- `key_value`
- `count`
- `enumeration`
- `comparison`
- `mechanism`
- `trace`
- `diagram`
- `diagnostic`
- `change_impact`
- `absence`

Mapping to existing code:

- `Predicates.IsScalarAnswer` -> `scalar` only when not contradicted by richer
  typed outputs.
- `Predicates.IsCountQuestion` -> `count`.
- `Predicates.IsCategoryEnumeration` / `IntentEnumerate` -> `enumeration`
  unless the typed shape says architecture/mechanism enrichment only.
- `DiagramHint` / `AnswerContract.Diagram.Required` -> `diagram`.
- `DiagnosticProfile` / `LogTriage` / `PerfTrace` -> `diagnostic`.
- `ChangeImpactProfile.Active()` -> `change_impact`.
- `ScenarioArchitectureExplain` + `IntentExplain` -> `mechanism` or `summary`,
  not enumeration by default.

Important invariant:

`is_history_lookup=true` contributes origin candidates, not requested output.
It may coexist with any output above.

### 4.3 Claim Binding

`ClaimBinding` answers "which origin and grounding rule supports this claim?"

Every principal claim eventually needs:

- `claim_id`: stable local id.
- `target_ref`: exact target, bucket, repo, function, config key, commit, or
  artifact frame being described.
- `origin`: one of `EvidenceOrigin`.
- `requested_output`: one or more `RequestedOutput` values.
- `support_refs`: references appropriate for the origin.
- `grounding_policy`: whether missing support is hard, soft, repairable, or
  disclosure-only.

Support reference shape by origin:

- `current_source`: file/path/line or scoped negative search.
- `vcs_metadata`: git ref, command/tool result id, commit id, optional path
  scope. No current-source file:line required.
- `vcs_diff`: commit/ref + path + hunk/range. Old symbols are historical facts,
  not current-source anchors.
- `runtime_artifact`: artifact id/path + frame/span/message/time. No repo
  citation unless resolved to current source.
- `command_measurement`: command/tool result id + scope + parsed value.
- `repo_negative_search`: repo + query/pattern + result_count=0 + searched_at.
- `cross_repo_index`: repo slug + graph snapshot id + path/symbol if present.

### 4.4 Gate Policy

Each claim binding compiles a gate policy:

- `hard`: publish would be definitely wrong/unsafe without support.
- `repairable`: system can repair locally without model rewrite.
- `soft`: advisory, may produce localized supplement.
- `display_only`: renderer-only transformation, never fed back to model/gates.

Examples:

- Current-source definition claim without file:line: hard.
- VCS commit id summary without current file:line: valid.
- VCS diff old symbol missing in current checkout: valid as `vcs_diff`, hard
  only if the answer claims it exists now.
- Command line count with parseable deterministic result: valid as
  `command_measurement`; no fake file evidence needed.
- Runtime artifact frame path when `resolved_files=0`: valid artifact evidence,
  invalid repo citation.
- System-added table with blank generated cells: do not render.

## 5. End-to-End Flow

### Analyzer

Analyzer should continue emitting existing predicates for compatibility, but a
deterministic post-processor compiles:

- `EvidenceOrigins`: from predicates, attached artifact profiles, structured
  git/log/perf context, and known tool outputs.
- `RequestedOutputs`: from typed predicates, profiles, contract fields, and
  question structure.

Analyzer hard gates should validate schema coherence but avoid using origin
signals as answer shape. For example, `is_history_lookup=true` must not imply
`is_scalar_answer=true`.

### Explorer

Explorer tools should emit source-specific facts:

- `emit_evidence` remains for `current_source` and scoped repo negative search.
- Structured git tools emit `vcs_metadata` / `vcs_diff` aggregate facts.
- Command measurement facts emit `command_measurement`.
- Log/perf tools emit `runtime_artifact`.

`emit_investigation_complete` should accept optional support facts by origin and
requested output. If an optional VCS/support aggregate has malformed
`support_refs`, normalize or route it to audit/support. Do not reopen
exploration solely to prove VCS metadata with current-source lines.

### Extractor

Extractor should not reconstruct principal answer shape from old predicates. It
should consume claim bindings:

- for scalar/count, preserve command/VCS provenance;
- for mechanism/trace/diagram, keep rich grounded evidence summaries;
- for enumeration, require complete member-set only when `RequestedOutput`
  includes `enumeration` and the question family requires principal member
  coverage.

### Finalizer

Finalizer prompt should receive:

- requested outputs;
- origin-specific support lanes;
- rich evidence summaries merged by stable anchor/target;
- clear distinction between principal claims and support/audit context.

System补表 may run only when:

- the requested output requires a structured member table/list; or
- the finalizer omitted a required principal member; and
- the system can fill every generated cell without changing model-authored
  content.

Otherwise system additions must be localized supplements with explicit labels.

### Renderer / Reviewers

Reviewers must consume the exact post-normalization rendered surface plus block
metadata. Renderer-only transforms such as Mermaid ASCII fallback are
`display_only`: they must not pollute prompts, memory, citations, or gates.

## 6. Migration Plan

### Batch A — Contract Types And Projection

Goal: add typed contract objects without behavior changes.

Tasks:

- Add closed enums for `AnswerEvidenceOrigin`, `AnswerRequestedOutput`, and
  `ClaimGroundingPolicy`.
- Add `AnswerIntentContract` or equivalent projection built from
  `RequestModel`, `AnswerContract`, artifact profiles, and existing predicates.
- Add tests covering pure history scalar, latest-feature summary, recent-N list,
  commit comparison, diff+current-code mechanism, diff+diagram, runtime log,
  command measurement, exact absence, and normal current-source mechanism.
- Make projection inspectable in debug logs / prompt traces, but keep existing
  routing behavior unchanged in this batch.

2026-05-20 Batch A.1:

- Added `internal/types/answer_intent_contract.go` with closed enums for
  evidence origin, requested output, and grounding policy.
- Added `CompileAnswerIntentContract` as a side-effect-free projection from the
  existing typed request/answer contract. It does not inspect raw user prose or
  model free text.
- Added focused tests for the highest-risk boundaries: history scalar, history
  narrative, history+current mechanism, history+diagram, current-source count /
  measurement, external runtime artifact, current-source mechanism, and
  bucketed comparison.
- Validation: `go test ./internal/types -run TestCompileAnswerIntentContract`
  PASS.
- Remaining Batch A work: expose the projection in debug/prompt diagnostics
  without making it a new hard gate.

2026-05-20 Batch A.2:

- Added a finalizer prompt diagnostic section titled
  `Evidence Origin / Requested Output Boundary`.
- The section renders `CompileAnswerIntentContract` as orientation only: it
  explicitly says evidence origins are not answer shapes and does not replace
  block requirements, typed support lanes, citations, or aggregate facts.
- Added prompt tests for history narrative, history+diagram mixed origins, and
  external runtime artifact boundaries.
- Validation:
  `go test ./internal/agent ./internal/types -run 'TestRenderAnswerDocUnifiedIntentContract|TestCompileAnswerIntentContract|TestBuildInitialInstructionHistory'`
  PASS.

### Batch B — VCS / Command Origins

Goal: retire commit-hash and count compatibility shims by routing through typed
origins.

Tasks:

- Tag structured git tool outputs as `vcs_metadata` or `vcs_diff`.
- Tag safe count/scalar command outputs as `command_measurement`.
- Update aggregate fact normalization so origin/role decisions use the typed
  projection rather than scattered history/count predicates.
- Replace decorated commit-hash support-ref exemption with
  `origin=vcs_metadata`.
- Add eval guards:
  - latest merged feature summary;
  - recent 10 commits with details;
  - compare two commits;
  - diff + current implementation explanation;
  - diff + current implementation + diagram;
  - all-history topic search + current implementation explanation.

2026-05-20 Batch B.1:

- Added `AnswerAggregateFactEvidenceOrigins` as a compatibility projection from
  existing `aggregate_facts` dimensions / narrow tool provenance tokens into
  the unified `AnswerEvidenceOrigin` enum.
- The projection recognizes structured sources such as `git_history_search`,
  `git_diff`, `exec_command`, `negative_search`, and runtime artifact profiles.
  It is explicitly a soft/projection layer; later batches should replace free
  provenance-string fallbacks with first-class tool-emitted origin fields.
- Finalizer aggregate fact diagnostics now render `evidence_origin=[...]` so
  the model can preserve VCS metadata, VCS diff, command measurement, runtime
  artifact, and negative-search facts without forcing them through
  current-source file:line citation rules.
- Validation:
  `go test ./internal/types ./internal/agent` PASS.

2026-05-20 Batch B.2:

- Redirected the decorated commit-hash `support_refs` exception through
  `AnswerAggregateFactEvidenceOrigins`. The remaining commit-hash regex now
  only validates the member surface under a typed VCS origin; ordinary decorated
  code members still require per-member `support_refs`.
- Added regression coverage for both sides: decorated commit hashes with
  `git_history_search` origin pass without fake file:line support, while
  `Gate.Run (8个独立检查)` still rejects without `support_refs` even if the
  aggregate carries VCS provenance.
- Validation: `go test ./internal/tool ./internal/types` PASS.

2026-05-20 Batch B.3:

- Tagged deterministic system count aggregates at emission time with the unified
  `origin` dimension instead of leaving downstream stages to infer them from
  question shape. `exec_command` count proofs now carry
  `origin=command_measurement`; git history count proofs carry
  `origin=vcs_metadata`; git diff count proofs carry `origin=vcs_diff`.
- Kept the old `proof_source` dimension for compatibility and diagnostics, but
  made `origin` the primary stable contract field for later finalizer/reviewer
  work. This is a typed emission improvement, not a new hard gate.
- Added regression coverage that verifies the emitted count aggregate projects
  through `AnswerAggregateFactEvidenceOrigins` to the expected unified origin.
- Validation: `go test ./internal/tool ./internal/types` PASS.

2026-05-20 Batch B.4:

- Tagged structured git tool outputs at their banner source:
  - `git_log` and `git_history_search` emit
    `evidence_origin=vcs_metadata`;
  - `git_diff` emits `evidence_origin=vcs_diff`;
  - `git_show` emits `evidence_origin=vcs_metadata` and, when the result
    contains patch/stat/name-only diff material, `diff_origin=vcs_diff`.
- Extended aggregate-origin projection to understand `diff_origin` and
  `secondary_origin` dimensions so a model-authored aggregate can carry both
  commit metadata and diff evidence without being forced into one citation
  shape.
- Added regression coverage for git tool banners and for mixed
  metadata+diff aggregate origins.
- Validation: `go test ./internal/tool ./internal/types` PASS.

2026-05-20 Batch B.5:

- Added a narrow `exec_command` emission marker for deterministic count
  measurements. The tool now appends
  `evidence_origin=command_measurement measurement=count` only when the
  successful command output passes the existing `DeterministicCountProofInteger`
  parser.
- Ordinary shell output remains untagged, so repository reads, grep listings,
  and ad-hoc diagnostic commands are not accidentally reclassified as scalar
  measurements.
- Added regression coverage that non-measurement output stays untagged and that
  the added marker does not break deterministic count extraction.
- Validation: `go test ./internal/tool ./internal/types` PASS.

### Batch C — Runtime Artifact Origins

Goal: stop treating external logs/traces as repo citations or code members.

Tasks:

- Add artifact-frame / runtime-call-chain aggregate shape or binding.
- Block repo citation leakage for external-only runtime artifact claims.
- Let artifact-only diagnostics skip current-source facets/reviewers unless the
  user explicitly asks to verify current code.
- Keep log/perf frame order, innermost frame, duration, message, and language as
  structured display fields.

2026-05-20 Batch C.1:

- Tagged successful `emit_log_triage` and `emit_perf_trace` summaries with
  `evidence_origin=runtime_artifact`, making the source boundary visible at
  the tool-emission layer before analyzer/explorer/finalizer prompts consume
  the bundles.
- This does not relax or add any gate. It only preserves typed provenance so
  runtime observations can answer artifact questions without being rewritten as
  current-repo file:line claims.
- Added regression coverage for log observation-only bundles, log frame
  bundles, and perf trace bundles.
- Validation: `go test ./internal/tool ./internal/types` PASS.

### Batch D — Claim Binding In Finalizer And Reviewer

Goal: make finalizer and reviewers consume the same typed claim surface.

Tasks:

- Compile principal/support/audit lanes from claim bindings.
- Feed reviewers the exact final rendered surface plus claim metadata.
- Convert optional support-lane defects to local repair or localized supplement.
- Hard retry only when a principal claim violates its origin-specific hard
  policy.

### Batch E — System Supplement Safety

Goal: preserve model-authored rich answers and stop system-generated pollution.

Tasks:

- Enforce one source-of-truth rule for model table/prose vs system补表.
- Never replace a valid model Markdown table; append separate localized
  supplement only for missing, provably required facts.
- No empty generated table cells.
- Bucket/repo/scope identity must be part of every generated principal row key.
- Add regression tests for multi-repo comparisons, config precedence, Cangjie /
  ArkTS inventories, count-only scalar answers, and runtime artifact answers.

## 7. Acceptance Matrix

The contract is not accepted until these families pass without answer collapse
or noisy retries:

| Family | Required proof |
| --- | --- |
| Current-source mechanism | grounded file:line evidence stays principal |
| Current-source scalar | exact scalar label and strongest citation |
| Count / measurement | command-backed value, no fake file evidence |
| Pure VCS scalar | git metadata provenance, no current-source citation floor |
| VCS feature summary | rich narrative, not `value: commit` |
| Recent-N history list | list remains list; commit metadata is principal |
| Commit comparison | comparison buckets preserved |
| Diff + current code | VCS diff facts and current-source facts stay separate |
| Diff + diagram | diagram obligation survives VCS routing |
| Runtime log/trace | artifact frames do not become repo citations |
| Exact absence | zero-result query scope/result_count shown |
| Multi-repo compare | repo bucket isolation prevents cross-contamination |

## 8. Red Lines For Implementers

- Do not inspect raw user question text or model prose to decide hard flow.
- Do not add keyword tables for history, diff, log, trace, diagram, or language
  terms.
- Do not make `is_history_lookup`, `is_diagnostic_question`, or
  `scenario=architecture_explain` imply a visible answer shape by itself.
- Do not force VCS/runtime/measurement facts through current-source file:line
  gates.
- Do not let system-generated rows replace or flatten valid model-authored
  prose/tables/diagrams.
- Do not silently drop model content during compatibility recovery. Repair
  losslessly, preserve as display attachment, or reject with a precise reason.

## 9. Implementation Checklist

- [x] Batch A.1: add typed projection and tests.
- [x] Batch A.2: expose projection in finalizer prompt diagnostics without a
  hard gate.
- [x] Batch B.1: project aggregate facts to unified evidence origins in
  finalizer diagnostics.
- [x] Batch B.3: tag deterministic VCS and command measurement count
  aggregates at tool emission.
- [x] Batch B.4: tag structured git tool outputs with VCS metadata/diff
  origins at tool emission.
- [x] Batch B.5: tag deterministic non-git command measurements at tool
  emission without tagging arbitrary shell output.
- [x] Batch B.2: route decorated commit-hash support-ref exception through
  unified VCS origin projection.
- [ ] Batch B: remove remaining VCS/hash compatibility fallback once structured
  tool-emitted origin fields land.
- [x] Batch C.1: tag log/perf triage tool outputs with runtime artifact origin.
- [ ] Batch C: add runtime artifact frame aggregate/binding.
- [ ] Batch D: make finalizer support lanes read claim bindings.
- [ ] Batch D: align reviewer input to final rendered surface.
- [ ] Batch E: enforce system supplement safety invariants.
- [ ] Run targeted evals after each batch and update
  `eval_20260520_full_sweep_gap_tracking.md`.
