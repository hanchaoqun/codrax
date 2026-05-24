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
ledger. MCP, web, external document, connector, and cross-repo index origins
also have type-level source/span contracts, prompt projection coverage, and
origin-specific citation guards. The remaining risk is future producer plumbing
and broader executable eval replay, not a missing second evidence stack.

Risk if ignored: the next external source family may bypass the ledger and
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

### R7. Extractor Still Leaks Current-Source Citation Pressure Onto External Evidence

2026-05-24 focused convergence replay exposed an accident-class redline
failure: in mixed VCS/current-source and command/current-source cases the model
correctly complained that commit/diff/command observations are not repository
`file:line` facts, but `emit_hypothesis_verdict` and extractor no-tool recovery
still pushed the model toward repo citations or `emit_answer_symbol`.

This is not a single VCS bug. The same shape applies to git metadata, git diff
hunks, runtime logs/traces, bare command measurements, cross-repo index facts,
external documents, web pages, MCP resources, and connector responses. These
origins are already represented by `AnswerEvidenceOrigin` and
`ObservationLedger`; extractor and verdict tools must consume those typed
origins instead of inventing per-origin exceptions.

Redline: a non-current-source observation can be principal evidence, but it
must never be forced through current-source citation or symbol-slate channels.
When the model supplies an origin-specific reference such as `git_log: ...`,
`git_diff: ...`, `exec_command: ...`, `mcp_resource: ...`, or `web_page: ...`,
the tool may accept it only if the typed ledger/aggregate contract proves that
origin is present. Otherwise it remains a normal validation error.

### R8. System Supplements Can Still Overpower Model/User Intent

2026-05-24 replay also exposed another repeated redline failure: deterministic
supplement/materialization paths can append large system-authored tables that
look more authoritative than the model's answer, broaden the requested entity
set, or turn a scoped answer into a generic inventory. This is the same family
as prior `系统按已验证证据补充成员...` regressions.

Highest-level rule: **the system must never use its own structural preference
to override user intent or model-authored useful content.** Deterministic
supplements are allowed only as append-only, localized, independently labeled
repair notes when the missing fact is precise, typed, and non-overlapping.
They must not replace a model table, delete prose, broaden the requested
scope, or materialize a larger row set than the accepted model-authored
aggregate. If the system is unsure, it must prefer no supplement plus an
honest boundary/advisory over a competing table.

Risk if ignored: every new row compiler or aggregate repair path can recreate
the same accident under a different label, making the answer worse while the
pipeline technically "passes."

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
| T11 | Done | Make explorer completion monotonic and lane-owned: accepted parallel closures own principal state, non-winning partial siblings cannot pollute aggregate facts or repair debt, post-completion support reads become enrichment unless a typed load-bearing facet is missing, typed explore-lane ownership now reaches BusContext/AgentContext, explorer prompt guidance, per-window scoped lane plans, exact duplicate support handoff, and REPL parallel-lane UX. Rich completion summaries are covered by prompt tests: accepted closure reason, member_notes, and TurnA investigation notes are visible downstream as advisory context while typed evidence/support lanes remain authoritative. A later accepted closure may replace the authoritative closure reason, but the superseded model-authored `reason` is preserved as an advisory investigation note so useful tool-call prose is not silently lost. | `docs/design/explorer_convergence_monotonicity_20260524.md`, `docs/design/explore_lane_ownership_20260524.md`, `internal/types/explore_lane_plan.go`, `internal/context/builder.go`, `internal/render/status_messages.go`, `internal/orchestrator/explore_parallel_dispatch.go`, `internal/agent/answer_document_evaluator_test.go`, `internal/agent/extractor_test.go`, `internal/types/observation_ledger_test.go` | typed lane unit tests + prompt tests + render tests + orchestrator scoped-lane tests; `go test ./internal/agent ./internal/types ./internal/tool -run 'TestAnswerDocumentEvaluator_BuildInitialInstruction_(HistoryMemberSetKeepsClosureProse|RendersRequiredMechanismAnchors)|TestExtractor_BuildPrompt|TestCompileObservationLedger_AggregateRichNotesPreferMemberNotes|TestEmitInvestigationComplete|TestNormalizePrincipalEnumerationRowBlocks'` |
| T12 | In progress | Generalize typed relation facts beyond enumeration-only paths while preventing source-inventory repair from rewriting relation member sets. Interface/trait/protocol → implementer relations are foundational repomap facts and must surface for diagrams, mechanism explanations, comparisons, counts, and enumerations when analyzer emits a typed relation axis such as `predicate_axis=implement`. | `internal/context/typed_relations.go`, `internal/types/request_traits.go`, `internal/agent/explorer.go`, `internal/tool/source_inventory_reconcile.go`, `eval/cases/qf_type_relation_loop_controller.case` | typed relation unit tests across repomap languages + focused qf replay |
| T13 | In progress | Relation coverage should become a common typed contract, not an `implements`-only special case. Extend the same "typed relation member + grounded evidence + source scope + model-authored member_set" safety rule to inheritance/subclass, override/conformance, caller/callee, registration/binding, import/dependency, package/export membership, config key→read site, route→handler, event/observer/subscriber, and external observation→source-anchor relations as their precise graph/evidence carriers become available. R1/R2/R3/R4/R5/R6/R7/R8/R9/R9a/R9b are done. `extends` consumes language-neutral `Relation.Kind=inheritance`; `called-by`, `imports/exports`, registration evidence, external-observation source anchors, ChangeImpact `references/type_usage` prompt hints, config-key `configures` prompt hints, and route-handler `routes-to` prompt hints all use the shared selector/provider boundary. Reference/type-usage, configures, and routes-to remain prompt-only and exact-target/evidence-gated; ambiguous rows never feed hard coverage. Remaining high-ROI work is observer/subscriber, override/conformance, and broader eval/telemetry before considering any new hard selector. Detailed contract and task list are tracked in `docs/design/typed_relation_coverage_contract_20260524.md`. | `internal/types` relation provider boundary, `internal/context` probe/render, existing repomap graph relations, observation ledger origins | per-relation unit tests, mixed external/current-source evals |
| T14 | Done | Rich row notes can still be rendered dry when the finalizer chooses a Markdown table and puts per-member descriptions only in the summary paragraph. Implemented a localized, append-only verified-note supplement that never rewrites or deletes model tables and fires only when typed principal rows are visible but row-level descriptions are missing. | answer document display supplement, principal enumeration row compiler, `docs/design/typed_relation_coverage_contract_20260524.md` R8 | table/list tests proving model-authored content is preserved and supplement is independent; focused R8 eval passed with `finalizer_iters=1` |
| T15 | Done | Multi-question requests need a unified investigation-unit contract. `SubTopics[]` is analyzer work decomposition, `Buckets[]` is user answer partition, and REPL "关注点" wording is ambiguous. Added derived `InvestigationPlan`, projected it to `EventAnalysisReady`, localized REPL/status summaries to "调查单元/用户分区", and added focused eval coverage. Runtime scheduling changes remain deliberately deferred to a separate eval-backed design so this batch does not replace model/user intent with system grouping. | `docs/design/multi_question_investigation_units_20260524.md`, `internal/types/investigation_plan.go`, render EventAnalysisReady projection, `eval/cases/read_combo_loose_multi_question_units.case`, `eval/cases/read_combo_log_current_source_bucketed_units.case` | `go test ./...`; focused evals passed with finalizer 1 round / no finalizer rejects |
| T16 | Done | Harden eval/convergence telemetry so answer/source/evidence content cannot be counted as system retries. Shared control-line helpers now cover finalizer rejects/rewrites, answer-document patch calls, semantic/self reviewers, mid-loop injects, and per-agent iteration/dispatch counters; the Go telemetry collector applies the same control-line fence. | `docs/design/telemetry_control_line_isolation_20260524.md`, `eval/run.sh`, `eval/runner_lib.sh`, `eval/parallel_all.sh`, `eval/parallel_priority.sh`, `eval/telemetry` | `bash eval/runner_lib_test.sh`; `go test ./eval/telemetry`; `read_combo_git_two_diffs_current_code-20260524-191054` reports `fin_reject=0`, `fin_rewrite=0` |
| T17 | In progress | Close the external-observation extraction gap. `emit_hypothesis_verdict` accepts origin-specific references from VCS/diff/command/runtime/cross-repo/external-doc/web/MCP/connector only when those typed origins are present in the accepted ledger/aggregate contract, and keeps them out of current-source citation fields. Extractor no-tool recovery avoids `emit_answer_symbol` for narrative/value/comparison questions already carried by origin-specific evidence. The second implementation slice extends verdict regression coverage to cross-repo index, external documents, web, MCP, and connector origins, fixes a prompt contradiction where an external-ledger narrative disabled `emit_answer_symbol` but still rendered a hard "Anchor skeleton" instruction, and updates the finalizer Observation Ledger prompt to render actual records for cross-repo, external-doc, web, MCP, and connector families instead of checking only static policy text. Remaining work is broader focused eval replay once future web/MCP/connector producer plumbing exists. | `internal/tool/emit_hypothesis_verdict.go`, `internal/agent/extractor.go`, `internal/agent/answer_document_evaluator.go`, `internal/types/observation_ledger.go` | verdict normalization tests for VCS/diff/command/cross-repo/external-doc/web/MCP/connector; extractor soft-stop tests for external-ledger narratives; finalizer prompt tests for cross-repo/web/connector/MCP actual records; focused mixed-origin evals |
| T18 | In progress | Add supplement safety fence v2. System-generated member/table supplements must be append-only, localized, non-overlapping, and bounded by the accepted typed member set. They must never broaden a source inventory, replace a model table, render a larger/competing row set than the accepted aggregate fact, or synthesize an unmarked system summary in front of a model-authored principal carrier. Current implementation suppresses competing enum/member supplements when model-authored carriers exist, limits verified-note supplements to typed requests that asked for explanatory dimensions, suppresses duplicate negative-observation supplements when the model already emitted a structured absent resolution with the relevant typed scope/target visible, and now keeps model-authored table/list carriers as the only visible blocks instead of prepending `principal_enum_summary`. Remaining work is focused eval replay and any display/reviewer gap found there. | `internal/tool/answer_document_principal_enum_compile.go`, `internal/tool/answer_document_pre_emit_check.go`, display supplement tests | regression tests with scoped source inventory, cross-repo comparison, mechanism/config/scalar cases; ban competing system tables and unmarked system summaries when model content covers the answer |
| T19 | In progress | Add "system structural preference must not overpower model/user intent" redline tests across extractor/finalizer/display. Tests now fail if the principal-enumeration compiler rewrites model prose/table cells, synthesizes an unmarked summary before authored carriers, forces a current-source citation for origin-specific evidence, or appends a duplicate supplement after the model already expressed the accepted typed fact structurally. Remaining work is extractor/display assertions and focused eval assertions for no unwanted `系统按已验证证据补充成员` in covered answers. | `internal/tool`, `internal/agent`, `internal/render`, eval guards | unit tests + focused eval assertions for no unwanted system supplements or unmarked system summaries in covered answers |
| T20 | In progress | Add lane novelty / completed-lane throttling. Detailed design now lives in `docs/design/explore_lane_novelty_throttling_20260524.md`. First implementation slice caps exact duplicate support/verification handoff lanes to a small verification budget. Focused `u7k-20260524-191826` proved this slice is safe but insufficient for same-lane deepening (`explorer_dispatches=0`, `explorer_iters=66`, `midloop_inject=19`). The second slice adds same-lane accepted typed-delta advisory: after accepted evidence/aggregate/observation facts exist, repeated source/VCS/command navigation with no new typed delta receives a soft "emit or close" nudge. The latest guard scopes the no-novelty streak by typed origin and suppresses hints when same-origin multi-unit lanes cannot be mapped precisely, so VCS/current-source or same-origin buckets cannot contaminate each other. The newest orchestrator slices add collective typed-lane convergence and coupling-aware pre-dispatch grouping: required owner closures cancel remaining support siblings, and external runtime artifact + current-source verification sub-topics stay in one shared-context dispatch instead of launching duplicate early grep/read loops. Focused `t20-coupled-20260524-230105` kept mixed log/source and trace/source finalizers to one turn with zero rejects/rewrites and reduced `explorer_iters` from 30→9 and 26→7 versus the prior collective replay. Remaining work is not harder scheduling; track the residual log semantic reviewer concern under T11/T17 answer quality, and continue broader external-observation / relation-provider evals. This remains soft scheduling only. | `docs/design/explore_lane_novelty_throttling_20260524.md`, `internal/agent/explorer.go`, `internal/orchestrator/explore_parallel_dispatch.go`, `internal/orchestrator/dag_node_dispatch.go`, `internal/types/explore_lane_plan.go`, `internal/types/investigation_plan.go`, observation ledger deltas, eval telemetry | agent/orchestrator unit tests; focused `u7k`, log/source, trace/source, mixed VCS/current reruns with no finalizer churn; next target is T17/T13 breadth plus T11 answer-quality validation |
| T21 | Done | Mixed-origin read-without-emit prompt conflict: initial prompts correctly declare VCS/diff/log/trace/command/repo-index/external observations as first-class non-file:line evidence, but generic mid-loop read-without-emit wording still said any fact not passed through `emit_evidence` is invisible. `u7k-20260524-194928` showed the model re-anchoring commit clues to design-document title lines. The fix makes read-without-emit hints and tool-surface restriction origin-aware: current-source claims still require `emit_evidence`; origin-specific observations are preserved through `reason` / `aggregate_facts`, and navigation is not narrowed to source emit-only in mixed lanes. Rebuilt `u7k-20260524-200712` / `u7k-20260524-201412` produced good answers with one-turn finalizers; the remaining failure was a brittle eval expectation and the case was updated instead of rerunning. | `internal/agent/explorer.go`, observation origin contract, mixed VCS/current eval | agent unit tests; focused mixed VCS/current prompt audit; `u7k.case` expectation corrected |

### 2026-05-24 Remote Update Task Review

- Remote commit `c20c4fcb` added the repo-lens `source_inventory` tool view
  and the auditable `SourceInventoryObservation` carrier. This is aligned with
  the unified evidence/answer contract: repo-map inventories now enter the same
  ObservationLedger path instead of creating another evidence stack.
- Red-line risk found during review: a repo-lens inventory can produce many
  current-source supporting rows. In mixed questions such as VCS+current source,
  log+current source, trace+current source, or command+current source, those
  mechanical rows must not crowd out the actual answer-bearing external
  observation or load-bearing source evidence.
- Batch decision: keep source inventory advisory and model-visible, but budget
  it at the shared ObservationLedger prompt-projection layer. The source
  inventory set/count row and its `row_set_ref` are preferred over flooding the
  prompt with many member/attribute rows. Pure source-inventory questions keep
  the normal prompt limit because there is no competing origin to protect.
- This is a prompt-budget and evidence-ranking change only. It does not rewrite
  model-authored final answers, does not add system补表, and does not infer
  intent from user/model prose.
- Follow-up red-line cleanup: legacy aggregate-member-set carrier titles
  (`系统按已验证证据补充成员：...` / `System-verified member supplement: ...`)
  are now recognized by the shared system-supplement predicate. This prevents
  older system-generated blocks from being mistaken for model-authored
  principal carriers by later pre-emit/display checks.
- Latest remote refresh through `81ac9726` adds scoped/grouped repo-map
  projection for source-inventory lens calls. Review result: it supports the
  same advisory carrier direction and does not change the T20 task order. The
  task list stays: finish T20 lane ownership / early-duplicate control first,
  then continue T17 broader external-observation producer eval and T13
  relation-provider coverage.

### 2026-05-24 T20/T17 Focused Replay After Remote Update

Focused replay `eval/results/t20-t17-focused-valid-20260524-221651` used the
post-`7879e5b4` code snapshot. A first `/tmp` binary replay was discarded
because the executable-directory config anchor missed `providers.yaml` and fell
back to `placeholder-v1`; that was an eval setup error, not a product result.

Valid replay summary:

- `read_combo_git_two_diffs_current_code`: PASS, finalizer one turn, no reject.
- `read_combo_log_current_source_explanation`: PASS, finalizer one turn, but
  `explorer_iters=63`, `read_file=41`, `midloop=18`.
- `read_combo_trace_current_source_explanation`: PASS, finalizer one turn, but a
  later sibling started breadth/depth search after earlier runtime/current-source
  siblings had already emitted completion.
- `read_combo_command_current_source_explanation`: PASS, finalizer one turn; the
  command measurement stayed in the origin-specific lane instead of being forced
  into repo file:line evidence.
- `u7k`: product answer contained commit lineage and scalar current-code chain
  with one-turn finalizer, but the eval failed on brittle
  `answer_aggregate_fact.go` expectation and still showed `explorer_iters=48`,
  `midloop=27`.

Architectural conclusion: T17 external-observation correctness is mostly
connected for VCS/log/trace/command + current-source mixed questions. The next
highest ROI is T20 scheduling convergence, specifically collective typed-lane
convergence: when mixed-origin handoff rules disable single-winner early cancel,
parallel explore should still cancel remaining support/duplicate windows once
every required typed lane owner has accepted `emit_investigation_complete`.
This is a typed scheduling fix and must not inspect raw user text or model prose.

### 2026-05-24 T20 Collective-Lane Replay After `e477ce42`

Replay `eval/results/t20-collective-20260524-223835` used the post-remote
`e477ce42` snapshot plus collective typed-lane convergence.

- `read_combo_log_current_source_explanation`: PASS, finalizer one turn,
  `finalizer_rejects=0`, `finalizer_rewrites=0`; still
  `explorer_iters=30`, `midloop=20`.
- `read_combo_trace_current_source_explanation`: PASS, finalizer one turn,
  `finalizer_rejects=0`, `finalizer_rewrites=0`; still
  `explorer_iters=26`, `midloop=10`.
- `u7k`: finalizer one turn and the product answer explained commit lineage plus
  the current scalar chain. The eval failure was a brittle expectation requiring
  older file names (`emit_analysis.go` / `answer_aggregate_fact.go` /
  `analyzer_intent.go` / `emit_investigation_complete.go`) even though the
  accepted answer used current stable files such as `analyzer_predicate.go` and
  `request_traits.go`; the case was widened instead of changing product logic.

Systemic conclusion: T17/T20 no longer create finalizer churn in this focused
set. The remaining high-ROI gap is not answer validation, but early explorer
coordination: support siblings can still start near-identical scans before any
owner lane has accepted typed closure. The next batch should add typed
pre-dispatch lane ownership / novelty budgeting and UX-visible lane purpose,
without changing finalizer gates or using raw prose/text matching.

After pulling `81ac9726`, the remote change was reviewed as scoped to
repo-map/source-inventory projection and grouped lens output. It does not change
the parallel explore scheduling contract. Post-pull package tests for
orchestrator, agent, types, tool, and repomap scoped source-inventory tests
passed before the T20.10 focused replay.

### 2026-05-24 T20 Coupling-Aware Dispatch Slice

Implementation follows the existing typed plan instead of adding a new
classifier:

- `CompileInvestigationPlan` now treats external runtime artifact + current
  source verification as `shared_context`. The log/trace observation and the
  current-source mechanism explanation are two facets of one answer, so running
  every analyzer sub-topic as a separate explorer worker produced duplicate
  early scans.
- `exploreWindowDispatchGroups` keeps `shared_context` and `sequential`
  analyzer-decomposition evidence siblings unified, but still splits ordinary
  independent sub-topics and explicit user buckets/comparative partitions.
- This is scheduling-only. It does not change evidence authority, does not
  decide the answer, and does not use raw user/model prose.

Validation:

- `go test ./internal/types -run 'TestCompileInvestigationPlan|TestCompileExploreLanePlan'`
- `go test ./internal/orchestrator -run 'TestExploreWindowDispatchGroups|TestDispatchExploreWindowsParallel|TestParallelExploreAllowsEarlyConvergence'`
- Focused replay `eval/results/t20-coupled-20260524-230105`:
  - `read_combo_log_current_source_explanation`: PASS, finalizer one turn,
    `finalizer_rejects=0`, `finalizer_rewrites=0`, `explorer_dispatches=1`,
    `explorer_iters=9` (down from 30 in `t20-collective-20260524-223835`).
  - `read_combo_trace_current_source_explanation`: PASS, finalizer one turn,
    `finalizer_rejects=0`, `finalizer_rewrites=0`, `explorer_dispatches=1`,
    `explorer_iters=7` (down from 26).
  - Residual: the log case emitted one non-blocking semantic reviewer concern
    about not tracing the runtime-log event to a full current-source call chain.
    This is an answer-depth / observation-ledger quality follow-up, not a
    scheduling regression.

### 2026-05-24 T11 Rich Summary Handoff Addendum

- Closure prose remains advisory and cannot override typed evidence, support
  refs, current-source citations, or accepted aggregate facts.
- Runtime/log/trace observation-only answers intentionally omit the model
  closure reason from the authority section, because artifact observations are
  not caller-side provenance or current-source proof.
- A fallback now preserves that same accepted closure reason as advisory
  narrative even when a Turn A snapshot is unavailable, so log/trace answers do
  not silently lose the only tool-call-internal synthesis. The fallback does
  not create citations, does not change validators, and does not promote the
  reason above typed observation ledgers.
- Guard test:
  `TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRuntimeClosureReasonWithoutTurnAArtifacts`.

### 2026-05-24 T17/T11 Mixed Runtime Current-Source Answer-Depth Guard

The T20.10 focused replay left one non-blocking semantic reviewer concern in
`read_combo_log_current_source_explanation`: the final answer was factually
useful and one-turn, but it kept some exact current-source anchors only in the
reference list and disclosed that the runtime log was not traced to a complete
current-source call chain. This is not a reason to reopen exploration or force a
rewrite. It is an answer-depth guidance gap:

- external/runtime observations remain origin-specific evidence and must not be
  converted into fake repo citations;
- when current-source evidence has already been read, finalizer should weave the
  exact symbols / functions / config keys / error types / literals into the
  visible explanation instead of leaving them only in bibliography-style
  citations;
- if the external observation cannot be traced to a full current-source call
  chain, the answer should state that boundary while still explaining the
  adjacent mechanism proven by source evidence.

Implementation is prompt-only and localized in the current-source explanation
profile. It does not add a hard gate, does not modify model-authored blocks, and
does not permit system supplements.

## Implementation Notes For Future Batches

- Do not turn `emit_evidence` into a catch-all. It remains current checkout
  source/config/doc evidence only.
- Do not parse raw user/model prose for hard decisions. Visible-coverage checks
  may inspect the already rendered answer surface to decide whether a typed row
  is visibly covered, but not to infer user intent or evidence origin.

## T18 Supplement Safety Fence v2 Design

Root cause from the latest `s5b` replay:

- The model-authored table represented the requested member identity across
  multiple columns (`package` column + `entry function` column + `location`
  column). The deterministic coverage checker looked for a whole member label
  inside a single cell, so `counterfactual -> Expand` was treated as missing
  even though the row was visible as `counterfactual | Expand | ...`.
- The verified-note supplement fired even when the current typed request only
  asked for `name` and `location`. That turned useful evidence summaries into
  a second system-authored explanation table and made the answer look like the
  system had rewritten the model's presentation.

T18 first implementation batch:

1. **Cross-column visible coverage.** For Markdown/model tables, row coverage
   must evaluate the joined data row as well as individual cells. A member can
   be rendered as separate columns such as `type | method`, `package | entry`,
   `route | handler`, `commit | changed path`, or any other language-specific
   pair. This consumes only the visible answer surface and typed row identity;
   it does not infer intent from prose.
2. **Typed-note supplement permission.** Verified note supplements may appear
   only when structured request metadata asks for explanatory content, for
   example `source_inventory_profile.requested_fields` includes `summary` or
   `requested_answer_dimensions` includes `function_or_purpose`,
   `comparison_axis`, or `impact`. If the typed request only asks for names,
   locations, counts, or values, the system must preserve the model answer and
   skip note supplements.
3. **No broad table replacement.** Missing-row supplements remain allowed only
   for rows that are truly absent after cross-column coverage. They must stay
   separate and bounded to the absent rows. Future batches will add stronger
   size/ratio guards if evals show large competing supplement tables.

This design applies across all repomap languages because it works on typed row
identity, evidence origins, and rendered table structure rather than Go-specific
syntax.
- A non-current observation can be principal answer evidence. It just cannot
  create current-source citation pressure.
- If a model table is good but mechanically incomplete, the system may add a
  clearly marked supplemental block. It must not rewrite or replace the table.
- If a non-critical, non-lossless issue remains, prefer localized boundary
  disclosure or accepted-with-advisory telemetry over finalizer rewrite.

- 2026-05-24 T18/T19 P0 red-line batch:
  - Added a structured-absence guard for negative observation supplements.
    When the finalizer has already emitted `exact_resolution.status=absent`
    and the visible answer contains the accepted typed target/scope, the system
    must not append a duplicate `系统按已验证证据补充未命中范围` block merely
    because the prose did not contain the literal scalar `0`. This consumes
    only structured answer fields plus accepted aggregate dimensions; it does
    not inspect the user question or model prose with keyword gates.
  - Added regression coverage for VCS negative observations proving this guard
    does not invent repo citations and does not hide the model-authored answer.
  - Remaining T19 matrix work: collect the existing supplement tests under a
    developer-facing red-line suite, then add extractor/display guards for
    origin-specific evidence and preserved-content rendering.

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
  - B5 focused audit completed with `PARALLEL=2 RUNS=1 TIMEOUT=1500
    bash eval/convergence_audit.sh`. Eight of nine focused cases passed. The
    only failing case was `read_combo_git_two_diffs_current_code`, where the
    model had correct VCS diff/current-source evidence but visible wording did
    not satisfy the case regex. Across the passing cases finalizer converged in
    one iteration with zero finalizer rejects/rewrite renders.
  - New systemic gap from B5: parallel explorer correctness is mostly stable,
    but pre-dispatch focus ownership is weak. `u7k` took `explorer_iters=67`
    and `midloop_inject=24`; logs show several workers independently digging
    through the same scalar-history/current-source chain. B4's
    mixed-origin lane guard prevents premature closure, but it does not assign
    exclusive owners for `(origin, facet, dimension, investigation unit)` before
    fork launch.
  - T11 B6 is now the next systemic fix: add a typed `ExploreLanePlan` /
    `ExploreLane` derived from `InvestigationPlan`, `AnswerIntentContract`,
    `AnswerPresentationContract`, and existing evidence/facet contracts. It
    must not scan user text/model prose, must not decide the answer, and must
    not drop non-owner rich summaries; it only scopes which explorer owns which
    lane and lets exact typed overlap become support/verification/delay rather
    than duplicate principal digging.
  - Full 9-case forensics are recorded in
    `docs/design/explorer_convergence_monotonicity_20260524.md`. The resulting
    priority order is: first fix telemetry false positives (T16), then implement
    typed explore-lane ownership (T11 B6), then close schema-native repair gaps
    for analyzer/source-inventory and `aggregate_facts`, then dedupe repeated
    same-lane evidence-repair hints, and finally reduce generic caveat /
    supplement noise. The failed mixed VCS/current-source case must not be used
    as evidence of finalizer retry: actual finalizer accepted in one turn.
  - T11 B6 partial implementation:
    `types.CompileExploreLanePlan` derives typed lanes from
    `InvestigationPlan`, `AnswerIntentContract`, and
    `AnswerPresentationContract`; `BusContext` / `AgentContext` thread the
    plan to the explorer prompt; parallel dispatch scopes each worker to its
    exact compiler-produced `_tN -> subtopic-(N+1)` lane where available; exact
    duplicate ownership becomes `handoff=support`; and the dock now localizes
    evidence-channel labels such as `证据通道：历史差异、当前源码`. This is soft
    exploration guidance only. It does not rewrite answers, validate final
    output, or parse raw user/model prose. Remaining work is the focused
    convergence rerun.
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
  - 2026-05-24 T13 reference/type-usage slice complete: typed
    `ChangeImpactProfile` with an explicit target can now request
    `references` prompt hints through the shared `BuildTypedRelationQuery`
    selector. The repomap provider consumes `reference/type_usage` graph rows,
    exact endpoint IDs or uniquely resolved target names become precise
    candidates, ambiguous name-only rows stay prompt-only, and MultiGraph
    prefixes child paths through the common relation source. This deliberately
    does not enable a new hard gate or system supplement; future hard use would
    require an explicit bounded relation member-set plus grounded same-member
    evidence.
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

- 2026-05-24 T18/T21 red-line incident closed:
  - Focused eval `s5b-20260524-181246` exposed a contract violation: the model
    repeatedly emitted `aggregate_facts[].role="principal_answer"` for the
    complete `internal/analysis` package-entry member set, but the system
    reported `role="supporting_coverage" is not principal_answer` and kept the
    explorer open. The model correctly complained that the system was
    overwriting its role.
  - Root cause: `required_files` has a prompt/pre-read budget cap. The analyzer
    emitted 25 high-confidence file hints; only the capped subset survived into
    the source-inventory scope list. `NormalizeAggregateFactRolesForRequest`
    then treated members supported by files beyond that capped transport list
    as "outside requested source inventory scope" and demoted a model-authored
    principal member set. This let a budget cap become a semantic hard gate.
  - Architectural rule added: transport / prompt / pre-read caps may constrain
    what is injected into a model prompt, but must never define the legal answer
    scope for a complete, grounded principal handoff. Precise, accepted,
    model-authored `principal_answer` member sets for exhaustive or relation
    handoff contracts outrank source-inventory transport truncation.
  - Code fix: `aggregateFactOutsideRequestedSourceInventoryScope` now refuses to
    demote a grounded principal member set when the typed request requires
    exhaustive or relation member-set handoff. The older source-inventory
    out-of-scope demotion still applies to ordinary inventory candidates that
    are not the principal handoff.
  - Guard tests:
    - `TestPrincipalAggregateMemberSetFactRefsForRequest_ExhaustiveHandoffSurvivesTruncatedSourceInventoryScopes`
      pins the type-level contract.
    - `TestEmitInvestigationComplete_PreCompleteCheck_ExhaustiveHandoffNotDemotedByRequiredFileCap`
      pins the actual explorer closure/pre-complete path.
    - Existing out-of-scope source-inventory and typed-relation tests continue
      to pass, preserving the intended boundary.
  - Residual: rerun `s5b` with the rebuilt binary to confirm the transcript no
    longer contains the role-demotion complaint and to measure remaining
    explorer cost. The separate supplement-safety fix is unit-tested for
    cross-column and structured label/text relation coverage.

- 2026-05-24 T18 supplement safety fence v2 tightened:
  - Architectural correction: enumeration answers are model-authored surfaces,
    not deterministic table-rendering exercises. Models can express a completed
    member set as Markdown table, structured table, ordered list, bullet list,
    multi-column relation table, grouped prose, or a localized hybrid. The
    system cannot enumerate every possible surface grammar without repeatedly
    creating case-by-case bugs.
  - New rule: once exploration has accepted a complete principal `member_set`
    and finalizer has received that context, a model-authored principal
    table/list carrier in the final document suppresses deterministic
    missing-row, field, and note supplements for that set. If the system cannot
    confidently parse every row, it must prefer **no supplement** over a
    competing system table. This intentionally allows a rare model omission to
    remain rather than letting system structure preference overpower user/model
    intent.
  - The rule is structural, not prose-keyword based. It looks only at accepted
    principal aggregate facts, typed enumeration facets/surface roles, and
    answer block kinds. It does not infer user intent from raw request text or
    model散文.
  - Code fix: both deterministic row compiler and legacy aggregate-member-set
    carrier now share this conservative suppressor. The older row-by-row
    supplement behavior remains only for answers with no model-authored
    principal carrier at all.
  - Guard tests updated: partial/corrupt/incompatible authored tables, dry
    authored tables, external-origin tables, and cross-column relation tables
    now assert that no `系统按已验证证据补充...` / `System-verified...`
    competing table is appended. Existing no-carrier supplement tests still
    protect the low-priority fallback path.

- 2026-05-25 B2-J9/T18/T20 regression audit:
  - Focused eval `s5b-20260525-002621` reported PASS, but the answer was not
    commercially acceptable: analyzer spent one round issuing 25
    `repo_map(view="source_inventory")` calls, parallel explorer lanes kept
    investigating the same `current_source` topic, and deterministic
    answer-document normalization moved model-emitted package row citations
    across directories (`aggregator` displayed `hint/composer.go:142`, `gate`
    displayed `aggregator.go:112`).
  - Root cause cluster:
    - source-inventory discovery was allowed to leak into analyze as deep
      navigation even though analyze is classification-only;
    - accepted parallel closures from sibling lanes were still all visible to
      downstream aggregation, producing conflicting 25/47/21 member-set
      obligations;
    - citation repair treated generic symbol/function names as stronger than
      the model's already same-directory package citation.
  - First closure batch:
    - suppress source-inventory discovery hints during analyze;
    - reject `repo_map(view="source_inventory")` during analyze with a precise
      stage-boundary repair hint that asks for `emit_analysis` and leaves row
      verification to explore;
    - in enumeration blocks, keep an already same-directory citation for a
      directory/package label instead of moving it to a different package based
      on generic symbol-citation repair.
  - Remaining high-ROI tasks stay open under T20/B2-K: source-inventory lens
    output should become a verification checklist, and lane ownership must
    suppress conflicting sibling principal member sets before finalizer sees
    them. This is a soft/convergence contract, not a hard semantic override.
