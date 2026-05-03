package types

// TypedRelationHint is a system-derived structural-relation candidate
// surfaced to the LLM as input HINTING (not as auto-filled answer).
//
// Generalisation of the s5a / LoopController class of bug: when the
// user asks an enumeration / relational-lookup / count question whose
// answer set is defined by a typed graph relation (interface
// implementation, inheritance, callers, scope membership, override,
// reference, registration), grep-based explorer investigation can
// silently miss members (truncation, blank-identifier params,
// cross-file dispatch). The typed graph (repomap) DOES have the
// complete relation; we surface that via this hint so the LLM has
// the full set as a structured input — but the LLM remains the SOLE
// author of doc.Symbols / Summary etc. Per the
// feedback_no_system_backfill_to_user_panel red line, this struct
// flows into the agent's prompt context, NEVER into AnswerDocument.
//
// Render-time merge contract: the prompt assembler unifies hints with
// the LLM-emitted EvidenceItem pool into a SINGLE "Structured
// Evidence" table with a Provenance column distinguishing
// "llm_evidence" (LLM's emit_evidence rows) from "typed_graph" (this
// struct's rows). Dedup by (Subject, Object, AnchorKind) — when both
// sources cover the same tuple the LLM-emit row wins (it carries
// richer rationale).
type TypedRelationHint struct {
	// Relation names the typed graph relation surfaced in this hint.
	// Closed enum: implements / extends / called-by / scoped-to /
	// registers / overrides / references. Adding a new value requires
	// adding a probe in internal/context/typed_relations.go.
	Relation string `json:"relation"`

	// SourceName is the user-named anchor entity the relation is
	// keyed on (interface name / class name / function name / package
	// path). Verbatim from analyzer entities.
	SourceName string `json:"source_name"`

	// SourceKind classifies SourceName's entity kind for prompt
	// rendering ("interface" / "trait" / "protocol" / "class" /
	// "function" / "package" / etc.). Read from typed Symbol.Kind.
	SourceKind string `json:"source_kind"`

	// Members is the deterministic set of relation members. Order is
	// stable (alphabetic by Member.Name). Capped at the probe site to
	// prevent prompt bloat on huge relations.
	Members []TypedRelationMember `json:"members"`
}

// TypedRelationMember is one member of a TypedRelationHint set.
type TypedRelationMember struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
	// Distance encodes transitive-closure depth for relations where
	// depth matters (inheritance chain, call-graph). 1 = direct; 2+ =
	// transitive. Direct-only relations leave it 0 / 1.
	Distance int `json:"distance,omitempty"`
}

// TypedRelationProvenance is the Provenance column tag used in the
// unified Structured Evidence rendering. Two values today: LLM-side
// emit_evidence vs system-derived typed graph traversal.
type TypedRelationProvenance string

const (
	// TypedRelationProvenanceLLMEvidence marks rows derived from LLM
	// emit_evidence. These carry the LLM's authored rationale and
	// flow through the EvidenceClosure ledger.
	TypedRelationProvenanceLLMEvidence TypedRelationProvenance = "llm_evidence"

	// TypedRelationProvenanceTypedGraph marks rows synthesised at
	// prompt-build time from typed graph relation traversal. These do
	// NOT flow through EvidenceClosure (the ledger stays clean as a
	// pure "what the LLM observed" record); they are recomputed each
	// dispatch from the typed graph, so persistence is unnecessary.
	TypedRelationProvenanceTypedGraph TypedRelationProvenance = "typed_graph"
)

// TypedRelationAnchorKind maps a relation tag to the AnchorKind that
// LLM emit_evidence rows would use when describing the same tuple,
// so render-time dedup can match (Subject, Object, AnchorKind)
// across the two provenance types.
//
// Adding a new Relation value requires adding the matching AnchorKind
// row here. The TestTypedRelationAnchorKindCoverage structural test
// (in internal/types/typed_relation_hint_test.go) enforces it.
func TypedRelationAnchorKind(relation string) AnchorKind {
	switch relation {
	case "implements", "extends", "scoped-to":
		return AnchorDefinition
	case "overrides":
		return AnchorDefinition
	case "called-by", "references":
		return AnchorCall
	case "registers":
		return AnchorAssignment
	}
	return ""
}

// AllTypedRelations enumerates every Relation value the probe table
// supports. Mirrors the AllAnchorKinds / AllViolationKinds pattern.
// Used by structural tests to guarantee no relation slips through
// without a probe and an AnchorKind mapping.
func AllTypedRelations() []string {
	return []string{
		"implements",
		"extends",
		"called-by",
		"scoped-to",
		"registers",
		"overrides",
		"references",
	}
}
