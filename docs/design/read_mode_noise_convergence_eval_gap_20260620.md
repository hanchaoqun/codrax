# Codrax Read Mode Noise And Convergence Eval Gap Plan

## Scope

This document records the system gaps exposed by the 2026-06-20 read-mode
representative eval batch. Noise reduction is the highest-priority user pain
because it directly causes repeated "verification not stable enough" loops and
large model contexts, but the repair scope is deliberately broader: typed
localization, proof coverage, schema ergonomics, handoff preservation, relation
support, trace causal projection, eval watchdogs, prompt/tool-surface hygiene,
status-card clarity, and final-answer evidence preservation all need phased
work.

The target is a generalized commercial design, not case-by-case prompt patches.
Hard gates must consume typed artifacts only: request-model fields, aggregate
facts, relation axes, support refs, read ranges, trace rows, and structured
tool results. User prose, model rationale, visible thinking, and rendered
summaries remain soft context only.

## System Gap Classes

| Class | Priority | Why it matters | Representative gaps |
| --- | --- | --- | --- |
| Noise and convergence debt | P0 | Causes repeated post-complete retries, stale "verification not stable" status, high token use, and unnecessary model turns. | RNE-C1, RNE-C4, RNE-C6, RNE-C13, RNE-C14, RNE-C40, RNE-C50, RNE-C51 |
| Typed localization and proof coverage | P0 | Determines whether read and write tasks investigate the right owner, prove the active lane, and avoid answering or patching from the wrong source surface. | RNE-C8, RNE-C16, RNE-C17, RNE-C20, RNE-C21, RNE-C23, RNE-C32, RNE-C36, RNE-C41, RNE-C44, RNE-C45 |
| Handoff and final-answer fidelity | P0 | Preserves rich exploration evidence into extractor/finalizer/report consumers without exposing executable repair instructions to the wrong stage. | RNE-C5, RNE-C10, RNE-C11, RNE-C22, RNE-C26, RNE-C40 |
| Prompt/tool-surface hygiene | P0 | Prevents hard-ish scheduling, authority prompts, or tool decisions from being driven by raw user words, model prose, visible thinking, or unavailable-tool directives. | RNE-C27, RNE-C30, RNE-C31, RNE-C61 |
| Performance and observability | P0/P1 | Makes slow tool execution, schema repair loops, context assembly, and eval timeouts diagnosable and bounded. | RNE-C7, RNE-C12, RNE-C15, RNE-C24, RNE-C39, RNE-C42, RNE-C43, RNE-C47, RNE-C48, RNE-C49, RNE-C50, RNE-C51 |
| Cross-language source inventory | P1 | Avoids Python-only or Go-only fixes by making supported-language scope, parser fallback, and absence proof language-neutral. | RNE-C9, RNE-C19, RNE-C23, RNE-C32, RNE-C33, RNE-C37, RNE-C38, RNE-C44, RNE-C46, RNE-C49, RNE-C50 |

## Eval Batch Summary

Results root:
`eval/results/goal-stagehandoff-20260620`.

| Case | Result | Key signals | System gap |
| --- | --- | --- | --- |
| `qf_relation_subagent_registry` | killed after runaway | One branch accepted the `explorer` answer, while another repeatedly downgraded relation `member_set` support. Logs also promoted and demoted many unrelated tool-registration Resolution Chains. | Relation support and chain evidence lack a low-delta convergence boundary. Noise can keep a solved one-member lookup alive. |
| `qf_architecture` | pass, slow | 171s, 8 `repo_map`, 49k estimated context tokens, 3 answer-contract violations. | Broad architecture questions need compact typed inventory views, not raw evidence floods. |
| `sr_cpp_virtual_chain` | pass, slow | 250s, 22 reads, 20 explorer iterations, 5 dispatches, 40 answer-chain lines. | Cross-language call-chain answers still over-read and over-promote chains. |
| `arkts_repomap` | timeout | Model proved no real ArkTS entries, then bounced between `absence`, `negative_observation`, `scalar_value`, and required `member_set`. | Empty exhaustive sets are not first-class. The contract requires `member_set`, while type validation rejects `members=[]`. |
| `trace_query_wakeup_causal_io_chain` | fail | 22 `trace_query` calls and 55k estimated context tokens, but final answer dropped the intermediate network/cookie path-role facet. | Trace causal-path observations are collected but not projected into a compact final-answer obligation. |
| `read_combo_log_current_code_dimensions` | timeout | Completed initial exploration, then re-entered more exploration after "verification not stable enough" and reached extraction near the timeout. | Mixed runtime-log plus current-source answers need better typed proof coverage and low-delta completion semantics. |

Visible `<think>` content in REPL logs is expected for user transparency and is
not a bug. The gap is that executable repair/navigation instructions and noisy
evidence surfaces can enter the wrong consumer stage.

## Latest Representative Refresh

Latest refresh:
`eval/convergence_audit_summary_20260620_batch3.md`.

| Case | Result | Key signals | Manual audit |
| --- | --- | --- | --- |
| `qf_relation_subagent_registry` | pass | `read_file=11`, `repo_map=2`, `explorer_iters=19`, `midloop=8`, about 69k context tokens in the run log. | Correct final answer, but still too much support-context noise for a one-symbol relation lookup. RNE-C1/RNE-C4/RNE-C6/RNE-C12 remain open. |
| `qf_architecture` | pass | `read_file=2`, `repo_map=2`, `explorer_iters=4`, one explorer dispatch. | Correct enough, but broad architecture answers still need a compact typed inventory/relevance budget rather than large prompt surfaces. |
| `read_combo_log_current_code_dimensions` | fail before Batch N refinement | `read_file=0`, `repo_map=0`; analyzer emitted required `current_key_code` plus artifact-citation policy. | Current-source obligation was lost because artifact citation external-only/exclude drift collapsed the turn to observation-only. Fixed by the Batch N refinement in this ledger; residual performance and final-answer patch rounds remain tracked separately. |
| `trace_query_wakeup_causal_io_chain` | pass | `trace_query=4`, no source reads. | Trace lane is now using the right tool and no longer demands source navigation, but trace_query throughput and status-card clarity stay under RNE-C12/RNE-C22 follow-up. |
| `arkts_repomap` | fail | `read_file=4`, `repo_map=0`, `list_files=1`, `explorer_iters=16`, `midloop=9`. | The run proved absence with grep/list/read instead of typed source inventory, missed the in-repo ArkTS fixture/corpus surface, and shows source inventory is still advisory rather than an executable localization authority. RNE-C23 and RNE-C32 cover this as a cross-language gap. |
| `sr_cpp_virtual_chain` | pass with warning | `repo_map=1`, `read_file=4`, `contract_warning=7`. | Functional answer was correct, but citation repair/contract telemetry remains noisy after success. RNE-C15 and Batch M must split blocking defects from repaired/audit-only schema drift. |

Focused Batch N regression rerun:
`eval/results/read_combo_after_lane_fix_20260620/read_combo_log_current_code_dimensions-20260620-105809`
passed with `read_file=5`, `repo_map=1`, `investigation_complete_calls=1`,
and current-source citations. It still took 242s with about 66k context tokens
and one `answer_richness_facet_coverage` contract warning, so the lane fix is
not a performance/noise fix by itself.

Six-case U4/U5 refresh:
`eval/results/readmode_rep_batch_u4_u5_20260620_summary.md` passed all six
representative cases after source-inventory slate prioritization. Manual audit
still found commercial gaps: `arkts_repomap` answered correctly but appended
duplicate system member supplements, source-localization/repo_map/localizer
audit tables, and a generic degraded-floor caveat; `trace_query` remained
source-tool free but still had one unavailable-tool attempt and multiple
completion retries; `read_combo_log_current_code_dimensions` and
`qf_architecture` passed but used high context/iteration budgets and contract
repair loops. These observations are tracked below as RNE-C40 through RNE-C43
and are not limited to "noise" fixes.

U9a source-inventory hotspot audit:
the remaining source-inventory failures collapsed from broad pipeline drift into
one structural cluster. `SourceInventoryObservation.SourceClasses` and the
shared `SourcePathRole` classifier already exist, and current HEAD computes the
class universe from git-tracked files. The open correctness gap was that
absence/final-answer gates still treated a normal `scope=negative` citation as
enough even when the typed source-class universe said repo-owned production,
fixture, corpus, third-party, vendor, generated, test, or documentation source
classes remained open. This is now handled as RNE-C53's final hard-consumption
slice; RNE-C47/RNE-C48 keep tracking the separate execution-budget,
wall-clock/cancellation, and resumable-pagination kernel work.

Post-gate ArkTS representative rerun:
`eval/results/arkts_repomap-20260620-172346` failed only the harness string
check `missing:@Component`; manual audit judged the final answer functionally
correct for the user question because it listed all 4 `@Entry` page structs and
2 `@Builder` fragments with file paths and line anchors. The run still exposed
a real system gap: `repo_map(view=source_inventory, roles=[function,method])`
returned zero member rows for ArkTS decorator-bearing corpus files, so the
answer relied on `read_file` evidence and then spent repeated completion rounds
reconciling an empty lens with positive read evidence. This is tracked as
RNE-C58 rather than treating the eval's extra `@Component` substring as the
primary product criterion.

RNE-C58 first slice delivered:
source-inventory candidate construction now applies active typed query filters
before spending the per-role scan budget, so relevant parser-produced symbols in
auxiliary/corpus/third-party language surfaces are not hidden behind thousands
of unrelated symbols in global graph order. Direct `list_files` source
inventory observations now accept only real repo-relative output rows; rendered
tool advisories such as source-inventory hints, navigation summaries, and
`member_rows=0` text cannot enter typed observation members. Narrow
auxiliary/corpus/third-party scopes now trigger the same auxiliary projection as
root inventory lanes through `SourcePathRole`, so a precise
`internal/thirdparty/...` source-inventory repair no longer disables member
parsing. Default source-inventory repo-map queries now combine typed
`SourceInventoryProfile.SourceQuotes` with typed `AnalyzerHints.Entities`, so
generic field-shape quotes such as "list file path and function name" do not
erase exact decorator/API identifiers already captured by analysis. Remaining
work is the
broader SourceInventoryMemberProjection kernel for richer cross-language facets
and explicit `no_member_parser` coverage states.

RNE follow-up representative refresh:
`eval/convergence_audit_summary_20260620_rne_followup.md` ran six read-mode
cases with two-way parallelism. All six passed, but five were flagged for
commercial smoothness or audit noise: `qf_architecture` still used 42 explorer
iterations and 8 repo_map calls; `cangjie_repomap` still hit context pruning;
`read_combo_log_current_source_explanation` still used 5 analyzer iterations
and 18 explorer iterations; several passing cases still emitted advisory
contract warnings. This confirms the correctness path is improving while RNE-C54
shared proof ledger, RNE-C55 typed repair/handoff carrier, RNE-C59 execution
kernel extraction, and analyzer prompt/tool-surface cleanup remain active work.

Four-kernel convergence status:

- Typed SourceClassUniverse matrix: delivered. Source-class counts are computed
  from git-tracked paths via `SourcePathRole`, carried on
  `SourceInventoryObservation`, and consumed by exact-absence gates.
- Bounded source-inventory execution kernel: partial. The current code now has
  a dedicated `internal/tool/sourceinventory` budget/page kernel for scan,
  materialization, query-scan widening, wall-clock, cancellation, cursor
  offset, and page state, plus a scoped `ExecutionView` that owns sorted file
  materialization, language indexing, scope membership, truncation state, and
  complete/incomplete reporting. `internal/tool` candidate construction
  consumes it through a thin adapter, and the zero-value budget bypass ratchet is
  now 0. Remaining work is true cursor-backed resumable candidate pagination
  before full attribute/member materialization.
- Lane-neutral proof coverage kernel: delivered for the current read/write
  loop. Write mode consumes `loopkernel.DeriveProofCoverageAuthority`;
  read mode projects `TurnAArtifacts` into the same `ProofSnapshot` authority
  and truth ledger; ReasoningGraph records the projection; and parallel
  explore dispatch consumes covered read proof by lane ownership key before
  canceling equivalent support siblings. Weak/missing proof cannot satisfy the
  dispatch-group handoff.
- Typed repair/handoff carrier: partial. `ToolResult` now carries a bounded
  `ToolHandoffCarrier` derived from typed `ToolRepair`,
  `PlanRepairPack`, `ObservationRecord`, and accepted `EvidenceItem` IDs.
  `TurnAArtifacts` preserves those carriers across explorer fork/merge and
  handoff bounds, and extractor/finalizer prompts render a bounded typed view
  without tool summaries, model rationale, or repair-hint prose. ReasoningGraph
  records the same carrier as `tool_handoff_projected`. Emit-tool JSON repair
  now uses a schema-derived descriptor registry compiled from `Parameters()`,
  stored as typed `ToolRepair` metadata, and carried as
  `ToolJSONSurfaceDescriptor`. Remaining work is status-card carrier
  projection.

## Gap Ledger

| ID | Priority | Gap | Root cause | Target state |
| --- | --- | --- | --- | --- |
| RNE-C1 | P0 | Noisy Resolution Chain / concrete-value / registry evidence can dominate convergence. | Chain promotion and concrete-value extraction can generate many support-context rows for broad architecture questions; downstream gates see repeated missing anchors even when the principal answer is solved. | Build a typed relevance budget before handoff: principal answer evidence, mandatory proof obligations, and unresolved blockers are rendered first; support chains are capped by consumer and demoted when they do not change the next action. |
| RNE-C2 | P0 | Empty exhaustive sets loop forever. | `aggregate_facts.member_set` requires positive members, but exhaustive enumeration gates require a member-set handoff even when the correct answer is zero. | Treat `kind=member_set, value="0", members=[]` as a valid exact empty set. It satisfies exhaustive handoff only when backed by typed `negative_search` or `negative_observation` zero-result evidence. |
| RNE-C3 | P0 | Relation `member_set` support can fail despite usable line-backed evidence. | Support refs and evidence labels are stricter than the evidence surface: one member can have multiple support locations, and labels such as method names may not equal the answer member string. | Member support resolution should accept typed support refs when the cited location or emitted evidence proves the member, with deterministic label/location matching and no prose inference. |
| RNE-C4 | P0 | Repeated downgrade lacks low-delta stop conditions. | The same pre-complete rejection can be reissued after no new typed evidence, no new reads, and no new candidate set. | Record downgrade fingerprints per exploration lane. When a retry produces no typed delta, either accept with caveat if proof obligations are met or block with a concise typed reason instead of broadening. |
| RNE-C5 | P0 | Trace causal path facts can be lost in final answer. | Trace queries emit many rows, but the finalizer receives no compact path-role obligation ledger for source, sink, intermediate, and exclusion roles. | Add a typed trace path projection consumed by extractor/finalizer: path members, role labels, confidence, and supporting trace row refs. |
| RNE-C6 | P1 | `Resolution Chain anchor line is outside the fetched slices` is correct but noisy. | The range-aware guard demotes chains whose anchors were not read and schedules surgical reads. On low-value support chains this creates repair churn. | Keep the guard, but make its pending reads consumer-scoped and budgeted. Support-only chains become advisory when principal proof is already satisfied. |
| RNE-C7 | P1 | Large read-mode evals can run without a watchdog. | `eval/run.sh` read-mode paths do not consistently wrap Codrax with the shared timeout helper. | Add eval-level wall-time controls for read mode, with configurable defaults and typed timeout summaries. |
| RNE-C8 | P1 | Mixed log/current-source tasks can re-explore after enough evidence exists. | Runtime observation proof and source-localization proof are not unified into a single coverage ledger. | Observation proof coverage should become an online state: runtime symptom, source owner, impact path, and verification caveats are independently tracked. |
| RNE-C9 | P1 | Cross-language call-chain cases still over-read. | Repo-map navigation facts and read-range obligations are not used as a strict enough exploration budget for C/C++, ArkTS, Cangjie, Java/Kotlin, JS/TS, Ruby, Go, config, and workflow files. | Language-neutral localization and impact views set read budgets and proof obligations, with language adapters providing typed parse/edge facts where available. |
| RNE-C10 | P0 | Post-complete localization repair can still leak into extraction. | A retry directive / localization supplement can mention a forced `read_file` after exploration already accepted closure. The extractor cannot run read tools, so it burns an unavailable-tool round and may broaden reasoning from stale repair text. | Stage handoff must project localization repair debt into extractor-safe obligations: verdict/caveat/status only. Any real need for more `read_file` must route back to exploration before StageExtract. |
| RNE-C11 | P0 | Final system supplements can contradict a solved principal answer. | Localization/navigation supplements are rendered even after the principal member set, citations, and repo_map coverage are accepted. The final answer can simultaneously pass eval and display "localization needed" / low-proof caveats that are stale or support-only. | Final report supplements need a typed relevance gate: principal-answer proof status first, support-only localization debt collapsed or hidden, and residual caveats shown only when they are blocking or materially affect the user's answer. |
| RNE-C12 | P0 | Tool/preflight/context assembly latency can dominate successful read runs. | Large evidence ledgers and repeated completion prechecks can make `emit_evidence`, `emit_investigation_complete`, and "organizing context" slow even when the next decision is already known. Static tool schemas, grounding views, completion preflight state, and evidence scans are rebuilt in multiple places. The latest mixed runtime/source rerun passed but still spent 242s and about 66k context tokens, proving that correctness and smoothness are separate deliverables. | Add timing telemetry and shared typed preflight/cache layers: static tool parameters cached, schema normalization marked once, grounding context cached by dispatch/version, and completion gates consume one `CompletionPreflightView` instead of rescanning evidence repeatedly. |
| RNE-C13 | P0 | Accepted-closure advisory debt is not consumed consistently across scheduler surfaces. | `chain_promotion.*` pending reads can be advisory for auto-complete but still appear in retry hints or forced-read drains if the cleanup happens after hint rendering. This creates the visible pattern "investigation complete → verification not stable → same support-chain read again". | A single typed `RepairDebtClass` policy must feed auto-complete, fact-retry suppression, retry-hint rendering, forced-read drains, and audit checkpoints. Accepted closure may keep principal blockers, but advisory debt is cleared before any model-facing retry hint. |
| RNE-C14 | P1 | User-facing retry notices can remain stale after auto-complete. | The renderer may already have emitted "verification not stable enough" before the scheduler recognizes that accepted closure supersedes a retry carry-over. The system state is correct, but the REPL progress card looks like a real retry. | Status-card events should be driven by typed next-action decisions after stale retry carry-over suppression, so auto-completed advisory retries render as "accepted; skipping support-only retry" instead of another unstable-verification cue. |
| RNE-C15 | P1 | Final contract telemetry still has schema-level noise after eval pass. | A run can report eval `answer_contract_violations=0` while the internal CGEC summary records non-blocking answer-document field violations such as candidate role annotation drift. The C++ virtual-chain pass still had seven contract warnings, and the mixed runtime/source rerun needed a finalizer patch for `answer_richness_facet_coverage` despite enough evidence. | Final contract telemetry should distinguish blocking user-answer defects, repaired/non-blocking schema drift, and audit-only annotation gaps with typed severity, then feed the same status card and reasoning graph. |
| RNE-C16 | P0 | Mixed runtime-artifact plus current-code requests can collapse to observation-only. | Analyzer can emit typed `requested_answer_dimensions.role=current_key_code` and `source_scope_profile`, while `CurrentSourceLaneDecision` only treats `current_source_mode=allow` or explicit current-source profile as hard current-source anchors. Explorer then receives `Runtime Artifact Only Start`, accepts `external_only_log`, and finalizer discovers the missing current-code dimension too late. The latest regression showed a second variant: artifact-citation external-only is valid for citation provenance, but it must not close a required current-code answer dimension. | Source-lane authority consumes typed required `current_key_code` dimensions paired with valid source scope or dimension anchors, not prose. Observation-only remains valid for pure runtime metric dimensions. Explicit source exclusion may override weak source-scope drift, but it must not override a separate required current-source answer dimension unless the typed request model marks that dimension out of scope. `external_only_*` citation policy cannot close a required current-source lane. |
| RNE-C17 | P0 | Missing repo_map lens debt can requeue after grounded current-source evidence exists. | After source anchors and evidence are accepted, pre-finalize localization can still treat a missing `file_map` lens as principal and repeatedly show "verification not stable enough". This is support/navigation debt, not necessarily answer-blocking proof debt. | Navigation debt must be classified by consumer and principal coverage. If current-source evidence/read coverage satisfies the active lane, missing lens debt is advisory or at most one bounded repair; it must not keep requeueing identical source-localizer work. |
| RNE-C18 | P1 | Requested-dimension role drift causes avoidable finalizer patch rounds. | The visible answer may already cover the user's dimension label, but coverage hints can report a role-shaped miss such as `日志线索 (diff_clue)`. This happens when origin/dimension role projection overfits a role enum rather than the typed source quote/label coverage. | Dimension coverage should consume normalized dimension ids, labels, source quotes, and block evidence origins together. Role aliases are soft display metadata; missing-dimension hints must not depend on a misleading role name alone. |
| RNE-C19 | P1 | Runtime-artifact language guesses can pollute repository reasoning. | Log triage may emit `meta.lang=python` for a generic runtime log even when the repository is Go. That artifact-language guess can become noise for downstream source localization and audit language summaries. | Separate artifact format/language from repository/source language. Runtime triage language is advisory artifact metadata only; source-language decisions come from repo_map/file paths/typed source inventory. |
| RNE-C20 | P1 | Parallel exploration siblings can duplicate a solved current-source lane. | The mixed runtime/source eval passed only after 24 reads, 4 repo_map calls, 56 explorer iterations, 18 midloop injections, and context pruning. Several sibling routes collected overlapping finalizer/LLM timeout evidence. | Add a shared proof-coverage ledger across siblings. Once one route satisfies runtime observation + current-source owner + mechanism coverage, sibling routes consume the ledger, skip duplicate reads, and converge through a compact handoff. |
| RNE-C21 | P0 | Runtime-only trace answers can be blocked or polluted by source navigation debt. | Trace-only tasks can produce enough `trace_query` observation facts, then pre-finalize Tier-1 still demands `repo_map` `task_map/file_map` and final rendering appends repo navigation/localizer supplements. The request's typed route is artifact-local, but the source-localizer authority is not origin-aware enough. | Add an origin-aware proof profile. When a task is runtime-artifact-only and has no typed current-source obligation, repo navigation/localizer debt is not a blocking pre-finalize floor and is not rendered as a user-facing source supplement. Trace observation audit remains visible through trace-specific typed observations. |
| RNE-C22 | P0 | Trace root-cause rank is not promoted to the principal answer carrier. | `trace_query` emits `root_evidence` / `root_cause_rank` rows for `threadpool-400`, but final answer can lead with the target thread's direct wait state and fail evals or user reading that expect the primary root-cause node to be first-class. | Add a typed `TraceCausalProjection` with roles such as `target_direct_wait`, `primary_root_cause`, `causal_chain_hop`, `direct_waker`, and `drilldown_boundary`. Finalizer consumes this projection as a principal obligation instead of rediscovering rank from raw trace rows or prose. |
| RNE-C23 | P0 | Source inventory can falsely prove absence when supported-language fixture/corpus files are outside the default source scope. | The ArkTS eval repository contains `.ets` corpus sources under `internal/thirdparty/tree-sitter-arkts/corpus/sources`, but the run answered that no ArkTS source files exist. The scanner/navigation view and grep/list outputs did not expose a typed source-scope boundary that differentiates product source, fixtures, corpora, testdata, vendored code, and generated assets. | Introduce a language-neutral `SourceInventoryAuthority` with typed scope classes. Absence answers must state the exact searched source classes and cannot close if matching files exist in an in-scope class required by the task/eval. This must cover all supported languages, including C/C++, Cangjie, ArkTS, JS/TS, Java/Kotlin, Ruby, Go, config, workflow, and other repomap languages. |
| RNE-C24 | P1 | Empty/negative aggregate schema repair still burns many model rounds. | After exact empty member-set support landed, models still repeatedly repair `negative_search` / `negative_observation` dimensions and sometimes mix repo no-hit evidence with infrastructure evidence. | Add a unified aggregate-fact repair/normalization layer that returns precise typed repair hints and canonicalizes no-hit repo searches, artifact no-hit observations, and exact empty sets before retry. The layer must not infer from model prose; it only consumes schema-validated fields and tool result metadata. |
| RNE-C25 | P0 | Runtime-artifact-only turns can still print repo-index progress before any source lane is needed. | The targeted trace rerun no longer used `read_file` or `repo_map`, but Stage 1 still warmed and reported the repo index before perf triage / source-lane authority had proved current-source work unnecessary. | Make repo-index warmup lazy and origin-aware. Runtime-artifact-only requests should not scan or report repo index progress unless a typed current-source obligation, repo_map tool call, or source-localizer route actually needs it. |
| RNE-C26 | P1 | Requested answer dimensions can be repaired only at finalizer patch time. | The trace rerun answered correctly, but the finalizer needed a second patch round solely to add requested dimensions such as related chain and drilldown direction. | Compile a typed answer-surface scaffold from requested dimensions and runtime projections before the first finalizer call. Patch rounds should be reserved for real missing evidence, not predictable section skeletons. |
| RNE-C27 | P0 | Legacy `RawRequest` lexical fallbacks can still influence hard-ish gates or prompt authority. | A quick audit found scattered helpers such as trace finalizer guidance, trace flavor/platform hints, and coherence entity mention checks reading raw user text directly. Some uses are legitimate exact-provenance/path-artifact extraction, but their current shape is not centralized and can drift into keyword-driven routing or unnecessary prompt noise. | Add a `RequestSignalAuthority` / typed provenance layer: raw text may be used only for path/artifact tokenization, exact quoted provenance, or analyzer-emitted typed profiles. Hard gates and authority prompts consume typed ledgers/profiles, never ad hoc RawRequest substring checks. Batch Y1 delivered typed runtime-trace finalizer gating; Batch Y2 continues the remaining raw-signal audit. |
| RNE-C28 | P0 | Runtime negative observations still use a source-shaped schema path. | The Batch W trace rerun had enough `trace_query` evidence but one `emit_investigation_complete` attempt was rejected because `aggregate_facts[6]` used `negative_observation` for a missing IO layer detail without a dimension target/query/pattern/predicate. The retry completed, but the rejection added another explorer loop. | Split negative fact authority by origin. Runtime/log/trace negative observations have typed runtime dimension carriers such as `missing_signal`, `missing_event`, and `missing_field`; repo no-hit, artifact no-hit, and trace missing-field facts canonicalize through one repair layer before the model sees a retry. Batch V1 delivers the runtime carrier path; Batch V2 keeps the broader repair-hint layer. |
| RNE-C29 | P1 | Observation-only finalizer prompts and contract checks still carry broad source-rule noise. | The Batch W trace rerun no longer indexed or read source, but the finalizer prompt still included many source citation / repo_map / member-set / absence-contract instructions. The first emit was accepted only after deterministic observed-artifact carrier repairs and a second contract check. | Add observation-only answer-surface specialization: finalizer receives a compact runtime-artifact contract, source-specific rules are hidden unless a current-source lane is active, and deterministic carrier repair remains as fallback rather than the normal path. |
| RNE-C30 | P0 | Stage-owned retry directives can bypass stage tool-surface projection. | Previous fixes scoped explore-owned retry hints away from extraction, but a retry hint whose owner is already `StageExtract` can still carry stale exploration actions from a downstream validator or repair directive. `BuildPromptContext` rendered `ac.RetryHint` verbatim before the extractor/finalizer-specific handoff sanitizer ran, so the model could see "use repo_map/read_file" in a stage where those tools are unavailable. | All model-facing retry directives pass through a capability-surface projector keyed by current `ToolSuggestions`. If a directive mentions unavailable known tools, the original prose is replaced with a stage-safe structured emit/caveat instruction; original text remains in logs/state for audit. This is prompt hygiene only, not user-intent or model-prose hard routing. |
| RNE-C31 | P0 | Hypothesis-to-task binding can still score model/template prose. | `analysis/binder` used task node objective surface tokens as part of relevance scoring. Node objectives are useful user/model guidance, but they are not typed routing authority and can reintroduce prose-driven scheduling even without direct RawRequest parsing. | Binder relevance consumes only typed search hints and typed falsification-kind affinity. Objective prose remains visible guidance but cannot change task/hypothesis routing. |
| RNE-C32 | P0 | Source inventory remains prompt-advisory instead of an executable localization authority. | The ArkTS representative run had `repo_map=0`, `source_lens=0`, and answered absence after `list_files`/`read_file` work. The explorer prompt mentions `repo_map(view="source_inventory")`, but no typed scheduler obligation required it before closing an absence/member-set answer over supported language files. | Add a language-neutral `SourceInventoryAuthority`: analyzer/source-inventory profiles, absence answers, exhaustive member-set questions, and cross-language scope requests create a typed navigation obligation. The scheduler or localizer executes a bounded `repo_map(source_inventory)` view before absence closure, records scope classes, and feeds only the Top-N typed view to the model. |
| RNE-C33 | P0 | Source inventory profile synthesis is not strong enough when analyzer omits the optional profile. | Typed source-enumeration requests can arrive as `IntentEnumerate + IsCategoryEnumeration` plus navigation query terms, while `source_inventory_profile` is absent. Pre-explore advisory can then be missing or too broad, and pre-complete lens execution gates do not see an active inventory lane. | Synthesize an advisory-only `SourceInventoryProfile` from typed request shape plus analyzer navigation query terms, query-filter candidates by supported language and structural symbol surface, and make synthetic inventory lanes participate in the same executable lens gate as explicit profiles. |
| RNE-C34 | P1 | Source inventory telemetry can under-report system-supplied authority. | Eval metrics count explicit model toolcalls to `repo_map(view="source_inventory")`, but system-built advisory/observation projections and pre-complete synthetic inventory lanes can guide or gate the run without being visible as `source_lens`. This can make a PASS look healthier or less healthy than it really is. | Split telemetry into explicit lens toolcalls, system-compiled advisory projections, active inventory obligations, and satisfied/blocked inventory gates. Status cards and eval summaries should explain which carrier closed the lane. |
| RNE-C35 | P1 | Support/audit rows can still appear near principal source-inventory slates. | ArkTS logs showed parser/helper rows such as `builderFunctionRegex` in support-context lanes while the final principal answer correctly used the aggregate member set. The final answer passed, but broad support rows increase context noise and can pollute weaker finalizer drafts. | Finalizer and extractor consume a principal slate compiled from accepted `member_set` / source-inventory observation rows; support/audit rows stay in a capped secondary lane and cannot become principal members without a typed role transition. |
| RNE-C36 | P0 | System-supplied localization observations are not authoritative enough in model-facing exploration. | The source-inventory lens can now be system-compiled, but the explorer may still spend its next turn reading parser/helper implementation files and then answer from a weaker absence prior. Typed observations exist, yet the model-facing context does not make them the principal investigation slate early enough. | Promote system-supplied localization observations into a typed `PrincipalLocalizationSlate` before free-form exploration. Explorer receives owner/member/path candidates first, support rows capped second, and any close/absence attempt checks the same slate. The scheduler may still allow additional exploration, but not by ignoring a fresher typed authority. |
| RNE-C37 | P0 | Repository-wide typed source-inventory queries can be narrowed by default production scope. | Analyzer can emit `source_scope_profile=production` as a default even when the source-inventory lane is a repository-wide typed query. Fixture, corpus, testdata, vendored, generated, and example source files can then disappear before the inventory authority sees them. | Repository-wide typed query/root-scope inventory lanes search all source classes by default; explicit production-only inventory remains production-filtered. The rule is driven by typed inventory provenance and source-scope fields, not language names or user keywords. |
| RNE-C38 | P1 | Analyzer pre-scan/listing priors can conflict with later source-inventory authority. | Early grep/list_files or pre-scan summaries can tell the model "no matching source" before the typed inventory lane has run, especially for non-Go language surfaces inside corpus/fixture directories. That stale prior increases model noise and can bias absence answers. | Source-inventory-shaped requests get either an early bounded inventory observation before broad pre-scan summaries are rendered, or the pre-scan result is marked advisory/low-priority until the typed inventory obligation is satisfied. |
| RNE-C39 | P0/P1 | `emit_investigation_complete` aggregate normalization remains slow on large ledgers. | Latest logs showed `emit_investigation_complete` taking about 15s with `aggregate_normalization` as the slowest phase. The tool may already know the next typed decision, but repeated scans over evidence, aggregate facts, read history, and grounding views still dominate "organizing context" / completion checks. | Finish Batch K2: one `CompletionPreflightView` per dispatch/version, cached aggregate normalization inputs, shared source-inventory/localization proof projections, and timing rows that feed status cards without becoming semantic gates. |
| RNE-C40 | P0 | Section-based principal enumerations are misread as uncovered and get duplicate system补表. | The renderer supports `section` blocks with `items[]` and citations, but the principal enumeration compiler and exhaustive member coverage only treated table/ordered/bullet blocks as row carriers. The finalizer naturally used two sections for `@Entry` and `@Builder`; the system then appended duplicate "系统按已验证证据补充成员" blocks after the correct answer. | Treat `section` items as a first-class structured enumeration carrier wherever item surfaces are rendered: row compiler, coverage invariants, principal-evidence view, supplement suppression, and tests. System supplements are append-only audit aids only when they add non-overlapping members. |
| RNE-C41 | P0 | Source-inventory/member-set answers can be fully line-grounded without owner anchors, but last-mile source-localizer supplements still fire. | Owner-supported localization is the right requirement for owner/role lookup answers, but exhaustive source inventory answers can be proved by per-member definition citations. Requiring owner anchors for every fixture/corpus member turns a solved inventory answer into "observed_only" noise. | Add a typed `grounded_principal_enumeration` authority signal. If every visible principal enumeration item has a valid citation and enumeration facet, suppress generic source-localization, repo_map, localizer-followup, and degraded-floor user caveats; retain full typed artifacts for audit. Ordinary observed-only non-enumeration answers still keep warnings. |
| RNE-C42 | P1 | Eval telemetry does not distinguish explicit tool calls from system-compiled authority. | `arkts_repomap` had source-inventory authority active but reported `source_lens=0` because metrics count only model `repo_map(view=source_inventory)` calls. This obscures whether a lane was closed by explicit model exploration, a system observation, or a gate. | Extend eval/status telemetry with explicit lens calls, system-compiled source-inventory observations, active inventory obligations, satisfied gates, and blocked/advisory state. This is a reporting/observability fix, not a new hard gate. |
| RNE-C43 | P1 | Broad architecture and mixed runtime/source runs still pass with high iteration/context budgets. | Latest PASS runs still showed `read_combo` at 334s/82k context/25 explorer iterations and `qf_architecture` at 273s/29 explorer iterations/contract warnings. Correctness improved, but routine user experience remains too expensive for commercial smoothness. | Continue Batch D/K2/R with typed relevance budgets, shared proof coverage across siblings, compact architecture inventory projections, and cached completion preflight. These fixes must reduce turns and context without hiding evidence. |
| RNE-C44 | P0 | Repo-wide file-family absence can be falsely accepted from content grep or non-recursive root listing. | `grep include=*.ext` searches file contents under the current search filter; an empty result proves zero matching contents in searched files, not that matching paths or source-family files do not exist. Non-recursive top-level `list_files` with a glob has the same proof gap for nested language surfaces. | Delivered first fix: targeted `list_files(path=".", include/file_type=...)` now retries as recursive when the root-only pass is empty, then falls back to repo-owned auxiliary/corpus trees only if the primary recursive pass is still empty. Completion still rejects negative_search / empty member_set closure when a path-filtered grep no-hit is the only proof. |
| RNE-C45 | P0 | Soft path-discovery advisories can be ignored before closure. | Tool output correctly advises that grep is not path discovery, but the model may still call `emit_investigation_complete` with `negative_search` or an exact empty set. Prompt guidance alone is insufficient for hard absence proof. | Completion preflight escalates the typed tool-result advisory into a narrow hard proof obligation. The gate consumes only tool banners, aggregate fact kinds, and typed request shape, never user words or model prose. |
| RNE-C46 | P1 | Repo-owned auxiliary/corpus directories are excluded unless scope is explicit. | Default search/list filters skip large auxiliary trees to control noise. That is correct for routine production questions, but false absence proof for repository-wide inventory or fixture/corpus evals unless the universe explicitly includes those repo-owned auxiliary surfaces. | `list_files` supports `include_auxiliary=true` for repo-owned auxiliary/corpus surfaces while retaining dependency/cache exclusions. Completion requires that flag when such trees exist and the proof claims repo-wide file-family absence after a production-only/no-hit scan. |
| RNE-C47 | P0 | `repo_map(view="source_inventory", roles=["file"], scope=".")` can enter a high-CPU runaway. | `source_inventory` is a semantic member/count lens, but the schema allowed `file` as the sole primary role. The model used it as file-family discovery after a completion refusal, causing a root-scope source_inventory pass to spin at high CPU instead of failing fast. | Tool-boundary preflight rejects sole-primary `file` role before graph load/index work and directs the model to `list_files(recursive=true, include/file_type=...)`. `file` remains allowed only for bounded non-root file-local attribute expansion or alongside semantic roles such as `config_file`; broad root `roles=["file"]` is refused even when `attribute_roles` is present. Broad root source-inventory calls also receive deterministic budget normalization before lens execution: unset `top_n` is default-bounded, explicit oversized `top_n` is clamped, and row-local attribute expansion is disabled until the model chooses a narrower scope. |
| RNE-C48 | P0 | `source_inventory` candidate construction needs a tool-internal execution budget, not only model-call correction. | Boundary normalization and broad-root parameter clamping prevent the observed `roles=["file"]` runaway class and explicit over-wide model calls from materializing huge result pages, but the lens engine also needs its own guard so future wide role/scope combinations cannot build full candidate/advisory sets before render-time paging. The 2026-06-20 samples exposed both dimensions: the model can choose a too-wide tool shape, and the algorithm must remain bounded even when that happens. | Delivered fourth slice: candidate construction separates materialization budget from scan budget, and broad root navigation-only lenses (`file`/`package`/`config_file`) now bypass full reconcile entirely and return an active bounded navigation sample with `complete=false`, `repo_lens:broad_navigation_guard`, and `repo_lens:candidate_budget_truncated`. A no-match or sparse-role wide scan cannot disappear or become absence proof. Remaining: add explicit wall-clock/cancellation checkpoints and true resumable pagination before full candidate/attribute materialization. The guards consume typed role/scope/top_n/query parameters and graph indexes only, not user prose or model explanations. |
| RNE-C49 | P0 | Filtered `list_files` path discovery can emit directory noise as if it were matches. | Recursive `list_files(include=.../file_type=...)` used directory traversal rows as output rows. For file-family discovery this makes a precise `.ets` query look like a generic directory listing, causing the model to discard a valid path-discovery tool and fall back to slow shell `find`/grep. | When include/file_type filters are present, directories are traversal-only and never output as result rows. Only matching files appear in the returned universe; unfiltered list_files keeps its directory-listing behavior. |
| RNE-C50 | P0 | Empty source-inventory answers can loop between `absence`, `resolved`, `member_set`, and unavailable tool surfaces. | A task can gather contextual grounded evidence that proves why the requested member set is empty. The completion gate then rejects `absence` because evidence exists, but also downgrades `resolved` until an executable source-inventory lens/empty `member_set` handoff is present. In retry-only tool surfaces, the model may be asked to run `repo_map` when only `emit_*` tools are available, producing low-delta loops. | Delivered: model-authored empty `member_set` plus typed zero-result support closes the lane without semantic contradiction from supporting evidence. Structured repair directives now declare required tools, and completion-only surfaces expose only `emit_*` plus those typed repair tools, so a mandatory source-inventory repair can call `repo_map` without reopening broad exploration. |
| RNE-C51 | P0 | Source-inventory observation ledger compiles huge member sets before budgeting. | The post-U7g ArkTS rerun no longer hit the old `repo_map` CPU path, but after a broad source-inventory observation with 47,117 members the mid-loop answer-surface builder spent high CPU in `CompileObservationLedger -> compileSourceInventoryObservationObservations -> sourceInventoryObservationSupportRefs`, expanding every member/support ref before prompt budgeting. | Delivered: source-inventory observation compilation now preserves the set-level full count and row-set ref but projects only a bounded member/attribute sample into the ledger, samples support refs with map-based dedupe, and records truncation notes for consumers. Deeper source-inventory candidate-construction budgets remain under RNE-C48. |
| RNE-C52 | P0 | Attached trace queries can drift into repo-source exploration when the model passes an over-wide path. | The U9 `trace_query_wakeup_causal_io_chain` run had an attached trace but the model called `trace_query(view=wakeup_chain, path=".")`. The resolver treated `.` as `source=path`, failed on a directory, and the model then listed/read source files, polluting a runtime-artifact-only answer with repo-source debt. | Delivered: `trace_query` source resolution now normalizes typed default selectors (`path=""`, `.`, repo root, workdir) to `source=attached_trace` when an attached trace exists, while explicit trace files stay `source=path` and arbitrary directories fail fast with a typed repair hint. This consumes only tool params and `BusContext`, never user prose or model explanations. |
| RNE-C53 | P0 | Cross-language inventory can still prefer production/source-code absence over repo-owned corpus and fixture surfaces. | U9 ArkTS failed by saying no real `.ets` ArkTS source existed even though `internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets` was a repo-owned corpus source. Cangjie harness passed but manual audit showed the same weakness: parser/test examples were used instead of a first-class source-inventory authority view. | Delivered full truth-path fix: `repo_map(view=source_inventory)` now builds a bounded source-class projection through the shared `SourcePathRole` classifier from git-tracked files; `list_files` targeted empty-primary scans auto-retry recursive + repo-owned auxiliary; Turn A handoff/extractor prompts preserve `SourceInventoryObservation.source_classes[]`; and the final source-inventory exact-absence boundary now consumes the typed class universe in both `emit_answer_document` preflight and orchestrator V2 contract. A normal negative-scope citation no longer proves source-family absence while source classes are open; the model must supply/consume a complete empty source-inventory member set or answer with unknown/caveat. |
| RNE-C56 | P1 | Broad source-inventory repair does not auto-narrow to required files/scopes after a no-row lens. | The passing ArkTS rerun still took 235s / 15 explorer iterations. After the closure gate requested `repo_map(view="source_inventory")`, the model ran a broad root lens with all semantic roles; it returned `member_rows=0` plus `source_classes`, then needed extra loops even though analyzer `required_files` and read evidence already named the exact corpus sources. | Delivered first slice: broad no-row source-inventory lenses now auto-narrow to typed analyzer `required_files` / common scope, and if the model supplied an over-specific query that still yields no rows, the same required-file scope is retried with the query relaxed. Tool output carries `source_inventory_narrowing:*` advisories and `repo_lens:auto_narrow_*` provenance. The current-lens renderer was also separated from cumulative MutableState so old `list_files`/direct-child rows cannot suppress no-row narrowing. Remaining: extend the same narrowing authority to accepted evidence files/source-class scopes and scheduler-driven execution before model close attempts. |
| RNE-C57 | P1 | Source-inventory closure repair can suggest a file path as the `repo_map` path. | The post-U9f ArkTS eval showed the pre-complete repair directive asking for source_inventory over `required_files`, where the only required file was `internal/tool/repomap/index/extract_arkts.go`. The model copied that file path into `repo_map.path`, hit a deterministic tool refusal, then retried with the containing directory. | Delivered: repair directives now normalize file-shaped source-inventory scopes into a legal `repo_map` call shape: `path` becomes the containing directory and `scopes` contains the relative basename. Multi-scope directory repairs still use `path="."` plus repo-relative scopes. This is path-kind normalization over typed scope strings, not user/model prose matching. |
| RNE-C58 | P0 | SourceInventory class universe prevents false absence, but cross-language member/facet extraction can still be incomplete. | The post-gate ArkTS rerun proved the hard absence boundary works, but `repo_map(source_inventory)` returned zero `function/method` rows for decorator-bearing `.ets` corpus files; the correct answer came from `read_file` evidence instead. Root cause had four generic parts: scan budget was spent on unrelated symbols before applying active typed query filters; direct list-files observation accepted rendered advisory text as if it were member rows; auxiliary projection triggered for broad/root scopes but not for precise repo-owned auxiliary scopes such as `internal/thirdparty/...`; and generic source-inventory field quotes could replace exact analyzer entities in the default repo-map query. The same class can recur for C/C++, Cangjie, ArkTS, Java/Kotlin annotations, Ruby DSL methods, JS/TS decorators, config/workflow declarations, and any supported language where file-level class inventory exists before member/facet extraction. | Delivered first slice: query-active candidate construction filters parser-produced symbol fields before consuming scan budget, list-files observation rows must resolve to existing repo-relative paths before becoming typed members, narrow auxiliary source-class scopes trigger auxiliary projection through `SourcePathRole`, and default repo-map source-inventory queries merge typed `source_quotes` with typed `AnalyzerHints.Entities`. Remaining: build a language-neutral SourceInventoryMemberProjection kernel over repomap parser outputs plus line-feature fallback; files/classes come from `SourcePathRole`, members come from typed language adapters, and facets such as decorators/annotations/modifiers/visibility/config-key kind are represented as structured attributes. `repo_map(source_inventory)` should return bounded member rows for supported-language constructs or an explicit `no_member_parser` coverage state, so completion can avoid contradictory "empty lens + positive read evidence" loops. |
| RNE-C59 | P0 | Source-inventory remains a god-object with per-role materialization instead of one execution kernel. | U9b audit showed `source_inventory_reconcile.go` carrying advisory, lens execution, rendering, absence support, candidate building, and class universe responsibilities in one 5k+ line file. Even after broad-call guards, file/config/package roles and completion support paths could each rebuild and sort the same scoped file slice, so every new role/scope/language shape risked reintroducing a performance bug under a new label. | Delivered: source-inventory candidate construction builds one `sourceInventoryExecutionView` per lens/support pass, and the budget/page/cursor authority has moved into the dedicated `internal/tool/sourceinventory` kernel package. File/config/package candidates, graph-symbol file membership, broad attribute defer, completion support indexing, and page metadata consume the shared API. Remaining: extract scoped sorted-file execution view into the kernel, then add true cursor-backed resumable candidate pagination before full member/attribute materialization; no hard logic may consume user/model prose. |
| RNE-C60 | P0 | Analyzer source-scope and irrelevant-file channels can turn repo-wide inventory into production-only absence. | The ArkTS U9h/U9i sequence showed two related structural contradictions: the analyzer could put repo-owned auxiliary `.ets` corpus files into `irrelevant_files`, and it could emit `source_scope=production` from repository-layout inference even though the current request did not explicitly say production-only. That let later stages preserve some evidence but still drift into a "production 0" answer. | Delivered: `source_scope_profile` now supports validated `source_quotes[]`; quoted production/test/aux/all scope can remain a hard path boundary, while unquoted production in source-inventory/enumeration repair is treated as model inference and cannot exclude existing source/config paths. Principal source-scope `irrelevant_files` are dropped and promoted to `required_files`; auxiliary promotions synthesize `source_scope=all` with `include_auxiliary_as_principal=true`. The analyzer prompt/schema says scope boundaries need current-request quotes, but hard behavior consumes only typed fields, path existence, `SourcePathRole`, and `SourceScopeAllowsPathRole`. |
| RNE-C61 | P0 | Typed repair-required tools can be hidden by mid-loop restricted explorer surfaces. | The post-U9j ArkTS refresh produced a `RepairStructuredHandoff` requiring `repo_map` after the source-inventory lens gate downgraded completion. The no-emit escalated explorer surface still exposed only `emit_evidence` and `emit_investigation_complete`, so the model received a typed instruction to run `repo_map` while the schema/runtime surface made `repo_map` unavailable. After repeated no-delta turns, low-delta force-complete accepted an incomplete inventory. | Delivered: no-emit restricted explorer surfaces now overlay active `RepairDirective.Tools`, and schema filtering plus runtime boundary validation consume the same typed surface. The status hint prefers the actual observed tool surface when known. This fix consumes only schema-validated repair fields and tool names, never model prose or user-intent keywords. Remaining broader work stays under Batch U9h: tool/schema repair, supported JSON surfaces, and accepted evidence IDs should flow through one cross-stage typed carrier. |
| RNE-C54 | P1 | Parallel exploration siblings duplicate proof work after one lane has enough evidence. | U9 mixed log+current-code passed but took 277s with 13 reads, 3 repo_map calls, 17 explorer iterations, and duplicated LLM timeout/finalizer investigation across siblings. One sibling tried to close while another continued reading the same files. | Delivered: read-mode TurnA handoff projects a typed `loopkernel.ProofSnapshot` from accepted closure, source localization, source-inventory observation, runtime-observation-only completion, and evidence refs. The snapshot derives `ProofCoverageAuthority` and `TruthLedger` through the same loopkernel authority functions write mode uses; ReasoningGraph records it; and parallel dispatch consumes covered proof by lane ownership key to cancel equivalent support siblings. Weak/missing proof, failed proof, or proof without accepted closure/runtime completion cannot satisfy the handoff. |
| RNE-C55 | P1 | Tool/JSON repair hints remain stage-local and can lose accepted evidence IDs. | U9 mixed runtime/source logs showed `emit_analysis` retrying an unsupported `source_quotes` field, `emit_investigation_complete` aggregate-fact shape repair, and extractor verdict citation/evidence-id gaps after accepted evidence existed upstream. | Delivered carrier + consumer + graph + schema-registry slices: tool decode/schema repair, plan repair packs, typed observations, accepted evidence IDs, and schema-derived JSON surfaces now project into `ToolHandoffCarrier`, survive Turn-A snapshots/fork merge/bounds, render in extractor/finalizer as bounded typed fields, and emit ReasoningGraph `tool_handoff_projected` events. The renderer omits repair hints and tool summaries so no prose becomes hard-ish routing. Remaining: status cards should project the same carrier. |

## Architecture Direction

### 1. Typed Relevance Budget

Before rendering handoff context, build a deterministic relevance view with:

- Principal obligations: the exact scalar/member/path/trace answer carrier.
- Blocking proof debt: missing typed evidence, missing read range, failed support
  ref, unavailable environment, or unresolved contradiction.
- Advisory support: mechanisms, registries, helper chains, concrete values,
  broad inventories, and historical repair notes.
- Consumer scope: explorer, extractor, finalizer, evaluator, and report writer
  each receive different Top-N slices.

The view must preserve full artifact refs for audit, while prompt surfaces stay
small and role-labeled.

### 2. First-Class Empty Member Sets

`member_set` represents an exact set. Exact sets can be empty. A zero-member
set is a valid principal carrier only when:

- `kind=member_set`
- `value="0"`
- `members=[]`
- role resolves to `principal_answer`
- the request requires an exhaustive set or relation set, and
- typed no-hit evidence is available through `negative_search` or
  `negative_observation` aggregate facts. External trace/log/history no-hit
  results also use `negative_observation` instead of relying on prose or broad
  origin metadata.

This avoids forcing the model into `scalar_value` for set-valued zero answers.

### 3. Relation Support Resolver

Support resolution should be deterministic and evidence-backed:

- Prefer explicit `support_refs`.
- Accept multiple refs for one member when each ref location or evidence row
  proves that member.
- Accept method/owner support when the cited snippet contains the member value
  or the typed evidence object binds owner to return/literal value.
- Do not use raw user words, closure reasons, or model rationale.

### 4. Low-Delta Convergence Guard

Each soft downgrade records a typed fingerprint:

- gate kind
- required carrier type
- invalid fact/member ids
- pending read origin and line ranges
- current evidence/read aggregate version

If the next attempt repeats the same fingerprint without new typed inputs, the
controller should stop broadening. It should either ask for the exact missing
typed carrier, accept with a caveat if enough proof exists, or produce a
blocked report with a concise reason.

### 5. Trace Path Projection

Trace-heavy tasks need a compact causal ledger:

- path source/sink/intermediate/exclusion roles
- trace row ids / span ids / timestamps
- observed vs inferred causality
- missing current-source anchors, if any

Final answer obligations consume that ledger directly instead of rediscovering
roles from many raw trace rows.

## Delivery Plan

| Batch | Status | Task | Acceptance |
| --- | --- | --- | --- |
| Batch A | delivered | Record this eval/gap/design ledger. | All P0/P1 gaps above are tracked with testable acceptance criteria. |
| Batch B | delivered | Implement first-class empty `member_set` support. | `value="0", members=[]` validates, satisfies exhaustive handoff when principal and backed by `negative_search` / `negative_observation`, and does not create bogus answer rows. Unit tests cover the no-hit path; the ArkTS representative rerun confirms the change does not break positive ArkTS enumeration. |
| Batch C1 | delivered | Strengthen relation/member-set support resolution. | One-member answers with a citable owner line and a unique read-file value line can receive deterministic `Member @ file:line` support without another model retry; ambiguous multi-line matches still downgrade. |
| Batch C2a | delivered | Propagate accepted-closure advisory debt consistently. | Support-only `chain_promotion.*` / Phase-1 pending reads are advisory after accepted closure across auto-complete, fact-retry, retry-hint render, and forced-read drains; principal anchors still block. |
| Batch C2b | planned | Add relation retry delta handling. | Duplicate support rows stay advisory, and repeated identical relation support downgrades stop broadening after one bounded repair turn. |
| Batch D | planned | Add typed relevance budget for chain/concrete/support evidence. | Architecture inventory and C++ call-chain cases render bounded Top-N prompt sections while preserving full refs in audit artifacts. |
| Batch E | planned | Add downgrade fingerprint / low-delta guard. | Identical pre-complete rejection without new typed input stops after one bounded repair turn. |
| Batch F | planned | Add broader trace causal path-role projection. | Trace eval preserves source/sink/intermediate/exclusion path roles in final answer with fewer trace_query calls. Batch T covers the principal root-cause carrier; this batch remains for richer path-role facets. |
| Batch G | planned | Add read eval watchdog. | Read-mode eval paths use a configurable timeout and emit typed timeout summaries. |
| Batch H | delivered | Quarantine post-complete localization repair for extractor. | Extractor receives non-executable localization status/caveat fields and does not attempt `read_file` / `repo_map` after accepted closure. |
| Batch H2 | delivered | Project stage-owned retry directives through current tool surface. | Extractor/finalizer retry directives cannot carry unavailable exploration-tool action plans into the prompt; explorer/analyzer hints that reference tools available in their own stage stay intact. |
| Batch I | delivered | Gate final system supplements by principal-answer relevance. | A passing scalar/member-set answer does not show stale "localization needed" or generic low-proof caveats unless typed proof debt is principal/blocking. |
| Batch K | partial | Add tool/preflight/context assembly telemetry and caches. | K1 delivered generic slow timing logs for tools that publish `RuntimeTimings`; existing schema and grounding caches remain in use. K2 remains for cross-gate completion preflight/context assembly reuse. |
| Batch L | delivered | Align user-facing retry/status notices with typed next-action state. | Accepted closure that skips support-only retry debt emits one progress-class skip notice from the typed advisory-debt decision, not another retry cue. |
| Batch M | planned | Split final contract telemetry by typed severity. | Eval pass/fail, user-answer blocking defects, repaired schema drift, and audit-only annotation gaps are reported separately and consumable by URGR/reasoning graph. |
| Batch N | delivered | Fix mixed runtime/source lane authority. | `current_key_code` requested dimensions plus typed source scope require source coverage before closure; pure runtime metric dimensions stay source-optional; the representative mixed log/current-code eval passes with real source reads and citations. |
| Batch O | delivered | Demote stale repo_map navigation debt after principal source coverage. | Missing lens debt no longer requeues after current-source proof is satisfied unless it is tied to an unresolved principal owner/path obligation. |
| Batch P | planned | Normalize requested-dimension coverage by typed ids/labels/origins. | Finalizer does not patch solely because a role enum label drifted; it patches only when the typed requested dimension is absent from visible answer content. |
| Batch Q | planned | Split runtime-artifact language metadata from repo/source language. | Log/trace artifact language guesses cannot drive source-language summaries, source localization, or hard gates. |
| Batch R | planned | Share proof coverage across exploration siblings. | Mixed runtime/source cases converge with bounded duplicate reads and smaller context while preserving source citations. |
| Batch S | delivered | Add origin-aware runtime-only source navigation suppression. | Trace/log artifact-only tasks with no current-source obligation skip repo_map/localizer blocking floors and user-facing source supplements while keeping runtime observation audit. |
| Batch T | delivered | Add trace causal projection and source-optional supplement gating. | Trace causal projection selects attributable wakeup-chain root-cause nodes over aggregate sentinels, dedupes supporting hops, and runtime-artifact source-optional answers no longer render source localization / repo_map audit supplements. |
| Batch U | partial | Add source inventory authority with scope classes. | Delivered: source-inventory scope handling no longer defaults typed source enumerations to production-only when no explicit source-scope profile exists, so fixture/corpus/testdata/vendor surfaces can be considered when the typed task requires repository-wide enumeration. Remaining: expose source-class breakdowns explicitly in status/eval reports. |
| Batch U1 | partial | Make source inventory an executable navigation obligation. | Delivered: explicit and synthetic source-inventory lanes now share a pre-complete executable lens gate; advisory/list_files rows do not replace `repo_map(view="source_inventory")`; query-filtered cross-language inventory avoids parser-helper noise; system-compiled repo-lens advisory/observation provenance now satisfies the same gate. Remaining: scheduler/localizer auto-execution before model close attempts, plus inventory-lane telemetry split. |
| Batch U1a | delivered | Synthesize and auto-observe typed source-inventory query lanes. | Typed `IntentEnumerate + IsCategoryEnumeration` requests with supported-language/query terms can synthesize an advisory-only inventory profile, query-filter graph candidates, auto-publish a system source-inventory observation, and avoid analyzer cardinality retry loops when members are intentionally discovered after classification. |
| Batch U1b | delivered | Keep repository-wide typed inventory queries from defaulting to production-only scope. | Synthetic typed query/root-scope inventory lanes include fixture/corpus/testdata/example source classes across supported languages; explicit production inventory profiles still filter auxiliary sources. Unit tests cover both sides. |
| Batch U2 | planned | Add source-inventory lane telemetry and status-card explanations. | Eval/status cards distinguish explicit model lens toolcalls, system advisory projections, active inventory obligations, satisfied gates, and blocked gates. |
| Batch U3 | planned | Cap support/audit rows around principal source-inventory slates. | Finalizer/extractor render accepted principal member sets first and keep parser/helper/support rows out of principal answer candidates unless a typed role transition promotes them. |
| Batch U4 | partial | Promote localization/source-inventory observations into principal exploration slates. | Delivered: explorer fresh-start prompts now render the typed source-inventory slate before breadth scan, parser/helper reads, grep/list guidance, and suggested search terms. Remaining: move the same priority into the shared observation-ledger ranking and answer-side soft absence advisory. |
| Batch U5 | partial | De-prioritize pre-scan noise until typed inventory obligations run. | Delivered: when a source-inventory slate is active, analyzer search terms render as secondary grep fallbacks and generic no-hit searches are explicitly advisory until the typed slate is reconciled. Remaining: make pre-scan/list no-hit status cards and eval telemetry carry the same advisory-vs-authority split. |
| Batch U6 | delivered | Align final-answer coverage gates with section-based source-inventory enumerations. | `section` items now participate in principal enumeration row coverage, exhaustive member coverage, grounded principal-enumeration evidence, last-mile source supplement suppression, and degraded-floor disclosure suppression. Ordinary observed-only non-enumeration answers still keep warnings. |
| Batch U7a | delivered | Add language-neutral file-family path discovery. | `list_files` now accepts `include`, `file_type`, and `include_auxiliary` so source-family/path discovery does not depend on shell `find` or content grep; grep/exec results emit typed advisories when they are being used as path discovery substitutes. Default filters still skip dependency/cache noise. |
| Batch U7b | delivered | Add typed file-family absence-proof gate. | `emit_investigation_complete` rejects negative_search / exact empty member-set closure for typed inventory/enumeration lanes when the only zero proof is path-filtered content grep or non-recursive listing. Recursive `list_files` with matching include/file_type or `repo_map(view="source_inventory")` satisfies the proof; repo-owned auxiliary trees require explicit `include_auxiliary=true`. |
| Batch U7c | delivered | Add source_inventory sole-file role fast refusal. | `repo_map(view="source_inventory")` rejects `roles=["file"]` as the only primary role before graph load/index work, preventing root-scope high-CPU runs. File path discovery uses `list_files`; file-local attribute expansion and `config_file` inventories remain supported. |
| Batch U7d | delivered | Add broad-root source_inventory budget normalization and early completion refusal. | Large root-scope source_inventory calls receive a bounded default `top_n`, explicit oversized `top_n` is clamped, and row-local attribute expansion is disabled until a narrower scope is provided. File-family absence refusal runs before expensive completion preflight/source-inventory advisory work when the typed proof gap is already known. |
| Batch U7e | partial | Add source_inventory internal execution budgets. | Delivered: candidate construction caches typed source-scope/repository-wide query decisions once per pass, applies typed query/language filters before candidate append, and enforces broad tool-lens per-role materialization budgets with `complete=false` / `repo_lens:candidate_budget_truncated` guidance. Remaining: progress checkpoints, cancellation/time budget, and true resumable pagination before full candidate/attribute materialization. |
| Batch U7f | delivered | Make filtered list_files output file-only. | With `include` or `file_type`, recursive `list_files` traverses directories but only emits matching files, so file-family discovery cannot be confused with generic directory inventory. Unfiltered listing behavior remains unchanged. |
| Batch U7g | delivered | Add executable empty-set completion recovery. | Empty member-set lanes close from `member_set(value="0", members=[])` plus typed zero-result support even when supporting evidence exists. Structured repairs carry typed required tools, and completion-only dispatches expose only `emit_*` plus those tools, so missing-lens recovery can execute `repo_map` without broadening to unrelated exploration tools. |
| Batch U7h | delivered | Budget source-inventory observation-ledger projection. | Large source-inventory observations keep the full set count and optional row-set ref, but ledger compilation projects only a bounded member/attribute sample and sampled support refs before downstream answer-surface budgeting. The post-U7g eval sample confirmed the previous hotspot was ledger projection, not `repo_map` execution. |
| Batch U7i | delivered | Close broad file-role and no-match scan runaway paths. | `repo_map(source_inventory)` now refuses broad root `roles=["file"]` before index work even when row-local `attribute_roles` are present. Source-inventory candidate builders also enforce a separate scan budget, preserving empty-but-incomplete truncated observations so no wide no-match scan can become an absence proof. |
| Batch U7 | planned | Add source-inventory/status telemetry split. | Eval and REPL status cards distinguish explicit source-inventory lens calls, system-compiled source-inventory authority, active obligations, satisfied gates, blocked gates, and advisory pre-scan no-hit state. |
| Batch U9a | delivered | Normalize attached trace default selectors inside `trace_query`. | `path=""`, `.`, repo root, and workdir now resolve to the attached trace when one exists; explicit trace files remain `source=path`; arbitrary directories fail fast with a typed repair hint instead of causing repo-source exploration drift. |
| Batch U9b | delivered | Make source-class inventory authority executable before absence closure. | Typed root source-enumeration `repo_map(source_inventory)` calls merge bounded supported-language auxiliary source classes through the shared `SourcePathRole` universe, restore the default graph afterward, stamp class-universe provenance, and exact absence/final-answer gates now consume the source-class universe before allowing source-family absence. |
| Batch U9c | delivered | Prevent premature source-inventory closure from model-authored member sets. | A typed source-inventory lane can no longer skip executable `repo_map(view="source_inventory")` just because the model emitted a principal `member_set`. The pre-complete gate now requires typed lens execution/provenance first, then accepts the model-authored member/absence handoff. This closes the ArkTS failure mode where parser-helper Go files were handed off as a complete set while repo-owned corpus sources were never queried. |
| Batch U9d | delivered | Add typed SourceClassUniverse matrix. | `SourceInventoryObservation` now carries `source_classes[]` computed from repo-tracked paths through the existing `SourcePathRole` authority, including production/test/fixture/example/documentation/prompt_support/thirdparty/vendor/generated. Class-only observations stay active without member rows, render `source_classes`, and no longer rely on the retired parallel `sourceInventoryAuxiliarySourceClass` taxonomy. |
| Batch U9e | delivered | Consume SourceClassUniverse in handoff/path-discovery/final-answer lanes. | Turn A snapshots preserve `SourceInventoryObservation`, extractor slates render `source_classes`, targeted root `list_files(include/file_type)` empty results retry recursively, empty-primary targeted scans fall back to repo-owned auxiliary/corpus sources without reopening dependency/cache noise, and exact-absence preflight/final contract consume source-class counts directly. |
| Batch U9f | delivered | Add source-inventory narrowing authority first slice. | Broad no-row source-inventory repairs narrow to analyzer `required_files` / common scope, relax over-specific query text inside the typed scope when needed, and keep current-lens rendering separate from cumulative coverage state. Accepted-evidence/source-class scheduler narrowing remains tracked as the next source-inventory authority slice. |
| Batch U9j | delivered | Make source-scope production boundaries quote-backed and repair principal source negative channels. | `source_scope_profile.source_quotes[]` is schema/type supported and validated against the current request. For source-inventory/enumeration lanes, unquoted production scope cannot hide existing auxiliary/corpus source paths through `irrelevant_files`; those paths promote to required files and synthesize all-scope when needed. ArkTS U9j passes and manual audit confirms 4 `@Entry` rows + 2 `@Builder` rows are rendered. |
| Batch U9k | delivered | Preserve typed repair-required tools in restricted explorer surfaces. | No-emit escalation now keeps `emit_*` plus active `RepairDirective.Tools`, so source-inventory completion repair can execute `repo_map` without reopening unrelated broad tools or lying to the model about unavailable tools. Runtime validation and schema filtering use the same typed surface. |
| Batch U9i | planned | Add cross-language source-inventory member/facet projection. | `repo_map(source_inventory)` returns typed member rows or explicit `no_member_parser` coverage for supported-language constructs across Go, Python, JS/TS, Ruby, Java/Kotlin, C/C++, Rust, Cangjie, ArkTS, config, and workflow files. Decorators/annotations/modifiers/config facets are structured attributes, not rendered-prose hints. |
| Batch U9g | delivered | Share proof coverage across exploration siblings. | Read TurnA artifacts project a typed `ProofSnapshot` into the lane-neutral loopkernel proof/truth authority and ReasoningGraph audit view. Parallel explore dispatch now consumes covered proof snapshots by typed lane ownership key to cancel equivalent support siblings; weak proof without owner localization or complete inventory does not satisfy handoff. |
| Batch U9h | partial | Unify tool JSON repair and accepted-evidence handoff. | Delivered: `ToolHandoffCarrier` is projected from typed tool repair, plan repair packs, observation refs, accepted evidence refs, and schema-derived JSON surface descriptors; BaseAgent, forced-read injection, Mutable dispatch buffers, TurnA snapshots, fork merge, and Turn-A handoff bounds all preserve the carrier without parsing summaries. Extractor/finalizer prompts render a bounded typed carrier view and intentionally omit repair hints/tool summaries. ReasoningGraph emits `tool_handoff_projected` with typed field/evidence/observation IDs. The emit-tool schema descriptor registry is compiled from each tool's `Parameters()` and attached through `ToolRepair.Metadata`, not handwritten prompt prose. Remaining: status-card carrier projection. |
| Batch U9t | partial | Extract source-inventory execution budget kernel. | Delivered: replaced the private per-role `sourceInventoryCandidateBudget` helpers with one typed execution budget object, moved budget/page/cursor authority into `internal/tool/sourceinventory`, moved scoped sorted file materialization/language cache/file-set/complete state into `sourceinventory.ExecutionView`, and set the zero-value budget bypass ratchet to 0. File/config/package/graph candidate builders and completion support consume this API instead of open-coded scan/materialization/deadline checks. Remaining: implement true cursor-backed resumable candidate pagination before full attribute/member materialization. |
| Batch V | partial | Add aggregate negative-fact canonicalization and repair hints. | V1 delivered runtime/log/trace missing-signal carriers and typed default runtime scope. V2 remains for the broader canonical repair-hint layer across repo no-hit, artifact no-hit, and exact empty sets. |
| Batch V2 | planned | Complete aggregate negative-fact repair hints. | Invalid no-hit payloads receive one precise typed repair hint or deterministic normalization before retry; the model does not bounce between `negative_search`, `negative_observation`, `scalar_value`, and empty `member_set` schemas. |
| Batch W | delivered | Add lazy repo-index warmup for runtime-artifact-only read turns. | Runtime trace/log answers with no typed current-source obligation do not print or pay repo-index warmup unless a later typed source route actually opens. |
| Batch X | planned | Add requested-dimension answer-surface scaffold. | First finalizer draft receives the typed section/dimension skeleton, reducing predictable finalizer patch rounds. |
| Batch Y | partial | Centralize RawRequest-derived signals into typed request authority. | Trace platform/flavor overrides, coherence mention signals, and similar raw-text helpers are migrated behind typed profiles or exact provenance extractors with hygiene tests blocking new hard gates over raw user words. |
| Batch Y1 | delivered | Move finalizer runtime trace guidance off RawRequest / Objective wording. | Runtime Trace Answer Guidance now renders from typed observation ledger and PerfBundle observations only. Raw question words or attachment spelling alone cannot trigger Harmony/scheduler guidance. |
| Batch Y2 | partial | Complete production RawRequest signal authority audit. | Remaining production RawRequest reads are classified as exact provenance, path/artifact tokenization, typed analyzer echo, or unsafe semantic routing; unsafe reads move behind typed profiles or soft advisory views with structural tests preventing new hard gates. |
| Batch Y2a | delivered | Remove trace_query platform/flavor selection from raw user wording. | `trace_query` now selects platform/flavor only from typed tool parameters, attached-source metadata, or content detection. Raw request words cannot choose Harmony/Android/Donghu semantics. |
| Batch Y2b | delivered | Finish first hard-ish production prose-signal cleanup pass. | Binder relevance no longer consumes task objective prose; remaining RawRequest/Objectives are classified as exact provenance/path extraction, typed analyzer echo, or follow-up candidates for RequestSignalAuthority. |
| Batch Z | planned | Add observation-only finalizer contract specialization. | Runtime/log/trace answers without a current-source lane receive compact runtime-artifact answer contracts, avoid source-rule prompt noise, and pass without deterministic metadata auto-repair in representative cases. |
| Batch J | partial | Re-run the representative batch and refresh this ledger. | Batch3 is recorded here. Remaining failures now map to RNE-C23/RNE-C32 rather than only empty-set schema loops; another full refresh is required after Batch U1 and Batch M/K2. |

## Test Matrix

- Unit: aggregate fact normalization accepts exact empty `member_set`.
- Unit: exhaustive handoff accepts a principal empty `member_set` with typed
  negative/no-hit evidence.
- Unit: positive `member_set` still rejects missing members or mismatched
  cardinality.
- Unit: relation support resolver accepts deterministic evidence/location
  matches and rejects unsupported members.
- Unit: repeated downgrade fingerprints are stable and delta-aware.
- Unit: final system supplements are suppressed when principal citations and
  owner-supported localization already prove the main answer, but still render
  for observed-only or missing-owner answers.
- Unit: degraded termination caveats are suppressed only for grounded principal
  answers with supported localization and remain visible for observed-only
  localization.
- Unit: tool/preflight timing and cache keys are dispatch/version scoped.
- Unit: runtime timing log summaries are compact and capped so telemetry does
  not become a new prompt/status noise source.
- Unit: a required `current_key_code` dimension plus typed source scope requires
  the current-source lane for external observations.
- Unit: pure runtime metric dimensions remain source-optional under default
  external-observation policy.
- Unit: explicit `exclude` still overrides source-scope drift and preserves
  observation-only behavior.
- Unit: `external_only_log` / `external_only_trace` waiver is ignored when the
  typed current-source lane is required and no current-source coverage exists.
- Unit: read localizer follow-up demotes navigation-only debt after owner
  evidence coverage, while explicit missing owner paths still block.
- Unit: pre-finalize Tier-1 floor proceeds when current-source owner evidence
  exists and only repo_map lens debt remains.
- Unit: final answer mutation still stamps repo_map navigation coverage for
  audit, but does not stamp a blocking localizer follow-up after owner evidence.
- Unit: runtime-artifact-only trace tasks do not require repo_map/localizer
  coverage when the typed request model has no current-source obligation.
- Unit: runtime-artifact trace tasks that explicitly require current-source
  coverage still keep repo_map/localizer pre-finalize and final-supplement
  obligations.
- Unit: trace causal projection binds a single primary root-cause node, direct
  wait node, ordered causal chain, and drilldown boundary from structured
  trace observations.
- Unit: trace causal projection prefers attributable wakeup-chain nodes over
  aggregate sentinel subjects such as `unknown-thread`, and deduplicates
  repeated supporting hop rows.
- Unit: runtime-artifact source-optional answers suppress final source
  localization, owner-anchor, repo_map navigation, and localizer follow-up
  supplements even when optional source exploration happened.
- Unit: source inventory reports scope classes for product, test, fixture,
  corpus, vendor, generated, config, and workflow files across supported
  languages.
- Unit: source inventory obligations are created from typed source-inventory,
  absence, exhaustive-set, and supported-language scope profiles; they are not
  created from user-word keyword matches or model rationale.
- Unit: source inventory obligation execution deduplicates existing repo_map
  source-inventory observations and never re-runs an identical lens after no
  typed input changed.
- Unit: typed source-enumeration requests that lack `source_inventory_profile`
  synthesize an advisory-only inventory profile from analyzer navigation query
  terms, not from raw user words or model prose.
- Unit: query-filtered source inventory treats supported-language tokens as a
  typed language filter and excludes parser/helper rows from other languages.
- Unit: synthetic source-inventory lanes participate in the same executable
  lens gate as explicit analyzer profiles.
- Unit: system-compiled source-inventory advisory/observation provenance
  satisfies the executable lens gate without requiring a duplicate model
  `repo_map` call.
- Unit: repository-wide typed query/root-scope source-inventory lanes include
  fixture, corpus, testdata, example, and vendored source classes even when the
  analyzer defaulted `source_scope_profile` to production.
- Unit: explicit production source-inventory profiles still exclude auxiliary
  source classes from principal candidates.
- Unit: absence answers cannot close if the typed source inventory finds
  matching files inside a required source-scope class.
- Unit: source-inventory `exact_resolution.status=absent` cannot close from a
  generic negative-scope citation while `SourceInventoryObservation.source_classes`
  shows an open repo-owned source-class universe and no complete empty principal
  source-inventory set exists.
- Unit: source-inventory member projection reports either bounded typed members
  with facet attributes or explicit `no_member_parser` coverage for every
  supported language family; an empty parser result must not silently conflict
  with positive read-file evidence.
- Unit: source-inventory-shaped turns mark pre-scan no-hit/list summaries as
  advisory until the typed inventory obligation is satisfied.
- Unit: file-family absence closure rejects path-filtered grep-only no-hit
  proof, accepts recursive `list_files` with matching include/file_type, and
  requires `include_auxiliary=true` when repo-owned auxiliary/corpus trees are
  part of the repository universe.
- Unit: `repo_map(view="source_inventory", roles=["file"])` refuses before
  graph/index work when `file` is the only primary role, while bounded
  file-local attribute expansion and `config_file` inventories still work.
- Unit: large root-scope `source_inventory` calls normalize broad parameters
  before lens execution by setting a bounded default `top_n`, clamping
  explicit oversized `top_n`, and disabling row-local attributes until a
  narrower scope is provided.
- Unit: recursive `list_files` with `include` or `file_type` outputs matching
  files only; directory rows, including directories whose names match the glob,
  are traversal-only and do not appear as results.
- Unit: empty source-inventory completion accepts a principal
  `member_set(value="0", members=[])` when paired with typed zero-result
  support and does not reject solely because contextual supporting evidence was
  emitted.
- Unit: if a completion downgrade requires a non-emit tool, the structured
  repair declares that tool and the completion-only surface exposes only
  `emit_*` plus the declared repair tools instead of asking the model to
  satisfy an unavailable instruction or reopening broad exploration.
- Unit: the same typed repair-required tool overlay applies to no-emit
  escalated explorer surfaces, and runtime boundary validation rejects
  unrelated broad tools while allowing the declared repair tool.
- Unit: large source-inventory observations compile to one set-level record plus
  bounded member/attribute samples before prompt budgeting; set count and
  truncation notes survive for audit/status consumers.
- Unit: source-inventory fresh-start explorer prompts render the principal
  typed slate before breadth scan / grep / parser-helper exploration guidance.
- Unit: analyzer search terms become secondary grep fallbacks when a principal
  source-inventory slate is active.
- Unit: `section` blocks with structured `items[]` participate in principal
  enumeration row coverage and exhaustive member coverage, not just
  table/ordered/bullet blocks.
- Unit: a grounded principal enumeration section suppresses duplicate
  system member supplements, source-localization/repo_map/localizer user
  supplements, and the generic degraded-floor disclosure.
- Unit: ordinary observed-only non-enumeration answers still show degraded
  termination / localization warnings when owner proof is missing.
- Unit: negative search / negative observation / exact empty member-set facts
  are canonicalized without requiring model prose repair loops.
- Unit: runtime/log/trace negative observations can represent a missing event
  field or missing same-window detail through typed runtime dimensions without
  requiring repo-search query/pattern fields.
- Unit: analyzer graph normalization and required-file projection skip eager
  repo graph loading for runtime-artifact requests whose current-source lane is
  typed optional, while ordinary source requests and required current-source
  dimensions still load the graph.
- Unit: runtime-artifact deferred multi-repo focus renders an advisory source
  note and does not warm a graph before a typed source route opens.
- Unit: observation-only finalizer contracts hide source-specific citation,
  member-set, repo_map, and absence-search rules unless the active answer
  surface includes a current-source lane.
- Unit: accepted-closure advisory retry/pending-read skips emit at most one
  progress-class user notice and never render as a retry notice.
- Hygiene: hard gates and scheduler decisions do not read `RawRequest`,
  user-word substrings, model rationale, visible thinking, or rendered summary
  text directly; any legitimate raw text extraction must enter through typed
  request profiles or exact provenance/path-token parsers first.
- Hygiene: hypothesis/task binding relevance is invariant to task objective
  prose and changes only when typed search hints or typed falsification kind
  change.
- Eval: `arkts_repomap` no longer loops on empty set shape.
- Eval: `arkts_repomap` uses repo_map/source_inventory before absence or
  member-set closure and reports the exact source-scope classes searched.
- Eval: source-inventory telemetry distinguishes explicit lens calls from
  system advisory projections and executable gate satisfaction.
- Eval: `qf_relation_subagent_registry` converges without repeated identical
  relation support downgrade.
- Eval: `trace_query_wakeup_causal_io_chain` preserves intermediate path roles.
- Eval: runtime-artifact trace reruns produce no `read_file` / `repo_map` tool
  calls and no source-localization / repo_map user-facing supplements when
  current-source evidence is not typed-required.
- Eval: runtime-artifact trace reruns produce no user-visible repo-index
  progress before a typed current-source route opens.
- Eval: `read_combo_log_current_code_dimensions` reads current-source anchors
  and cites repo lines instead of answering observation-only.
- Eval: architecture inventory and C++ call-chain cases stay bounded in reads,
  context tokens, and explorer iterations.

## Progress Ledger

- 2026-06-20 Batch A delivered: representative eval results recorded. First
  implementation target selected as typed empty `member_set` because it is a
  structural contradiction that can cause unbounded retries across languages and
  no-result repository searches.
- 2026-06-20 Batch B delivered: exact empty `member_set` is now a first-class
  typed aggregate. Exhaustive completion accepts it only when paired with
  `negative_search` or `negative_observation`; positive empty member sets still
  reject, and unsupported empty sets soft-downgrade instead of closing.
- 2026-06-20 Batch B verification: `arkts_repomap` rerun passed under
  `eval/results/goal-stagehandoff-20260620-rerun` with the new binary. This
  rerun exercised positive ArkTS fixture enumeration rather than the exact
  empty-set branch, so the no-hit branch is covered by focused unit tests. The
  eval still took 215s, 18 `read_file` calls, 19 explorer iterations, and ~48k
  estimated context tokens, so Batch D/E remain P0 for commercial smoothness.
- 2026-06-20 Batch C1 delivered: member-set support enrichment now auto-fills
  a generic `Member @ file:line` support ref from a unique matching `read_file`
  line for non-relation bare members. This targets the common owner/value split
  where the citable function definition line is already present but the answer
  member is returned or assigned on the next line. Ambiguous multi-match
  read-file surfaces still require an explicit model-authored support ref.
- 2026-06-20 Batch C1 verification: `qf_relation_subagent_registry` rerun
  passed instead of running away, but still took 247s with 14 `read_file` calls,
  24 explorer iterations, 13 midloop injections, 45 concrete values, and ~58k
  estimated context tokens. The log also showed extractor-stage unavailable
  `read_file` attempts from post-complete localization repair text, now tracked
  as RNE-C10 / Batch H.
- 2026-06-20 Batch H delivered: retry hints now carry a typed owner stage.
  Explore-owned forced-read/window hints no longer render in the extractor
  prompt after the pipeline moves forward; extractor-owned retry hints still
  work. `qf_relation_subagent_registry` rerun passed with
  `unavailable_tool_attempts=0` and answer-contract violations=0. The run still
  took 187s with 12 reads, 19 explorer iterations, 8 midloop injections, 42
  concrete values, and ~52k estimated context tokens. It also showed stale
  final system supplements, now tracked as RNE-C11 / Batch I.
- 2026-06-20 Batch I delivered: final-answer last-mile supplements now pass
  through a typed principal-source-surface gate. When the user-visible
  principal answer already has valid citation refs and owner-supported
  localization, read-mode localization status, repo_map coverage, localizer
  follow-up, owner-anchor tables, and degraded-termination floor caveats no
  longer append to the user surface. Observed-only or missing-owner answers
  still preserve the relevant warnings. This fixes the stale "localization
  needed" / generic low-proof noise without hiding incomplete proof debt.
- 2026-06-20 Batch I verification: `qf_relation_subagent_registry` rerun
  passed under `eval/results/qf_relation_subagent_registry-20260620-073640`.
  The final answer no longer renders user-facing localization/navigation/
  localizer/degraded-floor supplements after the cited principal answer. The
  run still took 192s with 7 reads, 6 `repo_map` calls, 16 explorer iterations,
  8 midloop injections, 40 concrete values, 32 answer-chain lines, and ~55k
  estimated context tokens. It also still performed repeated
  "verification not stable enough" repair rounds driven by a support-only
  `Resolution Chain anchor line is outside the fetched slices` directive. That
  residual confirms RNE-C1/RNE-C4/RNE-C6 remain open and should be handled by
  typed relevance budgets plus low-delta convergence guards rather than more
  final-render suppression.
- 2026-06-20 non-noise gap refresh: tool/preflight/context assembly latency is
  now tracked as RNE-C12 / Batch K. Noise remains the highest-priority user
  pain, but the commercial plan continues to cover convergence, relation
  support, trace projection, eval watchdogs, mixed runtime/source proof, and
  tool latency.
- 2026-06-20 Batch C2a delivered: accepted-closure advisory debt now uses one
  typed `RepairDebtClass` policy across auto-complete, fact-retry suppression,
  retry-hint rendering, and forced-read drains. `chain_promotion.*` support
  reads and Phase-1 breadth debt become advisory after accepted closure; exact
  primary anchors, required-file hints, and other principal blockers still
  block. The change is structural: it does not parse user intent keywords,
  model rationale, visible thinking, or rendered retry prose.
- 2026-06-20 Batch C2a verification: focused tests passed for
  `internal/types`, `internal/agent`, and `internal/orchestrator`; the wider
  package set `go test ./internal/types ./internal/agent ./internal/orchestrator`
  passed, and full `go test ./...` passed after the final retry-order
  refinement. The first rerun
  `qf_relation_subagent_registry-20260620-075206` proved the forced-read drain
  skipped advisory debt but still showed the model a stale Forced Read List,
  so the cleanup was moved before retry-hint rendering and stale retry
  carry-over is ignored when accepted closure leaves only non-blocking debt.
  The second rerun `qf_relation_subagent_registry-20260620-075655` passed with
  wall time 149s, `read_file=4`, `repo_map=1`, `explorer_iters=9`,
  `midloop_inject=4`, `answer_chain_lines=8`, and
  `answer_contract_violations=0`, down from the previous 240s / 14 reads / 7
  repo_map / 23 explorer iterations / 12 midloop / 28 chain lines. Logs confirm
  `Forced Read List` was no longer rendered for the support-only Resolution
  Chain debt and stale retry carry-over was ignored twice.
- 2026-06-20 remaining gaps after Batch C2a: the REPL output can still show
  "verification not stable enough" before the scheduler auto-completes the
  advisory retry, now tracked as RNE-C14 / Batch L. Context remains large
  (~58k estimated tokens) even after tool calls dropped, so RNE-C1/RNE-C12 and
  Batch D/K remain high-priority. Internal CGEC still records a non-blocking
  answer-document annotation violation even when eval answer-contract metrics
  are zero, now tracked as RNE-C15 / Batch M.
- 2026-06-20 representative six-case refresh:
  `eval/convergence_audit_summary_20260620_batch1.md` passed 5/6 cases and
  isolated `read_combo_log_current_code_dimensions` as a true mixed
  runtime/source lane failure. The failure had `read_file=0` and `repo_map=0`;
  analyzer emitted typed requested dimensions and source scope, but source-lane
  authority still collapsed the turn into observation-only. This is now tracked
  as RNE-C16 / Batch N.
- 2026-06-20 Batch N delivered: `RequestModel.CurrentSourceLaneDecision` now
  treats required `current_key_code` answer dimensions plus a valid typed source
  scope as a current-source requirement for external observations. The change
  consumes only typed analyzer fields and structured policy, not user keywords,
  model rationale, or visible thinking. Pure runtime metric dimensions still
  remain source-optional under default external-observation policy, and explicit
  source exclusion still wins.
- 2026-06-20 Batch N verification: focused `internal/types`, `internal/agent`,
  and `internal/tool` tests passed, and full `go test ./...` passed after the
  source-scope refinement. The representative
  `read_combo_log_current_code_dimensions` rerun passed under
  `eval/results/read_combo_log_current_code_dimensions-20260620-083333` with
  actual current-source work (`read_file=24`, `repo_map=4`) and source citations
  in the final answer. Residual cost remains high (`explorer_iters=56`,
  `midloop_inject=18`, `max_context_tokens_est=73423`, finalizer 2 rounds), so
  stale navigation debt, requested-dimension coverage drift, runtime language
  metadata hygiene, and sibling proof sharing are tracked as RNE-C17 through
  RNE-C20 for the next batches.
- 2026-06-20 Batch O delivered: read localizer follow-up derivation now treats
  navigation-only repo_map lens debt as advisory once the same TurnA artifacts
  contain typed owner-level current-source evidence. Explicit
  `MissingPaths` / `OwnerMissingPaths` remain blocking, and repo_map navigation
  coverage is still stamped on the answer document for audit. This keeps the
  useful coverage ledger while preventing post-completion "verification not
  stable enough" requeues caused only by a stale missing lens. Focused
  `internal/types`, `internal/orchestrator`, and `internal/tool` tests passed;
  full regression and representative eval rerun follow this ledger update.
- 2026-06-20 Batch O verification: focused
  `go test ./internal/types ./internal/orchestrator ./internal/tool -run
  'ReadLocalizerFollowup|CheckTier1Floor_ReadLocalizer|ApplyAndPersistMutation_.*Localizer|ApplyAndPersistMutation_DemotesNavigationOnly'`
  passed, and full `go test ./...` passed. The six-case representative rerun
  is recorded in
  `eval/convergence_audit_summary_20260620_batch2_after_batch_o.md`: 4/6 cases
  passed. `read_combo_log_current_code_dimensions` improved from
  24 reads / 4 repo_map / 56 explorer iterations / 18 midloop injections to
  11 reads / 1 repo_map / 11 explorer iterations / 7 midloop injections, and
  the pre-finalize localizer requeue no longer appeared for that case. The
  rerun exposed four broader gaps now tracked as RNE-C21 through RNE-C24:
  runtime-only trace tasks still received source navigation debt,
  trace root-cause rank was not first-class in the final answer, ArkTS source
  inventory falsely answered absence despite in-repo `.ets` corpus files, and
  empty/negative aggregate facts still produced schema repair churn. Noise
  remains the highest priority, but these non-noise gaps are part of the same
  commercial delivery ledger and will be fixed in separate typed-authority
  batches rather than as eval-specific patches.
- 2026-06-20 Batch S delivered: added shared
  `RuntimeArtifactReadSourceNavigationNotRequired` authority and consumed it in
  the pre-finalize Tier-1 localizer floor plus final-answer navigation/localizer
  stamping. Attached trace / runtime-artifact observation-only tasks no longer
  produce source-navigation requeues or source-localizer user supplements when
  the typed current-source lane is not required. The reverse path is pinned:
  a runtime artifact with a required `current_key_code` dimension and typed
  source scope still keeps repo_map/localizer obligations. Focused tests passed
  for `internal/types`, `internal/orchestrator`, and `internal/tool`.
- 2026-06-20 Batch S verification: full `go test ./...` passed. The targeted
  representative rerun
  `eval/convergence_audit_summary_20260620_trace_after_batch_s.md` passed
  `trace_query_wakeup_causal_io_chain` with `read_file=0`, `repo_map=0`,
  `midloop_inject=0`, `finalizer_iters=1`, and no flags. Log audit found no
  `pre-finalize read localizer follow-up`, no repo_map navigation supplement,
  and no read-localizer supplement. The broader trace principal-root-cause
  projection work remains tracked separately as RNE-C22 / Batch T.
- 2026-06-20 Batch T delivered: added typed `TraceCausalProjection` over
  structured `trace_query` observation records and final-answer source
  supplement gating for runtime-artifact answers without a current-source hard
  requirement. The projection ranks attributable wakeup-chain nodes ahead of
  aggregate sentinel rows such as `unknown-thread`, compares impact within the
  same attribution class, and deduplicates repeated supporting hop rows. Final
  answer mutation now suppresses source localization / owner-anchor /
  repo_map-navigation / localizer-follow-up supplements for source-optional
  runtime-artifact answers while preserving the full TurnA artifacts for audit.
- 2026-06-20 Batch T verification: focused tests passed for
  `internal/types` and `internal/tool`, and full `go test ./...` passed. The
  targeted trace rerun
  `eval/convergence_audit_summary_20260620_trace_after_batch_t2.md` passed
  with `read_file=0`, `repo_map=0`, `list_files=0`, `explorer_iters=3`, and
  `midloop_inject=2`. A log audit found no source-localization, repo_map
  navigation, localizer follow-up, or `Resolution Chain anchor` user-facing
  supplements. The run still printed repo-index warmup progress before source
  lane suppression and needed a second finalizer patch for requested answer
  dimensions, now tracked as RNE-C25 / Batch W and RNE-C26 / Batch X.
  Scattered `RawRequest` lexical helper usage was also refreshed into this
  ledger as RNE-C27 / Batch Y so the follow-up work covers red-line hygiene
  alongside noise and convergence.
- 2026-06-20 Batch W delivered: runtime-artifact read turns now defer run-entry
  multi-repo graph warmup when the typed route starts from an attached runtime
  artifact, and analyzer post-`emit_analysis` source graph normalization /
  required-file projection reuse the shared runtime-artifact source-navigation
  authority before eager-loading repomap. Existing loaded graphs can still be
  reused; ordinary source questions and runtime-artifact questions with a
  required current-source dimension still load source graphs. Focused tests
  passed for `internal/types`, `internal/agent`, and `internal/orchestrator`,
  and `make build` succeeded.
- 2026-06-20 Batch W verification: targeted
  `trace_query_wakeup_causal_io_chain` rerun
  `eval/convergence_audit_summary_20260620_trace_after_batch_w3.md` passed with
  `read_file=0`, `repo_map=0`, `list_files=0`, `source_lens=0`,
  `analyzer_iters=1`, `finalizer_iters=1`, and no user-visible `仓库索引`,
  `repo_map: phase`, or source tool calls in `run-1.out`. Log audit found only
  the expected `source graph eager load skipped` debug entries. The same rerun
  still had `explorer_iters=5`, one `contract_warning`, a
  `negative_observation` schema repair rejection, and deterministic finalizer
  observed-artifact carrier auto-repair; those are now tracked separately as
  RNE-C28 / Batch V follow-up and RNE-C29 / Batch Z.
- 2026-06-20 Batch V1 delivered: runtime/log/trace `negative_observation`
  facts now canonicalize typed missing-signal carriers such as
  `missing_signal`, `missing_event`, and `missing_field` into the absent-target
  axis, and `emit_investigation_complete` adds a bounded runtime scope from
  typed attached-log / attached-trace / runtime-artifact context. Ordinary
  source no-hit proof still requires explicit repo/source scope, and optional
  malformed aggregate facts without an absent target continue to be dropped
  rather than accepted. This closes the trace missing-IO-layer schema rejection
  path without using user-intent keywords, model rationale, visible thinking,
  or rendered prose as control flow. The broader aggregate repair-hint layer
  remains tracked as Batch V2.
- 2026-06-20 Batch V1 verification: focused `internal/types` and
  `internal/tool` tests passed, full `go test ./...` passed, and the targeted
  `trace_query_wakeup_causal_io_chain` rerun
  `eval/convergence_audit_summary_20260620_trace_after_batch_v1.md` passed with
  `read_file=0`, `repo_map=0`, `list_files=0`, `source_lens=0`,
  `analyzer_iters=1`, `explorer_iters=2`, `midloop=0`, `finalizer_iters=1`,
  and no flags. Log audit found no `negative_observation` target/schema
  rejection, no source tool calls, and no user-visible repo-index progress.
- 2026-06-20 Batch H2 delivered: `BuildPromptContext` now projects every
  model-facing Retry Directive through the current skill `ToolSuggestions`
  before rendering. If a stage-owned retry hint mentions known tools that are
  not exposed in the current stage, the prompt receives a stage-safe instruction
  to use only available structured emit/render tools and preserve missing proof
  as uncertainty or caveat. Explorer hints that mention available explorer tools
  remain byte-for-byte unchanged. This closes the residual "investigation
  complete -> extraction tries unavailable repo/read tools" prompt path without
  treating user intent, model rationale, visible thinking, or free-form retry
  prose as hard control flow.
- 2026-06-20 Batch H2 verification: focused `internal/context`,
  `internal/types`, and `internal/agent` tests passed, full `go test ./...`
  passed, and `qf_relation_subagent_registry` reran successfully in
  `eval/convergence_audit_summary_20260620_qf_relation_after_batch_h2.md`.
  The extraction stage made exactly one structured
  `emit_hypothesis_verdict` call and log audit found no unavailable
  `repo_map` / `read_file` tool attempt in extraction. The rerun still reports
  a non-blocking `contract_warning` from finalizer-side
  `answer_prose_density` and `block_kind_vs_lane_allowed`; that residual is
  tracked by RNE-C15 / RNE-C29 rather than by the stage retry projection batch.
- 2026-06-20 non-noise scope refresh: this ledger is not limited to noise
  cleanup. Noise remains P0 because it is the most visible cause of loops and
  context bloat, but typed localization/proof coverage, handoff fidelity,
  prompt/tool-surface hygiene, performance observability, status-card UX, and
  cross-language source inventory remain tracked as separate commercial
  delivery classes.
- 2026-06-20 Batch Y2b delivered: `analysis/binder` relevance no longer reads
  task node objective prose. Hypothesis binding now uses typed search hints
  plus typed falsification-kind affinity only, with fallback binding semantics
  preserved for coverage. This closes a hard-ish scheduling path from
  model/template natural language without using user-intent keywords, model
  rationale, visible thinking, or rendered summaries as logic.
- 2026-06-20 Batch L delivered: accepted-closure advisory debt skip decisions
  now emit at most one progress-class user notice (`NoticeInvestigationReady`)
  instead of leaving users with only a stale retry impression. The emit sites
  are the typed scheduler decisions that clear stale retry carry-over or
  advisory pending reads; they do not inspect user prose, model prose, visible
  thinking, or retry text.
- 2026-06-20 Batch K1 delivered: tool runtime timings now also produce compact
  debug logs and slow-path warnings (`[tool_timing]`) from the shared
  `attachToolRuntimeTimings` helper. This makes slow `emit_evidence` /
  `emit_investigation_complete` phases visible in ordinary logs without making
  timing a semantic hard gate. Existing static schema caches, grounding
  context cache, and completion preflight view remain active; K2 continues to
  track deeper context/preflight reuse and "organizing context" latency.
- 2026-06-20 representative six-case refresh Batch3 recorded:
  `eval/convergence_audit_summary_20260620_batch3.md` passed 4/6 cases.
  `read_combo_log_current_code_dimensions` failed with zero source reads and
  zero repo_map calls, exposing a second mixed-lane authority bug where
  artifact-citation external-only/exclude drift could close a required
  `current_key_code` dimension. `arkts_repomap` failed with `repo_map=0`,
  `source_lens=0`, and an absence answer after list/read exploration,
  confirming that source inventory is still advisory rather than an executable
  typed authority. `sr_cpp_virtual_chain` passed but emitted seven contract
  warnings, keeping final contract severity split in Batch M. The passing
  relation/architecture cases still used large contexts, so RNE-C1/RNE-C4/
  RNE-C6/RNE-C12 remain active. This refresh explicitly keeps non-noise gaps
  in scope: localization authority, source inventory, citation/contract
  telemetry, proof coverage, and performance are tracked beside noise.
- 2026-06-20 Batch N refinement delivered: `current_key_code` requested-answer
  dimensions now remain a current-source requirement even when an attached
  runtime artifact uses external-only artifact citations. Artifact lines still
  cannot be borrowed as current-source citations, but that citation-provenance
  policy no longer closes a separate typed current-source answer dimension.
  Weak source-scope drift can still be overridden by explicit typed source
  exclusion; the required current-source dimension path is stronger. Focused
  tests passed for `internal/types`, `internal/agent`, and `internal/tool`,
  and `read_combo_log_current_code_dimensions` reran successfully under
  `eval/results/read_combo_after_lane_fix_20260620/read_combo_log_current_code_dimensions-20260620-105809`
  with `read_file=5`, `repo_map=1`, `investigation_complete_calls=1`, and
  current-source citations. Residual 242s wall time, about 66k context tokens,
  and one answer-richness contract warning stay open under RNE-C12/RNE-C15/
  RNE-C26/RNE-C29.
- 2026-06-20 source inventory gap split: RNE-C23 keeps the scope-class and
  supported-language inventory model; new RNE-C32 / Batch U1 tracks the
  scheduler/localizer side that must turn source-inventory, absence,
  exhaustive-set, and supported-language scope profiles into a bounded typed
  `repo_map(source_inventory)` obligation before closure. This is language
  neutral and covers C/C++, Cangjie, ArkTS/ETS, JS/TS, Java/Kotlin, Ruby, Go,
  config, workflow, and all other repomap-supported surfaces. It must not be
  implemented with user-keyword matching or model-prose routing.
- 2026-06-20 Batch U1a delivered: source-inventory advisory generation now
  synthesizes an advisory-only profile for typed source-enumeration queries
  when the analyzer omitted optional `source_inventory_profile`. The synthetic
  lane is query-filtered by supported language and structural symbol surface,
  auto-publishes a system source-inventory observation before exploration, and
  shares the same pre-complete executable lens gate as explicit profiles.
  Analyzer L0B cardinality rejection now exempts `IntentEnumerate` source
  inventory shapes so member discovery can happen after classification.
  ArkTS decorator entries expose structural `@Entry` / `@Component` /
  `@Builder` / `@Styles` / `@Extend(...)` surfaces for inventory matching.
- 2026-06-20 Batch U1b delivered: repository-wide typed query/root-scope
  inventory lanes no longer inherit production-only narrowing from a default
  analyzer `source_scope_profile`. This is driven by typed source-inventory
  provenance (`typed_source_enumeration_query` + `query_root_scope`) and keeps
  explicit production inventory profiles production-filtered. Focused tests
  cover the synthetic all-source-class path and the explicit production guard.
- 2026-06-20 source-inventory eval refresh: the latest ArkTS runs after query
  filtering and auto-observe still failed before the final gate/scope fixes:
  `eval/results/arkts_after_inventory_authority_batch_20260620/arkts_repomap-20260620-115049`
  showed the system compiling an advisory/observation, but the explorer still
  read parser/helper support files and the pre-complete gate did not yet count
  system advisory provenance as an executed lens. The gate/provenance and
  root-scope fixes above close those two structural holes, but RNE-C36/RNE-C38
  remain open because model-facing exploration still needs a principal
  localization slate and pre-scan no-hit priors must stay advisory until typed
  inventory authority has run.
- 2026-06-20 slow completion refresh: the same ArkTS log showed
  `emit_investigation_complete` calls around 15s with
  `aggregate_normalization` as the slowest phase. Batch K1 made this visible;
  Batch K2/RNE-C39 remains the commercial fix for cached
  `CompletionPreflightView`, aggregate-normalization reuse, and status-card
  timing explanations.
- 2026-06-20 Batch U4/U5 first slice delivered: explorer fresh-start prompts
  now render the typed source-inventory slate before breadth-scan guidance,
  generic grep/list exploration, parser/helper reads, and analyzer search-term
  suggestions. When that slate is active, search terms render only as
  secondary grep fallbacks and generic no-hit searches are labeled advisory
  until the typed slate is reconciled, contradicted, or explicitly scoped out.
  This is a model-facing typed projection change over existing
  `SourceInventoryAdvisory` / `SourceInventoryObservation`; it does not parse
  user prose, model prose, visible thinking, or rendered summaries. The same
  batch also made explorer keyword-search post-processing nil-safe when typed
  analyzer keywords exist but no repo search result is available. Remaining
  U4/U5 work: put this priority into shared `ObservationLedger` ranking,
  status cards, eval telemetry, and answer-side soft absence advisories.
- 2026-06-20 U4/U5 six-case representative refresh passed all six cases, but
  manual audit exposed new commercial gaps beyond noise: correct source
  inventory answers still showed duplicate system member supplements and stale
  localization/degraded-floor warnings; source-inventory telemetry under-counts
  system-compiled authority; broad architecture and mixed runtime/source cases
  still consume too many turns/tokens; trace answers can still show retry/tool
  surface residue. These are recorded as RNE-C40 through RNE-C43.
- 2026-06-20 Batch U6 delivered: final-answer coverage gates now treat
  `section` items as structured enumeration carriers. The same typed
  `grounded_principal_enumeration` view suppresses duplicate system补表,
  read-localization/repo_map/localizer supplements, and the generic
  degraded-floor disclosure when every visible principal enumeration item has
  a valid citation. Non-enumeration observed-only answers keep the existing
  warning behavior. Focused tests cover principal evidence, exhaustive member
  coverage, row compiler suppression, last-mile read supplements, and
  termination disclosure.
- 2026-06-20 Batch U7a/U7b delivered: source-family discovery is now exposed
  through `list_files(include=..., file_type=..., include_auxiliary=...)`,
  with grep/exec soft advisories when a content search or shell `find` is
  being used as path discovery. `emit_investigation_complete` now has a typed
  file-family absence-proof gate: for typed source-inventory/enumeration
  closures, a path-filtered grep no-hit cannot by itself close
  `negative_search` or exact empty `member_set`; the model must run recursive
  `list_files` with matching include/file_type or the executable
  source-inventory lens. If repo-owned auxiliary/corpus trees exist, the proof
  must explicitly opt into `include_auxiliary=true`. The implementation reads
  only tool-result banners, aggregate fact enums, typed request fields, and
  repository directory metadata; it does not parse user intent keywords,
  model rationale, visible thinking, or rendered summaries as hard logic.
  Focused tests passed for the new empty-proof gate, list_files filters, grep
  advisory, exec advisory, U6 enumeration coverage, last-mile supplement
  suppression, and degraded-floor disclosure suppression.
- 2026-06-20 Batch U7c delivered: the ArkTS eval rerun exposed a severe
  tool-boundary bug after the new absence-proof gate correctly rejected a
  grep-only no-hit closure. The model retried with
  `repo_map(view="source_inventory", roles=["file"], scope=".")`, which is
  semantically a file-path discovery request and drove high CPU for over
  thirteen minutes. `repo_map` now rejects sole-primary `file` role before
  graph load/index work and points to `list_files` for file-family discovery.
  The guard consumes only typed tool parameters; it does not inspect user
  prose, model rationale, visible thinking, or rendered summaries. Focused
  tests verify fast refusal and preserve existing bounded `file` +
  `attribute_roles` and `config_file` inventory behavior.
- 2026-06-20 Batch U7d delivered: the same bug also required an algorithm-side
  guard, not just model-call correction. Large root-scope source_inventory
  calls now get deterministic parameter-budget normalization before lens
  execution: unset `top_n` is bounded, explicit oversized `top_n` is clamped,
  and row-local attributes are disabled until a narrower scope is provided.
  The file-family absence-proof gate also runs before expensive completion
  preflight/source-inventory advisory work when typed tool results already
  prove the closure is invalid. Focused tests cover the budget guard and the
  existing path-family closure matrix. RNE-C48 remains open for deeper
  candidate-construction cancellation, progress checkpoints, and resumable
  per-role pagination inside the source_inventory engine itself.
- 2026-06-20 ArkTS U7d representative eval refresh:
  `eval/results/arkts_repomap-20260620-131827` failed after 362s with
  `missing:@Component` / `missing_section:01_entry_component_minimal.ets`, but
  it validated the immediate runaway fix: the model retried
  `repo_map(view="source_inventory", roles=["file"])` and the tool refused it
  before graph/index work instead of entering the prior high-CPU state. The run
  exposed two follow-up gaps. First, `list_files(recursive=true, include=*.ets)`
  rendered directory traversal rows, so the model misread the result as a
  generic directory listing and fell back to slow shell `find`; Batch U7f fixes
  that output semantics. Second, empty source-inventory closure looped through
  `absence`/`resolved`/`member_set` repairs and even emitted a missing
  `repo_map` instruction while the retry surface only exposed `emit_*` tools;
  Batch U7g tracks the scheduler/completion fix. The run also confirms that
  auxiliary ArkTS corpus surfaces such as
  `internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets`
  still need stronger typed scope preservation through source-inventory
  handoff.
- 2026-06-20 Batch U7f delivered: filtered `list_files` now outputs only
  matching files when `include` or `file_type` is present. Directories remain
  traversal state and are omitted from the result even if their names match the
  glob. The focused test adds a fake `*.ets` directory to pin that behavior and
  keeps unfiltered list_files behavior unchanged.
- 2026-06-20 Batch U7g delivered: empty source-inventory completion now accepts
  a principal `member_set(value="0", members=[])` when paired with typed
  `negative_search` / `negative_observation` zero-result support, even if
  contextual grounded evidence was emitted to explain the empty boundary.
  `RepairDirective` also carries typed required tools; the completion-only
  explorer surface remains narrow but exposes those tools, so a mandatory
  source-inventory repair can call `repo_map` without reopening broad grep,
  read, or shell surfaces. Focused tests cover the absence/context evidence
  path and the typed repair tool-surface projection.
- 2026-06-20 post-U7g ArkTS eval refresh:
  `eval/results/arkts_repomap-20260620-134129` was manually terminated after
  sampling confirmed a new high-CPU hotspot. The old
  `repo_map(source_inventory, roles=["file"])` runaway did not recur. Instead,
  the process burned CPU in answer-surface observation compilation after a broad
  source-inventory observation with `member_rows=47117`; the sampled stack was
  `CompileObservationLedger -> compileSourceInventoryObservationObservations ->
  sourceInventoryObservationSupportRefs`. The run also showed that analyzer
  `list_files(include=*.ets)` proof was not reused as explorer completion proof,
  and the model still drifted into parser implementation evidence instead of
  the repository ArkTS corpus surface. Those localization/proof gaps remain
  tracked by RNE-C32/RNE-C36/RNE-C38/RNE-C44/RNE-C50.
- 2026-06-20 Batch U7h delivered: source-inventory observation-ledger
  compilation now budgets before record materialization. Set records preserve
  full `ResultCount` and row-set refs, while member records are capped,
  attribute records are capped, and set-level support refs are sampled with
  map-based dedupe. Focused tests cover a 200-member observation and confirm
  the ledger keeps count/truncation notes without expanding every row.
- 2026-06-20 U8 six-case representative refresh:
  `eval/results/readmode_rep_batch_u8_20260620_summary.md` produced 4 pass /
  2 fail and flagged every case for commercial-smoothness follow-up. The
  functional failures were `qf_relation_subagent_registry` (answer contract /
  explicit count surface drift) and `arkts_repomap` (manually terminated after
  stack sampling confirmed a new `source_inventory` candidate-construction
  hot path). The non-failing but material gaps were `qf_architecture`
  consuming 52 explorer iterations / 28 reads / 8 repo_map calls, trace-only
  completion reopening current-source reads before answering, and
  log+current-code completion requiring 18 explorer iterations plus finalizer
  auto-repair. These map to RNE-C1/RNE-C4/RNE-C12/RNE-C15/RNE-C20/RNE-C21/
  RNE-C26/RNE-C43/RNE-C48.
- 2026-06-20 Batch U7e first slice delivered: the U8 ArkTS sample
  `/tmp/codrax-arkts-sample-93149.txt` showed the process burning CPU in
  `sourceInventoryCandidateSets -> sourceInventoryGraphCandidates ->
  sourceInventorySourceInRequestedScope ->
  sourceInventoryUsesRepositoryWideTypedQueryLane`, with GC pressure from
  repeatedly cloning `SourceInventoryAdvisory` inside the symbol loop. The
  candidate builder now constructs one `sourceInventoryScopeFilter` per pass
  and reuses it across query-role discovery, lens scope filtering, file,
  package, symbol, and attribute scans. This is language-neutral and covers
  ArkTS/ETS, C/C++, Cangjie, JS/TS, Java/Kotlin, Ruby, Go, config, workflow,
  and all other repomap-supported source surfaces. It consumes typed IR,
  advisory provenance, source-scope fields, and graph indexes only.
- 2026-06-20 runtime trace source-lane refinement delivered: attached-trace
  observation-only completion now uses the same
  `runtimeArtifactGroundingBypassAllowed` decision as evidence-floor waiver
  handling before applying current-source forced-read gates. Pure runtime
  trace questions no longer reopen source reads after a successful
  `emit_investigation_complete`; typed current-code/source-scope dimensions
  still require source coverage. Focused tests pin both branches.
- 2026-06-20 Batch U7e/runtime refinement verification:
  `eval/results/arkts_repomap-20260620-141931` passed in 243s after the
  source-inventory scope-filter cache, with no renewed high-CPU runaway. The
  answer still required 13 explorer iterations, 7 reads, and 3 list calls, so
  RNE-C36/RNE-C38/RNE-C43 remain open for broader navigation/noise
  convergence. `eval/results/trace_query_wakeup_causal_io_chain-20260620-142254`
  passed in 166s with `read_file=0`, `repo_map=0`, and `trace_query=3`,
  confirming that pure attached-trace analysis no longer reopens source
  navigation after investigation completion.
- 2026-06-20 Batch U7e second slice delivered: `source_inventory` now covers
  both runaway dimensions. Tool-boundary preflight still corrects the known
  over-wide model call shape (`roles=["file"]` as sole primary role), while
  the lens algorithm itself now has typed, language-neutral materialization
  budgets for broad tool-lens calls. Candidate construction applies query and
  language filtering before budget accounting, caps per-role materialized rows
  from `top_n`/repo size, marks truncated observations incomplete, and renders
  `repo_lens:candidate_budget_truncated` guidance so the model cannot treat a
  bounded sample as exhaustive proof. Focused validation:
  `go test ./internal/tool -run
  'TestRepoMapSourceInventoryRejectsSoleFileRoleBeforeIndexWork|TestRepoMapSourceInventoryBroadRootBudgetGuard|TestPublishSourceInventoryObservationFromLens_BudgetsBroadCandidateMaterialization|TestPublishSourceInventoryObservationFromLens_ModelDrivenRolesAndScopes|TestPublishSourceInventoryObservationFromLens_UsesCrossLanguageRepoMapKinds|TestPublishSourceInventoryObservationFromLens_DefersBroadAttributesWithNarrowingHint'`
  passed. Remaining RNE-C48 work is cancellation/time-budget checkpoints and
  true resumable pagination.
- 2026-06-20 U9 six-case representative refresh recorded:
  `qf_relation_subagent_registry` and `qf_architecture` were correct but still
  over-budget for simple/broad answers; `arkts_repomap` failed by missing a
  repo-owned ArkTS corpus source; `cangjie_repomap` harness-passed but manual
  audit showed weak source-inventory usage; `trace_query_wakeup_causal_io_chain`
  failed when `trace_query(path=".")` drifted from attached-trace analysis into
  source file exploration; and `read_combo_log_current_code_dimensions` passed
  but duplicated proof work across siblings and hit schema/repair retries.
  These map to RNE-C43/RNE-C52/RNE-C53/RNE-C54/RNE-C55.
- 2026-06-20 Batch U9a delivered: `trace_query` now corrects typed default
  selectors for attached trace turns and refuses directory paths before parse
  work. The fix is deliberately parameter/state based: it reads only
  `source`, `path`, repo/work directories, attached trace blob/content, and
  filesystem metadata; it does not parse raw user wording or model rationale.
  Focused validation:
  `go test ./internal/tool -run
  'TestTraceQuery(AttachedTraceNormalizesDotPath|DirectoryPathWithoutAttachmentFailsFast|ExplicitFilePathIsNotOverriddenByAttachment|ExplicitPathProducesRuntimeArtifactSummary|AttachedSourceHintControlsPrioritySemantics|AttachedBlobPathInheritsAttachedSourceHint)'`
  passed.
- 2026-06-20 Batch U7i delivered: user feedback on the high-CPU
  `repo_map(view=source_inventory, roles=["file"], scope=".")` path was split
  into the two required dimensions. The tool boundary now corrects the model's
  over-wide call shape before index work, including broad root file-role calls
  that attach row-local `attribute_roles`. The lens engine now also has an
  independent scan budget in addition to visible-row materialization budget; a
  sparse/no-match broad scan records `complete=false` and
  `repo_lens:candidate_budget_truncated` instead of disappearing or becoming
  absence proof. Hard decisions consume typed tool params, graph size,
  role/scope/query values, and structured observation provenance only.
- 2026-06-20 U10 six-case representative refresh after Batch U9b:
  `trace_query_wakeup_causal_io_chain`, `qf_relation_subagent_registry`,
  `qf_architecture`, `cangjie_repomap`, and
  `read_combo_log_current_code_dimensions` passed; `arkts_repomap` failed.
  Manual audit: ArkTS did not call `repo_map` at all, closed from grep/list
  evidence, and rendered a false "no .ets source" conclusion while missing the
  repo-owned ArkTS corpus file
  `internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets`.
  Cangjie passed but needed 343s, 49 explorer iterations, 14 `read_file`
  calls, and 13 `list_files` calls; it also showed repeated
  `emit_investigation_complete` disagreement between absence/result/member-set
  semantics before one late `source_inventory` call. This proves the remaining
  RNE-C53 issue is not the `repo_map` algorithm alone: typed source-inventory
  execution must be a pre-complete authority and cannot be bypassed by a
  premature model-authored principal `member_set`.
- 2026-06-20 Batch U9c delivered: `sourceInventoryLensExecutionDowngrade` now
  evaluates the typed lens-execution gap before accepting any principal
  `member_set` handoff. A model can still author the final set or honest
  absence boundary, but only after the executable `repo_map(view="source_inventory")`
  lens has produced structured tool/advisory/observation provenance. Focused
  regression adds a premature ArkTS-like member-set handoff and verifies the
  investigation stays open with a typed `repo_map` repair directive.
- 2026-06-20 Batch U9d delivered: source-inventory now uses the existing
  `SourcePathRole` authority as the source-class universe instead of another
  local string taxonomy. `ClassifySourcePathRole` includes repo-owned
  `thirdparty`, `vendor`, and `generated`; `SourceScopeAuxiliary` covers those
  classes; `repo_map(source_inventory)` auxiliary projection reuses that
  classifier; and `SourceInventoryObservation.source_classes[]` is computed
  from `git ls-files` or a bounded filesystem fallback rather than the filtered
  search graph. Class-only observations stay active and render
  `source_classes: ...` with an explicit warning that no candidate rows is not
  absence proof. Focused validation:
  `go test ./internal/types ./internal/tool ./internal/tool/repomap -run
  'SourcePathRole|SourceScope|SourceInventory|RepoMapSourceInventory|EmitAnalysis|EmitInvestigationComplete_PreCompleteCheck_SourceInventory'`
  passed. Remaining RNE-C53 work is Batch U9e: make exact absence/final-answer
  gates consume this matrix so a bounded negative citation cannot silently prove
  only the production slice when repo-owned auxiliary classes are still in
  scope.
- 2026-06-20 Batch U9e first slice delivered: the RNE-C53 ArkTS truth path is
  now closed without prompt keyword routing. Targeted root `list_files` calls
  with `include`/`file_type` retry recursively when the root-only pass is empty;
  targeted recursive scans fall back to repo-owned auxiliary/corpus trees only
  when the primary scan is empty; dependency/cache noise stays filtered.
  Turn-A handoff now carries the full `SourceInventoryObservation`, and
  extractor slates render `source_classes` so the source-class universe is not
  lost after exploration. Focused tests passed for `ListFiles_*`,
  `TurnAHandoffPreservesSourceInventoryObservation`, source-path role/scope,
  source-inventory, repomap source-inventory, and pre-complete source-inventory
  gates. Full `go test ./...` and `make test` passed.
- 2026-06-20 Batch U9e eval verification: old-control reruns
  `eval/results/arkts_repomap-20260620-162740` and
  `eval/results/arkts_repomap-20260620-163343` failed by missing
  `01_entry_component_minimal.ets` / `@Component`. After the list_files
  recursive+auxiliary fallback and handoff fix,
  `eval/results/arkts_repomap-20260620-163935` passed. The analyzer-stage
  tool result now shows
  `[list_files: path=. recursive=true include=*.ets include_auxiliary=true]`
  and includes
  `internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets`;
  the later source-inventory lens renders
  `source_classes: production:827,test:949,fixture:85,prompt_support:1,thirdparty:14`.
  Residual commercial-smoothness debt remains: the passing run still took 235s
  / 15 explorer iterations / 3 repo_map calls because the broad source-inventory
  repair returned no rows before the model had to continue from already-read
  corpus evidence. This is tracked as RNE-C56 / Batch U9f.
- 2026-06-20 Batch U9f delivered first source-inventory narrowing slice and
  broad-navigation algorithm guard. A broad no-row `repo_map(source_inventory)`
  lens now narrows to typed analyzer `required_files` / common scope and, when
  the tool `query` is too specific, retries the same typed scope with the query
  relaxed. The renderer now returns the current lens view while MutableState
  still keeps cumulative source-inventory coverage, preventing old
  `list_files`/direct-child rows from suppressing no-row repair. Separately,
  broad root navigation-only source-inventory roles (`file`, `package`,
  `config_file`) bypass full reconcile and return a bounded incomplete sample
  with `repo_lens:broad_navigation_guard` / `repo_lens:candidate_budget_truncated`.
  Focused validation passed:
  `go test ./internal/tool ./internal/tool/repomap -run
  'ClassOnlyRenderExcludesPriorRows|AutoNarrowsBroadNoRows|BroadNavigationLensIsBounded|SourceInventory|RepoMapSourceInventory'`.
  The pre-U9f ArkTS rerun
  `eval/results/arkts_repomap-20260620-165135` passed with one explicit
  source-inventory lens, zero finalizer rejects, and no unavailable-tool
  attempts; a post-U9f rerun should verify whether the typed narrowing path
  fires in the live model loop, because that earlier run was started before the
  current-lens contamination fix.
- 2026-06-20 Batch U9f post-change eval verification:
  `eval/results/arkts_repomap-20260620-170353` passed with wall 210s,
  `explorer_iters=11`, `repo_map=3`, `source_inventory_lens=2`,
  `finalizer_rejects=0`, and no unavailable-tool attempts. The live logs
  confirm the new typed narrowing path fired:
  `source_inventory_narrowing: broad no-row lens -> typed required_files/common_scope`
  followed by
  `source_inventory_narrowing_query: relaxed broad query inside typed required_files/common_scope`
  and `repo_lens:auto_narrow_required_files` /
  `repo_lens:auto_narrow_query_relaxed` provenance. Residual finding: the
  analyzer still selected the ArkTS extractor implementation file as the
  primary `required_files` scope before corpus evidence became principal, so
  the narrowed lens produced many Go function candidates. This is not a
  broad-file runaway, but it is a localization/source-scope authority debt for
  the next source-inventory slice.
- 2026-06-20 Batch U9f follow-up fix delivered: source-inventory closure
  repair directives now normalize file-shaped scopes into legal repo_map call
  shapes before rendering the model-facing repair. A scope such as
  `internal/tool/repomap/index/extract_arkts.go` becomes
  `path="internal/tool/repomap/index"` with `scopes=["extract_arkts.go"]`
  instead of inviting an invalid `repo_map.path=<file>` call. Focused
  validation passed:
  `go test ./internal/tool -run
  'SourceInventoryLensExecutionRepoMapCallShape|SourceInventoryLensExecution|SourceInventory'`.
- 2026-06-20 Earlier RNE-C59 execution-view slice delivered.
  Source-inventory lens construction now creates one scoped execution view per
  pass and shares it across file/config/package candidate builders, graph-symbol
  file membership, broad attribute defer, and completion support indexing.
  The view is language-neutral: it stores sorted repo-relative files, caches
  language subsets, records inventory-file completeness, and exposes exact
  membership checks. This reduces the class of per-role materialize/sort
  regressions without changing read/write scheduler entry points or adding
  prose/keyword routing. Focused validation passed:
  `go test ./internal/tool -run
  'TestSourceInventoryExecutionView_ScopedLanguageAndMembership|TestPublishSourceInventoryObservationFromLens_(BudgetsBroadCandidateMaterialization|BudgetsBroadNoMatchScan|DefersBroadAttributesWithNarrowingHint)'`,
  `go test ./internal/tool -run
  'TestPublishSourceInventoryObservationFromLens_(BudgetsBroadCandidateMaterialization|BudgetsBroadNoMatchScan|PublishesSourceClassUniverseWithoutCandidates|DefersBroadAttributesWithNarrowingHint)|TestSourceInventoryObservationFromLensDirectChildren|TestRepoMapSourceInventory'`,
  `go test ./internal/tool/repomap/... ./internal/types -run
  'TestClassifySourcePathRole|TestLang|TestExtract|TestResolver|TestScanner'`,
  and `go test ./internal/tool -run
  'TestSourceInventoryLensExecutionGap|TestSourceInventoryCandidateUniverseCoverageGap'`.
- 2026-06-20 Source-inventory execution-view eval audit separated harness pass from manual
  correctness. `eval/convergence_audit_summary_20260620_u9g_after.md` showed
  ArkTS failed functionally: the final answer still said there was no actual
  `.ets` source while repo-owned corpus files existed. The first U9h repair
  (`eval/convergence_audit_summary_20260620_u9h_arkts_after.md`) made the
  harness pass, but manual audit found the answer still framed the result as
  "production 0" and treated corpus files as excluded context. This became
  RNE-C60: analyzer source-scope/negative channels must be quote-backed typed
  authority, not model layout inference.
- 2026-06-20 Batch U9j delivered RNE-C60. `source_scope_profile` now carries
  optional `source_quotes[]`; parse-time repair keeps only quotes copied from
  the current request. In source-inventory/enumeration lanes, unquoted
  `requested_scope=production` is not a hard exclusion boundary. Existing
  repo-relative source/config paths in `irrelevant_files` are dropped from the
  negative channel and promoted to `required_files`; when those paths are
  auxiliary/corpus/test/thirdparty classes, the analyzer IR synthesizes
  `source_scope=all` with auxiliary principal scope. Focused tests passed:
  `go test ./internal/tool -run
  'TestReconcilePrincipalScopeIrrelevantFiles|TestEmitAnalysisExecute_RepairsPrincipalSourceScopeIrrelevantFiles|TestEmitAnalysisExecute_SynthesizesAllScopeForSourceInventoryIrrelevantAuxiliaryPaths|TestEmitAnalysisExecute_OverridesUnquotedProductionScopeForSourceInventoryAuxiliaryPaths|TestEmitAnalysisSchemaIncludesSourceScopeProfile|TestEmitAnalysis_Execute_PersistsSourceScopeProfile'`,
  `go test ./internal/types -run
  'TestSourceScopeProfile|TestSourceScopeAllowsPathRole'`, full
  `go test ./...`, `make test`, and `make`.
- 2026-06-20 Batch U9j eval verification:
  `eval/convergence_audit_summary_20260620_u9j_arkts_scope_quote_after.md`
  passed with no eval flags. Manual audit of
  `eval/results/arkts_repomap-20260620-184821` confirms the final answer lists
  4 `@Entry` ArkTS page-entry structs (`Index`, `ParentComponent`,
  `StyledPage`, `ListPage`) and 2 `@Builder` fragments (`defaultHeader`,
  `GlobalCard`) with `.ets` paths and citations, while separately disclosing
  that these are thirdparty tree-sitter corpus examples and production Go code
  contains only comments/string references. Residual low-priority audit noise:
  the log still records one CGEC `CitationReq` repair event even though eval
  contract flags are zero and the visible answer is correct; track under Batch
  M/K2 rather than RNE-C53/C60 correctness.
- 2026-06-20 Batch U9j Cangjie follow-up:
  `eval/convergence_audit_summary_20260620_u9j_cangjie_scope_quote_after.md`
  still reports harness `FAIL` with `missing_section:package`, but manual audit
  of `eval/results/cangjie_repomap-20260620-185259` shows the final answer
  enumerates the repo-owned Cangjie corpus and includes package-path evidence
  in the result table: 1 `extend` block (`extend String` in
  `04_extend_operator.cj`), 1 `foreign func` (`native_add` in
  `07_foreign_ffi.cj`), and 5 `public class` declarations (`Greeter`,
  `Version`, `Animal`, `Dog`, `Service`) with their `demo.*` packages. Treat
  this as an eval/requested-dimension presentation gap, not an RNE-C60 source
  truth-path failure. Track the generic fix under Batch P/X/M/K2: requested
  dimensions should become typed answer obligations and visible status/section
  scaffolds so oracle and human audit can consume the same structured result.
- 2026-06-20 U9k representative refresh:
  `eval/convergence_audit_summary_20260620_goal_batch_after_u9j.md` ran six
  read-mode cases with `PARALLEL=2`: `qf_relation_subagent_registry`,
  `cangjie_repomap`, and `read_combo_trace_current_source_explanation` passed;
  `qf_architecture`, `arkts_repomap`, and
  `read_combo_log_current_source_explanation` failed. Manual audit split the
  failures by system class. `qf_architecture` had enough exploration evidence
  but the final answer collapsed the requested responsibility dimensions,
  confirming Batch X/M answer-surface scaffold and contract-severity work.
  `read_combo_log_current_source_explanation` stayed observation-only despite a
  current-source answer dimension, confirming RNE-C16/N source-lane authority
  still needs another slice. `arkts_repomap` exposed the new P0 RNE-C61
  disconnect: the pre-complete source-inventory repair required `repo_map`, but
  the no-emit restricted tool surface made `repo_map` unavailable.
- 2026-06-20 Batch U9k delivered RNE-C61. Explorer schema filtering and runtime
  tool-boundary validation now compute the same restricted surface with active
  typed `RepairDirective.Tools` overlaid during no-emit escalation. This keeps
  repair execution narrow: `repo_map` can run when the typed repair requires it,
  while unrelated broad tools such as `grep`/`read_file` stay blocked. Focused
  tests passed:
  `go test ./internal/agent -run
  'TestExplorerFilterToolSchemasCompletionOnly|TestExplorerFilterToolSchemasNoEmitEscalationAllowsTypedRepairTools|TestExplorer_RuntimeBoundary_(ReadWithoutEmitRejectsNavigation|AllowsTypedRepairToolDuringNoEmitEscalation|OriginSpecificReadWithoutEmitDoesNotRestrictNavigation)|TestExplorer_FilterToolSchemas_EvidenceRepairCoveredTargetsBecomeEmitOnly|TestExplorer_FilterToolSchemas_ReadWithoutEmitEscalatedMaterializationOnly'`.
  Additional regression passed: `go test ./internal/tool -run
  'TestPublishSourceInventoryObservationFromLens|TestSourceInventoryObservationFromLensDirectChildren|TestRepoMapSourceInventory|TestSourceInventoryLensExecution|TestSourceInventoryCandidateUniverseCoverageGap'`
  and full `go test ./...`.
  The focused ArkTS rerun
  `eval/convergence_audit_summary_20260620_goal_batch_arkts_after_repair_surface.md`
  passed. Manual audit of `eval/results/arkts_repomap-20260620-191333`
  confirms the model executed `repo_map(view=source_inventory)` after the repair
  directive and the final answer lists 4 `@Entry` entries (`Index`,
  `ParentComponent`, `StyledPage`, `ListPage`) plus 2 `@Builder` fragments
  (`defaultHeader`, `GlobalCard`) with explicit `.ets` paths. Residual
  smoothness debt remains: the run still used 23 explorer iterations and context
  pruning, so RNE-C48/RNE-C59 execution-kernel work and RNE-C58 member/facet
  projection remain active rather than closed by this patch.
- 2026-06-20 Batch U9l delivered a mixed runtime/current-source lane repair.
  The first focused control rerun
  `eval/convergence_audit_summary_20260620_goal_batch_log_source_after_exclusion_kind.md`
  still failed with `read=0` / `repo_map=0`: the analyzer no longer emitted an
  explicit current-source exclusion, but an external runtime diagnostic request
  that asked for mechanism explanation still collapsed to observation-only after
  `current_version_check` was cleared for lack of exact runtime frame/path
  anchors. The repair is typed, not prose-routed. `external_observation_policy`
  now requires `exclusion_kind=explicit_user_exclusion` before
  `current_source_mode=exclude` can close the source lane, and
  `RequestModel.HasRuntimeArtifactCurrentVerificationAnchor` now treats an
  external runtime artifact plus diagnostic/cross-component root-cause or
  explanation shape as a current-source mechanism bridge unless an explicit
  typed exclusion is present. Focused tests, full `go test ./...`, and the
  post-change eval
  `eval/convergence_audit_summary_20260620_goal_batch_log_source_after_diagnostic_bridge.md`
  passed. Manual audit of
  `eval/results/read_combo_log_current_source_explanation-20260620-193552`
  confirms the final answer cites current source
  (`internal/llm/stream_errors.go`, `internal/llm/openai.go`,
  `internal/render/status_messages.go`, and
  `internal/agent/answer_document_evaluator.go`) while preserving the runtime
  log boundary. Residual smoothness debt remains: the passing run still used
  10 explorer iterations and 39% of the context window, so the broader
  RNE-C48/RNE-C59 execution-kernel and context-noise work remains active.
- 2026-06-20 Batch U9m source-inventory audit disposition: the latest
  source-inventory hot-spot audit was reconciled against current `main`.
  RNE-C53's correctness core is now implemented by the existing
  `SourcePathRole` / `SourceScope` authority, `SourceClasses` on
  `SourceInventoryObservation`, and both pre-emit/post-emit exact-absence
  gates; production-only negative citations no longer close source-inventory
  absence while repo-owned auxiliary/thirdparty classes are present. The
  remaining P0 was the over-broad execution path. `repo_map` already rejects
  `source_inventory` with sole `roles=["file"]` before graph/index work, but
  the broad-navigation fast path could still bypass the source-class matrix.
  This slice exported a single typed helper for attaching the repo-owned
  source-class universe and made the broad-navigation guard call it before
  persisting handoff state. Focused tests passed:
  `go test ./internal/tool/repomap -run
  'TestRepoMapSourceInventory(RejectsSoleFileRoleBeforeIndexWork|RejectsBroadRootFileRoleWithAttributesBeforeIndexWork|BroadNavigationLensIsBoundedBeforeReconcile|BroadRootBudgetGuard)'`,
  `go test ./internal/tool -run
  'TestPublishSourceInventoryObservationFromLens_PublishesSourceClassUniverseWithoutCandidates|TestPreCheckAbsenceScopeBound_SourceInventoryClassUniverseRequiresInventoryProof|TestSourceInventoryLensExecution|TestSourceInventoryCandidateUniverseCoverageGap'`,
  and
  `go test ./internal/orchestrator -run
  'TestValidateSourceInventoryExactAbsenceBound_RequiresClosedClassUniverse'`.
  Remaining work is not another taxonomy patch: RNE-C47/RNE-C48 still need a
  unified source-inventory execution budget with wall-clock cancellation,
  resumable pagination/cursors, and one scan/materialization kernel replacing
  residual per-role helper loops.
- 2026-06-20 Batch U9n delivered the first execution-budget kernel slice for
  RNE-C47/RNE-C48. The existing per-role candidate budget now carries an
  internal deadline for broad advisory source-inventory lenses, and the graph
  symbol candidate path now has a separate query-prefilter scan cap: query
  misses are bounded by scoped symbol count and deadline, while query matches
  that appear after broad noise still get a wider deterministic budget. This
  closes the "narrow query over a huge symbol set scans forever because nothing
  matches" class without regressing ArkTS-style "target symbols appear after a
  noisy production graph" discovery. No user-facing tool schema, read
  scheduler, or prompt routing changed. Focused validation passed:
  `go test ./internal/tool -run
  'TestSourceInventoryCandidateBudget(DeadlineTruncatesFileScan|CountsScopedQueryMisses)|TestPublishSourceInventoryObservationFromLens_(BudgetsBroadCandidateMaterialization|BudgetsBroadNoMatchScan|DefersBroadAttributesWithNarrowingHint)'`.
  Remaining work: expose resumable cursor state as a typed execution artifact
  rather than a render-only offset hint, and continue collapsing residual
  helper-specific loops into one execution view/budget carrier.
- 2026-06-20 Batch U9o representative refresh:
  `eval/convergence_audit_summary_20260620_goal_batch_after_u9n.md` ran six
  read-mode cases after the first execution-budget slice. Harness verdict was
  5/6 PASS. Manual audit split the remaining noise into three classes rather
  than another per-case patch. First, `arkts_repomap` still exposed a visible
  answer-surface gap: accepted `surface_terms` such as `@Component` existed in
  typed evidence but the final answer could omit those source-visible labels.
  Second, mixed runtime/current-source cases still used long exploration
  loops, so RNE-C48/RNE-C59 remain active smoothness debts even when correctness
  is recovered. Third, trace/log source-lane cases prove the current bridge is
  better but still not cheap; future work should reduce repeated context
  pruning and redundant exploration through shared coverage snapshots.
- 2026-06-20 Batch U9p delivered RNE-C62. `emit_answer_document` now materializes
  a small system-verified supplement when all of the following typed conditions
  hold: a principal support member is already visible, its citation is present,
  accepted `explorer.emit_evidence` rows carry missing `surface_terms`, and the
  terms are required for that evidence/member pair. The normalizer reads the
  `AnswerSupportPlan`, citations, and typed evidence only; it does not inspect
  user prose or model rationale for routing. Focused validation passed:
  `go test ./internal/tool -run
  'TestNormalizePrincipalSupportSurfaceTermSupplement|TestNormalizeAggregateMemberSetCarriers|TestPreCheckModelSurfaceTerms|TestRunPreEmitChecks'`,
  `go test ./internal/types -run
  'TestMissingPrincipalSupportMembers|TestPrincipalSupportMemberObligations'`,
  `make build`, and full `go test ./...`. The focused ArkTS rerun
  `eval/convergence_audit_summary_20260620_goal_batch_arkts_after_surface_term_supplement.md`
  passed and logged the supplement path:
  `materialized 1 principal support surface-term row(s) from accepted evidence
  handoff`.
- 2026-06-20 RNE-C65 opened after U9p manual audit. The ArkTS focused harness
  passed, but the visible answer listed only `Index` while earlier correct runs
  listed four `@Entry` owners and two `@Builder` fragments. The analyzer
  pre-scan had already observed the stronger candidate universe through typed
  tool outputs, but the model omitted `required_files` in `emit_analysis`, so
  downstream forced-read and pre-complete coverage obligations never inherited
  those candidates. The explorer then accepted a partial enumeration as a
  complete member set. This is a read-mode coverage-authority gap, not an ArkTS
  special case: deterministic prescan candidates for source-inventory or
  exhaustive category-enumeration lanes must be projected into bounded required
  read obligations and closure checks must treat unread prescan candidates as
  open coverage. The fix must consume structured tool outputs, repo-relative
  path validation, typed request traits, and source-scope policy; it must not
  parse user wording or model prose.
- 2026-06-20 Batch U9q delivered the first RNE-C65 coverage-authority slice.
  `emit_analysis` now projects deterministic analyzer prescan file candidates
  into high-confidence `RequiredFileHints` for typed source-inventory /
  exhaustive source-enumeration lanes when the model omits `required_files`.
  The projection reads only successful tool results (`grep(files_only=true)` and
  bounded `list_files` shapes), repo-relative path resolution, file existence,
  `SourceScopeProfile`, `SourcePathRole`, and code/config suffix policy. It
  does not inspect raw user prose or model rationale. Required-file coverage
  now has a request-aware cap: ordinary current-source lanes keep the existing
  cap of 4, while source-inventory lanes use a bounded cap of 6. The
  pre-dispatch forced-read seeder, required-file completion gate, and Phase1
  unread filter all consume the same typed policy. Phase1 still suppresses
  broad graph-adjacency noise after a read focus, but no longer drops
  `RequiredFiles` / high-confidence `RequiredFileHints` as "non-mandatory"
  signals. Focused validation passed:
  `go test ./internal/types -run
  'TestRequiredFileHintCoverage|TestAnalyzerHints_RequiredFileHints|TestRequiredFileHint_JSONRoundtrip|TestRequiredFileHint_OmitemptyRationale'`,
  `go test ./internal/tool -run
  'TestEmitAnalysis_ProjectsPrescanFilesForSourceInventoryCoverage|TestEmitAnalysis_Execute_PersistsSourceInventoryProfile'`,
  and `go test ./internal/orchestrator -run
  'TestSeedRequiredFileHintForcedReadsBeforeExplore_SourceInventoryUsesInventoryCap|TestRunForcedReads_PreDispatchRequiredFilesUseSharedCoverageCap'`.
  Focused ArkTS eval
  `eval/convergence_audit_summary_20260620_goal_batch_arkts_after_required_prescan_projection.md`
  passed. Manual audit of
  `eval/results/arkts_repomap-20260620-202239` confirms the final answer lists
  4 `@Entry` page-entry structs (`Index`, `ParentComponent`, `StyledPage`,
  `ListPage`) and 2 `@Builder` fragments (`defaultHeader`, `GlobalCard`) with
  `.ets` file paths and line citations. This live run did not exercise the new
  projection fallback because the model itself emitted `required_files`; the
  unit test covers the missing-`required_files` branch. The eval still validates
  the shared downstream coverage path: `required_file_hints` kept 6 high
  confidence files and the final report no longer collapsed to a single partial
  member. Remaining RNE-C65 follow-up: extend the projection with a typed
  observation carrier if future logs show useful prescan candidates hidden
  behind blob-only raw refs.
- 2026-06-20 Batch U9r delivered a second RNE-C47/RNE-C48 execution-kernel
  slice. `SourceInventoryObservation` now carries typed `page` and `execution`
  metadata: offset, limit, total, emitted rows, `next_cursor`, page completeness,
  budgeted execution, candidate-budget truncation, and broad-attribute deferral.
  The metadata is attached at lens construction time and preserved by
  clone/merge/MutableState handoff, so future gates/status cards/ReasoningGraph
  projections do not need to parse rendered markdown for `next_cursor` or
  budget truncation. Renderer pagination still uses its call-time query, so
  existing `top_n`/`cursor` model-facing behavior is preserved. Focused
  validation passed:
  `go test ./internal/types -run
  'TestSourceInventoryObservation_CloneAndMergePreservesCountInvariant|TestSourceInventoryObservation_ClassUniverseCanBeActiveWithoutMemberRows'`,
  `go test ./internal/tool -run
  'TestPublishSourceInventoryObservationFromLens_BudgetsBroadCandidateMaterialization|TestSourceInventoryCandidateBudgetDeadlineTruncatesFileScan|TestSourceInventoryCandidateBudgetCountsScopedQueryMisses'`,
  and `go test ./internal/tool/repomap -run
  'TestRepoMapSourceInventoryBroadNavigationLensIsBoundedBeforeReconcile|TestRepoMapSourceInventoryBroadRootBudgetGuard'`.
  Remaining RNE-C47/RNE-C48 follow-up: move candidate construction into a
  dedicated execution-kernel package with explicit cancellation checkpoints
  shared by file/config/package/graph candidates, then have scheduler/status
  consume typed page/execution state instead of text advisories.
- 2026-06-20 Batch U9s closed a residual hard-gate hygiene gap in
  source-inventory lens execution proof. Current HEAD already has the RNE-C53
  source-class universe authority: `SourceInventoryObservation.SourceClasses`
  is derived from git-tracked repository files with `SourcePathRole`, the
  exact-absence pre-emit and post-contract gates consume that matrix, and
  third-party/corpus/vendor/generated/test/documentation classes cannot be
  erased by the normal search graph's production-source filters. The remaining
  issue was that `SourceInventoryLensExecutionGapForContext` still accepted a
  `repo_map` tool result by matching rendered Summary text such as
  "Repo Lens: Source Inventory". That made a hard pre-complete gate depend on
  Markdown prose. U9s now requires the existing typed `ObservationRecord`
  emitted by `repo_map` (`Producer=repo_map`,
  `Predicate=repo_map_navigation_route`, route=`source_inventory`) and ignores
  Summary text for this decision. A zero-result source-inventory lens still
  satisfies the execution proof when it carries the typed navigation
  observation, while a Summary-only synthetic result remains blocking. Focused
  validation passed:
  `go test ./internal/tool -run
  'TestSourceInventoryLensExecutionGap|TestSourceInventoryCandidateUniverseCoverageGap'`.
- 2026-06-20 Batch U9t delivered the first source-inventory execution-kernel
  extraction slice. The old private per-role `sourceInventoryCandidateBudget`
  helpers are gone; candidate construction now receives one
  `sourceInventoryExecBudget` object that owns materialization caps, scan caps,
  query-scan widening, wall-clock deadline, cooperative cancellation via
  `BusContext.Context()`, and cursor/page metadata. File/config/package/graph
  candidate builders and completion support call the budget object's methods
  instead of reading open-coded limit fields or standalone helper functions.
  This does not change the model-facing `repo_map` schema or rendered answer
  text; it moves execution control into one typed kernel seam. Focused
  validation passed:
  `go test ./internal/tool -run
  'TestSourceInventory(ExecBudget|CandidateBudget)|TestPublishSourceInventoryObservationFromLens_(BudgetsBroadCandidateMaterialization|BudgetsBroadNoMatchScan|PublishesSourceClassUniverseWithoutCandidates|DefersBroadAttributesWithNarrowingHint)|TestSourceInventoryLensExecutionGap'`
  and
  `go test ./internal/tool/repomap -run
  'TestRepoMapSourceInventoryBroadNavigationLensIsBoundedBeforeReconcile|TestRepoMapSourceInventoryBroadRootBudgetGuard|TestRepoMapSupportedViewsMatchSchemaEnum'`.
  The post-U9s representative eval
  `eval/convergence_audit_summary_20260620_after_typed_source_inventory_lens.md`
  passed all six cases. Five cases still emitted advisory flags
  (`contract_warning`, `auto_repair`, or `context_prune`), so U9g shared
  proof coverage and U9h typed repair/handoff carrier remain the next
  commercial smoothness work. Remaining RNE-C59 work: move the private
  execution budget/view into a dedicated source-inventory kernel package and
  add true cursor-backed resumable candidate pagination before full
  materialization.
- 2026-06-20 Batch U9u moved source-inventory budget/page/cursor authority into
  a dedicated kernel package. `internal/tool/sourceinventory.Budget` now owns
  materialization caps, scan caps, query-scan widening, wall-clock deadline,
  cooperative cancellation, cursor offset, page limit, and typed page state.
  The legacy `internal/tool` budget type is now a thin adapter whose only
  package-local responsibility is appending private candidate structs. Focused
  validation passed:
  `go test ./internal/tool/sourceinventory`,
  `go test ./internal/tool -run
  'TestSourceInventory(ExecBudget|CandidateBudget)|TestPublishSourceInventoryObservationFromLens_(BudgetsBroadCandidateMaterialization|BudgetsBroadNoMatchScan|PublishesSourceClassUniverseWithoutCandidates|DefersBroadAttributesWithNarrowingHint)|TestSourceInventoryLensExecutionGap|TestSourceInventoryCandidateUniverseCoverageGap'`.
  Remaining RNE-C59: move `sourceInventoryExecutionView` scoped file
  materialization into the kernel and implement true cursor-backed resumable
  candidate pagination before full member/attribute materialization.
- 2026-06-20 Batch U9g delivered the first shared proof-coverage slice. Added
  `loopkernel.ProofSnapshot` and `ProofSnapshotFromReadTurnA`, deriving read
  proof authority from typed TurnA fields only: accepted result kind, grounded
  evidence refs, source localization, source-inventory observation, and
  runtime-observation-only completion. `DeriveProofCoverageAuthority` and a
  shared `TruthLedgerFromProofCoverageAuthority` now serve both read and write
  projections. ReasoningGraph read projection records the proof snapshot as an
  authority event, giving status-card/eval consumers the same proof/truth view
  without parsing logs, prompts, visible thinking, or final-answer prose.
  Focused validation passed:
  `go test ./internal/loopkernel ./internal/reasoninggraph`. Remaining RNE-C54:
  have `dispatchExploreWindowsParallel` consume per-dispatch snapshots by lane
  ownership key so equivalent sibling proof work is skipped while new blockers,
  source classes, failed proof, or missing principal obligations still run.
- 2026-06-20 Batch U9g follow-up completed scheduler consumption for shared
  proof coverage. `dispatchExploreWindowsParallel` now accepts a lane handoff
  when the fork's typed read `ProofSnapshot` is covered and backed by accepted
  result kind or runtime-observation-only completion, even if the mutable
  investigation-complete flag was not set. It still rejects weak/missing proof,
  so low-confidence localization cannot prematurely cancel siblings. Focused
  validation passed:
  `go test ./internal/orchestrator -run
  'TestDispatchExploreWindowsParallel_CollectiveLane(Convergence|ConsumesReadProofSnapshot)|TestExploreParallelResultSatisfiesLaneHandoffRejectsWeakProof'`.
- 2026-06-20 Batch U9h delivered the first typed repair/handoff carrier slice.
  Added `ToolHandoffCarrier` as the lane-neutral structured carrier for
  `ToolRepair`, `PlanRepairPack`, supported JSON retry surfaces, typed
  observation refs, and accepted evidence refs. `BaseAgent`, forced-read
  injection, `MutableState.AppendDispatchToolResult`, `SetTurnAArtifacts`,
  explore-fork merge, and Turn-A handoff bounds now attach/preserve the carrier
  without parsing tool summaries, model rationale, user wording, or visible
  thinking. Focused validation passed:
  `go test ./internal/types`,
  `go test ./internal/agent ./internal/orchestrator`. Remaining RNE-C55:
  per-tool schema descriptor registry, extractor/finalizer/status-card Top-N
  carrier rendering, and ReasoningGraph carrier projection.
- 2026-06-20 Batch U9h follow-up added typed carrier consumers for extractor
  and finalizer prompts. The renderer projects only typed fields:
  tool/reason/repair code, failing JSON field paths, accepted enum field keys,
  accepted evidence IDs, source/line/owner/anchor, and typed observation refs.
  It deliberately omits `ToolRepair.Hint`, tool summaries, model rationale, and
  visible thinking. Focused validation passed:
  `go test ./internal/agent -run
  'TestRenderTypedToolHandoffCarriers|TestRenderAnswerDocToolHandoffCarriers|TestAnswerDocumentEvaluator_BuildInitialInstruction|TestRenderExtractorSourceInventory'`.
  Remaining RNE-C55: status-card/ReasoningGraph carrier projection and a
  per-tool schema descriptor registry for all emit tools.
- 2026-06-20 Batch U9h graph follow-up added ReasoningGraph carrier
  projection. `BaseAgent` now emits `tool_handoff_projected` from
  `ToolResult.Handoff`, carrying typed JSON field paths, enum field keys,
  accepted evidence IDs, and observation IDs. It does not parse tool summaries
  or repair prose. Focused validation passed:
  `go test ./internal/agent -run TestObserveToolHandoffCarrierProjected`,
  `go test ./internal/reasoninggraph`. Remaining RNE-C55:
  per-tool schema descriptor registry and status-card carrier projection.
- 2026-06-20 Batch U9t follow-up moved scoped source-inventory file
  materialization into `internal/tool/sourceinventory.ExecutionView`. The
  kernel now owns sorted scoped files, language-index cache, file membership,
  budget truncation, and complete/incomplete state. `sourceInventoryCandidate`
  builders and investigation-complete support indexing consume the same budget
  adapter; the zero-value budget bypass ratchet is now 0. Focused validation
  passed:
  `go test ./internal/tool/sourceinventory ./internal/tool -run
  'TestExecutionView|TestSourceInventoryConvergence|TestSourceInventoryBroadRootFileRoleStaysBoundedBeforeFullReconcile|TestSourceInventoryCandidateBudget'`.
  Remaining RNE-C59: cursor-backed candidate pagination before attribute/member
  materialization, rather than only page metadata after candidate construction.
- 2026-06-20 Batch U9h schema-registry follow-up added a typed emit-tool JSON
  surface registry compiled from each emit tool's `Parameters()` schema. Repair
  exits attach `ToolJSONSurfaceDescriptor` through `ToolRepair.Metadata`;
  `ToolHandoffCarrier`, extractor/finalizer prompt renderers, and
  ReasoningGraph consume accepted/failing JSON field paths and enum surfaces
  from that typed metadata. The implementation does not parse tool summaries,
  repair hints, model rationale, or user wording. Focused validation passed:
  `go test ./internal/types ./internal/tool ./internal/agent ./internal/reasoninggraph -run
  'TestToolHandoff|TestToolJSONSurface|TestStrictDecodeFailureAttachesToolJSONSurfaceMetadata|TestRenderTypedToolHandoff|TestObserveToolHandoffCarrierProjected'`.
  Remaining RNE-C55: project the same bounded carrier view into REPL/status
  cards.
