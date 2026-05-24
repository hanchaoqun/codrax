# Gap Architecture Retriage

Date: 2026-05-23

Status: active implementation tracker. Local-small-model compatibility gaps are
explicitly out of scope for this pass unless they expose a shared production
contract defect.

## Goal

Re-audit the current gap ledgers and recent eval logs, then decide whether the
remaining issues are isolated defects or architecture gaps. This document is the
handoff for the next batches, so implementation does not depend on chat memory.

The non-negotiable boundary is unchanged:

- user intent and model-authored useful answer content are primary;
- system repairs are append-only, localized, and typed;
- hard gates consume precise structured signals only;
- grounded in-scope evidence and its rich summaries must outrank closure prose,
  stale aggregate counts, and generic support hints;
- external observations such as git, logs, traces, commands, cross-repo index,
  external documents, web, MCP, and connectors are first-class evidence through
  origin-specific refs, not fake current-source `file:line` citations.

## Inputs Reviewed

- `docs/design/eval_20260520_full_sweep_gap_tracking.md`
- `docs/design/eval_20260522_full_sweep_gap_tracking.md`
- `docs/design/unified_evidence_answer_contract_20260520.md`
- `docs/design/observation_ledger_contract_20260521.md`
- `docs/design/principal_ledger_prompt_convergence_20260523.md`
- `docs/design/json_payload_cognitive_load_gap_20260523.md`
- `docs/design/answer_surface_safety_batch_20260523.md`
- Targeted replay logs under:
  - `eval/results/principal-ledger-20260523-211258`
  - `eval/results/principal-ledger-u7l-fix-20260523-213328`
  - `eval/results/principal-ledger-short-hash-20260523-215117`
  - `eval/results/u7l-20260523-223616`

## Current Code Audit

The repo already has the right primitives. The next batches should reuse these
instead of introducing another evidence stack.

| Contract | Current code | Audit result |
| --- | --- | --- |
| Unified observation records | `internal/types/observation_ledger.go` | Good base: has current source, VCS, diff, command, runtime artifact, cross-repo index, external document, web, MCP, connector origins, source refs, spans, provenance lanes, row-set refs. |
| Shared prompt projection | `internal/types/observation_prompt_projection.go` | Good base: finalizer/reviewer share ranking, rich-note dedupe, origin-aware note budgets, and source/span formatting. |
| Context adapters | `internal/types/observation_ledger_context.go` | Good base: accepted Turn A artifacts outrank analyzer/pre-scan noise; evidence dedupe exists. |
| Finalizer observation prompt | `internal/agent/answer_document_evaluator.go` | Mostly converged on Observation Ledger, but some retry hints and schema surfaces still need continued audits for sentinel/internal wording. |
| Semantic reviewer observation prompt | `internal/orchestrator/semantic_quality_reviewer.go` | Shares the observation projection; remaining gaps are mostly reviewer-locus / advisory vs rewrite policy, not a separate ledger. |
| System supplement compilers | `internal/tool/answer_document_principal_enum_compile.go`, `internal/tool/answer_document_pre_emit_check.go` | Much safer after recent batches, but this remains a high-risk surface because deterministic supplements can still look principal if future code bypasses visible-surface coverage checks. |
| Origin helpers | `internal/types/answer_evidence_origin.go`, `internal/types/answer_claim_binding.go` | Shared helpers exist, but `observation_ledger_contract` still tracks scattered origin checks in pre-emit / contract / reviewer as an open cleanup item. |

## A/B Decision

Decision: **A — architecture remediation is still required**, but it should be
incremental and reuse existing contracts.

Reasoning:

1. The remaining high-ROI gaps are not single-case bugs. They are repeated
   boundary failures between typed evidence carriers, model-authored answer
   surfaces, deterministic supplements, reviewers, and retry/status telemetry.
2. Recent code has already closed many symptoms: schema-aware repair,
   origin-specific member sets, row-set refs, exact VCS changed paths,
   observation prompt projection, and visible supplement localization. The next
   work should consolidate these primitives rather than patch prompts.
3. Broad finalizer rewrites are the wrong default for the remaining class.
   Most residuals should be local typed repair, localized supplement, boundary
   disclosure, or telemetry-only advisory unless a precise structural defect
   would otherwise ship.

## Systemic Root Causes

### R1. External Observations Are Accepted, But Not Fully Future-Proofed

Git/log/trace/command are now mostly first-class through the observation
ledger. MCP has a support-only adapter. Web, external document, and connector
origins exist in the type system, but their future producer contract and eval
skeletons are not yet complete.

Risk if ignored: the next external source family may add a bespoke path and
reintroduce fake `file:line`, `citation_ref=-1`, or raw-prose-only support.

### R2. Gate Consumers Still Have Scattered Origin Decisions

The type layer exposes `ObservationRecordHasCurrentSourceLineSpan`,
`ObservationRecordHasStrongCurrentSourceAnchor`,
`AnswerClaimBindingHasExactCurrentSourceSupport`, and
`AnswerEvidenceOriginCarriesOriginSpecificSupport`. Some pre-emit, contract,
and reviewer branches still own local origin/routing decisions around current
source, runtime artifact, and no-source citation compatibility.

Risk if ignored: a future change may correctly add an external observation to
the ledger but still trip an old current-source citation gate or broad rewrite.

### R3. System Supplements Remain A Powerful Escape Hatch

Recent batches made supplements localized, append-only, and less dry. Still,
the compiler paths are inherently dangerous: they can improve completeness but
also make the final answer look worse than the model's own table or prose.

Risk if ignored: a new supplement path can publish a competing "complete system
table" or generic caveat even when the model-authored answer is sufficient.

### R4. Rich Evidence Ranking Is Mostly Centralized, But Eval Coverage Is Thin

`PrioritizeObservationRecords` preserves one best record per requested origin
and boosts strong current-source rows only when current source is requested.
This is the correct contract, but executable coverage is still stronger for
VCS/log/trace than for future MCP/web/connector and cross-repo-index facts.

Risk if ignored: a mixed request such as "based on MCP docs/log line/diff,
analyze current code" can lose one lane under prompt budgeting.

### R5. Retry/Review Telemetry Is Improving, But Advisory Work Can Still Look
Like Rewrite Pressure

Retry counters and combined reviewer notices are better. The open systemic
handoff gap G76 still says advisory-only contract checks can dominate accepted
runs and look like finalizer pressure.

Risk if ignored: customer UX still reads "the system is fighting itself" even
when the answer is accepted.

### R6. Runtime Artifact Mixed Requests Need Explicit Two-Lane Language

The ledger and request traits already distinguish observation-only runtime
artifacts from mixed artifact + current-source requests, but one prompt surface
still used wording equivalent to "the answer must come from the log/trace
itself" whenever `resolved_files=0`. That wording was too broad: it was correct
for artifact-only questions, but it caused the analyzer to ignore a separate
user request to explain the observation against current source code.

Risk if ignored: any log/trace/web/MCP/connector fact that has no direct current
source hit can suppress a legitimate current-code explanation lane. The failure
mode is subtle because the artifact-only answer may look plausible while still
violating the user's requested evidence mix.

## Priority Order

1. **Batch A — external observation contract hardening.**
   Lock down future web/external-document/MCP/connector shape and source/span
   projection with code tests. This is low-risk and prevents future evidence
   families from inventing separate carriers.
2. **Batch B — ledger-based origin gate consolidation.**
   Audit and migrate the highest-risk pre-emit / reviewer / contract origin
   checks to shared helpers. Add tests proving external observations prefer
   local supplement/boundary disclosure over finalizer rewrite when a current
   source citation is not required.
3. **Batch C — system supplement safety fence.**
   Add a developer-facing regression suite that every deterministic supplement
   path must pass: preserve model-authored prose/tables, append only
   non-overlapping typed facts, never render all-empty or principal-looking
   tables, and localize system-authored labels.
4. **Batch D — eval expansion and log audit.**
   Add or refresh evals for:
   - git diff hunk + current code;
   - log line + current code;
   - trace window + current code;
   - MCP JSON field exists/absent skeleton;
   - web paragraph contains / absent skeleton;
   - connector row exists / absent skeleton;
   - cross-repo index fact + current source.
5. **Batch E — advisory-cost telemetry cleanup.**
   Separate expensive advisory-only checks from hard contract checks in
   telemetry and status. Do not change hard safety semantics.

## Batch Task List

| Batch | Status | Task | Code / Doc Areas | Validation |
| --- | --- | --- | --- | --- |
| T0 | Done | Create this retriage document with A/B decision, root causes, and task order. | `docs/design/gap_architecture_retriage_20260523.md` | Doc review |
| T1 | Done | Add code-level tests that aggregate facts for web, external docs, MCP, connectors, cross-repo index, VCS, command, log, and trace all project through `ObservationLedger` with origin-local `SourceRef` / `ObservationSpan`, not current-source citations. | `internal/types/observation_ledger.go`, `internal/types/observation_ledger_test.go`, `internal/types/observation_prompt_projection_test.go` | `go test ./internal/types` |
| T2 | Done | Extend MCP response projection, if current fields are too weak, without duplicating the ledger. Prefer typed fields only when producers can populate them; otherwise keep support-only `RawRef`. | `internal/types/context.go`, `internal/types/observation_ledger.go`, MCP tests | `go test ./internal/types ./internal/mcp` |
| T3 | Done | Audit pre-emit / contract / reviewer origin decisions and replace local origin switches with shared ledger helpers where safe. | `internal/tool/answer_document_pre_emit_check.go`, `internal/orchestrator/contract_check.go`, `internal/orchestrator/semantic_quality_reviewer.go`, `internal/types/answer_claim_binding.go`, `internal/types/answer_evidence_origin.go` | `go test ./internal/types ./internal/tool` |
| T4 | Done | Add supplement safety guard tests across current-source, VCS, runtime, command, cross-repo, web/MCP/connector-like origins. | `internal/tool/*supplement*_test.go`, `internal/render/answerdoc_test.go` | `go test ./internal/tool` |
| T5 | Done | Add executable eval cases for existing producers and placeholder/documented skeletons for future MCP/web/connector producers. | `eval/cases`, docs | `bash -n eval/cases/read_combo_*.case` |
| T6 | Done | Run targeted eval batch and refresh gap ledgers with every retry/reject, classifying model error vs system over-gate. | `eval/results`, `docs/design/eval_*.md` | targeted eval reruns + prompt tests |
| T7 | Done | Reduce mixed VCS/current-source exploration repair cost without relaxing evidence grounding. | explorer mid-loop policy, evidence salience/ranking | targeted VCS eval; compare `midloop_inject` / `explorer_iters` |
| T8 | Done | Deepen answer-document JSON recovery for JSON-encoded `blocks[]` / native-array confusion so light model syntax slips do not cause visible finalizer reject loops. | answer-document tool param compat / recovery | focused finalizer recovery unit tests |
| T9 | Done | Prompt-only runtime mixed-lane guidance is not sufficient. Add a typed `current_source_explanation_profile` that reuses existing `AnswerIntentContract`, runtime observation-only routing, and `ObservationLedger` instead of creating a duplicate evidence stack. | `docs/design/current_source_explanation_profile_20260524.md`, analyzer schema / request traits / finalizer prompt | typed unit tests + regression evals for log+code, trace+code, VCS+code, command+code |
| T10 | Done | Preserve explicit user-requested answer dimensions (for example `diff 线索 / 当前关键代码 / 作用 / 影响`) through analyzer → surface plan → finalizer prompt without hard gates or system table replacement. Typed contract, finalizer prompt, runtime/current-source lane routing, and VCS/log/trace eval coverage are complete. | `docs/design/user_requested_answer_dimensions_20260524.md`, analyzer schema, `AnswerPresentationContract`, `RequestModel.HasRuntimeArtifactCurrentVerificationAnchor`, finalizer prompt, `eval/cases/read_combo_*_dimensions.case` | typed unit tests + focused mixed evidence evals |
| T11 | In progress | Make explorer completion monotonic: accepted parallel closures own principal state, non-winning partial siblings cannot pollute aggregate facts or repair debt, and post-completion support reads become enrichment unless a typed load-bearing facet is still missing. B1-B4 are done; focused eval replay and any follow-up facet partition remain. | `docs/design/explorer_convergence_monotonicity_20260524.md`, `internal/orchestrator/explore_parallel_dispatch.go`, `internal/orchestrator/orchestrator.go`, `internal/types/evidence_closure.go` | parallel convergence unit tests + focused qf/s5b/u7k evals |
| T12 | In progress | Generalize typed relation facts beyond enumeration-only paths while preventing source-inventory repair from rewriting relation member sets. Interface/trait/protocol → implementer relations are foundational repomap facts and must surface for diagrams, mechanism explanations, comparisons, counts, and enumerations when analyzer emits a typed relation axis such as `predicate_axis=implement`. | `internal/context/typed_relations.go`, `internal/types/request_traits.go`, `internal/agent/explorer.go`, `internal/tool/source_inventory_reconcile.go`, `eval/cases/qf_type_relation_loop_controller.case` | typed relation unit tests across repomap languages + focused qf replay |
| T13 | In progress | Relation coverage should become a common typed contract, not an `implements`-only special case. Extend the same "typed relation member + grounded evidence + source scope + model-authored member_set" safety rule to inheritance/subclass, override/conformance, caller/callee, registration/binding, import/dependency, package/export membership, config key→read site, route→handler, event→observer/subscriber, and external observation→source-anchor relations as their precise graph/evidence carriers become available. R1/R2 are done: common typed relation candidate/query/provider types exist, MultiGraph exposes generic candidates for exact implementer rows, and the existing implementer pre-complete gate now runs through the generic coverage helper. Remaining work starts with import/dependency provider R3. Detailed contract and task list are tracked in `docs/design/typed_relation_coverage_contract_20260524.md`. | `internal/types` relation provider boundary, `internal/context` probe/render, existing repomap graph relations, evidence origins | per-relation unit tests, at least one non-Go fixture for each graph-backed relation |
| T14 | Done | Rich row notes can still be rendered dry when the finalizer chooses a Markdown table and puts per-member descriptions only in the summary paragraph. Implemented a localized, append-only verified-note supplement that never rewrites or deletes model tables and fires only when typed principal rows are visible but row-level descriptions are missing. | answer document display supplement, principal enumeration row compiler, `docs/design/typed_relation_coverage_contract_20260524.md` R8 | table/list tests proving model-authored content is preserved and supplement is independent; focused R8 eval passed with `finalizer_iters=1` |

## Implementation Notes For Future Batches

- Do not turn `emit_evidence` into a catch-all. It remains current checkout
  source/config/doc evidence only.
- Do not parse raw user/model prose for hard decisions. Visible-coverage checks
  may inspect the already rendered answer surface to decide whether a typed row
  is visibly covered, but not to infer user intent or evidence origin.
- A non-current observation can be principal answer evidence. It just cannot
  create current-source citation pressure.
- If a model table is good but mechanically incomplete, the system may add a
  clearly marked supplemental block. It must not rewrite or replace the table.
- If a non-critical, non-lossless issue remains, prefer localized boundary
  disclosure or accepted-with-advisory telemetry over finalizer rewrite.

- 2026-05-24 T10 complete:
  - Focused VCS/log/trace evals cover explicit requested dimensions across
    history, runtime log, and runtime trace evidence mixed with current source.
  - A concrete routing gap was found and fixed: request-anchored
    `requested_answer_dimensions.current_key_code` now opens the current-source
    lane for external runtime artifacts. This keeps ordinary observation-only
    artifacts lightweight while honoring explicit "current key code" requests.
  - No deterministic supplement was added. The eval evidence showed the safer
    architecture fix was upstream lane routing; renderer supplements remain a
    last resort only when accepted-path telemetry proves a deterministic,
    append-only supplement is necessary.
  - Follow-up found during T11 B5:
    `read_combo_git_two_diffs_current_code-20260524-113636` completed with
    both VCS and current-source content and `finalizer_iters=1`, but the model
    did not visibly repeat `diff 线索 / 当前关键代码 / 作用 / 影响` under each
    commit. The fix remains prompt-only and soft: ask for per-subject dimension
    labels where possible, without adding a hard gate or system补表.
  - Re-verification:
    `read_combo_git_two_diffs_current_code-20260524-114950` passed after the
    prompt-only label guidance change: `analyzer_iters=2`, `explorer_iters=8`,
    `extractor_iters=1`, `finalizer_iters=1`, `midloop_inject=3`,
    `tool_read_file=4`, and no finalizer rewrite/reject. The answer preserved
    both VCS/current-source evidence and explicit per-commit labels
    `diff 线索与当前关键代码` / `作用与影响`.
  - UX follow-up: analysis-stage `emit_analysis` summaries now surface
    `requested_answer_dimensions` and `current_source_explanation_profile`
    details in the REPL. This is display-only and does not add a prompt/gate
    path.
- 2026-05-24 T11 started:
  - Added `docs/design/explorer_convergence_monotonicity_20260524.md` after
    auditing existing parallel dispatch, fork merge, accepted-closure reuse,
    aggregate fact merge, and Turn A artifact preservation paths.
  - Initial root cause: parallel dispatch cancels siblings after an accepted
    closure, but still merges all already-finished non-error sibling forks in
    declaration order. This lets non-winning partial lanes import broad
    aggregate facts, support-only pending debt, or duplicate StageOutput after
    the typed closure boundary. The first implementation batch will make merge
    winner-aware without weakening explicit multi-facet wait rules.
  - B1/B2 implemented and unit-tested: after accepted early convergence, only
    the winning fork merges into parent principal state. A non-winning partial
    sibling can no longer leak `StageReport`, pending repair debt, retained
    aggregate facts, or Turn A accepted aggregate facts. The existing typed
    wait rules for explicit enumeration, bucketed/diagram, and mixed-origin
    mechanism cases remain guarded by tests.
  - B3 implemented and unit-tested: accepted-closure auto-complete now uses
    shared typed debt helpers. Advisory repairs and known breadth/support reads
    (`phase1_unread`, `chain_promotion.concrete_values_tracer*`) no longer
    reopen exploration after a valid closure; unknown/exact debt remains
    blocking by default, including `primary_anchor`, `required_file_hint`, and
    `multi_path_anchor`.
- 2026-05-24 T12 started:
  - Focused `qf_type_relation_loop_controller` replay passed but was
    semantically incomplete: the answer surfaced the main read-pipeline
    evaluators plus `subExplorerEvaluator`, but omitted `logTriagerEvaluator`,
    `perfTriagerEvaluator`, and `writeAnalyzerEvaluator`, all of which
    implement `LoopController.Observe`.
  - Root cause is not lack of graph data. `repomap.Graph.ImplementersOf`
    already provides a language-neutral typed relation over
    `Symbol.Implements`; the context probe only exposed it for category
    enumeration / relation lookup / count questions. Architecture, diagram,
    mechanism, and comparison questions with `predicate_axis=implement`
    therefore fell back to grep/read discovery and could miss conditional
    pre-stage or write-mode evaluators.
  - Design decision: typed relation hints are foundational evidence context,
    not an enumeration-only feature. They remain prompt hints, not
    system-authored user-facing replacements, and they are emitted only from
    precise analyzer fields plus exact repomap graph relations.
  - Follow-up replay after broadening relation hints exposed a deeper
    source-inventory contract bug: the model emitted a correct typed relation
    member set for 8 production `LoopController` implementers, then
    `source_inventory` reconciliation replaced it with "all exported types in
    `internal/agent/agent.go`" (`StageOutput`, `Dependencies`,
    `ToolSchemaFilter`, `streamPreviewBuffer`, ...). This is a system
    overreach, not a model error. Fix direction: source inventory may repair
    explicit package/file symbol inventory questions, but when typed request
    fields identify a relation-shaped answer (`predicate_axis=implement` or
    relational lookup), it must not rewrite or append principal relation
    members. This preserves relation facts for every repomap-supported
    language and keeps system supplements append-only rather than replacing the
    model's relation answer.
  - A second replay showed why the previous bug caused so many retries:
    analyzer could emit both `predicate_axis=implement` /
    `is_relational_lookup=true` and `source_inventory_profile=true`. The
    source-inventory scope filter then treated `LoopController` as a file/path
    inventory scope and demoted the correct relation member_set to
    `supporting_coverage`, even when the model explicitly set
    `role=principal_answer`. T12 therefore treats typed relations as a higher
    precedence contract: emit_analysis drops source-inventory profiles for
    relation-shaped requests, aggregate role normalization ignores
    source-inventory scope filters for typed relations, and source-inventory
    rewrites always append `system:source_inventory` provenance when they do
    legitimately repair source inventory questions.
  - Final T12 batch in this turn:
    - Added typed relation pre-complete coverage for implementer sets:
      exploration may not close a relation member_set that omits a production
      implementer when both signals are precise: `Graph.ImplementersOf`
      reports the member and the explorer already emitted grounded evidence
      for that same member/file inside the requested source scope. Graph-only
      members are not forced, and test/doc/auxiliary members remain excluded
      under production scope unless the analyzer explicitly opts them in.
    - Added `types.TypedRelationImplementerSource` so multi-repo implementer
      carriers can participate without `internal/tool` importing concrete
      multigraph code and creating an import cycle. Single-repo continues to
      use `*repomap/types.Graph` directly.
    - Added finalizer prompt guidance that non-empty principal row notes should
      appear on the same row as a description/说明 column or equivalent item
      text. Focused replay `qf_type_relation_loop_controller-20260524-031521`
      now passes and includes all 8 production implementers, including
      `answerDocumentEvaluator`, `logTriagerEvaluator`,
      `perfTriagerEvaluator`, and `writeAnalyzerEvaluator`.
    - Residual UX/content gap: the replay still placed per-member
      descriptions in the summary paragraph while the Markdown table stayed
      dry (`实现类型 / 文件位置 / Observe 方法行号`). This is not evidence loss
      — the prompt contained rich notes and the final answer summary used
      them — but the table-level presentation is weaker than desired. T14 now
      implements an append-only localized supplement instead of system
      replacement.
    - 2026-05-24 T14 complete: added a third principal-enumeration supplement
      mode for verified notes. It preserves model-authored tables/lists/prose,
      appends `系统按已验证证据补充说明：...` / `System-verified note
      supplement: ...` only when visible principal rows are dry, avoids
      duplicating notes already visible elsewhere, and supports non-current
      origins such as VCS rows without inventing current-source citations.
      Validation: `go test ./internal/tool ./internal/types ./internal/agent
      ./internal/orchestrator` and focused eval
      `eval/results/r8-rich-notes-20260524-103156/read_combo_criterion_rich_functions-20260524-103159`
      passed; finalizer stayed one round and preserved rich Chinese row
      descriptions.
  - Relation classes that need the same contract after `implements`:
    inheritance/subclass, override/conformance, caller/callee, registration or
    binding table, import/dependency, package/export membership, config key to
    read site, route to handler, event/observer/subscriber, and external
    observation to current-source anchor. The gate criterion must remain
    source-agnostic: precise typed carrier + grounded observation/evidence +
    source scope + model-authored member_set, never raw keyword matching over
    the user request or model prose.
- 2026-05-24 T9 complete:
  - Added a typed `current_source_explanation_profile` so analyzer can request
    current-source explanation for external observations without overloading
    diagnostic current-version checks or visible answer dimensions.
  - Wired the profile through request traits, answer-intent origins, and
    finalizer prompt projection. Invalid optional profile fields warn/drop
    instead of causing analyzer retries.
  - Added mixed eval coverage for log+code, trace+code, command+code, and
    VCS latest-merge feature+code. Focused batch passed 4/4 and the VCS case
    specifically guards against the customer-reported "latest merge feature"
    answer collapsing to a scalar commit id.
  - During trace validation, found a shared answer-document JSON repair gap:
    string-wrapped `blocks` with a dangling quote after `items[]` caused lossy
    block recovery and a visible finalizer reject. Fixed the common recovery
    layer so every visible block/item is preserved; rerun passed with zero
    answer-document rejects and no system supplement pollution.

## Progress Log

- 2026-05-23 T1 complete:
  - Added a ledger compiler regression covering external document, web page,
    MCP resource, connector resource, cross-repo index, VCS metadata, VCS diff,
    command output, log artifact, and trace artifact aggregate facts.
  - Verified every non-current origin keeps origin-local `SourceRef` /
    `ObservationSpan` details and does not become current-source
    citation-eligible.
  - Reused the existing Observation Ledger instead of introducing a new carrier.
  - Filled existing `ObservationSourceRef` passthrough gaps for `mime_type`,
    `fetched_at`, `tool_call_id`, and cross-repo `path`.
  - Validation: `go test ./internal/types`.
- 2026-05-23 T2 complete:
  - Extended the existing `MCPResponse` carrier with typed resource coordinates
    (`resource_uri`, `payload_ref`, `row_set_ref`, `page_ref`, `mime_type`,
    `json_pointer`, `selector`, `row`, and line span).
  - Projected those fields through `ObservationLedger` as
    `mcp_resource` origin-local support, while preserving the old `raw_ref`
    support-only path.
  - Added a regression proving MCP coordinates do not become current-source
    citation anchors.
  - Validation: `go test ./internal/types ./internal/mcp`.
- 2026-05-23 T3 complete:
  - Audited semantic reviewer and contract-check origin surfaces; both already
    consume compiled claim bindings / observation ledger summaries for the
    high-risk external-origin handoff.
  - Replaced remaining pre-emit local origin switches for "origin-specific
    only" support with shared `types` helpers, so future VCS/log/trace/command/
    cross-repo/web/MCP/connector origins do not need new case-by-case branches.
  - Added helper tests proving current-source, system-inference, and unknown
    origins cannot accidentally suppress current-source citation fallbacks.
  - Validation: `go test ./internal/types ./internal/tool`.
- 2026-05-23 T4 complete:
  - Added an external-resource member supplement guard proving system-generated
    resource tables are append-only, explicitly marked, localized, rich-note
    preserving, and do not invent current-source citations.
  - Added web/MCP/connector negative-observation guards proving no-hit
    supplements localize origin labels, keep payload/row/tool-result details,
    and stay citation-free.
  - Existing current-source, VCS, runtime, command, and architecture supplement
    guards remain covered by the `internal/tool` suite.
  - Validation: `go test ./internal/tool`.
- 2026-05-23 T5 complete:
  - Added executable mixed-observation evals for the currently supported
    producers:
    - `read_combo_git_two_diffs_current_code`: recent two commit diffs plus
      current-source implementation impact, guarding against scalar commit-id
      collapse.
    - `read_combo_log_current_code_boundary`: attached log line plus current
      source boundary, guarding against treating artifact lines as source
      citations.
    - `read_combo_trace_current_code_boundary`: HiTrace duration plus current
      source explanation, guarding against trace line/source line conflation.
  - Added `external_observation_skeletons.md` for future MCP/web/connector eval
    producers. These are intentionally not `.case` files until the runner has
    first-class typed attachment knobs for those origins.
  - Validation: `bash -n eval/cases/read_combo_git_two_diffs_current_code.case
    eval/cases/read_combo_log_current_code_boundary.case
    eval/cases/read_combo_trace_current_code_boundary.case`.
- 2026-05-23 T6 complete:
  - Targeted batch before the fix: 2/3 PASS.
    - `read_combo_git_two_diffs_current_code`: PASS, no finalizer reject, but
      high exploration cost (`explorer_iters=41`, `midloop_inject=19`). The
      answer preserved VCS diff + current-code lanes and did not collapse to a
      scalar commit id.
    - `read_combo_trace_current_code_boundary`: PASS, preserved trace duration
      and current-code explanation, but had one light finalizer reject:
      `structured recovery could not preserve every visible blocks[] item`;
      second emit succeeded. Track under T8 rather than changing answer
      semantics.
    - `read_combo_log_current_code_boundary`: FAIL because the answer stayed
      observation-only and emitted no current-source file:line even though the
      user asked to combine the log with current source.
  - Root cause for the failing log case: two prompt surfaces were fighting the
    unified evidence contract. The Log/Trace Triage external-source directive
    said the answer must come from artifact semantics, while the analyzer
    runtime shortcut separately allowed mixed current-source verification. The
    broader wording won, so the analyzer emitted
    `current_version_check=false` and no `required_files` / `exact_targets`.
  - Fix:
    - Log/Trace Triage now says artifact facts must stay in the attached-log /
      attached-trace lane, but mixed requests must keep a separate
      current-source lane.
    - Analyzer runtime shortcut now distinguishes current-status diagnostics
      from mechanism explanations backed by current code: the former can use
      `current_version_check`; the latter should use `required_files` /
      `exact_targets` when the pre-scan finds concrete anchors.
  - Validation:
    - `go test ./internal/context -run 'TestFormat(Log|Perf)TriageStructured_ExternalSourceDirective'`
    - `go test ./internal/agent -run 'TestAnalyzerPrompt_RuntimeObservationOnlyShortcut|TestBuildAnalysisIR_ExternalOnly|TestValidateObservationOnlyRuntimeToolCall|TestBuildToolSchemas_ObservationOnlyRuntime'`
    - Rerun `read_combo_log_current_code_boundary`: PASS. The final answer
      cited current-source files including `internal/orchestrator/user_messages.go`,
      `internal/orchestrator/read_stage_retry.go`,
      `internal/render/renderer_dock.go`, `internal/llm/openai.go`, and
      `internal/agent/answer_document_evaluator.go`, while preserving the log
      line as runtime-artifact evidence.
  - Residuals:
    - The passing log rerun still used `explorer_iters=24` and
      `midloop_inject=12`, plus one accepted `emit_investigation_complete`
      retry caused by an underspecified `negative_observation` aggregate fact.
      This is not a final answer correctness issue, but it is a cognitive-load
      / tool-contract polish gap.
    - Semantic reviewer accepted the answer. Self-consistency noted a line
      mismatch (`6942` vs `6943`) at confidence below rewrite floor, so the
      system correctly did not force another finalizer rewrite. Keep observing
      this class under advisory-cost telemetry rather than hard gating.

- 2026-05-23 T7 partial:
  - Integrated the upstream explorer-tool-surface narrowing fix already present
    on `main` (`c9d1fe22`): when evidence backlog / no-emit escalation is active,
    the explorer tool list is narrowed to materialization tools instead of
    continuing broad reads. This reused the existing `FilterToolSchemas` hook
    and avoided a duplicate gating layer.
  - Rerun `read_combo_git_two_diffs_current_code`: PASS, runtime reduced from
    426s to 231s, `explorer_iters` from 41 to 34, and `midloop_inject` from 19
    to 13. No finalizer retry was needed.
  - Residual found in the accepted answer: deterministic system supplements for
    VCS changed-file sets were rendered as partial tables titled like complete
    member tables and used the primary column label `提交` even when every row
    was a file path. The same system-generated partial count then triggered a
    soft count advisory (`expected_count=4/6`, visible supplement count=3/4),
    which leaked as a generic "验收未达到预期" note.
  - Fix in this batch:
    - Missing-row supplements are now titled as
      `系统按已验证证据补充缺失成员...` /
      `System-verified missing member supplement...`, making the append-only
      and partial nature explicit.
    - The aggregate-count checker ignores these system missing-member
      supplement counts, because they represent only rows not already visible in
      the model answer. Complete model-authored tables and verified-field
      supplements remain checked.
    - The primary column label now follows the row's structural surface before
      origin: path-like rows (using the shared code/config path grammar that
      covers all repomap-supported language extensions plus config/docs) render
      as `文件` / `File`; VCS commit rows still render as `提交` / `Commit`.
  - Guard tests:
    - VCS recent-commit supplements still use the commit column.
    - VCS changed-file supplements use the file column and no longer look like a
      commit table.
    - System missing-member supplements do not trip aggregate cardinality drift.
    - Existing external-resource append-only supplement tests were updated to
      the clearer missing-member title.
- 2026-05-24 T7 complete:
  - Added accepted-path caveat filtering for typed facet coverage telemetry:
    `facet_uncovered` now stays user-visible for exact/scalar/enumeration/
    explicit-diagram/relational-comparison surfaces, but architecture/mechanism
    answers no longer append generic "coverage may be insufficient" notes for
    non-requested `component_relation` / `diagram_spine` metadata.
  - Replaced generic self-consistency caveats with a specific, localized
    summary/body conflict note when the reviewer already emitted structured
    contradiction claims. This keeps the no-rewrite policy while making the
    shipped boundary actionable.
  - Fixed a second supplement path: the older aggregate member-set carrier now
    reuses the deterministic row compiler's system-supplement coverage, so a
    member set rendered once as a missing-row table is not appended again as a
    dry ordered list.
  - Guard tests:
    - explicit diagram requests keep diagram facet/block caveats;
    - architecture/history accepted answers suppress optional facet telemetry;
    - relational comparisons keep `component_relation` caveats;
    - self-contradiction caveats include the conflicting claims rather than the
      generic family template;
    - aggregate member carriers do not duplicate rows already covered by a
      system row supplement, while legacy carrier tests still materialize rows
      when only prose mentions members.
  - Validation:
    - `go test ./internal/tool`
    - `go test ./internal/orchestrator`
    - Focused eval `read_combo_git_two_diffs_current_code`:
      - PASS after fixes.
      - Latest run: 214s, `explorer_iters=8`, `midloop_inject=4`,
        `finalizer_iters=3`, `semantic_quality_concerns=0`.
      - Final visible answer no longer has duplicate carrier/list supplements;
        system supplements are limited to append-only missing changed-file
        groups whose covered rows were not already in the model-authored body.
  - Residual recorded for next JSON/payload batch:
    - The latest eval still had two light finalizer rejects before success:
      `structured recovery could not preserve every visible blocks[] item`;
      logs show the model attempted to emit JSON-encoded blocks rather than a
      native `blocks[]` array. The final answer shipped correctly after a third
      emit, so this is not an answer correctness issue, but it remains a
      latency/model-cognitive-load gap under the JSON Payload cluster.

- 2026-05-24 T8 started:
  - Root cause from `read_combo_git_two_diffs_current_code-20260524-000524`:
    the first two finalizer emits contained a complete user-facing answer, but
    `blocks` was a JSON-encoded string and one `diagram` block missed the outer
    block-closing brace before the following caveat block. Existing recovery
    could preserve two structured blocks plus the diagram attachment, but the
    caveat block was swallowed by the malformed diagram candidate, so the tool
    correctly rejected as lossy and forced two model retries.
  - Planned fix: add an answer-document-specific, lossless-only structural
    repair for stringified `blocks[]`: when scanning the top-level blocks array,
    if a top-level block object is still open and the next array element starts
    with a new object, insert the missing outer `}` and accept only if the full
    array then parses into valid block-shaped objects. This does not inspect
    user/model prose for semantics, does not invent answer content, and does not
    publish partial recovery when the repair is uncertain.

- 2026-05-24 T8 complete:
  - Implemented one shared answer-block-array syntax repair helper used by both
    full `emit_answer_document.blocks` and patch
    `emit_answer_document_patch.add_blocks/replace_blocks`. The generic JSON
    layer still lives in `internal/toolparam`; the new helper is deliberately
    answer-document-specific because it must validate `id`, `kind`, and the
    block-kind enum before accepting the repair.
  - Added regressions for:
    - stringified full `blocks[]` where a diagram block misses its outer block
      close before the next caveat block;
    - the same shape under patch `add_blocks`;
    - the prior lossy fallback path still rejecting unattached dropped blocks.
  - Validation:
    - `go test ./internal/tool`
    - `go test ./internal/orchestrator`
    - `git diff --check`
    - Focused eval
      `read_combo_git_two_diffs_current_code-20260524-002022`: no
      answer-document reject loop (`finalizer_iters=1`, no
      `structured recovery could not preserve every visible blocks[] item`).
      The case still failed its literal regex because the final answer did not
      put `diff/current-source` wording in the expected shape; that is a
      separate user-surface preservation gap, not a JSON recovery failure.

- 2026-05-24 T10 started:
  - Added `docs/design/user_requested_answer_dimensions_20260524.md`.
  - Root cause: mixed-evidence answers can preserve origins and summaries while
    dropping the user-visible dimensions explicitly requested in the question.
    This is an answer-surface contract gap, not a JSON repair or VCS evidence
    gap.
  - Direction: add a soft, typed analyzer profile for user-requested dimensions
    and project it into the existing `AnswerPresentationContract`. The profile
    must guide finalizer presentation only; it must not become a hard rewrite
    gate and must not let deterministic system补表 replace model-authored
    content.

- 2026-05-24 T10 B1/B2 complete:
  - Added `RequestedAnswerDimensionProfile` as a soft analyzer-emitted
    presentation contract with current-request provenance validation.
  - Projected dimensions through `RequestModel`, `QuestionStructureView`,
    `AnswerSurfacePlan`, and `AnswerPresentationContract`.
  - Rendered a localized finalizer prompt section that asks the model to keep
    user-requested dimensions visible without replacing richer content with a
    system table.
  - Added tests covering normalization, `emit_analysis` persistence, semantic
    view projection, and finalizer prompt rendering.
