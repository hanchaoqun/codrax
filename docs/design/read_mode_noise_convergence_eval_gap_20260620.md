# Codrax Read Mode Noise And Convergence Eval Gap Plan

## Scope

This document records the system gaps exposed by the 2026-06-20 read-mode
representative eval batch. Noise reduction is the highest-priority track, but
the repair scope is broader: schema ergonomics, typed handoff, relation
support, trace causal projection, eval watchdogs, and final-answer evidence
preservation all need phased work.

The target is a generalized commercial design, not case-by-case prompt patches.
Hard gates must consume typed artifacts only: request-model fields, aggregate
facts, relation axes, support refs, read ranges, trace rows, and structured
tool results. User prose, model rationale, visible thinking, and rendered
summaries remain soft context only.

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
| RNE-C12 | P0 | Tool/preflight/context assembly latency can dominate successful read runs. | Large evidence ledgers and repeated completion prechecks can make `emit_evidence`, `emit_investigation_complete`, and "organizing context" slow even when the next decision is already known. Static tool schemas, grounding views, completion preflight state, and evidence scans are rebuilt in multiple places. | Add timing telemetry and shared typed preflight/cache layers: static tool parameters cached, schema normalization marked once, grounding context cached by dispatch/version, and completion gates consume one `CompletionPreflightView` instead of rescanning evidence repeatedly. |
| RNE-C13 | P0 | Accepted-closure advisory debt is not consumed consistently across scheduler surfaces. | `chain_promotion.*` pending reads can be advisory for auto-complete but still appear in retry hints or forced-read drains if the cleanup happens after hint rendering. This creates the visible pattern "investigation complete → verification not stable → same support-chain read again". | A single typed `RepairDebtClass` policy must feed auto-complete, fact-retry suppression, retry-hint rendering, forced-read drains, and audit checkpoints. Accepted closure may keep principal blockers, but advisory debt is cleared before any model-facing retry hint. |
| RNE-C14 | P1 | User-facing retry notices can remain stale after auto-complete. | The renderer may already have emitted "verification not stable enough" before the scheduler recognizes that accepted closure supersedes a retry carry-over. The system state is correct, but the REPL progress card looks like a real retry. | Status-card events should be driven by typed next-action decisions after stale retry carry-over suppression, so auto-completed advisory retries render as "accepted; skipping support-only retry" instead of another unstable-verification cue. |
| RNE-C15 | P1 | Final contract telemetry still has schema-level noise after eval pass. | A run can report eval `answer_contract_violations=0` while the internal CGEC summary records non-blocking answer-document field violations such as candidate role annotation drift. | Final contract telemetry should distinguish blocking user-answer defects, repaired/non-blocking schema drift, and audit-only annotation gaps with typed severity, then feed the same status card and reasoning graph. |
| RNE-C16 | P0 | Mixed runtime-artifact plus current-code requests can collapse to observation-only. | Analyzer can emit typed `requested_answer_dimensions.role=current_key_code` and `source_scope_profile`, while `CurrentSourceLaneDecision` only treats `current_source_mode=allow` or explicit current-source profile as hard current-source anchors. Explorer then receives `Runtime Artifact Only Start`, accepts `external_only_log`, and finalizer discovers the missing current-code dimension too late. | Source-lane authority consumes typed required `current_key_code` dimensions paired with valid source scope, not prose. Observation-only remains valid for pure runtime metric dimensions; explicit `exclude` still wins; `external_only_*` waiver cannot close a required current-source lane. |
| RNE-C17 | P0 | Missing repo_map lens debt can requeue after grounded current-source evidence exists. | After source anchors and evidence are accepted, pre-finalize localization can still treat a missing `file_map` lens as principal and repeatedly show "verification not stable enough". This is support/navigation debt, not necessarily answer-blocking proof debt. | Navigation debt must be classified by consumer and principal coverage. If current-source evidence/read coverage satisfies the active lane, missing lens debt is advisory or at most one bounded repair; it must not keep requeueing identical source-localizer work. |
| RNE-C18 | P1 | Requested-dimension role drift causes avoidable finalizer patch rounds. | The visible answer may already cover the user's dimension label, but coverage hints can report a role-shaped miss such as `日志线索 (diff_clue)`. This happens when origin/dimension role projection overfits a role enum rather than the typed source quote/label coverage. | Dimension coverage should consume normalized dimension ids, labels, source quotes, and block evidence origins together. Role aliases are soft display metadata; missing-dimension hints must not depend on a misleading role name alone. |
| RNE-C19 | P1 | Runtime-artifact language guesses can pollute repository reasoning. | Log triage may emit `meta.lang=python` for a generic runtime log even when the repository is Go. That artifact-language guess can become noise for downstream source localization and audit language summaries. | Separate artifact format/language from repository/source language. Runtime triage language is advisory artifact metadata only; source-language decisions come from repo_map/file paths/typed source inventory. |
| RNE-C20 | P1 | Parallel exploration siblings can duplicate a solved current-source lane. | The mixed runtime/source eval passed only after 24 reads, 4 repo_map calls, 56 explorer iterations, 18 midloop injections, and context pruning. Several sibling routes collected overlapping finalizer/LLM timeout evidence. | Add a shared proof-coverage ledger across siblings. Once one route satisfies runtime observation + current-source owner + mechanism coverage, sibling routes consume the ledger, skip duplicate reads, and converge through a compact handoff. |
| RNE-C21 | P0 | Runtime-only trace answers can be blocked or polluted by source navigation debt. | Trace-only tasks can produce enough `trace_query` observation facts, then pre-finalize Tier-1 still demands `repo_map` `task_map/file_map` and final rendering appends repo navigation/localizer supplements. The request's typed route is artifact-local, but the source-localizer authority is not origin-aware enough. | Add an origin-aware proof profile. When a task is runtime-artifact-only and has no typed current-source obligation, repo navigation/localizer debt is not a blocking pre-finalize floor and is not rendered as a user-facing source supplement. Trace observation audit remains visible through trace-specific typed observations. |
| RNE-C22 | P0 | Trace root-cause rank is not promoted to the principal answer carrier. | `trace_query` emits `root_evidence` / `root_cause_rank` rows for `threadpool-400`, but final answer can lead with the target thread's direct wait state and fail evals or user reading that expect the primary root-cause node to be first-class. | Add a typed `TraceCausalProjection` with roles such as `target_direct_wait`, `primary_root_cause`, `causal_chain_hop`, `direct_waker`, and `drilldown_boundary`. Finalizer consumes this projection as a principal obligation instead of rediscovering rank from raw trace rows or prose. |
| RNE-C23 | P0 | Source inventory can falsely prove absence when supported-language fixture/corpus files are outside the default source scope. | The ArkTS eval repository contains `.ets` corpus sources under `internal/thirdparty/tree-sitter-arkts/corpus/sources`, but the run answered that no ArkTS source files exist. The scanner/navigation view and grep/list outputs did not expose a typed source-scope boundary that differentiates product source, fixtures, corpora, testdata, vendored code, and generated assets. | Introduce a language-neutral `SourceInventoryAuthority` with typed scope classes. Absence answers must state the exact searched source classes and cannot close if matching files exist in an in-scope class required by the task/eval. This must cover all supported languages, including C/C++, Cangjie, ArkTS, JS/TS, Java/Kotlin, Ruby, Go, config, workflow, and other repomap languages. |
| RNE-C24 | P1 | Empty/negative aggregate schema repair still burns many model rounds. | After exact empty member-set support landed, models still repeatedly repair `negative_search` / `negative_observation` dimensions and sometimes mix repo no-hit evidence with infrastructure evidence. | Add a unified aggregate-fact repair/normalization layer that returns precise typed repair hints and canonicalizes no-hit repo searches, artifact no-hit observations, and exact empty sets before retry. The layer must not infer from model prose; it only consumes schema-validated fields and tool result metadata. |

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
| Batch F | planned | Add trace causal path projection. | Trace eval preserves intermediate path-role facets in final answer with fewer trace_query calls. |
| Batch G | planned | Add read eval watchdog. | Read-mode eval paths use a configurable timeout and emit typed timeout summaries. |
| Batch H | delivered | Quarantine post-complete localization repair for extractor. | Extractor receives non-executable localization status/caveat fields and does not attempt `read_file` / `repo_map` after accepted closure. |
| Batch I | delivered | Gate final system supplements by principal-answer relevance. | A passing scalar/member-set answer does not show stale "localization needed" or generic low-proof caveats unless typed proof debt is principal/blocking. |
| Batch K | planned | Add tool/preflight/context assembly telemetry and caches. | `emit_evidence`, `emit_investigation_complete`, schema normalization, grounding view, and completion preflight expose sub-stage timings and reuse dispatch/version-scoped typed views. |
| Batch L | planned | Align user-facing retry/status notices with typed next-action state. | Accepted closure that skips support-only retry debt shows a clear auto-complete/skip notice, not a stale "verification not stable enough" retry cue. |
| Batch M | planned | Split final contract telemetry by typed severity. | Eval pass/fail, user-answer blocking defects, repaired schema drift, and audit-only annotation gaps are reported separately and consumable by URGR/reasoning graph. |
| Batch N | delivered | Fix mixed runtime/source lane authority. | `current_key_code` requested dimensions plus typed source scope require source coverage before closure; pure runtime metric dimensions stay source-optional; the representative mixed log/current-code eval passes with real source reads and citations. |
| Batch O | delivered | Demote stale repo_map navigation debt after principal source coverage. | Missing lens debt no longer requeues after current-source proof is satisfied unless it is tied to an unresolved principal owner/path obligation. |
| Batch P | planned | Normalize requested-dimension coverage by typed ids/labels/origins. | Finalizer does not patch solely because a role enum label drifted; it patches only when the typed requested dimension is absent from visible answer content. |
| Batch Q | planned | Split runtime-artifact language metadata from repo/source language. | Log/trace artifact language guesses cannot drive source-language summaries, source localization, or hard gates. |
| Batch R | planned | Share proof coverage across exploration siblings. | Mixed runtime/source cases converge with bounded duplicate reads and smaller context while preserving source citations. |
| Batch S | delivered | Add origin-aware runtime-only source navigation suppression. | Trace/log artifact-only tasks with no current-source obligation skip repo_map/localizer blocking floors and user-facing source supplements while keeping runtime observation audit. |
| Batch T | planned | Add trace causal projection and principal root-cause obligation. | Trace final answers surface the typed primary root cause, direct wait, causal chain, and drilldown boundaries from one compact projection. |
| Batch U | planned | Add source inventory authority with scope classes. | Supported-language searches expose product/test/fixture/corpus/vendor/generated scope classes; absence closes only against the classes required by the typed task. |
| Batch V | planned | Add aggregate negative-fact canonicalization and repair hints. | Empty-set and no-hit searches converge through one canonical schema path without repeated dimension repair loops. |
| Batch J | planned | Re-run the representative batch and refresh this ledger. | At least the original 6 cases are re-run; remaining failures identify new architecture gaps rather than repeated schema/noise loops. |

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
- Unit: source inventory reports scope classes for product, test, fixture,
  corpus, vendor, generated, config, and workflow files across supported
  languages.
- Unit: absence answers cannot close if the typed source inventory finds
  matching files inside a required source-scope class.
- Unit: negative search / negative observation / exact empty member-set facts
  are canonicalized without requiring model prose repair loops.
- Eval: `arkts_repomap` no longer loops on empty set shape.
- Eval: `qf_relation_subagent_registry` converges without repeated identical
  relation support downgrade.
- Eval: `trace_query_wakeup_causal_io_chain` preserves intermediate path roles.
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
