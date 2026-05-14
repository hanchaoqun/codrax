# 2026-05-14 Random Eval System Gap Tracking

Status: active

This document tracks every issue and systemic gap discovered during the
2026-05-14 randomized full eval sweep and adjacent historical-result audit.
It is intentionally a running ledger. Final clustering and implementation
decisions happen only after the sweep completes.

Hard rule for fixes: preserve the repository principle that hard gates consume
precise typed signals, while noisy signals remain soft guidance. Do not land
case-specific keyword patches when a typed contract or deterministic
canonicalization layer is the real boundary.

## Sweep Snapshot

- Workspace: `/Users/han/opt/codrax`
- Sweep command owner: existing `bash eval/parallel_all.sh` process
- Sweep start: `2026-05-14T04:18:52Z`
- Runner snapshot: `./.codrax-sweep-20260514-121852`
- Cases: 66 top-level `eval/cases/*.case`
- Parallelism: 4
- Randomized order: true, seed `1778732332`
- Timeout: 1800s per case
- Note: default `CASES_GLOB` excludes `eval/cases/harmony/*.case`

## Local Working Tree At Start

- Branch: `main...origin/main`
- Pre-existing modified files:
  - `docs/download/index.html`
  - `docs/user_guide.html`
- Pre-existing untracked files:
  - `AGENTS.md`
  - `providers.yaml.local`

No code changes from this investigation had been made when this ledger was
created.

## Active Issue Ledger

| ID | Case / Source | Status | Symptom | Systemic Gap | Generalized Fix Direction |
| --- | --- | --- | --- | --- | --- |
| E20260514-G1 | `s11b` | Fixed Batch 2e / PASS replay | Answer names `EmitStageRetryAttempt` but the user asked for the retry budget parameter, expected `MaxRetriesPerStage`. | Role-disambiguated scalar answers were not enforced. The pipeline collected both the retry cap and attempt counter, but final selection did not bind the requested noun role to the cited identifier. | Added a required typed `answer_role_profile` carrier for positive scalar/mechanism role bindings, propagated it through `AnswerSemanticView`, and hard-gated final rows by enum-to-enum comparison against `items[].candidate_role`. |
| E20260514-G2 | `s1b` | Fixed Batch 2g / PASS replay | Final answer explains the validation requeue path but omits user anchor `runTaskGraph` and does not preserve an accepted upstream/evidence-node surface term. | Exact mechanism anchors can vanish between evidence and final answer when they remain only retrieval hints or prose instructions. Classic scalar/enumeration lanes do not cover mechanism-entrypoint preservation. | Added a typed `RequiredMechanismAnchors` answer-view carrier compiled from analyzer typed lanes and kind-bearing contract terms. Finalizer and pre-emit checks consume only structured AnswerDocument fields (`items[].label`, block titles, diagram edge endpoints), never `RawRequest` or rendered prose scans. |
| E20260514-G3 | prior `s5a` random sweep | Confirmed product failure | Explorer found all `LoopController` implementers, but finalizer exhausted retries and emitted raw LLM text because aggregate member labels/citations could not satisfy conflicting checks. | Aggregate member sets are not canonicalized into a single render contract. Equivalent forms such as `Type (file:line)` and `Type@file:line` survive as separate obligations, while citation alignment expects a different principal label surface. | Build deterministic `MemberDisplayRow` records from accepted aggregate facts before finalization: stable member key, visible label, source location, citation handle, claim form, and display candidates. Coverage and citation checks must consume the same rows. |
| E20260514-G4 | `u11b` | PASS with 50 rejects | The run found the correct 4 production `CitationReq.Required=false` sites, but finalizer looped over source-location labels with qualifiers before passing. | Source-location member rows with qualifiers can be treated simultaneously as labels, source refs, and prose, creating retry loops even when the final answer is correct. | Reuse the G3 canonical member-row layer for source-location sets and qualifiers. Principal source rows should cite stable support handles instead of requiring the model to reproduce exact `file:line (qualifier)` strings. |
| E20260514-G5 | `m1a` | Fixed Batch 2h / focused PASS | Explorer struggles to anchor literal tool names such as `emit_answer_document called` and `case "emit_answer_symbol"` as evidence. | Evidence anchoring was symbol-centric for facts whose source truth is a string literal, switch case, or method-return literal. Exact literals could be forced through symbol-definition machinery and then collide with carrier-visibility rules. | Added typed source-literal evidence: `anchor_kind=string_literal` compiles to `claim_form=literal_value_fact`, grounds only against parsed source-code literal spans on cited lines, and is consumable by exact scalar/literal answer lanes. Comments, identifiers, prose, and rendered/user text are not hard-gate inputs. |
| E20260514-G6 | prior `qf_sequence_analyzer_gate` random sweep | Confirmed harness gap | Output contains the expected mermaid sequence and symbols, but eval verdict reports a regex miss. | Eval harness regex expectations are brittle: shell word-splitting and escaped-dot semantics can mark a correct answer as failed. | Move eval expectations to structured arrays or a small parser with tests for multiline regexes, escaped dots, fences, and alternations. Harness failures should be separable from product failures. |
| E20260514-G7 | prior `patch_go_typo`; current `logtri_java` / `mr_keyword` | Confirmed infrastructure gap | Prior write-mode plan failed after transient provider/network errors; current read/log cases pass only after EOF/stall retries and degraded continuation. | LLM transport/provider failures can consume semantic budgets, inflate runtime, and sometimes appear as product failures. Stage EOF/stall handling is not separated clearly enough from semantic insufficiency. | Add provider fallback/backoff classification for stage transport errors, classify infra failures distinctly in eval, and preserve deterministic tool/evidence state across transport retries. For micro-scope typo patches, design a deterministic rescue path only when model-emitted analysis has exact file/line/symbol evidence and write mode is explicitly enabled. |
| E20260514-G8 | `m1a` | PASS with 33 rejects | Finalizer retry tail shows conflict between explicitly listing `emit_answer_document` / `emit_answer_document_patch` and a validator/prompt rule that removes internal `AnswerDocument` carrier names from user-visible prose. | Internal-carrier concealment is too coarse when the user explicitly asks for tool names whose literal names overlap carrier terminology. The finalizer cannot both hide the term family and list the requested `emit_*` tools without trial-and-error. | Split user-requested tool literals from internal schema carrier names. Use typed visibility policy: exact user-requested tool/function names are allowed when grounded; internal carrier types remain hidden unless the user asks for implementation schemas. |
| E20260514-G9 | `u3a` | PASS with 3 rejects | Explorer could not cleanly ground conditions such as `if e.investigationComplete {` and had to use assignment/absence workarounds. | Condition-anchor validation handles calls and richer expressions better than boolean field guards. This creates avoidable retries for common guard-style mechanisms. | Add precise support for field-access boolean guards in condition evidence, e.g. `condition_subject=field_access` plus exact line text and owner field identity. The hard signal is the parsed/verified field access, not a keyword search. |
| E20260514-G10 | `u8b` | TIMEOUT | Exhaustive enumeration oscillates between 91 exported `type X string` declarations, 81-82 partially materialized members, and 93 extractor items with duplicates. | Large closed sets lack both a scalable deterministic aggregate handoff and a typed candidate-vs-qualified-member boundary. The LLM is forced to manually filter, count, de-duplicate, and materialize dozens of mechanically discoverable rows. | Add a deterministic typed enumeration carrier for repo-map/AST query results: candidate rows, qualification predicate (`has_const_set`, exported, package scope), per-member file:line support handles, excluded-candidate reasons, duplicate keys, and compact canonical aggregate. Finalizer consumes the qualified set, not raw grep candidates. |
| E20260514-G11 | `logtri_java`, `logtri_goroutine_dump`, `logtri_node` | PASS with rejects | Finalizer retry feedback says user-visible prose contains internal field name `citation_ref`, and external-runtime cases repeat repair loops around artifact-only citation / path visibility before passing. | Internal carrier-field visibility validation may conflate AnswerDocument JSON fields with rendered user text, or allow hidden carrier notation to leak through model thought into repair prompts, causing confusing loops. | Make carrier-name concealment operate on the rendered answer surface only, while schema/JSON field names remain invisible implementation structure. Error feedback should point to the exact rendered block/item text that contains the forbidden token. |
| E20260514-G12 | `u7a`, `s7a` | TIMEOUT | Deterministic scalar questions either found the exact value early (`u7a` commit hash/subject) or only needed a local count command (`s7a`), but still timed out in LLM-driven later stages. | Tool-sourced scalar answers lack a deterministic terminal render path. Once the value is known from VCS/shell, the pipeline still depends on LLM extract/finalize loops. | Add a typed `deterministic_scalar_result` handoff for VCS/history/count/measurement facts with direct render support and focused verification, while preserving citation/command provenance. |
| E20260514-G13 | `logtri_rust`, `logtri_goroutine_dump`, `logtri_node` | TIMEOUT / slow or noisy PASS | External-only runtime logs are self-contained, but the Rust case timed out, the goroutine dump needed 1240s plus evidence repairs, and the Node case passed only after stripping artifact path details under validator pressure. | External runtime artifacts with `resolved_files=0` are not hard-routed strongly enough to artifact-only answer flow. The model can still spend budget looking for nonexistent repo files, trying to ground artifact-only frames against current code, or fighting contradictory path-visibility hints. | For `external_only_log|trace` with sufficient triage facts, short-circuit exploration to an artifact-only structured answer. Current-repo verification should be an explicit caveat, not a required investigation lane, and artifact frame paths should have a clear rendered-surface policy. |
| E20260514-G14 | `qf_type_relation_loop_controller` | TIMEOUT | LoopController class/type diagram timed out after an analyzer stream stall and partial implementer discovery. | Diagram relation questions combine several existing hard parts: set-valued implementer enumeration, aggregate member canonicalization, and diagram block rendering. Transport stalls can consume the budget before the typed relation set is stable. | Reuse canonical aggregate relation rows for interface->implementer sets, then render Mermaid from those rows deterministically or with a small constrained finalizer surface. |
| E20260514-G15 | `u8b`, `u7a`, `logtri_rust`, `qf_type_relation_loop_controller`, `s7a` | Fixed Batch 5a / runner test | Timeout summary records elapsed times of 1800-2019s and briefly showed process accounting out of line with `parallel=4`. | Timeout enforcement and accounting were not crisp enough for commercial eval operations; long provider stalls could overshoot budgets and make parallel-slot health hard to reason about. | Made eval timeout execution use a single process-group runner: start command in a fresh session, TERM the whole group at wall-time, KILL after grace, and test that background grandchildren cannot survive timeout. |
| E20260514-G16 | `u9b` | Fixed Batch 5b / PASS replay | Final answer correctly said the whole call does not fail, only the bad entry is rejected, and the rest continue successfully, but the eval initially failed because the answer had no canonical per-item verdict token. | Either/or error-granularity answers lacked a typed verdict surface consumed by both finalizer and eval. The product could answer semantically while omitting the canonical granularity token, and the harness could miss a valid synonym. | Added a typed `error_granularity_profile` analyzer lane and typed `error_granularity_verdict` decision-block surface. Downstream prompts, pre-emit checks, contract checks, renderer, and eval now consume the typed enum instead of prose synonyms. |
| E20260514-G17 | `qf_relation_subagent_registry` | PASS with 11 rejects, 1450s | The answer set is exactly one member (`explorer`), but the run nearly timed out while adding a third citation, summary/caveat blocks, call-site anchors, and fixing numeric line references being misread as count claims. | Small deterministic member-set / registry questions are over-scaffolded. Citation floors and prose validators are not adapted to cardinality or to a canonical member row with separate registration evidence and Name() evidence. | Build deterministic relation/member rows with fields for member label, registration call, name-return literal, entrypoint, and count. For closed sets with small cardinality, finalizer should render from rows and citation floor should derive from row obligations instead of a generic `citation_count_ge=3`. |
| E20260514-G18 | `u1a` | PASS with 10 rejects, 1547s | Security call-chain answer gathered source/sink/defenses but finalizer spent many iterations rebuilding 20+ citation refs and still produced at least one suspicious citation drift (`verifyResourceCaps / wrapShellCommandWithCaps` item cites `shellOperatorWrites`). | Mechanism/call-chain answers over long defense paths lack a deterministic chain row / citation-index compiler. The model manually maintains citation arrays, claim forms, and inline code anchors, so valid evidence turns into index bookkeeping. | Build typed `MechanismStepRow` / `SecurityFlowRow` records for taint source, guards, transformations, and sink. Compile citations and block items deterministically from rows; finalizer should write prose on top of stable row ids rather than hand-numbering citation refs. |
| E20260514-G40 | focused `s5b` after Batch 1h | Confirmed FAIL | Analyzer no longer fabricated a numeric `enumeration_boundary`, but it also omitted `completeness_obligation`; explorer/finalizer then treated a 25-member category enumeration as ordinary lower-bound enumeration and shipped 17 rows plus a caveat. | A typed principal member lane from exploration was available (`intent=enumerate`, `question_kind=enumeration`, `is_category_enumeration=true`, 25 analyzer entities), but only explicit count/completeness signals forced structured `member_set` handoff. This let upstream rich member information become soft search context instead of a downstream contract. | Treat non-relational typed category-enumeration entity lanes as principal member lanes when they contain multiple members and the analyzer intent/kind is enumerate. Require structured `member_set` handoff and allow lowercase package/module/file-stem members through the same typed lane across supported languages. |
| E20260514-G41 | focused `s5b` analyzer trace | Confirmed red-line drift | StageAnalyze used line-level `grep(files_only=false)` and pulled function signatures plus a noisy "26 packages" count into analyzer thinking, despite the evidence-lite runtime boundary requiring files-only pre-scan. | Prompt/runtime contract drift: old ClassificationGrep Round-2 carve-out let content evidence leak into classification, creating hard/soft signal inversion. The analyzer saw source-line noise before exploration, then downstream gates had to fight that noise. | Restore the evidence-lite boundary in both prompt and runtime: analyze may use `repo_map`, `list_files`, and `grep(files_only=true)` only; source-line proof belongs to explore. Keep legacy config fields inert for compatibility, but never admit line-level grep in StageAnalyze. |
| E20260514-G42 | focused `s5b` post-Batch 1i replay | PASS with repairs | The strict `member_set` handoff forced complete coverage and ultimately passed, but the first completion downgrade exposed a repair-cost spike: one missing `findings_validator → Validate` support row caused a second explore dispatch; the model then tried to re-emit a giant evidence slate with invalid `surface_terms` before succeeding via member-specific `@ file:line` rows. | The pipeline now has the right hard gate, but repair consumption is still model-heavy. Accepted evidence/support rows are not compiled into a deterministic per-member support table early enough, and stale ungrounded evidence cannot be superseded cleanly, so the model pays extra turns reconstructing support_refs. | Add a deterministic `MemberSupportRow` compiler from accepted typed evidence, read_file gutters, and aggregate support_refs. Completion repair should name only missing members and candidate support locations; finalization should consume stable rows instead of relying on the model to rebuild 25 support refs by hand. |
| E20260514-G43 | focused `s5b` post-Batch 1j replay | Confirmed FAIL / stopped loop | The completion support gap was fixed, but finalizer entered a repeated pre-emit rejection loop because the model-authored member `perftriage → MergePerfBundles + CorroborateStallFiles` was displayed as two cited rows. The validator required the exact composite member string while citation alignment preferred split rows. | Principal member-set coverage and structured relation-shape checks did not share a generalized display-equivalence rule for "same left-axis, multiple explicit right-side symbols." This recreated the G3/G4 row-grain conflict at the final answer boundary. | Treat composite relation members such as `pkg → A + B` / `pkg: A 和 B` as precise multi-target relation rows. A structured list may satisfy them by rendering one row per target only when every row has the same left axis; different left-axis rows must not satisfy the composite member. |
| E20260514-G44 | focused `s5b` post-Batch 1j pass | Residual PASS cost | Self-consistency reviewer falsely claimed the ordered package list was not alphabetic, triggering an unnecessary rewrite even though the sequence was already `aggregator, amplifier, axis, ...`. | Semantic review can turn a noisy natural-language judgment into an expensive rewrite despite structurally valid typed rows. This is a soft reviewer acting like a hard gate. | Teach the reviewer to consume deterministic ordered-list metadata or downgrade ordering disputes to advisory when the structured row set is already accepted. For sortedness, use a deterministic comparator over visible labels instead of model prose. |
| E20260514-G45 | audit of commits `d2289e7a` / `e07cafb5` | Red-line remediation | The attempted G44/G45 fixes introduced hard decisions driven by keyword matching over reviewer prose, user request prose, and final answer prose. | This violated the repository rule that hard gates consume typed, precise signals only. It also violated the stronger operational rule that user/model text must not be keyword-matched to decide logic. | Reverted the attempted gates and deleted their tests first. Follow-up Batches 6b/6c replaced the product intent with typed lanes: `contradiction_kind` + deterministic row order profile for reviewer ordering, and `answer_exclusion_policy` + answer-row `candidate_role` for excluded categories. |
| E20260514-G46 | audit of commit `af8f5a9c` | Red-line remediation / fixed Batch 6a | Field/value count coverage was triggered by scanning `RawRequest` / analyzer keywords for dotted fields and literal words such as `false`, `true`, `nil`, `null`, or `undefined`. | The feature goal was valid, but the hard pre-complete downgrade inferred its target/literal from user prose and keyword lists instead of a typed analyzer lane. This made the gate language-fragile and risked unrelated-count false positives. | Added analyzer `field_value_profile` (`target`, `owner`, `field`, `literal`, `literal_kind`, `source_quote`, confidence) and moved explorer/pre-complete consumption to that typed carrier. Downstream hard gates no longer infer field/value coverage from RawRequest or Keywords; exact `source_quote` validation is confined to the analyzer emit boundary. |
| E20260514-G47 | audit of commit `d2289e7a` | Red-line remediation / fixed Batch 6b | The attempted sorted-row self-consistency filter looked for ordering words in reviewer contradiction prose and reasoning, then suppressed a hard rewrite when rows appeared sorted. | This still let model-authored reviewer text drive hard control flow. The valid system need is to stop row-order false positives, but the control signal must be a typed review kind plus deterministic AnswerDocument row metadata. | Added `contradiction_kind` to `emit_self_consistency_review` and a deterministic V2 principal-row order profile. Only `row_order_mismatch` contradictions can be suppressed, and only when every principal list/table row block is deterministically ascending by item label or citation path axis. Unknown/text-only reviewer claims are not suppressed. |
| E20260514-G48 | audit of commit `e07cafb5` | Red-line remediation / fixed Batch 6c | The attempted user-excluded-category gate detected excluded categories by matching words in the user request and detected leaks by matching words/tokens in final answer prose. | Exclusion is a real answer-scope contract, but request/answer text keyword scans are not acceptable hard-gate inputs. The policy and row category must travel as typed carrier fields. | Added analyzer `answer_exclusion_policy` and answer-row `items[].candidate_role`. The hard check is now enum-to-enum: if a principal answer item carries a candidate role excluded by the analyzer policy, it is rejected. Scope-boundary prose is not scanned or rejected. |
| E20260514-G49 | Batch 2c red-line audit of G8/G11 pre-emit visibility | Fixed Batch 2c | `preCheckVisibleInternalCarrierTerms` scanned rendered answer prose for carrier-name tokens and scanned `RawRequest` to decide whether those tokens were allowed; adjacent artifact/multi-repo checks also scanned rendered prose for repo/path names. | This reintroduced the same red-line class at the final-answer boundary: a hard control decision depended on keyword-like matching over model output and user text, and it could fight explicit tool-name literal answers. | Removed rendered-answer/RawRequest keyword gates from pre-emit visibility checks. Preserved structural hard gates where the signal is typed (`citations[]`, negative-scope citations, `exact_resolution`). Expanded answer-row `candidate_role` into scalar/literal roles (`tool_name`, `config_key`, `route`, `import_path`, `literal_value`, `commit_hash`, `budget_cap`, `attempt_counter`, `guard_condition`) so future role binding consumes typed row metadata instead of prose. |
| E20260514-G50 | focused `m1b` after Batch 2c | Fixed Batch 2d / PASS replay | The case passed, but extractor soft-stop forced `emit_answer_symbol` for two exact tool-name sub-topics. The model then tried to cite a string-literal/reference line as a symbol definition and needed repairs before finalization. | Multi-topic anchor skeleton activation treated all architecture sub-topics as symbol anchors, even when the typed request said the principal answer was scalar/literal. This pulled exact literal answers back into symbol-definition machinery and recreated Batch 2's seesaw. | Gate anchor skeletons off when `RequestModel.Predicates.IsScalarAnswer` or scalar-source-literal lookup is active. Multi-topic exact literals now stay in scalar/section/table rows with `candidate_role` metadata instead of `emit_answer_symbol` definition slates. |
| E20260514-G51 | focused `s11b` Batch 2e carrier audit | Fixed Batch 2e / PASS replay | Early Batch 2e replays passed but the analyzer omitted the optional positive role carrier, so the final answer was still relying on prompt guidance rather than a hard structural lane. | Optional carriers are easy to drop under prescan/retry pressure. Exact scalar role questions need an always-present typed object, active only when the analyzer sets `is_role_binding_requested=true`, so downstream stages can consume a uniform contract without reading request prose or answer prose. | Made `answer_role_profile` a top-level required `emit_analysis` object; when active it requires enum roles plus verbatim source quotes at the analyzer boundary. Finalizer prompts, pre-emit checks, and post-emit contract checks now consume only `RequiredCandidateRoles` and `items[].candidate_role`. |
| E20260514-G52 | Batch 5a eval runner timeout audit | Fixed Batch 5a / runner test | The parallel eval launcher delegated timeout behavior to whichever `timeout` binary happened to exist on the host, and stale grandchildren could keep consuming resources after the worker shell returned. | Timeout/process cleanup is an eval infrastructure lane, not a product prompt issue. It needs one deterministic process-tree contract across macOS/Linux so PASS/FAIL/timeout summaries reflect the actual active workload. | `eval_run_with_timeout` now prefers the Python process-group runner on every host with Python 3, falls back to `timeout -k 10` only when Python is unavailable, and has a regression test proving a background grandchild is killed before it can write a marker file. |
| E20260514-G53 | `u9b` post-Batch 5b replay | Fixed Batch 5c / PASS replay | The typed verdict replay passed, but the same request still triggered two rejected `enumeration_boundary` emits on the phrase "exactly one item", then ran with `family=enumeration`, `enumeration_push=1`, and `explorer_iters=40`. | Scenario counts that describe a failure condition can leak into answer-set cardinality and enumeration-family planning. The typed error-granularity lane solved the answer verdict, but a neighboring count/enumeration lane still treats contextual quantities as principal answer-set obligations. | Added a typed lane conflict resolver: when `error_granularity_profile` is active and the analyzer does not also emit a count/category/relation answer predicate, scenario counts remain contextual parameters, not `enumeration_boundary` contracts or enumeration facet families. |
| E20260514-G54 | `u9b` Batch 5c replay | Fixed Batch 5c / support-lane test | After G53 was fixed, the finalizer emitted the required principal `decision` block with `error_granularity_verdict`, but principal evidence support rejected `decision` because the lane allowed only summary/section/list/diagram blocks. | Typed answer-block requirements and support-lane allowed-block policies were compiled independently. Adding a typed decision verdict changed the required answer surface, but citation-routing policy still assumed generic principal evidence never needs a decision block. | Made principal evidence support policy consume the typed error-granularity profile: active failure-scope verdict questions allow principal `decision` blocks to cite principal evidence, while the default block policy remains unchanged for other generic answers. |
| E20260514-G55 | `u9b` post-rebase replay | Fixed Batch 5c / classifier-variant guard | After rebasing onto the remote finalizer recovery work, the same request sometimes classified as `intent=root_cause`; root-cause family priority then preempted the failure-scope decision guard and rebuilt a three-surface diagnostic answer. | The typed lane conflict resolver was present but ordered below a broader diagnostic family. A classifier wording variant could reopen the over-scaffolding seesaw even though the same typed `error_granularity_profile` was active. | Promoted no-attachment failure-scope decision answers ahead of root-cause scenario/family routing. Attached log/perf traces still use the diagnostic family because artifact/current-code drift remains part of their answer surface. |
| E20260514-G56 | G12 / `s7a` deterministic count audit | Fixed Batch 2f / PASS replay | Count questions could close once a deterministic `exec_command` produced a parseable `count=N`, but that value was only a completion permission. The finalizer still had to recover the scalar from prompt/tool history, and two conflicting deterministic count outputs were not treated as an ambiguous structured handoff. | Tool-sourced scalar proof and final-answer scalar obligation were split. The same raw tool output acted as a soft memory source for rendering and a hard gate for completion, without a typed aggregate carrier or an ambiguity rule. | Compile unambiguous deterministic count tool output into an `AnswerAggregateScalar` with `answer_axis=count` and `proof_source=exec_command`; if multiple deterministic count values conflict, fail closed and require a structured handoff. History lookups remain excluded so commit/history scalars still need typed history rows rather than a raw command shortcut. |

## End-to-End Traces

### E20260514-G1: Retry Budget Role Drift (`s11b`)

User request asks whether analyze retries when `emit_analysis` is absent and
what the retry budget parameter is called.

Observed data flow:

1. Analyzer/explorer found the retry mechanism and the adjacent attempt counter.
2. Evidence included `internal/orchestrator/orchestrator.go` where
   `dynamicAnalyzeRetries(o.settings.MaxRetriesPerStage)` computes the cap.
3. Evidence also included `EmitStageRetryAttempt`, the per-attempt counter
   propagated into `AgentContext` and used to alter tool choice on retry.
4. Final answer chose the counter-like identifier for the requested budget
   noun.
5. Eval failed on missing `MaxRetriesPerStage`.

Root cause: the handoff had nearby valid identifiers but no typed role contract
forcing "budget cap" to win over "attempt counter".

Generalization: any scalar mechanism question asking "what parameter/field
controls X" can drift to a neighboring implementation variable unless answer
candidate role is explicit.

### E20260514-G2: Mechanism Anchor Loss (`s1b`)

User request asks about `runTaskGraph`, validation failure handling, and whether
the whole window is re-expanded.

Observed data flow:

1. Analyzer included `runTaskGraph`, `requeueValidationTargets`,
   `EdgeValidationFeedback`, and scheduler state in the task entities.
2. Evidence found `runTaskGraph` dispatching into the read scheduler and
   validation failures flowing through `EdgeValidationFeedback` into selective
   upstream evidence-node requeue.
3. Final answer described the selective requeue mechanism but rendered the
   entrypoint as `runReadSchedulerLoop` and did not preserve enough of the
   upstream/evidence-node language expected by the case.
4. Eval failed on missing `runTaskGraph` and the upstream/evidence-node regex.

Root cause: exact user anchors are used during retrieval but are not always a
hard visible-answer contract for mechanism explanations.

Generalization: for "explain how X does Y" and "does X re-run Z" questions,
named endpoints are part of the answer surface, not only search hints.

Batch 2g progress:

- Added `AnswerSemanticView.RequiredMechanismAnchors` as a typed final-answer
  carrier for mechanism-style explanations. It is compiled from analyzer typed
  lanes (`MentionedEntities`, `ExactTargets`) plus kind-bearing
  `MustIncludeTerms`, and it only admits code/tool/file-stem anchors rather
  than free-form user phrases.
- Finalizer prompts now render a "Typed Mechanism Anchor Contract" telling the
  model to satisfy required endpoints with structured carrier fields:
  `blocks[].items[].label`, block titles, or diagram `edge_anchors`
  endpoints.
- Pre-emit validation checks only structured `AnswerDocumentV2` fields. Summary
  text and rendered prose are intentionally ignored, so this does not become a
  keyword gate over the user request or model answer.
- Seesaw guard: the carrier is disabled for scalar, count, category
  enumeration, relational lookup, config query, and return-value paths because
  those already have stronger typed principal lanes. The anchor list is capped
  to keep mechanism prompts focused.
- First `s1b` replay after the initial change passed
  (`eval/results/s1b-20260515-051031`) but logged
  `required_mechanism_anchors=0`, proving the result still relied on the old
  path. The compiler was then broadened to consume the direct
  `MentionedEntities` typed lane for mechanism families instead of relying only
  on must-include terms.
- Second `s1b` replay passed (`eval/results/s1b-20260515-051535`) with
  `required_mechanism_anchors=1` in semantic-view traces. The finalizer also
  repaired one fabricated structured label (`pendingValidationTargets`) to a
  grounded function anchor (`renderWindowHint`), showing the new structured
  anchor lane composes with the existing grounded-label checks.
- Residual follow-up: this replay still needed a large exploration window
  (`explorer_iters=20`, `midloop_inject=9`). That is an exploration-efficiency
  issue for future mechanism-row work, not a blocker for the G2 answer-surface
  preservation fix.

### E20260514-G3: Aggregate Member Canonicalization Loop (`s5a`, current sweep PASS with 6 rejects)

User request asks for all concrete types implementing `LoopController`.

Observed data flow:

1. Explorer discovered the complete set of implementers.
2. One aggregate fact used display members like `Type (file:line)`.
3. A retry emitted compact support refs like `Type@file:line`.
4. Stable aggregate facts retained both member sets.
5. Finalizer prompt exposed both display labels and source-location surfaces.
6. Pre-emit coverage demanded every emitted member form, while citation
   alignment demanded label/citation pairings that treated the same text under
   a different surface role.
7. The finalizer exhausted its iteration budget and fell back to raw LLM output.
8. In the current 2026-05-14 sweep, the same case passes in 285s but still
   needs 14 file reads, 6 mid-loop hint injections, 3 finalizer iterations, 6
   rejects, and a semantic-quality pass. The user-visible failure is gone, but
   the aggregate-row instability is still present.

Root cause: equivalent member identities were not collapsed into one typed
principal row before final answer generation.

Generalization: all exhaustive set answers with source refs can hit the same
loop: implementers, call sites, config sites, enum members, route handlers,
package entries, and production code locations.

### E20260514-G4: Source-Location Member Rows With Qualifiers (`u11b`)

User request asks for the count and exact file/line locations where production
code sets `CitationReq.Required` to `false`.

Observed data flow:

1. Analyzer pre-scan found production files containing direct assignment and
   struct-literal initializer forms.
2. Explorer confirmed 4 production sites:
   `internal/agent/analyzer.go:1927`,
   `internal/orchestrator/contract_check.go:63`,
   `internal/orchestrator/orchestrator.go:6425`, and
   `internal/orchestrator/orchestrator.go:6557`.
3. Explorer correctly excluded `internal/tool/emit_evidence.go:236` because it
   is a documentation/example string, not an assignment site.
4. Extractor/finalizer received a member set whose members combined source
   locations with qualifiers, for example `file:line (赋值语句, conditional)`.
5. Finalizer repeatedly alternated between using the full qualified member as
   the item label and using only the bare source location, because coverage
   wanted the full member string while citation alignment wanted the exact
   source-location target.
6. The case eventually passed, but the run recorded 50 rejects.

Root cause: the principal member identity, visible label, qualifier text, and
citation target are not separated before finalization.

Generalization: source-location inventories with descriptive qualifiers should
not require the model to discover which substring belongs in the label versus
the prose. A deterministic row contract should carry these fields separately.

### E20260514-G5: Literal Tool-Name Evidence Anchors (`m1a`)

User request asks how explorer and extractor collaborate and asks to list the
Turn A / Turn B `emit_*` tools.

Observed data flow so far:

1. Explorer needs to prove tool names that appear as string literals, switch
   case values, or `Name()` return literals.
2. `emit_evidence` feedback appears to push the model toward symbol-like
   anchors such as `case`, `return`, or loop-control symbols rather than the
   literal string itself.
3. The run spends extra turns repairing evidence that is conceptually precise
   but represented in a non-symbol source form.

Root cause: literal identifiers in source text do not have a first-class
evidence anchor kind distinct from code symbols.

Generalization: CLI flags, tool names, provider IDs, enum string values,
config keys, route patterns, log markers, and protocol message names all need
precise literal grounding.

Related recurrence: `m1b` asks only for two `emit_*` tool names:
analyzer -> `emit_analysis`, finalizer -> `emit_answer_document`. Analyzer
initially misclassifies this two-row lookup as scalar, then retries. Later
finalizer repairs cite the `Name()` return literals but still oscillate around
whether the lowercase tool string is a permitted user-visible literal or an
internal carrier-name leak. This reinforces that tool-name literals need their
own typed evidence and visibility lane. The current sweep case ultimately
passes, but only after 843s, 12 file reads, 29 mid-loop injections, 33
finalizer iterations, and 88 rejects.

Batch 2h progress:

- Added a first-class source literal evidence lane:
  `AnchorStringLiteral` -> `ClaimLiteralValueFact`. The analyzer/explorer can
  now represent a source-code literal value as the principal fact instead of
  coercing it into a definition, call, assignment, or free-text reference.
- The grounder validates `string_literal` anchors by parsing literal spans from
  the cited source line and comparing the evidence anchor against those spans.
  It accepts quoted/raw/template/char-style literals used by Go, Python,
  JavaScript, TypeScript/ArkTS, Java, Kotlin, Rust, C/C++, Swift, Ruby, Lua,
  Proto, Cangjie, and similar line-oriented syntaxes, including route/config
  punctuation. It accepts only exact literal content or exact quoted literal
  surface; it rejects ordinary identifiers, partial-literal substrings,
  comment-only mentions, and inline-comment-only mentions.
- Exact scalar/list/table lanes can now consume `literal_value_fact` as a
  display-label claim form for enumeration, role lookup, and config precedence
  facets. This keeps tool names, route strings, config keys, enum string
  values, provider IDs, and protocol literal names out of symbol-definition
  repair loops.
- Anti-seesaw boundary: `text_reference_fact` remains for docs/prose-style
  references; `literal_value_fact` is source-code literal evidence only; symbol
  definitions/calls/assignments keep their existing typed lanes. Hard decisions
  consume structured evidence fields and source-code line spans only. They do
  not scan `RawRequest`, rendered answer prose, model reviewer text, or
  keyword frequency.

### E20260514-G6: Eval Harness Regex Brittleness (`qf_sequence_analyzer_gate`)

Historical random sweep case expected a mermaid sequence answer containing
symbols such as `normalizer.Normalize`, `compiler.Compile`, or
`binder.BindByRelevance`.

Observed data flow:

1. The case output contained a `mermaid` / `sequenceDiagram` block and named
   the expected analyzer-gate functions.
2. The eval verdict still reported a regex miss.
3. The case's `EXPECT_MATCHES_REGEX` used escaped dots and multiline
   alternation inside shell variables.
4. `eval/run.sh` splits regex expectations through shell word/newline handling,
   making the intended pattern sensitive to quoting and escaping.

Root cause: the harness encodes structured expectations as shell strings,
where multiline regexes and escaped source-code identifiers are brittle.

Generalization: harness false negatives can mask product regressions or waste
engineering time. Eval expectations need their own structured representation
and tests, especially for diagrams, code identifiers, paths, and multiline
answer surfaces.

### E20260514-G7: Transport / Provider Resilience (`patch_go_typo`, `logtri_java`, `mr_keyword`)

Multiple cases show transport failures as a distinct system dimension.

Observed data flow:

1. Historical `patch_go_typo` found the exact typo, but planner failed after
   provider/network EOF and connection errors, producing no plan file.
2. Current `patch_go_typo` passed, showing the write logic itself is sound when
   the provider path is healthy.
3. Current `logtri_java` and `mr_keyword` both encountered EOF/stall failures
   during later stages, then recovered or continued from already collected
   evidence and eventually passed.
4. The summary still records the cases as PASS, but their wall time and retry
   paths are much higher than a healthy run.

Root cause: transport failure, semantic insufficiency, and retry repair are not
cleanly separated in reporting and stage budgeting.

Generalization: commercial-grade behavior needs stable recovery from LLM stream
stalls, provider EOFs, and transient connection errors without hiding them as
semantic model failures or treating them as ordinary answer repair loops.

### E20260514-G8: Explicit Tool Names vs Carrier Concealment (`m1a`)

User request explicitly asks to list the Turn A and Turn B `emit_*` tools.

Observed data flow so far:

1. Explorer collects grounded evidence for tool implementations and tool names,
   including finalizer tools whose names contain `answer_document`.
2. Finalizer tries to answer with `emit_answer_document` and
   `emit_answer_document_patch` as requested tool members.
3. Retry feedback repeatedly asks the model to remove internal
   `AnswerDocument` carrier terminology from user-visible prose.
4. The model oscillates between hiding the literal tool names and satisfying
   the requested member-set coverage for those same tools.

Root cause: the visibility policy appears to match the carrier term family too
broadly and does not distinguish "grounded user-requested tool literal" from
"internal schema carrier type".

Generalization: any user question about internal tools, APIs, structs, or
protocol names can legitimately require exposing names that overlap otherwise
hidden implementation carriers.

Related recurrence: `m1b` is a smaller version of the same failure mode. The
user explicitly asks for `emit_answer_document`; validator feedback repeatedly
requests removal of `AnswerDocument` carrier names, while citation alignment
alternates between the `EmitAnswerDocument` type definition and the `Name()`
method line that returns the requested tool string. The correct fix is not more
prompt wording; it is a typed distinction between `tool_name_literal` and
`carrier_type_name`. This recurrence passes only after 88 rejects, confirming
the rule conflict is structural rather than a one-off model stumble.

### E20260514-G9: Boolean Field Condition Anchors (`u3a`)

User request compares `ShouldStop` behavior in `explorer.go` and
`extractor.go`.

Observed data flow so far:

1. Explorer reads both `ShouldStop` implementations and the shared
   `iterationCapShouldStop` helper.
2. It correctly identifies the explorer path's first guard:
   `if e.investigationComplete {`.
3. Evidence emission struggles to ground the boolean field condition and later
   uses related assignment and absence evidence to prove the same mechanism.

Root cause: condition evidence is not ergonomic for field-access boolean guards,
even though these guards are precise structural facts.

Generalization: guard-heavy Go code frequently uses boolean fields, enum
fields, and config flags in conditions. These should be first-class condition
anchors across mechanism, diagnostic, and change-impact questions.

### E20260514-G10: Large Exhaustive Enumeration Materialization (`u8b`)

User request asks for all exported string enum types in `internal/types`.

Observed data flow so far:

1. Analyzer correctly classifies the task as exhaustive enumeration.
2. Explorer finds 91 `type X string` declarations in the target package.
3. The task also requires the narrower predicate "has a corresponding const
   set", so raw declarations are only candidates, not automatically principal
   members.
4. The model manually filters and recounts, at one point producing 81 members
   while the broader grep count remains 91.
5. The pipeline then requires member-specific evidence/support for every
   retained member, causing long manual batching and retry pressure.
6. This is mechanically discoverable from source, but the model becomes the
   bottleneck for filtering candidates and transferring every row into
   structured evidence.

Root cause: large closed-set handoff does not have a compact deterministic
carrier that preserves candidate rows, qualification decisions, exclusions, and
per-member support handles while avoiding LLM retyping of every evidence row.

Generalization: this applies to enum inventories, route tables, CLI flags,
config keys, exported symbols, test cases, supported language matrices, and
any bounded set with tens or hundreds of source-backed members.

### E20260514-G11: Carrier Field Visibility False Positive (`logtri_java`)

User request asks to trace the root cause of an external Java exception stack.

Observed data flow so far:

1. Log triage correctly identifies the nested cause chain:
   `Connection refused: /10.0.0.5:5432` -> unresolved user -> NPE ->
   top-level RuntimeException.
2. Analyzer marks stack frames as external (`resolved_files=0`) and routes the
   answer through runtime-observation grounding rather than repo citations.
3. Finalizer uses `citation_ref=-1` structurally for runtime observations.
4. Retry feedback reports that `citation_ref` appears in user-visible prose,
   even though the tail suggests the field may only be present in the structured
   carrier.

Root cause: internal-field concealment likely checks the wrong surface or emits
too-coarse diagnostics. It should inspect rendered answer text, not hidden
carrier JSON fields.

Generalization: any answer family that uses negative citation refs,
block/item schema fields, or internal carrier metadata can receive misleading
repair instructions unless validation separates structured payload from
rendered prose.

Additional reproduction: `logtri_goroutine_dump` passed but repeated the same
pattern. The finalizer explicitly repaired a generated answer because internal
notation for artifact-only citation handles appeared in a user-visible summary
draft. This strengthens the conclusion that visibility validation and repair
feedback must be scoped to the final rendered answer blocks, with hidden carrier
fields excluded from the check.

Additional reproduction: `logtri_node` passed after 297s, 6 rejects, and 6
self-consistency reviewer activations. The finalizer alternated between naming
runtime artifact paths such as `/app/src/user.js` and removing them because
repair feedback framed them as current-repo path leakage. Artifact paths are
not repo citations, but they are often useful runtime observations; the policy
needs to distinguish "artifact frame path rendered as observed data" from
"current-repo citation or helper-specific implementation detail".

### E20260514-G12: Deterministic Scalar Handoff Missing (`u7a`, `s7a`)

`u7a` asks which commit first introduced `EvidenceClosure`.

Observed data flow:

1. Analyzer correctly classifies the request as a git-history lookup.
2. Explorer runs the deterministic VCS query and finds the answer:
   short hash `01e0864` and subject
   `feat(cgec): Citation-Grounded Evidence Closure — 4-invariant cross-stage contract`.
3. The output reaches extract, but the case times out instead of rendering the
   known scalar pair.

`s7a` asks for an exact total line count under `internal/tool`.

Observed data flow:

1. Analyzer recognizes a count / measurement-scalar question.
2. The answer should be computable by deterministic file listing plus `wc -l`.
3. The run times out before producing a value.

Root cause: deterministic scalar values found or findable by tools are still
forced through ordinary LLM-driven extract/finalize stages.

Generalization: commit hashes, authorship dates, line counts, file counts,
sizes, checksums, command outputs, and other tool-sourced literals need a
first-class scalar handoff and renderer.

Batch 2f progress:

- Started the deterministic count branch of the scalar handoff. When the
  analyzer typed lane says `is_count_question=true` and the task is not a git
  history lookup, an unambiguous deterministic `exec_command` count result is
  now compiled into an `AnswerAggregateScalar` with `answer_axis=count` and
  `proof_source=exec_command`.
- Completion no longer treats raw command output only as permission to stop.
  The scalar value travels through the existing aggregate-fact carrier that the
  finalizer and pre-emit checks already consume, so the exact number becomes a
  structured answer obligation rather than prompt memory.
- Seesaw guard: if multiple successful deterministic count command outputs
  disagree, completion fails closed and asks for structured handoff instead of
  letting one broad/noisy command silently satisfy another count. History-count
  questions remain outside this shortcut because they need commit/filter rows
  and exclusion reasons from the G25 history carrier.
- Red-line guard: the hard decisions consume typed predicates
  (`is_count_question`, `is_history_lookup`) plus the deterministic count proof
  parser over tool output. They do not inspect user request prose or model
  answer prose for keywords.

Verification:

- `go test ./internal/tool -run 'DeterministicCommandOutput|ConflictingDeterministicCounts|HistoryCountRequiresAggregateHandoff|RelationalCountAlsoNeedsStructuredProof|DeterministicCount'`
- `go test ./internal/types -run 'DeterministicCountProof'`
- `go test ./...`
- `make`
- `bash eval/run.sh eval/cases/s7a.case 1`
  (`eval/results/s7a-20260515-045731`, PASS; `extractor_iters=1`,
  `finalizer_iters=1`, no repair lines)

### E20260514-G13: External-Only Runtime Log Fast Path (`logtri_rust`)

User request asks "what is this error?" for a Rust panic log.

Observed data flow:

1. Log triage extracts the complete runtime fact:
   `called Option::unwrap() on a None value`, with frames in external Rust
   paths and `resolved_files=0`.
2. Analyzer fixes the diagnostic intent after an initial shape rejection.
3. Explorer begins to search the codebase for config loading despite the
   external-only warning.
4. The case times out instead of answering directly from the artifact.

Root cause: external-only runtime artifacts are treated as a soft routing hint
rather than a precise hard boundary once triage has sufficient facts.

Generalization: crash logs, traces, goroutine dumps, mobile stack traces,
browser console logs, and third-party service logs often have no source files
in the current checkout. They need artifact-only completion with explicit
"cannot verify current code" caveats.

Additional reproduction: `logtri_goroutine_dump` eventually passed, but only
after 1240s and three rejects. The attached log already contained the complete
answer: goroutines 15, 87, and 120 all crashed with
`fatal error: concurrent map writes`. The run still attempted to inspect
`internal/agent/analyzer.go`, discovered frame drift because
`main.writeSession` no longer exists at that location, searched for related
current-code symbols, and tried to ground artifact frames against repository
comments. Correct final behavior was possible, but the route was too expensive
for a self-contained external artifact.

Additional reproduction: `logtri_node` is a simpler external-only Node stack.
The log already answers the question: the TypeError originates at
`processUser`, called by `handleRequest`, after an undefined value is read for
`name`. The run still tried to read `/app/src/user.js`, emitted ungrounded
evidence, used an evidence-floor waiver, and then spent finalizer iterations
removing and rephrasing artifact frame paths. This should be a direct
artifact-only render.

### E20260514-G14: Diagram Relation Over A Set (`qf_type_relation_loop_controller`)

User request asks for a Mermaid type/class relationship diagram for
`LoopController` and its main implementation types.

Observed data flow:

1. Analyzer initially stalls for over 16 minutes before retrying.
2. Retry pre-scan finds the `LoopController` interface and a partial
   implementer set, including `explorerEvaluator`, `analyzerEvaluator`,
   `subExplorerEvaluator`, `extractorEvaluator`, and
   `answerDocumentEvaluator`.
3. The case times out before stabilizing the full set and rendering a diagram.

Root cause: relation diagrams over implementation sets need both exhaustive
typed member discovery and a stable diagram rendering contract. Without a
canonical relation row layer, the run can spend budget on discovery, duplicate
member reasoning, and diagram formatting.

Generalization: interface->implementer, class hierarchy, route->handler,
module->export, package->entry, and registry->name diagrams all share this
relation-set rendering problem.

### E20260514-G15: Timeout Accounting And Slot Health

The sweep is configured with `parallel=4` and `timeout=1800s`.

Observed data flow:

1. Summary records several timeout rows with elapsed values above the configured
   1800s timeout, e.g. 1981-2019s.
2. During polling, a timed-out case briefly appeared to remain in `ps` while new
   cases were already running, making active-slot accounting hard to inspect.
3. The stale-looking process cleared later, but the operational signal is noisy.

Root cause: timeout handling does not give crisp process-group cleanup and
stage-level diagnostics to the operator.

Generalization: full eval sweeps need reliable operational semantics: no leaked
child processes, no ambiguous active-slot count, and timeout rows that explain
which stage was running and why cleanup lagged.

### E20260514-G16: Error-Granularity Verdict Surface (`u9b`)

User request asks whether `emit_evidence` fails the whole batch when exactly one
item lacks `anchor_kind`, or whether the rest of the batch succeeds while only
the bad item is rejected.

Observed data flow:

1. Analyzer initially found the relevant tests and implementation, but several
   transport / missing-`emit_analysis` retries delayed the run.
2. Explorer found the decisive code path in `internal/tool/emit_evidence.go`:
   the batch loop calls `buildEmitEvidenceItemWithSwap` per entry, validation
   errors add that entry to the rejected list and `continue`, and the whole
   batch is rejected only when `len(built) == 0`.
3. Finalizer first emitted a correct summary with inline identifier surfaces
   such as `rejectedItems`; a validator then requested removing or grounding
   those inline code identifiers and adding a required facet.
4. Final answer still states the correct behavior:
   "整个调用不会失败", "只有缺失该字段的那一个条目被拒绝", and
   "其余正常条目继续处理并成功返回".
5. Eval fails on the first regex group because the expected synonym set includes
   `per.?item`, `单项`, `单条`, `逐项`, `每项`, `partial`, and `部分`, but not
   the answer's natural `条目` / `条目级` wording.

Root cause: the pipeline does not carry a typed, canonical verdict for
error-handling granularity questions, and the harness is matching free prose
with an incomplete synonym regex.

Generalization: batch-vs-item, all-or-nothing-vs-partial, fail-fast-vs-collect,
strict-vs-best-effort, and transaction-vs-record-level questions need the
answer to expose a canonical verdict in addition to prose explanation. Eval
should assert that typed verdict, not depend solely on natural-language regexes.

Batch 5b implementation:

- Added required analyzer carrier `error_granularity_profile`. It is inactive
  when no failure-scope verdict is requested; when active it carries grounded
  `source_quotes`, confidence, and optional typed `requested_verdict_options`
  for alternatives explicitly contrasted by the current request.
- Added `AnswerDocumentV2` decision-block field
  `error_granularity_verdict` with canonical enums:
  `per_item_rejection`, `whole_batch_failure`, `partial_success`, `fail_fast`,
  `collect_errors`, and `not_enough_evidence`.
- Propagated the lane through `RequestModel`, `AnswerSemanticView`, finalizer
  instructions, dynamic answer schema, block normalization, pre-emit checks,
  post-emit contract checks, rendering, and eval expectations.
- Added a specificity guard for either/or questions: if analyze captured
  explicit requested alternatives, the final typed verdict must be one of those
  alternatives or `not_enough_evidence`. This prevents a broader umbrella value
  such as `partial_success` from satisfying a question that explicitly asks
  "per item or whole batch?".
- Red-line guard: hard decisions compare typed analyzer profile fields against
  typed answer-document fields only. They do not inspect the user's prose or
  rendered answer prose to decide whether the verdict obligation is satisfied.

Verification:

- `go test ./internal/types ./internal/tool ./internal/agent ./internal/orchestrator ./internal/skill -run 'ErrorGranularity|TestEmitAnalysisSchemaIncludesErrorGranularityProfile|TestEmitAnalysisSchemaMatchesContract|TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersErrorGranularityContract|TestAnalysisSkill_RequiredFieldsEnumeratedEverywhere|TestAnalysisSkill_CurrentQuestionPrimacy_NamesEveryIntentField'`
- `go test ./...`
- `make`
- `bash eval/run.sh eval/cases/u9b.case 1`
  - `eval/results/u9b-20260514-222125`
  - PASS, with rendered principal verdict token `per_item_rejection`.

Batch 5c follow-up:

- Closed G53 with a typed lane conflict resolver. When the analyzer emits an
  active `error_granularity_profile` and does not also emit a count/category/
  relation answer predicate, numeric phrases are treated as failure-scenario
  context instead of `enumeration_boundary` or enumeration-family obligations.
- The same replay exposed G54 before the final PASS: the required typed
  decision block could not cite principal evidence because the support lane
  still allowed only generic prose/list/diagram block kinds. The support policy
  now consumes the typed error-granularity profile and admits decision blocks
  only for this active verdict lane.
- A post-rebase replay exposed G55: the same typed profile can arrive with
  `intent=root_cause`, and root-cause family priority was broad enough to
  preempt the failure-scope decision guard. The resolver now handles this
  classifier variant before no-attachment diagnostic routing, while preserving
  attached log/perf root-cause behavior.
- Red-line guard: these fixes consume typed carriers
  (`error_granularity_profile`, `SemanticPredicates`, `Intent`,
  `QuestionFamily`, and support-lane metadata). No hard decision reads user
  prose, rendered answer prose, or model explanation text.

Verification:

- `go test ./internal/types ./internal/analysis/amplifier ./internal/analysis/compiler ./internal/agent ./internal/tool ./internal/skill -run 'ErrorGranularity|FailureScope|R1_NoFire_ErrorGranularity|ResolveQuestionFamily_ErrorGranularity|CompileGeneric_ErrorGranularity|ReconcileScenario|InferScenario|SuppressesContextualEnumeration|AnalysisSkill'`
- `go test ./internal/types ./internal/orchestrator -run 'ErrorGranularity|PrincipalEvidenceSupportLane|SupportPlan|PrincipalSupportBlockKind|FacetCoverage'`
- `bash eval/run.sh eval/cases/u9b.case 1`
  - `eval/results/u9b-20260514-232523`
  - PASS, with `tool_read_file=2`, `explorer_iters=5`,
    `finalizer_iters=2`, no repair execution, and typed verdict
    `per_item_rejection`.

### E20260514-G53: Contextual Failure-Scenario Counts vs Enumeration Lanes

User request asks whether a batch call fails completely when exactly one item
is missing `anchor_kind`, or whether only the bad item is rejected and the rest
continues.

Observed data flow:

1. Batch 5b correctly added and rendered the typed
   `error_granularity_verdict=per_item_rejection`.
2. The analyzer still tried to emit `enumeration_boundary` for the contextual
   phrase "exactly one item"; those emits were rejected, but planning later
   still resolved the semantic family as `enumeration`.
3. The request then paid enumeration-family cost (`explorer_iters=40` in the
   residual replay) even though the answer surface was a single verdict plus
   explanation.
4. This created a seesaw: tightening the verdict lane fixed answer semantics
   but activated an adjacent count/enumeration lane from a scenario parameter.

Root cause: count-like phrases had no typed answer-axis binding. A quantity
that describes the failing member in a scenario could be interpreted as a
principal answer-set cardinality unless another layer corrected it.

Corrective action:

- Added `ErrorGranularityCountsAreContextual` as the shared typed resolver:
  active `error_granularity_profile`, non-enumerate intent, and no typed
  count/category/relation predicate means numeric phrases are contextual.
- Suppressed contextual `enumeration_boundary` at `emit_analysis`, guarded R1
  multi-subject amplification, resolved scenario/family to generic for
  failure-scope decision answers, and updated analyzer reconciliation plus
  skill guidance to match the same contract.
- Added tests across the typed helper, amplifier, compiler scenario, analyzer
  reconcile, facet family, generic semantic-view compile, and emit-analysis
  boundary.

Commercial-grade invariant:

- The pipeline may still use counts structurally when the analyzer emits an
  explicit count/category/relation answer lane. Without that typed lane,
  scenario quantities stay soft context and cannot hard-route the answer into
  enumeration machinery.

### E20260514-G54: Typed Verdict Block vs Principal Evidence Support Lane

The first G53 replay found a second-layer support-plan conflict before the
final PASS.

Observed data flow:

1. The finalizer emitted the required principal `decision` block with
   `error_granularity_verdict=per_item_rejection`.
2. That decision cited current-code evidence from the principal evidence lane.
3. `validatePrincipalSupportBlockKind` rejected the citation because generic
   principal evidence allowed `summary`, `section`, ordered/bullet lists, and
   diagrams, but not `decision`.
4. Repair feedback then pulled the model toward a summary block, while the
   typed verdict contract still required a principal decision block.

Root cause: typed answer-block requirements and support-lane allowed-block
policies were compiled independently. Adding a typed decision verdict changed
the required principal surface but did not widen the corresponding evidence
support lane.

Corrective action:

- Added request-aware principal evidence block policy. The default generic
  support-lane policy remains unchanged, but when
  `ErrorGranularityProfile.Active()` is true, principal evidence can support
  `decision` blocks.
- Added a support-plan regression test so future typed verdict lanes cannot
  introduce a required principal block that has no legal citation route.

Commercial-grade invariant:

- New typed answer surfaces must be wired through both sides of the contract:
  required answer block shape and legal evidence-support block kinds. A
  carrier that changes one side must be visible to the other side through typed
  metadata, not by asking the finalizer to work around validator feedback.

### E20260514-G55: Failure-Scope Verdict Lane vs Root-Cause Family Priority

A post-rebase replay of `u9b` surfaced a classifier variant after remote
finalizer recovery changes landed.

Observed data flow:

1. The analyzer emitted the correct active `error_granularity_profile` and no
   count/category/relation answer predicate.
2. It labeled the task `intent=root_cause` because the wording asks about
   failure behavior.
3. `ResolveQuestionFamily` returned `root_cause_trace` before consulting the
   failure-scope decision guard.
4. The finalizer was then asked to render historical observation, current code
   verification, and a bounded current-status verdict, instead of the narrower
   failure-granularity decision answer. That reopened a scaffold loop unrelated
   to answer correctness.

Root cause: the typed resolver existed, but its priority was lower than the
broad diagnostic family. A harmless classifier variant could therefore bypass
the same typed lane.

Corrective action:

- Promoted no-attachment failure-scope decision answers ahead of root-cause
  scenario/family routing in the compiler and facet-family resolver.
- Kept attached log/perf artifact diagnostics on the root-cause family because
  artifact/current-code drift is a real part of those answer surfaces.
- Added tests for root-cause-labeled failure-scope requests in scenario
  inference, analyzer scenario reconciliation, and question-family resolution.

Commercial-grade invariant:

- Narrow typed answer lanes should outrank broad diagnostic labels when they
  describe the principal answer surface. Broad families can still win when a
  separate typed artifact lane makes their additional surfaces structurally
  necessary.

### E20260514-G17: Small Relation Member Set Over-Scaffolding (`qf_relation_subagent_registry`)

User request asks for the default SubAgent names registered into
`SubAgentRegistry`, including total count, complete members, registration
evidence, and `Name()` return evidence.

Observed data flow:

1. Analyzer initially stalls and then finds the decisive facts:
   `RegisterDefaultSubAgents` registers only `NewSubExplorer(deps)`, and
   `SubExplorer.Name()` returns `"explorer"`.
2. Explorer confirms the complete set is a singleton: `members=["explorer"]`
   with `value=1`.
3. Extractor first tries to use the registration call as the member anchor, but
   the answer-symbol layer rejects it because the call site is not the literal
   member definition. The model then oscillates between literal member evidence
   and registration call evidence.
4. Finalizer emits a correct one-member answer, but the generic citation floor
   and block contract require a third citation, summary block, caveat block,
   uncertainty facet, and call-site anchoring.
5. A later repair sees `subagent.go:64` as a visible number and flags a count
   mismatch against expected count `1`.
6. The run finally passes after 1450s, 11 rejects, 13 file reads, and 6
   finalizer iterations. The final answer is acceptable, but it contains
   awkward boundary prose and line-reference wording caused by validator
   pressure rather than user need.

Root cause: the pipeline has the right aggregate facts but lacks a canonical
row model for relation/registry members. A single row should carry:
member label (`explorer`), registration call (`RegisterDefaultSubAgents` ->
`NewSubExplorer`), name-return literal (`SubExplorer.Name()` -> `"explorer"`),
entrypoint call (`initApp`), and rendered count (`1`). Instead, the model
manually re-balances these surfaces across independent validators.

Generalization: plugin registries, CLI command maps, route tables, provider
maps, default skill/tool registrations, and enum-value registries all need
small closed-set rendering from typed rows. Generic citation floors should not
force unrelated extra citations or let file-line numerals interfere with the
user-facing count.

### E20260514-G18: Security Call-Chain Citation Indexing (`u1a`)

User request asks whether codrax's `exec_command` tool has command-injection
risk, and asks for actual taint source, sink, and existing protection points.

Observed data flow:

1. Analyzer and explorer find the key chain: `execCommandParams.Command` is the
   taint source, `NewShellCommandContext` / `exec.CommandContext` are the sink,
   and the defenses include read-only gating, shell operator and command
   substitution rejection, argument checks, resource caps, process-group
   cleanup, and multi-repo active-set gating.
2. Extractor initially treats the mechanism question as prose, then emits
   anchor symbols because it sees two sub-topics (`exec_command`, `ExecCommand`).
3. Finalizer first emits no citation pool, then rebuilds a long citation list,
   then repairs missing inline code references, then repairs citation array
   length, then repairs citation drift for individual call-chain items.
4. The case passes after 1547s, 10 rejects, 11 file reads, 6 finalizer
   iterations, and semantic reviewer activity.
5. The final answer is mostly useful, but one visible row says
   `verifyResourceCaps / wrapShellCommandWithCaps` while citing
   `internal/tool/exec_command_readonly.go:250` (`shellOperatorWrites`),
   showing residual citation drift despite the pass.

Root cause: long mechanism/security explanations are represented as free-form
blocks plus manually indexed citations. The system has typed evidence, but no
deterministic row compiler that maps source -> guard -> transformation -> sink
into stable step ids and citation handles.

Generalization: security flows, taint analyses, lifecycle sequences, request
pipelines, scheduler paths, and multi-layer defense explanations should not ask
the finalizer to hand-maintain citation indexes. A typed row model can preserve
ordering, claim form, cited source, and rendered text separately.

### E20260514-G40: Typed Principal Enumeration Lane Not Consumed (`s5b`)

User request asks to list every child package under `internal/analysis/` plus
each package's entry function.

Observed data flow:

1. Analyzer emitted `intent=enumerate`, `question_kind=enumeration`,
   `predicates.is_category_enumeration=true`, `is_relational_lookup=false`,
   and 25 entity values corresponding to the child package directory names.
2. Analyzer omitted `completeness_obligation`, so
   `RequiresExhaustiveEnumerationMemberSetHandoff` returned false even though
   the non-relation category enumeration entity lane was already a typed
   principal-member lane.
3. Explorer read 26 files, including the 25 package files plus one outside
   `internal/analysis`, and accepted closure without an authoritative
   `member_set`.
4. Extractor downgraded to a lower-bound symbol slate, and finalizer rendered
   17 cited rows plus a caveat that omitted tail packages such as `subject`,
   `sourcemix`, `stopcond`, and `normalizer` from the principal list.
5. The answer was structurally accepted because no hard completeness/member-set
   contract survived into finalization.

Root cause: "explicit count/completeness" was the only non-relational path into
structured member-set handoff. A typed category-enumeration member lane was
treated as search context unless a second obligation field also fired.

Generalization: any package/module/enum/type/category list can lose members
when analyzer identifies many principal members but omits a separate universal
obligation. The durable boundary is the typed member lane itself plus
non-relational enumeration shape, not localized "all/every" wording.

### E20260514-G41: Analyze Stage Line-Level Grep Drift (`s5b`)

Observed data flow:

1. The analysis prompt still allowed a Round-2 `grep(files_only=false)` carve-out.
2. Runtime `validateAnalyzerPrescanToolCall` admitted that call when the legacy
   ClassificationGrep trigger/budget allowed it.
3. Analyzer read function-signature line snippets and inferred a noisy "26
   packages" count in thinking/sub-topic prose, while the precise directory
   listing showed 25 child directories.
4. Batch 1g prevented that noisy count from becoming a hard
   `enumeration_boundary`, but the content leak still polluted soft guidance
   and encouraged later stages to reason from analyzer prose instead of
   structured exploration rows.

Root cause: prompt and runtime had drifted away from the evidence-lite
boundary. Analyze should classify from existence/location probes, not line
content.

Generalization: line-level analyzer probes can import any source-text noise
into request classification: counts, helper names, string literals, function
signatures, and comments. Those are exploration evidence and must not drive
hard classification contracts.

### E20260514-G42: Member-Set Repair Support Rows Are Model-Heavy (`s5b`)

Observed data flow:

1. After the Batch 1i typed principal lane change, focused `s5b`
   (`eval/results/s5b-20260514-190353`) reached explore with all 25 package
   members, including the tail packages `sourcemix`, `stopcond`, and `subject`.
2. Explorer emitted a `member_set` but the first completion attempt was
   downgraded because `findings_validator → Validate` had no accepted typed
   evidence or member-specific `support_ref`.
3. The repair dispatch correctly read
   `internal/analysis/findings_validator/validator.go`, but then attempted to
   re-emit a full 25-row evidence payload with many invalid `surface_terms`.
   The tool rejected that payload; the model eventually succeeded by emitting a
   25-member `member_set` whose members/support refs carried exact `@ file:line`
   anchors.
4. Finalizer consumed the structured principal lane and passed after adding the
   required uncertainty-boundary caveat. The eval passed, but explorer needed
   29 iterations, two explorer dispatches, and 12 mid-loop injections.

Root cause: strict completion now prevents partial principal sets, but the
repair lane has not yet been compiled into stable per-member support rows.
Missing one typed support item asks the model to reconstruct a large aggregate
payload instead of letting the system identify the exact missing member and
candidate support location.

Generalization: large enumerations across Go, Python, ArkTS, Cangjie, package
names, module names, and source-location rows need a deterministic
member-support table before finalization. Hard gates should keep using precise
typed evidence/support refs, while repair hints should be narrow and row-based
so fixing one missing support member does not destabilize already-accepted
members.

### E20260514-G43: Composite Relation Member Grain Conflict (`s5b`)

Observed data flow:

1. After the Batch 1j support-row change, focused `s5b`
   (`eval/results/s5b-20260514-192600`) no longer repeated the earlier
   `findings_validator → Validate` completion downgrade. The accepted
   `read_file` gutter line was available as a member-specific support row.
2. The same run exposed a downstream seesaw: the aggregate member set contained
   a composite principal member,
   `perftriage → MergePerfBundles + CorroborateStallFiles`.
3. The finalizer repeatedly tried two reasonable renderings:
   one combined row to preserve the model-authored member string, and two
   cited rows so each function could point at its own file:line.
4. Pre-emit validation rejected both shapes in alternation. Member coverage
   wanted the exact composite string visible, while citation alignment wanted
   each visible symbol row to cite its own line. I stopped the run rather than
   spending more provider budget on a contract conflict.

Root cause: relation-member coverage had a single-target display equivalence
rule but no precise multi-target relation equivalence. A hard gate was
comparing the raw authored display string, while another hard gate compared
the split source-citation rows.

Generalization: many language and package ecosystems expose the same shape:
`package -> A + B`, `namespace: Foo, Bar`, `service → Start 和 Stop`, or
`module → init/cleanup`. The generalized row rule is: split display rows may
satisfy a composite member only when every target row carries the same
left-axis label and all explicit right-side symbols are present. This preserves
the hard-gate principle because the left axis and target symbols are precise
typed surfaces, not search scores or prose guesses.

### E20260514-G44: Self-Consistency Sortedness False Positive (`s5b`)

Observed data flow:

1. After the G43 pre-emit fix, focused `s5b`
   (`eval/results/s5b-20260514-194231`) reached finalization and
   `emit_answer_document` accepted a 25-row answer with no repeat of the
   composite `perftriage` member rejection loop.
2. The semantic self-consistency reviewer then marked the answer inconsistent
   because the summary said the list was sorted by subpackage directory, while
   the reviewer claimed `aggregator -> amplifier -> axis -> ...` was not
   alphabetical.
3. The reviewer judgment was incorrect for the visible list order, but it still
   triggered a rewrite dispatch. The final eval passed, but recorded extra
   finalizer/semantic-quality cost.

Root cause: a free-form reviewer assertion about ordering was treated as
rewrite-worthy despite the accepted answer document already carrying a
deterministic ordered list. The reviewer used noisy prose reasoning where a
simple comparator over visible row labels would have been precise.

Generalization: sortedness, count agreement, duplicate detection, and row
presence are deterministic row-table properties. They should be checked by the
same canonical row layer used by pre-emit validation, while the reviewer stays
advisory for semantic coherence that cannot be reduced to typed rows.

### E20260514-G45: Red-Line Remediation For Attempted Text-Matching Gates

Audit scope:

1. Commit `d2289e7a` added a deterministic row-order checker, but the decision
   to apply it was triggered by keywords in the self-consistency reviewer's
   contradiction text.
2. Commit `e07cafb5` added a user-excluded-category answer check, but it
   detected exclusions by matching words in the user request and detected leaks
   by matching words in the final answer prose.
3. Both paths produced hard `Viol*` decisions from prose text rather than from
   typed analyzer/explorer/finalizer fields.

Corrective action:

- Removed the G45 answer-side excluded-category hard gate entirely.
- Removed the G44 reviewer-text row-order suppression path entirely.
- Stopped the in-flight `qf_multi_member_set_count_caveat` replay that was
  running against the invalid gate.
- Follow-up Batches 6b/6c closed the underlying product gaps with typed lanes:
  reviewer `contradiction_kind` plus deterministic V2 row-order metadata, and
  analyzer `answer_exclusion_policy` plus answer-row `candidate_role`.

### E20260514-G46: Field/Value Count Gate RawRequest Inference

Audit scope:

1. Commit `af8f5a9c` introduced a useful cross-language field/value coverage
   gate for cases like `CitationReq.Required=false`.
2. The pre-complete target extraction read `RawRequest`, analyzer keywords, and
   entities to infer both the field surface and the literal value. The literal
   path included a hardcoded boolean/null token set plus adjacent regexes.
3. That meant a hard downgrade could be triggered by user prose / keyword
   surfaces rather than a typed request carrier, violating the red-line class
   identified in G45.

Corrective action:

- Added `RequestModel.FieldValueProfile` as the typed analyzer carrier:
  `target`, `owner`, `field`, `literal`, `literal_kind`, `source_quote`,
  `confidence`, and `rationale`.
- Added `emit_analysis.field_value_profile` schema and validation. The only
  RawRequest check is analyzer-boundary provenance validation that
  `source_quote` was copied from the current request and contains both the
  target and literal; downstream stages do not inspect the user question.
- Updated explorer field/value guidance and the pre-complete coverage downgrade
  to consume only `FieldValueProfile`.
- Removed the downstream RawRequest/keyword inference helpers
  (`fieldValueCountTargetCandidates`, `fieldValueCountLiteralFromRequestModel`,
  `fieldValueAdjacentLiteral`, and the explorer's
  `requestModelHasFieldValueLookupSurface`).

Commercial-grade invariant:

- If the analyzer cannot emit a validated `field_value_profile`, field/value
  coverage remains soft guidance from ordinary search terms. Hard coverage
  downgrades require the typed carrier.

### E20260514-G47: Self-Consistency Row-Order Gate Typed Kind

Audit scope:

1. Commit `d2289e7a` tried to fix G44 by suppressing reviewer row-order
   false positives after checking whether AnswerDocument rows were sorted.
2. The deterministic row check was the right direction, but the trigger was a
   keyword scan over reviewer `topic`, `summary_claim`, `body_claim`, and
   `reasoning`.
3. That made model-authored review prose a hard-control input.

Corrective action:

- Added `contradiction_kind` to the reviewer tool contract with typed values
  such as `numeric_mismatch`, `direction_mismatch`, `row_order_mismatch`, and
  `fabricated_identifier`.
- Added a deterministic V2 row-order profile over principal list/table rows:
  visible item labels and citation-path axes are normalized and compared
  directly.
- Suppression is allowed only for typed `row_order_mismatch` contradictions,
  and only when every eligible principal row block is deterministically
  ascending. Unknown-kind or prose-only ordering claims still flow through.

Commercial-grade invariant:

- Reviewer text may explain a contradiction, but hard routing consumes the
  typed kind plus deterministic row metadata only.

### E20260514-G48: User-Excluded Candidate Categories Typed Policy

Audit scope:

1. Commit `e07cafb5` tried to prevent excluded candidates from leaking into
   answers, e.g. listing variables after the user asked not to list variables.
2. It inferred exclusion intent from user-request keywords and inferred answer
   leaks from final-answer prose/token matching.
3. That violated the red line even though the product requirement is valid.

Corrective action:

- Added analyzer `answer_exclusion_policy` with
  `excluded_candidate_roles[]`, `source_quotes[]`, confidence, and rationale.
  Source quotes are only provenance-validated at the analyzer boundary.
- Added `items[].candidate_role` to the block-only answer carrier so final
  answer rows can declare language-neutral roles (`function`, `type`,
  `variable`, `test`, `generated`, `private`, etc.).
- Added a typed hard check that compares
  `AnswerExclusionPolicy.ExcludedCandidateRoles` with principal answer item
  `candidate_role` values. It does not read RawRequest or answer prose.

Commercial-grade invariant:

- Without a validated `answer_exclusion_policy`, exclusion remains ordinary
  instruction-following guidance. Without an answer-row `candidate_role`, the
  hard check does not guess from prose; future coverage improvements should
  add deterministic row compilers, not text scans.

### E20260514-G19: Explore-to-Extract Fact Handoff Loss (`qf_architecture`, FAIL on stale 2026-05-14 sweep)

User request asks for the read-mode pipeline stages and each stage's
responsibility.

Observed data flow:

1. Analyzer correctly classifies the request as an architecture/stage
   enumeration and identifies `StageAnalyze`, `StageExplore`, `StageExtract`,
   and `StageFinalize`, with conditional pre-stages for logs and perf traces.
2. Explorer reads the enum, topology, stage binding, and agent files, then
   records a complete narrative: conditional `log_triage` / `perf_triage`, main
   `analyze -> explore -> extract -> finalize`, and each agent/evaluator role.
3. Extractor sees 1116 evidence items and multiple answer chains, but because
   the complete stage list did not arrive as formal answer symbols, it marks
   the main hypothesis inconclusive and preserves uncertainty that the evidence
   itself has already resolved.
4. Finalizer then reconstructs the answer from prose evidence, enters citation
   repair loops, and starts correcting enum-line citation drift and hidden
   carrier-name leakage.
5. The case ultimately passes in 417s with 11 file reads, 6 mid-loop hint
   injections, 3 finalizer iterations, and 6 rejects. The pass is real, but
   the cost profile shows a systemic typed-handoff gap rather than a one-off
   phrasing issue.

Follow-up from the 2026-05-14 full sweep: the stale snapshot regressed from
high-cost PASS to FAIL because the final answer listed the stages, topology,
agent bindings, and scheduler layers, but never stated the `finalize` stage's
role with a render/generate/compose surface. That is a real product miss for
the user request ("each stage roughly does what"), not merely a regex synonym
gap. The data-flow failure is row-column loss: stage name/order/agent survived,
but the required responsibility column was optional prose and could disappear
for one stage.

Root cause: architecture/list answers can be fully solved in explorer prose and
aggregate facts, but the extract/finalize boundary lacks a required canonical
fact carrier for "closed ordered stage list + responsibilities + optional
conditional pre-stages." When that carrier is absent, later stages confuse
"typed extraction missing" with "answer unknown."

Generalization: pipeline stage lists, request lifecycles, middleware chains,
state machines, hook order, and startup/shutdown flows all need an ordered
`ProcessStepRow`/`ArchitectureStageRow` surface with `name`, `order`,
`trigger/condition`, `agent/owner`, `responsibility`, `input`, `output`, and
`support_ref`. Missing structured symbols should become a repair request to
build rows from grounded explorer facts, not a semantic inconclusive verdict;
missing responsibility on any visible stage row should be caught as a row-shape
gap before final text ships.

### E20260514-G20: Change-Impact File Set Label Conflict (`u4b`, PASS with 24 rejects)

User request asks which source files would stop compiling if the package
`internal/tool/ground` were deleted, and asks to list call sites with file
paths.

Observed data flow:

1. Analyzer correctly classifies the task as package-deletion impact analysis.
   The initial grep surface contains both real imports and noisy mentions.
2. Explorer narrows the set to 7 production files that directly import
   `internal/tool/ground`, then emits the set as an aggregate member set named
   `affected_production_files`, with members carrying `file:line` import sites.
3. Extractor recognizes this as a bounded file-path enumeration and emits
   answer symbols despite a conflicting structured-emission hint that the
   dispatch "does not require" answer symbols.
4. Finalizer enters 9 iterations and 24 rejects. The repair loop alternates
   between two incompatible validators: one says each item label must be the
   file path, the other says every visible member must include
   `label="affected_production_files"` plus the `file:line` member string.
5. The final answer passes only by leaking the aggregate carrier name into
   user-visible prose (`affected_production_files 成员集...`) and repeating
   `affected_production_files: path:line` inside every item.

Root cause: principal set metadata (`member_set.label`) is conflated with the
user-visible item label. For change-impact answers, the canonical item label is
the affected file path, while the aggregate label is a schema/group role that
should remain internal. Validators currently assert both as visible labels.

Generalization: package deletion, field type migration, API rename, route
removal, config-key retirement, and symbol visibility changes all require a
typed `ImpactFileRow`/`AffectedSiteRow` with separate fields for group role,
display label, exact site, dependency kind, and citations. Post-emit checks
should compare the row set semantically instead of requiring internal carrier
labels to appear in prose.

Related recurrence: `u10b` asks for files affected by changing
`CitationReq.Required` from a bool to a three-state enum. Explorer narrows the
answer from an over-broad analyzer pre-scan to 8 production files, and the case
passes in 409s, but still needs 11 file reads, 2 extractor iterations, 2
finalizer iterations, 1 repair, and 3 rejects. This is the same
`ImpactFileRow` problem in a milder form.

### E20260514-G21: Exact Absence Needs First-Class Negative Evidence (`s3a`, PASS with 8 rejects)

User request asks how the effective value of the exact config key
`explore_mid_loop_hint_budget` is computed across code defaults, `codrax.yaml`,
and CLI overrides.

Observed data flow:

1. Analyzer initially finds the exact identifier in glossary/test surfaces and
   nearby config-precedence files, then correctly instructs explorer not to
   substitute nearby keys.
2. Explorer proves absence across the three relevant layers:
   `ExploreHeuristics` and `DefaultExploreHeuristics()` have no field/default,
   `codrax.yaml.example` has no key, and `cmd/root.go` has no CLI binding.
3. Explorer tries to emit an absence result; the tool rejects
   `absence_justification` because evidence has already been emitted, forcing
   the model to encode "resolved absence" indirectly.
4. Extract/finalize require hypothesis verdicts and a negative-scope citation,
   then reject labels and inline identifiers such as a fabricated
   `ExploreMidLoopHintBudget` surface.
5. The final answer is semantically correct but passes with 3 residual quality
   concerns, includes a synthetic `(repo-wide grep)` absence citation, and
   exposes validator caveats in the delivered answer.

Root cause: exact-absence answers lack a typed proof artifact. The pipeline has
positive evidence rows and citation refs, but no canonical `AbsenceProof` that
records query, bounded search scope, layers checked, exact negative pattern,
nearby non-substitutes, and confidence. As a result, absence is represented by
ad hoc prose, fake path-like citations, and repair-loop heuristics.

Generalization: missing config keys, absent CLI flags, nonexistent functions,
zero implementers, no callers, no dependency edges, and "not supported" API
questions all need first-class negative evidence. The finalizer should render a
deterministic absence table from that proof and keep repo-wide search metadata
out of normal source-citation slots.

Related recurrence: `s3d` asks for another absent config key
`explore_xyz_phantom_unique_budget`. It passes in 341s with 2 extractor
iterations, 2 finalizer dispatches, 1 repair, and 3 rejects. The run repeats
the same absence-model issues: `absence_justification` cannot coexist cleanly
with grounded evidence, config-layer anchors drift between precedence lines and
key-list lines, and absent literals in backticks are treated as hallucinated
identifiers.

### E20260514-G22: Diagram Call-Chain Row / Edge Anchor Drift (`qf_sequence_analyzer_gate`, PASS with repairs)

User request asks for a Mermaid sequence diagram showing the call order from
`buildAnalysisIR` to `gate.Run`, followed by key intermediate functions.

Observed data flow:

1. Analyzer and explorer find the full static call chain in
   `internal/agent/analyzer.go`: `buildAnalysisIR -> compiler.Compile ->
   risk.Evaluate -> hdp.Plan -> compiler.RecomputeBudget ->
   amplifier.AmplifyPostCompile -> binder.BindByRelevance -> ... ->
   gate.RunWith`, with `gate.Run` in `internal/analysis/gate/gate.go`.
2. Explorer performs extra reads to fill the span between
   `binder.BindByRelevance` and `gate.RunWith`, including branch guards,
   optional counterfactual expansion, contract construction, required-file
   derivation, and mode extraction.
3. Extract correctly skips symbol enumeration and expects diagram/list support
   lanes to carry the answer.
4. Finalizer then cycles through repairs: missing `branch_guard` facet,
   diagram block lacking typed `relation_kind=call` edges, branch-guard
   evidence mismatch, summary/body order contradiction, and repeated
   extract/finalize re-entry.
5. The case ultimately passes in 679s with 6 file reads, 3 extractor
   iterations, 3 finalizer dispatches, 2 repair executions, 3 rejects, 2
   semantic-quality dispatches, and 7 self-review events.

Root cause: sequence diagrams are rendered from free-form prose plus ad hoc
diagram text, while validators expect typed relation edges, branch guards, and
ordered list rows. The evidence is known, but there is no single canonical
`CallChainStepRow` / `CallEdge` structure that both Mermaid rendering and
post-emit validation consume.

Generalization: call chains, lifecycle diagrams, scheduler paths, middleware
flows, and state transitions should use a shared typed edge table with
`from`, `to`, `relation_kind`, `guard`, `optional`, `source_line`, and
`display_order`. Mermaid should be generated from that table, and the
intermediate function list should reuse the same rows.

Related recurrence: `s8a` asks for the deterministic chain from
`buildAnalysisIR` to `gate.Run` without asking for a diagram. Explorer first
collects the decisive chain including `compiler.Compile`,
`compiler.RecomputeBudget`, `hdp.Plan`, and `binder.BindByRelevance`, then the
pipeline over-expands into 80+ call nodes and a diagram validation problem over
method-chain endpoints such as `ctx.Mutable.SearchGraph`. The final answer
fails the eval after 723s because those key compile/planning/binding terms are
not visible, while the answer includes an unrequested Mermaid diagram and
accessor-level detours. This is a product failure of call-chain row selection,
not evidence retrieval.

### E20260514-G23: Low-Information Log Should Short-Circuit (`logtri_degraded`, PASS with semantic drift)

User request attaches only Lorem Ipsum placeholder text and asks to analyze the
log.

Observed data flow:

1. `log_triage` correctly recognizes the input as placeholder/noise text with
   no language, errors, stack traces, or runtime events, and emits it as an
   unknown chunk.
2. Analyzer sees the internal prior-stage finding
   `two-step produced zero partial bundles (segments=1) -- degraded` and
   treats that operational finding as diagnostic content to investigate in the
   repository.
3. The task drifts from "tell the user this is not a meaningful runtime log" to
   repo investigation of log-triage internals and the phrase "two-step
   produced zero partial bundles."
4. The case passes in 557s with 2 extractor iterations, 2 finalizer dispatches,
   1 repair, 3 rejects, 1 semantic-quality dispatch, and 1 self-review event.
   The final direction is still product-risky: a user who asked to analyze a
   placeholder log receives an internal codrax two-step mechanism diagnosis
   instead of a crisp "no diagnostic signal in the provided artifact" answer.

Root cause: low-information runtime artifacts lack an early deterministic
answer path. Internal pipeline-health annotations are leaking into the
analyzer's semantic task model as if they were user evidence.

Generalization: empty logs, placeholder text, screenshots with no OCR signal,
truncated files, binary blobs, and unsupported artifact formats should return a
typed `NoDiagnosticSignal` result. Internal triage degradation metadata should
be recorded for telemetry and optional caveat text, but it must not become a
repo-debugging target unless the user asks to debug codrax itself.

### E20260514-G24: Import-Path Enumeration Is Not Symbol Definition (`qf_imports`, PASS with 3 rejects)

User request asks which `internal/` packages are imported by
`internal/agent/explorer.go`, listing import paths only.

Observed data flow:

1. Explorer reads the target file's import block and extracts 14 internal
   package import paths, including aliased imports such as `promptctx` and
   `repotypes`.
2. The evidence is anchored to import statements in `internal/agent/explorer.go`
   rather than to definitions inside the imported packages.
3. Extractor tries to emit each import path through `emit_answer_symbol` using
   the package path as the symbol name and the import line as the symbol line.
4. The validator rejects the slate because import paths are references/string
   literals, not symbol definitions. The extractor then falls back to
   `completeness=unknown`, despite the import list itself being fully known.
5. The final answer passes in 320s with only 2 file reads, but the structured
   channel remains degraded: 5 extractor attempts, `completeness=unknown`, 3
   rejects, and a prose-only finalizer that reuses the import list despite the
   failed typed slate.

Root cause: import/dependency enumerations lack a first-class row type. The
answer surface is a list of import-reference literals from a specific file,
but the structured channel currently only knows "symbols with definition
lines" or free-form prose.

Generalization: imports, module dependencies, route strings, tool names,
config keys, build tags, SQL table names, and annotation strings need
`ImportPathRow` / `LiteralReferenceRow` evidence with fields for owner file,
literal text, alias, line, dependency kind, and display label. They should not
be forced through definition-symbol grounding.

### E20260514-G25: Git History Scalar Needs Structured Commit Evidence (`u7b`, PASS with 6 rejects)

User request asks: among the most recent 20 commits that modified
`internal/orchestrator/`, how many directly involve the `runTaskGraph`
function? Return a number.

Observed data flow:

1. Analyzer correctly classifies the task as history lookup + scalar count,
   but can only pre-scan current source and says git history is required.
2. Explorer uses `exec_command` to run git queries. It first obtains the recent
   20 commits for `internal/orchestrator/`, then tries several ad hoc filters
   for `runTaskGraph`.
3. The model initially misinterprets `git log -20 -S "runTaskGraph"` as "last
   20 commits, then filter by string." It later realizes `-20` limits the first
   20 matching pickaxe commits across history, so the set is wrong.
4. It then manually compares recent commit hashes, separates a comment-only
   mention outside `internal/orchestrator/`, and converges on scalar answer
   `0`.
5. The meaning of "directly involves" is reasoned in prose instead of carried
   as a typed predicate over per-commit evidence: function body changed, call
   site changed, comment-only mention, or outside-directory mention.
6. The case passes in 654s with 3 explorer dispatches and 6 rejects, after
   converging to scalar answer `0`.

Root cause: history scalar questions do not have a structured `CommitRow` /
`HistoryFilterResult` artifact. The pipeline mixes shell output, model memory
of commit lists, and prose interpretations of git semantics. That makes the
answer sensitive to command syntax details and predicate ambiguity.

Generalization: "last N commits touching X", "how many changes affected Y",
"which commits introduced/removed Z", regression-window narrowing, ownership
history, and churn metrics need deterministic git evidence rows: commit id,
path scope, diff match type, matched file, matched line/text, predicate verdict,
and exclusion reason. The scalar should be computed from rows, not inferred
from free-form command transcripts.

### E20260514-G26: Comparison Scalar Rows vs Symbol/Member-Set Obligations (`u3b`, PASS with 65 rejects)

User request compares `compiler.templateArchitectureExplain` and
`compiler.templateRootCause` across TaskGraph node count, citation lower bound,
and retry budget, asking for specific values.

Observed data flow:

1. Analyzer/explorer find the needed values early:
   both templates have 5 fixed TaskGraph nodes plus dynamic evidence nodes;
   architecture explain uses citation floor `3` and retry budget `3`; root
   cause uses citation floor `2` and retry budget `4`.
2. Extract initially decides correctly that this is a numeric comparison table,
   not a symbol enumeration.
3. A retry then forces `emit_answer_symbol` for 7 anchors
   (`templateArchitectureExplain`, `templateRootCause`, and related constants)
   even though those anchors are support, not the answer surface.
4. Finalizer receives scalar aggregates and a principal member set describing
   comparison rows, then repeatedly repairs because table prose does not
   surface every exact scalar/member string in the expected form, and because
   comparison rows lack inline code identifiers.
5. The most visible loop is between two incompatible surfaces: when labels are
   source symbols such as `templateArchitectureExplain`, citation alignment is
   satisfied but member-set coverage complains that the conceptual comparison
   rows are not visible; when labels are conceptual rows such as
   `TaskGraph固定节点数` or `Citation下限(finalize SuccessCriteria)`, member
   coverage is satisfied but citation alignment treats those display labels as
   code identities and rejects them. The case eventually passes in 1403s only
   after 4 finalizer dispatches, 65 rejects, and 3 patch-style repair loops.

Root cause: numeric comparison answers do not have a canonical
`ComparisonMetricRow` representation. The pipeline alternates between treating
template names/constants as answer symbols and treating the actual values as
scalar/member-set aggregates. Validators then require both support anchors and
rendered comparison members to be visible, creating duplicate obligations. The
pre-emit label/citation gate also lacks a precise distinction between a
source-code identity label and a model-authored display-row label.

Generalization: any side-by-side comparison of defaults, budgets, counts,
thresholds, flags, enum values, stage settings, or config precedence needs rows
with fields `subject_a`, `subject_b`, `metric`, `value_a`, `value_b`,
`evidence_a`, `evidence_b`, and `difference`. Support symbols should stay
citations, not become principal answer items unless the user asks for them.

### E20260514-G27: Role-Locate Scalar Answers Pull Support Facts Into Principal Surface (`u11a`, PASS with 5 rejects)

User request asks for the exact entry function responsible for parsing the user
request and producing structured `AnalysisIR`, plus the file location.

Observed data flow:

1. Analyzer and explorer locate the answer early: `buildAnalysisIR` in
   `internal/agent/analyzer.go:1289`.
2. The extractor initially oscillates over whether a single identifier lookup
   should be a scalar answer or an `emit_answer_symbol` slate, then emits one
   symbol.
3. Finalizer receives enough answer-grade definition evidence for the scalar
   result, but the role-lookup contract also elevates `current_code_path`.
4. The first finalizer answer pulls in the nearby call-site helper
   `runAnalyzeV3`, which is present in reasoning/search context but not in the
   answer-grade support lane. The inline identifier gate correctly rejects it.
5. The repaired answer replaces the helper with grounded
   `analyzerEvaluator.ParseOutput` and adds the required `current_code_path`
   facet, passing in 249s, but after 5 analyzer iterations, 5 explorer
   iterations, 4 finalizer iterations, 2 finalizer dispatches, and 5 rejects.

Root cause: minimal role-locate questions lack a crisp boundary between the
principal scalar ("the function name and file") and explanatory support facts
("how it is called today"). Once `current_code_path` is elevated, the model
tries to satisfy it by adding extra code identifiers to prose, even though the
user did not ask for a call-chain explanation.

Generalization: single-target role lookup needs a `RoleResolutionRow` carrying
`requested_role`, `resolved_identifier`, `definition_location`, and optional
`supporting_call_edge`. The renderer should keep the principal answer to the
resolved identifier/location, and attach supporting call edges only as cited
same-item detail when explicitly grounded. Support helper identifiers must not
be promoted into new principal claims.

### E20260514-G28: Retired-Symbol Change Impact Needs Existence State + Scope Partition (`u10a`, PASS with 13 rejects)

User request asks which files need changes if `ShapeValue` in
`internal/types/analysis_ir.go` is renamed to `ShapeScalar`.

Observed data flow:

1. Analyzer classifies the task as rename/change-impact and finds many
   `ShapeValue` hits.
2. Explorer then discovers the premise is stale: `ShapeValue` is not a live
   Go constant anymore; current hits are comment, test, and migration-doc
   references to a retired answer-shape system.
3. The investigation still tries to fit the task into ordinary symbol rename
   impact analysis, mixing production comments, tests, docs, and historical
   migration records into one affected-file surface.
4. Evidence emission struggles on docs because historical references are not
   live symbol definitions and often do not provide a whole-word code anchor at
   the cited line. Several doc evidence items need repair or are recovered.
5. Extract/finalize then have to answer a counterfactual with a false premise:
   either "rename the live constant" or "update historical text references".
   Without a typed existence state, scope decisions become prose-level
   judgement rather than deterministic partitioning.
6. Finalizer then oscillates over item labels: file paths are the user's
   requested principal output and are present in aggregate member sets, but
   label-grounding tries to treat them as code-symbol labels unless they are
   file/path display rows with citable support. The model alternates between
   file paths, `ShapeValue`, and descriptive labels, each satisfying one gate
   while upsetting another.

Root cause: change-impact analysis does not first produce a typed
`TargetExistence` verdict and scoped occurrence inventory. A retired/deleted
symbol is structurally different from a live definition with references, but
the current pipeline only sees search hits and tries to infer impact from them.

Generalization: rename/migration questions need `ChangeImpactTarget` with
`exists_as_live_symbol`, `definition_site`, `requested_scope`, and occurrence
rows partitioned by `production_code`, `production_comment`, `test_fixture`,
`documentation`, and `historical_record`. When the target is absent or retired,
the principal answer should explicitly pivot to "no live code rename; only
text/history updates in these scopes" and avoid treating doc/test strings as
live refactor edges. File-output rows must remain file-path display labels,
with file:line anchors as support, rather than being converted into repeated
symbol labels such as `ShapeValue`.

### E20260514-G29: Package Entry Catalog Loses Function Names Across Stage Boundary (`s5b`, PASS with 2 rejects / 5 self-repairs)

User request asks for every sub-package directory under `internal/analysis/`
and the single entry-point function for each.

Observed data flow:

1. Analyzer identifies 25 child directories and splits them into 9 + 8 + 8
   subtopics.
2. Explorer reads the package files and eventually corrects early wrong
   guesses (`axis.Select`, `binder.Bind`, `budget.NewBudget`) to grounded
   entry functions such as `Affinity`, `BindByRelevance`, and `Compute`.
3. The completed investigation knows the 25 package/function/file-line triples,
   but the structured handoff primarily carries package names and line
   locations. Function names are visible in prose/reasoning and some evidence
   items, not in a canonical row that downstream can trust.
4. Extractor says the evidence buffer only exposes partial function names, then
   alternates between `lower_bound`, skip-`emit_answer_symbol`, and relying on
   finalizer prose despite a closed 25-item enumeration request.
5. Finalizer reconstructs the table, but repair loops appear around ordering,
   citation-label alignment, fabricated labels for package names, and duplicate
   function names (`Score` in more than one package).
6. The case eventually passes in 555s, after 3 analyzer dispatches, 2 extractor
   dispatches, 2 finalizer dispatches, 2 rejects, and 5 self-consistency
   repairs.

Follow-up from the 2026-05-14 full sweep: the explorer can now emit a single
`aggregate_facts.member_set` whose members contain package/function/line
surfaces such as `aggregator -> Aggregate (line 132)`. The remaining systemic
gap is that source-line decoration is treated as visible row identity instead
of row support. That makes an otherwise correct row like label=`aggregator`,
text=`Aggregate`, citation=line 132 look incomplete unless the prose repeats
`line 132` inside the member label. Non-location decorators such as
`New (Classifier)` are different: they disambiguate the entry surface and must
remain identity-bearing.

Root cause: "directory + entry function" is a two-column catalog, but the
pipeline models it as either answer-symbol enumeration or free-form ordered
list. The exact row identity (package dir, entry function, file, line,
selection rationale) is not preserved as a single typed principal item.

Generalization: package/module catalogs, handler registries, command tables,
route maps, feature flags, and plugin inventories need an `EntryPointRow` /
`CatalogRow` shape. The row should carry display columns and citations
together, and validators should compare row identity rather than asking labels
to be source-code tokens.

### E20260514-G30: Exported API Catalogs Need Parser-Derived Category Rows (`u8a`, PASS with 29 rejects / 5 self reviews; Batch 0 count-binding slice fixed)

User request asks for all exported API in `internal/analysis/criterion`,
categorized into functions, types, constants, and variables.

Observed data flow:

1. Analyzer uses file-map and grep-style export hints to infer the API surface.
2. Explorer reads `grammar.go` and `eval.go`, finds exported types
   (`Kind`, `Env`, `Result`), functions (`Eval`, `EvalAll`,
   `IsRegistered`, `RegisteredKinds`, `SetExternalArtifactFloor`), exported
   variable `ErrUnknownKind`, and `Kind*` constants.
3. The model initially claims 26 `Kind*` constants and 35 total exported
   symbols, then re-counts the source and `registered` map and corrects to 25
   constants and 34 total exported symbols.
4. The correction is done by reasoning over grep counts and map entries, not by
   a deterministic Go export table. The same data is carried as prose/evidence
   rather than a typed category row set with per-category counts.
5. During finalization, the model splits the output into separate category
   lists, but the aggregate count-claim validator still scans caveat prose
   globally. A legitimate "25 Kind constants" statement is compared against
   unrelated member sets such as "exported functions" (expected 4/5), "types"
   (expected 3), and "variables" (expected 1), creating repeated count repairs
   even when the visible category lists are structurally separate.
6. The repair path then tries a partial document patch that replaces citations
   while preserving citation-bearing blocks, which the mutation runtime
   rejects. A full re-emit follows, but the same broad count binding continues
   to compare the caveat's "20/21/25 Kind" statements against unrelated
   category lists. This confirms the issue is a typed-boundary defect rather
   than a single bad answer draft.
7. The case eventually passes only after the finalizer removes numeric claims
   from the caveat text. The visible answer is correct enough for eval, but
   the system has forced a weaker uncertainty disclosure because precise
   category counts were unsafe to say in a shared caveat block.

Root cause: exported API enumeration is treated as ordinary symbol listing
instead of a language-aware public-surface catalog. Counts and category
membership are model-derived, so constant blocks, registration maps, aliases,
methods, package vars, and generated/test-only surfaces can drift. The
count-claim validator also lacks scoped binding between a numeric claim and the
specific aggregate/member-set label it modifies.

Generalization: API-surface questions need parser-derived `ExportedAPIRow`
records: `package`, `category` (function/type/const/var/method),
`name`, `file`, `line`, `decl_group`, and `scope`. Category counts should be
computed from rows, not from model prose or grep-line arithmetic. This also
covers enum cases, public SDK surfaces, CLI commands, plugin hooks, and
cross-language export tables. Count validation should bind only to the nearest
typed category row or explicit label, not to every number in nearby caveat
prose.

Batch 0 progress: the hard gate no longer treats a member-name mention inside
a shared caveat as sufficient binding when several principal member sets are
present. Numeric claims in multi-set caveats now bind to explicit aggregate
labels only. The remaining G30 work is still the larger cross-language
`ExportedAPIRow` contract, including parser-derived public/private/exported
classification across all supported languages.

Adjacent progress: while hardening the focused eval, support-row compilation
was updated so model-facing display members such as `Kind @ grammar.go:26`
inherit the precise repo-relative `support_ref` location
(`internal/analysis/criterion/grammar.go:26`) when the file suffix and line
match uniquely. This keeps short display paths user-friendly while preserving
full-path citation obligations for all supported languages.

### E20260514-G31: Carrier Visibility Policy Conflicts With Architecture Data-Flow Explanations (`qf_logic_view_read_pipeline`, PASS with 3 rejects / 1 semantic repair)

User request asks for a Mermaid architecture view of the read-mode pipeline,
including analyzer, explorer, extractor, finalizer, Mutable, and BusContext
data flow.

Observed data flow:

1. Analyzer/explorer correctly identify the architecture lane: Orchestrator
   dispatches stages, each agent consumes and writes BusContext/Mutable state,
   and finalization renders the user-visible answer.
2. The finalizer attempts to explain the final stage by naming internal carrier
   surfaces such as `emit_answer_document` and `AnswerDocumentV2`, because those
   are precise code-level objects in the evidence lane.
3. The visible-carrier pre-emit gate rejects those names as implementation
   details because the user did not explicitly ask for answer schema/tool
   internals.
4. The answer repairs and passes in 530s, but the model paid extra cycles to
   translate a valid architecture fact into product-language data-flow prose.

Root cause: the visibility policy has only two coarse modes: hide carrier
terms unless the user explicitly asks for the literal tool/schema name, or show
them. Architecture/data-flow requests sit in between: carrier objects can be
valid supporting facts, but the user-facing surface should describe the state
transition role ("writes structured final-answer state") unless the literal
name is requested.

Generalization: tool names, schema names, context field names, transport
objects, protobuf/JSON payload types, and event topics need a typed visibility
role: `literal_subject`, `supporting_carrier`, or `internal_only`. Finalizer
guidance and gates should render `supporting_carrier` as role prose by default,
while preserving exact literals for scalar/literal questions such as
`emit_answer_document` or `citation_ref` lookups.

### E20260514-G32: Principal Count Mixes Diagnostic Detector With Panic Sources (`logtri_oversized`, PASS with self-consistency repair)

User request asks where the panic in an attached oversized log originated.

Observed data flow:

1. Log triage and exploration identify several current-repo panic-capable code
   sites (`NewOpenAIAdapter`, `NewLRU`, `RegisterCaveatFamily`,
   `RegisterViolKind`) plus the log parser's panic-detection regex
   `logLinePatterns`.
2. Finalizer turns both categories into one numeric summary: "5 panic sources",
   while the principal ordered list contains only 4 actual panic-emitting
   functions.
3. Self-consistency correctly flags the mismatch: the detector anchor is not a
   fifth panic source.
4. Semantic review then accepts the answer by explaining that `logLinePatterns`
   is only the detection tool, leaving the user-visible final answer with the
   wrong count still present in summary and caveat.

Root cause: diagnostic answers lack a typed separation between
`observed_artifact_detector`, `candidate_panic_source`, and
`resolved_panic_source`. Once detector code and source code share one evidence
lane, principal counts can include support machinery rather than only the
requested origin set. The reviewer stack also treats a count mismatch as
explainable commentary instead of forcing a rewrite when the visible final
answer still contains the contradiction.

Generalization: log/perf/root-cause answers need `DiagnosticOriginRow` records
with explicit roles: detector/parser, observed frame, candidate source,
resolved source, and excluded candidate. Counts must be computed only from rows
whose role matches the user-requested answer set. Self-consistency mismatches
on principal counts should remain hard rewrite triggers unless the final
document actually removes or corrects the offending count.

### E20260514-G33: Diagram Payload Can Be Hidden Inside a Section and Disappear From Rendered Output (`qf_diagram_pipeline`, fixed in Batch 0)

User request explicitly asks for a Mermaid flowchart of the 4 read-mode stages.

Observed data flow:

1. Analyzer/explorer correctly identify the four stages and their order:
   StageAnalyze → StageExplore → StageExtract → StageFinalize.
2. Finalizer constructs a Mermaid `flowchart TD` body, but attaches it as the
   `diagram` field of a `kind="section"` block rather than emitting a dedicated
   `kind="diagram"` block.
3. `emit_answer_document` accepts the payload and reports accepted block kinds
   as `summary,section,ordered_list`.
4. The renderer outputs prose/list content but no visible Mermaid fence; eval
   fails with missing ` ```mermaid` / `flowchart` / `graph TD|LR` regex.

Root cause: AnswerDocument validation allows a diagram payload to exist on a
non-diagram block, while the renderer/eval only treat `kind="diagram"` as a
visible diagram surface. The schema accepts a state that cannot satisfy an
explicit user diagram request.

Generalization: all rich surfaces need kind/payload invariants at the
pre-emit boundary: `diagram != nil` requires `kind=diagram`; `kind=diagram`
requires a non-empty diagram body; explicit diagram requests require at least
one renderable diagram block in the final document. The same principle applies
to tables, code fences, charts, and any future structured visual payload: a
payload in the wrong block kind should fail loud before rendering, not become
silent prose-only output.

Batch 0 progress: normalization and mutation merge validation now reject
`diagram` payloads on non-diagram blocks, so the model must either emit a
renderable `kind=diagram` block or remove the payload. Targeted eval
`qf_diagram_pipeline` passed after this invariant landed.

### E20260514-G34: Mechanism Summaries Need Canonical Visible Code Anchors (`s11a`, PASS with 1 patch repair)

User request asks whether the analyzer stage before Explorer is allowed to
call `read_file`.

Observed data flow:

1. Exploration correctly traces the capability surface: stage binding flows
   through `types.AllStageBindings()`, analysis-skill `ToolSuggestions`,
   `AnalysisToolSuggestions`, `BuildAnalysisSkill`, and
   `BaseAgent.buildToolSchemas`.
2. The first final answer has citations and typed `claim_uses`, but the main
   summary prose abstracts the chain enough that it contains no visible inline
   code identifiers.
3. The answer-document validator rejects the block for low code-anchor density
   despite the underlying evidence being correct.
4. A patch repair injects the same chain as visible identifiers and the case
   passes in 286s.

Root cause: mechanism explanations have typed evidence rows and source
citations, but no canonical `MechanismStepRow` / `CapabilitySurfaceRow` that
owns both the code identity and the display sentence. The finalizer can
accidentally render a structurally grounded mechanism as abstract prose, then
depend on a late prose-density gate to repair it.

Generalization: permission/capability questions, feature gates, route
dispatch, middleware stacks, plugin registration, and config precedence all
need deterministic mechanism rows with `subject`, `relation`, `target`,
`source_line`, and `display_label`. Validators should consume those rows, and
the renderer should naturally surface at least one grounded identifier per
load-bearing mechanism block.

### E20260514-G35: Runtime Artifact Frames Are Still Being Treated Like Source Citations (`logtri_go`, PASS with 2 rejects / 1 semantic review)

User request attaches a Go panic log and asks where the panic came from.

Observed data flow:

1. Log triage extracts the runtime error
   `runtime error: invalid memory address or nil pointer dereference`, the
   observed frame `buildAnalysisIR` at old-build `internal/agent/analyzer.go:250`,
   and caller frame `ParseOutput` at old-build line 320.
2. Exploration maps those observed frames to current code anchors:
   `ParseOutput` around current line 994/1039 and `buildAnalysisIR` around
   current line 1289/1290, while explicitly noting line-number drift.
3. Finalizer first tries to patch the answer and hits citation-pool and
   label/citation alignment errors.
4. The full re-emit then puts the runtime artifact frame into the normal
   source `citations[]` pool as `file=internal/agent/analyzer.go,line=250`
   with quote text copied from the log, even though that line is not the
   current source evidence for the crash.
5. Semantic review reports an uncertainty-boundary gap but the case still
   passes, leaving a user-visible answer that visually presents an old runtime
   frame as if it were a normal current source citation.

Root cause: diagnostic artifact observations, current source anchors, and
drift mappings share the same citation carrier. The renderer needs to cite
current source lines, but runtime frames are observations with their own
artifact coordinates. When those roles collapse, old line numbers can masquerade
as source citations and validators oscillate between label alignment and
drift caveats.

Generalization: log, crash dump, trace, perf, and telemetry answers need a
separate `ArtifactObservationRef` / `DriftMappingRow` path. Artifact refs should
render as "observed frame" evidence, not as repo file citations, and source
citations should only point to current checkout lines. Root-cause verdicts
should be computed from explicit rows: observed frame, current mapped anchor,
nearest mechanism, unresolved boundary.

### E20260514-G36: Scalar Count Questions Should Not Be Forced Into Full Enumeration Tables (`s7b`, PASS with 3 rejects)

User request asks only for the exact count of distinct `Criterion Kind`
constants in `internal/analysis/criterion/grammar.go`.

Observed data flow:

1. Explorer obtains the precise scalar via command evidence:
   an unrestricted grep count is noisy (`51`), while an awk-limited const-block
   count is precise (`25`).
2. The structured handoff already contains a `total_count` fact and the exact
   25-member set.
3. Finalizer nevertheless treats the answer contract as requiring an
   `ordered_list` of all 25 constants, emits the list before the summary, and
   is rejected for block order plus missing uncertainty disclosure.
4. The question's primary deliverable is a scalar value; the full enumeration
   is support evidence and should be optional unless the user asks for members.

Root cause: exact-count answers are routed through enumeration answer shape
when a member slate exists, even if the user asks only "how many". That turns a
low-risk scalar into a large list-rendering task with extra citation,
uncertainty, and ordering failure surfaces.

Generalization: count/size/version/hash/commit scalar questions need a
`ScalarMeasurementRow` with `value`, `unit`, `method`, `scope`, and optional
supporting member set. The renderer should lead with a scalar block and keep
the member list behind a compact support/caveat lane unless the request asks
for names. This covers code counts, enum cardinality, git history counts,
dependency counts, file sizes, and runtime totals.

### E20260514-G37: Section Prose Cannot Carry Native Citation Anchors (`mr_cross_repo_compare`, PASS with semantic review)

User request asks to compare two user-named sub-repos and keep their answers
separate.

Observed data flow:

1. Multi-repo analyzer/explorer correctly preserves the two buckets
   `repo-greet-go` and `repo-tools-py`, finds each exported entry symbol, and
   emits a grounded anchor list.
2. Finalizer renders the main explanation in two `section` blocks and puts
   citations in a later "core exported identifier anchors" ordered list.
3. The section prose itself has no native item-level citation field, so semantic
   review reports `current_code_path` and `uncertainty_boundary` as declared
   but unanchored in the prose stream.
4. The answer passes, but the product surface is split: the user reads the
   actual explanation first and only later sees the citation-bearing anchor
   list.

Root cause: section blocks are useful for multi-topic structure but cannot
carry native source citations per statement. The system compensates with a
separate anchor list, which keeps structural validators satisfied but leaves
the main narrative semantically thin.

Generalization: comparison, architecture, package catalog, and multi-topic
answers need citation-bearing display rows inside each topic: either section
items with citation refs, or a typed `TopicRow` / `CatalogRow` renderer that
combines heading, prose, row labels, and citations. Main prose should not depend
on an appendix-style anchor list for its grounding.

### E20260514-G38: Universal Enumeration Without A Number Was Promoted To Hard Count (`s5b` focused replay)

User request asks for all sub-package directories under `internal/analysis/`
and their entry functions. The request uses a universal quantifier ("所有子包")
but does not declare a numeric count.

Observed data flow:

1. Analyzer/tool context can see a noisy pre-scan count for candidate
   directories.
2. The analyzer emitted `enumeration_boundary.declared_count=26` with
   `source_quote="所有子包"`.
3. `NormalizeRequestedEnumerationBoundary` accepted the boundary because the
   quote appeared in the raw request, even though the quote did not contain the
   number 26.
4. The explorer prompt then rendered "The user explicitly declared a bounded
   principal set: 所有子包 (26 item(s))", turning a noisy inferred count into a
   hard downstream requirement.
5. Precise file listing later showed the actual closed set is 25, creating a
   classic seesaw: honoring the hard boundary breaks grounded evidence, while
   honoring evidence appears to violate the hard boundary.

Root cause: `enumeration_boundary` is a hard gate but only verified quote
presence, not that the declared count was itself user-declared. A model- or
pre-scan-derived count could therefore cross from noisy guidance into
structural obligation.

Generalization: "all/every/complete" without an explicit number should produce
a `CompletenessObligation`, not an `EnumerationBoundary`. Hard declared-count
boundaries must require a precise typed signal: the source quote carries the
same explicit numeric count that will later constrain answer cardinality.

### E20260514-G39: Unsupported Relation Members Can Enter Exhaustive Closure (`s5b` focused replay)

User request still asks for all `internal/analysis` sub-package directories and
their entry functions. After G38, the analyzer no longer creates a false hard
count, but the focused replay exposed a later closure-boundary issue.

Observed data flow:

1. Explorer emitted answer-grade evidence for 24 package/function rows with
   citable definition anchors.
2. `emit_investigation_complete.aggregate_facts.member_set` carried 25 members,
   adding `subject: AnalyzeIR` even though `internal/analysis/subject` had not
   been read and no support ref or typed evidence existed for that row.
3. The pre-complete citable-member check used a local simplified member parser.
   It did not split relation-style members such as `subject: AnalyzeIR`, so the
   code-shaped right side was not recognized as needing typed support.
4. Extractor/finalizer saw two conflicting principal lanes: 24 answer-grade
   support rows and a 25-member model-authored set containing one unsupported
   relation row.
5. Finalizer tried to satisfy both, rendered the unsupported `subject` row
   without citation, then failed citation alignment. This is the same seesaw
   pattern in a different place: include the unsupported member and citation
   gates fail; drop it and aggregate count/completeness appears violated.

Root cause: pre-complete member usability did not consume the shared aggregate
relation/callable candidate parser, so relation members were not held to the
same typed-support standard as later support-plan/finalizer lanes.

Generalization: any two-axis exhaustive enumeration (`package: entry`,
`route -> handler`, `module/import`, `type: method`, `config key -> default`)
must validate both axes before closure. If a member surface contains a code-like
attribute, path, source location, or callable signature, it is not principal
answer-grade until typed evidence or a member-specific support ref backs that
same member.

## Exploration Handoff Failure Analysis

The dominant product gap is not raw retrieval. In many cases, exploration found
the needed facts, but those facts did not survive as typed, row-level carriers
that extract/finalize/render/validators could consume deterministically.

### H1: Complete Principal Sets Found, But Row Identity Is Lost

Cases: `s5a`, `u11b`, `u8b`, `qf_type_relation_loop_controller`,
`qf_relation_subagent_registry`, `u4b`, `u3b`, `u10a`, `s5b`, `u8a`,
`mr_cross_repo_compare`.

Exploration often already has the complete member set, support refs, and
category split. The downstream break happens when this becomes free-form prose
or separate anchor lists. Finalization then re-infers labels, counts, and
citations, which creates label/citation drift, count drift, and appendix-style
grounding.

Required system change: introduce a canonical `DisplayRow` family that travels
from exploration closure through extraction and answer rendering. The row must
carry `id`, `category`, `display_label`, `value/count`, `support_ref`, and
`surface_role` together so validators consume the same object the renderer
uses.

### H2: Relation / Call-Chain Facts Found, But Diagram/List Consumers Rebuild Them

Cases: `qf_sequence_analyzer_gate`, `s8a`, `qf_diagram_pipeline`,
`qf_logic_view_read_pipeline`, `u1a`.

Exploration finds call edges, branch guards, stage order, or flow nodes. Later
stages rebuild Mermaid and ordered lists from prose, so endpoints, optional
guards, and block kind can drift.

Required system change: carry `CallEdge` / `MechanismStepRow` / `DiagramRow`
from exploration into rendering. Mermaid should be generated from these rows,
not authored independently by the finalizer.

### H3: Exact Scalars And Literals Found, But Routed Through Symbol Enumeration

Cases: `s11b`, `u7a`, `s7a`, `u7b`, `s7b`, `u11a`, `m1a`, `m1b`,
`qf_imports`, `u9b`.

Exploration gets exact counts, commit filters, role targets, import paths, or
tool-name literals. Downstream shape selection then asks for answer symbols,
definition citations, or long enumerations, which adds noise to a scalar/literal
answer.

Required system change: add exact-answer carriers before symbol enumeration:
`ScalarMeasurementRow`, `LiteralReferenceRow`, `HistoryFilterResult`, and
`RoleResolutionRow`. Symbol-definition lanes should be optional support, not
the default carrier for every exact answer.

### H4: Diagnostic Artifact Roles Found, But Merged Into Source Evidence

Cases: `logtri_degraded`, `logtri_oversized`, `logtri_go`,
`logtri_rust`, `logtri_goroutine_dump`, `logtri_node`.

Log/perf triage distinguishes observed frames, detectors, current source
anchors, and drift boundaries. Later answer surfaces can merge them into one
citation list or one candidate-source count, so detector code becomes a source,
old log line numbers look like current repo citations, and low-signal logs
become repo-debug tasks.

Required system change: artifact and source references must remain different
types: `ArtifactObservationRef`, `DiagnosticOriginRow`, and `DriftMappingRow`.
Only current source anchors enter `citations[]`; artifact observations render
as observation evidence.

### H5: Negative / Absence Proofs Are Not A First-Class Handoff

Cases: `s3a`, `s3d`, parts of `u9b`.

Exploration can establish bounded absence, but downstream lacks an
`AbsenceProof` row with query, searched scope, negative pattern, nearby
non-substitutes, and confidence. The answer then invents fake repo-wide
citations or leaks validator caveats.

Required system change: make absence a typed proof artifact, not prose plus a
negative citation workaround.

### H6: Validator Gates Use Noisy Surfaces Instead Of Precise Handoff Signals

Cases: `u8a`, `u3b`, `qf_diagram_pipeline`, `mr_cross_repo_compare`, `s5b`.

Here the issue is not missing exploration data. The gate itself binds to noisy
surface clues: member-name presence in caveats, CJK concept labels treated as
code identities, diagram payload accepted under a section block, or section
prose judged unanchored because citations live in an appendix block. The same
class appears when a universal-quantifier quote is accepted as a hard numeric
enumeration boundary even though the quote does not contain the number, or when
a relation member bypasses typed-support checks because a local parser does not
recognize its code-like right axis.

Required system change: hard gates should read only precise typed fields:
explicit aggregate labels, block kind/payload invariants, row-scoped citation
refs, declared artifact/source roles, and source quotes that carry the exact
declared count. Pre-complete, support-plan, and pre-emit gates must share the
same member candidate parser. Text similarity, candidate counts, and member
mentions can guide retries, but must not fail structurally valid answers.

## Final Cluster Analysis

### C1: Typed Row / Canonical Display Boundaries

Cases: `s5a`, `u11b`, `u8b`, `qf_type_relation_loop_controller`,
`qf_relation_subagent_registry`, `u4b`, `u3b`, `u10a`, `s5b`, `u8a`,
`mr_cross_repo_compare`.

Shared gap: the system knows the principal set, but the final surface is built
from free-form labels, caveats, and separate anchor lists. Counts, labels, and
citations drift because row identity is not one typed object.

Generalized fix direction: introduce/strengthen deterministic rows for public
API entries, catalog entries, comparison metrics, change-impact files,
mechanism steps, and section/topic rows. Validators should compare row identity
and row-scoped counts, not scan global prose.

### C2: Rich Surface Kind/Payload Invariants

Cases: `qf_diagram_pipeline`, `qf_sequence_analyzer_gate`, `s8a`,
`qf_logic_view_read_pipeline`.

Shared gap: diagrams and call chains are valid evidence but can be placed in
the wrong carrier or over-expanded into unrelated nodes. The renderer and eval
consume block kind, while validation sometimes accepts payload-only intent.

Generalized fix direction: enforce kind/payload invariants at normalization and
mutation boundaries, and generate diagrams from typed call-edge rows instead of
free-form Mermaid text.

### C3: Scalar / Literal / Exact Role Resolution

Cases: `s11b`, `s1b`, `u7a`, `s7a`, `u7b`, `u11a`, `s7b`, `m1a`, `m1b`,
`qf_imports`.

Shared gap: exact answers are often known early, but the final answer shape
forces enumeration, symbol-definition, or carrier-visibility pathways that add
noise. Tool names, import paths, scalar counts, commit history, and role
locations need their own exact-literal carriers.

Generalized fix direction: route exact scalar/literal answers through typed
`ScalarMeasurementRow`, `LiteralReferenceRow`, `HistoryFilterResult`, and
`RoleResolutionRow` before general symbol enumeration.

### C4: Diagnostic Artifact / Source Role Separation

Cases: `logtri_degraded`, `logtri_oversized`, `logtri_go`, `logtri_rust`,
`logtri_goroutine_dump`, `logtri_node`.

Shared gap: runtime artifacts, detector code, current source anchors, and drift
mappings can collapse into one evidence/citation lane. That produces misleading
counts, repo-debug drift on low-signal logs, and old frames rendered as source
citations.

Generalized fix direction: introduce `DiagnosticOriginRow`,
`ArtifactObservationRef`, and `DriftMappingRow`; keep observed artifact facts,
detectors, candidates, resolved sources, and current-code verification in
separate roles.

### C5: Infra / Eval Harness / Timeout Accounting

Cases: regex false negatives (`u9b`, prior diagram regex), timeouts beyond
1800s (`u7a`, `s7a`, `logtri_rust`, `qf_type_relation_loop_controller`), and
provider EOF/stalls.

Shared gap: harness expectations and process cleanup sometimes obscure whether
the product answer is wrong, slow, or infra-limited.

Generalized fix direction: add precise timeout group cleanup, classify infra vs
semantic failures separately, and make eval expectations semantic enough to
avoid regex-only false negatives while preserving hard checks for exact answers.

## Batch Repair Plan

### Batch 0: Hard-Gate Precision And Rich-Surface Invariants

Priority: immediate. This is the smallest safe slice because it removes known
false hard failures without changing task semantics.

Scope:

- `diagram` payload invariant: `diagram != nil` must imply
  `kind=diagram`; `kind=diagram` must imply non-empty diagram payload.
- Multi-principal-set count binding: when several principal `member_set`
  facts exist, hard cardinality checks bind only to an explicit set label, not
  to member-name mentions in shared caveats.
- Preserve existing single-set behavior where member-name count binding is a
  useful precise enough shortcut.

Eval / tests:

- Unit regression for non-diagram block carrying diagram payload.
- Unit regression for multi-set caveat containing numbers for another set.
- Re-run `qf_diagram_pipeline`, `u8a`, and `s7b`.
- Add eval case `qf_multi_member_set_count_caveat`: exported API answer with
  function/type/var/Kind sets and a caveat mentioning numeric Kind counts; pass
  criteria include no cardinality reject against unrelated sets.

Implementation status:

- Added normalization and mutation-runtime invariants that reject a `diagram`
  payload unless the block itself is `kind=diagram`.
- Scoped aggregate count hard gates so multi-principal member sets require an
  explicit set label before a visible number is checked against that set.
- Upgraded aggregate support-row compilation so short display locations are
  reconciled with precise `support_ref` paths before final-answer citation
  coverage checks.
- Added focused unit regressions for these invariants.
- Added `qf_multi_member_set_count_caveat` as a focused eval for multi-set
  count-binding precision. The case intentionally excludes variables so it does
  not mask the Batch 1 public/exported API row problem.
- Verified `go test ./internal/tool`, `go test ./...`,
  `qf_diagram_pipeline`, and `qf_multi_member_set_count_caveat`.

### Batch 1: Canonical Exploration-to-Answer Row Contract

Priority: first major architecture batch. This directly addresses the largest
class of exploration-rich but downstream-thin failures.

Scope:

- Define a common row interface for exploration closure payloads:
  `DisplayRow` with stable `row_id`, `category`, `display_label`,
  `value/count`, `support_ref`, `surface_role`, and optional `attributes`.
- Materialize specific row flavors: `ExportedAPIRow`, `CatalogRow`,
  `ComparisonMetricRow`, `ImpactFileRow`, `EntryPointRow`, and `TopicRow`.
- Store these rows alongside aggregate facts in MutableState, then compile
  answer-support lanes from rows instead of prompt prose.
- Make row IDs the unit of validator comparison.

Eval / tests:

- Add `u8a_api_surface_rows`: exported API categorized by function/type/const/var
  must render category counts and row citations without global count scanning.
- Add `mr_cross_repo_compare_grounded_sections`: each user bucket must show
  citation-bearing rows in the bucket body, not only a separate anchor appendix.
- Re-run `s5a`, `u11b`, `s5b`, `u3b`, `u4b`, `u10a`.

Initial Batch 1a progress:

- Principal support planning now treats count aggregate facts with
  `value == len(members)` as complete member-row slates for downstream
  consumption. The original aggregate kind is preserved, but final-answer
  support lanes no longer lose member rows merely because exploration emitted
  `total_count` / `grouped_count` instead of `member_set`.
- The rule is deliberately narrow: count facts without members, mismatched
  counts, or partial/sample members are not promoted.
- Added a unit regression proving complete count members become principal row
  obligations with precise support refs.
- Verified `go test ./...` and re-ran
  `qf_multi_member_set_count_caveat` successfully after the row-consumption
  change. The eval still records finalizer repair loops, so citation-array
  compilation remains in Batch 1 rather than being declared solved.

Batch 1b progress:

- Moved the summary-lead block ordering invariant into the shared
  `emit_answer_document` / `emit_answer_document_patch` mutation chokepoint.
  When a renderable summary block appears after principal detail blocks, the
  runtime now deterministically moves the first summary block before the first
  renderable detail block while preserving all detail-block relative order.
- This is a structural document normalization, not a prompt repair and not a
  semantic rewrite: rows, citations, claim forms, facets, caveats, and diagrams
  remain unchanged. It removes a repair-loop class where an otherwise valid
  row/citation payload was rejected only because the lead-in summary appeared
  after a table/list.
- Added a mutation-runtime regression proving summary-first canonicalization is
  shared by full and patch-derived merged documents. The older pre-emit summary
  check remains as a defensive validator, but ordinary runtime writes no longer
  ask the model to spend a retry round on deterministic block ordering.
- Verification after the change: `go test ./internal/tool`, `go test ./...`,
  and a 1-run `qf_multi_member_set_count_caveat` eval all passed. The focused
  eval recorded `finalizer_iters=2` and no summary-order reject, while explorer
  breadth remains unsolved (`explorer_iters=13` in that run), so this closes
  only the deterministic block-order sub-gap and leaves broader row/citation
  compilation in Batch 1.

Batch 1c progress:

- Split relation member identity from source-location support for aggregate
  row display candidates. Relation members such as
  `package -> Entry (line 123)` now expose `package -> Entry` and
  `package/Entry` as valid visible row identities, while the line number stays
  available as support/citation data. Ordinary semantic decorators such as
  `New (Classifier)` or `Engine (New + Submit/Apply)` remain identity-bearing.
- The implementation lives in the shared `AnswerAggregateMemberDisplayCandidates`
  relation parser, not in an eval-specific prompt or finalizer exception. It is
  language-neutral over row surfaces and handles both English and Chinese
  source-line markers (`line`, `ln`, `行`, `第...行`), so Go, TS/ArkTS, Python,
  Java/Kotlin, Rust, C/C++, Cangjie, and other supported repomap languages use
  the same row/support boundary.
- Added unit coverage in `internal/types` for source-line decorator candidate
  expansion and in `internal/tool` for pre-emit member-set coverage when a row
  renders package/function identity and carries line proof through citation
  rather than visible prose.
- While re-running `s5b`, another exploration-to-consumption gap surfaced:
  `emit_evidence` can auto-recover a wrong line number to the exact same-file
  definition, but the repaired row remains audit/recovered until the model
  re-emits it with the exact gutter line. The mid-loop repair latch now repeats
  the structured file/line target on closure-only redirects and states that
  auto-recovery is not a completed strict-citation repair. This keeps recovered
  rows from being mistaken for consumed, line-text-grounded evidence without
  weakening citation hard gates.
- Verification so far: `go test ./internal/agent ./internal/types
  ./internal/tool` and `go test ./...` passed. Focused `s5b` eval is running
  on the current binary to measure whether the repair loop reduction follows
  the structural fix.

Batch 1d progress:

- Upgraded generic aggregate support-row compilation so a member like
  `aggregator: Aggregate (aggregator.go:132)` first strips the source-location
  support ref, matches the package/function relation against typed definition
  evidence, and then upgrades the support obligation to the full repo-relative
  path (`internal/analysis/aggregator/aggregator.go:132`). This keeps short
  display paths readable while making hard citation coverage use precise typed
  evidence.
- Added a stable-member-set merge guard at `emit_investigation_complete`:
  when two completions carry the same labeled principal `member_set`, the
  merger compares typed member-set inclusion. A later narrowed retry cannot
  overwrite an earlier accepted superset, while a later true superset still
  wins. Disjoint same-label sets remain separate instead of being collapsed.
- This is the anti-seesaw guard for G29: exploration/reconcile/finalize can add
  evidence or improve display metadata, but they cannot trade away already
  accepted complete principal rows. The rule is structural (`kind`, label,
  unit, dimensions, and member display candidates), not prompt prose.
- Added regression tests for all three directions: stable superset retained,
  current superset wins, and disjoint same-label sets stay distinct. Also added
  support-plan coverage proving short display locations upgrade from typed
  definition evidence and still satisfy full-path citations.
- Verification: `go test ./internal/tool -run TestMergeCompletionAggregateFacts`,
  `go test ./internal/types -run
  TestBuildAnswerSupportPlan_RelationMemberShortDisplayLocationUsesTypedEvidencePath`,
  and `go test ./internal/tool ./internal/types ./internal/agent` passed.
  Focused `s5b` is running on the rebuilt binary; the in-flight trace has
  already hit the previous narrowing-prone reconcile path after a complete
  25-member first closure, so this is the right validation target.

Batch 1e progress:

- The rebuilt `s5b` trace confirmed the next downstream-consumption gap: the
  25-entry `member_set` survived exploration and reconcile, but finalizer
  pre-emit still treated rows such as
  `aggregator -> New(cfg Config) *Aggregator @ internal/analysis/...:112` as
  absent when the answer rendered the same data as split fields:
  `label=aggregator`, `text=New(cfg Config) *Aggregator`, and a matching
  `citation_ref`. The problem was not missing exploration; it was a hard gate
  comparing only literal full-member strings.
- Generalized the aggregate relation parser so the right side of a typed
  member relation may be a language-neutral callable signature. It now accepts
  function/method signature surfaces across the supported read-language matrix
  (Go, Python, JavaScript/TypeScript/ArkTS, Java/Kotlin, Rust, C/C++, Ruby,
  Swift, Lua, Proto, Cangjie), including return arrows inside the signature.
  Source locations remain support refs stripped by typed parsing, not visible
  identity requirements.
- Updated pre-emit member coverage and citation alignment to consume the same
  relation candidates when the relation is split across item label/text in
  either direction (`pkg` label + signature text, or callable label + package
  text). This avoids the seesaw where forcing exact literal member strings
  would make the answer less usable while still leaving citations as the hard
  support check.
- Added regressions for signature relation parsing across all supported
  languages and for `member_set` coverage / relation-answer shape on
  signature+support-ref split rows.

Batch 1f progress:

- The next `s5b` replay exposed a true seesaw: the same left-axis principal
  set (the 25 `internal/analysis` subpackages) could arrive as two relation
  `member_set` facts with different labels and different right-side entry
  candidates. One set carried a complete support-ref array; the older alternate
  had the same left axis but no complete support refs. Treating both as
  principal forced finalizer to satisfy conflicting "single entry function"
  values for the same subpackage.
- Generalized principal relation-set selection: when alternate relation
  `member_set` facts have the same exact left-axis set, a relation set with
  complete per-member support refs remains principal and unsupported alternates
  over that identical left axis become support-only. Fully supported relation
  sets over the same left axis are kept, so legitimate multi-attribute answers
  are not collapsed.
- This keeps the hard gate on precise signals: identical typed left-axis set
  plus complete parsed support-ref coverage. It does not compare vague label
  wording or ranker scores, so it avoids replacing one prompt-shaped failure
  with another.

Batch 1g progress:

- The next focused `s5b` replay exposed a hard-boundary precision gap before
  exploration row consumption: analyzer emitted
  `enumeration_boundary.declared_count=26` with source quote `所有子包`, even
  though the user did not declare a number. That noisy count then became a hard
  prompt obligation and conflicted with precise file listing evidence showing
  25 child package directories.
- Tightened `NormalizeRequestedEnumerationBoundary` so a boundary must be
  grounded in a quote that both appears in the current request and carries the
  same explicit decimal count. Universal/no-number phrasing such as "all",
  "every", or `所有` stays in `CompletenessObligation` and cannot become a
  cardinality hard gate through an analyzer pre-scan guess.
- This is intentionally conservative and language-neutral: it does not add a
  keyword table or localized counter parser. If a count is not present as an
  explicit numeric token in the user quote, the system prefers full-coverage
  guidance plus evidence-derived rows over a fabricated hard boundary.
- Added type-level and tool-level regressions proving countless universal
  quotes are rejected while explicit numeric quotes still persist.

Batch 1h progress:

- The rerun after Batch 1g confirmed a later anti-seesaw gap: the accepted
  closure carried 24 answer-grade package/function support rows plus a
  25-member aggregate set where `subject: AnalyzeIR` had no typed evidence,
  no support ref, and no read source file. Finalizer then tried to satisfy the
  unsupported row and failed citation alignment.
- Reused the shared `AnswerAggregateMemberDisplayCandidates` parser inside the
  `emit_investigation_complete` pre-complete member usability check. Relation
  members such as `package: Entry`, `type -> method`, and callable-signature
  rows now expose their code-like axis before closure, so unsupported right
  sides require typed evidence or member-specific support refs just like they
  do in support-plan and pre-emit validation.
- This removes another seesaw source by making closure, support planning, and
  final-answer gates consume the same member-candidate interpretation. A
  model-authored aggregate member can no longer be accepted as principal in
  pre-complete and then become uncitable in finalizer.
- Added a pre-complete regression for `aggregator: Aggregate` plus unsupported
  `subject: AnalyzeIR`; the tool now keeps investigation open and asks for
  member support instead of shipping a contradictory 25-row closure.

Batch 1i progress:

- The next focused `s5b` replay showed the anti-seesaw issue moving one stage
  earlier: after the fake numeric boundary was fixed, analyzer emitted a valid
  typed category-enumeration principal lane (`intent=enumerate`,
  `question_kind=enumeration`, `is_category_enumeration=true`, 25 member
  entities) but omitted `completeness_obligation`. Downstream treated the
  member lane as soft search context, so extractor/finalizer shipped a
  lower-bound list instead of a complete principal set.
- Generalized the handoff boundary: non-relational category enumerations with
  multiple analyzer-emitted principal members now require a structured
  `member_set` handoff even when the analyzer omitted an explicit all/count
  obligation. This consumes typed analyzer fields and a precise multi-member
  count; it does not scan raw request text for localized enumeration words.
- Updated R3 must-include pinning to let the same typed principal lane carry
  lowercase package/module/file-stem members across the supported language
  matrix (`aggregator`, `com.example.api`, `react-dom`, `@scope/pkg`,
  `foo::bar`, `packages/core`, etc.). Relation-shaped enumerations remain
  excluded because their entity list mixes relation targets and helper
  surfaces until exploration proves the qualifying members.
- Restored the analyze-stage evidence-lite red line in both prompt and runtime:
  `grep(files_only=false)` is rejected in StageAnalyze regardless of legacy
  ClassificationGrep trigger/config state. This keeps source-line content,
  function signatures, and tool-output counts out of hard classification
  decisions; explore remains the stage that reads line-level evidence.
- Strengthened `s5b.case` so tail packages (`subject`, `sourcemix`,
  `stopcond`) and their entry functions are eval-gated, catching partial
  lower-bound answers that only list common middle packages.
- Focused `s5b` replay on the current binary passed
  (`eval/results/s5b-20260514-190353`, PASS). The trace confirms the new
  positive path: analyze emits in 2 iterations without line-level grep, explore
  reads the tail packages, `aggregate_facts.member_set` carries 25 members with
  exact support refs, and finalizer renders all 25 rows. Residual repair cost is
  tracked as G42 rather than treated as solved by prompt pressure.

Batch 1j progress:

- Implemented the G42 deterministic support-row bridge inside
  `emit_investigation_complete`: accepted `read_file` gutter output is parsed
  into file/line/text rows, then relation members such as
  `findings_validator → Validate` can receive a generated
  `Member @ path:line` support ref when the line is definition-like and the
  file path matches the relation's left axis.
- The guard is intentionally precise and cross-language: relation-left path
  matching covers Go package/file stems, Java dotted packages, scoped npm
  package paths, C++ namespace separators, monorepo package paths, and hyphen
  modules. Same-name functions in a different left-axis path do not satisfy
  the member.
- The in-flight `s5b` replay (`eval/results/s5b-20260514-192600`) confirmed the
  completion-stage support gap moved forward, then exposed G43 at finalization:
  a composite relation member (`perftriage → MergePerfBundles +
  CorroborateStallFiles`) conflicted with split per-function citation rows.
- Implemented the G43 anti-seesaw rule in pre-emit validation: composite
  same-left relation members can be satisfied by structured split rows only
  when each explicit right-side target appears under that same left axis.
  Rows under another package/module left axis remain rejected.
- Added focused positive/negative unit coverage for composite relation split
  rows, plus the read-file support-row compiler and cross-language path
  matching tests. `go test ./internal/tool ./internal/types ./internal/agent`
  passed.
- Full `go test ./...` passed.
- Fresh focused `s5b` replay passed on the rebuilt binary
  (`eval/results/s5b-20260514-194231`, PASS). Compared with the stopped
  `192600` run, the G43 finalizer loop did not recur: `emit_answer_document`
  accepted the 25-row answer. Residual cost remains high
  (`explorer_dispatches=2`, `explorer_iters=23`, `midloop_inject=13`,
  `semantic_quality_dispatches=2`) because the first exploration closure still
  under-materializes line-grounded evidence and the self-consistency reviewer
  raised the G44 sortedness false positive.

### Batch 2: Exact Answer Lane Before Symbol Enumeration

Priority: second major architecture batch. This reduces latency and long-list
failure surfaces for scalar/literal tasks.

Scope:

- Add `ScalarMeasurementRow` for counts, versions, hashes, sizes, and command
  measurements.
- Add `LiteralReferenceRow` for imports, config keys, tool names, route strings,
  and string-literal APIs.
- Add `HistoryFilterResult` and `RoleResolutionRow` for git/history and
  role-locate questions.
- Shape selection should choose scalar/literal first when the user asks "how
  many", "what exact value", "which literal", or "where is the role".

Eval / tests:

- Add `s7b_scalar_only_count`: exact count answer should lead with a scalar and
  must not require a full 25-item list.
- Add `qf_import_literal_references`: import-path lists must use literal
  reference rows, not answer-symbol definition rows.
- Add `m1_tool_name_literal`: explicit `emit_answer_document` question must
  keep literal tool names visible without carrier-leak rejection.

Initial Batch 2a progress:

- Added typed surface inheritance for aggregate-backed literal/reference rows.
  When a `member_set` entry is matched to same-line typed evidence whose
  `ClaimForm` uses display labels (`import_edge`, `text_reference_fact`,
  precedence/external observation), the support lane now keeps the aggregate
  member as a display-label principal row instead of treating code-shaped
  strings such as import paths as symbol definitions.
- This consumes exploration-stage information directly: explorer
  `anchor_kind=import` evidence plus `aggregate_facts.member_set` now reaches
  extractor shape selection as non-symbol principal evidence, so extractor no
  longer needs to call `emit_answer_symbol` for import-reference literal
  enumerations.
- Added a tool-layer guard for the same typed decision: if the extractor still
  calls `emit_answer_symbol` in a dispatch whose principal answer is already
  covered by non-symbol typed support lanes, the tool now no-ops without
  mutating the answer-symbol slate. That prevents a stale static tool affordance
  from turning display-label rows back into rejected symbol-definition attempts.
- The implementation is language-neutral over `AnchorImport` / `ClaimForm`
  rather than Go import syntax. It covers import/use/require-style references
  across the supported language matrix whenever the grounder has produced typed
  import evidence.
- Added regression coverage in support-plan and extractor prompt tests, plus
  `qf_import_literal_references` eval to require the full literal import-path
  set and guard against symbol-definition routing regressions.

Verification:

- `go test ./internal/types ./internal/agent ./internal/tool`
- `go test ./...`
- `bash eval/run.sh eval/cases/qf_import_literal_references.case 1`
  (`eval/results/qf_import_literal_references-20260514-163355`,
  PASS; extractor_iters=1, finalizer_iters=1, no repair lines)
- `bash eval/run.sh eval/cases/qf_imports.case 1`
  (`eval/results/qf_imports-20260514-163822`, PASS; typed support lane shows
  import rows as `claim_form=import_edge`, `member_surface=display_label`)

Initial Batch 2b progress:

- Started the scalar-count side of the same exact-answer lane. For count-only
  scalar requests (`is_count_question=true`, `is_scalar_answer=true`,
  `intent=return_value` / numeric subject, and no category/relation/history/
  diagnostic/cross-component predicate), a `member_set` attached to the
  aggregate count is now treated as support evidence for the scalar rather than
  as a required user-visible principal list.
- The cardinality gate remains active: visible count claims still have to match
  the support `member_set` cardinality. The change only removes the accidental
  requirement to list every member when the user asked for "how many", not for
  names/locations.

Verification:

- `go test ./internal/tool`
- `bash eval/run.sh eval/cases/s7b.case 1`
  (`eval/results/s7b-20260514-164411`, PASS; finalizer_iters=1, no repair
  lines; answer no longer required a 25-item principal member list)

Initial Batch 2c progress:

- Removed the pre-emit carrier-name visibility gate that scanned rendered
  answer prose and `RawRequest` to decide whether terms such as
  `citation_ref`, `emit_answer_document`, or `AnswerDocumentV2` should hard
  reject. This was a red-line risk and a direct source of the G8/G11
  tool-literal vs carrier-concealment oscillation.
- Kept structural gates that do not inspect model prose: observation-only
  runtime answers still reject current-repo `citations[]`, and exact absence
  still requires typed negative-scope citations with `negative_pattern`.
  Artifact helper paths and inactive-sub-repo disclosures need a future typed
  disclosure field before they can be hard-gated again; until then they stay in
  prompt guidance rather than keyword control flow.
- Expanded the answer-row `candidate_role` enum into exact scalar/literal and
  role-disambiguation lanes: `tool_name`, `config_key`, `route`, `import_path`,
  `literal_value`, `commit_hash`, `budget_cap`, `attempt_counter`, and
  `guard_condition`. This gives G1/G5/G8/G12/G25-style exact-answer work a
  shared row-level carrier instead of forcing role intent into prose.
- Added pre-emit regression tests proving rendered carrier words, explicit
  tool-name rows, and multi-repo absence wording are no longer accepted/rejected
  by answer-text or request-text keyword scans.

Verification:

- `go test ./internal/tool ./internal/types ./internal/agent -run 'TestRunPreEmitChecksDoesNotKeywordMatchRenderedCarrierTerms|TestRunPreEmitChecksAllowsTypedToolNameRowsWithoutRawRequestScan|TestRunPreEmitChecksDoesNotKeywordMatchMultiRepoAbsenceDisclosure|TestPreCheckRuntimeObservationRepoContaminationRejectsRepoCitationsOnly|TestEmitAnswerDocumentSchema_CandidateRoleEnumMatchesTypes|TestEmitAnalysis_Consistency'`

Focused `m1b` replay after Batch 2c:

- `bash eval/run.sh eval/cases/m1b.case 1`
  (`eval/results/m1b-20260514-210308`, PASS).
- Residual G50 found despite PASS: analyzer initially carried a stale
  `emit_answer` guess, explorer corrected it to `emit_answer_document`, but
  extractor still treated the two exact tool-name sub-topics as an anchor
  skeleton and forced `emit_answer_symbol`. The run passed only after
  symbol-slate repair (`extractor_iters=5`, `midloop_inject=8`).

Initial Batch 2d progress:

- Tightened `AnswerSemanticView.AllowsAnchorSkeleton`: multi-topic architecture
  explanations still get anchor skeletons, but multi-topic exact scalar/literal
  lookups do not. The typed signal is `RequestModel.Predicates.IsScalarAnswer`
  or the existing scalar-source-literal lookup classifier, not any wording in
  the question or model answer.
- This keeps `emit_*` tool names, config keys, routes, import paths, commit
  hashes, and other exact literal answers in the scalar/literal lane rather
  than forcing them through `emit_answer_symbol` definition-line validation.

Verification:

- `go test ./internal/types ./internal/agent -run 'TestAllowsAnchorSkeleton|TestExtractor_BuildPrompt_MultiTopicScalarLiteralSkipsAnswerSymbols'`
- `go test ./...`

Focused `m1b` replay after Batch 2d:

- `bash eval/run.sh eval/cases/m1b.case 1`
  (`eval/results/m1b-20260514-211006`, PASS).
- Seesaw check: extractor no longer forced a symbol slate for the exact
  tool-name sub-topics. `extractor_iters=1`, `finalizer_iters=1`,
  `repair_plan_lines=0`, `repair_exec_lines=0`, and there were no
  `missing_emits` repairs. Remaining `emit_answer_symbol` mentions in the log
  are generic tool instructions/source evidence, while the dispatch carries
  `does NOT require emit_answer_symbol`.

Initial Batch 2e progress:

- Closed the G1/G5/G12 positive-role side of the scalar/literal lane. Batch 2c
  introduced row-level `candidate_role`, but G1 still lacked a required
  upstream carrier that said which positive role must be present. Two focused
  `s11b` replays passed while the profile was optional, but their logs showed
  `answer_role_profile` was omitted; that meant the success was not yet a
  durable structural contract.
- Added `AnswerRoleProfile` to `RequestModel` and bumped the AnalysisIR version
  to `v13`. `emit_analysis` now requires an `answer_role_profile` object on
  every call. The inactive form carries `is_role_binding_requested=false`; the
  active form carries `required_candidate_roles[]`, `source_quotes[]`, and
  confidence. Source quotes are validated only at the analyzer emit boundary as
  exact current-request provenance, parallel to the existing field-value and
  exclusion carriers.
- Propagated active `required_candidate_roles` into `AnswerSemanticView` as
  `RequiredCandidateRoles`. The finalizer receives a "Typed Answer Role
  Contract" section that instructs it to put the enum on principal
  scalar/list/table rows, including one-row `items[]` anchors for scalar
  answers whose literal is also in `block.text`.
- Added pre-emit and post-emit checks that compare only typed fields:
  `RequestModel.AnswerRoleProfile.RequiredCandidateRoles` against principal
  `emit_answer_document.blocks[].items[].candidate_role`. These checks do not
  inspect `RawRequest`, model answer prose, rendered text, or term frequency.
- Seesaw guard: this complements `answer_exclusion_policy` rather than
  replacing it. Exclusion says which row roles must stay out; role profile says
  which row roles must be present. Both consume the same candidate-role enum,
  so a future exact scalar role can be added once in the enum/profile lane
  instead of adding per-case validators.

Verification:

- `go test ./internal/types ./internal/tool ./internal/orchestrator ./internal/agent ./internal/skill -run 'TestBuildAnswerSemanticView_RequestedCandidateRolesPropagated|TestAnalysisIR_VersionConstant|TestEmitAnalysisSchemaIncludesAnswerRoleProfile|TestEmitAnalysis_Execute_(PersistsAnswerRoleProfile|RejectsUngroundedAnswerRoleProfile|RejectsMissingAnswerRoleProfile)|TestPreCheckRequiredCandidateRoles_UsesTypedItemRoles|TestRunTypedAnswerRoleProfileCheck|TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedCandidateRoles|TestAnalysisSkill_CurrentQuestionPrimacy_NamesEveryIntentField|TestEmitAnalysisSchemaMatchesContract'`
- `go test ./...`
- `bash eval/run.sh eval/cases/s11b.case 1`
  (`eval/results/s11b-20260514-214324`, PASS). The successful analyzer emit
  logged `required_roles=budget_cap`; semantic-view traces carried
  `required_candidate_roles=1`; the finalizer prompt rendered the typed role
  contract; and the final `emit_answer_document` principal item labeled
  `MaxRetriesPerStage` with `candidate_role="budget_cap"`.
- Earlier optional-carrier replays are retained for audit:
  `eval/results/s11b-20260514-213007`, `eval/results/s11b-20260514-213322`,
  and `eval/results/s11b-20260514-213639`. They passed, but they are not counted
  as the final structural verification because they started before
  missing-profile rejection was active in the runtime.

Initial Batch 2g progress:

- Closed the G2 mechanism-anchor side of the exact-answer lane. Mechanism
  explanation answers now carry required endpoint anchors through
  `AnswerSemanticView.RequiredMechanismAnchors`, compiled from typed analyzer
  lanes and kind-bearing contract terms.
- The finalizer receives these endpoints as a typed contract and must preserve
  them in structured answer fields. The pre-emit check compares only structured
  fields (`items[].label`, block titles, diagram `edge_anchors` endpoints), so
  it does not inspect the raw request, model prose, rendered prose, or keyword
  frequency.
- The design is language-neutral: code symbols, tool-name literals, and file
  stems are represented as contract term kinds rather than Go-specific syntax.
  Phrases stay soft guidance unless they are represented by an eligible typed
  anchor kind.
- Seesaw guard: scalar/literal, count, category-enumeration, relation,
  config-query, and return-value lanes are excluded because they already use
  stronger principal carriers (`candidate_role`, aggregate scalar/member rows,
  relation rows). This prevents mechanism anchors from pulling exact scalar
  answers back into broad architecture surfaces.

Verification:

- `go test ./internal/types -run 'RequiredMechanismAnchors|AnswerSemanticView'`
- `go test ./internal/tool -run 'RequiredMechanismAnchors'`
- `go test ./internal/agent -run 'RequiredMechanismAnchors|RequestedCandidateRoles'`
- `go test ./internal/types ./internal/tool ./internal/agent`
- `bash eval/run.sh eval/cases/s1b.case 1`
  (`eval/results/s1b-20260515-051031`, PASS but
  `required_mechanism_anchors=0`; treated as insufficient structural proof)
- `bash eval/run.sh eval/cases/s1b.case 1`
  (`eval/results/s1b-20260515-051535`, PASS with
  `required_mechanism_anchors=1`)

Initial Batch 2h progress:

- Closed the G5 source-literal evidence side of the exact-answer lane. Added
  `anchor_kind=string_literal` and `claim_form=literal_value_fact` as typed
  carriers for facts whose ground truth is a source-code literal value rather
  than a symbol definition.
- The grounder now validates these anchors with a source-line literal parser.
  It recognizes common single-line string/char/raw/template literal forms used
  across the repository's supported language matrix, while rejecting ordinary
  identifier matches and comment-only matches. This makes the hard signal the
  cited source literal span, not a grep hit or answer/request keyword.
- Final-answer shape compilation and evaluator guidance now allow
  `literal_value_fact` anywhere exact literal rows are structurally valid:
  enumeration rows, role/scalar lookup rows, and config precedence rows. That
  unifies tool names, route strings, config keys, enum values, provider IDs,
  protocol names, and similar source literals under one lane.
- Seesaw guard: this change does not relax carrier concealment by scanning
  visible text. User-visible exact literals must be backed by typed evidence
  rows, while internal carrier names remain ordinary implementation details
  unless a typed row makes them the requested source literal.

Verification:

- `go test ./internal/types ./internal/tool/ground ./internal/tool ./internal/agent -run 'ClaimForm|StringLiteral|EmitEvidence_AcceptsStringLiteral|AnswerDocument|GroundItem_StringLiteral|Schema'`
- `go test ./internal/types ./internal/tool ./internal/agent`
- `make`
- `bash eval/run.sh eval/cases/m1_tool_name_literal.case 1`
  (`eval/results/m1_tool_name_literal-20260515-054222`, PASS;
  `extractor_iters=1`, `finalizer_iters=1`, no repair lines, no
  semantic-quality concerns)
- `bash eval/run.sh eval/cases/m1b.case 1`
  (`eval/results/m1b-20260515-054352`, PASS; `extractor_iters=1`,
  `finalizer_iters=1`, no repair lines, no semantic-quality concerns)

### Batch 3: Relation / Diagram Generation From Typed Edges

Priority: third architecture batch. This handles call chains, sequence diagrams,
and architecture flow.

Scope:

- Introduce `CallEdge` / `MechanismStepRow` with `from`, `to`,
  `relation_kind`, `guard`, `optional`, `source_line`, and `display_order`.
- Generate Mermaid and ordered-list call chains from the same rows.
- Use branch guards as edge attributes, not free-form list addenda.

Eval / tests:

- Re-run `qf_sequence_analyzer_gate`, `s8a`, `qf_logic_view_read_pipeline`,
  `u1a`.
- Add an eval asserting an explicit diagram request must render a visible
  Mermaid block and include expected stage/call nodes.

### Batch 4: Diagnostic Artifact / Source Separation

Priority: fourth architecture batch because it affects log/perf tasks and
requires renderer contract changes.

Scope:

- Add `ArtifactObservationRef`, `DiagnosticOriginRow`, and `DriftMappingRow`.
- Keep detector/parser, observed frame, candidate source, resolved source, and
  excluded candidate roles separate.
- Render artifact observations outside `citations[]`; current source citations
  remain current-checkout only.
- Add a deterministic `NoDiagnosticSignal` short path for empty/placeholder
  artifacts.

Eval / tests:

- Add `logtri_go_old_frame_drift`: old-build frame must render as observed
  artifact, not as current source citation.
- Add `logtri_oversized_detector_not_source`: detector regex cannot count as a
  panic source.
- Add `logtri_placeholder_no_signal`: placeholder logs must short-circuit.

### Batch 5: Infra, Timeout, And Eval Harness Reliability

Priority: continuous cleanup in parallel with product batches.

Scope:

- Enforce process-group cleanup so timeout seconds do not exceed configured
  wall-time by hundreds of seconds.
- Classify provider EOF/stall as infra retries distinct from semantic retries.
- Replace brittle regex-only expectations where the answer is semantically
  correct but phrasing differs.

Eval / tests:

- Add harness tests for timeout process cleanup.
- Update `u9b` expectation to accept semantically equivalent per-item wording
  while preserving exact required concepts.
- Track reject counts and stage iterations as regression metrics, not only
  PASS/FAIL.

Initial Batch 5a progress:

- Replaced host-dependent timeout behavior with one deterministic process-group
  runner in `eval/runner_lib.sh`. On hosts with Python 3, eval workers now run
  the case command in a fresh session, enforce the configured wall-time
  deadline, send TERM to the whole process group, wait a short grace period,
  and then KILL the group. GNU `timeout` / `gtimeout` are now fallback-only and
  use `-k 10` when Python is unavailable.
- Added a runner contract test that starts a timed-out shell with a background
  grandchild. The test waits longer than the grandchild's write delay and fails
  if the marker file appears, proving timeout cleanup reaches beyond the
  immediate parent shell.
- Seesaw guard: this does not alter eval verdict matching, product prompts,
  LLM stage budgets, or case expectations. It only makes worker lifetime and
  parallel-slot accounting match the configured timeout boundary.

Verification:

- `bash eval/runner_lib_test.sh`

Batch 5b progress:

- Implemented the G16 typed error-granularity lane end to end. Analyze now
  emits the always-present `error_granularity_profile` object, active only for
  failure-scope questions. Final answer decision blocks can carry the typed
  `error_granularity_verdict` enum, and renderer surfaces the canonical token
  before the natural-language decision text.
- Added specificity protection for contrasted alternatives. If the current
  request asks between explicit options such as per-item rejection and whole
  batch failure, downstream checks accept only those typed options or
  `not_enough_evidence`, not a broader umbrella verdict.
- Updated `u9b` to assert the canonical typed token while preserving the
  original evidence concepts.
- Recorded G53 as the next anti-seesaw follow-up: error-granularity questions
  with contextual quantities still need a typed conflict resolver so scenario
  counts do not activate enumeration-family obligations.

Verification:

- `go test ./internal/types ./internal/tool ./internal/agent ./internal/orchestrator ./internal/skill -run 'ErrorGranularity|TestEmitAnalysisSchemaIncludesErrorGranularityProfile|TestEmitAnalysisSchemaMatchesContract|TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersErrorGranularityContract|TestAnalysisSkill_RequiredFieldsEnumeratedEverywhere|TestAnalysisSkill_CurrentQuestionPrimacy_NamesEveryIntentField'`
- `go test ./...`
- `make`
- `bash eval/run.sh eval/cases/u9b.case 1`
  - `eval/results/u9b-20260514-222125`
  - PASS

Batch 5c progress:

- Fixed G53 by making failure-scope verdict questions an explicit typed-lane
  conflict case. Active `error_granularity_profile` plus absence of typed
  count/category/relation answer predicates keeps scenario counts contextual:
  they no longer become `enumeration_boundary`, R1 multi-subject obligations,
  enumeration scenario/family routing, or enumeration semantic-view compile
  pressure.
- Fixed the follow-on G54 support-lane conflict. A required principal
  `decision` block with `error_granularity_verdict` now has a legal principal
  evidence route when the typed error-granularity profile is active. Other
  generic answers keep the original principal evidence block policy.
- Fixed the post-rebase G55 classifier variant. Failure-scope verdict questions
  classified as no-attachment `root_cause` now route through the same generic
  typed decision lane; attached log/perf diagnostics keep the root-cause family.
- Anti-seesaw check: the resolver is shared by emit-analysis, amplifier,
  compiler scenario, analyzer reconciliation, facet family, and support-plan
  compile. The fix is not u9b-specific and does not infer anything from user
  wording or answer prose.

Verification:

- `go test ./internal/types ./internal/analysis/amplifier ./internal/analysis/compiler ./internal/agent ./internal/tool ./internal/skill -run 'ErrorGranularity|FailureScope|R1_NoFire_ErrorGranularity|ResolveQuestionFamily_ErrorGranularity|CompileGeneric_ErrorGranularity|ReconcileScenario|InferScenario|SuppressesContextualEnumeration|AnalysisSkill'`
- `go test ./internal/types ./internal/orchestrator -run 'ErrorGranularity|PrincipalEvidenceSupportLane|SupportPlan|PrincipalSupportBlockKind|FacetCoverage'`
- `bash eval/run.sh eval/cases/u9b.case 1`
  - `eval/results/u9b-20260514-232523`
  - PASS (`tool_read_file=2`, `explorer_iters=5`, `finalizer_iters=2`,
    `repair_exec_lines=0`)
