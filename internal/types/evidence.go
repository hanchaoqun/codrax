package types

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// EvidenceKind classifies a structured evidence item produced by either
// the LLM investigation notes or the deterministic analysis layers.
type EvidenceKind string

const (
	EvidenceDirect       EvidenceKind = "direct"
	EvidenceConditional  EvidenceKind = "conditional"
	EvidenceRegistration EvidenceKind = "registration"
	EvidenceMechanism    EvidenceKind = "mechanism"
	EvidenceRelationship EvidenceKind = "relationship"
	EvidenceAbsent       EvidenceKind = "absent"
	EvidenceConcrete     EvidenceKind = "concrete_value"
	EvidenceDataflowPath EvidenceKind = "dataflow_path"
	EvidenceConflict     EvidenceKind = "conflict"
	EvidenceUnresolved   EvidenceKind = "unresolved"
	EvidenceTruncated    EvidenceKind = "analysis_truncated"
)

// allEvidenceKinds is the canonical, ordered list of every
// EvidenceKind value. The first six are LLM-emittable via
// emit_evidence; the last five are deterministic-only (written by
// mechanism_scan, dataflow/lower, the concrete-values extractor, and
// the analysis-truncation reporter) and are intentionally rejected
// if the LLM tries to emit them — see IsLLMEmittable below.
var allEvidenceKinds = []EvidenceKind{
	EvidenceDirect,
	EvidenceConditional,
	EvidenceRegistration,
	EvidenceMechanism,
	EvidenceRelationship,
	EvidenceAbsent,
	EvidenceConcrete,
	EvidenceDataflowPath,
	EvidenceConflict,
	EvidenceUnresolved,
	EvidenceTruncated,
}

// AllEvidenceKinds returns the canonical list of every EvidenceKind
// value in stable declaration order. Callers must not mutate the
// returned slice.
func AllEvidenceKinds() []EvidenceKind {
	out := make([]EvidenceKind, len(allEvidenceKinds))
	copy(out, allEvidenceKinds)
	return out
}

// IsLLMEmittable reports whether this EvidenceKind is one the LLM is
// allowed to produce through the emit_evidence tool. The five
// "investigation-shape" kinds (direct / conditional / registration /
// mechanism / relationship) are emittable; the six remaining kinds
// are not — they fall into two families:
//
//  1. Deterministic-only: concrete_value / dataflow_path / conflict /
//     unresolved / analysis_truncated are written exclusively by Go
//     code that has already done the structural work, so allowing
//     the LLM to emit them would let it launder unverified claims
//     through a channel whose semantic contract is "I ran a
//     deterministic check".
//
//  2. Schema-deprecated: absent. An absent item has no concrete file
//     anchor (the whole point is "I searched and found nothing"),
//     but the tool's validator requires line_start > 0 + anchor_kind +
//     anchor_symbol uniformly across all kinds. The semantic
//     contradiction produced the logtri_custom rejection loop
//     (LLM retries kind=absent with line_start=0, gets uniformly
//     rejected, eventually hacks around it by picking an arbitrary
//     "related" file line). The proper absence channel is
//     emit_investigation_complete.absence_justification, which is
//     whole-answer scoped, has dedicated validation, and waives the
//     citation floor by contract. Keeping EvidenceAbsent as a type
//     constant for the legacy prose-tag parser (agent/evidence.go
//     parseEvidenceLine reads `[ABSENT]` bracketed-tag notes from
//     the LLM's <think> blocks, an independent pre-tool channel
//     that never had the validator problem), while removing it from
//     the emit_evidence tool schema.
//
// The emit_evidence tool derives its schema enum and its accept-list
// from this predicate so the canonical list and the tool contract
// cannot drift apart.
func (k EvidenceKind) IsLLMEmittable() bool {
	switch k {
	case EvidenceDirect, EvidenceConditional, EvidenceRegistration,
		EvidenceMechanism, EvidenceRelationship:
		return true
	}
	return false
}

// LLMEmittableEvidenceKinds returns the subset of AllEvidenceKinds
// for which IsLLMEmittable is true, in canonical order. This is what
// the emit_evidence tool schema's enum is built from.
func LLMEmittableEvidenceKinds() []EvidenceKind {
	out := make([]EvidenceKind, 0, len(allEvidenceKinds))
	for _, k := range allEvidenceKinds {
		if k.IsLLMEmittable() {
			out = append(out, k)
		}
	}
	return out
}

// GroundingStatus is the post-validation verdict attached to each
// EvidenceItem by the grounding layer (internal/tool/ground). Three
// values replace the old binary "grounded vs /ungrounded-suffixed"
// model:
//
//   - Grounded: Tier 1 (line_text) or Tier 2 (symbol_table) accepted
//     the item's (Source, LineStart, AnchorSymbol) triple verbatim.
//   - Recovered: one of the Recovery tiers (R1-R5) found a nearby
//     match and rewrote LineStart and/or Source. The original LLM
//     claim was close enough that the system could repair it. The
//     item is still usable as a citation.
//   - Ungrounded: all tiers failed. The item is preserved as a
//     "lead" but is kept out of citation pools and rendered in a
//     dedicated "Unverified Leads" section. LineStart keeps the
//     LLM's original value so the note carries a human-readable
//     reference, but downstream must not emit it as a confirmed
//     file:line citation.
type GroundingStatus string

const (
	GroundingGrounded   GroundingStatus = "grounded"
	GroundingRecovered  GroundingStatus = "recovered"
	GroundingUngrounded GroundingStatus = "ungrounded"
)

// AnchorKind tells the grounder what KIND of code location the
// EvidenceItem.LineStart points at. Emitted by the LLM through the
// emit_evidence tool, required (not optional) so Tier 2 and the
// recovery tiers have a concrete dispatch key — otherwise the grounder
// would have to guess "is this the definition line? a call site? a
// return statement?" from Subject text, which was the source of the
// same-name-across-receivers misgrounding class.
//
// AnchorImport covers `import`/`use`/`require` statements whose
// AnchorSymbol is typically a package path or alias.
type AnchorKind string

const (
	AnchorDefinition AnchorKind = "definition"
	AnchorCall       AnchorKind = "call"
	AnchorCondition  AnchorKind = "condition"
	AnchorReturn     AnchorKind = "return"
	AnchorAssignment AnchorKind = "assignment"
	AnchorImport     AnchorKind = "import"
)

var allAnchorKinds = []AnchorKind{
	AnchorDefinition, AnchorCall, AnchorCondition,
	AnchorReturn, AnchorAssignment, AnchorImport,
}

// AllAnchorKinds returns the canonical list of every AnchorKind value
// in stable declaration order. Used by the emit_evidence schema to
// drive the required-enum list.
func AllAnchorKinds() []AnchorKind {
	out := make([]AnchorKind, len(allAnchorKinds))
	copy(out, allAnchorKinds)
	return out
}

// EvidenceContextRole classifies how an evidence item should be used
// when the user asked about an exact target. The LLM may recommend a
// role through emit_evidence, but the system validates and can
// downgrade the role before storing it on the item.
type EvidenceContextRole string

const (
	EvidenceContextRoleUnknown          EvidenceContextRole = ""
	EvidenceContextRoleDefining         EvidenceContextRole = "defining"
	EvidenceContextRoleAbsenceSupport   EvidenceContextRole = "absence_support"
	EvidenceContextRoleRelatedContext   EvidenceContextRole = "related_context"
	EvidenceContextRoleIllustrativeOnly EvidenceContextRole = "illustrative_only"
)

var allEvidenceContextRoles = []EvidenceContextRole{
	EvidenceContextRoleUnknown,
	EvidenceContextRoleDefining,
	EvidenceContextRoleAbsenceSupport,
	EvidenceContextRoleRelatedContext,
	EvidenceContextRoleIllustrativeOnly,
}

func AllEvidenceContextRoles() []EvidenceContextRole {
	out := make([]EvidenceContextRole, len(allEvidenceContextRoles))
	copy(out, allEvidenceContextRoles)
	return out
}

func (r EvidenceContextRole) IsValid() bool {
	for _, declared := range allEvidenceContextRoles {
		if r == declared {
			return true
		}
	}
	return false
}

// EvidenceDiagramRole classifies where an evidence item sits inside a
// config-precedence diagram. Like ContextRole, this is an
// LLM-recommended signal that the system validates structurally.
type EvidenceDiagramRole string

const (
	EvidenceDiagramRoleUnknown  EvidenceDiagramRole = ""
	EvidenceDiagramRoleDefault  EvidenceDiagramRole = "default"
	EvidenceDiagramRoleConfig   EvidenceDiagramRole = "config"
	// EvidenceDiagramRoleYAML is a deprecated alias kept so older
	// tests / saved traces continue to compile; the canonical role is
	// now `config`, which covers YAML/JSON/TOML/INI/... config-file
	// layers instead of only YAML.
	EvidenceDiagramRoleYAML     EvidenceDiagramRole = EvidenceDiagramRoleConfig
	EvidenceDiagramRoleRuntime  EvidenceDiagramRole = "runtime"
	EvidenceDiagramRoleOverride EvidenceDiagramRole = "override"
)

var allEvidenceDiagramRoles = []EvidenceDiagramRole{
	EvidenceDiagramRoleUnknown,
	EvidenceDiagramRoleDefault,
	EvidenceDiagramRoleConfig,
	EvidenceDiagramRoleRuntime,
	EvidenceDiagramRoleOverride,
}

func AllEvidenceDiagramRoles() []EvidenceDiagramRole {
	out := make([]EvidenceDiagramRole, len(allEvidenceDiagramRoles))
	copy(out, allEvidenceDiagramRoles)
	return out
}

func (r EvidenceDiagramRole) IsValid() bool {
	for _, declared := range allEvidenceDiagramRoles {
		if r == declared {
			return true
		}
	}
	return false
}

// GroundingTier names the exact tier that produced a grounded or
// recovered verdict. Rendered in the per-item feedback the
// emit_evidence tool returns, so the LLM can tell "my line was right"
// from "my line was close and the system adjusted it".
type GroundingTier string

const (
	TierLineText         GroundingTier = "line_text"
	TierSymbolTable      GroundingTier = "symbol_table"
	TierFQNameSameFile   GroundingTier = "fqname_same_file"
	TierSnippetFuzzy     GroundingTier = "snippet_fuzzy"
	TierPackageSymbol    GroundingTier = "package_symbol"
	TierNearestCall      GroundingTier = "nearest_call"
	TierNearestCondition GroundingTier = "nearest_condition"
)

// EvidenceItem is the normalized, structured representation of a
// single evidence statement that can be carried across agents/stages.
//
// Anchor* fields are new (2026-04-17 redesign) and required on any
// LLM-produced item. Snippet is optional but improves Tier R2
// (snippet_fuzzy) recovery accuracy. Grounding* fields are filled by
// internal/tool/ground.GroundItem at emit_evidence.Execute time.
type EvidenceItem struct {
	ID          string       `json:"id"`
	Kind        EvidenceKind `json:"kind"`
	Subject     string       `json:"subject,omitempty"`
	Predicate   string       `json:"predicate,omitempty"`
	Object      string       `json:"object,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Condition   string       `json:"condition,omitempty"`
	Source      string       `json:"source,omitempty"`
	EvidenceRef string       `json:"evidence_ref,omitempty"`
	LineStart   int          `json:"line_start,omitempty"`
	LineEnd     int          `json:"line_end,omitempty"`
	DerivedFrom []string     `json:"derived_from,omitempty"`
	Confidence  float64      `json:"confidence,omitempty"`
	Producer    string       `json:"producer,omitempty"`

	// Role fields: the system-validated usage lanes for exact-target
	// and config-trace questions. Populated from optional emit_evidence
	// hints and/or structural validation after grounding.
	ContextRole EvidenceContextRole `json:"context_role,omitempty"`
	DiagramRole EvidenceDiagramRole `json:"diagram_role,omitempty"`

	// Anchor fields: required for LLM-emitted items, let the grounder
	// dispatch Tier 2 and the recovery tiers without guessing.
	AnchorKind   AnchorKind `json:"anchor_kind,omitempty"`
	AnchorSymbol string     `json:"anchor_symbol,omitempty"`
	Snippet      string     `json:"snippet,omitempty"`

	// Grounding output: filled by the grounder. Downstream renderers
	// branch on GroundingStatus; Tier and Note are human-readable.
	GroundingStatus GroundingStatus `json:"grounding_status,omitempty"`
	GroundingTier   GroundingTier   `json:"grounding_tier,omitempty"`
	GroundingNote   string          `json:"grounding_note,omitempty"`
}

// EvidenceCountsTowardTier1Floor reports whether an evidence item
// should contribute to the Tier-1 proven-ratio denominator used by
// explorer completion and the orchestrator's pre-finalize gate.
//
// The floor is meant to protect citation-bearing answer anchors:
// facts the finalizer may actually need to cite. Auxiliary lanes that
// the system has already classified as non-defining context
// (illustrative examples, absence-support prose, unresolved exact-hit
// mentions) should not bloat the denominator and force an otherwise
// sufficient investigation to reopen just to repair evidence the
// pipeline itself says must remain context only.
func EvidenceCountsTowardTier1Floor(ev EvidenceItem) bool {
	if ev.Kind == EvidenceUnresolved {
		return false
	}
	switch ev.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	default:
		return true
	}
}

// FlowFindingDigest is the compact, stage-safe output of the dataflow
// engine. It intentionally omits the full graph and preserves only the
// user-facing path plus the evidence IDs needed for replay.
type FlowFindingDigest struct {
	ID                string   `json:"id"`
	Path              []string `json:"path,omitempty"`
	Conditions        []string `json:"conditions,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	Sinks             []string `json:"sinks,omitempty"`
	Hops              []string `json:"hops,omitempty"`
	EvidenceIDs       []string `json:"evidence_ids,omitempty"`
	UnsupportedReason string   `json:"unsupported_reason,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
}

// StableEvidenceID returns a deterministic ID derived from the
// semantically relevant evidence fields. Callers should use this for
// dedupe/merge so that the same evidence coming from main/sub explorers
// or deterministic passes coalesces cleanly.
func StableEvidenceID(kind EvidenceKind, subject, predicate, object, condition, source string, lineStart, lineEnd int) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		string(kind),
		subject,
		predicate,
		object,
		condition,
		source,
		fmt.Sprintf("%d", lineStart),
		fmt.Sprintf("%d", lineEnd),
	}, "\x1f")))
	return fmt.Sprintf("ev-%x", h.Sum64())
}

// IsCitable reports whether this evidence item carries a file:line
// anchor that downstream stages (extractor, finalizer) can trust as
// a citation target without re-grounding. The positive set is
// deliberately narrow: Tier-1-proven (the LLM actually read the
// file) OR Tier-2-proven (the LLM's claim matched the repomap
// graph structurally). Recovered and Ungrounded items have a line
// number that the finalizer's strict Tier-2 check may reject at
// citation time, so prompt-layer renderers strip LineStart for
// those to prevent downstream LLMs from citing them.
//
// Legacy items with empty GroundingStatus (pre-session-5
// deterministic concrete_value) count as citable — they are
// deterministic facts, not LLM claims.
func (e EvidenceItem) IsCitable() bool {
	switch e.GroundingStatus {
	case GroundingGrounded:
		return true
	case GroundingRecovered, GroundingUngrounded:
		return false
	default:
		return true
	}
}

// DisplayLocation renders the "file:line" marker appropriate for a
// given strictness:
//
//   - strict=false: historical behaviour. Full "file:line" when Source
//     and LineStart are set; just "file" otherwise.
//   - strict=true: file:line ONLY when IsCitable; recovered/ungrounded
//     items show file alone (line suppressed). Used at the Turn B
//     / finalize boundary so downstream LLMs cannot pick up a
//     recovered line number that finalize-time grounding will later
//     reject.
//
// Empty source returns "".
func (e EvidenceItem) DisplayLocation(strict bool) string {
	if e.Source == "" {
		return ""
	}
	if strict && !e.IsCitable() {
		return e.Source
	}
	if e.LineStart > 0 {
		if e.LineEnd > e.LineStart {
			return fmt.Sprintf("%s:%d-%d", e.Source, e.LineStart, e.LineEnd)
		}
		return fmt.Sprintf("%s:%d", e.Source, e.LineStart)
	}
	return e.Source
}

// CompletenessClaim is the set-level authority the producer of an
// AnswerSymbol slate attaches to that slate. It answers "how
// authoritative is this list?" — a question that extractAnswerSymbols
// (the pre-P2.1 selection layer) could never answer and that the
// downstream rendering layer (context/builder.go §Answer Symbols)
// silently defaulted to "complete", producing the UNRESOLVED #1 bug
// where a partial LLM-derived allowlist was sold to the finalizer as
// a verified-complete answer. See memory/project_p2_1_session_1_shipped.md.
//
// The three claims form a strict authority ladder:
//
//   - CompletenessComplete: these are ALL the answers. The finalizer
//     runs its Translation mode prompt with "MUST NOT add or remove
//     symbols" directive. Safe only when the upstream producer has
//     structurally validated that no answers were missed.
//
//   - CompletenessLowerBound: these are confirmed present, but more
//     may exist. The finalizer runs a softened prompt: "MUST include
//     at least these names; MAY add more if evidence supports them."
//     This is the honest default when a partial allowlist is the best
//     we have — it preserves the floor without forbidding the ceiling.
//
//   - CompletenessUnknown: no authority at all. The finalizer drops
//     the answer-symbols section entirely and falls back to the
//     shape-based prompt (step_list / explanation / etc.). Zero-value
//     of the type so an un-set field naturally means "no claim".
//
// The enum is closed — the emit_answer_symbol schema in P2.1 P9 will
// reject any other string. CompletenessUnknown is the zero value so
// that legacy code paths that do not set the field degrade safely.
type CompletenessClaim string

const (
	// CompletenessUnknown is the zero value. It means the producer
	// either did not run a cardinality check at all, or ran one and
	// could not reach a definitive verdict. The rendering layer must
	// treat the answer-symbol slate as non-authoritative and fall back
	// to the shape-based prompt. Leaving the field at zero value is
	// always safe; it is the "fail closed" default for P2.1's
	// completeness contract.
	CompletenessUnknown CompletenessClaim = ""

	// CompletenessComplete asserts that the attached slate lists every
	// symbol that answers the question. The finalizer runs Translation
	// mode with "MUST NOT add or remove symbols". Producers MUST have
	// run a structural completeness check (Turn A's deterministic
	// extraction with a ≥1 baseline, or Turn B's emit_answer_symbol
	// claim validated against Turn A's TerminalEvidenceCount and the
	// AnalysisIR.AnswerContract.MustInclude cross-ref) before writing
	// this value. Writing it without the check is forbidden by the
	// P9 schema validator.
	CompletenessComplete CompletenessClaim = "complete"

	// CompletenessLowerBound asserts that the attached slate is a
	// proper floor on the true answer set — every listed symbol is
	// confirmed present, but the slate may be missing additional
	// symbols that also answer the question. The finalizer runs the
	// softened Translation prompt that preserves the floor without
	// forbidding the ceiling. This is the right claim when
	// investigation was bounded (read-file budget hit, grep recall
	// partial, etc.) but produced high-confidence matches.
	CompletenessLowerBound CompletenessClaim = "lower_bound"
)

// IsValid reports whether c is one of the three defined CompletenessClaim
// values. Used by the emit_answer_symbol schema validator and by
// applyStageOutput's merge rule so an invalid value cannot leak into
// BusContext.
func (c CompletenessClaim) IsValid() bool {
	switch c {
	case CompletenessUnknown, CompletenessComplete, CompletenessLowerBound:
		return true
	}
	return false
}

// AnswerSymbol is the structured, deterministic form of a single
// answer. Produced by Turn B's extractor via emit_answer_symbol,
// it is the bridge between the deterministic pipeline (which
// identifies the correct symbol) and the finalizer (which must
// render it as prose without adding or removing names).
type AnswerSymbol struct {
	Name      string           `json:"name"`
	File      string           `json:"file,omitempty"`
	Line      int              `json:"line,omitempty"`
	Chain     string           `json:"chain"`               // full chain text that yielded this symbol
	Kind      AnswerSymbolKind `json:"kind"`                // see answer_symbol_kind.go for the closed taxonomy
	Rationale string           `json:"rationale,omitempty"` // optional: why this terminal was picked
}

// AnswerChain is the typed ranked-and-scored answer-relevance envelope
// around an EvidenceItem. Produced by identifyAnswerChains (pure Go,
// deterministic, no LLM) from the explorer's structuredEvidence pool.
//
// Design rationale: identifyAnswerChains used to return []string where
// each string was a pre-flattened render of the underlying evidence
// (ev.Summary plus a " (file:line)" suffix). That violated the
// architecture principle "prose only at the LLM boundary" because the
// flattening happened in Go mid-pipeline, losing the structured fields
// for every downstream consumer. The typed form keeps the underlying
// EvidenceItem intact and defers rendering to context/builder.go's
// prompt assembly step (the single legal flatten point).
//
// Fields:
//   - Item: the underlying EvidenceItem (all structured fields —
//     Subject / Predicate / Object / Source / LineStart / Summary /
//     Kind / Confidence). Downstream consumers that need a specific
//     field access Item directly instead of parsing a display string.
//   - Score: the relevance score identifyAnswerChains computed. Kept
//     in the envelope because (a) it is committed API semantics
//     ("how well does this chain answer the question"), (b) debug
//     / eval tooling wants to see why the ranking came out this way,
//     (c) recomputing is expensive.
//   - StrictOK: whether the chain passed the L0-1 terminal + origin
//     predicates. The strict subset is used to compute β for Turn B's
//     cardinality validator. Non-strict chains are retained in the
//     slate as Ground Truth fallback but never contribute to β.
type AnswerChain struct {
	Item     EvidenceItem `json:"item"`
	Score    float64      `json:"score"`
	StrictOK bool         `json:"strict_ok"`
}

// MergeAnswerChains combines multiple AnswerChain slices into one,
// preserving first-seen order and dropping duplicates by the chain
// identity tuple (Summary, Source, LineStart, Subject, Predicate,
// Object). The key matches identifyAnswerChains's own dedup key so a
// merged slice has the same cardinality as a freshly-produced one.
// Used by the orchestrator to deduplicate BusContext.AnswerChains on
// stage self-loops where the explorer's ParseOutput re-emits the
// full snapshot each run.
func MergeAnswerChains(groups ...[]AnswerChain) []AnswerChain {
	seen := make(map[string]struct{})
	var out []AnswerChain
	for _, g := range groups {
		for _, c := range g {
			key := fmt.Sprintf("%s|%s|%d|%s|%s|%s",
				c.Item.Summary, c.Item.Source, c.Item.LineStart,
				c.Item.Subject, c.Item.Predicate, c.Item.Object)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

// MergeAnswerSymbols combines multiple AnswerSymbol slices into one,
// preserving first-seen order. Dedup key is a composite of Name +
// File + Line — identity fields the finalizer's translation-mode
// renderer uses to emit a unique bullet. Chain / Kind / Rationale
// are treated as metadata that may vary run-to-run without implying
// a different symbol.
func MergeAnswerSymbols(groups ...[]AnswerSymbol) []AnswerSymbol {
	seen := make(map[string]struct{})
	var out []AnswerSymbol
	for _, g := range groups {
		for _, s := range g {
			key := s.Name + "\x1f" + s.File + "\x1f" + itoa(s.Line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// itoa is a tiny in-package int-to-string to avoid importing strconv
// in this file just for the dedup key builder.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// StableFlowFindingID returns a deterministic ID for a compact
// dataflow finding based on its path/condition shape.
func StableFlowFindingID(path, conditions, sources, sinks []string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		strings.Join(path, "\x1e"),
		strings.Join(conditions, "\x1e"),
		strings.Join(sources, "\x1e"),
		strings.Join(sinks, "\x1e"),
	}, "\x1f")))
	return fmt.Sprintf("flow-%x", h.Sum64())
}
