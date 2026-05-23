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
	Relation TypedRelationKind `json:"relation"`

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

// TypedRelationKind is the closed relation vocabulary shared by
// prompt hints, coverage checks, and future relation providers.
//
// Values are intentionally stable wire strings: existing JSON,
// prompt rendering, and aggregate relation surfaces continue to use
// the same text values while internal code gets a typed boundary.
type TypedRelationKind string

const (
	TypedRelationImplements TypedRelationKind = "implements"
	TypedRelationExtends    TypedRelationKind = "extends"
	TypedRelationCalledBy   TypedRelationKind = "called-by"
	TypedRelationScopedTo   TypedRelationKind = "scoped-to"
	TypedRelationRegisters  TypedRelationKind = "registers"
	TypedRelationOverrides  TypedRelationKind = "overrides"
	TypedRelationReferences TypedRelationKind = "references"
	TypedRelationImports    TypedRelationKind = "imports"
	TypedRelationExports    TypedRelationKind = "exports"
)

// TypedRelationPurpose tells providers whether candidates are being
// assembled for prompt context or for a hard coverage gate. Providers
// may expose softer rows for prompt context, but coverage gates must
// consume exact rows only.
type TypedRelationPurpose string

const (
	TypedRelationPurposePromptHint   TypedRelationPurpose = "prompt_hint"
	TypedRelationPurposeCoverageGate TypedRelationPurpose = "coverage_gate"
)

// TypedRelationPrecision records how trustworthy a relation carrier is.
// Only exact precision levels may participate in hard coverage gates;
// name-only and heuristic rows are guidance for the model, never proof
// that the model is wrong.
type TypedRelationPrecision string

const (
	TypedRelationPrecisionExactSymbolID TypedRelationPrecision = "exact_symbol_id"
	TypedRelationPrecisionExactFile     TypedRelationPrecision = "exact_file"
	TypedRelationPrecisionExactEvidence TypedRelationPrecision = "exact_evidence"
	TypedRelationPrecisionNameOnly      TypedRelationPrecision = "name_only"
	TypedRelationPrecisionHeuristic     TypedRelationPrecision = "heuristic"
)

// CoverageGateEligible reports whether this precision is exact enough
// to support a hard relation coverage check.
func (p TypedRelationPrecision) CoverageGateEligible() bool {
	switch p {
	case TypedRelationPrecisionExactSymbolID,
		TypedRelationPrecisionExactFile,
		TypedRelationPrecisionExactEvidence:
		return true
	default:
		return false
	}
}

// TypedRelationCarrierKind identifies the structural source of a
// relation candidate. It is diagnostic/provenance metadata, not a
// user-facing answer label.
type TypedRelationCarrierKind string

const (
	TypedRelationCarrierGraph               TypedRelationCarrierKind = "graph"
	TypedRelationCarrierMultiGraph          TypedRelationCarrierKind = "multi_graph"
	TypedRelationCarrierEvidence            TypedRelationCarrierKind = "evidence"
	TypedRelationCarrierExternalObservation TypedRelationCarrierKind = "external_observation"
)

// TypedRelationQuery is the cycle-free request passed to relation
// providers. It contains only typed analyzer/request data and scoped
// source names; providers must not inspect raw user prose.
type TypedRelationQuery struct {
	Kinds      []TypedRelationKind  `json:"kinds,omitempty"`
	Sources    []string             `json:"sources,omitempty"`
	Request    *RequestModel        `json:"-"`
	MaxMembers int                  `json:"max_members,omitempty"`
	Purpose    TypedRelationPurpose `json:"purpose,omitempty"`
}

// AllowsKind reports whether a provider should return rows for kind.
// An empty Kinds list means "any supported kind".
func (q TypedRelationQuery) AllowsKind(kind TypedRelationKind) bool {
	if kind == "" {
		return false
	}
	if len(q.Kinds) == 0 {
		return true
	}
	for _, candidate := range q.Kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// TypedRelationCandidate is the internal carrier used by relation
// coverage checks. Prompt rendering may still use TypedRelationHint,
// but both should be derived from the same provider data over time.
type TypedRelationCandidate struct {
	Relation   TypedRelationKind        `json:"relation"`
	SourceName string                   `json:"source_name,omitempty"`
	SourceKind string                   `json:"source_kind,omitempty"`
	SourceFile string                   `json:"source_file,omitempty"`
	SourceLine int                      `json:"source_line,omitempty"`
	Member     TypedRelationMember      `json:"member"`
	Carrier    TypedRelationCarrierKind `json:"carrier,omitempty"`
	Precision  TypedRelationPrecision   `json:"precision,omitempty"`
}

// CoverageGateEligible reports whether the candidate can be considered
// by a hard relation coverage gate. Callers must still verify source
// scope and same-member grounded evidence before rejecting anything.
func (c TypedRelationCandidate) CoverageGateEligible() bool {
	return c.Relation != "" && c.Precision.CoverageGateEligible()
}

// TypedRelationCandidateSource is the common, cycle-free relation
// provider boundary. Multi-repo carriers, external observation ledgers,
// and future graph adapters can implement this without downstream
// packages importing their concrete types.
type TypedRelationCandidateSource interface {
	TypedRelationCandidates(TypedRelationQuery) []TypedRelationCandidate
}

// TypedRelationImplementerSource is the narrow, cycle-free bridge for
// relation consumers that need multi-repo implementer members without
// importing the concrete repomap/multigraph package. Single-repo callers may
// still read *repomap/types.Graph directly; multi-repo carriers implement this
// interface and return path-from-parent file surfaces.
type TypedRelationImplementerSource interface {
	ImplementerMembersOf(interfaceName string) []TypedRelationMember
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
func TypedRelationAnchorKind(relation TypedRelationKind) AnchorKind {
	switch relation {
	case TypedRelationImplements, TypedRelationExtends, TypedRelationScopedTo:
		return AnchorDefinition
	case TypedRelationOverrides, TypedRelationExports:
		return AnchorDefinition
	case TypedRelationCalledBy, TypedRelationReferences:
		return AnchorCall
	case TypedRelationRegisters:
		return AnchorAssignment
	case TypedRelationImports:
		return AnchorImport
	}
	return ""
}

// AllTypedRelations enumerates every Relation value the shared
// relation contract knows about. A value in this list is not
// automatically hard-gate eligible; precision policy and provider
// support decide that per relation family.
func AllTypedRelations() []TypedRelationKind {
	return []TypedRelationKind{
		TypedRelationImplements,
		TypedRelationExtends,
		TypedRelationCalledBy,
		TypedRelationScopedTo,
		TypedRelationRegisters,
		TypedRelationOverrides,
		TypedRelationReferences,
		TypedRelationImports,
		TypedRelationExports,
	}
}
