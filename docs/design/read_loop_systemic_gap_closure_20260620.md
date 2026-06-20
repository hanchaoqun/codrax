# Read-Loop Systemic Gap Closure (2026-06-20)

Closes five systemic read-mode architecture gaps surfaced by the 2026-06-20
eval batches. Each fix is **system-level and generalized** (closes a whole
class, never a single case), **typed-only** (no user-prose / model-prose /
keyword matching in any hard gate or router), **reuse-first** (no reinvention
of existing primitives), and **noise-reducing** (the top user pain). Designs
below are post-adversarial-review; the red-team corrections are folded in and
called out as **[RT]**.

## Design contract (applies to every task)

- Hard gates / routers read **typed artifacts only**: request-model fields,
  relation/scope enums, evidence roles, aggregate facts, structured tool
  results, FNV hashes over sorted typed id sets. User prose, model rationale,
  visible thinking, rendered summaries are **soft context only**.
- **No per-case patching.** One typed mechanism per gap closes the class; a new
  instance just emits its typed tag.
- **Reuse, don't reinvent.** Named primitives below are mandatory reuse points.
- **Handoff fidelity.** Earlier-stage key info/evidence flows to later
  consumers by priority; nothing rich is silently dropped.
- **One decode chokepoint.** Model tool-call JSON keeps flowing through the
  existing `NormalizeEmitAnswerBlock` / `citationRefFromWire` contract; new
  typed fields register there, no parallel decoders.
- **Don't destabilize working scenarios.** L1 read-scheduler byte-identity
  holds; currently-passing evals must not regress (eval-gate before risky
  landings).

## Reusable primitives (do-not-reinvent map)

| Concern | Reuse |
| --- | --- |
| Rolling-history equality / no-progress | `types.ClosureFingerprint` + `AppendFingerprint`/`lastNFingerprintsEqual` (evidence_closure.go / cgec_enforcers.go); `dataworkflow` `ProgressSignature` + `RecentRelationNoProgressCount` (progress.go) |
| Compact typed evidence projection | `types.ObservationLedger` + `ProjectObservationPromptRecords` + `PrioritizeObservationRecords` + `CompileObservationLedger` (observation_ledger*.go) |
| Producer-precedence chokepoint | `ObservationLedger.HasDeterministicRuntimeQueryObservation()` + `runtimeObservationProducerIsDeterministicQuery` |
| Scope-class typed filter | `ClassifySourcePathRole` + `SourceScopeAllowsPathRole` (source_path.go / source_scope_profile.go); `sourceInventory*Candidates` (source_inventory_reconcile.go:3067) |
| Repo-wide enumeration (no re-scan) | `sourceInventoryScopedGraphFiles` + `sourceInventoryRequestedScopes` over the cached `SearchGraph()` |
| AST structural-test harness | `write_mode_red_lines_test.go`, `jargon_structural_lint_test.go` |
| Accept-with-caveat / force-complete terminals | `evidence_floor_waiver` lane, `RepairForceCompleteDowngrade`, `SetInvestigationComplete` |

---

## Gap ① — Read-loop low-delta convergence boundary (P0, #1 user pain)

**Problem.** The pre-complete downgrade path recomputes the same multi-section
downgrade Summary each `emit_investigation_complete` attempt, returns
`Success=true`, and only bumps a plain `PreCompleteDowngrades` int no boundary
reads (`emit_investigation_complete.go:1903/1915/2154`). The only read-loop
no-progress primitive (`detectStallAndAct`) trips on **zero closure-delta** once
per explore round, blind to per-attempt downgrade identity. → runaway
"verification not stable" loops, 49k–73k token climbs, killed runs.

**Generalized design.** A per-lane **typed `DowngradeFingerprint`** + a
low-delta convergence boundary at the single emit-attempt site, mirroring the
I4 closure detector but keyed on the downgrade.

- `DowngradeFingerprint{ GateLane DowngradeLane (typed enum per sub-downgrade,
  emitted by the producer — never inferred from the Summary string),
  RequiredCarrierHash, InvalidIDHash, PendingReadOriginHash uint32,
  EvidenceVersion, ReadVersion int }`. Adjacent equal == same blocking lane,
  same required carrier, same invalid ids, **and** no new evidence/read version.
- Boundary built on the **generic `ConvergenceLedger`** (Batch 0): append fp;
  `ConsecutiveEqualTail >= soft` → accept-with-typed-caveat via the existing
  evidence-floor-waiver lane; `>= hard` → block-with-typed-reason via
  `RepairForceCompleteDowngrade`. Thresholds config-driven.

**[RT] corrections folded in:**
- `resolvePreCompleteDowngrade` is the **single gate for both** the downgrade
  TEXT and the repair/violation raising; the producer passes a *deferred* typed
  repair that the helper raises **only on the first (<soft) attempt** — else the
  `AddRepair`/`ViolPreCompleteDowngrade` flood survives text compression and the
  noise claim is false.
- Add an **accepted-closure monotonicity pin** for the acceptable-lane terminal
  flip (no reconcile round re-opens an accepted closure).
- Drop the unsupported "reads violation-cluster repeat count" reuse claim.

**Tasks.** (1) `DowngradeLane` enum + `DowngradeFingerprint` in types. (2)
`downgradeFingerprints` history + `RecentEqualDowngradeCount` on EvidenceClosure
(delegating to ConvergenceLedger). (3) typed `CompletionCaveat` +
`downgradeLaneAcceptableWithCaveat` pure map. (4) `resolvePreCompleteDowngrade`
helper (text+deferred-repair, single gate). (5) `resolveLowDeltaTerminal`
(accept-caveat / force-complete). (6) route all ~13 sub-downgrade producers +
the two call sites through the helper, each emitting its `DowngradeLane`. (7)
config knobs `pipeline_read_downgrade_low_delta_soft/hard` (pointer-typed,
KnownFields-safe). (8) AST red-line: boundary routes only on typed fp fields +
the acceptability map, never on Summary text. **Acceptance:** a no-new-delta
re-attempt converges to accept-caveat or typed-block within `soft`/`hard`
attempts; token/round blowup bounded; `convergence_audit_summary` cases stop
running away.

## Gaps ②+⑤ — Mechanize the typed-signal discipline

**Problem.** No structural guard stops a hard gate from reading `RawRequest` /
model prose (114 raw reads, `SignalAuthority` type = 0), so a prose-keyed gate
re-appears every eval cycle (②). Typed signals are re-derived at N sites with no
single source: `answer_document_evaluator.go:4219` re-implements
producer-precedence inline, so `trace_query:run2` is seen by 2 of 3 channels (⑤).

**Generalized design.** Two mechanized guards, **no heavyweight new "authority"
abstraction** (routing through existing typed fields + the AST guard is the
right weight).

1. **Collapse all divergent channels first** onto the shared chokepoint
   `ObservationLedger.HasDeterministicRuntimeQueryObservation()` /
   `runtimeObservationProducerIsDeterministicQuery`. **[RT]:** there are **three**
   divergent sites, not one — `answer_document_evaluator.go:4219`, **`:6586`**,
   and **`trace_causal_projection.go:97`** — all must route through the
   chokepoint **before** the anti-reimpl guard lands (else it reds the build).
2. **Anti-reimplementation chokepoint test** (AST): fail on inline re-derivation
   of the producer-precedence family. **[RT]:** tie the ban to the chokepoint
   **family** (producer-base normalization for the trace_query/pre-triage set),
   **not** to every `.Producer == <literal>` — that over-fires on ~12 legit
   non-family comparisons (emit_evidence, read_file, concrete_values, …).
3. **Precise-signals AST guard:** within the precise hard-gate function set
   (result type `[]types.Violation` / `contract.Result` / pinned
   `runXxxOracleV2|runXxxCheck|preCheckXxx|xxxRejection` names), fail on a
   `.RawRequest`/prose read **unless** it is an argument to a **named sanctioned
   provenance sink**. **[RT]:** the legit `strings.Contains(rm.RawRequest, …)`
   sites (contract_check_block.go:4344, write_analysis_quality.go) must route
   through a **named typed-token sink** so the guard matches a sink name (sound)
   rather than attempting "typed local" inference it cannot do without type
   resolution. Sanctioned-sink set is the small named allow-list; no per-site
   name silencing.

**Tasks.** (1) collapse `:4219`+`:6586`+`:97`. (2) anti-reimpl family lint
(`internal/types/signal_chokepoint_lint_test.go`). (3) shared AST hard-gate
predicate helper. (4) precise-signals guard in orchestrator. (5) port to
`internal/analysis/gate` + `internal/agent`. (6) introduce the named typed-token
sink + route the 2–3 legit sites; doc the extension path. **Acceptance:**
trace_query eval before/after across **all three** collapsed sites; guards green
on current tree; a synthetic prose-keyed hard gate fails the guard.

## Gap ③ — One compact typed handoff surface (Turn A→B)

**Problem.** The extractor handoff still renders the **raw flat `EvidenceItem`
list + value-lens** (`extractor.go:204-257`) while `ObservationLedger` already
gives finalizer/reviewer a compact typed projection → two parallel surfaces,
context cost scales with raw evidence volume (49k tokens), principal member
buried.

**Generalized design.** Delete the extractor's two raw renderers
(`extractorTranscriptEvidenceItems` flat list + `renderExtractorValueEvidenceFacts`
value-lens + the bespoke `extractorValueFact`/score/needle machinery) and render
one section from `ProjectObservationPromptRecords` (priority-ordered: principal
entities/aggregates first, support refs advisory, exclusions/unknowns typed;
Top-N **by typed role lane**, not raw volume). Reuse the existing
`CompileObservationLedger` call (hoist to compute once). **Net red-line
improvement:** the deleted value-lens token-matched request prose against
evidence text (a noisy-signal-as-selection) — removing it eliminates a real
keyword-match site.

**[RT] BLOCKING fix:** `ObservationRecord.Value` is never set for evidence
records and `observationPromptExcerpt` suppresses current-source excerpts when
`IncludeCurrentSourceExcerpt=false`, so scalar/return/config-literal questions
would **lose the literal** the value-lens preserved. Either populate
`Value`/surface for evidence records at compile time **or** keep the
current-source excerpt on; pin with a **literal-survival test** before deleting
the value-lens. **[RT]:** `ObservationLedger` has ~20 consumers (not 2) — only
add projection where verified missing (the extractor handoff).

**Tasks.** (1) lift the per-record bullet formatter into a shared
`RenderObservationPromptRecordLines`. (2) `ExtractorObservationPromptProjectionOptions(limit)`
sibling. (3) replace flat-list+value-lens with the single ledger section
(hoist compile). (4) **populate evidence-record Value/surface (literal survival)**
+ literal-survival test. (5) delete the dead value-lens machinery. (6)
perf-floods regression: 100+ EvidenceItem → bounded section row count;
steering note (Turn-B can't re-read) keeps repo_map/trace_query typed-view hint.

## Gap ④ — Repo-wide scope-class absence advisory (the correctness gap)

**Problem.** Absence ("no such source files exist") can close while in-scope
files actually exist in-repo, because both absence gates only inspect buffered
evidence / model-named searches, never the repo-wide scope-classed inventory
(`validateAbsenceScopeBound` contract_check_block.go, soft; the buffer-only
`hasGroundedOrRecovered`).

**[RT] — major rework (this gap was the riskiest):**
- **DROP the investigation-side hard gate.** An early reject in the
  `result_kind==absence` branch re-implements the **reverted G1/Plan-E**
  exhaustive-coverage gate (reverted at `emit_investigation_complete.go:1587-1630`
  after the s1a-125430 timeout). Investigation-side coverage enforcement is
  known-bad. **Answer-side only.**
- **SOFT-only.** `(scope files) − ReadSet > 0` is breadth-of-scope — a **noisy**
  signal; per the red line it drives only soft guidance (advisory caveat / retry
  hint), **never a hard reject**.
- **Subtype-aware.** Fire **only** on file-enumeration / member-set absence
  (where in-scope file presence genuinely refutes zero) and stay **inert** on
  symbol-existence / behavior-verdict absence (the canonical supported case at
  schema `:97`) — else it blocks correct symbol-absence answers.
- **Per-question candidate set**, not scope breadth: key it to the analyzer
  primary entity (the explicit replacement direction at
  `emit_investigation_complete.go:1627-1630`), reusing
  `sourceInventory*Candidates` (source_inventory_reconcile.go:3067) — **no new
  parallel `ScopeClassAbsenceRefutation` primitive**.
- **Eval gate first:** prove currently-passing symbol-existence and config-trace
  absence cases still CLOSE under the new code before landing.

**Generalized design (corrected).** An **answer-side, SOFT, subtype-gated**
advisory: when the analyzer classifies a *file-enumeration/member-set* absence
and the existing per-entity candidate enumeration shows in-scope candidate files
that were never surfaced, attach a typed caveat / retry-hint (language-neutral,
via `ClassifySourcePathRole` + the cached graph) — never a hard block.

**Tasks.** (1) absence-subtype typed predicate (file-enum/member-set vs
symbol/behavior) from analyzer IR. (2) answer-side advisory reusing
`sourceInventory*Candidates` per primary entity, subtracting ReadSet∪evidence
Sources. (3) wire as SOFT cause in `validateAbsenceScopeBound` (keep
negative-pattern as second soft cause). (4) AST lint: predicate reads typed
inputs only. (5) eval gate: symbol-existence + config-trace absence still close.

## Cross-cutting / Batch 0 foundation

- **`ConvergenceLedger`** generic (Push / ConsecutiveEqualTail / StalledAt,
  bounded window) — `ClosureFingerprint` + `dataworkflow.ProgressSignature`
  delegate the window/equal-tail mechanics; domain repair policy stays put.
  gap① builds on it. **[RT]:** land first, independently.
- **Decode chokepoint:** the unified layer **already exists**
  (`NormalizeEmitAnswerBlock`/`citationRefFromWire`, answer_block_normalize.go:166).
  **[RT]:** do NOT build a new merged `ApplyDecodeContract` registry (it would
  wrongly merge byte-level wire repair with struct-level validation). Scope:
  ensure new typed fields the gaps add decode through the existing chokepoint;
  add a **decode-chokepoint red-line test** (allow-listing the two existing
  layers) to lock "no new decode site outside the funnel".
- **AST-lint harness template** doc comment as the canonical pattern gaps ②④⑤
  mirror.

---

## Batch plan (each batch: implement → `go test ./...` green → commit → push)

| Batch | Scope | Risk | Depends |
| --- | --- | --- | --- |
| **0** | `ConvergenceLedger` generic + decode-chokepoint red-line test + AST-lint template doc | low | — |
| **1** | Gaps ②+⑤: collapse 3 channels + anti-reimpl lint + precise-signals AST guard + named typed-token sink | med | 0 (harness) |
| **2** | Gap ①: DowngradeFingerprint + low-delta boundary + single resolve gate + monotonicity pin + config | med | 0 (ledger) |
| **3** | Gap ③: extractor → ObservationLedger projection + literal-survival fix + delete value-lens | med-high | 0 |
| **4** | Gap ④: answer-side SOFT subtype-aware absence advisory + eval gate first | high | 1 (sink), 0 |

**Closed-loop definition per gap:** the class-level typed mechanism lands, the
structural/AST guard or regression test pins it, the named eval cases no longer
exhibit the symptom, and `go test ./...` is green with no regression of
currently-passing scenarios.
