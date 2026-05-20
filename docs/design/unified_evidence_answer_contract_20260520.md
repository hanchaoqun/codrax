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
- Batch A.2 below exposes the projection in prompt diagnostics without making
  it a new hard gate.

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

2026-05-20 Batch B.6:

- Covered the fallback path where the model uses `exec_command` to run git
  directly. The shell command parameter is parsed conservatively as structured
  tool input: only segments whose effective command is `git` are classified;
  quoted text such as `printf 'git log'` is ignored.
- `exec_command` git metadata commands (`git log`, `git rev-list`,
  `git show --no-patch`, `git rev-parse`, etc.) now emit
  `evidence_origin=vcs_metadata`; diff commands (`git diff`, default
  `git show`, `git log -p` / `--stat` / `--name-only`) also emit
  `diff_origin=vcs_diff` where appropriate.
- Git history counts produced through `exec_command` now flow into deterministic
  history aggregate enrichment as `proof_source=exec_command_git_history` with
  `origin=vcs_metadata`, while still carrying `measurement_origin` /
  `command_measurement` when the output is a parsed count. This prevents a
  history answer from collapsing into a generic command scalar.
- Added regression coverage for quoted non-commands, `git -C` global options,
  metadata-only git output, diff output, and combined history-count
  metadata+measurement output.
- Validation: `go test ./internal/tool ./internal/types` PASS.

2026-05-20 Batch B.7:

- Added eval guard `u7l` for "最近 10 次提交都做了哪些事情，作用和影响分别是什么".
  This protects the customer failure class where a rich history summary was
  collapsed into a single `value: <commit>` answer.
- Pure recent-N history enumeration now remains a VCS-principal lane even when
  the analyzer marks the visible answer shape as `enumeration` /
  `category_enumeration`. The request may ask for a list; that does not imply
  current-source file reads or `emit_answer_symbol` extraction.
- Explorer/extractor prompts now explicitly allow a completed pure VCS history
  investigation to close through `emit_investigation_complete(reason,
  aggregate_facts)` without synthetic `read_file` / `emit_evidence` rows.
- Added JSON compatibility repair for answer-document item objects that contain
  exactly one schema-unknown non-empty string field and no `text` / `cells`.
  The repair moves that lone string to `text` losslessly; ambiguous unknown
  fields still go through the existing quarantine path.
- Validation:
  `go test ./internal/context ./internal/types ./internal/tool ./internal/agent ./internal/orchestrator`
  PASS for the targeted VCS/history tests recorded in the gap document.

2026-05-20 Batch B.8:

- Analyzer skill instructions now carve out obvious repository-history / git
  classification questions from source-code pre-scan. The analyzer should emit
  `question_kind=history` and `is_history_lookup=true` directly, then choose
  visible output shape separately: scalar only for literal hash/date/author/count
  questions; list/comparison/mechanism/diagram for richer history requests.
- Finalizer prose guidance now explicitly bans leaking internal carrier terms
  such as `citation_ref` and `citations[]` into user-visible command/VCS
  answers.
- Validation:
  `go test ./internal/skill ./internal/agent -run 'TestAnalysisSkill_PromptDocumentsDirectHistoryClassification|TestRenderAnswerDocUnifiedIntentContract|TestRenderAnswerDocClaimBindings'`
  PASS.

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

2026-05-20 Batch C.2:

- Added runtime artifact claim bindings for already-validated log/perf bundles.
  Log errors, observations, perf frames, jank spans, stalls, and startup timing
  now compile to `origin=runtime_artifact` with `source=log_triage` or
  `source=perf_trace`, even when no `aggregate_facts` were emitted.
- These bindings deliberately use `AggregateIndex=-1` and never synthesize
  `current_source`. Runtime observations can answer artifact/log/trace
  questions directly, while current checkout claims still require a separate
  current-source lane.
- Finalizer prompt rendering now consumes the unified claim binding compiler
  instead of aggregate facts alone, so runtime-only diagnostics receive the same
  origin/policy handoff as VCS, command measurement, and current-source facts.
- Added regression coverage for log stack frames, log observations, perf frame
  durations, and runtime-only finalizer prompt rendering.
- Validation:
  `go test ./internal/types -run 'TestCompileAnswerClaimBindings|TestCompileRuntimeArtifactClaimBindings'`
  PASS;
  `go test ./internal/agent -run 'TestRenderAnswerDocClaimBindings|TestRenderAnswerDocUnifiedIntentContract'`
  PASS.

### Batch D — Claim Binding In Finalizer And Reviewer

Goal: make finalizer and reviewers consume the same typed claim surface.

Tasks:

- Compile principal/support/audit lanes from claim bindings.
- Feed reviewers the exact final rendered surface plus claim metadata.
- Convert optional support-lane defects to local repair or localized supplement.
- Hard retry only when a principal claim violates its origin-specific hard
  policy.

2026-05-20 Batch D.1:

- Added `AnswerClaimBinding` as the deterministic bridge from stable
  `aggregate_facts` to origin-specific answer-writing policy. Each binding
  carries the aggregate index/kind/role, target, evidence origin, requested
  outputs, support refs, and `ClaimGroundingPolicy`.
- Added `CompileAnswerClaimBindingsFromAggregateFacts`, which consumes typed
  aggregate dimensions and request-model fields only. It does not inspect raw
  user prose or model answer text.
- Added a final answer-writing prompt section,
  `Claim Binding / Gate Policy Handoff`, so the same origin/policy view is
  visible before closure prose, aggregate facts, and principal member-set
  requirements. This is still guidance/orientation, not a new hard reject.
- Regression coverage verifies:
  - history count aggregates keep both `vcs_metadata` and
    `command_measurement` bindings;
  - current-source principal aggregates compile to `hard`;
  - runtime artifact aggregates do not synthesize `current_source`;
  - the final answer-writing prompt renders origin-specific policies.
- Validation: `go test ./internal/types ./internal/agent ./internal/tool` PASS.

2026-05-20 Batch D.2:

- Wired the pre-emit scalar/value coverage check to the unified claim-binding
  contract. A model-authored scalar aggregate whose binding is
  `vcs_metadata`, `vcs_diff`, `runtime_artifact`, `command_measurement`, or
  another non-current-source origin no longer forces a hard visible-scalar
  repair when the requested output is narrative/mechanism/diagnostic/diagram.
- Exact-output requests remain protected: scalar, key-value, count, and absence
  outputs still require the aggregate value to appear visibly. This preserves
  commit-id/count/no-hit answers while preventing feature-summary/history
  answers from collapsing into a raw commit hash.
- The rule consumes only typed aggregate origins and requested-output
  projection. It does not inspect raw user text or model prose, so
  `exec_command git ...` fallbacks benefit once their aggregate/tool output is
  tagged with `origin=vcs_metadata` or `diff_origin=vcs_diff`.
- Added regression coverage for the analyzer-missed-history case
  (`origin=vcs_metadata` but no `is_history_lookup`) and the exact VCS scalar
  case that must still be hard-protected.
- Validation:
  `go test ./internal/tool -run 'TestPreCheckAggregateScalarValueCoverage'`
  PASS.

2026-05-20 Batch D.3:

- Added a typed VCS-history narrative boundary for exploration, extraction, and
  parallel convergence. Pure non-scalar history narratives can now treat
  `vcs_metadata` / `vcs_diff` as the principal lane and current source files as
  optional support; mixed history+current-code questions keep both lanes when
  analyzer fields require mechanism, diagram, diagnostic, comparison, relation,
  change-impact, or explicit endpoint trace evidence.
- Explorer prompts now distinguish `VCS History Narrative Handoff` from
  `Mixed History / Current-source Handoff`. The pure-history lane drops
  analyzer RequiredFiles as mandatory current-source reads, preventing a valid
  VCS conclusion from being dragged into unrelated source forced-read loops.
- Parallel exploration may converge early for pure VCS narratives and for
  non-bucketed history-backed current-code mechanism explanations once a fork
  has passed `emit_investigation_complete` prechecks. It still waits for sibling
  handoffs for diagrams, cross-component comparisons, explicit enumerations,
  relation lookups, diagnostics, and change-impact shapes.
- Extractor/family routing now treats history-backed current-code mechanism as
  architecture/generic narrative rather than answer-symbol enumeration. This
  prevents commit lists or one historical clue from forcing `emit_answer_symbol`
  and later system member tables.
- Generic forced-read gates (`primary_anchor_unread`, `phase1_unread`,
  `multi_path_anchor`) are skipped for history-backed current-code mechanism
  narratives, but explicit source-to-sink call-chain traces with typed
  endpoints keep the hard current-source gates.
- Added eval guard `u7k` for "all-history topic + current implementation
  explanation" and recorded the remaining VCS/diff-origin follow-up in
  `eval_20260520_full_sweep_gap_tracking.md`.
- Validation:
  `go test ./internal/types -run 'TestHistoryLookupPrefersVCSNarrativePrincipal|TestIsHistoryBackedCurrentCodeExplanation|TestNormalizeAggregateFactRolesForRequest_DemotesNonScalarHistoryScalars|TestPrincipalAggregateMemberSetFactRefsForRequest_HistoryMechanismTreatsExplicitSetsAsSupport|TestResolveQuestionFamily_HistoryCurrentCodeExplanationBeatsOneItemBoundary'`
  PASS;
  `go test ./internal/agent -run 'TestBuildInitialInstructionHistory|TestExtractor_BuildPrompt_HistoryCurrentCodeMechanismSkipsAnswerSymbol'`
  PASS;
  `go test ./internal/orchestrator -run 'TestDispatchExploreWindowsParallel_History|TestParallelExploreAllowsEarlyConvergence_History'`
  PASS;
  `go test ./internal/tool -run 'TestEmitInvestigationComplete_PreCompleteCheck_HistoryCurrent'`
  PASS.

2026-05-20 Batch D.4:

- Aligned the post-emit reviewer body with the final visible answer surface for
  V2 blocks after deterministic normalization. Reviewer input now includes
  block titles for every body-shaped block, structured table columns, and
  structured item cells, so localized system-supplement boundaries such as
  “系统按已验证证据补充成员...” are visible to both self-consistency and
  semantic-quality review.
- This keeps the review path typed and renderer-aligned: it reads
  `AnswerDocumentV2` after the tool-layer normalizers have run, not raw model
  prose or rejected drafts. The change is intentionally presentation-only; it
  does not create new retry gates and does not rewrite model-authored content.
- Regression coverage pins a structured system-supplement table with title,
  columns, exact location cell, and rich Chinese note, proving the reviewer
  sees the same facts the user-facing panel can render.
- Validation:
  `go test ./internal/orchestrator -run 'TestRenderConsistencyReviewBodyV2|TestSemanticQualityReviewBodyIncludes'`
  PASS.

2026-05-20 Batch D.5:

- Added a structured semantic-review outcome path. When semantic quality review
  returns `sufficient=true` with confidence at or above the configured floor,
  the orchestrator now suppresses only low-precision soft coverage/prose/facet
  caveat violations from that same accepted draft.
- The suppressor is intentionally narrow: `must_include`, citation failures,
  self-contradictions, success criteria, and any operator-promoted strict kind
  remain actionable. Low-confidence `sufficient=true` verdicts also do not
  suppress anything.
- This removes the repeated user-facing pattern where reviewer telemetry says
  the answer is sufficient but the final panel still appends generic
  “覆盖度可能不充分 / 未达到预期标准” notes from older soft validators.
- Validation:
  `go test ./internal/orchestrator -run 'TestRunSemanticQualityReviewWithOutcome|TestSuppressSemanticSufficientCaveatViolations|TestRunSemanticQualityReview|TestAppendSoftContractCaveats'`
  PASS.

2026-05-20 Batch D.6:

- Added claim-binding context to the post-emit semantic-quality reviewer input.
  The reviewer now receives compact `claim_id / origin / policy / requested
  outputs / target / support_ref count` rows, so it can distinguish
  VCS/runtime/measurement/negative-search provenance from current-source
  file:line obligations.
- Added `FilterFinalizerRetryRootViolationsForBus`, which applies the normal
  soft/strict policy and then narrows operator strict-promotion by the active
  claim bindings. Generic coverage/support-lane signals such as facet,
  principal-claim-use, prose-density, uncertainty, semantic-underfilled, and
  principal-support-member omissions do not force another finalizer rewrite
  when the current principal handoff is non-current-source, non-exact-output
  narrative support. Precise obligations remain hard: `must_include`,
  citation/self-contradiction/success criteria, exact scalar/count/absence
  outputs, and requested diagram/table/list block omissions are not suppressed.
- The filter is intentionally typed-only. It consumes `AnswerClaimBinding`
  origins, requested outputs, grounding policy, and `MissingBlockKind`; it does
  not parse user text, model prose, or violation detail strings.
- Validation:
  `go test ./internal/orchestrator -run 'TestRunSemanticQualityReviewWithOutcome|TestRenderSemanticQualityUserMessage_IncludesClaimBindingBoundary|TestFilterFinalizerRetryRootViolations'`
  PASS.

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

2026-05-20 Batch E.1:

- Audited the current deterministic supplement paths:
  `normalizePrincipalEnumerationRowBlocks`,
  `compileEnumerationDisplayTableRows`, and
  `compileCitationBackedTableRows`.
- Existing implementation already follows the core safety boundary: valid
  model-authored Markdown/prose is preserved, missing deterministic rows are
  appended in a separate localized supplement block, exact-absence
  non-enumeration answers suppress补表, scalar history/count support ledgers do
  not become principal tables, and generated tables omit or skip columns/rows
  that would render blank cells.
- Existing tests also pin the important anti-collapse cases: richer
  model-authored row notes beat weak evidence summaries, categorized Markdown
  table notes are preserved, coarse const-block citations are corrected only
  when the item/file match is unambiguous, explicit conflicting model locations
  are not overwritten, redundant section shells collapse, excluded candidate
  sets do not leak, and singleton count-basis metadata is not rendered as a
  principal member table.
- Validation:
  `go test ./internal/tool -run 'TestNormalizePrincipalEnumerationRowBlocks|TestCompileEnumerationDisplayTableRows|TestCompileCitationBackedTableRows'`
  PASS.
- Remaining E work is eval-driven hardening, not a known missing primitive:
  multi-repo bucket identity, config-precedence support-only aggregates, and
  external runtime/artifact member supplements remain tracked in
  `eval_20260520_full_sweep_gap_tracking.md`.

2026-05-20 Batch E.2:

- Fixed a role-only aggregate reconciliation hole found by
  `qf_multi_member_set_count_caveat`. Parallel exploration can legitimately
  emit multiple principal member sets with the same candidate role but different
  semantic buckets, for example read-mode and write-mode `Kind` constants.
  Role-only definition evidence must not expand either sibling bucket into the
  full same-role universe.
- `reconcileCompletionAggregateFactsWithDefinitionEvidence` now permits
  definition-based member-set expansion only when there is exactly one
  principal complete member set for that candidate role. If multiple same-role
  principal sets exist, the system preserves the model/explorer bucket boundary
  and lets the broader surface planner decide whether a true superset should be
  principal while sub-buckets become support.
- Added tests that pin both halves of the contract: same-role sibling buckets
  are not auto-expanded by role-only evidence, and when a broad same-role
  superset exists it is the only principal set while narrower buckets are kept
  as `supporting_coverage`.
- Validation:
  `go test ./internal/tool -run 'TestReconcileCompletionAggregateFactsWithDefinitionEvidence'`
  and
  `go test ./internal/types -run 'TestBuildAnswerSurfacePlan_DemotesSameRoleSubBucketsWhenBroadSupersetExists'`
  PASS.

### Batch F — Unified Non-Code Evidence Contract

Goal: make VCS, logs, traces, command output, scoped negative searches, and
repo-index facts first-class evidence without pretending they are current
source `file:line` anchors.

This is the final commercial target for non-code evidence. It is deliberately
not a second system: it reuses the existing `AnswerEvidenceOrigin`,
`CompileAnswerIntentContract`, `AnswerAggregateFactEvidenceOrigins`, and
`CompileAnswerClaimBindings` primitives. The work is to connect every stage to
those primitives consistently.

#### F.1 Evidence Producer Contract

Every non-code evidence producer must attach a typed origin at the production
boundary:

- `git_log`, metadata-only `git_show`, and git-history searches:
  `origin=vcs_metadata`.
- `git_diff`, patch/stat/name-only `git_show`, and diff-like `exec_command`
  git output: `origin=vcs_diff` or `diff_origin=vcs_diff`.
- `emit_log_triage` and `emit_perf_trace`: `origin=runtime_artifact`.
- deterministic safe shell measurements: `origin=command_measurement`.
- scoped zero-result searches: `origin=repo_negative_search` with
  `repo/scope/query|pattern/result_count/searched_at`.
- repo-map and multi-repo overview facts: `origin=cross_repo_index`.

The producer contract answers "where did this fact come from"; it does not say
how the answer should be displayed.

#### F.2 Handoff Contract

Exploration hands off non-code evidence through:

- raw tool outputs for recent command/VCS results when the finalizer needs the
  exact literal or commit-by-commit summary;
- `emit_investigation_complete.reason` for model-authored narrative synthesis;
- `emit_investigation_complete.aggregate_facts` for exact counts, lists,
  scalar values, groups, exclusions, and bounded absences that must be preserved
  as structured data;
- runtime bundles (`LogBundle`, `PerfBundle`) for attached artifacts.

`emit_evidence` remains the current-checkout source citation tool. It is valid
only when the model has read and grounded a real current source/config/doc line.
It is not the carrier for old/deleted diff lines, stack-frame coordinates,
command rows, or zero-result searches.

#### F.3 Claim Binding / Gate Contract

Downstream code must compile one shared `AnswerClaimBinding` view from
aggregate facts, runtime bundles, and the request intent contract. Hard retry is
reserved for precise principal violations under the active origin:

- `current_source` principal claims may require current file:line citations.
- exact scalar/count/absence outputs must visibly preserve their exact value or
  boundary.
- `vcs_metadata`, `vcs_diff`, `runtime_artifact`, `command_measurement`,
  `repo_negative_search`, and `cross_repo_index` principal claims should be
  locally repaired or disclosed when imperfect; they must not trigger
  current-source forced reads unless the user also asked for current behavior.

This preserves user intent: a commit-history list remains a list, a diff+current
code question keeps two lanes, and a log-only diagnostic may answer from the
log without inventing repo citations.

#### F.4 Finalizer / Renderer Contract

The finalizer must see the same origin boundary as reviewers and pre-emit gates:

- finalizer prompt uses the existing `Evidence Origin / Requested Output
  Boundary` and `Claim Binding / Gate Policy Handoff` sections;
- explorer/extractor receive the same contract earlier via the `Evidence Origin
  Boundary` prompt section;
- renderer displays non-code evidence as natural localized provenance, never as
  internal carriers (`citation_ref`, `citations[]`, enum names);
- system supplements may append clearly marked localized notes, but must not
  replace a valid model-authored table/prose/diagram.

#### F.5 Test / Eval Matrix

Required tests before the contract is considered complete:

- unit tests for every producer origin banner and aggregate-origin projection;
- prompt tests proving explorer/extractor see the upstream origin boundary and
  finalizer does not receive duplicate copies from `BuildPromptContext`;
- finalizer/reviewer tests proving non-code principal claims do not become
  current-source citation requirements;
- eval cases:
  - latest commit feature summary;
  - recent 10 commits with purpose and impact;
  - compare two commits;
  - diff-only summary;
  - diff + current code explanation;
  - diff + current code + diagram;
  - all-history search plus current implementation;
  - log-only diagnostic;
  - trace-only performance diagnosis;
  - scoped negative search mixed with present current-source facts.

2026-05-20 Batch F.1:

- Added a shared upstream `Evidence Origin Boundary` prompt section for
  explorer/extractor. It renders the same `CompileAnswerIntentContract`
  projection used by finalizer, explains that non-current-source facts are
  first-class evidence in their own lane, and forbids converting VCS/log/trace/
  command/negative-search facts into fake current-source `emit_evidence` rows.
- The section intentionally does not render for finalizer because finalizer
  already receives `renderAnswerDocUnifiedIntentContract` and claim bindings.
  A regression test pins that no duplicate finalizer origin-boundary prompt is
  introduced.
- Tool-sourced finalizer guidance now clarifies that `citation_ref=-1` is an
  internal carrier and visible prose should say repository history / diff output
  / command output instead of exposing `citation_ref` or `citations[]`.
- Validation:
  `go test ./internal/context -run 'TestBuildPromptContext_ToolSourced|TestBuildPromptContext_EvidenceOriginBoundary|TestBuildPromptContext_AnalysisSkill_NoDuplicateSections'`
  PASS.

2026-05-20 Batch F.2:

- Moved one more compatibility edge toward the final contract:
  `NormalizeAnswerAggregateFacts` now canonicalizes exact legacy provenance
  tokens such as `git_diff`, `git_history_search`, `exec_command`, and
  `measurement_origin` into structured origin dimensions when those dimensions
  are missing.
- This reuses the existing `AnswerAggregateFactEvidenceOrigins` projection and
  does not create a new inference system. The recognized tokens are closed and
  typed; arbitrary model prose remains audit-only provenance and does not drive
  hard gates.
- Existing explicit dimensions win. The normalizer never duplicates `origin`,
  `diff_origin`, or `measurement_origin`, and it skips additions when the
  aggregate dimension budget is already full rather than rejecting a previously
  usable handoff.
- Validation:
  `go test ./internal/types -run 'TestAnswerAggregateFactEvidenceOrigins|TestNormalizeAnswerAggregateFacts_AddsStructuredOriginDimensions|TestNormalizeAnswerAggregateFacts_PreservesExistingOriginDimensions|TestCompileAnswerIntentContract|TestCompileAnswerClaimBindings|TestHistoryLookupPrefersVCSNarrativePrincipal'`
  PASS.

2026-05-20 Batch F.3:

- Strengthened VCS tool descriptions so the model is taught at the tool boundary
  that `git_diff` / `git_history_search` results are VCS provenance, not
  current-checkout `file:line` evidence.
- `git_diff` now explicitly tells the model to use it for diff-only and
  diff+current-code questions before deciding whether current source must also
  be read, and not to mirror old/deleted diff lines through `emit_evidence`.
- `git_history_search` now says its result should travel through
  `aggregate_facts` / `reason` unless the model separately reads current source.
- Added tests that pin this guidance so future tool-description edits do not
  accidentally reintroduce fake current-source evidence loops.
- Validation:
  `go test ./internal/tool -run 'TestGit(Diff|HistorySearch|Log|Show)'`
  PASS.

2026-05-20 Batch F.4:

- Wired exact-absence output into `CompileAnswerIntentContract` when
  `AnswerContract.ExactResolution.AllowAbsence` is set. This keeps bounded
  absence as a requested output instead of letting negative-search aggregates
  be demoted to generic support in mixed answers.
- Negative-search aggregate normalization now materializes
  `result_count=0` as a structured dimension when the model supplied the count
  only through `value=0`. This aligns the formal negative-evidence channel with
  the commercial contract: `repo + query/pattern + result_count=0 + scope +
  searched_at`.
- Added regression coverage for:
  - diff-only aggregates producing `vcs_diff` claim bindings without
    synthesizing `current_source`;
  - perf-trace-only finalizer prompt boundaries avoiding current-repo citation
    pressure;
  - mixed current-source + negative-search answers preserving the bounded
    absence detail rather than hiding it behind a present source fact.
- Validation:
  `go test ./internal/types -run 'TestCompileAnswerIntentContract|TestCompileAnswerClaimBindings|TestAnswerAggregateFactEvidenceOrigins|TestNormalizeAnswerAggregateFacts'`
  PASS;
  `go test ./internal/tool -run 'TestPreCheckAggregateScalarValueCoverage|TestGit(Diff|HistorySearch|Log|Show)'`
  PASS;
 `go test ./internal/agent -run 'TestRenderAnswerDocUnifiedIntentContract|TestRenderAnswerDocClaimBindings'`
  PASS.

2026-05-21 Batch F.5:

- Semantic-quality reviewer JSON compatibility now accepts `concerns` as the
  normal array, a single object, `null`/missing, or a string rationale. A string
  rationale is preserved as a fallback structured concern only when
  `sufficient=false`; `sufficient=true` plus a stray string such as "none" does
  not create a retry concern. This keeps semantic-review meaning intact while
  absorbing harmless schema drift.
- Tool-sourced finalizer guidance now forbids unsupported module/component/
  category counts unless the exact count is present in VCS/command output or
  `aggregate_facts`. This is a soft prompt contract, not a hard prose parser:
  when only a commit list is available, the answer should use qualitative
  grouping rather than inventing numeric group totals.
- Validation:
  `go test ./internal/orchestrator -run 'TestSemanticQualityReviewer_(RepairsStringConcerns|IgnoresStringConcernsWhenSufficient|RepairsSingleObjectConcern|HappyPath|FallbackConcern|RepairLocus)'`
  PASS;
 `go test ./internal/context -run 'TestBuildPromptContext_(ToolSourcedValueGuidance|RawToolOutputs)'`
  PASS.

2026-05-21 Batch F.6:

- VCS row-order self-consistency is now gated by precise tool order. For
  typed `row_order_mismatch` contradictions on commit-history lists, the
  system suppresses reviewer complaints when the visible commit labels follow
  the exact `git_log` commit order. The reviewer prompt also explicitly forbids
  inferring chronology from patch size, severity, or change magnitude.
- Principal enumeration coverage now treats commit-hash labels as covering
  decorated VCS member rows such as `hash: subject (stat)`. This prevents the
  deterministic row compiler from appending a duplicate "system-verified
  member supplement" table when the model has already rendered the commit list.
- Raw tool output rendering now preserves substantially larger `git_log` /
  `git_history_search` / `git_show` payloads for history answers. VCS history
  member-set prompts no longer suppress the model-authored closure reason,
  because the aggregate member set may be an identity skeleton while the closure
  prose carries useful per-commit summaries.
- Validation:
  `go test ./internal/orchestrator -run 'TestFilter(VCSHistoryRowOrderContradictions|DeterministicRowOrderContradictions)|TestSelfConsistencyReviewerPrompt_DocumentsFabricationShape'`
  PASS;
  `go test ./internal/tool -run 'TestNormalizePrincipalEnumerationRowBlocks_(DoesNotDuplicateVCSCommitRowsAlreadyVisible|AppendsOnlyMissingRows|SystemSupplement)'`
  PASS;
  `go test ./internal/context -run 'TestFormatRawToolOutputs|TestBuildPromptContext_ToolSourcedValueGuidance_FinalizeAvoidsUnsupportedGroupCounts'`
  PASS;
  `go test ./internal/agent -run 'TestAnswerDocumentEvaluator_BuildInitialInstruction_(HistoryMemberSetKeepsClosureProse|PrincipalMemberSetSuppressesClosureProse)'`
  PASS.

2026-05-21 Batch F.7:

- Enumeration display rows now carry the same `AnswerEvidenceOrigin` projection
  as the aggregate fact that produced them. This closes a runtime/log/diff
  leakage class where a member such as `goroutine 15 @ internal/foo.go:100` or
  an old diff hunk path could be promoted into a current-checkout citation just
  because it looked like `file:line`.
- Current-source location enrichment is now allowed only when the aggregate
  origin includes `current_source` (or has no typed origin and therefore follows
  the legacy current-source path). Pure `runtime_artifact`, `vcs_diff`,
  `vcs_metadata`, `command_measurement`, `repo_negative_search`, and
  `cross_repo_index` aggregates remain first-class answer evidence but cannot
  borrow current-source support refs implicitly.
- Pre-emit member-set carrier repair follows the same rule: non-current-source
  aggregate rows do not synthesize citation refs or evidence summaries from the
  current checkout. Runtime artifact coordinate-only placeholders such as
  `<native>@runtime:0` are also skipped by the system supplement compiler when
  they have no user-useful note/location, preventing blank artifact补表 rows.
- Runtime-artifact member coverage may be satisfied by visible prose, not only
  by table/list rows. This prevents diagnostic answers that already say
  `main.writeSession` or a goroutine id in the body from receiving a duplicate
  system-generated member table.
- Invalid optional `candidate_role` metadata is normalized to `other` instead
  of rejecting the entire finalizer emit. This is safe because `other` does not
  satisfy any more specific required role; hard role checks still fail if the
  requested typed role is missing.
- Validation:
  `go test ./internal/types -run 'TestCompileEnumerationDisplaySets_(RuntimeArtifactDoesNotPromoteFramePathToCurrentCitation|PreservesNonFileRows)'`
  PASS;
  `go test ./internal/tool -run 'TestNormalizeEmitAnswerBlock_(RepairsInvalidCandidateRoleToOther|NormalizesCandidateRoleAlias)|TestNormalizePrincipalEnumerationRowBlocks_(RuntimeArtifactProseCoveragePreventsSupplement|SkipsRuntimeArtifactCoordinateOnlySupplement|DoesNotDuplicateVCSCommitRowsAlreadyVisible|SystemSupplementSkipsRowsThatWouldCreateBlankCells)'`
  PASS;
 `go test ./internal/types ./internal/tool ./internal/agent ./internal/context ./internal/orchestrator`
  PASS.

2026-05-21 Batch F.8:

- External runtime artifacts with `resolved_files=0` now stay
  observation-only unless there is a separate typed current-checkout anchor
  (`exact_targets`, `required_files`, or resolved runtime frames). This prevents
  analyzer mirror drift such as `current_version_check=true` from turning a
  user request like "which goroutines failed?" into a current-source
  verification task.
- `HasObservationOnlyRuntimeArtifact` and `CompileAnswerIntentContract` now
  share that anchor rule, so explorer/finalizer prompts, citation floors,
  current-status scaffolds, and deterministic row compilers agree on the same
  origin boundary.
- The analyzer skill now documents the same direct-classification rule as VCS
  history: external-source log/trace sections with `resolved_files=0` should not
  trigger source-code pre-scan merely to classify stack-frame literals. If the
  user asks for current-code verification, the analyzer should express that as
  structured current-version / exact-target fields and let explore verify it.
- Runtime artifact row coverage also recognizes compact numbered prose such as
  `goroutine（15、87、120）`, avoiding duplicate system supplement tables when the
  final answer already covers every runtime member naturally.
- Validation:
  `go test ./internal/types -run 'TestCompileAnswerIntentContract_ExternalRuntimeArtifact'`
  PASS;
  `go test ./internal/agent -run 'TestAnalysisSkill_PromptDocuments(DirectHistoryClassification|ExternalRuntimeDirectClassification)|TestBuildAnalysisIR_ExternalOnly(SpuriousCurrentVersionCheckStaysObservationOnly|CurrentVersionCheckKeepsCurrentStatus)'`
  PASS;
  `go test ./internal/tool -run 'TestNormalizePrincipalEnumerationRowBlocks_RuntimeArtifact(GoroutineShorthandPreventsSupplement|ProseCoveragePreventsSupplement|SkipsRuntimeArtifactCoordinateOnlySupplement)'`
  PASS;
 `bash eval/run.sh eval/cases/logtri_goroutine_dump.case 1`
  PASS (`logtri_goroutine_dump-20260521-013104`: `analyzer_iters=1`,
  `tool_read_file=0`, `midloop_inject=0`, `explorer_iters=1`,
  `finalizer_iters=1`, evidence origins only `runtime_artifact`).

2026-05-21 Batch F.9:

- Finalizer emission now performs a safe, local citation normalization for
  runtime artifacts before pre-emit gates run. If the answer is
  observation-only (`external_only_log` / `external_only_trace` without a
  current-checkout verification anchor), the system preserves the visible
  answer text but removes repo-style `citations[]` entries and downgrades their
  item carriers to `citation_ref=-1`.
- Mixed runtime/current answers keep valid current-source citations. Only
  artifact-side frame coordinates that the typed drift map identifies as
  observed runtime locations are removed; remaining citation indexes are
  remapped deterministically. This prevents external paths such as
  `src/main.cj:18` from rendering as if they were current-checkout source
  citations, without forcing another finalizer rewrite.
- Validation:
  `go test ./internal/tool -run 'TestNormalizeRuntimeArtifactCitationRefs|TestPreCheckArtifactObservedFrameCitations'`
  PASS.

2026-05-21 Batch F.10:

- Observation-only runtime artifacts now have a decisive analyze/explore route:
  when structured log/trace triage says the artifact is external to the current
  checkout (`resolved_files=0`) and the user did not request current-code
  verification, analyzer/explorer prompts skip repository prescan and source
  evidence tools. Explorer keeps only `emit_investigation_complete` for
  runtime-artifact closure facts; `emit_evidence` remains reserved for
  current-checkout source anchors.
- Deterministic answer normalizers now respect the same origin boundary.
  Runtime-only call chains, goroutine IDs, native frames, and artifact members
  no longer flow through current-source enumeration carriers, aggregate member
  coverage gates, cardinality gates, or system补表 compilers. The visible model
  answer remains authoritative unless the user explicitly asks for a raw stack
  table or current-code verification.
- Validation:
  `go test ./internal/tool ./internal/types ./internal/agent ./internal/context ./internal/orchestrator`
  PASS;
  `bash eval/run.sh eval/cases/harmony/hilog_cangjie_panic.case 1`
  PASS (`hilog_cangjie_panic-20260521-021900`: `tool_read_file=0`,
  `midloop_inject=0`, `analyzer_iters=1`, `explorer_iters=2`,
  `finalizer_iters=1`, `semantic_quality_dispatches=0`, no runtime citation
  pool, no system-generated member table).

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
- [x] Batch B.6: classify git commands executed through `exec_command` into
  VCS metadata/diff origins without relying on user/model prose.
- [x] Batch B.7: keep pure recent-N history enumeration in the VCS lane and add
  `u7l` eval coverage for recent 10 commits with purpose/impact.
- [x] Batch B.8: teach analyzer direct repository-history classification and
  prevent user-visible leakage of `citation_ref` / `citations[]` in VCS answers.
- [x] Batch B.2: route decorated commit-hash support-ref exception through
  unified VCS origin projection.
- [ ] Batch B: remove remaining VCS/hash compatibility fallback once structured
  tool-emitted origin fields fully cover archived/replayed payloads.
- [x] Batch C.1: tag log/perf triage tool outputs with runtime artifact origin.
- [x] Batch C.2: add runtime artifact frame/observation claim bindings.
- [x] Batch F.7: make enumeration display/system supplement origin-aware so
  runtime artifact and VCS/diff members cannot become implicit current-source
  citations.
- [x] Batch F.8: keep external-source runtime artifacts observation-only unless
  a typed current-checkout verification anchor exists.
- [x] Batch D.1: compile aggregate claim bindings and render them in the final
  answer-writing prompt.
- [x] Batch D.2: make pre-emit scalar/value coverage consume claim bindings
  before forcing visible exact values.
- [x] Batch D.3: add typed VCS-history narrative and mixed current-source
  convergence boundary.
- [x] Batch D: make reviewer and pre-emit support lanes consume claim bindings
  for retry/local-repair decisions.
- [x] Batch D.4: align reviewer input to the final V2 visible surface
  (titles, table columns, structured cells, diagrams for semantic review).
- [x] Batch D.5: suppress low-precision generic soft caveats only after a
  high-confidence sufficient semantic-review verdict.
- [x] Batch E.1: enforce and test system supplement safety invariants for the
  existing deterministic补表 compilers.
- [x] Batch E.2: prevent role-only aggregate reconciliation from expanding
  sibling same-role buckets into one another.
- [x] Batch F.1: add upstream `Evidence Origin Boundary` prompt section for
  explorer/extractor and pin that finalizer does not get duplicate contract
  copies from `BuildPromptContext`.
- [x] Batch F.2: canonicalize exact legacy aggregate provenance tokens into
  explicit structured origin dimensions during aggregate normalization.
- [x] Batch F.3: teach VCS tool descriptions to preserve the VCS provenance lane
  and avoid fake `emit_evidence` rows.
- [x] Batch F.4: add reviewer/pre-emit tests for diff-only, trace-only, and
  negative-search-plus-present-source mixed answers; materialize
  `result_count=0` on negative-search aggregate dimensions.
- [x] Batch F.5: harden semantic-quality reviewer JSON compatibility and add
  soft finalizer guidance against unsupported VCS grouping counts.
- [x] Batch F.6: suppress imprecise VCS row-order reviewer false positives,
  avoid duplicate system VCS supplements, and preserve richer VCS raw/closure
  context for finalization.
- [ ] Run targeted evals after each batch and update
  `eval_20260520_full_sweep_gap_tracking.md`.
