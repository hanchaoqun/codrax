# Evidence Salience Typed Lane — Design Doc

**Status**: Draft (2026-05-19) — not yet greenlit
**Baseline**: `58881dba` HEAD at design time. Every `file:line` in §3 / §5 / §6 is pinned to this commit; re-grep helper names rather than absolute line numbers if you re-land after rebases.
**Scope**: 5 phases (B0–B4) producing ~700–1000 net LOC + tests. **Single-repo / current-default-behavior MUST stay byte-identical when the new lane is empty.** L1 red line. **L2**: this lane is opt-in for the LLM; the system never reorders evidence purely because the lane is absent.

---

## §1 Why this work

### 1.1 Forensic state of current ranking

Codrax currently ranks evidence through three deterministic stages and one floor gate, with **no LLM-facing typed lane for "this fact is load-bearing to the answer."** The only model-emitted boolean that touches scoring is `LoadBearingSummary` (`internal/types/evidence.go:693`), and that flag is intentionally narrowed to `summary-text-carries-a-scalar` semantics (`internal/tool/emit_evidence.go:327-329` schema: *"Set true ONLY when the `summary` text holds a scalar (commit hash, version string, count, single concrete identifier...) AND the typed fields cannot themselves carry that scalar"*). It is **not** a general salience signal.

Concrete ranker call sites:

| Stage | Function | File:Line | Signal type |
|---|---|---|---|
| Producer band (primary key, 3 stable buckets) | `evidenceSortRank` | `internal/agent/evidence.go:425-434` | Precise (enum: producer string match) |
| Producer band re-verify at render | `producerRank` | `internal/context/builder.go:1331-1340` | Precise (mirror of above) |
| Producer-band bucket fill, O(N) | `selectEvidenceItemsForRender` | `internal/context/builder.go:1490-1518` | Precise |
| Question-aware re-rank within band | `rankEvidenceByRelevanceWithSubject` | `internal/agent/evidence.go:660-739` | **Noisy** (entity-overlap word frequency × `subject.Score` × `axis.Affinity` matrix) |
| Extractor visibility scoring (`renderExtractorValueEvidenceFacts`) | `extractorValueEvidenceScore` | `internal/agent/extractor.go:804-837` | Mixed (needle freq + kind boost + anchor boost + `LoadBearingSummary` flag) |
| Extractor display cap | `extractorValueEvidenceDisplayLimit` | `internal/agent/extractor.go:655-690` | Precise (analyzer-IR-driven multipliers; range `[12, 32]`) |
| Finalizer block-internal re-rank | `answerDocEnrichmentEvidenceScore` | `internal/agent/answer_document_evaluator.go:3915-…` | Mixed (same as extractor + per-lane bonus) |
| Finalizer display cap | `answerDocEnrichmentDisplayLimit` | `internal/agent/answer_document_evaluator.go:3842-3872` | Precise (range `[8, 28]`) |
| Tier-1-ratio floor (hard gate) | `checkTier1Floor` | `internal/orchestrator/tier1_floor.go:34-72` | Precise; default `Tier1Floor=0` disables it (`internal/tool/grounding_policy.go:82-90`) |

**Where the LLM influences ranking today**:

1. **`LoadBearingSummary bool`** — single-purpose scalar-survival flag (`internal/types/evidence.go:665-693` doc-comment; `internal/tool/emit_evidence.go:151-161` tool wire), trusted blindly when summary ≠ "" (`internal/tool/emit_evidence.go:1109-1111` rejection only on empty summary). Boost: +10–14 to extractor score (`internal/agent/extractor.go:930-936`).
2. **Implicit choice of block sequence in `AnswerDocumentV2.Blocks[]`** — the finalizer renders blocks in model-declared order, but **inside** each block the evidence pool is re-ranked by the system (`selectAnswerDocTypedEnrichmentFacts` at `internal/agent/answer_document_evaluator.go:3756-3825` re-scores + sort + slice).

**Where the LLM cannot influence ranking today**:

- Choosing which specific evidence items are answer-load-bearing vs. supporting vs. context.
- Marking an enumeration's exhaustive set so the display cap does not silently drop a member.
- Pinning evidence the system would otherwise demote via the noisy `rankEvidenceByRelevanceWithSubject` cap (diversity dedup at `internal/agent/evidence.go:716-738` collapses same `(source, subject, anchor_symbol)` to ≤ 2 entries).

### 1.2 Two structural deficits

**Deficit A — noisy signals leak into hard cutoffs.** `extractorValueEvidenceScore` ends with `if score == 0 { return 0 }` (`internal/agent/extractor.go:829-831`), then `renderExtractorValueEvidenceFacts` drops score-0 candidates whenever **any** candidate scored > 0 (`internal/agent/extractor.go:619-627`). The score comes from token-frequency needle matches, whose vocabulary is `extractorValueEvidenceNeedles` (`internal/agent/extractor.go:731-801`): purely tokenized analyzer-IR + `Objective` text. If the user's question vocabulary does not overlap the evidence row's surface tokens (concept ↔ implementation gap), the row is silently dropped — even when the explorer LLM read the file and judged the fact load-bearing. The `LoadBearingSummary` boost (+10–14) **cannot save the row**: the boost is applied *after* the `if score == 0` early-exit at line 829, so a load-bearing row with zero needle matches still returns 0.

**Deficit B — model has no escape lane for over-aggressive caps.** The display cap (extractor `[12, 32]` / finalizer `[8, 28]`) is analyzer-IR-driven only. For exhaustive enumerations or long diagnostic chains the cap is too small (extractor +4 for `Predicates.IsCategoryEnumeration` at `extractor.go:668-670`; finalizer +4 likewise at `answer_document_evaluator.go:3855-3857`). When the explorer correctly identified the full set, there is no typed channel for the model to say *"this is an exhaustive enumeration; do not truncate the tail"*. The existing `EvidenceFloorWaiver` (`internal/types/evidence_floor_waiver.go`) is purely a **grounding-floor** waiver — it does not influence display caps.

### 1.3 Architectural principle

Per `CLAUDE.md` red line (§ Architectural principle) and the post-`2026-05-10` typed-escape pattern (memory anchor: *"系统侧 hard gate MUST 提供 model-declarable typed escape lane,模型 confidence (via typed tool-call field, not `<think>`) 是系统决策 INPUT"*):

> **Precise signals for hard gates; noisy signals for soft guidance only.**
> When the model has stronger judgment than any system-side heuristic on the same question, the model MUST be given a typed lane to express that judgment. The lane is bounded (enum, not free text), trusted on the hard-gate side, recorded for audit, and the system documents its prompt-side teaching so misuse becomes traceable.

This work adds **one** typed lane (`EvidenceSalience`) the explorer can attach to each `EvidenceItem` at `emit_evidence` time. Four enum values, bounded, audited. The same lane teaches the extractor/finalizer score function (soft guidance), the display cap (hard but only widens, never narrows), and the diversity dedup (hard guard against silent drop).

---

## §2 Architecture overview

### 2.1 The lane

A new typed field on `EvidenceItem`:

```go
// internal/types/evidence_salience.go (new file)

// EvidenceSalience names the model's judgment about how this fact
// participates in the user-facing answer. Bounded 4-value enum; the
// zero value means "the explorer made no claim" and is treated as
// SalienceSupporting by every downstream consumer (legacy default).
//
// Salience is the model's typed channel for declaring answer
// participation — orthogonal to grounding (proven? recovered?),
// orthogonal to producer (LLM emit vs. dataflow), orthogonal to
// authority (factual/conditional/historical/illustrative). All four
// axes compose; this axis answers WHO is the answer leaning on.
type EvidenceSalience string

const (
    // SalienceUnset is the legacy default (untyped pre-2026-05-19
    // emits) and the explicit "no claim" emit. Treated as Supporting
    // by downstream consumers.
    SalienceUnset EvidenceSalience = ""

    // SalienceLoadBearing names the answer's commitments-of-record.
    // Dropping this row drops a user-visible claim the answer cannot
    // honor without it. Set when: the row IS the answer to a sub-
    // question, OR the row carries the only proof of a positive
    // assertion the answer will make. Use sparingly — typically 1-4
    // per investigation.
    SalienceLoadBearing EvidenceSalience = "load_bearing"

    // SalienceExhaustListed names a member of an enumeration whose
    // completeness is part of the answer contract. Use when the user
    // asked "list all X" / "what are the N foo" / "enumerate the bar"
    // and this row is one of the members. Hard-protected against
    // display-cap truncation: the system widens the cap rather than
    // drop an exhaust-listed row.
    SalienceExhaustListed EvidenceSalience = "exhaust_listed"

    // SalienceSupporting is the routine middle ground: the row helps
    // the answer chain reach a load-bearing conclusion (intermediate
    // step in a call chain, a guard whose absence would change the
    // verdict, a config layer in a precedence trace). Replaceable by
    // an equivalent row of the same kind/anchor — use SalienceLoad-
    // Bearing instead when no equivalent substitute exists.
    SalienceSupporting EvidenceSalience = "supporting"

    // SalienceContext names a row the model read while investigating
    // but that the answer does not lean on. Background, related
    // context, items emitted to keep the evidence pool diverse.
    // System may demote these freely; they only surface if cap
    // allows after load-bearing / exhaust-listed / supporting are
    // packed.
    SalienceContext EvidenceSalience = "context"
)

func (s EvidenceSalience) IsValid() bool { /* enum-table check */ }
func (s EvidenceSalience) Resolve() EvidenceSalience {
    if s == SalienceUnset {
        return SalienceSupporting
    }
    return s
}
```

The `Resolve()` method centralises the "unset → supporting" projection so downstream consumers never branch on the empty string.

### 2.2 System-side consumption

Three consumption rules, all in the spirit of *"precise typed signal → can drive hard gates; the typed signal must never silently degrade legacy behavior"*:

| Consumer | Rule | Site (new code goes near…) |
|---|---|---|
| **Score function** (`extractorValueEvidenceScore` / `answerDocEnrichmentEvidenceScore`) | `load_bearing` / `exhaust_listed` rows get a deterministic **score floor** (≥ kind-anchor minimum) so noisy needle-zero never reaches the `if score == 0` drop. `supporting` adds a moderate boost. `context` no boost. | `internal/agent/extractor.go:804-837`; `internal/agent/answer_document_evaluator.go:3915-3940` |
| **Display cap** (`extractorValueEvidenceDisplayLimit` / `answerDocEnrichmentDisplayLimit`) | Count of `load_bearing` + `exhaust_listed` rows in pool widens the cap upward (additive, capped at the per-stage hard ceiling). Never narrows. | `internal/agent/extractor.go:655-690`; `internal/agent/answer_document_evaluator.go:3842-3872` |
| **Diversity dedup** (`rankEvidenceByRelevanceWithSubject`) | `load_bearing` and `exhaust_listed` are exempt from the `(source, subject, anchor_symbol)` ≤ 2 cap; they always pass through. | `internal/agent/evidence.go:716-738` |

Producer band (`evidenceSortRank` / `producerRank` / `selectEvidenceItemsForRender`) is **unchanged**. Salience is orthogonal: a `load_bearing` deterministic-extractor row still sits in rank-1 band, but it cannot be silently dropped by score==0 or the diversity cap.

### 2.3 Tier-1 floor interaction (none, by design)

The Tier-1 floor (`internal/orchestrator/tier1_floor.go:34-72`) reads `GroundingStatus`/`GroundingTier`, not salience. Salience MUST NOT change `countTier1Evidence` (`tier1_floor.go:80-106`); a `load_bearing` row that is only `Recovered` still does not count toward the tier-1 numerator. This preserves the existing meaning of the floor: it measures **what the LLM proved by re-reading**, independent of which rows the LLM declared important.

The waiver / disposition lane (`EvidenceFloorWaiver` + `RuntimeGroundingDisposition` at `internal/types/evidence_floor_waiver.go`) remains the floor-relaxation channel. The new salience lane is the **inclusion** channel — different axis.

### 2.4 Prompt-side teaching

Add a short paragraph to `emit_evidence` tool description (`internal/tool/emit_evidence.go:214-263` `Description()`) and the param schema (`internal/tool/emit_evidence.go:265-389` `emitEvidenceParametersSchema`) teaching the four values. The wording mirrors the `LoadBearingSummary` schema description at `:327-329`: bounded, lead with the use case, end with the rejection rule.

No skill prompts (explorer / extractor / finalizer) need ranker-vocabulary changes — the lane name is `salience`, the values are evidence-domain vocabulary, no pipeline-internal terms (R6 red line: no `tier1_floor`, no `extractor`, no `rank profile` leakage).

---

## §3 Existing code we reuse (do NOT reinvent)

### 3.1 Typed lane template (verbatim copy these patterns)

| Already exists | Where | What we copy |
|---|---|---|
| `EvidenceFloorWaiverReason` enum + `IsValid()` | `internal/types/evidence_floor_waiver.go:40-109` | The bounded-enum + `validXxxValues` slice + `IsValid()` shape |
| `RuntimeGroundingDisposition.IsActive()` (predicate gate) | `internal/types/evidence_floor_waiver.go:169-185` | The "fully populated typed lane → boolean predicate" shape; ours is `EvidenceSalience.Resolve() != SalienceUnset` (Step "Resolve" replaces "IsActive") |
| `RuntimeGroundingDisposition` projection from waiver | `internal/types/evidence_floor_waiver.go:199-216` | We do not need a parallel projection (salience is single-stage), but we follow the audit-trail shape: typed enum + the model's rationale is **NOT** required (saves prompt tokens; the enum value is the rationale) |
| `BugClass` typed lane + canonical `HumanZh/HumanEn` | `internal/types/bug_class.go:27-130` | The pattern: typed enum value is internal, human-facing labels are separate. Our salience does NOT need a human-label table — value names are user-facing-vocabulary already |

### 3.2 emit_evidence schema injection points

| Already exists | Where | What we hook |
|---|---|---|
| `emitEvidenceItem` Go struct | `internal/tool/emit_evidence.go:122-172` | Add `Salience string `json:"salience,omitempty"`` next to `LoadBearingSummary` at line 161 |
| `emitEvidenceParametersSchema` properties map | `internal/tool/emit_evidence.go:265-389` | Add the `"salience": map[string]any{ "type": "string", "enum": ..., "description": ...}` block next to `"load_bearing_summary"` at line 327 |
| `buildEmitEvidenceItem` (struct → types.EvidenceItem) | `internal/tool/emit_evidence.go:1090-1115` (around `LoadBearingSummary` copy at :1102) | Add `item.Salience = types.EvidenceSalience(strings.TrimSpace(in.Salience))` + an enum-validity check that rejects unknown values |
| Tool `Description()` | `internal/tool/emit_evidence.go:214-263` | Append one paragraph teaching the 4-value enum, mirroring the `load_bearing_summary` paragraph style |

### 3.3 EvidenceItem field placement

| Already exists | Where | What we add |
|---|---|---|
| `EvidenceItem` struct | `internal/types/evidence.go:550-706` | Add `Salience EvidenceSalience `json:"salience,omitempty"`` immediately after `LoadBearingSummary` at line 693 |
| `EvidenceItem.MergeFrom` (carry-forward on dedup) | `internal/types/evidence.go:1241-…` (around `GroundingStatus` merge logic) | Add salience merge: prefer non-Unset; on conflict, prefer higher-priority value (load_bearing > exhaust_listed > supporting > context) — same priority order as `evidenceSalienceRank` below |
| `EvidenceItem.IsCitable` | `internal/types/evidence.go:960-984` | Unchanged. Salience does not change citability. |

### 3.4 Ranker hook surface

| Already exists | Where | What we modify (small, surgical) |
|---|---|---|
| `evidenceSortRank` | `internal/agent/evidence.go:425-434` | **Unchanged.** Salience does not influence producer band. |
| `rankEvidenceByRelevanceWithSubject` diversity dedup loop | `internal/agent/evidence.go:727-737` | Add `if si.item.Salience.Resolve() == SalienceLoadBearing || si.item.Salience.Resolve() == SalienceExhaustListed { result = append(result, si.item); continue }` BEFORE the `counts[key] >= 2` check |
| `extractorValueEvidenceScore` | `internal/agent/extractor.go:804-837` | Replace the unconditional `if score == 0 { return 0 }` at line 829 with: `if score == 0 && !item.SalienceLockedForScoring() { return 0 }`. Add salience boost AFTER the score==0 check via a new `extractorValueEvidenceSalienceBoost(item.Salience.Resolve(), profile) int` helper (≥ floor of 6 for load_bearing/exhaust_listed; 3 for supporting; 0 for context). |
| `extractorValueEvidenceDisplayLimit` | `internal/agent/extractor.go:655-690` | Add `if n := countLoadBearingOrExhaust(ta); n > 0 { limit += min(n, 8) }` before the min/max clamp. |
| `renderExtractorValueEvidenceFacts` drop-loop | `internal/agent/extractor.go:619-627` | Replace `if candidate.score > 0` with `if candidate.score > 0 || candidate.item.SalienceLockedForScoring()` — guards against silent drop of salience-flagged rows that scored zero on needle. |
| `answerDocEnrichmentEvidenceScore` | `internal/agent/answer_document_evaluator.go:3915-…` | Mirror the extractor change — call shared `extractorValueEvidenceSalienceBoost` (extractor side is the canonical impl, finalizer re-uses to keep the two passes ranking-consistent). |
| `answerDocEnrichmentDisplayLimit` | `internal/agent/answer_document_evaluator.go:3842-3872` | Same widen as extractor: `if n := countLoadBearingOrExhaust(ctx.EvidenceItems); n > 0 { limit += min(n, 8) }` before clamp. |
| `selectAnswerDocTypedEnrichmentFacts` drop-loop | `internal/agent/answer_document_evaluator.go:3806-3813` | Same guard as the extractor `:619-627` change. |

### 3.5 Test scaffolding we reuse

| Already exists | Where | What we use it for |
|---|---|---|
| `internal/agent/evidence_test.go` ranker test patterns | `internal/agent/evidence_test.go` | Add `TestRankEvidenceByRelevance_SalienceExemptsDiversityCap` |
| `internal/agent/extractor_test.go` value-fact tests | search for `TestRenderExtractorValueEvidenceFacts*` | Add `TestExtractorValueEvidenceScore_SalienceLockedSurvivesZeroNeedle` |
| `internal/agent/answer_document_evaluator_test.go:3541` runtime-grounding test pattern | `internal/agent/answer_document_evaluator_test.go:3541-3656` | Mirror the structure for the salience-locked enrichment test |
| `internal/tool/emit_evidence_test.go` schema-validation tests | grep for `TestEmitEvidence_LoadBearingSummary*` | Add `TestEmitEvidence_SalienceEnumValidation` (unknown value → error) |

---

## §4 Data model

### 4.1 New types

**`internal/types/evidence_salience.go`** (new file, ~80 LOC):

```go
package types

import "strings"

type EvidenceSalience string

const (
    SalienceUnset         EvidenceSalience = ""
    SalienceLoadBearing   EvidenceSalience = "load_bearing"
    SalienceExhaustListed EvidenceSalience = "exhaust_listed"
    SalienceSupporting    EvidenceSalience = "supporting"
    SalienceContext       EvidenceSalience = "context"
)

var validEvidenceSaliences = []EvidenceSalience{
    SalienceLoadBearing, SalienceExhaustListed,
    SalienceSupporting, SalienceContext,
}

func EvidenceSalienceValues() []EvidenceSalience { /* defensive copy */ }
func EvidenceSalienceStrings() []string          { /* []string for schema enum */ }

func (s EvidenceSalience) IsValid() bool {
    if s == SalienceUnset { return true } // unset is always valid (default)
    for _, v := range validEvidenceSaliences {
        if v == s { return true }
    }
    return false
}

// Resolve returns Supporting for Unset and the receiver value otherwise.
// Single chokepoint for the "unset → supporting" projection; every
// downstream consumer reads from Resolve(), never from the raw field.
func (s EvidenceSalience) Resolve() EvidenceSalience {
    if s == SalienceUnset { return SalienceSupporting }
    return s
}

// IsLocked reports whether this salience value MUST be protected
// against silent drop / dedup eviction. True for load_bearing and
// exhaust_listed; false for the supporting / context background tier.
func (s EvidenceSalience) IsLocked() bool {
    switch s.Resolve() {
    case SalienceLoadBearing, SalienceExhaustListed:
        return true
    }
    return false
}

// Rank maps salience values into a stable integer for tie-break /
// merge-priority decisions. Lower = higher priority. Same direction
// as evidenceSortRank.
func (s EvidenceSalience) Rank() int {
    switch s.Resolve() {
    case SalienceLoadBearing:   return 0
    case SalienceExhaustListed: return 1
    case SalienceSupporting:    return 2
    case SalienceContext:       return 3
    }
    return 2 // defensive default = supporting
}

// ParseEvidenceSalience normalises a wire-side raw string ("LOAD_BEARING ",
// "load_bearing", "") into the typed value. Returns (SalienceUnset, false)
// for unknown non-empty input so the caller can reject the emit.
func ParseEvidenceSalience(raw string) (EvidenceSalience, bool) {
    trimmed := EvidenceSalience(strings.ToLower(strings.TrimSpace(raw)))
    if trimmed == SalienceUnset { return SalienceUnset, true }
    if trimmed.IsValid() { return trimmed, true }
    return SalienceUnset, false
}
```

**`EvidenceItem` struct addition** (`internal/types/evidence.go:550-706`):

```go
// Add immediately after LoadBearingSummary at line 693:

// Salience is the model's typed claim about how this fact participates
// in the user-facing answer (2026-05-19 add — typed salience lane).
//
// Default zero-value (SalienceUnset) means the explorer made no claim;
// downstream consumers project Unset → Supporting via Salience.Resolve().
//
// The two locked values (SalienceLoadBearing / SalienceExhaustListed)
// are protected against silent drop by the noisy ranker stages
// (needle-zero score-filter at extractor.go:619-627 and 829-831;
// diversity dedup at evidence.go:716-738). The two non-locked values
// (SalienceSupporting / SalienceContext) flow through the legacy soft
// ordering with a small boost / no boost respectively.
//
// Salience is ORTHOGONAL to:
//   - Producer band (evidenceSortRank: explorer.emit > programmatic > dataflow)
//   - GroundingStatus / GroundingTier (proof axis)
//   - Authority (factual / conditional / historical / illustrative)
//   - LoadBearingSummary (a narrow scalar-survival flag for Summary prose)
//
// emit_evidence's prompt teaches the four values; the schema rejects
// unknown enum values via additionalProperties=false + enum constraint.
Salience EvidenceSalience `json:"salience,omitempty"`
```

**Helper on `EvidenceItem`** (`internal/types/evidence.go`, alongside existing helpers around `:960-984`):

```go
// SalienceLockedForScoring is the single chokepoint asked by every
// ranker site that must know "is this row protected from silent drop".
// Centralised so a future policy change (e.g. opt-in/out of the lock
// per-question profile) lands in one place.
func (e EvidenceItem) SalienceLockedForScoring() bool {
    return e.Salience.IsLocked()
}
```

### 4.2 Schema (wire side)

`emit_evidence` schema enum:

```jsonc
"salience": {
  "type": "string",
  "enum": ["load_bearing", "exhaust_listed", "supporting", "context"],
  "description": "OPTIONAL. Names how this fact participates in the user-facing answer. \
load_bearing = the answer cannot honor a claim without this row (use sparingly, typically 1-4 per investigation); \
exhaust_listed = this row is one member of an enumeration whose completeness is part of the answer (\"list all X\" / \"enumerate the bar\"); \
supporting = an intermediate step the answer chain needs (a guard, a config layer, a call-chain hop); \
context = background the answer does not lean on. Omit (default) when uncertain — the system treats unset as supporting. \
load_bearing and exhaust_listed rows are protected against silent display-cap truncation and against same-subject diversity dedup; supporting / context flow through the ordinary soft ordering."
}
```

`emitEvidenceItem` Go struct addition (`internal/tool/emit_evidence.go:122-172`):

```go
Salience string `json:"salience,omitempty"`
```

`buildEmitEvidenceItem` validation (around the `LoadBearingSummary` rejection at `internal/tool/emit_evidence.go:1106-1112`):

```go
salience, ok := types.ParseEvidenceSalience(in.Salience)
if !ok {
    return types.EvidenceItem{}, fmt.Errorf(
        "items[%d]: salience=%q is not one of %v", idx, in.Salience,
        types.EvidenceSalienceStrings())
}
item.Salience = salience
```

### 4.3 Telemetry

Add a `salience` field to the histogram log at `internal/context/builder.go:1432`:

```go
logging.Debug("[trace/fev] %d producer=%q src=%s:%d subj=%q kind=%q claim_form=%q grounding=%s salience=%q",
    i, it.Producer, it.Source, it.LineStart, it.Subject, it.Kind, types.ClaimFormOf(it), it.GroundingStatus, it.Salience.Resolve())
```

Aggregate `counts` (`builder.go:1434-1436`) also tallies salience distribution for post-hoc audit.

---

## §5 Existing-code change sites (exact)

This section is the implementation contract. Each row names the exact line(s) and the surgical edit. **No file-wide refactors.**

### 5.1 New file

| File | LOC | Purpose |
|---|---|---|
| `internal/types/evidence_salience.go` | ~80 | Enum + `Resolve()` + `IsLocked()` + `Rank()` + `ParseEvidenceSalience` |
| `internal/types/evidence_salience_test.go` | ~120 | Enum-table tests + parse-rejection tests |

### 5.2 Modified files

| File | Lines (baseline `58881dba`) | Change |
|---|---|---|
| `internal/types/evidence.go` | After `:693` (after `LoadBearingSummary` field) | Add `Salience EvidenceSalience` field + doc comment |
| `internal/types/evidence.go` | After existing `IsCitable` around `:984` | Add `SalienceLockedForScoring()` helper |
| `internal/types/evidence.go` | In merge logic around `:1241-1260` (`MergeFrom`) | Add salience carry-forward: prefer lower `Rank()` |
| `internal/tool/emit_evidence.go` | After `:161` (`LoadBearingSummary` field in `emitEvidenceItem`) | Add `Salience string `json:"salience,omitempty"`` |
| `internal/tool/emit_evidence.go` | After `:329` (the `load_bearing_summary` schema block) | Add the `salience` schema block (§4.2 above) |
| `internal/tool/emit_evidence.go` | After `:262` (in `Description()`, append before the closing period) | Append the 1-paragraph teaching block (mirrors `load_bearing_summary` doc style) |
| `internal/tool/emit_evidence.go` | After `:1112` (the `LoadBearingSummary` empty-summary rejection) | Add the `ParseEvidenceSalience` rejection (§4.2 above) |
| `internal/agent/evidence.go` | At `:727-737` (diversity dedup loop) | Insert salience-locked exemption BEFORE the `counts[key] >= 2` check |
| `internal/agent/extractor.go` | At `:829-831` (`if score == 0 { return 0 }`) | Change to `if score == 0 && !item.SalienceLockedForScoring() { return 0 }`; immediately after, add `score += extractorValueEvidenceSalienceBoost(item.Salience.Resolve(), profile)` |
| `internal/agent/extractor.go` | After `:936` (`extractorValueEvidenceLoadBearingBoost`) | Add `extractorValueEvidenceSalienceBoost(s EvidenceSalience, profile) int` returning {6, 6, 3, 0} per resolved value |
| `internal/agent/extractor.go` | At `:619-627` (the drop-zero-score filter loop) | Change predicate from `candidate.score > 0` to `candidate.score > 0 \|\| candidate.item.SalienceLockedForScoring()` |
| `internal/agent/extractor.go` | At `:683-690` (display limit clamp tail) | Insert `limit += min(countLoadBearingOrExhaust(ta.EvidenceItems), 8)` before `if limit < extractorMinValueFacts` clamp |
| `internal/agent/extractor.go` | Helper section | Add `countLoadBearingOrExhaust(items []types.EvidenceItem) int` |
| `internal/agent/answer_document_evaluator.go` | At `:3915-3940` (`answerDocEnrichmentEvidenceScore`) | Mirror extractor change: call shared salience boost; the base call to `extractorValueEvidenceScore` already inherits the new behavior via the score==0-with-locked fix above |
| `internal/agent/answer_document_evaluator.go` | At `:3806-3813` (the drop-zero-score filter in `selectAnswerDocTypedEnrichmentFacts`) | Same predicate change as extractor `:619-627` |
| `internal/agent/answer_document_evaluator.go` | At `:3865-3871` (display limit clamp) | Insert `limit += min(countLoadBearingOrExhaust(ctx.EvidenceItems), 8)` before the `if limit < 8` clamp |
| `internal/context/builder.go` | At `:1432` (debug log) | Append `salience=%q` and pass `it.Salience.Resolve()` |

**Producer band (`evidenceSortRank`, `producerRank`, `selectEvidenceItemsForRender`) is UNCHANGED.** Salience is orthogonal to producer.

### 5.3 Single-call-site discipline (don't fan out)

`SalienceLockedForScoring()`, `Salience.Resolve()`, `countLoadBearingOrExhaust()`, and `extractorValueEvidenceSalienceBoost()` are the **only** new helpers. Every system-side branch on salience reads through one of these four. Tests assert this (grep test: no `item.Salience ==` outside the four helpers + their test files).

---

## §6 Implementation phases

Five phases. Each phase ships independently (tests + commit + go test ./... green) and the salience-empty case is byte-identical to baseline at every phase boundary.

### B0 — Types and wire (foundational, no behavior change)

**Scope**: `internal/types/evidence_salience.go` (new), `internal/types/evidence_salience_test.go` (new), `EvidenceItem.Salience` field, `MergeFrom` carry-forward, schema enum, `emitEvidenceItem` field, `buildEmitEvidenceItem` validation, `Description()` paragraph.

**Acceptance**: All existing tests green. A new emit_evidence test with `salience=""` produces an item where `item.Salience.Resolve() == SalienceSupporting`. An emit with `salience="load_bearing"` succeeds; `salience="LOAD-BEARING"` is rejected. Schema introspection (`json.Unmarshal` of the schema bytes) lists the four enum values verbatim.

**Behavior change**: None. The field is added but no consumer reads it yet.

**LOC estimate**: ~280 (80 new file + 120 test + 80 modifications).

### B1 — Diversity-cap exemption (smallest behavior change)

**Scope**: `internal/agent/evidence.go:716-738` diversity dedup loop. `load_bearing` and `exhaust_listed` rows pass through.

**Acceptance**: New test `TestRankEvidenceByRelevance_SalienceExemptsDiversityCap` — 5 rows with same `(source, subject, anchor_symbol)` but distinct content, 3 marked `load_bearing`, 2 unmarked. All 3 load-bearing survive; 2 of the 2 unmarked are kept (cap was 2, so both fit), 0 dropped. Without salience flag, only 2/5 survive (regression-test the legacy behavior in the same file with `salience=""`).

**Behavior change**: Only when the LLM sets the flag.

**LOC estimate**: ~120 (10 prod + 110 test).

### B2 — Extractor score floor + drop-loop guard

**Scope**: `internal/agent/extractor.go:619-627`, `:804-837`, `:930-…` (new helper), `:655-690` (display cap widen).

**Acceptance**:
1. `TestExtractorValueEvidenceScore_SalienceLockedSurvivesZeroNeedle` — row with `salience=load_bearing` and 0 needle matches retains a score ≥ 6 instead of 0.
2. `TestRenderExtractorValueEvidenceFacts_LoadBearingNotDroppedWhenOthersScore` — when at least one row scores > 0, a `salience=load_bearing` row with score==0 is **still** rendered.
3. `TestExtractorValueEvidenceDisplayLimit_WidensForLoadBearingExhaust` — 6 `exhaust_listed` rows in the pool widen the default cap of 16 to 22 (capped at 24, then by `extractorMaxValueFacts=32`).

**Behavior change**: Adds rows previously silently dropped. Display cap may widen.

**LOC estimate**: ~200 (60 prod + 140 test).

### B3 — Finalizer mirror

**Scope**: `internal/agent/answer_document_evaluator.go:3806-3813`, `:3842-3872`, `:3915-3940`. Mirror B2 in the finalizer pass.

**Acceptance**: Three tests parallel to B2 in `answer_document_evaluator_test.go`. Re-rank consistency: a pool that the extractor surfaces top-N should be a stable prefix of the finalizer's top-N when the analyzer IR is unchanged.

**Behavior change**: Same as B2, finalizer-side.

**LOC estimate**: ~180.

### B4 — Skill-prompt teaching + telemetry

**Scope**:
1. `internal/tool/emit_evidence.go:214-263` tool `Description()`: append the salience paragraph.
2. `internal/skill/explorer*.go` (locate via `grep -rn "load_bearing_summary" internal/skill/`): add a one-sentence pointer next to the existing `load_bearing_summary` teaching so the explorer skill consistently teaches both flags. R6: no internal-pipeline terms.
3. `internal/context/builder.go:1432` debug log: add `salience=%q`.
4. New eval fixture: an enumeration question where the explorer must mark all 7 members `exhaust_listed`; the finalizer answer's verbatim member list MUST contain all 7 (today it sometimes drops 1-2 to the cap).

**Acceptance**: Eval fixture passes 4/4 reruns. `grep -r "salience" internal/skill/` shows the teaching is co-located with `load_bearing_summary`. Production debug logs include the histogram.

**Behavior change**: Skill prompts now teach the lane. LLM can start using it.

**LOC estimate**: ~150 (mostly skill prose + fixture).

---

## §7 Red lines (enforced by tests)

- **L1 — single-repo / salience-empty byte-identical**: every prod test under `internal/agent/`, `internal/context/`, `internal/orchestrator/`, `internal/tool/` MUST stay green without modification when salience is unset on all evidence items. Add a structural test `TestEvidenceItem_LegacyDefaultSalienceTreatsAsSupporting` asserting `EvidenceItem{}.Salience.Resolve() == SalienceSupporting`.
- **L2 — opt-in, never coerced**: the system MUST NOT auto-fill `Salience` on items the LLM did not flag. Programmatic extractors (`concrete_values`, `dataflow.*`) emit with `salience=""`. Test: `TestProgrammaticEvidence_NeverAutoSetsSalience`.
- **L3 — no leakage into producer band**: `evidenceSortRank` / `producerRank` / `selectEvidenceItemsForRender` MUST NOT read `Salience`. Test: `TestProducerRank_IgnoresSalience` constructs two items with identical Producer but different Salience and asserts the same rank.
- **L4 — no leakage into Tier-1 floor**: `checkTier1Floor` / `countTier1Evidence` / `countGroundingHealth` (`internal/orchestrator/tier1_floor.go`) MUST NOT read `Salience`. Test: `TestCheckTier1Floor_IgnoresSalience` runs both 100%-load-bearing-but-recovered and 100%-context-but-grounded scenarios, asserts the floor computes ratio off `GroundingStatus`/`GroundingTier` only.
- **L5 — single chokepoint discipline**: only four helpers (`Salience.Resolve()`, `Salience.IsLocked()`, `Salience.Rank()`, `SalienceLockedForScoring()`) AND one count helper (`countLoadBearingOrExhaust`) AND one boost helper (`extractorValueEvidenceSalienceBoost`) read the field. Lint test (grep-based): no other `\.Salience\b` in production code outside `internal/types/evidence_salience.go` and `internal/types/evidence.go` (field declaration + merge).
- **L6 — no internal-pipeline terms in LLM prompts**: `salience` paragraph in `Description()` and `salience` schema description MUST NOT mention `tier1_floor`, `extractor`, `finalizer`, `producer rank`, `display cap` (use "the system may truncate long lists" instead), or any Go identifier. Test: `TestEmitEvidenceToolDescription_NoInternalLeakage` grep-asserts against a blocklist.
- **L7 — schema and Go enum stay in sync**: `EvidenceSalienceStrings()` (Go) MUST equal the schema enum literal verbatim. Test: `TestEvidenceSalienceSchemaSyncedWithGo` decodes the schema JSON and string-compares the enum list against `EvidenceSalienceStrings()`.

---

## §8 Test plan summary

| Phase | Test file | New tests |
|---|---|---|
| B0 | `internal/types/evidence_salience_test.go` (new) | Enum validity, `ParseEvidenceSalience` rejection, `Resolve()` projection, `IsLocked()` truth table, `Rank()` ordering |
| B0 | `internal/types/evidence_test.go` | `EvidenceItem.MergeFrom` salience carry-forward (lower rank wins; Unset loses to anything; among locked, prefer LoadBearing > ExhaustListed) |
| B0 | `internal/tool/emit_evidence_test.go` | Schema validation: unknown value rejected; empty value accepted as Unset; case-sensitivity (`LOAD_BEARING` rejected) |
| B1 | `internal/agent/evidence_test.go` | `TestRankEvidenceByRelevance_SalienceExemptsDiversityCap` (locked rows bypass `(source, subject, anchor_symbol) ≤ 2` cap) |
| B2 | `internal/agent/extractor_test.go` | (1) score==0 + locked → survives; (2) drop-loop guard; (3) display cap widens |
| B3 | `internal/agent/answer_document_evaluator_test.go` | Mirror of (1)(2)(3) for the finalizer enrichment pass; consistency check that finalizer top-N is a prefix of extractor top-N when IR is unchanged |
| B4 | `internal/tool/emit_evidence_test.go` | `TestEmitEvidenceToolDescription_NoInternalLeakage` (R6 leakage blocklist) |
| All | Structural | L1-L7 red-line tests (see §7) |

Eval bar (B4):
- `qf_arch` × 4 reruns: stays at current pass rate (no regression).
- Add a new enumeration fixture (~ "list all 7 things X does") under `eval/cases/`: with the lane, 4/4 reruns include all 7 members verbatim in the visible answer. Without the lane (rerun on `58881dba`), expected baseline pass rate ≤ 2/4.

---

## §9 Open questions

These are deliberate non-decisions; flag them to the user before B0 lands.

1. **`SalienceSupporting` boost magnitude**. Proposal in §3.4 is +3 (vs. +6 for locked, 0 for context). Smaller than `LoadBearingSummary`'s +10–14 because supporting is the *legacy default* — too-large a boost would shift current rankings on legacy emits when the LLM starts using the lane sparingly. We could keep supporting at +0 and reserve any boost for locked rows. Recommend +3 as a soft signal; revisit after B4 eval.
2. **Display-cap widening cap**. Proposal: `+min(n, 8)` — at most 8 extra slots regardless of how many locked rows. 8 is half of `extractorDefaultValueFacts=16`. Could be lower (4) to be more conservative; could be unbounded but capped by `extractorMaxValueFacts=32`. Pick a value at B2 design-review.
3. **Should `evidenceSortRank` (producer band) read salience?** Proposal: NO (L3 red line). Argument: producer band measures *how the row was authored* (LLM judgment vs. mechanical extraction), salience measures *what role the row plays in the answer*. Mixing them would let a load-bearing dataflow row out-rank a supporting explorer emit — interesting but a large behavior change. Recommend keeping orthogonal and revisiting only if B4 eval shows a need.
4. **Should the finalizer block-order be salience-aware?** Today the LLM chooses `AnswerBlock` order and the system preserves it. We could optionally sort blocks by max-salience-of-evidence-in-block — but this fights the LLM's narrative ordering and is the kind of change that would surprise users. Recommend NO; if surfaced later, treat as its own design doc.
5. **Should `EvidenceFloorWaiver` consumers (`emit_investigation_complete.go:621`, `answer_document_pre_emit_check.go:478`, etc.) read salience to decide which kinds of evidence "count" toward floor relaxation?** Likely no — those gates are about runtime-artifact disposition, not in-repo salience. Confirm during B0.

---

## §10 Out of scope (deliberately)

- **Numeric salience scores**. The user red line is clear: numeric model-emitted scores invite incomparable subjective ratings. Stick to the bounded enum.
- **System auto-inferring salience from keyword overlap**. Violates L2; the lane is the model's voice.
- **Per-block / per-citation salience**. Salience lives on the `EvidenceItem`, not the rendered citation. The finalizer can decide what to show; the model only labels what is important about each fact.
- **Backwards-compatible coercion of `LoadBearingSummary=true` → `salience=load_bearing`**. They are different semantics: `LoadBearingSummary` says the *prose summary* must survive; `Salience=load_bearing` says the *row* must survive. The two are independent (e.g. an exhaustively-listed config-key row may not need its prose to survive). Keep them independent; document the distinction in the `Description()` paragraph.

---

## §11 Speed-check (after every phase)

`go test ./...` must stay 50/50 packages green at every commit. After B4, run the eval fixture × 4 to confirm the bar. After B4 commit lands, update `MEMORY.md` with a one-line entry pointing here.
