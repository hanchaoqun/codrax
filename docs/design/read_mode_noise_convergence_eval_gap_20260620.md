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
| Batch C2 | planned | Add relation retry delta handling. | Duplicate support rows stay advisory, and repeated identical relation support downgrades stop broadening after one bounded repair turn. |
| Batch D | planned | Add typed relevance budget for chain/concrete/support evidence. | Architecture inventory and C++ call-chain cases render bounded Top-N prompt sections while preserving full refs in audit artifacts. |
| Batch E | planned | Add downgrade fingerprint / low-delta guard. | Identical pre-complete rejection without new typed input stops after one bounded repair turn. |
| Batch F | planned | Add trace causal path projection. | Trace eval preserves intermediate path-role facets in final answer with fewer trace_query calls. |
| Batch G | planned | Add read eval watchdog. | Read-mode eval paths use a configurable timeout and emit typed timeout summaries. |
| Batch H | delivered | Quarantine post-complete localization repair for extractor. | Extractor receives non-executable localization status/caveat fields and does not attempt `read_file` / `repo_map` after accepted closure. |
| Batch I | delivered | Gate final system supplements by principal-answer relevance. | A passing scalar/member-set answer does not show stale "localization needed" or generic low-proof caveats unless typed proof debt is principal/blocking. |
| Batch K | planned | Add tool/preflight/context assembly telemetry and caches. | `emit_evidence`, `emit_investigation_complete`, schema normalization, grounding view, and completion preflight expose sub-stage timings and reuse dispatch/version-scoped typed views. |
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
- Eval: `arkts_repomap` no longer loops on empty set shape.
- Eval: `qf_relation_subagent_registry` converges without repeated identical
  relation support downgrade.
- Eval: `trace_query_wakeup_causal_io_chain` preserves intermediate path roles.
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
