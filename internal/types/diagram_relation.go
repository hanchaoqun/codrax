package types

import "strings"

// diagram_relation.go — Phase 3 (V2 runtime consolidation §6) of
// the diagram relation-aware validation rollout. See
// docs/design/v2_runtime_consolidation.md §6.
//
// DiagramRelationKind classifies the BASIC semantic relation that a
// rendered diagram edge asserts. It exists so that the contract
// validator can move beyond endpoint-only grounding ("both nodes
// appear in evidence") to relation-legality ("a guarded edge MUST be
// supported by a guard_condition claim_use").
//
// ── Why a separate enum (vs reusing ClaimForm) ──
//
// ClaimForm is a per-EvidenceItem projection: it answers "what
// sentence shape can this evidence validly support". Diagram edges
// are a different surface — an edge has from/to/label and the label
// is LLM-authored prose, not typed evidence. Mapping the label
// directly onto ClaimForm would conflate two concerns and force
// every edge-grounding lookup through the (heavier) ClaimForm
// projection.
//
// DiagramRelationKind sits as a small typed bridge: label →
// DiagramRelationKind → ClaimForm (via ClaimFormForRelation). The
// label-side parser is the only piece that touches LLM-authored
// strings; everything downstream operates on the typed enum.
//
// ── Red lines (architectural principle) ──
//
//   - The keyword dictionary used by InferRelationFromLabel is a
//     CLOSED set of exact substring matches. NO fuzzy matching, NO
//     frequency weighting, NO machine-learned similarity. An unknown
//     label MUST resolve to DiagramRelUnknown (treated as
//     "label-free edge" — legal so long as endpoint grounding
//     holds).
//   - Hard validator gates may consume DiagramRelationKind because
//     it is a typed enum; they MUST NOT consume the raw label
//     string.
//   - The mapping ClaimFormForRelation MUST stay synchronised with
//     the ClaimForm enum in claim_form.go. Adding a new ClaimForm
//     that an edge can carry requires extending this file too.
//
// SectionDiagramEdgeLabelVocabulary is the canonical sub-section
// title used by the answer-document-skill prompt to introduce the
// label keyword bullet list (Phase 3-C5). Single source of truth so
// LLM-facing prose that REFERENCES the section by name (e.g. the
// orchestrator validator's Repair text) cannot drift from the
// rendered heading.
const SectionDiagramEdgeLabelVocabulary = "Diagram-edge label vocabulary"

type DiagramRelationKind string

const (
	// DiagramRelUnknown is the safe fallback for labels that match
	// none of the canonical keywords (or for unlabelled edges). The
	// validator MUST treat unknown as "label-free": legal when
	// endpoint grounding holds and when the diagram's
	// EdgeRelations[].Min has been satisfied by labelled edges.
	DiagramRelUnknown DiagramRelationKind = ""

	// DiagramRelCall denotes an action-flow edge: caller invokes /
	// dispatches / executes callee. Maps to ClaimCallEdge.
	DiagramRelCall DiagramRelationKind = "call"

	// DiagramRelGuard denotes a conditional branch edge in a flow
	// diagram (e.g., `A -->|if x>0| B`). Maps to
	// ClaimGuardCondition.
	DiagramRelGuard DiagramRelationKind = "guard"

	// DiagramRelImport denotes a static dependency edge between
	// modules / packages / files. Maps to ClaimImportEdge. Note: a
	// runtime call between two packages is DiagramRelCall, not
	// DiagramRelImport.
	DiagramRelImport DiagramRelationKind = "import"

	// DiagramRelPrecedence denotes a layered ordering relation
	// (config trace overrides, before/after roles). Maps to
	// ClaimPrecedenceRole.
	DiagramRelPrecedence DiagramRelationKind = "precedence"

	// DiagramRelContain denotes a hierarchical containment edge in
	// architecture diagrams (subsystem A contains module B). It has
	// NO typed claim_form — containment is a block-level
	// component_relation facet, not an edge-level claim. Returns
	// ClaimUnknown from ClaimFormForRelation.
	DiagramRelContain DiagramRelationKind = "contain"

	// DiagramRelObserve denotes an external-observation edge (a log
	// frame, metric, or trace span attached to a node). Maps to
	// ClaimExternalObservation.
	DiagramRelObserve DiagramRelationKind = "observe"
)

// allDiagramRelationKinds is the canonical iteration order for tests
// and exhaustive switches. DiagramRelUnknown is intentionally
// excluded — it is a sentinel, not a member of the typed set.
var allDiagramRelationKinds = []DiagramRelationKind{
	DiagramRelCall,
	DiagramRelGuard,
	DiagramRelImport,
	DiagramRelPrecedence,
	DiagramRelContain,
	DiagramRelObserve,
}

// AllDiagramRelationKinds returns the canonical iteration order.
// The returned slice is a copy and safe to mutate.
func AllDiagramRelationKinds() []DiagramRelationKind {
	return append([]DiagramRelationKind(nil), allDiagramRelationKinds...)
}

// IsValid reports whether rk is a known non-empty
// DiagramRelationKind. DiagramRelUnknown is intentionally not
// "valid" — it is the sentinel for absence.
func (rk DiagramRelationKind) IsValid() bool {
	for _, v := range allDiagramRelationKinds {
		if v == rk {
			return true
		}
	}
	return false
}

// ClaimFormForRelation maps a relation kind onto the ClaimForm that
// an edge of this kind requires from its supporting claim_use.
// Returns ClaimUnknown for relations without a typed claim form
// (DiagramRelContain — containment is a block-level facet, not an
// edge claim) and for DiagramRelUnknown.
func ClaimFormForRelation(rk DiagramRelationKind) ClaimForm {
	switch rk {
	case DiagramRelCall:
		return ClaimCallEdge
	case DiagramRelGuard:
		return ClaimGuardCondition
	case DiagramRelImport:
		return ClaimImportEdge
	case DiagramRelPrecedence:
		return ClaimPrecedenceRole
	case DiagramRelObserve:
		return ClaimExternalObservation
	}
	return ClaimUnknown
}

// RelationForClaimForm is the inverse bridge for edge-capable claim
// forms. It upgrades an already-typed edge claim into the basic
// relation kind even when the LLM omitted relation_kind on
// edge_anchors[]. Returns DiagramRelUnknown for non-edge claim forms.
func RelationForClaimForm(cf ClaimForm) DiagramRelationKind {
	switch cf {
	case ClaimCallEdge:
		return DiagramRelCall
	case ClaimGuardCondition:
		return DiagramRelGuard
	case ClaimImportEdge:
		return DiagramRelImport
	case ClaimPrecedenceRole:
		return DiagramRelPrecedence
	case ClaimExternalObservation:
		return DiagramRelObserve
	}
	return DiagramRelUnknown
}

// diagramRelationKeywords is the canonical exact-substring keyword
// dictionary consumed by InferRelationFromLabel. Each entry
// associates a lowercase keyword with the relation it asserts.
//
// Order matters: scanning is priority-first, so more specific
// keywords (guard markers) precede generic ones (call verbs). A
// label that contains BOTH "if" and "call" resolves to
// DiagramRelGuard because the conditional marker is the dominant
// semantic.
//
// Synchronisation:
//   - Adding / removing keywords MUST be reflected in the
//     skill-prompt list shown to the LLM (see skill/defaults.go in
//     C5). LLM-facing prompts read this dictionary as their authority.
//   - When the ClaimForm enum gains a new edge-capable form, extend
//     this dictionary AND ClaimFormForRelation in the same patch.
var diagramRelationKeywords = []struct {
	keyword string
	kind    DiagramRelationKind
}{
	// Guard / decision (most specific — wins over generic verbs)
	{"guard", DiagramRelGuard},
	{"if ", DiagramRelGuard},
	{"when", DiagramRelGuard},
	{"cond", DiagramRelGuard},
	{"branch", DiagramRelGuard},

	// Precedence / ordering
	{"precedence", DiagramRelPrecedence},
	{"override", DiagramRelPrecedence},
	{"layer", DiagramRelPrecedence},

	// Import / dependency
	{"import", DiagramRelImport},
	{"depends", DiagramRelImport},
	{"requires", DiagramRelImport},

	// External observation
	{"observe", DiagramRelObserve},
	{"observ", DiagramRelObserve},
	{"log", DiagramRelObserve},
	{"metric", DiagramRelObserve},
	{"trace", DiagramRelObserve},

	// Containment
	{"contain", DiagramRelContain},
	{"include", DiagramRelContain},
	{"part of", DiagramRelContain},

	// Call / invocation (lowest priority — generic verbs)
	{"call", DiagramRelCall},
	{"invoke", DiagramRelCall},
	{"dispatch", DiagramRelCall},
	{"send", DiagramRelCall},
}

// InferRelationFromLabel scans a verbatim edge label and returns
// the matched relation kind. The match is an exact substring against
// the canonical lowercase keyword set ONLY — no tokenisation, no
// fuzzy matching, no frequency weighting. Returns DiagramRelUnknown
// for labels that match no keyword and for empty / whitespace-only
// labels.
//
// Callers should treat DiagramRelUnknown as "label-free edge" — it
// is a legitimate state that does not by itself constitute a
// validator violation.
func InferRelationFromLabel(label string) DiagramRelationKind {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return DiagramRelUnknown
	}
	lower := strings.ToLower(trimmed)
	for _, entry := range diagramRelationKeywords {
		if strings.Contains(lower, entry.keyword) {
			return entry.kind
		}
	}
	return DiagramRelUnknown
}

// DiagramRelationKeywords returns the canonical (keyword, kind)
// dictionary in scanning-priority order. The returned slice is a
// copy and safe to mutate. Skill prompts read this to enumerate the
// label vocabulary the LLM is expected to use.
func DiagramRelationKeywords() []struct {
	Keyword string
	Kind    DiagramRelationKind
} {
	out := make([]struct {
		Keyword string
		Kind    DiagramRelationKind
	}, 0, len(diagramRelationKeywords))
	for _, e := range diagramRelationKeywords {
		out = append(out, struct {
			Keyword string
			Kind    DiagramRelationKind
		}{Keyword: e.keyword, Kind: e.kind})
	}
	return out
}
